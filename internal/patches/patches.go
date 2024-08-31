package patches

import (
	"encoding/json"
	"fmt"
	"reflect"

	"dario.cat/mergo"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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
	return jsonPatched, nil
}

func ApplyMerge(obj any, patches *cmnv1alpha1.MergePatch) (any, error) {
	if patches == nil || patches.Patch == nil {
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
		marshalledPatch, err := patches.Patch.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal merge patch: %s", err)
		}

		strictErr, err := json.Unmarshal(marshalledPatch, &emptyObj)
		err = errors.Join(append(strictErr, err)...)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal merge patch: %s", err)
		}
	}

	return obj, errors.WithMessage(mergo.Merge(obj, emptyObj, mergo.WithOverride), "failed to apply merge patch")
}

func ApplyJSONPatch(obj any, patches *cmnv1alpha1.JSONPatch) (any, error) {
	if patches == nil || len(patches.Patch) == 0 {
		return obj, nil
	}

	marshalledPatchDoc, err := json.Marshal(patches.Patch)
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
