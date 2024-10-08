package main

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
	gocmp "github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	ollamav1alpha1 "aerf.io/ollama-operator/apis/ollama/v1alpha1"
	modelcontroller "aerf.io/ollama-operator/internal/controllers/model"
)

var cli struct {
	File     []byte `arg:"" type:"filecontent" help:"input yaml/json with Model CR. Accepts file piped to this binary if '-' is passed as argument"`
	OnlyDiff bool   `help:"Print only diff after applying patches"`
}

func main() {
	kctx := kong.Parse(&cli,
		kong.Name("mergepatcher"),
		kong.Description("A CLI to test merge-patching functionality of Model CRD"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Summary: true,
		}),
	)
	unstr := &unstructured.Unstructured{}
	kctx.FatalIfErrorf(yaml.Unmarshal(cli.File, unstr), "unable to unmarshal file content to %T", unstr)

	model := &ollamav1alpha1.Model{}
	err := yaml.UnmarshalStrict(cli.File, model)
	kctx.FatalIfErrorf(err, "unable to unmarshal file content to %T", model)

	model.Namespace = cmp.Or(model.Namespace, "default")

	if cli.OnlyDiff {
		modelNoPatches := model.DeepCopy()
		modelNoPatches.Spec.ServicePatches = nil
		modelNoPatches.Spec.StatefulSetPatches = nil

		noPatchesResources, err := modelcontroller.Resources(modelNoPatches)
		kctx.FatalIfErrorf(err, "unable to create resources out of model instance")

		resources, err := modelcontroller.Resources(model)
		kctx.FatalIfErrorf(err, "unable to create resources out of model instance")

		fmt.Println(gocmp.Diff(noPatchesResources, resources))

	} else {
		res, err := modelcontroller.Resources(model)
		kctx.FatalIfErrorf(err, "unable to create resource out of model instance")
		kctx.FatalIfErrorf(printObjects(res), "unable to print child objects")
	}
}

func printObjects(objs []*unstructured.Unstructured) error {
	buf := make([]string, 0, len(objs))
	for _, obj := range objs {
		out, err := yaml.Marshal(obj)
		if err != nil {
			return err
		}
		buf = append(buf, string(out))
	}

	fmt.Println(strings.TrimSuffix(strings.Join(buf, "---\n"), "\n"))
	return nil
}
