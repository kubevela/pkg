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
	"cuelang.org/go/cue/parser"
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

// Which syntax views keep the hidden fields a call lives in, and which of
// those will build again.
//
// Building somewhere new is not optional: a context holds on to what has
// been built in it, so rebuilding a trimmed value in the context it came
// from keeps the untrimmed one too and reclaims almost nothing.
//
// cue.All() does both, and this test used to report that nothing did. It
// built the bytes with no provider packages, so every view failed to
// resolve vela/base64 and read as "cannot rebuild" whatever it held.
// Rebuilding was never the bar anyway: a view that parses and builds can
// still be a different value, which is a trap this package fell into once
// already. What rules the rewrite out is the next test, where a rebuilt
// value hands back calls that already hold their answers.
func TestWhichSyntaxViewsKeepAHiddenCall(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	v, err := c.CompileString(context.Background(), hiddenCallSrc())
	require.NoError(t, err)
	require.True(t, v.LookupPath(cue.ParsePath(`_s["0"].$returns`)).Exists(),
		"the value under test has a hidden call in it")

	seen := map[string]view{}
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
		// Built the way a render builds, against the provider packages.
		// Compiling the bytes on their own cannot resolve vela/base64, so
		// it failed for that reason whatever the view held, and this read
		// as "never rebuildable" no matter what.
		rebuilds := func() bool {
			rebuild := build.NewContext().NewInstance("", nil)
			rebuild.Imports = c.PackageManager.GetImports()
			file, err := parser.ParseFile("-", bs, parser.ParseComments)
			if err != nil {
				return false
			}
			if err := rebuild.AddSyntax(file); err != nil {
				return false
			}
			return cuecontext.New().BuildInstance(rebuild).Err() == nil
		}()

		t.Logf("VIEW %-20s %4d bytes  keeps the hidden call=%-5v  builds on its own=%v",
			opt.name, len(bs), keptHidden, rebuilds)
		seen[opt.name] = view{keptHidden: keptHidden, rebuilds: rebuilds}
	}

	// Pinned, so a change in what CUE's views carry shows up here rather
	// than somewhere it would read as our own doing.
	require.Equal(t, view{keptHidden: true, rebuilds: true}, seen["All"],
		"All keeps the hidden call and builds again")
	require.False(t, seen["All+Resolved"].keptHidden,
		"resolving references drops the hidden fields, and the calls with them")
}

// view is what one syntax view of a value turned out to carry.
type view struct {
	keptHidden bool
	rebuilds   bool
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

	// Nothing is marked executed, because nothing can be: what the
	// resolver ran is not written into the value and does not survive a
	// rebuild. That is the dead end. So the walk finds these calls, and
	// the point is what it finds them in: nodes that already hold their
	// answer and still read as calls to make.
	pending := pendingCalls(rebuilt, map[string]bool{}, nil)
	t.Logf("a walk of the rebuilt value finds %d calls, every one already answered", len(pending))
	require.NotEmpty(t, pending,
		"if a rebuild stops handing back answered calls, this is worth revisiting")
	for _, call := range pending {
		answer := call.value.LookupPath(cue.MakePath(cue.Str(returnsKey)))
		require.True(t, answer.Exists() && answer.IsConcrete(),
			"%s is handed back to be made and already holds its answer, "+
				"which is the whole reason the value cannot be rewritten", call.key)
	}
}
