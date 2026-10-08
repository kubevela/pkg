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
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// Two things a template can see besides the answers: what order the
// calls ran in, and what happens when one of them fails.
//
// Neither shows in a rendered value, so nothing measured so far would
// notice the prepass changing them. A provider that writes to a cluster
// cares about both.

type orderIn struct {
	Params string `json:"$params"`
}
type orderOut struct {
	Returns string `json:"$returns"`
}

// recorder is a provider that remembers the order it was called in and
// can be told to fail on one particular call.
type recorder struct {
	mu     sync.Mutex
	seen   []string
	failOn string
}

func (r *recorder) pkg() cuexruntime.Package {
	pkg, err := cuexruntime.NewInternalPackage("rec", `
package rec

#Do: {
	#do:       "do"
	#provider: "rec"
	$params:   string
	$returns?: string
}

#Step: {
	check: #Do & {$params: "failed"}
	if check.$returns == "failed" {
		fail: #Do & {$params: "fail"}
	}
	wait: #Do & {$params: "wait-now"}
}
`, map[string]cuexruntime.ProviderFn{
		"do": cuexruntime.GenericProviderFn[orderIn, orderOut](
			func(_ context.Context, in *orderIn) (*orderOut, error) {
				r.mu.Lock()
				r.seen = append(r.seen, in.Params)
				fail := in.Params == r.failOn
				r.mu.Unlock()
				if fail {
					return nil, fmt.Errorf("refused to answer %s", in.Params)
				}
				return &orderOut{Returns: in.Params}, nil
			}),
	})
	if err != nil {
		panic(err)
	}
	return pkg
}

func (r *recorder) order() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seen...)
}

func loopOverNames(n int) string {
	var b strings.Builder
	b.WriteString("import \"vela/rec\"\nimport \"list\"\n")
	fmt.Fprintf(&b, "_idx: list.Range(0, %d, 1)\n", n)
	b.WriteString("_c: {for i in _idx {\"\\(i)\": rec.#Do & {$params: \"n\\(i)\"}}}\n")
	b.WriteString("out: {for i in _idx {\"\\(i)\": _c[\"\\(i)\"].$returns}}\n")
	return b.String()
}

// The order a loop's calls run in is the order the template wrote them,
// and it stays that way whoever answers them. A provider that creates
// numbered resources, or one whose calls each depend on the last having
// happened, is reading that order even though the value does not show
// it.
func TestTheOrderCallsRunInIsKept(t *testing.T) {
	for _, n := range []int{5, 12, 250} {
		var off, on []string
		for _, tc := range []struct {
			policy OptimisePolicy
			into   *[]string
		}{
			{OptimisePolicy{}, &off},
			{DefaultOptimisePolicy, &on},
		} {
			rec := &recorder{}
			c := NewCompilerWithInternalPackages(rec.pkg())
			_, err := c.CompileStringWithOptions(context.Background(),
				loopOverNames(n), WithOptimise(tc.policy))
			require.NoError(t, err)
			*tc.into = rec.order()
		}
		require.Len(t, off, n)
		require.Equal(t, off, on,
			"width %d: the prepass must run the calls in the order the resolver does", n)
		t.Logf("ORDER width=%-4d first=%v last=%v", n, off[0], off[len(off)-1])
	}
}

// A call that fails should fail the render, say which call, and not
// leave the rest of the loop half done and unreported.
func TestACallThatFailsInALoop(t *testing.T) {
	const n = 250
	for _, tc := range []struct {
		name   string
		policy OptimisePolicy
	}{
		{"resolver", OptimisePolicy{}},
		{"prepass", DefaultOptimisePolicy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{failOn: "n7"}
			c := NewCompilerWithInternalPackages(rec.pkg())
			v, err := c.CompileStringWithOptions(context.Background(),
				loopOverNames(n), WithOptimise(tc.policy))

			// the failure has to reach the caller, one way or the other
			reported := err
			if reported == nil {
				_, reported = v.MarshalJSON()
			}
			require.Error(t, reported, "a call that failed must not render as a success")
			require.Contains(t, reported.Error(), "refused to answer n7",
				"and the error should say which call it was")
			require.Contains(t, reported.Error(), `_c."7"`,
				"named where the template put the call, not where this answered it")
			require.NotContains(t, reported.Error(), iterationField,
				"a template never wrote that name and should not be shown it")

			// Eight, which is up to and including the one that failed.
			// Handing the loop back to the resolver instead would make
			// all eight again, and against a provider that writes that
			// is eight more writes on a render that is going to fail.
			require.Equal(t, 8, len(rec.order()),
				"a failing loop should stop where it failed and not be run twice")
			t.Logf("FAIL-%s ran %d call(s), reported: %s",
				tc.name, len(rec.order()), strings.SplitN(reported.Error(), "\n", 2)[0])
		})
	}
}

// A call that fails fails the render, and the error says which call and
// where it was in the template.
//
// Not "the value has no answer in it": there is no value. Asserting
// against the empty one a failed compile returns passes whatever happens,
// which is what this test used to do.
func TestAFailedCallFailsTheRender(t *testing.T) {
	rec := &recorder{failOn: "n3"}
	c := NewCompilerWithInternalPackages(rec.pkg())
	v, err := c.CompileStringWithOptions(context.Background(),
		loopOverNames(250), WithOptimise(DefaultOptimisePolicy))
	require.Error(t, err, "a call that fails has to fail the render")

	var called FunctionCallError
	require.ErrorAs(t, err, &called, "the error should say a call failed")
	require.Contains(t, called.Path, "_c",
		"it should name where the template put the call, not the document built to answer it")
	require.NotContains(t, called.Path, iterationField,
		"a template never wrote that name and should not be shown it")

	require.False(t, v.Exists(),
		"a failed render hands back no value to read an answer out of")
}
