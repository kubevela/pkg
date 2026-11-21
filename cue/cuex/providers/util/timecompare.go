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

	"github.com/kubevela/pkg/cue/cuex/providers"
)

// TimeCompareInput are the input parameters for #TimeCompare.
type TimeCompareInput struct {
	// A is the left-hand timestamp.
	A string `json:"a"`
	// B is the right-hand timestamp.
	B string `json:"b"`
	// Layout is the Go reference layout used to parse both. Defaults to RFC3339.
	Layout string `json:"layout"`
}

// TimeCompareOutput is the output of #TimeCompare.
type TimeCompareOutput struct {
	// Result is -1 if A is before B, 0 if they are the same instant, and +1 if
	// A is after B.
	Result int `json:"result"`
}

// TimeCompareParams is the provider input wrapper ($params).
type TimeCompareParams = providers.Params[TimeCompareInput]

// TimeCompareReturns is the provider output wrapper ($returns).
type TimeCompareReturns = providers.Returns[TimeCompareOutput]

// TimeCompare orders two timestamps, returning -1, 0 or +1 as Go's
// Time.Compare does.
//
// Comparison is by instant, not by text: "2026-01-01T00:00:00Z" and
// "2026-01-01T02:00:00+02:00" are the same moment and compare equal. That is
// the reason to call this instead of comparing the strings, which would report
// them as different.
func TimeCompare(_ context.Context, params *TimeCompareParams) (*TimeCompareReturns, error) {
	p := params.Params

	a, layout, err := parseTimestamp("a", p.A, p.Layout)
	if err != nil {
		return nil, err
	}
	b, _, err := parseTimestamp("b", p.B, layout)
	if err != nil {
		return nil, err
	}

	return &TimeCompareReturns{Returns: TimeCompareOutput{Result: a.Compare(b)}}, nil
}
