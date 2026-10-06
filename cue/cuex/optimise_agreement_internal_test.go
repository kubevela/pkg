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
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// Four shapes where answering a loop early has to come out the same as
// letting the resolver do it, and did not.

var agreeRuns atomic.Int64

// agreePackage has an echo that returns a string and a nested that builds
// its own value carrying a definition, which is how a result carries
// another call.
func agreePackage(order *[]string) cuexruntime.Package {
	p, err := cuexruntime.NewInternalPackage("agree", `#Echo: {
	#do: "echo"
	#provider: "agree"
	$params: string
	$returns?: string
}
#Nested: {
	#do: "nested"
	#provider: "agree"
	$params: string
	$returns?: {...}
}
`, map[string]cuexruntime.ProviderFn{
		"echo": cuexruntime.NativeProviderFn(func(_ context.Context, v cue.Value) (cue.Value, error) {
			agreeRuns.Add(1)
			s, _ := v.LookupPath(cue.ParsePath("$params")).String()
			if order != nil {
				*order = append(*order, s)
			}
			return v.FillPath(cue.ParsePath("$returns"), "r:"+s), nil
		}),
		"nested": cuexruntime.NativeProviderFn(func(_ context.Context, v cue.Value) (cue.Value, error) {
			agreeRuns.Add(1)
			return v.FillPath(cue.ParsePath("$returns"),
				v.Context().CompileString(`{plain: "visible", #def: "kept"}`)), nil
		}),
	})
	if err != nil {
		panic(err)
	}
	return p
}

func agreeKeys(n int) string {
	var ks []string
	for i := 0; i < n; i++ {
		ks = append(ks, strconv.Quote("k"+strconv.Itoa(i)))
	}
	return "[" + strings.Join(ks, ", ") + "]"
}

var (
	resolverOnly = OptimisePolicy{}
	prepassOn    = OptimisePolicy{Enabled: true, Threshold: 1, Batch: 100}
)

// Data the caller supplies for a field the template declares is unified
// into the built value, after the prepass has been and gone. So a prepass
// that answers a call reading that field answers it with whatever the
// template said on its own, and the caller's value never reaches the
// provider: same number of calls, different parameters, nothing said.
func TestTheCallersDataReachesAnAnsweredCall(t *testing.T) {
	c := NewCompilerWithInternalPackages(agreePackage(nil))
	src := `import "vela/agree"

parameter: {word: *"DEFAULT" | string}
keys: ` + agreeKeys(12) + `

calls: {
	for k in keys {
		(k): agree.#Echo & {$params: parameter.word + "-" + k}
	}
}
out: {
	for k in keys {
		(k): calls[k].$returns
	}
}
`
	for _, tc := range []struct {
		name   string
		policy OptimisePolicy
	}{{"resolver", resolverOnly}, {"prepass", prepassOn}} {
		t.Run(tc.name, func(t *testing.T) {
			v, err := c.CompileStringWithOptions(context.Background(), src,
				WithOptimise(tc.policy),
				WithData("parameter", map[string]any{"word": "ACTUAL"}))
			require.NoError(t, err)
			got, err := v.LookupPath(cue.ParsePath(`out["k0"]`)).String()
			require.NoError(t, err)
			require.Equal(t, "r:ACTUAL-k0", got,
				"the call must see what the caller passed, not the template's default")
		})
	}
}

// The same thing again, by the other route into the value.
//
// An intra-resolve mutation runs on the built value, after the prepass and
// before the resolve, and it can change anything: the field a call reads
// its parameters from included. So a prepass that answered first answered
// from what the template said on its own.
//
// Found by review after the fill above was fixed, which is the lesson
// worth keeping: the question was never "does WithData break this" but
// "what else is put into the value after this runs", and there were two
// answers.
func TestAMutationBeforeTheCallsReachesThem(t *testing.T) {
	c := NewCompilerWithInternalPackages(agreePackage(nil))
	src := `import "vela/agree"

word: *"BEFORE" | string
keys: ` + agreeKeys(12) + `

calls: {
	for k in keys {
		(k): agree.#Echo & {$params: word + "-" + k}
	}
}
out: {
	for k in keys {
		(k): calls[k].$returns
	}
}
`
	mutate := WithIntraResolveMutation("rewrite word",
		func(_ context.Context, v cue.Value) (cue.Value, error) {
			return v.FillPath(cue.ParsePath("word"), "AFTER"), nil
		})

	for _, tc := range []struct {
		name   string
		policy OptimisePolicy
	}{{"resolver", resolverOnly}, {"prepass", prepassOn}} {
		t.Run(tc.name, func(t *testing.T) {
			agreeRuns.Store(0)
			v, err := c.CompileStringWithOptions(context.Background(), src,
				WithOptimise(tc.policy), mutate)
			require.NoError(t, err)
			got, err := v.LookupPath(cue.ParsePath(`out["k0"]`)).String()
			require.NoError(t, err)
			require.Equal(t, "r:AFTER-k0", got,
				"the call has to see what the mutation left, not what the template said")
			require.EqualValues(t, 12, agreeRuns.Load())
		})
	}
}

// A loop is taken or it is not, and the decision can only be made for the
// whole of it. One iteration that cannot be answered used to be found
// after the ones before it had already called their provider, and the
// answers were then thrown away and the loop handed back, so the resolver
// called those providers a second time. Harmless for a provider that
// formats a string and not for one that writes.
func TestADeclinedLoopCallsNoProviderTwice(t *testing.T) {
	c := NewCompilerWithInternalPackages(agreePackage(nil))
	// the last iteration is not a call, so the loop cannot be taken
	src := `import "vela/agree"

keys: ` + agreeKeys(12) + `

calls: {
	for k in keys {
		(k): {
			if k != "k11" {
				agree.#Echo & {$params: k}
			}
			if k == "k11" {
				$returns: "plain"
			}
		}
	}
}
out: {
	for k in keys {
		(k): calls[k].$returns
	}
}
`
	var runs [2]int64
	var outs [2]string
	for i, policy := range []OptimisePolicy{resolverOnly, prepassOn} {
		agreeRuns.Store(0)
		v, err := c.CompileStringWithOptions(context.Background(), src, WithOptimise(policy))
		require.NoError(t, err)
		bs, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
		require.NoError(t, err)
		runs[i], outs[i] = agreeRuns.Load(), string(bs)
	}
	require.Equal(t, outs[0], outs[1], "the answers must agree")
	require.Equal(t, runs[0], runs[1],
		"a loop it cannot take must cost no provider calls, got %d against the resolver's %d",
		runs[1], runs[0])
}

// A loop that is answered costs each of its providers one call, which is
// the assertion nothing here had.
//
// What is written back has to read as an answered call and not one still
// asking to be made. An answer kept whole, which is how a result carries
// a call of its own, carries the node's own #do with it unless that is
// taken out, and then the resolver finds it and calls everything again.
// Both arms rendered the same answers while that was true.
func TestAnAnsweredLoopCallsEachProviderOnce(t *testing.T) {
	c := NewCompilerWithInternalPackages(agreePackage(nil))
	src := `import "vela/agree"

keys: ` + agreeKeys(12) + `

calls: {
	for k in keys {
		(k): agree.#Echo & {$params: k}
	}
}
out: {
	for k in keys {
		(k): calls[k].$returns
	}
}
`
	var runs [2]int64
	var outs [2]string
	for i, policy := range []OptimisePolicy{resolverOnly, prepassOn} {
		agreeRuns.Store(0)
		v, err := c.CompileStringWithOptions(context.Background(), src, WithOptimise(policy))
		require.NoError(t, err)
		bs, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
		require.NoError(t, err)
		runs[i], outs[i] = agreeRuns.Load(), string(bs)
	}
	require.Equal(t, outs[0], outs[1], "the answers must agree")
	require.EqualValues(t, 12, runs[0], "the resolver should make twelve calls")
	require.EqualValues(t, 12, runs[1],
		"answering the loop should make twelve calls and not twelve twice")
}

// A step attribute says what order calls run in. The resolver sorts by it;
// a prepass walking the file answers loops in the order they are written,
// which is not the same thing, so a file that asks for an order does not
// get one answered early.
func TestStepOrderIsKept(t *testing.T) {
	var order []string
	c := NewCompilerWithInternalPackages(agreePackage(&order))
	// written second first, and asked to run second
	src := `import "vela/agree"

keys: ` + agreeKeys(12) + `

second: {
	for k in keys {
		(k): agree.#Echo & {$params: "second-" + k}
	}
} @step(2)

first: {
	for k in keys {
		(k): agree.#Echo & {$params: "first-" + k}
	}
} @step(1)

out: {
	for k in keys {
		(k): first[k].$returns + second[k].$returns
	}
}
`
	var firsts [2]string
	for i, policy := range []OptimisePolicy{resolverOnly, prepassOn} {
		order = nil
		_, err := c.CompileStringWithOptions(context.Background(), src, WithOptimise(policy))
		require.NoError(t, err)
		require.NotEmpty(t, order)
		firsts[i] = order[0]
	}
	require.Equal(t, firsts[0], firsts[1],
		"the step attribute has to decide which loop runs first either way")
}

// A native provider builds its own value, and a definition is how such a
// value carries another call. Writing an answer back as JSON drops
// definitions without a word, so a call returned by a call would never
// run.
func TestANativeResultKeepsItsDefinitions(t *testing.T) {
	c := NewCompilerWithInternalPackages(agreePackage(nil))
	src := `import "vela/agree"

keys: ` + agreeKeys(12) + `

calls: {
	for k in keys {
		(k): agree.#Nested & {$params: k}
	}
}
out: {
	for k in keys {
		(k): calls[k].$returns
	}
}
`
	for _, tc := range []struct {
		name   string
		policy OptimisePolicy
	}{{"resolver", resolverOnly}, {"prepass", prepassOn}} {
		t.Run(tc.name, func(t *testing.T) {
			v, err := c.CompileStringWithOptions(context.Background(), src, WithOptimise(tc.policy))
			require.NoError(t, err)
			one := v.LookupPath(cue.ParsePath(`out["k0"]`))
			require.True(t, one.LookupPath(cue.MakePath(cue.Str("plain"))).Exists(),
				"the plain field should be there")
			require.True(t, one.LookupPath(cue.MakePath(cue.Def("#def"))).Exists(),
				"a definition the provider returned has to survive being written back")
		})
	}
}
