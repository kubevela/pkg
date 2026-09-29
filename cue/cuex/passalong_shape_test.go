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

package cuex_test

import (
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"github.com/stretchr/testify/require"
)

// TestAWaitingCallWrittenTheWayTemplatesWriteThem records why passing a result
// straight to the next call's parameters was not built, after it was.
//
// The idea holds: a link waits only because its parameters do, and rebuilding
// them from the answer costs 45us against 274us for unifying the value. What
// does not hold is reading the syntax to do it.
//
// Written inline, the syntax is what the idea needs: one let per call it waits
// on, tagged with that call's path, and the reference reads straight through
// it. Written the way a template writes one, through a definition from a
// provider package, it is not:
//
//   - there is a let for the definition as well, tagged //cue:path: #Do, and
//     nothing has an answer for that
//   - the value inside each let is wrapped in a #x, so the reference reads
//     LINK0.#x.$returns rather than LINK0.$returns
//
// Both are workable. Skip the lets that name no call, and wrap the answer to
// match. Neither is written down anywhere: the #x and the //cue:path comment
// are what internal/core/export happens to emit, so a resolver leaning on them
// would break on a CUE upgrade, and break by feeding a call the wrong
// parameters rather than by failing to build.
//
// The fixtures that made this look easy all wrote their calls inline. This one
// is here so the next attempt starts from the shape that actually turns up.
func TestAWaitingCallWrittenTheWayTemplatesWriteThem(t *testing.T) {
	cc := cuecontext.New()
	root := cc.CompileString(`
#Do: {#do: "do", #provider: "ph", $params: string, $returns?: string}
link0: #Do & {$params: "start"}
link1: #Do & {$params: link0.$returns}
`)
	require.NoError(t, root.Err())

	bs, err := format.Node(root.LookupPath(cue.ParsePath("link1")).Syntax())
	require.NoError(t, err)
	got := string(bs)

	require.Contains(t, got, "//cue:path: link0",
		"the call it waits on is hoisted and tagged, which is what made this look easy")
	require.Contains(t, got, "//cue:path: #Do",
		"but so is the definition it was written through, and that has no answer")
	require.Contains(t, got, "LINK0.#x.$returns",
		"and the reference reads through a #x the exporter added, not straight through")

	inline := cc.CompileString(`
a: {#do: "do", #provider: "ph", $params: "start"}
b: {#do: "do", #provider: "ph", $params: a.$returns}
`)
	require.NoError(t, inline.Err())
	bs, err = format.Node(inline.LookupPath(cue.ParsePath("b")).Syntax())
	require.NoError(t, err)
	require.NotContains(t, string(bs), "#x",
		"written inline there is no wrapper, which is why the fixtures passed")
	require.Equal(t, 1, strings.Count(string(bs), "//cue:path: "),
		"and only the one let, which is why they passed")
}
