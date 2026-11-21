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

func runDiff(t *testing.T, in util.TimeDiffInput) (util.TimeDiffOutput, error) {
	t.Helper()
	ret, err := util.TimeDiff(context.Background(), &util.TimeDiffParams{Params: in})
	if err != nil {
		return util.TimeDiffOutput{}, err
	}
	return ret.Returns, nil
}

func TestTimeDiff_Positive(t *testing.T) {
	out, err := runDiff(t, util.TimeDiffInput{
		From: "2026-08-13T06:00:00Z",
		To:   "2026-08-16T06:00:00Z",
	})
	require.NoError(t, err)
	require.Equal(t, "72h0m0s", out.Duration)
	require.Equal(t, int64(72*60*60*1e9), out.Nanoseconds)
}

func TestTimeDiff_Negative(t *testing.T) {
	// to before from must yield a negative interval, not an absolute value.
	out, err := runDiff(t, util.TimeDiffInput{
		From: "2026-08-16T06:00:00Z",
		To:   "2026-08-13T06:00:00Z",
	})
	require.NoError(t, err)
	require.Equal(t, "-72h0m0s", out.Duration)
	require.Equal(t, int64(-72*60*60*1e9), out.Nanoseconds)
}

func TestTimeDiff_Zero(t *testing.T) {
	out, err := runDiff(t, util.TimeDiffInput{
		From: "2026-08-13T06:00:00Z",
		To:   "2026-08-13T06:00:00Z",
	})
	require.NoError(t, err)
	require.Equal(t, "0s", out.Duration)
	require.Equal(t, int64(0), out.Nanoseconds)
}

func TestTimeDiff_ComparesInstantsAcrossOffsets(t *testing.T) {
	// Same moment written in two zones -- the interval is zero even though the
	// strings differ.
	out, err := runDiff(t, util.TimeDiffInput{
		From: "2026-08-13T06:00:00Z",
		To:   "2026-08-13T08:00:00+02:00",
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), out.Nanoseconds)
}

func TestTimeDiff_SubSecond(t *testing.T) {
	out, err := runDiff(t, util.TimeDiffInput{
		From:   "2026-08-13T06:00:00.000000000Z",
		To:     "2026-08-13T06:00:00.000000500Z",
		Layout: "2006-01-02T15:04:05.999999999Z07:00",
	})
	require.NoError(t, err)
	require.Equal(t, int64(500), out.Nanoseconds)
}

func TestTimeDiff_CustomLayout(t *testing.T) {
	out, err := runDiff(t, util.TimeDiffInput{From: "2026-08-13", To: "2026-08-14", Layout: "2006-01-02"})
	require.NoError(t, err)
	require.Equal(t, "24h0m0s", out.Duration)
}

func TestTimeDiff_MalformedFrom(t *testing.T) {
	_, err := runDiff(t, util.TimeDiffInput{From: "nope", To: "2026-08-13T06:00:00Z"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot parse from")
}

func TestTimeDiff_MalformedTo(t *testing.T) {
	_, err := runDiff(t, util.TimeDiffInput{From: "2026-08-13T06:00:00Z", To: "nope"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot parse to")
}

func TestTimeDiff_SaturationErrors(t *testing.T) {
	// Sub saturates silently at the Duration bounds rather than failing, and a
	// saturated value is indistinguishable from a real one. An interval far
	// wider than the ~292 year range must error.
	_, err := runDiff(t, util.TimeDiffInput{
		From: "0001-01-01T00:00:00Z",
		To:   "9999-12-31T23:59:59Z",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds the representable duration range")
}

func TestTimeDiff_Deterministic(t *testing.T) {
	in := util.TimeDiffInput{From: "2026-08-13T06:00:00Z", To: "2026-08-16T06:00:00Z"}
	a, err := runDiff(t, in)
	require.NoError(t, err)
	b, err := runDiff(t, in)
	require.NoError(t, err)
	require.Equal(t, a, b)
}
