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
	"time"

	"github.com/kubevela/pkg/cue/cuex/providers"
)

// TimeAddInput are the input parameters for #TimeAdd.
type TimeAddInput struct {
	// Value is the timestamp to offset.
	Value string `json:"value"`
	// Duration is a Go duration string such as "720h", "-30m" or "1h30m".
	Duration string `json:"duration"`
	// Layout is the Go reference layout used to both parse and emit. Defaults
	// to RFC3339, emitted as RFC3339Nano so fractional seconds are kept.
	Layout string `json:"layout"`
}

// TimeAddOutput is the output of #TimeAdd.
type TimeAddOutput struct {
	// Value is the offset timestamp, in the same layout as the input.
	Value string `json:"value"`
}

// TimeAddParams is the provider input wrapper ($params).
type TimeAddParams = providers.Params[TimeAddInput]

// TimeAddReturns is the provider output wrapper ($returns).
type TimeAddReturns = providers.Returns[TimeAddOutput]

// TimeAdd offsets a timestamp by a duration, which may be negative.
//
// The zone offset of the input is preserved: a "+02:00" timestamp comes back as
// "+02:00", matching Go's Time.Add, which keeps the Location. Note that this
// differs from the time provider in kubevela/workflow, which forces .UTC() and
// so discards the caller's offset.
func TimeAdd(_ context.Context, params *TimeAddParams) (*TimeAddReturns, error) {
	p := params.Params

	t, layout, err := parseTimestamp("", p.Value, p.Layout)
	if err != nil {
		return nil, err
	}

	d, err := time.ParseDuration(p.Duration)
	if err != nil {
		return nil, fmt.Errorf("cannot parse duration %q: %w", p.Duration, err)
	}

	sum := t.Add(d)

	// Time.Add wraps silently on overflow. A result that moved the wrong way
	// relative to the sign of the duration is the tell, and is the only way to
	// catch it without reaching into the unexported representation.
	if (d > 0 && sum.Before(t)) || (d < 0 && sum.After(t)) {
		return nil, fmt.Errorf("adding %q to %q overflows the representable time range", p.Duration, p.Value)
	}

	// Parsing under the default RFC3339 layout accepts fractional seconds, but
	// formatting with it drops them and would shift the resulting instant.
	// RFC3339Nano emits the same shape as RFC3339 for whole seconds, since it
	// trims trailing zeros. An explicit layout is the caller's choice and is
	// honoured as given.
	if p.Layout == "" {
		layout = time.RFC3339Nano
	}

	return &TimeAddReturns{Returns: TimeAddOutput{Value: sum.Format(layout)}}, nil
}
