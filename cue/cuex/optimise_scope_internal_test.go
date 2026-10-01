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

	"cuelang.org/go/cue/parser"
	"github.com/stretchr/testify/require"
)

// What an iteration reads is the whole risk of the prepass, so every way
// CUE can bind or mention a name gets a case. A name missed here becomes a
// document that is missing a field, which errors and is survivable; a name
// wrongly thought free becomes a larger document, which is only slower.
// The case that must never happen is a bound name reported as free and
// then filled from the wrong place, which is why the binding forms are
// where the cases are concentrated.
func TestFreeIdents(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want []string
	}{
		{"a bare reference", `x: a`, []string{"a"}},
		{"only the root of a selector", `x: a.b.c`, []string{"a"}},
		{"the root of an index expression", `x: a["k"]`, []string{"a"}},
		{"what an index reads too", `x: a[k]`, []string{"a", "k"}},
		{"labels are not references", `x: {a: 1, b: 2}`, nil},
		{"an interpolated label is", `x: {"\(k)": 1}`, []string{"k"}},
		{"interpolation in a value", `x: "\(a)-\(b)"`, []string{"a", "b"}},
		{"a call's arguments", `x: f(a, b)`, []string{"f", "a", "b"}},
		{"a package is a reference", `x: base64.#Encode & {$params: p}`,
			[]string{"base64", "p"}},

		// binding forms, where getting it wrong is silent
		{"a for binds its value", `x: {for i in src {"\(i)": i}}`, []string{"src"}},
		{"a for binds key and value", `x: {for k, v in src {"\(k)": v}}`, []string{"src"}},
		{"a for's source is read outside the binding",
			`x: {for i in i {"\(i)": i}}`, []string{"i"}},
		{"a nested for binds both", `x: {for a in s1 {for b in s2 {"\(a)\(b)": a}}}`,
			[]string{"s1", "s2"}},
		{"an if reads in scope", `x: {for i in src if i > n {"\(i)": i}}`,
			[]string{"src", "n"}},
		{"a let binds for the body", `x: {let y = a, z: y}`, []string{"a"}},
		// The alias form, which freeIdents has a branch of its own for.
		// CUE will not let an alias shadow a field in scope, which is why
		// not binding the name for what follows cannot put the wrong
		// declaration in a document: a template that would show it does
		// not parse.
		{"an alias reads its expression", `x: {y=inner: a, z: y}`, []string{"a", "y"}},
		// A pattern alias does not bind its name here, so the name leaks
		// out as free. That costs a decline where nothing declares it, and
		// where something does it carries that declaration: a field beside
		// the pattern rather than the pattern's own binding, so it cannot
		// change an answer.
		{"a pattern alias leaks its name", `x: {[p=string]: p}`, []string{"string", "p"}},
		{"a let is visible to fields written before it",
			`x: {z: y, let y = a}`, []string{"a"}},
		{"a let in a comprehension binds after itself",
			`x: {for i in src {let d = i * 2, "\(i)": d}}`, []string{"src"}},
		{"the blank identifier is not a reference",
			`x: {for _, v in src {"k": v}}`, []string{"src"}},

		// the shape the prepass is actually for
		{"a loop of calls", `
_calls: {
	for i in _idx {
		"\(i)": bench.#Put & {$params: {scope: _scope["\(i)"].$returns.scope, index: i}}
	}
}`, []string{"_idx", "bench", "_scope"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := parser.ParseFile("-", tc.src, parser.ParseComments)
			require.NoError(t, err)
			got := freeIdents(f)
			require.Equal(t, tc.want, got)
		})
	}
}
