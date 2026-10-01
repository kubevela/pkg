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
	"runtime"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

// Does a render hold every round it went through, or only the value it
// ended with?
//
// Each round unifies, and a unify builds a new value rather than changing
// the old one. They are all built in the one context the render was given.
// If a context keeps what has been built in it, a render of n rounds holds
// n values and depth costs memory on its own; if it does not, peak is the
// last value and depth is free.
//
// Same number of calls either way, arranged flat or in a chain, so anything
// that moves is the arrangement and not the work.

func flatSrc(calls int) string {
	var b strings.Builder
	b.WriteString("import \"vela/base64\"\n")
	for i := 0; i < calls; i++ {
		fmt.Fprintf(&b, "x%d: base64.#Encode & {$params: \"hello-%d\"}\n", i, i)
	}
	return b.String()
}

// chainedSrc is the same number of calls in groups, where each group waits
// on the one before it, so the resolver needs a round per group.
func chainedSrc(calls, groups int) string {
	per := calls / groups
	var b strings.Builder
	b.WriteString("import \"vela/base64\"\nimport \"strings\"\n")
	for g := 0; g < groups; g++ {
		for i := 0; i < per; i++ {
			if g == 0 {
				fmt.Fprintf(&b, "g%d_%d: base64.#Encode & {$params: \"seed-%d\"}\n", g, i, i)
				continue
			}
			// sliced, or base64 of base64 grows four thirds a group
			fmt.Fprintf(&b,
				"g%d_%d: base64.#Encode & {$params: strings.SliceRunes(g%d_%d.$returns, 0, 8)}\n",
				g, i, g-1, i)
		}
	}
	return b.String()
}

func TestDoesARenderHoldEveryRound(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()

	held := func(src string) uint64 {
		runtime.GC()
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		v, err := c.CompileString(ctx, src)
		require.NoError(t, err)
		runtime.GC()
		runtime.ReadMemStats(&b)
		out := b.HeapAlloc - a.HeapAlloc
		runtime.KeepAlive(v)
		return out
	}

	const calls = 2000
	byGroups := map[int]uint64{}
	for _, groups := range []int{1, 2, 4, 8} {
		var src string
		if groups == 1 {
			src = flatSrc(calls)
		} else {
			src = chainedSrc(calls, groups)
		}
		byGroups[groups] = held(src)
		t.Logf("HOLD %4d calls in %d group(s): %7dKB held, %5dB per call",
			calls, groups, byGroups[groups]/1024, int(byGroups[groups])/calls)
	}

	// The same calls arranged in eight dependent groups rather than one
	// flat one, so the only thing that changed is how many rounds the
	// resolver needed. Measured at two and a half times; asserted well
	// under that, because the number is a heap reading and the claim is
	// only that depth is not free.
	require.Greater(t, byGroups[8], byGroups[1]*3/2,
		"a render holding only its result would not grow with the number of rounds")
}

// And the same question asked of the context directly: a value built, read,
// and dropped, over and over in one context against a context each.
func TestAContextHoldsWhatWasBuiltInIt(t *testing.T) {
	const rounds = 40
	src := flatData(500)

	one := func(shared bool) uint64 {
		runtime.GC()
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		cc := cuecontext.New()
		for i := 0; i < rounds; i++ {
			if !shared {
				cc = cuecontext.New()
			}
			v := cc.CompileString(src)
			if v.Err() != nil {
				t.Fatal(v.Err())
			}
			if _, err := v.LookupPath(cue.ParsePath("x0")).String(); err != nil {
				t.Fatal(err)
			}
		}
		runtime.GC()
		runtime.ReadMemStats(&b)
		out := b.HeapAlloc - a.HeapAlloc
		runtime.KeepAlive(cc)
		return out
	}

	shared := one(true)
	fresh := one(false)
	t.Logf("CTX %d builds of the same value, everything dropped each time:", rounds)
	t.Logf("CTX   one context between them: %7dKB held afterwards", shared/1024)
	t.Logf("CTX   a context each:           %7dKB held afterwards", fresh/1024)

	// Forty one times, measured. Asserted at five, because this is the
	// premise the whole prepass rests on: a document per batch is only
	// worth building if what it was built in goes away with it. If this
	// ever stops holding, batching has no point and this test is where
	// that should be found out.
	require.Greater(t, shared, fresh*5,
		"a context that let go of what was built in it would hold no more than a fresh one")
}

func flatData(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "x%d: \"value-%d\"\n", i, i)
	}
	return b.String()
}
