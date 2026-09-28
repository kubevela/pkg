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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// peaker counts how many calls one function had at once.
type peaker struct {
	mu   sync.Mutex
	now  int
	peak int
}

func (p *peaker) run(latency time.Duration) {
	p.mu.Lock()
	p.now++
	if p.now > p.peak {
		p.peak = p.now
	}
	p.mu.Unlock()
	time.Sleep(latency)
	p.mu.Lock()
	p.now--
	p.mu.Unlock()
}

func (p *peaker) seen() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.peak
}

// TestInternalPerFunctionLimits: an internal package declares its ceilings the
// same way an external one does, so the two kinds are not a special case of
// each other.
func TestInternalPerFunctionLimits(t *testing.T) {
	cheap, costly := &peaker{}, &peaker{}
	mk := func(p *peaker) cuexruntime.ProviderFn {
		return cuexruntime.Concurrent(cuexruntime.GenericProviderFn[waitParams, waitReturns](
			func(_ context.Context, in *waitParams) (*waitReturns, error) {
				p.run(20 * time.Millisecond)
				return &waitReturns{Returns: in.Params}, nil
			}))
	}
	pkg, err := cuexruntime.NewInternalPackage("two", `
package two

#Cheap: {
	#do:          "cheap"
	#provider:    "two"
	#config: maxPerRender: 8
	$params:      string
	$returns?:    string
}

#Costly: {
	#do:          "costly"
	#provider:    "two"
	#config: maxPerRender: 2
	$params:      string
	$returns?:    string
}
`, map[string]cuexruntime.ProviderFn{"cheap": mk(cheap), "costly": mk(costly)})
	require.NoError(t, err)

	var b strings.Builder
	b.WriteString("import \"vela/two\"\ncheap: {\n")
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&b, "\t\"%d\": two.#Cheap & {$params: \"c-%d\"}\n", i, i)
	}
	b.WriteString("} @concurrency(10)\ncostly: {\n")
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&b, "\t\"%d\": two.#Costly & {$params: \"x-%d\"}\n", i, i)
	}
	b.WriteString("} @concurrency(10)\n")

	_, err = cuex.NewCompilerWithInternalPackages(pkg).
		CompileString(context.Background(), b.String())
	require.NoError(t, err)

	require.Equal(t, 8, cheap.seen(), "an internal function keeps its own ceiling")
	require.Equal(t, 2, costly.seen(), "and is not given another function's")
}
