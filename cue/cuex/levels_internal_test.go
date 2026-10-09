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
	"strings"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"
)

// A result only has to be in the value if something still to run reads it.
// Holding the rest back and unifying them together would cut the number of
// unifies, but only where there is more than one level to spread them over.
// This counts the levels and times what each costs, so the size of that
// opportunity is known before anything is built for it.

func chainSrc(width, depth int) string {
	var b strings.Builder
	b.WriteString("import \"vela/ph\"\n")
	b.WriteString(phaseWorkload)
	// width calls that nothing reads, so nothing waits on them
	for i := 0; i < width; i++ {
		fmt.Fprintf(&b, "\nloose%d: ph.#Do & {$params: \"w-%d\"}\n", i, i)
	}
	// and a chain, where every result but the last is read by the next
	if depth > 0 {
		b.WriteString("\nlink0: ph.#Do & {$params: \"start\"}\n")
		for i := 1; i < depth; i++ {
			fmt.Fprintf(&b, "link%d: ph.#Do & {$params: link%d.$returns}\n", i, i-1)
		}
	}
	return b.String()
}

// TestLevelCost reports how many levels a shape takes and what the applying
// costs, separated from everything else in the round.
func TestLevelCost(t *testing.T) {
	c := phaseCompiler(t)
	ctx := context.Background()
	const runs = 100

	t.Log("width depth  levels     levelTime   perLevel  wholeCompile")
	for _, shape := range []struct{ width, depth int }{
		{1, 0}, {8, 0}, {0, 2}, {0, 4}, {0, 8}, {0, 16}, {8, 8},
	} {
		src := chainSrc(shape.width, shape.depth)
		built, err := c.CompileStringWithOptions(ctx, src, DisableResolveProviderFunctions{})
		require.NoError(t, err)

		var levels int
		var apply time.Duration
		for i := 0; i < runs; i++ {
			levels, apply = levelsTimed(t, c, ctx, built)
		}
		per := time.Duration(0)
		if levels > 0 {
			per = apply / time.Duration(levels)
		}
		// the same source through the ordinary path, to check the opened up
		// loop above is measuring the same work and not something else
		whole := time.Now()
		for i := 0; i < runs; i++ {
			_, err := c.CompileString(ctx, src)
			require.NoError(t, err)
		}
		full := time.Since(whole) / runs
		t.Logf("%5d %5d %7d %12s %10s %10s", shape.width, shape.depth, levels,
			apply.Round(time.Microsecond), per.Round(time.Microsecond),
			full.Round(time.Microsecond))
	}
}

// levelsTimed runs a resolve, counting the levels and timing only the applying,
// which is the part a batch across levels would change.
func levelsTimed(t *testing.T, in *Compiler, ctx context.Context, value cue.Value) (int, time.Duration) {
	t.Helper()
	newValue := value
	executed := map[string]bool{}
	waitingFor := &callWaits{of: map[string][]string{}}
	providers := in.PackageManager.GetProviders()
	levels := 0
	var apply time.Duration

	pending := pendingCalls(newValue, executed, nil)
	remaining := pending
	for reread := false; len(remaining) > 0; reread = true {
		start := time.Now()
		next, rest, _, _, err := in.runLevel(ctx, newValue, providers, pending, remaining, executed, waitingFor, reread, nil)
		require.NoError(t, err)
		apply += time.Since(start)
		newValue, remaining = next, rest
		levels++
	}
	return levels, apply
}
