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

package cuex

import (
	"context"
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"github.com/stretchr/testify/require"
)

// The question the whole idea rests on: do the parameters an iteration
// works out on its own match the ones the resolver works out with the
// whole file in front of it?
//
// Asked by running both. The resolver renders the template and its calls
// are read for what they were made with; the document builder does each
// iteration alone and its call is read for the same thing. Anything that
// differs is the prepass being wrong, and it is the only kind of wrong
// that does not announce itself.

// paramsPerIteration builds a document for each iteration of the annotated
// field and reads the parameters out of it.
func paramsPerIteration(t *testing.T, c *Compiler, src string, n int) []string {
	t.Helper()
	f, err := parser.ParseFile("-", src, parser.ParseComments)
	require.NoError(t, err)
	scope, ok := scopeOfFile(f)
	require.True(t, ok, "the file's top level should be nameable")

	var field *ast.Field
	for _, d := range f.Decls {
		fd, is := d.(*ast.Field)
		if !is {
			continue
		}
		if name, _, err := ast.LabelName(fd.Label); err == nil && name == "_calls" {
			field = fd
		}
	}
	require.NotNil(t, field, "the template should hold the loop under _calls")

	comp, ok := loopOf(field)
	require.True(t, ok, "the annotated field should hold a loop")

	got := make([]string, n)
	for i := 0; i < n; i++ {
		doc, ok := iterationDoc(scope, comp, []ast.Expr{ast.NewLit(token.INT, fmt.Sprint(i))}, nil, n, nil, nil)
		require.True(t, ok, "iteration %d should be buildable", i)

		bs, err := format.Node(doc)
		require.NoError(t, err)
		if i == 0 {
			t.Logf("the document for one iteration:\n%s", bs)
		}

		bi := build.NewContext().NewInstance("", nil)
		bi.Imports = c.PackageManager.GetImports()
		require.NoError(t, bi.AddSyntax(doc))
		v := cuecontext.New().BuildInstance(bi)
		require.NoError(t, v.Err(), "iteration %d", i)

		// the one field the loop produced, whatever it is called
		it, err := v.LookupPath(cue.MakePath(cue.Hid(iterationField, "_"))).Fields(cue.All())
		require.NoError(t, err, "iteration %d", i)
		require.True(t, it.Next(), "iteration %d produced nothing", i)
		params, err := it.Value().LookupPath(cue.MakePath(cue.Str(paramsKey))).MarshalJSON()
		require.NoError(t, err, "iteration %d", i)
		got[i] = string(params)
		require.False(t, it.Next(), "iteration %d produced more than one", i)
	}
	return got
}

// paramsFromWholeFile is the same parameters as the resolver sees them,
// with the calls left unrun so nothing has been substituted.
func paramsFromWholeFile(t *testing.T, c *Compiler, src, field string, n int) []string {
	t.Helper()
	v, err := c.CompileStringWithOptions(context.Background(), src,
		DisableResolveProviderFunctions{})
	require.NoError(t, err)
	got := make([]string, n)
	for i := 0; i < n; i++ {
		params, err := v.LookupPath(cue.MakePath(
			cue.Hid(field, "_"), cue.Str(fmt.Sprint(i)), cue.Str(paramsKey))).MarshalJSON()
		require.NoError(t, err, "iteration %d", i)
		got[i] = string(params)
	}
	return got
}

func TestIterationParamsMatchTheWholeFile(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	const n = 5

	for _, tc := range []struct {
		name, field, src string
	}{
		{
			name:  "parameters built from the loop variable",
			field: "_calls",
			src: `
import "vela/base64"
import "list"
_idx: list.Range(0, 5, 1)
_calls: {
	for i in _idx {
		"\(i)": base64.#Encode & {$params: "seed-\(i)"}
	}
}
`,
		},
		{
			name:  "parameters reading another field",
			field: "_calls",
			src: `
import "vela/base64"
import "list"
_idx:    list.Range(0, 5, 1)
_prefix: "p"
_calls: {
	for i in _idx {
		"\(i)": base64.#Encode & {$params: "\(_prefix)-\(i)"}
	}
}
`,
		},
		{
			name:  "a field written twice, which is the hazard",
			field: "_calls",
			src: `
import "vela/base64"
import "list"
_idx: list.Range(0, 5, 1)
cfg: {a: "x"}
cfg: {b: "y"}
_calls: {
	for i in _idx {
		"\(i)": base64.#Encode & {$params: "\(cfg.a)\(cfg.b)-\(i)"}
	}
}
`,
		},
		{
			name:  "a let inside the loop",
			field: "_calls",
			src: `
import "vela/base64"
import "list"
_idx: list.Range(0, 5, 1)
_calls: {
	for i in _idx {
		let doubled = i * 2
		"\(i)": base64.#Encode & {$params: "n-\(doubled)"}
	}
}
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := paramsFromWholeFile(t, c, tc.src, tc.field, n)
			got := paramsPerIteration(t, c, tc.src, n)
			require.Equal(t, want, got,
				"an iteration on its own must ask for what the whole file asks for")
			t.Logf("PARAMS %v", got)
		})
	}
}
