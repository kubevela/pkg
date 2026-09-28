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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubevela/pkg/apis/cue/v1alpha1"
	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// countingEndpoint answers an external provider call after a delay, recording
// how many calls it had at once. The peak is what separates a real overlap
// from a quick machine, and it is what the cap is checked against.
type countingEndpoint struct {
	*httptest.Server
	inFlight atomic.Int32
	peak     atomic.Int32
}

func newCountingEndpoint(latency time.Duration) *countingEndpoint {
	e := &countingEndpoint{}
	e.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		now := e.inFlight.Add(1)
		for {
			peak := e.peak.Load()
			if now <= peak || e.peak.CompareAndSwap(peak, now) {
				break
			}
		}
		time.Sleep(latency)
		e.inFlight.Add(-1)
		_, _ = w.Write([]byte(`{"answered":true}`))
	}))
	return e
}

// externalCompiler builds a compiler around a Package declaring an external
// provider, which is the path a user's own provider takes.
func externalCompiler(tb testing.TB, endpoint string, declared string) *cuex.Compiler {
	tb.Helper()
	pkg, err := cuexruntime.NewExternalPackage(&v1alpha1.Package{
		ObjectMeta: metav1.ObjectMeta{Name: "ext"},
		Spec: v1alpha1.PackageSpec{
			Path: "ext",
			Provider: &v1alpha1.Provider{
				Protocol: v1alpha1.ProtocolHTTP,
				Endpoint: endpoint,
			},
			Templates: map[string]string{
				"ext.cue": `
package ext

#Ask: {
	#do:       "ask"
	#provider: "ext"
	$params:   string
	$returns?: {...}
	` + declared + `
}`,
			},
		},
	})
	require.NoError(tb, err)
	return cuex.NewCompilerWithInternalPackages(pkg)
}

func externalFanTemplate(n int, attr string) string {
	var b strings.Builder
	b.WriteString("import \"ext\"\nasks: {\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "\t\"%d\": ext.#Ask & {$params: \"k-%d\"}\n", i, i)
	}
	b.WriteString("}")
	b.WriteString(attr)
	b.WriteString("\n")
	return b.String()
}

// TestExternalProviderRunsConcurrently: a user's own provider can run its
// calls together once its own template says how many it will take, and gets
// none until a template asks as well. Both ends have to agree, and the owner
// of what is on the other end is the one who knows.
func TestExternalProviderRunsConcurrently(t *testing.T) {
	const n = 8
	e := newCountingEndpoint(30 * time.Millisecond)
	defer e.Close()
	c := externalCompiler(t, e.URL, "#config: maxPerRender: 8")

	seq, err := c.CompileString(context.Background(), externalFanTemplate(n, ""))
	require.NoError(t, err)
	require.EqualValues(t, 1, e.peak.Load(), "no template asked, so one call at a time")

	e.peak.Store(0)
	con, err := c.CompileString(context.Background(), externalFanTemplate(n, " @concurrency(8)"))
	require.NoError(t, err)
	require.EqualValues(t, n, e.peak.Load(), "both ends agreed, so they overlap")

	want, err := seq.MarshalJSON()
	require.NoError(t, err)
	got, err := con.MarshalJSON()
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got), "running together must not change the answer")
}

// TestExternalMaxPerRenderCaps covers the ceiling in both directions. It is a
// ceiling only: it brings a template down and never lifts one up, so a Package
// declaring a limit does not turn concurrency on by itself.
func TestExternalMaxPerRenderCaps(t *testing.T) {
	const n = 12
	for name, tt := range map[string]struct {
		declared string
		attr     string
		wantPeak int32
	}{
		"declares nothing":               {"", " @concurrency(6)", 1},
		"cap below what was asked":       {"#config: maxPerRender: 3", " @concurrency(12)", 3},
		"cap above what was asked":       {"#config: maxPerRender: 10", " @concurrency(4)", 4},
		"cap equal to what was asked":    {"#config: maxPerRender: 5", " @concurrency(5)", 5},
		"cap of one":                     {"#config: maxPerRender: 1", " @concurrency(12)", 1},
		"cap set but nothing asked":      {"#config: maxPerRender: 8", "", 1},
		"no cap and nothing asked":       {"", "", 1},
		"cap of zero runs one at a time": {"#config: maxPerRender: 0", " @concurrency(6)", 1},
	} {
		t.Run(name, func(t *testing.T) {
			e := newCountingEndpoint(25 * time.Millisecond)
			defer e.Close()
			c := externalCompiler(t, e.URL, tt.declared)

			_, err := c.CompileString(context.Background(), externalFanTemplate(n, tt.attr))
			require.NoError(t, err)
			require.EqualValues(t, tt.wantPeak, e.peak.Load(),
				"a provider's limit caps a template, never raises it")
		})
	}
}

// BenchmarkExternalFanout measures what the wrap is worth on the path a user's
// own provider takes.
func BenchmarkExternalFanout(b *testing.B) {
	for _, latency := range []time.Duration{1 * time.Millisecond, 10 * time.Millisecond} {
		for _, n := range []int{5, 20, 50} {
			e := newCountingEndpoint(latency)
			c := externalCompiler(b, e.URL, fmt.Sprintf("#config: maxPerRender: %d", n))
			seq := externalFanTemplate(n, "")
			con := externalFanTemplate(n, fmt.Sprintf(" @concurrency(%d)", n))

			b.Run(fmt.Sprintf("latency=%s/n=%d/sequential", latency, n), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					if _, err := c.CompileString(context.Background(), seq); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run(fmt.Sprintf("latency=%s/n=%d/concurrent", latency, n), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					if _, err := c.CompileString(context.Background(), con); err != nil {
						b.Fatal(err)
					}
				}
			})
			e.Close()
		}
	}
}

// perPathEndpoint records the peak for each function separately, which is what
// a per-function ceiling has to be measured against.
type perPathEndpoint struct {
	*httptest.Server
	mu    sync.Mutex
	now   map[string]int
	peaks map[string]int
}

func newPerPathEndpoint(latency time.Duration) *perPathEndpoint {
	e := &perPathEndpoint{now: map[string]int{}, peaks: map[string]int{}}
	e.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fn := strings.TrimPrefix(r.URL.Path, "/")
		e.mu.Lock()
		e.now[fn]++
		if e.now[fn] > e.peaks[fn] {
			e.peaks[fn] = e.now[fn]
		}
		e.mu.Unlock()
		time.Sleep(latency)
		e.mu.Lock()
		e.now[fn]--
		e.mu.Unlock()
		_, _ = w.Write([]byte(`{"answered":true}`))
	}))
	return e
}

func (e *perPathEndpoint) peak(fn string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.peaks[fn]
}

// TestExternalPerFunctionLimits is the reason the ceiling lives beside the
// function rather than beside the endpoint: one provider, two functions, and a
// cheap one is not held to what an expensive one can take.
func TestExternalPerFunctionLimits(t *testing.T) {
	e := newPerPathEndpoint(25 * time.Millisecond)
	defer e.Close()

	pkg, err := cuexruntime.NewExternalPackage(&v1alpha1.Package{
		ObjectMeta: metav1.ObjectMeta{Name: "two"},
		Spec: v1alpha1.PackageSpec{
			Path: "two",
			Provider: &v1alpha1.Provider{
				Protocol: v1alpha1.ProtocolHTTP,
				Endpoint: e.URL,
			},
			Templates: map[string]string{"two.cue": `
package two

#Cheap: {
	#do:          "cheap"
	#provider:    "two"
	#config: maxPerRender: 8
	$params:      string
	$returns?: {...}
}

#Costly: {
	#do:          "costly"
	#provider:    "two"
	#config: maxPerRender: 2
	$params:      string
	$returns?: {...}
}
`},
		},
	})
	require.NoError(t, err)
	c := cuex.NewCompilerWithInternalPackages(pkg)

	var b strings.Builder
	b.WriteString("import \"two\"\ncheap: {\n")
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&b, "\t\"%d\": two.#Cheap & {$params: \"c-%d\"}\n", i, i)
	}
	b.WriteString("} @concurrency(10)\ncostly: {\n")
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&b, "\t\"%d\": two.#Costly & {$params: \"x-%d\"}\n", i, i)
	}
	b.WriteString("} @concurrency(10)\n")

	_, err = c.CompileString(context.Background(), b.String())
	require.NoError(t, err)

	require.Equal(t, 8, e.peak("cheap"), "the cheap function should get its own ceiling")
	require.Equal(t, 2, e.peak("costly"), "the costly function should not be given the cheap one's")
}

// TestDeclaredCeilingCannotBeRaised: a template can put a second maxPerRender
// on the call, which leaves the field conflicting and still compiles. The
// ceiling has to hold anyway, so an unreadable one means one call at a time
// rather than none.
func TestDeclaredCeilingCannotBeRaised(t *testing.T) {
	e := newCountingEndpoint(5 * time.Millisecond)
	defer e.Close()
	c := externalCompiler(t, e.URL, "#config: maxPerRender: 2")

	var b strings.Builder
	b.WriteString("import \"ext\"\nasks: {\n")
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&b, "\t\"%d\": ext.#Ask & {$params: \"a-%d\", #config: maxPerRender: 50}\n", i, i)
	}
	b.WriteString("} @concurrency(10)\n")

	_, err := c.CompileString(context.Background(), b.String())
	require.NoError(t, err)
	require.EqualValues(t, 1, e.peak.Load(),
		"a ceiling that cannot be read must not become no ceiling")
}

// TestMaxPerRenderIsNotGlobal records what #config.maxPerRender does not do.
// It caps one render, and renders run concurrently, so the number an endpoint
// actually sees is this times however many are in flight. An endpoint needing
// a total limit has to hold one itself.
func TestMaxPerRenderIsNotGlobal(t *testing.T) {
	const (
		renders      = 3
		maxPerRender = 2
	)
	e := newCountingEndpoint(60 * time.Millisecond)
	defer e.Close()
	src := externalFanTemplate(6, " @concurrency(6)")

	// a compiler each: sharing one across goroutines races inside CUE, on the
	// *build.Instance a package holds, which is not what this is measuring.
	var wg sync.WaitGroup
	for i := 0; i < renders; i++ {
		c := externalCompiler(t, e.URL, fmt.Sprintf("#config: maxPerRender: %d", maxPerRender))
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.CompileString(context.Background(), src)
			require.NoError(t, err)
		}()
	}
	wg.Wait()

	require.Greater(t, int(e.peak.Load()), maxPerRender,
		"the ceiling is per render, so concurrent renders exceed it together")
	require.LessOrEqual(t, int(e.peak.Load()), renders*maxPerRender,
		"but each render is still held to its own")
}

// TestNothingDeclaredMeansOneAtATime is the safe default, and the reason for
// it: a provider function is marked in Go as safe to split, but splittable is
// not the same as safe to reorder. Whoever owns what is on the other end says
// how many it will take, and until they do it runs as it always did.
func TestNothingDeclaredMeansOneAtATime(t *testing.T) {
	e := newCountingEndpoint(20 * time.Millisecond)
	defer e.Close()

	quiet := externalCompiler(t, e.URL, "")
	_, err := quiet.CompileString(context.Background(), externalFanTemplate(20, " @concurrency(200)"))
	require.NoError(t, err)
	require.EqualValues(t, 1, e.peak.Load(),
		"a template cannot give itself concurrency the function never offered")

	e.peak.Store(0)
	opted := externalCompiler(t, e.URL, "#config: maxPerRender: 5")
	_, err = opted.CompileString(context.Background(), externalFanTemplate(20, " @concurrency(200)"))
	require.NoError(t, err)
	require.EqualValues(t, 5, e.peak.Load(),
		"and gets exactly what the function did offer, not what it asked for")
}
