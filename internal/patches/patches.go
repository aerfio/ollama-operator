package patches

import (
	"encoding/json"
	"fmt"
	"reflect"

	"dario.cat/mergo"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	jsonpatch "github.com/evanphx/json-patch/v5"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	applyappsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	kjson "sigs.k8s.io/json"
	"sigs.k8s.io/yaml"

	cmnv1alpha1 "aerf.io/ollama-operator/apis/common/v1alpha1"
)

func Apply(resource any, patches *cmnv1alpha1.Patches) (any, error) {
	if patches == nil {
		return resource, nil
	}
	merged, err := ApplyMerge(resource, &patches.MergePatch)
	if err != nil {
		return nil, fmt.Errorf("while applying merge patch: %s", err)
	}
	jsonPatched, err := ApplyJSONPatch(merged, &patches.JSONPatch)
	if err != nil {
		return nil, fmt.Errorf("while applying JSON patch: %s", err)
	}

	smpPatched, err := ApplyStrategicMergePatch(jsonPatched, &patches.StrategicMergePatch)
	if err != nil {
		return nil, fmt.Errorf("while applying StrategicMergePatch: %s", err)
	}
	return smpPatched, nil
}

func ApplyMerge(obj any, patches *cmnv1alpha1.MergePatch) (any, error) {
	if patches == nil || patches.MergePatch == nil {
		return obj, nil
	}

	emptyObj := reflect.New(reflect.TypeOf(obj).Elem()).Interface()
	if _, objIsUnstructured := obj.(*unstructured.Unstructured); objIsUnstructured {
		var err error
		// prevents "Object 'Kind' is missing in " when unmarshalling patch into emptyObj
		emptyObj, err = patches.PatchToUnstructured()
		if err != nil {
			return nil, fmt.Errorf("while converting patch to unstructured object: %s", err)
		}
	} else {
		marshalledPatch, err := patches.MergePatch.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal merge patch: %s", err)
		}

		strictErr, err := kjson.UnmarshalStrict(marshalledPatch, &emptyObj)
		err = errors.Join(append(strictErr, err)...)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal merge patch: %s", err)
		}
	}

	return obj, errors.WithMessage(mergo.Merge(obj, emptyObj, mergo.WithOverride), "failed to apply merge patch")
}

func ApplyJSONPatch(obj any, patches *cmnv1alpha1.JSONPatch) (any, error) {
	if patches == nil || len(patches.JSONPatch) == 0 {
		return obj, nil
	}

	marshalledPatchDoc, err := json.Marshal(patches.JSONPatch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON patch: %s", err)
	}
	decodedPatchDoc, err := jsonpatch.DecodePatch(marshalledPatchDoc)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JSON patch: %s", err)
	}

	marshalledResource, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object to patch into JSON: %s", err)
	}
	patchedResource, err := decodedPatchDoc.ApplyWithOptions(marshalledResource, &jsonpatch.ApplyOptions{
		SupportNegativeIndices:   false,
		AllowMissingPathOnRemove: false,
		EnsurePathExistsOnAdd:    true,
		EscapeHTML:               false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to apply JSON patch: %s", err)
	}

	emptyObj := reflect.New(reflect.TypeOf(obj).Elem()).Interface()
	if err := json.Unmarshal(patchedResource, emptyObj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON patched object into empty object with the same type as input, err: %s, resource: %s", err, patchedResource)
	}
	return emptyObj, nil
}

func ApplyStrategicMergePatch(obj any, patches *cmnv1alpha1.StrategicMergePatch) (any, error) {
	if patches == nil || patches.StrategicMergePatch == nil {
		return obj, nil
	}
	marshalledObj, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object to patch into JSON: %s", err)
	}

	marshalledSmp, err := patches.StrategicMergePatch.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal strategic merge patch: %s", err)
	}

	emptyObj := reflect.New(reflect.TypeOf(obj).Elem()).Interface()
	out, err := strategicpatch.StrategicMergePatch(marshalledObj, marshalledSmp, converFromApplyType(obj))
	if err != nil {
		return nil, fmt.Errorf("failed to apply strategic merge patch: %s", err)
	}

	err = yaml.UnmarshalStrict(out, emptyObj)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal bytes: %s", err)
	}
	return emptyObj, nil
}

func converFromApplyType(arg any) any {
	switch arg.(type) {
	case *applyappsv1.StatefulSetApplyConfiguration:
		return &appsv1.StatefulSet{}
	case *applycorev1.ServiceApplyConfiguration:
		return &corev1.Service{}
	default:
		panic(fmt.Errorf("unknown type %T, conversion function not recognized, please create an issue in the proejct repository, this is a bug", arg))
	}
}
