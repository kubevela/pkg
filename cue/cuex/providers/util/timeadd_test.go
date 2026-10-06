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

func runAdd(t *testing.T, in util.TimeAddInput) (string, error) {
	t.Helper()
	ret, err := util.TimeAdd(context.Background(), &util.TimeAddParams{Params: in})
	if err != nil {
		return "", err
	}
	return ret.Returns.Value, nil
}

func TestTimeAdd_Positive(t *testing.T) {
	out, err := runAdd(t, util.TimeAddInput{Value: "2026-08-13T06:00:00Z", Duration: "24h"})
	require.NoError(t, err)
	require.Equal(t, "2026-08-14T06:00:00Z", out)
}

func TestTimeAdd_Negative(t *testing.T) {
	// The case a hand-rolled nanosecond division in CUE gets wrong, because
	// integer division truncates toward zero rather than flooring.
	out, err := runAdd(t, util.TimeAddInput{Value: "2026-08-13T06:00:00Z", Duration: "-30m"})
	require.NoError(t, err)
	require.Equal(t, "2026-08-13T05:30:00Z", out)
}

func TestTimeAdd_CompoundDuration(t *testing.T) {
	out, err := runAdd(t, util.TimeAddInput{Value: "2026-08-13T06:00:00Z", Duration: "1h30m15s"})
	require.NoError(t, err)
	require.Equal(t, "2026-08-13T07:30:15Z", out)
}

func TestTimeAdd_ZeroDuration(t *testing.T) {
	out, err := runAdd(t, util.TimeAddInput{Value: "2026-08-13T06:00:00Z", Duration: "0s"})
	require.NoError(t, err)
	require.Equal(t, "2026-08-13T06:00:00Z", out)
}

func TestTimeAdd_PreservesZoneOffset(t *testing.T) {
	// Go's Time.Add keeps the Location, and so must this -- unlike the time
	// provider in kubevela/workflow, which forces UTC and drops the offset.
	out, err := runAdd(t, util.TimeAddInput{Value: "2026-08-13T06:00:00+02:00", Duration: "1h"})
	require.NoError(t, err)
	require.Equal(t, "2026-08-13T07:00:00+02:00", out)
}

func TestTimeAdd_CrossesLeapDay(t *testing.T) {
	// 2028 is a leap year, so 28 Feb + 24h is 29 Feb, not 1 Mar.
	out, err := runAdd(t, util.TimeAddInput{Value: "2028-02-28T12:00:00Z", Duration: "24h"})
	require.NoError(t, err)
	require.Equal(t, "2028-02-29T12:00:00Z", out)
}

func TestTimeAdd_CrossesNonLeapFebruary(t *testing.T) {
	// 2027 is not a leap year, so the same arithmetic lands on 1 Mar.
	out, err := runAdd(t, util.TimeAddInput{Value: "2027-02-28T12:00:00Z", Duration: "24h"})
	require.NoError(t, err)
	require.Equal(t, "2027-03-01T12:00:00Z", out)
}

func TestTimeAdd_SubSecondPrecision(t *testing.T) {
	out, err := runAdd(t, util.TimeAddInput{
		Value:    "2026-08-13T06:00:00.000000001Z",
		Duration: "1ns",
		Layout:   "2006-01-02T15:04:05.999999999Z07:00",
	})
	require.NoError(t, err)
	require.Equal(t, "2026-08-13T06:00:00.000000002Z", out)
}

func TestTimeAdd_DefaultLayoutKeepsFractionalSeconds(t *testing.T) {
	// time.Parse accepts fractional seconds under the RFC3339 layout, but
	// Format with it drops them, which would silently move the instant.
	out, err := runAdd(t, util.TimeAddInput{Value: "2026-08-13T06:00:00.5Z", Duration: "1s"})
	require.NoError(t, err)
	require.Equal(t, "2026-08-13T06:00:01.5Z", out)
}

func TestTimeAdd_CustomLayoutRoundTrips(t *testing.T) {
	// The layout is used to both parse and emit, so the output shape matches.
	out, err := runAdd(t, util.TimeAddInput{Value: "2026-08-13", Duration: "48h", Layout: "2006-01-02"})
	require.NoError(t, err)
	require.Equal(t, "2026-08-15", out)
}

func TestTimeAdd_MalformedValue(t *testing.T) {
	_, err := runAdd(t, util.TimeAddInput{Value: "nope", Duration: "1h"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot parse")
}

func TestTimeAdd_MalformedDuration(t *testing.T) {
	_, err := runAdd(t, util.TimeAddInput{Value: "2026-08-13T06:00:00Z", Duration: "1 hour"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot parse duration")
}

func TestTimeAdd_ExtremeButRepresentable(t *testing.T) {
	// ParseDuration caps at roughly 292 years, and RFC3339 caps the year at
	// 9999, so the overflow guard cannot be reached through this API. What
	// matters is that a near-maximum duration still succeeds rather than being
	// rejected by an over-eager guard.
	out, err := runAdd(t, util.TimeAddInput{Value: "9000-01-01T00:00:00Z", Duration: "2500000h"})
	require.NoError(t, err)
	require.NotEmpty(t, out)
}

func TestTimeAdd_Deterministic(t *testing.T) {
	in := util.TimeAddInput{Value: "2026-08-13T06:00:00Z", Duration: "72h"}
	a, err := runAdd(t, in)
	require.NoError(t, err)
	b, err := runAdd(t, in)
	require.NoError(t, err)
	require.Equal(t, a, b)
}
