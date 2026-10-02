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

	"cuelang.org/go/cue/format"
	"github.com/stretchr/testify/require"
)

// What an answered call is left as.
//
// A resolved call node costs 8408B at 4000 calls, of which 5375B is the
// #do and #provider that named it: definitions, carrying the closedness
// machinery definitions carry, read by nothing once the answer is in.
// $params is 673B and is kept, being an ordinary field and so part of
// what a visible node renders to.
//
// The prepass never puts those definitions back, so a loop it answers
// pays none of that. This pins it, because it is easy to reintroduce by
// writing the whole node back out and nothing else would notice.
func TestAnAnsweredCallKeepsOnlyWhatIsRead(t *testing.T) {
	c := NewCompilerWithDefaultInternalPackages()
	out, took := c.prepass(context.Background(), loopSrc(4),
		c.PackageManager.GetImports(),
		OptimisePolicy{Enabled: true, Threshold: 1})
	require.True(t, took)

	bs, err := format.Node(out.file)
	require.NoError(t, err)
	t.Logf("what the prepass hands on:\n%s", bs)

	require.NotContains(t, string(bs), doKey,
		"the #do that named the call is of no use once it is answered")
	require.NotContains(t, string(bs), providerKey,
		"nor the #provider")
	// Not just that the text says $returns anywhere: the loop that reads
	// the answers keeps its own _calls[...].$returns and satisfies that on
	// its own. What has to be there is the answer, written in.
	require.Contains(t, string(bs), `"c2VlZC0w"`,
		"base64 of seed-0 has to be in what is handed on, as a literal")
	require.NotContains(t, string(bs), "base64.#Encode",
		"and the call it came from should be gone, replaced by it")
	require.Contains(t, string(bs), paramsKey,
		"and the parameters render, so dropping them would change output")
}
