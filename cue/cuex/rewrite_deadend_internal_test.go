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
	"encoding/base64"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"github.com/stretchr/testify/require"
)

// Why an answered call cannot be cut down to its answer by rewriting syntax.
//
// A resolved call node keeps what was needed to make the call: the #do and
// #provider that named it. Nothing reads those once the answer is in, and at
// 4000 calls they are 5.4KB of the 8.4KB a node costs. A node cannot have a
// conjunct taken off it, so the only way to be rid of them is to write the
// value out as syntax, cut them there, and build it again.
//
// These two tests are why that does not work. They are kept so the approach
// is not reopened by reasoning about it rather than trying it.

func hiddenCallSrc() string {
	return `
import "vela/base64"
import "list"
_idx: list.Range(0, 3, 1)
_s: {for i in _idx {"\(i)": base64.#Encode & {$params: "seed-\(i)"}}}
out: {for i in _idx {"\(i)": _s["\(i)"].$returns}}
`
}

func asFile(t *testing.T, node ast.Node) *ast.File {
	t.Helper()
	switch n := node.(type) {
	case *ast.File:
		return n
	case *ast.StructLit:
		return &ast.File{Decls: n.Elts}
	}
	expr, ok := node.(ast.Expr)
	require.True(t, ok, "%T is neither a file nor an expression", node)
	return &ast.File{Decls: []ast.Decl{&ast.EmbedDecl{Expr: expr}}}
}

// The two views a rewrite could use are mutually exclusive. One keeps the
// hidden fields a call lives in and cannot be built without the provider
// packages; the other builds anywhere because it has dropped the hidden
// fields, and with them every call there was to trim.
//
// Building somewhere new is not optional: a context holds on to what has
// been built in it, so rebuilding a trimmed value in the context it came
// from keeps the untrimmed one too and reclaims almost nothing.
func TestNoSyntaxViewKeepsCallsAndRebuilds(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	v, err := c.CompileString(context.Background(), hiddenCallSrc())
	require.NoError(t, err)
	require.True(t, v.LookupPath(cue.ParsePath(`_s["0"].$returns`)).Exists(),
		"the value under test has a hidden call in it")

	for _, opt := range []struct {
		name string
		opts []cue.Option
	}{
		{"All", []cue.Option{cue.All()}},
		{"All+Resolved", []cue.Option{cue.All(), cue.ResolveReferences(true)}},
		{"Hidden+Defs+Final", []cue.Option{cue.Hidden(true), cue.Definitions(true), cue.Final()}},
		{"Raw+Hidden+Defs", []cue.Option{cue.Raw(), cue.Hidden(true), cue.Definitions(true)}},
	} {
		bs, err := format.Node(v.Syntax(opt.opts...))
		require.NoError(t, err, opt.name)
		keptHidden := strings.Contains(string(bs), "_s")
		rebuilds := cuecontext.New().CompileBytes(bs).Err() == nil

		t.Logf("VIEW %-20s %4d bytes  keeps the hidden call=%-5v  builds on its own=%v",
			opt.name, len(bs), keptHidden, rebuilds)
		require.False(t, keptHidden && rebuilds,
			"%s would make the rewrite possible, so it is worth revisiting", opt.name)
	}
}

// And the view that keeps the calls writes out the conjuncts a field was
// built from, not what it evaluated to. The comprehension that produced the
// calls is still in the text beside the answers it produced, so there is no
// call node to cut, and building the text again runs the comprehension
// again. The resolver is handed calls that have already run.
//
// Against a provider that formats a string that is waste. Against vela/kube
// it is a second write.
func TestRebuildingRunsTheComprehensionAgain(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	v, err := c.CompileString(context.Background(), hiddenCallSrc())
	require.NoError(t, err)

	emitted, err := format.Node(v.Syntax(cue.All()))
	require.NoError(t, err)
	require.Contains(t, string(emitted), "for i in _idx",
		"the comprehension is written out beside the answers it produced")
	require.NotContains(t, string(emitted), "#do",
		"and the call is a reference to the definition, not a struct with fields to cut")

	bi := build.NewContext().NewInstance("", nil)
	bi.Imports = c.PackageManager.GetImports()
	require.NoError(t, bi.AddSyntax(asFile(t, v.Syntax(cue.All()))))
	rebuilt := cuecontext.New().BuildInstance(bi)
	require.NoError(t, rebuilt.Err())

	// the answers are still right, which is what makes this quiet
	got, err := rebuilt.LookupPath(cue.ParsePath(`out["0"]`)).String()
	require.NoError(t, err)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte("seed-0")), got)

	pending := pendingCalls(rebuilt, map[string]bool{}, nil)
	t.Logf("a walk of the rebuilt value finds %d calls, every one already answered", len(pending))
	require.NotEmpty(t, pending,
		"if a rebuild stops handing back answered calls, this is worth revisiting")
}
