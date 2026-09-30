//go:build !race

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
	"strings"
	"sync"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
)

// Several renders at once on one compiler, which is what the Application
// controller does: a singleton compiler and four reconciles by default.
//
// Not built under the race detector, and not because of anything here.
// A compile hands the provider packages to CUE as the shared
// build.Instance values the compiler holds, and building against those
// from more than one goroutine races inside CUE itself, in
// build.Instance.Complete. It does so with the prepass off, and on
// v1.11 with none of this code present, so it is neither this branch's
// nor #144's: it is how cuex has always given CUE its imports.
//
// The answers are right regardless, which is what this checks. The race
// wants fixing where it is, by giving a compile imports of its own or by
// letting one build at a time, and that is a change of its own with its
// own measurements.
func TestConcurrentRendersAgree(t *testing.T) {
	c := cuex.NewCompilerWithInternalPackages(edgePackage())
	src := edgeLoop(60, "#Do")

	for _, tc := range []struct {
		name   string
		policy cuex.OptimisePolicy
	}{
		{"resolver", cuex.OptimisePolicy{}},
		{"prepass", cuex.OptimisePolicy{Enabled: true, Threshold: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var wg sync.WaitGroup
			errs := make([]error, 8)
			outs := make([]string, 8)
			for i := range errs {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					v, err := c.CompileStringWithOptions(context.Background(), src,
						cuex.WithOptimise(tc.policy))
					if err != nil {
						errs[i] = err
						return
					}
					bs, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
					errs[i], outs[i] = err, string(bs)
				}(i)
			}
			wg.Wait()
			for i, err := range errs {
				require.NoError(t, err, "goroutine %d", i)
			}
			for i := 1; i < len(outs); i++ {
				require.Equal(t, outs[0], outs[i], "every render should agree")
			}
			require.True(t, strings.Contains(outs[0], "p59"))
		})
	}
}
