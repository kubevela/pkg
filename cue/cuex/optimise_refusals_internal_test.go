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
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"github.com/stretchr/testify/require"
)

// The refusals in narrowing, asked of the functions directly.
//
// Reaching these through a render means writing a template contrived past
// the point of being one: a declaration that is not a struct, an index
// whose interpolation is not the loop variable. What they decide is
// whether a document gets narrowed or carried whole, so they are worth
// pinning somewhere, and here is cheaper to read than a template built to
// trip them.

// narrowTo cuts a declaration down to the keys an iteration reads, and
// refuses anything whose keys it cannot name.
func TestNarrowToRefusesWhatItCannotCount(t *testing.T) {
	field := func(src string) *ast.Field {
		f, err := parser.ParseFile("-", src, parser.ParseComments)
		require.NoError(t, err)
		require.Len(t, f.Decls, 1)
		return f.Decls[0].(*ast.Field)
	}

	t.Run("a struct keeps the keys that are read", func(t *testing.T) {
		out, ok := narrowTo([]*ast.Field{field(`x: {a: 1, b: 2}`)},
			map[string]bool{"a": true})
		require.True(t, ok)
		require.Len(t, out, 1)
		kept := out[0].Value.(*ast.StructLit)
		require.Len(t, kept.Elts, 1, "only the key that is read is carried")
		// which one, since a count of one passes whether it kept a or b
		label, _, err := ast.LabelName(kept.Elts[0].(*ast.Field).Label)
		require.NoError(t, err)
		require.Equal(t, "a", label, "and it is the key that was read")
	})

	t.Run("a declaration that is not a struct", func(t *testing.T) {
		_, ok := narrowTo([]*ast.Field{field(`x: "a string"`)},
			map[string]bool{"a": true})
		require.False(t, ok, "there are no keys to cut it down to")
	})

	t.Run("a struct still holding a comprehension", func(t *testing.T) {
		_, ok := narrowTo([]*ast.Field{field(`x: {for i in [1] {"\(i)": i}}`)},
			map[string]bool{"a": true})
		require.False(t, ok, "what keys it makes is not known until it runs")
	})

	t.Run("a key that is not there", func(t *testing.T) {
		_, ok := narrowTo([]*ast.Field{field(`x: {a: 1}`)},
			map[string]bool{"missing": true})
		require.False(t, ok,
			"a template reading something that does not exist is the resolver's error to report")
	})
}

// directKey recognises the one shape nearly every template keys a loop by,
// and says nothing about the rest.
func TestDirectKeyRecognisesOnlyTheIterationsOwnValue(t *testing.T) {
	// "\(i)", the interpolation a template writes to key a loop by what it
	// is looping over
	interp := func(elts ...ast.Expr) ast.Expr {
		return &ast.Interpolation{Elts: elts}
	}
	lit := func(kind token.Token, v string) ast.Expr {
		return &ast.BasicLit{Kind: kind, Value: v}
	}
	own := func(name string) ast.Expr {
		return interp(lit(token.STRING, `"\(`), ast.NewIdent(name), lit(token.STRING, `)"`))
	}

	t.Run("the iteration's own value, keyed by a string", func(t *testing.T) {
		got, ok := directKey(own("i"), "i", lit(token.STRING, `"k0"`))
		require.True(t, ok)
		require.Equal(t, "k0", got, "the quotes come off")
	})

	t.Run("and by a number", func(t *testing.T) {
		got, ok := directKey(own("i"), "i", lit(token.INT, "7"))
		require.True(t, ok)
		require.Equal(t, "7", got, "a number reads as written")
	})

	for _, tc := range []struct {
		name  string
		index ast.Expr
		at    ast.Expr
	}{
		{"not an interpolation at all", ast.NewString("k0"), lit(token.STRING, `"k0"`)},
		{
			"an interpolation of something else",
			interp(lit(token.STRING, `"k`), ast.NewIdent("i"), lit(token.STRING, `)"`)),
			lit(token.STRING, `"k0"`),
		},
		{
			"an interpolation that does not close the way one does",
			interp(lit(token.STRING, `"\(`), ast.NewIdent("i"), lit(token.STRING, `-x"`)),
			lit(token.STRING, `"k0"`),
		},
		{
			"an interpolation of a name that is not the loop variable",
			own("other"),
			lit(token.STRING, `"k0"`),
		},
		{
			"an interpolation of something that is not a name",
			interp(lit(token.STRING, `"\(`), ast.NewString("i"), lit(token.STRING, `)"`)),
			lit(token.STRING, `"k0"`),
		},
		{"an iteration value that is not a literal", own("i"), ast.NewIdent("notALiteral")},
		{"an iteration value of a kind with no text", own("i"), lit(token.NULL, "null")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := directKey(tc.index, "i", tc.at)
			require.False(t, ok, "worked out the general way instead")
		})
	}
}
