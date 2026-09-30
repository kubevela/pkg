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
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
)

// The hazard that decides whether a prepass can be applied to every loop or
// only to one that asked for it.
//
// A prepass evaluates an iteration in a document of its own, built from the
// fields that iteration reads. In CUE a name can be written more than once
// and the whole file unifies, so a field is not one declaration but all of
// them. Carrying one declaration and not the rest gives a name that is
// concrete, plausible, and different from what the template meant.
//
// That is the failure mode with no error attached to it, so it is worth
// knowing exactly how it behaves before deciding who is exposed to it.

func TestANameCanBeDeclaredMoreThanOnce(t *testing.T) {
	// what the template means: cfg is both halves unified
	src := `
cfg: {mode: "fast"}
cfg: {retries: 3}
params: {mode: cfg.mode, retries: cfg.retries}
`
	cc := cuecontext.New()
	whole := cc.CompileString(src)
	require.NoError(t, whole.Err())
	mode, err := whole.LookupPath(cue.ParsePath("params.mode")).String()
	require.NoError(t, err)
	retries, err := whole.LookupPath(cue.ParsePath("params.retries")).Int64()
	require.NoError(t, err)
	t.Logf("HAZARD whole file: mode=%q retries=%d", mode, retries)
	require.Equal(t, "fast", mode)
	require.EqualValues(t, 3, retries)

	// and what one declaration gives: retries is gone, so reading it errors
	one := cc.CompileString(`
cfg: {mode: "fast"}
params: {mode: cfg.mode, retries: cfg.retries}
`)
	_, err = one.LookupPath(cue.ParsePath("params.retries")).Int64()
	t.Logf("HAZARD one declaration, reading retries: %v", err)
	require.Error(t, err,
		"a missing declaration shows itself when the field it held is read")

	// the quiet case: the params take cfg whole, so nothing is missing and
	// nothing errors. The provider is simply called with less than it
	// should have been.
	full := cc.CompileString(`
cfg: {mode: "fast"}
cfg: {retries: 3}
params: cfg
`)
	partial := cc.CompileString(`
cfg: {mode: "fast"}
params: cfg
`)
	fullJSON, err := full.LookupPath(cue.ParsePath("params")).MarshalJSON()
	require.NoError(t, err)
	partialJSON, err := partial.LookupPath(cue.ParsePath("params")).MarshalJSON()
	require.NoError(t, err, "no error, which is the problem")
	t.Logf("HAZARD params from the whole file:   %s", fullJSON)
	t.Logf("HAZARD params from one declaration:  %s", partialJSON)
	require.NotEqual(t, string(fullJSON), string(partialJSON),
		"if these ever match, this hazard has gone away")
}
