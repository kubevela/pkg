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
	"sync/atomic"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

type stubParams struct {
	Params string `json:"$params"`
}

type stubReturns struct {
	Returns string `json:"$returns"`
}

type stubProvider struct{ fn cuexruntime.ProviderFn }

func (in stubProvider) GetName() string                             { return "stub" }
func (in stubProvider) GetProviderFn(string) cuexruntime.ProviderFn { return in.fn }

// TestRunTogetherReportsOnlyWhatRan: a call the deadline stopped being reached
// has no error against it and no result either. Reading "no error" as success
// hands back a result for work that never happened, which runLevel then marks
// executed and writes into the value.
func TestRunTogetherReportsOnlyWhatRan(t *testing.T) {
	var invoked atomic.Int32
	fn := cuexruntime.Concurrent(cuexruntime.GenericProviderFn[stubParams, stubReturns](
		func(_ context.Context, p *stubParams) (*stubReturns, error) {
			invoked.Add(1)
			return &stubReturns{Returns: "ok-" + p.Params}, nil
		}))
	providers := map[string]cuexruntime.Provider{"stub": stubProvider{fn: fn}}

	cc := cuecontext.New()
	calls := make([]pendingCall, 0, 8)
	for i := 0; i < 8; i++ {
		v := cc.CompileString(fmt.Sprintf(`{#do: "get", #provider: "stub", $params: "k-%d"}`, i))
		require.NoError(t, v.Err())
		calls = append(calls, pendingCall{
			path:     cue.ParsePath(fmt.Sprintf("reads.n%d", i)),
			fill:     cue.ParsePath(fmt.Sprintf("reads.n%d", i)),
			key:      fmt.Sprintf("reads.n%d", i),
			value:    v,
			fn:       "get",
			provider: "stub",
		})
	}

	// already past: no call should be reached at all
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	results, err := runTogether(ctx, providers, calls, 4)
	require.ErrorAs(t, err, &ResolveTimeoutErr{})
	require.Zero(t, invoked.Load(), "nothing should have been invoked")
	require.Empty(t, results, "results must name only calls that ran, not every call without an error")
}
