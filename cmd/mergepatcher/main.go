package main

import (
	"bytes"
	"cmp"
	"fmt"

	"github.com/alecthomas/kong"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	ollamav1alpha1 "aerf.io/ollama-operator/apis/ollama/v1alpha1"
	"aerf.io/ollama-operator/internal/controllers"
)

var cli struct {
	File []byte `arg:"" type:"filecontent" help:"input yaml/json with Model CR. Accepts file piped to this binary if '-' is passed as argument"`
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
	kctx.FatalIfErrorf(runtime.DefaultUnstructuredConverter.FromUnstructuredWithValidation(unstr.Object, model, true), "unable to unmarshal file content to %T", model)

	model.Namespace = cmp.Or(model.Namespace, "default")

	res, err := controllers.ResourcesFromModel(model)
	kctx.FatalIfErrorf(err, "unable to create resource out of model instance")
	kctx.FatalIfErrorf(printObjects(res), "unable to print child objects")
}

func printObjects(objs []*unstructured.Unstructured) error {
	buf := make([][]byte, 0)
	for _, obj := range objs {
		out, err := yaml.Marshal(obj)
		if err != nil {
			return err
		}
		buf = append(buf, out)
	}

	fmt.Println(string(bytes.TrimSuffix(bytes.Join(buf, []byte("---\n")), []byte("\n"))))
	return nil
}
