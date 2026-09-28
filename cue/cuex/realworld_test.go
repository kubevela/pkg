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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// stages watches what a provider has in flight, per function, so a test can
// claim that a call never overlaps the call it depends on. A clock would put
// that claim at the mercy of how loaded the machine is; this does not.
type stages struct {
	mu       sync.Mutex
	inFlight map[string]int
	peak     map[string]int
	overlap  bool // a later stage ran while an earlier one was still going
}

func newStages() *stages {
	return &stages{inFlight: map[string]int{}, peak: map[string]int{}}
}

func (in *stages) enter(fn string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.inFlight[fn]++
	if in.inFlight[fn] > in.peak[fn] {
		in.peak[fn] = in.inFlight[fn]
	}
	// get is stage one and apply is everything after it, so the two being in
	// flight together is the stages running into each other
	if in.inFlight["get"] > 0 && in.inFlight["apply"] > 0 {
		in.overlap = true
	}
}

func (in *stages) leave(fn string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.inFlight[fn]--
}

func (in *stages) peaked(fn string) int {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.peak[fn]
}

func (in *stages) ranIntoEachOther() bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.overlap
}

// ioCompiler builds a compiler with one provider that waits, the way one
// talking to an API server does.
func ioCompiler(tb testing.TB, latency time.Duration, concurrent bool) *cuex.Compiler {
	tb.Helper()
	var fn cuexruntime.ProviderFn = cuexruntime.GenericProviderFn[waitParams, waitReturns](
		func(_ context.Context, p *waitParams) (*waitReturns, error) {
			time.Sleep(latency)
			return &waitReturns{Returns: p.Params}, nil
		})
	if concurrent {
		fn = cuexruntime.Concurrent(fn)
	}
	pkg, err := cuexruntime.NewInternalPackage("io", `
package io

#Get: {
	#do:       "get"
	#provider: "io"
	#config: maxPerRender: 64
	$params:   string
	$returns?: string
}

#Apply: {
	#do:       "apply"
	#provider: "io"
	#config: maxPerRender: 64
	$params:   string
	$returns?: string
}`, map[string]cuexruntime.ProviderFn{"get": fn, "apply": fn})
	require.NoError(tb, err)
	return cuex.NewCompilerWithInternalPackages(pkg)
}

// TestRealWorldMatchesReference runs the mixed shape - independent calls, a
// chain, a fanout over a loop, a call gathering it back up, and a manifest -
// through both resolvers, with and without the template asking for
// concurrency. Running calls together must not change a byte.
func TestRealWorldMatchesReference(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	for _, fanout := range []int{1, 5, 20} {
		for _, attr := range []string{"", " @concurrency(8)"} {
			t.Run(fmt.Sprintf("fanout=%d attr=%q", fanout, attr), func(t *testing.T) {
				src := realWorldTemplate("base64", "#Encode", fanout, attr)
				wantOut, wantErr := render(referenceResolve(ctx, c, buildValue(t, c, src)))
				require.Empty(t, wantErr)
				gotOut, gotErr := render(c.CompileString(ctx, src))
				require.Equal(t, wantErr, gotErr)
				require.Equal(t, wantOut, gotOut)
			})
		}
	}
}

// TestRealWorldConcurrencyRendersTheSame is the same shape driven by a
// provider that is marked safe to run alongside itself, so the fanout really
// does overlap.
func TestRealWorldConcurrencyRendersTheSame(t *testing.T) {
	ctx := context.Background()
	src := realWorldTemplate("io", "#Get", 20, " @concurrency(8)")
	sequential, err := ioCompiler(t, 0, false).CompileString(ctx, src)
	require.NoError(t, err)
	concurrent, err := ioCompiler(t, 0, true).CompileString(ctx, src)
	require.NoError(t, err)

	want, err := sequential.MarshalJSON()
	require.NoError(t, err)
	got, err := concurrent.MarshalJSON()
	require.NoError(t, err)
	require.Equal(t, string(want), string(got))
}

// BenchmarkRealWorld is the shape at the sizes it comes in, against the
// resolver this replaced and with the fanout running together or not.
func BenchmarkRealWorld(b *testing.B) {
	ctx := context.Background()
	for _, fanout := range []int{5, 20, 100} {
		// a pure provider, where the resolver is the whole cost
		b.Run(fmt.Sprintf("pure/fanout=%d/now", fanout), func(b *testing.B) {
			c := cuex.NewCompilerWithDefaultInternalPackages()
			src := realWorldTemplate("base64", "#Encode", fanout, "")
			benchRealWorld(b, c, ctx, src, false)
		})
		b.Run(fmt.Sprintf("pure/fanout=%d/reference", fanout), func(b *testing.B) {
			c := cuex.NewCompilerWithDefaultInternalPackages()
			src := realWorldTemplate("base64", "#Encode", fanout, "")
			benchRealWorld(b, c, ctx, src, true)
		})
		// one that waits, where the calls are
		for _, tt := range []struct {
			name       string
			attr       string
			concurrent bool
		}{
			{"waiting/sequential", "", false},
			{"waiting/concurrent", " @concurrency(32)", true},
		} {
			b.Run(fmt.Sprintf("%s/fanout=%d", tt.name, fanout), func(b *testing.B) {
				c := ioCompiler(b, time.Millisecond, tt.concurrent)
				src := realWorldTemplate("io", "#Get", fanout, tt.attr)
				benchRealWorld(b, c, ctx, src, false)
			})
		}
	}
}

func benchRealWorld(b *testing.B, c *cuex.Compiler, ctx context.Context, src string, reference bool) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		if reference {
			v := buildScaleValue(b, c, src)
			_, err = referenceResolve(ctx, c, v)
		} else {
			_, err = c.CompileString(ctx, src)
		}
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestNamespaceFanMatchesReference runs the read-every-namespace-then-create
// shape through both resolvers, with and without the template asking for
// concurrency.
func TestNamespaceFanMatchesReference(t *testing.T) {
	ctx := context.Background()
	for _, n := range []int{1, 5, 20} {
		for _, attr := range []string{"", " @concurrency(8)"} {
			t.Run(fmt.Sprintf("n=%d attr=%q", n, attr), func(t *testing.T) {
				src := namespaceFanTemplate(n, attr)
				c := ioCompiler(t, 0, false)
				wantOut, wantErr := render(referenceResolve(ctx, c, buildValue(t, c, src)))
				require.Empty(t, wantErr)
				gotOut, gotErr := render(ioCompiler(t, 0, true).CompileString(ctx, src))
				require.Equal(t, wantErr, gotErr)
				require.Equal(t, wantOut, gotOut)
			})
		}
	}
}

// stagedCompiler is ioCompiler with the two functions told apart, so what ran
// beside what can be read off afterwards.
func stagedCompiler(tb testing.TB, latency time.Duration, rec *stages) *cuex.Compiler {
	tb.Helper()
	mk := func(name string) cuexruntime.ProviderFn {
		return cuexruntime.Concurrent(cuexruntime.GenericProviderFn[waitParams, waitReturns](
			func(_ context.Context, p *waitParams) (*waitReturns, error) {
				rec.enter(name)
				time.Sleep(latency)
				rec.leave(name)
				return &waitReturns{Returns: p.Params}, nil
			}))
	}
	pkg, err := cuexruntime.NewInternalPackage("io", `
package io

#Get: {
	#do:       "get"
	#provider: "io"
	#config: maxPerRender: 64
	$params:   string
	$returns?: string
}

#Apply: {
	#do:       "apply"
	#provider: "io"
	#config: maxPerRender: 64
	$params:   string
	$returns?: string
}`, map[string]cuexruntime.ProviderFn{"get": mk("get"), "apply": mk("apply")})
	require.NoError(tb, err)
	return cuex.NewCompilerWithInternalPackages(pkg)
}

// TestNamespaceFanRunsInTwoStages is the property the whole dependency
// ordering exists for. Every read can go at once and every create can go at
// once, but no create before its own read.
//
// It reads that off what the provider had in flight rather than off a clock:
// the reads overlap each other, the creates overlap each other, and the two
// never overlap. That says the same thing on a slow machine, under the race
// detector, or on a busy one.
func TestNamespaceFanRunsInTwoStages(t *testing.T) {
	const n, latency = 32, 5 * time.Millisecond
	rec := newStages()
	_, err := stagedCompiler(t, latency, rec).
		CompileString(context.Background(), namespaceFanTemplate(n, fmt.Sprintf(" @concurrency(%d)", n)))
	require.NoError(t, err)

	require.Greater(t, rec.peaked("get"), 1, "the reads should have run together")
	require.Greater(t, rec.peaked("apply"), 1, "and so should the creates")
	require.False(t, rec.ranIntoEachOther(),
		"a create ran while a read was still going, so this is not two stages")
}

// TestDeepFanMatchesReference runs the fleet shape - read every namespace,
// read a config map in each, create a resource from what came back - through
// both resolvers, with and without the template asking for concurrency.
func TestDeepFanMatchesReference(t *testing.T) {
	ctx := context.Background()
	for _, n := range []int{1, 5, 20} {
		for _, attr := range []string{"", " @concurrency(8)"} {
			t.Run(fmt.Sprintf("n=%d attr=%q", n, attr), func(t *testing.T) {
				src := deepFanTemplate(n, attr)
				c := ioCompiler(t, 0, false)
				wantOut, wantErr := render(referenceResolve(ctx, c, buildValue(t, c, src)))
				require.Empty(t, wantErr)
				gotOut, gotErr := render(ioCompiler(t, 0, true).CompileString(ctx, src))
				require.Equal(t, wantErr, gotErr)
				require.Equal(t, wantOut, gotOut)
			})
		}
	}
}

// TestDeepFanRunsInThreeStages is the same property one level deeper: read a
// namespace, read a config map in it, create from what came back. The reads
// overlap and no stage runs into the one above it, again read off what was in
// flight rather than off a clock.
func TestDeepFanRunsInThreeStages(t *testing.T) {
	const n, latency = 24, 5 * time.Millisecond
	rec := newStages()
	_, err := stagedCompiler(t, latency, rec).
		CompileString(context.Background(), deepFanTemplate(n, fmt.Sprintf(" @concurrency(%d)", n)))
	require.NoError(t, err)

	require.Greater(t, rec.peaked("get"), 1, "the reads should have run together")
	require.False(t, rec.ranIntoEachOther(),
		"a later stage ran while the one above it was still going")
}

// BenchmarkDeepFan is the fleet shape at fleet sizes.
//
// The resolver this replaced grows faster than the square of the call count -
// 155 of them took 694ms, 305 took 2.7s, 605 took 13.2s - so it is only
// measured here where it still finishes quickly. A thousand namespaces, which
// is 3005 calls, took it 47m45s and 145GB of allocation, against 432ms and
// 259MB for this one.
func BenchmarkDeepFan(b *testing.B) {
	ctx := context.Background()
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprintf("pure/n=%d/now", n), func(b *testing.B) {
			benchRealWorld(b, ioCompiler(b, 0, false), ctx, deepFanTemplate(n, ""), false)
		})
		if n <= 100 {
			b.Run(fmt.Sprintf("pure/n=%d/reference", n), func(b *testing.B) {
				benchRealWorld(b, ioCompiler(b, 0, false), ctx, deepFanTemplate(n, ""), true)
			})
		}
		b.Run(fmt.Sprintf("waiting/n=%d/sequential", n), func(b *testing.B) {
			benchRealWorld(b, ioCompiler(b, time.Millisecond, false), ctx, deepFanTemplate(n, ""), false)
		})
		b.Run(fmt.Sprintf("waiting/n=%d/concurrent", n), func(b *testing.B) {
			benchRealWorld(b, ioCompiler(b, time.Millisecond, true), ctx,
				deepFanTemplate(n, " @concurrency(64)"), false)
		})
	}
}
