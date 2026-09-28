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
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"
)

// TestBenchmarkSourcesResolve holds the generated templates to being templates.
//
// They were not. Copying a workload and renaming its parameter label left
// every reference to it, which are written parameter.image rather than
// parameter:, pointing at nothing, so the value came out with seventy errors
// and no calls in it at all. The benchmarks reading it reported that chains
// cost nothing and that width cost little, because the resolver was being
// handed nothing to do. A benchmark over a broken input is worse than none:
// it answers, and the answer is wrong.
func TestBenchmarkSourcesResolve(t *testing.T) {
	c := nothingCompiler(t)
	ctx := context.Background()

	for name, tt := range map[string]struct {
		src   string
		wants []string
	}{
		"withCalls": {
			withCalls(2),
			[]string{"call0.$returns", "call1.$returns", "output.spec.replicas"},
		},
		"biggerWorkload": {
			biggerWorkload(3, 2),
			[]string{"call0.$returns", "call1.$returns", "c0.output.spec.replicas", "c2.output.metadata.name"},
		},
		"chainOn": {
			chainOn(2, 4),
			[]string{"link0.$returns", "link3.$returns", "c0.output.spec.replicas"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			v, err := c.CompileString(ctx, tt.src)
			require.NoError(t, err)
			require.NoError(t, v.Validate(), "the generated template has to be a template")
			for _, path := range tt.wants {
				field := v.LookupPath(cue.ParsePath(path))
				require.True(t, field.Exists(), "%s should be there", path)
				require.NoError(t, field.Err(), "%s should have resolved", path)
			}
		})
	}
}

// TestChainsReallyChain: a chain is only a chain if each link waits for the
// one before it. If they all ran at once the benchmark would be measuring
// width and calling it depth.
func TestChainsReallyChain(t *testing.T) {
	c := nothingCompiler(t)
	v, err := c.CompileString(context.Background(), chainOn(1, 5))
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		got, err := v.LookupPath(cue.ParsePath(fmt.Sprintf("link%d.$returns", i))).String()
		require.NoError(t, err)
		require.Equal(t, "start", got, "link%d should carry what link0 was given", i)
	}
}
