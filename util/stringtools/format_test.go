/*
Copyright 2023 The KubeVela Authors.

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

package stringtools_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/util/stringtools"
)

func TestTrimLeadingIndent(t *testing.T) {
	for name, tt := range map[string]struct {
		Input  string
		Output string
	}{
		"normal": {
			Input: `
	x: 1
	y:
		z: 2`,
			Output: "x: 1\ny:\n\tz: 2",
		},
		"empty": {
			Input:  "\n\n\t\n",
			Output: "",
		},
		"multiline": {
			Input:  "\n\n\texample\n\n\ttest\n\n",
			Output: "example\n\ntest",
		},
		"null": {
			Input:  "",
			Output: "",
		},
		"no indent": {
			Input:  "x:1\n\ty:2\n",
			Output: "x:1\n\ty:2",
		},
	} {
		t.Run(name, func(t *testing.T) {
			out := stringtools.TrimLeadingIndent(tt.Input)
			require.Equal(t, tt.Output, out)
		})
	}
}
