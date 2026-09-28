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
