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

package util_test

import (
	"context"
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex/providers/util"
)

// evalCUE compiles src through a compiler carrying only the util package, then
// reads a string field off the result.
//
// The Go-level tests exercise the functions directly and so cannot catch a
// mismatch between the field names declared in util.cue and the json tags on
// the Go structs. That mismatch compiles, passes every unit test, and breaks
// every template -- so it needs a test that goes through CUE.
func evalCUE(t *testing.T, src, field string) (string, error) {
	t.Helper()
	compiler := cuex.NewCompilerWithInternalPackages(util.Package)
	v, err := compiler.CompileString(context.Background(), src)
	if err != nil {
		return "", err
	}
	return v.LookupPath(cue.ParsePath(field)).String()
}

func TestCUE_TimeAddThroughTemplate(t *testing.T) {
	got, err := evalCUE(t, `
import "vela/util"

out: util.#TimeAdd & {
  $params: {
    value:    "2026-08-13T06:00:00Z"
    duration: "24h"
  }
}
result: out.$returns.value
`, "result")
	require.NoError(t, err)
	require.Equal(t, "2026-08-14T06:00:00Z", got)
}

func TestCUE_TimeParseThroughTemplate(t *testing.T) {
	got, err := evalCUE(t, `
import "vela/util"

out: util.#TimeParse & {
  $params: value: "2026-08-13T06:00:00Z"
}
result: out.$returns.rfc3339
`, "result")
	require.NoError(t, err)
	require.Equal(t, "2026-08-13T06:00:00Z", got)
}

func TestCUE_TimeDiffThroughTemplate(t *testing.T) {
	got, err := evalCUE(t, `
import "vela/util"

out: util.#TimeDiff & {
  $params: {
    from: "2026-08-13T06:00:00Z"
    to:   "2026-08-16T06:00:00Z"
  }
}
result: out.$returns.duration
`, "result")
	require.NoError(t, err)
	require.Equal(t, "72h0m0s", got)
}

// evalCUEInt is the integer counterpart of evalCUE. Without it no test reads a
// numeric field back through CUE, and unixNano (~1.79e18) exceeds float64's
// exact-integer range -- a float64 hop at the JSON boundary would corrupt it
// silently while unix (~1.79e9) survived and told you nothing.
func evalCUEInt(t *testing.T, src, field string) (int64, error) {
	t.Helper()
	compiler := cuex.NewCompilerWithInternalPackages(util.Package)
	v, err := compiler.CompileString(context.Background(), src)
	if err != nil {
		return 0, err
	}
	return v.LookupPath(cue.ParsePath(field)).Int64()
}

func TestCUE_TimeParseIntegerFields(t *testing.T) {
	const src = `
import "vela/util"

out: util.#TimeParse & {
  $params: value: "2026-08-13T06:00:00Z"
}
unix:     out.$returns.unix
unixNano: out.$returns.unixNano
`
	unix, err := evalCUEInt(t, src, "unix")
	require.NoError(t, err)
	require.Equal(t, int64(1786600800), unix)

	unixNano, err := evalCUEInt(t, src, "unixNano")
	require.NoError(t, err)
	require.Equal(t, int64(1786600800)*int64(1e9), unixNano)
}

func TestCUE_TimeDiffNanoseconds(t *testing.T) {
	got, err := evalCUEInt(t, `
import "vela/util"

out: util.#TimeDiff & {
  $params: {
    from: "2026-08-13T06:00:00Z"
    to:   "2026-08-13T06:00:01Z"
  }
}
result: out.$returns.nanoseconds
`, "result")
	require.NoError(t, err)
	require.Equal(t, int64(1e9), got)
}

func TestCUE_TimeCompareThroughTemplate(t *testing.T) {
	tpl := `
import "vela/util"

out: util.#TimeCompare & {
  $params: {
    a: "%s"
    b: "%s"
  }
}
result: out.$returns.result
`
	for _, tc := range []struct {
		name string
		a, b string
		want int64
	}{
		{"before", "2026-08-13T06:00:00Z", "2026-08-14T06:00:00Z", -1},
		{"same", "2026-08-13T06:00:00Z", "2026-08-13T06:00:00Z", 0},
		{"after", "2026-08-14T06:00:00Z", "2026-08-13T06:00:00Z", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evalCUEInt(t, fmt.Sprintf(tpl, tc.a, tc.b), "result")
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
