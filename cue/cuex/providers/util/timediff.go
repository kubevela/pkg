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

	"github.com/kubevela/pkg/cue/cuex/providers"
)

// TimeDiffInput are the input parameters for #TimeDiff.
type TimeDiffInput struct {
	// From is the earlier timestamp, subtracted from To.
	From string `json:"from"`
	// To is the later timestamp.
	To string `json:"to"`
	// Layout is the Go reference layout used to parse both. Defaults to RFC3339.
	Layout string `json:"layout"`
}

// TimeDiffOutput is the output of #TimeDiff.
type TimeDiffOutput struct {
	// Duration is the Go duration string, e.g. "72h0m0s". Negative when To
	// precedes From.
	Duration string `json:"duration"`
	// Nanoseconds is the same interval as a number.
	Nanoseconds int64 `json:"nanoseconds"`
}

// TimeDiffParams is the provider input wrapper ($params).
type TimeDiffParams = providers.Params[TimeDiffInput]

// TimeDiffReturns is the provider output wrapper ($returns).
type TimeDiffReturns = providers.Returns[TimeDiffOutput]

// TimeDiff returns To minus From. The result is negative when To precedes From.
func TimeDiff(_ context.Context, params *TimeDiffParams) (*TimeDiffReturns, error) {
	p := params.Params

	from, layout, err := parseTimestamp("from", p.From, p.Layout)
	if err != nil {
		return nil, err
	}
	to, _, err := parseTimestamp("to", p.To, layout)
	if err != nil {
		return nil, err
	}

	d := to.Sub(from)

	// Sub saturates rather than failing: an interval wider than about 292 years
	// silently comes back as the maximum or minimum Duration. Saturation is
	// indistinguishable from a genuine extreme value, so reject both -- a
	// wrong-but-plausible interval is worse than an error.
	if d == math.MaxInt64 || d == math.MinInt64 {
		return nil, fmt.Errorf("interval between %q and %q exceeds the representable duration range of roughly 292 years", p.From, p.To)
	}

	return &TimeDiffReturns{Returns: TimeDiffOutput{
		Duration:    d.String(),
		Nanoseconds: int64(d),
	}}, nil
}
