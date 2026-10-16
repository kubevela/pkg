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

package util

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/kubevela/pkg/cue/cuex/providers"
)

// defaultLayout is used whenever a caller leaves Layout empty. RFC3339 is the
// form Kubernetes stamps into object metadata, so it is what templates almost
// always receive.
const defaultLayout = time.RFC3339

// parseTimestamp parses value with layout, defaulting to RFC3339 when layout is
// empty, and returns the layout it used so callers can format with it. field
// names the parameter in the error ("from", "a", ...) and may be empty.
func parseTimestamp(field, value, layout string) (time.Time, string, error) {
	if layout == "" {
		layout = defaultLayout
	}
	t, err := time.Parse(layout, value)
	if err != nil {
		if field != "" {
			field += " "
		}
		return time.Time{}, layout, fmt.Errorf("cannot parse %s%q with layout %q: %w", field, value, layout, err)
	}
	return t, layout, nil
}

// TimeParseInput are the input parameters for #TimeParse.
type TimeParseInput struct {
	// Value is the timestamp to parse.
	Value string `json:"value"`
	// Layout is the Go reference layout to parse Value with. Defaults to RFC3339.
	Layout string `json:"layout"`
}

// TimeParseOutput is the output of #TimeParse.
type TimeParseOutput struct {
	// Unix is whole seconds since the Unix epoch.
	Unix int64 `json:"unix"`
	// UnixNano is nanoseconds since the Unix epoch.
	UnixNano int64 `json:"unixNano"`
	// RFC3339 is the parsed instant in RFC3339Nano layout. The layout is
	// normalized; the zone offset is not -- a "+02:00" input comes back as
	// "+02:00", not as UTC.
	RFC3339 string `json:"rfc3339"`
}

// TimeParseParams is the provider input wrapper ($params).
type TimeParseParams = providers.Params[TimeParseInput]

// TimeParseReturns is the provider output wrapper ($returns).
type TimeParseReturns = providers.Returns[TimeParseOutput]

// TimeParse converts a timestamp string into epoch numbers.
//
// This is the primitive the rest of the time helpers are built on. CUE's
// builtin time package can turn an epoch into a string (time.Unix) but has no
// inverse, so without this a template holds timestamps it can never do
// arithmetic on or compare.
//
// All three representations are returned from the one call so that callers
// never need a second round-trip just to change form.
func TimeParse(_ context.Context, params *TimeParseParams) (*TimeParseReturns, error) {
	p := params.Params

	layout := p.Layout
	if layout == "" {
		layout = defaultLayout
	}

	t, err := time.Parse(layout, p.Value)
	if err != nil {
		return nil, fmt.Errorf("cannot parse %q with layout %q: %w", p.Value, layout, err)
	}

	// UnixNano is explicitly undefined outside this range -- Go returns a
	// wrapped, meaningless int64 rather than failing. Reject up-front so a
	// template can never consume a silently wrong number.
	if t.Before(time.Unix(0, math.MinInt64)) || t.After(time.Unix(0, math.MaxInt64)) {
		return nil, fmt.Errorf("%q is outside the range representable as Unix nanoseconds (roughly 1678-2262)", p.Value)
	}

	return &TimeParseReturns{Returns: TimeParseOutput{
		Unix:     t.Unix(),
		UnixNano: t.UnixNano(),
		RFC3339:  t.Format(time.RFC3339Nano),
	}}, nil
}
