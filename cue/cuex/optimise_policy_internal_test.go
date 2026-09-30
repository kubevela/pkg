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

package cuex

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptimisePolicy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy OptimisePolicy
		n      int
		want   bool
	}{
		{"off takes nothing", OptimisePolicy{}, 1000, false},
		{"a big loop is taken", DefaultOptimisePolicy, 1000, true},
		{"at the threshold exactly", DefaultOptimisePolicy, defaultThreshold, true},
		{"one below it", DefaultOptimisePolicy, defaultThreshold - 1, false},
		{"a threshold of one takes everything", OptimisePolicy{Enabled: true, Threshold: 1}, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.policy.allows(tc.n))
		})
	}
}
