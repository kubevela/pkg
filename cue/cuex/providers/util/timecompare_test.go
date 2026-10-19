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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex/providers/util"
)

func runCompare(t *testing.T, in util.TimeCompareInput) (int, error) {
	t.Helper()
	ret, err := util.TimeCompare(context.Background(), &util.TimeCompareParams{Params: in})
	if err != nil {
		return 0, err
	}
	return ret.Returns.Result, nil
}

func TestTimeCompare_Before(t *testing.T) {
	out, err := runCompare(t, util.TimeCompareInput{
		A: "2026-08-13T06:00:00Z",
		B: "2026-08-14T06:00:00Z",
	})
	require.NoError(t, err)
	require.Equal(t, -1, out)
}

func TestTimeCompare_After(t *testing.T) {
	out, err := runCompare(t, util.TimeCompareInput{
		A: "2026-08-14T06:00:00Z",
		B: "2026-08-13T06:00:00Z",
	})
	require.NoError(t, err)
	require.Equal(t, 1, out)
}

func TestTimeCompare_IdenticalStrings(t *testing.T) {
	out, err := runCompare(t, util.TimeCompareInput{
		A: "2026-08-13T06:00:00Z",
		B: "2026-08-13T06:00:00Z",
	})
	require.NoError(t, err)
	require.Equal(t, 0, out)
}

func TestTimeCompare_EqualAcrossOffsets(t *testing.T) {
	// The whole reason to call this instead of comparing strings: these are the
	// same instant written two ways, and must compare equal.
	out, err := runCompare(t, util.TimeCompareInput{
		A: "2026-08-13T06:00:00Z",
		B: "2026-08-13T08:00:00+02:00",
	})
	require.NoError(t, err)
	require.Equal(t, 0, out)
}

func TestTimeCompare_OrdersAcrossOffsets(t *testing.T) {
	// Lexical string comparison would call A the later of the two; by instant
	// it is the earlier.
	out, err := runCompare(t, util.TimeCompareInput{
		A: "2026-08-13T09:00:00+02:00", // 07:00Z
		B: "2026-08-13T08:00:00Z",
	})
	require.NoError(t, err)
	require.Equal(t, -1, out)
}

func TestTimeCompare_SubSecond(t *testing.T) {
	out, err := runCompare(t, util.TimeCompareInput{
		A:      "2026-08-13T06:00:00.000000001Z",
		B:      "2026-08-13T06:00:00.000000002Z",
		Layout: "2006-01-02T15:04:05.999999999Z07:00",
	})
	require.NoError(t, err)
	require.Equal(t, -1, out)
}

func TestTimeCompare_CustomLayout(t *testing.T) {
	out, err := runCompare(t, util.TimeCompareInput{A: "2026-08-13", B: "2026-08-14", Layout: "2006-01-02"})
	require.NoError(t, err)
	require.Equal(t, -1, out)
}

func TestTimeCompare_MalformedA(t *testing.T) {
	_, err := runCompare(t, util.TimeCompareInput{A: "nope", B: "2026-08-13T06:00:00Z"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot parse a")
}

func TestTimeCompare_MalformedB(t *testing.T) {
	_, err := runCompare(t, util.TimeCompareInput{A: "2026-08-13T06:00:00Z", B: "nope"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot parse b")
}

func TestTimeCompare_Deterministic(t *testing.T) {
	in := util.TimeCompareInput{A: "2026-08-13T06:00:00Z", B: "2026-08-14T06:00:00Z"}
	a, err := runCompare(t, in)
	require.NoError(t, err)
	b, err := runCompare(t, in)
	require.NoError(t, err)
	require.Equal(t, a, b)
}
