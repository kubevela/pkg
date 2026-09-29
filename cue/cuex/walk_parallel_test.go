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
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

// walkParallel splits the top level fields out and walks each subtree on its
// own goroutine, which is the shape the question asks about.
func walkParallel(root cue.Value, seen *int64) {
	it, err := root.Fields(cue.Optional(true), cue.Hidden(true))
	if err != nil {
		return
	}
	var subtrees []cue.Value
	for it.Next() {
		subtrees = append(subtrees, it.Value())
	}
	var wg sync.WaitGroup
	for _, sub := range subtrees {
		wg.Add(1)
		go func(v cue.Value) {
			defer wg.Done()
			walkSerial(v, seen)
		}(sub)
	}
	wg.Wait()
}

func walkSerial(v cue.Value, seen *int64) {
	atomic.AddInt64(seen, 1)
	it, err := v.Fields(cue.Optional(true), cue.Hidden(true))
	if err != nil {
		return
	}
	for it.Next() {
		walkSerial(it.Value(), seen)
	}
}

// TestParallelWalkRaces asks the question directly: can the subtrees of one
// value be walked at the same time? Run it with -race.
func TestParallelWalkRaces(t *testing.T) {
	src := realWorkload
	for i := 0; i < 40; i++ {
		src += fmt.Sprintf("\nextra%d: {a: \"x\", b: [1, 2, 3], c: {d: {e: %d}}}\n", i, i)
	}
	// The value a resolve walks is the one Unify just handed back, which has
	// evaluated nothing yet, so the walk is what forces it. One context is
	// shared across the rounds, the way a compiler shares one.
	cc := cuecontext.New()
	for round := 0; round < 200; round++ {
		base := cc.CompileString(src)
		patch := cc.CompileString(fmt.Sprintf(`added%d: {x: {y: {z: %d}}}`, round, round))

		v := base.Unify(patch)
		var seen int64
		walkParallel(v, &seen)
		require.Greater(t, seen, int64(0))
	}
}

// TestSameSubtreeConcurrently is the worst case: two goroutines finalising the
// very same vertices at the same time. A walk that split disjoint subtrees
// would still reach shared ones through references, so this is what any
// parallel walk eventually does.
func TestSameSubtreeConcurrently(t *testing.T) {
	src := realWorkload
	for i := 0; i < 20; i++ {
		src += fmt.Sprintf("\nshared%d: {a: \"x\", b: {c: {d: %d}}}\n", i, i)
	}
	cc := cuecontext.New()
	for round := 0; round < 100; round++ {
		base := cc.CompileString(src)
		require.NoError(t, base.Err())
		// no Err() here: asking evaluates the whole value on this goroutine,
		// which would leave the walk nothing to finalise and hide the very
		// thing being tested
		v := base.Unify(cc.CompileString(fmt.Sprintf(`tag: %d`, round)))

		var seen int64
		var wg sync.WaitGroup
		start := make(chan struct{})
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start // all of them onto the same nodes at once
				walkSerial(v, &seen)
			}()
		}
		close(start)
		wg.Wait()
	}
}
