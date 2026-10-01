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
	"sort"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"
)

// The names CUE provides need nothing carried into a document, and a loop
// that mentions one should still be answered. A name missing from the list
// costs that loop; a name wrongly on it would shadow a declaration, which
// is the next test.
func TestALoopMentioningAPredeclaredNameIsStillAnswered(t *testing.T) {
	c := NewCompilerWithInternalPackages(agreePackage(nil))

	names := make([]string, 0, len(predeclared))
	for name := range predeclared {
		if name == "_" {
			// the top type, which is not a name a template constrains a
			// parameter with in any useful way
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			// mentioned in a way that leaves the parameters concrete,
			// whatever kind of name it is
			src := `import "vela/agree"

keys: ` + agreeKeys(12) + `

calls: {
	for k in keys {
		(k): agree.#Echo & {$params: "\(len([` + name + `]))-" + k}
	}
}
out: {
	for k in keys {
		(k): calls[k].$returns
	}
}
`
			var outs [2]string
			for i, policy := range []OptimisePolicy{resolverOnly, prepassOn} {
				v, err := c.CompileStringWithOptions(context.Background(), src, WithOptimise(policy))
				require.NoError(t, err)
				bs, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
				require.NoError(t, err)
				outs[i] = string(bs)
			}
			require.Equal(t, outs[0], outs[1], "mentioning %s must not change the answers", name)

			OptimiseStats.Calls.Store(0)
			_, err := c.CompileStringWithOptions(context.Background(), src, WithOptimise(prepassOn))
			require.NoError(t, err)
			require.EqualValues(t, 12, OptimiseStats.Calls.Load(),
				"%s is one of CUE's own, so the loop should still be answered", name)
		})
	}
}

// And a template may declare a field with one of those names. The
// declaration wins, which is what CUE does, so it has to be carried.
func TestADeclarationBeatsAPredeclaredName(t *testing.T) {
	c := NewCompilerWithInternalPackages(agreePackage(nil))

	for _, name := range []string{"uint", "rune", "int64", "float32", "string"} {
		t.Run(name, func(t *testing.T) {
			src := `import "vela/agree"

` + name + `: "MINE"
keys: ` + agreeKeys(12) + `

calls: {
	for k in keys {
		(k): agree.#Echo & {$params: ` + name + ` + "-" + k}
	}
}
out: {
	for k in keys {
		(k): calls[k].$returns
	}
}
`
			var outs [2]string
			for i, policy := range []OptimisePolicy{resolverOnly, prepassOn} {
				v, err := c.CompileStringWithOptions(context.Background(), src, WithOptimise(policy))
				require.NoError(t, err)
				got, err := v.LookupPath(cue.ParsePath(`out["k0"]`)).String()
				require.NoError(t, err)
				outs[i] = got
			}
			require.Equal(t, "r:MINE-k0", outs[0], "the resolver reads the declaration")
			require.Equal(t, outs[0], outs[1], "and so must a loop answered early")

			OptimiseStats.Calls.Store(0)
			_, err := c.CompileStringWithOptions(context.Background(), src, WithOptimise(prepassOn))
			require.NoError(t, err)
			require.EqualValues(t, 12, OptimiseStats.Calls.Load(),
				"the declaration is carried, so the loop should be answered rather than declined")
		})
	}
}
