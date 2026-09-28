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
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

type waitParams struct {
	Params string `json:"$params"`
}
type waitReturns struct {
	Returns string `json:"$returns"`
}

// waiter is a provider that waits, the way one talking to an API server does,
// and records how many of its calls were ever in flight at once.
type waiter struct {
	latency  time.Duration
	inFlight atomic.Int32
	peak     atomic.Int32
	failOn   map[string]bool
}

func (w *waiter) call(_ context.Context, p *waitParams) (*waitReturns, error) {
	n := w.inFlight.Add(1)
	for {
		peak := w.peak.Load()
		if n <= peak || w.peak.CompareAndSwap(peak, n) {
			break
		}
	}
	time.Sleep(w.latency)
	w.inFlight.Add(-1)
	if w.failOn[p.Params] {
		return nil, fmt.Errorf("refused %s", p.Params)
	}
	return &waitReturns{Returns: "ok-" + p.Params}, nil
}

// waitingCompiler builds a compiler with one provider. concurrent says whether
// the function is marked safe to run alongside itself.
func waitingCompiler(t *testing.T, w *waiter, concurrent bool) *cuex.Compiler {
	t.Helper()
	var fn cuexruntime.ProviderFn = cuexruntime.GenericProviderFn[waitParams, waitReturns](w.call)
	if concurrent {
		fn = cuexruntime.Concurrent(fn)
	}
	pkg, err := cuexruntime.NewInternalPackage("wait", `
package wait

#Get: {
	#do:       "get"
	#provider: "wait"
	#config: maxPerRender: 64
	$params:   string
	$returns?: string
}`, map[string]cuexruntime.ProviderFn{"get": fn})
	require.NoError(t, err)
	return cuex.NewCompilerWithInternalPackages(pkg)
}

// fanTemplate is the shape this feature is for: a comprehension writing n
// calls into a struct, with the attribute above them.
func fanTemplate(n int, attr string) string {
	var b strings.Builder
	b.WriteString("import \"vela/wait\"\nreads: {\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "\t\"%d\": wait.#Get & {$params: \"k-%d\"}\n", i, i)
	}
	b.WriteString("}")
	b.WriteString(attr)
	b.WriteString("\n")
	return b.String()
}

func TestConcurrencyRunsCallsTogether(t *testing.T) {
	const n, latency = 8, 60 * time.Millisecond
	for name, tt := range map[string]struct {
		attr       string
		concurrent bool
		wantPeak   int32
	}{
		"marked, provider agrees":      {" @concurrency(8)", true, 8},
		"marked to two at a time":      {" @concurrency(2)", true, 2},
		"not marked":                   {"", true, 1},
		"marked, provider stays quiet": {" @concurrency(8)", false, 1},
	} {
		t.Run(name, func(t *testing.T) {
			w := &waiter{latency: latency}
			c := waitingCompiler(t, w, tt.concurrent)
			start := time.Now()
			_, err := c.CompileString(context.Background(), fanTemplate(n, tt.attr))
			elapsed := time.Since(start)
			require.NoError(t, err)
			// the peak is exact and does not care how loaded the machine is.
			// A wall clock upper bound here would only say the same thing
			// less reliably, so the clock is read for the serial case alone,
			// where a sleep cannot run short.
			require.Equal(t, tt.wantPeak, w.peak.Load(), "calls in flight at once")
			if tt.wantPeak == 1 {
				require.Greater(t, elapsed, time.Duration(n-1)*latency,
					"calls should have run one after another")
			}
		})
	}
}

// TestConcurrencyRendersTheSame is the guarantee that matters: running calls
// together must not change a byte of what comes out.
func TestConcurrencyRendersTheSame(t *testing.T) {
	const n = 12
	sequential, err := waitingCompiler(t, &waiter{}, false).
		CompileString(context.Background(), fanTemplate(n, " @concurrency(6)"))
	require.NoError(t, err)
	concurrent, err := waitingCompiler(t, &waiter{}, true).
		CompileString(context.Background(), fanTemplate(n, " @concurrency(6)"))
	require.NoError(t, err)

	want, err := sequential.MarshalJSON()
	require.NoError(t, err)
	got, err := concurrent.MarshalJSON()
	require.NoError(t, err)
	require.Equal(t, string(want), string(got))
}

// TestConcurrencyReportsTheFirstFailureInOrder pins error determinism: with
// several calls failing at once, the one reported is the first in the
// template rather than whichever lost the race.
func TestConcurrencyReportsTheFirstFailureInOrder(t *testing.T) {
	for i := 0; i < 20; i++ {
		w := &waiter{failOn: map[string]bool{"k-3": true, "k-5": true, "k-7": true}}
		c := waitingCompiler(t, w, true)
		_, err := c.CompileString(context.Background(), fanTemplate(10, " @concurrency(10)"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "refused k-3",
			"run %d reported a later failure than the first one", i)
	}
}

// TestConcurrencyIgnoresNativeProviders covers the one kind that can never
// take part: it is handed a cue.Value and reads it throughout, so there is
// nowhere to put the seam between reading and working.
func TestConcurrencyIgnoresNativeProviders(t *testing.T) {
	var inFlight, peak atomic.Int32
	var mu sync.Mutex
	native := cuexruntime.NativeProviderFn(func(_ context.Context, v cue.Value) (cue.Value, error) {
		n := inFlight.Add(1)
		mu.Lock()
		if n > peak.Load() {
			peak.Store(n)
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		inFlight.Add(-1)
		return v.FillPath(cue.ParsePath("$returns"), "done"), nil
	})
	_, splittable := cuexruntime.Concurrent(native).(cuexruntime.ConcurrentProviderFn)
	require.False(t, splittable,
		"marking a native provider must not make it look like one that can be split")

	pkg, err := cuexruntime.NewInternalPackage("nat", `
package nat

#Do: {
	#do:       "do"
	#provider: "nat"
	$params:   string
	$returns?: string
}`, map[string]cuexruntime.ProviderFn{"do": cuexruntime.Concurrent(native)})
	require.NoError(t, err)
	c := cuex.NewCompilerWithInternalPackages(pkg)

	src := "import \"vela/nat\"\nreads: {\n"
	for i := 0; i < 4; i++ {
		src += fmt.Sprintf("\t\"%d\": nat.#Do & {$params: \"k-%d\"}\n", i, i)
	}
	src += "} @concurrency(4)\n"

	_, err = c.CompileString(context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, int32(1), peak.Load(), "a native provider must run one call at a time")
}

// TestConcurrencyNeedsAStruct pins where the attribute has to go. A file-level
// one reaches nothing, and a template written that way would quietly keep
// running its calls one at a time.
func TestConcurrencyNeedsAStruct(t *testing.T) {
	const n, latency = 6, 40 * time.Millisecond
	for name, tt := range map[string]struct {
		src      string
		wantPeak int32
	}{
		"on the struct": {fanTemplate(n, " @concurrency(6)"), 6},
		"at file level": {"@concurrency(6)\n" + fanTemplate(n, ""), 1},
	} {
		t.Run(name, func(t *testing.T) {
			w := &waiter{latency: latency}
			_, err := waitingCompiler(t, w, true).CompileString(context.Background(), tt.src)
			require.NoError(t, err)
			require.Equal(t, tt.wantPeak, w.peak.Load())
		})
	}
}

// TestConcurrencyStopsAtTheDeadline: a group of calls running together is no
// freer to overrun the deadline than one call running on its own. The deadline
// is checked per call rather than once for the group, which is where fan-out
// makes a timeout matter.
func TestConcurrencyStopsAtTheDeadline(t *testing.T) {
	const n, limit, latency = 40, 4, 40 * time.Millisecond
	w := &waiter{latency: latency}
	c := waitingCompiler(t, w, true)

	// room for a couple of rounds of the pool, not for all ten
	ctx, cancel := context.WithTimeout(context.Background(), 3*latency)
	defer cancel()

	start := time.Now()
	_, err := c.Resolve(ctx, buildValue(t, c, fanTemplate(n, fmt.Sprintf(" @concurrency(%d)", limit))))
	elapsed := time.Since(start)

	require.ErrorAs(t, err, &cuex.ResolveTimeoutErr{})
	require.Less(t, elapsed, time.Duration(n/limit)*latency,
		"the group ran to the end of itself instead of stopping: %v", elapsed)
}

// TestConcurrencyUsesAPoolNotAGoroutinePerCall: the bound is on how many
// goroutines exist, not only on how many are working. A thousand calls at
// eight at a time should not park 992 goroutines.
func TestConcurrencyUsesAPoolNotAGoroutinePerCall(t *testing.T) {
	const n, limit = 400, 4
	w := &waiter{latency: time.Millisecond}
	c := waitingCompiler(t, w, true)

	before := runtime.NumGoroutine()
	var peak atomic.Int64
	peak.Store(int64(before))
	done := make(chan struct{})
	sampling := make(chan struct{})
	go func() {
		defer close(sampling)
		for {
			select {
			case <-done:
				return
			default:
				if now := int64(runtime.NumGoroutine()); now > peak.Load() {
					peak.Store(now)
				}
				time.Sleep(time.Millisecond / 4)
			}
		}
	}()

	_, err := c.CompileString(context.Background(), fanTemplate(n, fmt.Sprintf(" @concurrency(%d)", limit)))
	close(done)
	<-sampling
	require.NoError(t, err)
	require.Equal(t, int32(limit), w.peak.Load(), "the limit is still honoured")
	require.Less(t, int(peak.Load())-before, n/2,
		"%d calls at %d at a time should not have started anything like %d goroutines", n, limit, n)
}
