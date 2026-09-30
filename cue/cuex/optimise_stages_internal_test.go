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
	"testing"

	"github.com/stretchr/testify/require"
)

// A chain of stages, every one of them answered.
//
// This is what narrowing was for. Without it a stage reading the stage
// before it was given every answer that stage produced, so the closure was
// too big to carry and the loop was declined; only the first stage was ever
// taken. With it, an iteration is given the one answer it reads.
//
// The reasons are checked by name rather than in total, because a
// template's last loop is the comprehension that reads the answers and
// not one of the loops that made them: a single count says which loops
// were refused and never which.
func TestEveryStageOfAChainIsAnswered(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	imports := c.PackageManager.GetImports()
	policy := OptimisePolicy{Enabled: true, Threshold: 1}

	for _, stages := range []int{2, 3, 4} {
		OptimiseStats.Calls.Store(0)
		out, took := c.prepass(ctx, stagedLoops(4, stages), imports, policy)
		require.True(t, took, "%d stages: nothing was taken", stages)
		require.Equal(t, 4*stages, int(OptimiseStats.Calls.Load()),
			"%d stages: every call of every stage should have been answered", stages)
		require.Equal(t, 4*stages, out.calls)

		// the output comprehension is the only thing left, and it is
		// refused for the right reason: it makes no call
		require.Equal(t, map[string]string{"out": "notACall"}, out.declined,
			"%d stages: the only refusal should be the output loop", stages)
	}
}

// And the whole point of it: an iteration is not given the answers of a
// stage it reads one answer of.
func TestAnIterationIsNotGivenAWholeStage(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	imports := c.PackageManager.GetImports()
	policy := OptimisePolicy{Enabled: true, Threshold: 1}

	// wide enough that carrying a whole stage would be refused outright,
	// so taking it at all is evidence the narrowing happened
	out, took := c.prepass(ctx, stagedLoops(200, 2), imports, policy)
	require.True(t, took)
	require.Equal(t, 400, out.calls,
		"both stages should be answered, which needs the second to be given "+
			"one answer of the first rather than all two hundred")
}
