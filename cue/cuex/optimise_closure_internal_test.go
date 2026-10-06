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

func closureFor(t *testing.T, src string, seeds []string, skip ...string) ([]string, []string, bool) {
	t.Helper()
	f, err := parser.ParseFile("-", src, parser.ParseComments)
	require.NoError(t, err)
	scope, ok := scopeOfFile(f)
	if !ok {
		return nil, nil, false
	}
	skipping := map[string]bool{}
	for _, s := range skip {
		skipping[s] = true
	}
	names, imports, ok := scope.closureOf(seeds, skipping)
	var paths []string
	for _, spec := range imports {
		paths = append(paths, spec.Path.Value)
	}
	return names, paths, ok
}

func TestClosureOf(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		seeds   []string
		skip    []string
		want    []string
		imports []string
		ok      bool
	}{
		{
			name:  "a name and what it reads",
			src:   "a: b\nb: c\nc: 1\nunrelated: 2\n",
			seeds: []string{"a"},
			want:  []string{"a", "b", "c"},
			ok:    true,
		},
		{
			name:  "in the file's order, not the order found",
			src:   "c: 1\nb: c\na: b\n",
			seeds: []string{"a"},
			want:  []string{"c", "b", "a"},
			ok:    true,
		},
		{
			name:  "a cycle does not spin",
			src:   "a: b\nb: a\n",
			seeds: []string{"a"},
			want:  []string{"a", "b"},
			ok:    true,
		},
		{
			name:    "an import is carried as an import",
			src:     "import \"strings\"\na: strings.ToUpper(b)\nb: \"x\"\n",
			seeds:   []string{"a"},
			want:    []string{"a", "b"},
			imports: []string{`"strings"`},
			ok:      true,
		},
		{
			name:    "an aliased import is known by its alias",
			src:     "import s \"strings\"\na: s.ToUpper(\"x\")\n",
			seeds:   []string{"a"},
			want:    []string{"a"},
			imports: []string{`"strings"`},
			ok:      true,
		},
		{
			name:  "what CUE declares itself needs nothing carried",
			src:   "a: len(b)\nb: \"xy\"\n",
			seeds: []string{"a"},
			want:  []string{"a", "b"},
			ok:    true,
		},
		{
			name:  "a loop variable is skipped, not looked up",
			src:   "a: i\n",
			seeds: []string{"a"},
			skip:  []string{"i"},
			want:  []string{"a"},
			ok:    true,
		},
		{
			name:  "a name with no declaration declines",
			src:   "a: missing\n",
			seeds: []string{"a"},
			ok:    false,
		},

		// the hazard, which is the reason this exists
		{
			name:  "every declaration of a name is followed",
			src:   "cfg: {mode: m}\ncfg: {retries: r}\nm: \"fast\"\nr: 3\n",
			seeds: []string{"cfg"},
			want:  []string{"cfg", "m", "r"},
			ok:    true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			names, imports, ok := closureFor(t, tc.src, tc.seeds, tc.skip...)
			require.Equal(t, tc.ok, ok)
			if !tc.ok {
				return
			}
			require.Equal(t, tc.want, names)
			require.Equal(t, tc.imports, imports)
		})
	}
}

// A file whose top level holds something that cannot be named is declined
// outright: a closure over it cannot be shown to be complete.
func TestClosureDeclinesWhatItCannotName(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"an embedding at the top level", "someOther\na: 1\n"},
		{"a let at the top level", "let x = 1\na: x\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, ok := closureFor(t, tc.src, []string{"a"})
			require.False(t, ok, "a file with an unnameable top level is not safe to carry")
		})
	}
}
