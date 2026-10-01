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
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// What the prepass is worth, on the shape it is for, measured at the peak
// rather than at what is left over. Peak is the number a controller dies
// on and it is not the number any of the earlier work reported.

func peakDuring(fn func()) (uint64, time.Duration) {
	// Collected before the watching starts, not after. Starting the
	// sampler first lets it record whatever the last measurement left
	// behind as this one's peak, which makes every run after the first
	// look worse than it is and flatters whatever ran first.
	runtime.GC()

	var highest atomic.Uint64
	done := make(chan struct{})
	go func() {
		var m runtime.MemStats
		for {
			select {
			case <-done:
				return
			default:
			}
			runtime.ReadMemStats(&m)
			for {
				was := highest.Load()
				if m.HeapAlloc <= was || highest.CompareAndSwap(was, m.HeapAlloc) {
					break
				}
			}
			time.Sleep(200 * time.Microsecond)
		}
	}()
	// Closed however fn leaves: a require inside it unwinds the
	// goroutine rather than returning, and the sampler would then read
	// memory statistics, which stop the world, until the test binary
	// exited.
	defer close(done)
	start := time.Now()
	fn()
	took := time.Since(start)
	return highest.Load(), took
}

// stagedLoops is width calls in each of stages levels, every level reading
// the one before it.
func stagedLoops(width, stages int) string {
	var b strings.Builder
	b.WriteString("import \"vela/base64\"\nimport \"list\"\n")
	if stages > 1 {
		// only the later stages slice, and an unused import is an error
		b.WriteString("import \"strings\"\n")
	}
	fmt.Fprintf(&b, "_idx: list.Range(0, %d, 1)\n", width)
	b.WriteString("_s0: {for i in _idx {\"\\(i)\": base64.#Encode & {$params: \"seed-\\(i)\"}}}\n")
	for s := 1; s < stages; s++ {
		// sliced, or base64 of base64 grows four thirds a stage
		fmt.Fprintf(&b, "_s%d: {for i in _idx {\"\\(i)\": base64.#Encode & "+
			"{$params: strings.SliceRunes(_s%d[\"\\(i)\"].$returns, 0, 8)}}}\n", s, s-1)
	}
	fmt.Fprintf(&b, "out: {for i in _idx {\"\\(i)\": _s%d[\"\\(i)\"].$returns}}\n", stages-1)
	return b.String()
}

func TestWhatThePrepassIsWorth(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	off := OptimisePolicy{}
	on := OptimisePolicy{Enabled: true, Threshold: 1}

	for _, tc := range []struct{ width, stages int }{
		// one stage reads nothing of another, so an iteration carries only
		// what the template declared; with stages, it carries the whole of
		// every answer the stage before it gave
		{1000, 1},
		{2000, 1},
		{200, 4},
		{500, 4},
		{1000, 4},
	} {
		src := stagedLoops(tc.width, tc.stages)
		var wantJSON string

		// One render before anything is timed. Whichever arm goes first
		// otherwise pays for loading the provider packages into this
		// compiler, and the ratio below then flatters it.
		_, err := c.CompileStringWithOptions(ctx, src, WithOptimise(off))
		require.NoError(t, err)

		run := func(p OptimisePolicy) (uint64, time.Duration, string) {
			var out string
			peak, took := peakDuring(func() {
				v, err := c.CompileStringWithOptions(ctx, src, WithOptimise(p))
				require.NoError(t, err)
				bs, err := v.MarshalJSON()
				require.NoError(t, err)
				out = string(bs)
				runtime.KeepAlive(v)
			})
			return peak, took, out
		}

		offPeak, offTook, offOut := run(off)
		wantJSON = offOut

		OptimiseStats.Loops.Store(0)
		OptimiseStats.Calls.Store(0)
		onPeak, onTook, onOut := run(on)

		require.Equal(t, wantJSON, onOut,
			"width %d: the prepass must not change what the template renders", tc.width)
		require.NotZero(t, OptimiseStats.Calls.Load(),
			"width %d: if it answered nothing, this measures nothing", tc.width)
		// Why the test exists, so it should fail if that stops being true.
		// Measured between 1.7x and 4.5x depending on the shape; asserted
		// just above level, because a heap peak is a noisy reading and the
		// claim is only about the direction.
		require.Greater(t, offPeak, onPeak*11/10,
			"width %d: answering the loop early should hold less at once", tc.width)

		t.Logf("WORTH %4d wide x %d (%5d calls)  off peak %7dKB %8s   on peak %7dKB %8s   %.2fx peak %.2fx time  (%d loops, %d calls answered)",
			tc.width, tc.stages, tc.width*tc.stages,
			offPeak/1024, offTook.Round(time.Millisecond),
			onPeak/1024, onTook.Round(time.Millisecond),
			float64(offPeak)/float64(onPeak), float64(offTook)/float64(onTook),
			OptimiseStats.Loops.Load(), OptimiseStats.Calls.Load())
	}
}
