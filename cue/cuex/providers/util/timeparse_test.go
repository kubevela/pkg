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

func runParse(t *testing.T, in util.TimeParseInput) (util.TimeParseOutput, error) {
	t.Helper()
	ret, err := util.TimeParse(context.Background(), &util.TimeParseParams{Params: in})
	if err != nil {
		return util.TimeParseOutput{}, err
	}
	return ret.Returns, nil
}

func TestTimeParse_RFC3339Default(t *testing.T) {
	// Empty layout must fall back to RFC3339.
	out, err := runParse(t, util.TimeParseInput{Value: "2026-08-13T06:00:00Z"})
	require.NoError(t, err)
	require.Equal(t, int64(1786600800), out.Unix)
	require.Equal(t, int64(1786600800000000000), out.UnixNano)
	require.Equal(t, "2026-08-13T06:00:00Z", out.RFC3339)
}

func TestTimeParse_CustomLayout(t *testing.T) {
	out, err := runParse(t, util.TimeParseInput{Value: "2026-08-13", Layout: "2006-01-02"})
	require.NoError(t, err)
	require.Equal(t, "2026-08-13T00:00:00Z", out.RFC3339)
}

func TestTimeParse_OffsetIsAnInstantNotText(t *testing.T) {
	// The same moment written in two zones must yield the same epoch.
	utc, err := runParse(t, util.TimeParseInput{Value: "2026-08-13T06:00:00Z"})
	require.NoError(t, err)
	plus2, err := runParse(t, util.TimeParseInput{Value: "2026-08-13T08:00:00+02:00"})
	require.NoError(t, err)
	require.Equal(t, utc.Unix, plus2.Unix)
	require.Equal(t, utc.UnixNano, plus2.UnixNano)
}

func TestTimeParse_SubSecondPrecisionPreserved(t *testing.T) {
	out, err := runParse(t, util.TimeParseInput{Value: "2026-08-13T06:00:00.123456789Z"})
	require.NoError(t, err)
	require.Equal(t, int64(1786600800123456789), out.UnixNano)
	// Unix() truncates toward the second; nanoseconds survive only in UnixNano.
	require.Equal(t, int64(1786600800), out.Unix)
	require.Equal(t, "2026-08-13T06:00:00.123456789Z", out.RFC3339)
}

func TestTimeParse_Malformed(t *testing.T) {
	_, err := runParse(t, util.TimeParseInput{Value: "not-a-timestamp"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot parse")
}

func TestTimeParse_WrongLayoutForValue(t *testing.T) {
	_, err := runParse(t, util.TimeParseInput{Value: "2026-08-13T06:00:00Z", Layout: "2006-01-02"})
	require.Error(t, err)
}

func TestTimeParse_OutsideNanosecondRangeErrors(t *testing.T) {
	// Go's UnixNano is explicitly undefined before 1678 / after 2262 and returns
	// a wrapped int64 rather than failing. Reject instead of handing a template
	// a silently wrong number.
	for _, v := range []string{"1500-01-01T00:00:00Z", "2500-01-01T00:00:00Z"} {
		_, err := runParse(t, util.TimeParseInput{Value: v})
		require.Error(t, err, "value=%q", v)
		require.Contains(t, err.Error(), "outside the range", "value=%q", v)
	}
}

func TestTimeParse_JustInsideNanosecondRange(t *testing.T) {
	// The guard must not reject values that are genuinely representable.
	for _, v := range []string{"1700-01-01T00:00:00Z", "2200-01-01T00:00:00Z"} {
		_, err := runParse(t, util.TimeParseInput{Value: v})
		require.NoError(t, err, "value=%q", v)
	}
}

func TestPackage_RegistersTimeFunctions(t *testing.T) {
	// A function that compiles and passes its unit tests is still unreachable
	// from a template unless it is registered in the util package.
	for _, name := range []string{"timeparse", "timeadd", "timediff", "timecompare"} {
		require.NotNil(t, util.Package.GetProviderFn(name), "provider fn %q not registered", name)
	}
}

func TestTimeParse_Deterministic(t *testing.T) {
	in := util.TimeParseInput{Value: "2026-08-13T06:00:00Z"}
	a, err := runParse(t, in)
	require.NoError(t, err)
	b, err := runParse(t, in)
	require.NoError(t, err)
	require.Equal(t, a, b)
}
