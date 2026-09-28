/*
Copyright 2026 The KubeVela Authors.

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

package cuex_test

import (
	"context"
	"testing"

	"github.com/kubevela/pkg/cue/cuex"
)

// paramTemplate declares its parameters, the way a component or workflow step
// definition does.
const paramTemplate = `import "vela/base64"
parameter: {
	input:     string
	name:      string
	namespace: string
	image:     string
	labels: [string]: string
	env: [...{name: string, value: string}]
	resources: {...}
}
enc: base64.#Encode & {$params: parameter.input}
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {name: parameter.name, namespace: parameter.namespace, labels: parameter.labels}
	spec: template: spec: containers: [{
		name:      parameter.name
		image:     parameter.image
		env:       parameter.env
		resources: parameter.resources
	}]
}`

// a parameter block the size a real render carries
var benchParams = map[string]interface{}{
	"input":     "hello",
	"name":      "my-app",
	"namespace": "default",
	"image":     "registry.example.com/org/image:v1.2.3",
	"labels":    map[string]string{"a": "1", "b": "2", "c": "3", "d": "4"},
	"env": []map[string]string{
		{"name": "A", "value": "1"},
		{"name": "B", "value": "2"},
	},
	"resources": map[string]interface{}{
		"limits":   map[string]string{"cpu": "1", "memory": "1Gi"},
		"requests": map[string]string{"cpu": "100m", "memory": "128Mi"},
	},
}

// BenchmarkWithExtraData renders the parameters to CUE text and hands them to
// the parser with the template.
func BenchmarkWithExtraData(b *testing.B) {
	benchData(b, cuex.WithExtraData("parameter", benchParams))
}

// BenchmarkWithData fills the parameters into the compiled value.
func BenchmarkWithData(b *testing.B) {
	benchData(b, cuex.WithData("parameter", benchParams))
}

// BenchmarkWithDataFallback is WithData on a template that does not declare
// the field, so it takes the same path WithExtraData always takes.
func BenchmarkWithDataFallback(b *testing.B) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	src := `out: parameter.name`
	opt := cuex.WithData("parameter", benchParams)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.CompileStringWithOptions(ctx, src, opt); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkNoData is a floor for what a compile costs with neither option: a
// two line template, not paramTemplate with its parameters inlined, so it is
// not a like for like against the two above. Compile cost follows the size of
// the template, and this one is much smaller than theirs.
func BenchmarkNoData(b *testing.B) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	b.ReportAllocs()
	// registering the default packages is setup, and the benchmarks this is
	// compared against exclude theirs
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.CompileString(ctx, `import "vela/base64"
enc: base64.#Encode & {$params: "hello"}`); err != nil {
			b.Fatal(err)
		}
	}
}

func benchData(b *testing.B, opt cuex.CompileOption) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.CompileStringWithOptions(ctx, paramTemplate, opt); err != nil {
			b.Fatal(err)
		}
	}
}
