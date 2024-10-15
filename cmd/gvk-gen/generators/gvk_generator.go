/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package generators

import (
	"io"
	"sort"

	clientgentypes "k8s.io/code-generator/cmd/client-gen/types"
	"k8s.io/gengo/v2/generator"
	"k8s.io/gengo/v2/namer"
	"k8s.io/gengo/v2/types"
	"k8s.io/klog/v2"
)

type registerExternalGenerator struct {
	generator.GoGenerator
	outputPackage   string
	gv              clientgentypes.GroupVersion
	typesToGenerate []*types.Type
	imports         namer.ImportTracker
}

var _ generator.Generator = &registerExternalGenerator{}

func (g *registerExternalGenerator) Filter(_ *generator.Context, _ *types.Type) bool {
	return false
}

func (g *registerExternalGenerator) Imports(c *generator.Context) (imports []string) {
	return g.imports.ImportLines()
}

func (g *registerExternalGenerator) Namers(_ *generator.Context) namer.NameSystems {
	return namer.NameSystems{
		"raw": namer.NewRawNamer(g.outputPackage, g.imports),
	}
}

func (g *registerExternalGenerator) Finalize(context *generator.Context, w io.Writer) error {
	typesToGenerateOnlyNames := make([]string, len(g.typesToGenerate))
	for index, typeToGenerate := range g.typesToGenerate {
		typesToGenerateOnlyNames[index] = typeToGenerate.Name.Name
	}

	// sort the list of types to register, so that the generator produces stable output
	sort.Strings(typesToGenerateOnlyNames)

	sw := generator.NewSnippetWriter(w, context, "$", "$")
	m := map[string]any{
		// "groupName":         g.gv.Group,
		// "version":           g.gv.Version,
		"types": typesToGenerateOnlyNames,
		// "addToGroupVersion": context.Universe.Function(types.Name{Package: "k8s.io/apimachinery/pkg/apis/meta/v1", Name: "AddToGroupVersion"}),
		// "groupVersion":      context.Universe.Type(types.Name{Package: "k8s.io/apimachinery/pkg/apis/meta/v1", Name: "GroupVersion"}),
	}
	klog.InfoS("generating output file")
	sw.Do(registerExternalTypesTemplate, m)
	return sw.Error()
}

var registerExternalTypesTemplate = `
const (
$range .types -$
	$.$Kind = "$.$"
$end$
)
var (
$range .types -$
	$.$GroupVersionKind = SchemeGroupVersion.WithKind($.$Kind)
$end$
)
`
