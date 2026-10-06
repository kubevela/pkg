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
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"
)

// Which loops are part of a chain, read from the syntax before a call is
// made. By loop, because a file can hold a chain and an independent loop
// beside it and they want different thresholds.
func TestChainedLoopsReadTheShape(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	imports := c.PackageManager.GetImports()

	for _, tc := range []struct {
		name    string
		src     string
		chained bool
	}{
		{"one loop of calls", stagedLoops(4, 1), false},
		{"two stages", stagedLoops(4, 2), true},
		{"four stages", stagedLoops(4, 4), true},
		{
			// two loops of calls reading nothing of each other is still
			// one round for the resolver
			name: "two independent loops",
			src: `import "vela/base64"
import "list"
_idx: list.Range(0, 4, 1)
_a: {for i in _idx {"\(i)": base64.#Encode & {$params: "a-\(i)"}}}
_b: {for i in _idx {"\(i)": base64.#Encode & {$params: "b-\(i)"}}}
out: {for i in _idx {"\(i)": _a["\(i)"].$returns + _b["\(i)"].$returns}}
`,
			chained: false,
		},
		{
			// the loop that reads makes no call of its own, so everything
			// is still answerable in one round
			name: "a plain loop reads a loop of calls",
			src: `import "vela/base64"
import "list"
_idx: list.Range(0, 4, 1)
_a: {for i in _idx {"\(i)": base64.#Encode & {$params: "a-\(i)"}}}
out: {for i in _idx {"\(i)": _a["\(i)"].$returns}}
`,
			chained: false,
		},
		{
			// a chain and an independent loop in one file: the chain is
			// chained and the loner is not, which is the whole reason this
			// is per loop
			name: "a chain with a loner beside it",
			src: `import "vela/base64"
import "list"
_idx: list.Range(0, 4, 1)
_a: {for i in _idx {"\(i)": base64.#Encode & {$params: "a-\(i)"}}}
_b: {for i in _idx {"\(i)": base64.#Encode & {$params: _a["\(i)"].$returns}}}
_lone: {for i in _idx {"\(i)": base64.#Encode & {$params: "l-\(i)"}}}
out: {for i in _idx {"\(i)": _b["\(i)"].$returns + _lone["\(i)"].$returns}}
`,
			chained: true,
		},
		{
			name: "no provider package at all",
			src: `import "list"
_idx: list.Range(0, 4, 1)
out: {for i in _idx {"\(i)": "\(i)"}}
`,
			chained: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := parser.ParseFile("-", tc.src, parser.ParseComments)
			require.NoError(t, err)
			require.Equal(t, tc.chained, len(chainedLoops(f, imports)) > 0)
		})
	}
}

// The shipped policy end to end. One flat loop of twenty measured 0.74x,
// so it has to be left alone; the same twenty in four stages measured
// 1.48x, so it has to be taken.
func TestTheShippedPolicySeparatesTheShapes(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	imports := c.PackageManager.GetImports()

	for _, tc := range []struct {
		name          string
		width, stages int
		want          bool
	}{
		{"a lone loop of twenty is left alone", 20, 1, false},
		{"and of a hundred", 100, 1, false},
		{"one below the lone threshold", defaultLoneThreshold - 1, 1, false},
		{"at the lone threshold", defaultLoneThreshold, 1, true},
		{"two stages of twenty are answered", 20, 2, true},
		{"four stages of ten are answered", 10, 4, true},
		{"four stages of nine are not", 9, 4, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, took := c.prepass(ctx, stagedLoops(tc.width, tc.stages), imports, DefaultOptimisePolicy)
			require.Equal(t, tc.want, took)
		})
	}
}

// How many iterations a loop has, where that can be read without building
// anything. Getting this wrong either refuses a loop that should be taken
// or builds a document for one that should not, so a shape that cannot be
// counted has to say so rather than guess.
func TestLiteralLength(t *testing.T) {
	for _, tc := range []struct {
		name     string
		src      string
		inline   bool
		selector bool
		want     int
		known    bool
	}{
		{name: "a list written out", src: `keys: ["a", "b", "c"]`, want: 3, known: true},
		{name: "an empty list", src: `keys: []`, want: 0, known: true},
		{name: "the list inline", src: `keys: [1, 2]`, inline: true, want: 2, known: true},
		{name: "a list with an ellipsis says nothing", src: `keys: [1, 2, ...]`},
		{name: "a list built by a comprehension says nothing",
			src: `keys: [for i in [1, 2] {i}]`},
		{name: "a name declared twice says nothing", src: "keys: [\"a\"]\nkeys: [\"a\"]"},
		{name: "a name declared as something else says nothing", src: `keys: {a: 1}`},
		{name: "a call says nothing", src: `keys: list.Range(0, 10, 1)`},
		{name: "an undeclared name says nothing", src: `other: 1`},
		// A source that is neither a list nor a name: the commonest real
		// one, a loop over something a parameter supplies. Nothing can be
		// said about its length without evaluating it.
		{name: "a selector says nothing", src: `keys: {items: ["a"]}`, selector: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := parser.ParseFile("-", tc.src, parser.ParseComments)
			require.NoError(t, err)
			scope, ok := scopeOfFile(f)
			require.True(t, ok)

			// the source as a template writes it: the name it declared, or
			// the list itself where the loop holds it directly
			var source ast.Expr = ast.NewIdent("keys")
			switch {
			case tc.inline:
				source = f.Decls[0].(*ast.Field).Value
			case tc.selector:
				source = &ast.SelectorExpr{
					X:   ast.NewIdent("keys"),
					Sel: ast.NewIdent("items"),
				}
			}
			n, known := literalLength(scope, source)
			require.Equal(t, tc.known, known)
			if known {
				require.Equal(t, tc.want, n)
			}
		})
	}
}

// A chain and an independent loop in one file get different thresholds.
func TestALonerBesideAChainKeepsItsOwnThreshold(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	imports := c.PackageManager.GetImports()

	src := `import "vela/base64"
import "list"
_idx: list.Range(0, 20, 1)
_a: {for i in _idx {"\(i)": base64.#Encode & {$params: "a-\(i)"}}}
_b: {for i in _idx {"\(i)": base64.#Encode & {$params: _a["\(i)"].$returns}}}
_lone: {for i in _idx {"\(i)": base64.#Encode & {$params: "l-\(i)"}}}
out: {for i in _idx {"\(i)": _b["\(i)"].$returns + _lone["\(i)"].$returns}}
`
	f, err := parser.ParseFile("-", src, parser.ParseComments)
	require.NoError(t, err)
	chained := chainedLoops(f, imports)
	require.True(t, chained["_a"], "the producer is in the chain")
	require.True(t, chained["_b"], "and so is the reader")
	require.False(t, chained["_lone"],
		"the loop nothing reads and that reads nothing is not")

	// Twenty iterations: over the chained threshold, under the lone one.
	// So the two chained loops are answered and the loner is left, which
	// is forty of the sixty calls.
	OptimiseStats.Calls.Store(0)
	_, took := c.prepass(context.Background(), src, imports, DefaultOptimisePolicy)
	require.True(t, took)
	require.EqualValues(t, 40, OptimiseStats.Calls.Load(),
		"the chain is worth answering at twenty and the loner is not")
}

// Where an error about an answered call says the call was, which is the
// field the template wrote and not the document it was answered in.
func TestLoopPathNamesTheTemplatesOwnField(t *testing.T) {
	for _, tc := range []struct {
		name, loop, key, want string
	}{
		{"a plain field", "calls", "k0", "calls.k0"},
		{"a definition", "#made", "k0", "#made.k0"},
		{"a hidden field", "_calls", "k0", "_calls.k0"},
		{"a hidden definition", "_#made", "k0", "_#made.k0"},
		{"no field at all", "", "k0", "k0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, loopPath(tc.loop, tc.key).String())
		})
	}
}

// The identifier scan is asked about whatever a template holds, including
// nothing.
func TestFreeIdentsOfNothing(t *testing.T) {
	require.Empty(t, freeIdents(nil))
}
