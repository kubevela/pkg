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
	"github.com/stretchr/testify/require"
)

// The two reasons a whole file is left alone, and the answers either way.

// A template using the names the iteration documents are built with is left
// alone, rather than having its own field answered or a good loop declined.
func TestAFileUsingOurOwnNamesIsLeftAlone(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	imports := c.PackageManager.GetImports()

	loop := func(extra string) string {
		return fmt.Sprintf(`import "vela/base64"
import "list"

_idx: list.Range(0, 20, 1)
%s
_s0: {for i in _idx {"\(i)": base64.#Encode & {$params: "seed-\(i)"}}}
_s1: {for i in _idx {"\(i)": base64.#Encode & {$params: _s0["\(i)"].$returns}}}
out: {for i in _idx {"\(i)": _s1["\(i)"].$returns}}
`, extra)
	}

	// chained and twenty wide, so it is taken when nothing is in the way
	plain := loop("")
	_, took := c.prepass(ctx, plain, imports, DefaultOptimisePolicy)
	require.True(t, took, "the same file without the name is answered")

	for _, name := range []string{iterationField, keysIndexVar} {
		t.Run(name, func(t *testing.T) {
			src := loop(name + ": \"mine\"")
			out, took := c.prepass(ctx, src, imports, DefaultOptimisePolicy)
			require.False(t, took, "a file using %s has to be left alone", name)
			require.Equal(t, "file:declaresOurNames", out.why,
				"and say that is why")

			// and it still renders, through the resolver
			v, err := c.CompileStringWithOptions(ctx, src, WithOptimise(DefaultOptimisePolicy))
			require.NoError(t, err)
			got, err := v.LookupPath(cue.MakePath(cue.Hid(name, "_"))).String()
			require.NoError(t, err)
			require.Equal(t, "mine", got, "the template's own field is untouched")
		})
	}
}

// An iteration that would have to carry a whole answered loop is declined
// rather than built.
//
// Narrowing exists so a stage reading the stage before it carries the one
// element it reads. Where it cannot, because the loop reads that name some
// way other than by index, the document would hold every answer the earlier
// stage gave: n documents of n answers, which is the cost this exists to
// avoid.
func TestAnIterationCarryingAWholeLoopIsDeclined(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	imports := c.PackageManager.GetImports()

	// _s1 reads _s0 whole, through len, so there is no element to narrow to
	src := `import "vela/base64"
import "list"

_idx: list.Range(0, 200, 1)
_s0: {for i in _idx {"\(i)": base64.#Encode & {$params: "seed-\(i)"}}}
_s1: {for i in _idx {"\(i)": base64.#Encode & {$params: "\(len(_s0))-\(i)"}}}
out: {for i in _idx {"\(i)": _s1["\(i)"].$returns}}
`
	out, took := c.prepass(ctx, src, imports, OptimisePolicy{
		Enabled: true, Threshold: 1, LoneThreshold: 1, Batch: 100,
	})
	// _s0 has nothing to carry and is answered; _s1 would carry all of it
	require.True(t, took, "the first stage is still worth answering")
	require.Equal(t, "doc:carriesAWholeLoop", out.declined["_s1"],
		"the stage that would carry the whole of _s0 has to be left alone")

	// and the render is still right
	v, err := c.CompileStringWithOptions(ctx, src, WithOptimise(DefaultOptimisePolicy))
	require.NoError(t, err)
	require.NoError(t, v.LookupPath(cue.ParsePath("out")).Err())
}
