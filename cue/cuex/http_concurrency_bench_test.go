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
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	httpprovider "github.com/kubevela/pkg/cue/cuex/providers/http"
)

// latentServer answers after a fixed delay, standing in for a real endpoint.
// It also records how many requests were in flight at once, which is what
// separates a concurrency win from a faster machine.
type latentServer struct {
	*httptest.Server
	inFlight int32
	peak     int32
	served   int32
}

func newLatentServer(latency time.Duration) *latentServer {
	s := &latentServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		now := atomic.AddInt32(&s.inFlight, 1)
		for {
			peak := atomic.LoadInt32(&s.peak)
			if now <= peak || atomic.CompareAndSwapInt32(&s.peak, peak, now) {
				break
			}
		}
		time.Sleep(latency)
		atomic.AddInt32(&s.inFlight, -1)
		atomic.AddInt32(&s.served, 1)
		// Date moves on between two runs of the same template, so drop it and
		// leave a response that can be compared.
		w.Header()["Date"] = nil
		_, _ = w.Write([]byte(`ok`))
	}))
	return s
}

// httpFanTemplate makes n independent GETs against one endpoint, which is the
// shape @concurrency exists for: a loop whose calls do not read each other.
func httpFanTemplate(url string, n int, attr string) string {
	var b strings.Builder
	b.WriteString("import \"vela/http\"\nreads: {\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "\t\"%d\": http.#Get & {$params: url: \"%s/%d\"}\n", i, url, i)
	}
	b.WriteString("}")
	b.WriteString(attr)
	b.WriteString("\n")
	return b.String()
}

func httpCompiler() *cuex.Compiler {
	return cuex.NewCompilerWithInternalPackages(httpprovider.Package)
}

// TestHTTPConcurrencyIsReal checks the marker actually overlaps the requests
// before any timing is reported, and that the answers are identical either way.
// A benchmark alone cannot tell a real overlap from a quick machine.
func TestHTTPConcurrencyIsReal(t *testing.T) {
	const n = 8
	srv := newLatentServer(50 * time.Millisecond)
	defer srv.Close()

	c := httpCompiler()
	seq, err := c.CompileString(context.Background(), httpFanTemplate(srv.URL, n, ""))
	require.NoError(t, err)
	require.EqualValues(t, 1, atomic.LoadInt32(&srv.peak), "unmarked calls must stay one at a time")

	atomic.StoreInt32(&srv.peak, 0)
	con, err := c.CompileString(context.Background(), httpFanTemplate(srv.URL, n, " @concurrency(8)"))
	require.NoError(t, err)
	require.EqualValues(t, n, atomic.LoadInt32(&srv.peak), "marked calls must overlap")

	want, err := seq.MarshalJSON()
	require.NoError(t, err)
	got, err := con.MarshalJSON()
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got), "running together must not change the answer")
}

// BenchmarkHTTPFanout is the measurement: the same n requests, sequential
// against @concurrency(n), at a latency that stands in for a real endpoint.
func BenchmarkHTTPFanout(b *testing.B) {
	for _, latency := range []time.Duration{1 * time.Millisecond, 10 * time.Millisecond} {
		for _, n := range []int{5, 20, 50} {
			srv := newLatentServer(latency)
			c := httpCompiler()
			seq := httpFanTemplate(srv.URL, n, "")
			con := httpFanTemplate(srv.URL, n, fmt.Sprintf(" @concurrency(%d)", n))

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
			srv.Close()
		}
	}
}
