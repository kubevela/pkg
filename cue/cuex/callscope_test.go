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
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"
)

// A field can be a call without naming one: it refers to a field that is.
func TestACallReachedThroughAnotherField(t *testing.T) {
	c := nothingCompiler(t)
	v, err := c.CompileString(context.Background(), `
import "vela/nothing"
first: nothing.#Do & {$params: "a"}
second: first
`)
	require.NoError(t, err)

	got, err := v.LookupPath(cue.ParsePath("second.$returns")).String()
	require.NoError(t, err, "second is the same call as first and has the same answer")
	require.Equal(t, "a", got)
}

// A definition is not run where it is declared, so a field that copies one is
// the only call there is, and the syntax of that field names no provider.
func TestACallCopiedFromALocalDefinition(t *testing.T) {
	c := nothingCompiler(t)
	v, err := c.CompileString(context.Background(), `
import "vela/nothing"
#tpl: nothing.#Do & {$params: "a"}
second: #tpl
`)
	require.NoError(t, err)

	got, err := v.LookupPath(cue.ParsePath("second.$returns")).String()
	require.NoError(t, err, "second is a call, however little its own syntax says so")
	require.Equal(t, "a", got)
}
