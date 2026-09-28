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

package util_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/util"
)

// TestIterateVisitOrder pins the traversal: depth first, children before the
// node itself, definitions skipped, siblings ordered by their step attribute.
// Callers depend on it - cuex runs provider calls in this order.
func TestIterateVisitOrder(t *testing.T) {
	for name, tt := range map[string]struct {
		src  string
		want []string
	}{
		"nested structs and lists": {
			src:  `xs: ["a", {k: "v"}, ["nested"]]`,
			want: []string{"xs[0]", "xs[1].k", "xs[1]", "xs[2][0]", "xs[2]", "xs", ""},
		},
		"definitions are skipped": {
			src:  `#hidden: {a: "no"}, shown: {b: "yes"}`,
			want: []string{"shown.b", "shown", ""},
		},
		"hidden fields are visited": {
			src:  `_h: "yes", v: "yes"`,
			want: []string{"_h", "v", ""},
		},
		"optional fields are visited": {
			src:  `o?: "maybe", v: "yes"`,
			want: []string{"o", "v", ""},
		},
		"step attribute reorders siblings": {
			src:  `c: "c" @step(3), a: "a" @step(1), b: "b" @step(2)`,
			want: []string{"a", "b", "c", ""},
		},
		"unstepped siblings keep source order and sort last": {
			src:  `x: "x", y: "y" @step(1), z: "z"`,
			want: []string{"y", "x", "z", ""},
		},
		"no step attribute anywhere keeps source order": {
			src:  `c: "c", a: "a", b: "b"`,
			want: []string{"c", "a", "b", ""},
		},
	} {
		t.Run(name, func(t *testing.T) {
			var got []string
			util.Iterate(cuecontext.New().CompileString(tt.src), func(v cue.Value) bool {
				got = append(got, v.Path().String())
				return false
			})
			require.Equal(t, tt.want, got)
		})
	}
}

// TestIterateSkipsValuesInsideDefinitions covers the entry check: a value that
// already sits inside a definition is not walked at all.
func TestIterateSkipsValuesInsideDefinitions(t *testing.T) {
	v := cuecontext.New().CompileString(`#d: {a: "x", b: "y"}`)
	var visited int
	stop := util.Iterate(v.LookupPath(cue.ParsePath("#d")), func(cue.Value) bool {
		visited++
		return false
	})
	require.False(t, stop)
	require.Zero(t, visited)
}

// TestFieldValues covers the accessor beside Iterate: it hands back the same
// fields in the same order, which for a struct is the order the "step"
// attribute asks for and for a list is the order the items are in.
func TestFieldValues(t *testing.T) {
	for name, tt := range map[string]struct {
		src  string
		want []string
	}{
		"a list keeps its order": {`["a", "b", "c"]`, []string{"a", "b", "c"}},
		"a struct in step order": {
			`{
	third: "c" @step(3)
	first: "a" @step(1)
	second: "b" @step(2)
}`, []string{"a", "b", "c"}},
		"a struct with no steps keeps declaration order": {
			`{one: "a", two: "b"}`, []string{"a", "b"}},
		"an empty struct": {`{}`, nil},
	} {
		t.Run(name, func(t *testing.T) {
			got := util.FieldValues(cuecontext.New().CompileString(tt.src))
			out := make([]string, 0, len(got))
			for _, v := range got {
				s, err := v.String()
				require.NoError(t, err)
				out = append(out, s)
			}
			require.Equal(t, tt.want, nilIfEmpty(out))
		})
	}
}

func nilIfEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	return in
}
