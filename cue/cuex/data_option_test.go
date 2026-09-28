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

	"github.com/stretchr/testify/require"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"

	"github.com/kubevela/pkg/cue/cuex"
)

// dataCase is a template and the value supplied for one of its fields.
type dataCase struct {
	template string
	key      string
	data     interface{}
}

var dataCorpus = map[string]dataCase{
	"declared parameter read by a call": {
		template: `import "vela/base64"
parameter: input: string
enc: base64.#Encode & {$params: parameter.input}
out: enc.$returns`,
		key:  "parameter",
		data: map[string]interface{}{"input": "example"},
	},

	"declared parameter, no call": {
		template: `parameter: input: string
out: parameter.input`,
		key:  "parameter",
		data: map[string]interface{}{"input": "plain"},
	},

	"declared parameter with a default": {
		template: `parameter: {replicas: *1 | int, name: string}
out: {n: parameter.name, r: parameter.replicas}`,
		key:  "parameter",
		data: map[string]interface{}{"name": "app"},
	},

	"declared parameter, nested data": {
		template: `parameter: {...}
out: parameter`,
		key: "parameter",
		data: map[string]interface{}{
			"labels":  map[string]string{"a": "1", "b": "2"},
			"ports":   []int{80, 443},
			"nested":  map[string]interface{}{"deep": map[string]string{"k": "v"}},
			"enabled": true,
		},
	},

	"declared parameter, RawExtension": {
		template: `import "vela/base64"
parameter: input: string
enc: base64.#Encode & {$params: parameter.input}`,
		key:  "parameter",
		data: &kuberuntime.RawExtension{Raw: []byte(`{"input": "example"}`)},
	},

	"declared parameter, nil data": {
		template: `parameter: {...}
a: parameter`,
		key:  "parameter",
		data: (*kuberuntime.RawExtension)(nil),
	},

	"declared parameter, dotted key": {
		template: `parameter: {...}
a: parameter.nested`,
		key:  "parameter.nested",
		data: "value",
	},

	// the fallback: nothing declares the field, so it has to arrive as source
	"undeclared parameter": {
		template: `a: parameter`,
		key:      "parameter",
		data:     map[string]interface{}{"x": "y"},
	},

	"undeclared parameter, dotted key": {
		template: `a: parameter.nested`,
		key:      "parameter.nested",
		data:     "value",
	},

	"undeclared parameter, nil data": {
		template: `a: parameter`,
		key:      "parameter",
		data:     (*kuberuntime.RawExtension)(nil),
	},

	"quoted string label key": {
		template: `"my-param": {...}
out: "my-param"`,
		key:  `"my-param"`,
		data: map[string]interface{}{"k": "v"},
	},

	// A key that is not an addressable CUE path belongs in
	// TestPartitionDataRoutes rather than here. On the fallback both options
	// run the same FillPath and ToString, so comparing them compares one
	// implementation against itself and the case could never fail.
}

// TestWithDataMatchesWithExtraData is the regression net for the faster path:
// whatever WithExtraData renders, WithData has to render the same, whether it
// fills the value or falls back to appending source.
func TestWithDataMatchesWithExtraData(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	for name, tc := range dataCorpus {
		t.Run(name, func(t *testing.T) {
			wantOut, wantErr := render(c.CompileStringWithOptions(ctx, tc.template, cuex.WithExtraData(tc.key, tc.data)))
			gotOut, gotErr := render(c.CompileStringWithOptions(ctx, tc.template, cuex.WithData(tc.key, tc.data)))
			require.Equal(t, wantErr, gotErr, "error differs from WithExtraData")
			require.Equal(t, wantOut, gotOut, "rendered output differs from WithExtraData")
		})
	}
}

// TestWithDataMultiple covers several values in one compile, where some can be
// filled and some cannot.
func TestWithDataMultiple(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	template := `parameter: name: string
out: {n: parameter.name, c: context.namespace}`

	opts := func(with func(string, interface{}) cuex.CompileOption) []cuex.CompileOption {
		return []cuex.CompileOption{
			with("parameter", map[string]interface{}{"name": "app"}),
			with("context", map[string]interface{}{"namespace": "default"}),
		}
	}
	wantOut, wantErr := render(c.CompileStringWithOptions(ctx, template, opts(cuex.WithExtraData)...))
	gotOut, gotErr := render(c.CompileStringWithOptions(ctx, template, opts(cuex.WithData)...))
	require.Equal(t, wantErr, gotErr)
	require.Equal(t, wantOut, gotOut)
	require.Contains(t, gotOut, `"namespace":"default"`, "the undeclared value still arrived")
	require.Contains(t, gotOut, `"n":"app"`, "the declared value still arrived")
}
