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
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
)

// The prepass against every template the resolver is tested on, with the
// policy wide open: every loop, whatever its size, annotated or not.
//
// This is the run that says whether the prepass can ever be let at a loop
// that did not ask for it. Each case has to do one of two things: render
// to exactly what the resolver renders, or decline and leave the template
// alone. A case that renders differently is the failure the whole design
// is arranged to avoid, and one is enough to say no.

func renderOf(t *testing.T, c *cuex.Compiler, src string) string {
	t.Helper()
	v, err := c.CompileString(context.Background(), src)
	if err != nil {
		return "ERR: " + firstLineOf(err.Error())
	}
	bs, err := v.MarshalJSON()
	if err != nil {
		return "JSON-ERR: " + firstLineOf(err.Error())
	}
	return string(bs)
}

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func TestPrepassAgainstTheCorpus(t *testing.T) {
	c := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()

	names := make([]string, 0, len(resolveCorpus))
	for name := range resolveCorpus {
		names = append(names, name)
	}
	sort.Strings(names)

	var taken, declined, differed int
	for _, name := range names {
		src := resolveCorpus[name].Template
		before := renderOf(t, c, src)

		rewritten, calls, took := c.PrepassWideOpenForTest(ctx, src)
		if !took {
			declined++
			continue
		}
		after := renderOf(t, c, rewritten)
		if before != after {
			differed++
			t.Errorf("CORPUS %s renders differently after the prepass\n  before: %s\n  after:  %s",
				name, before, after)
			continue
		}
		taken++
		t.Logf("CORPUS %-46s %3d call(s) answered, same render", name, calls)
	}

	t.Logf("CORPUS --- %d taken, %d declined, %d differed, of %d",
		taken, declined, differed, len(names))
	require.Zero(t, differed,
		"a template that renders differently is the failure this cannot have")
	// Every case here is declined today: the comprehensions in the corpus
	// make no provider call, so there is nothing for a prepass to answer.
	// Said out loud, because a corpus that proves nothing looks exactly
	// like one that passes, and if a case ever starts being taken this is
	// where to notice it.
	require.Zero(t, taken,
		"no corpus case makes a call in a loop, so none should be taken; "+
			"if one now is, this test has started proving something and should say so")
	require.Equal(t, len(names), declined,
		"so every one of them should be declined")
}
