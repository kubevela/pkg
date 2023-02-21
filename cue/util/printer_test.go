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

package util_test

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/util"
	"github.com/kubevela/pkg/util/stringtools"
)

func TestPrint(t *testing.T) {
	testcases := map[string]struct {
		Path   string
		Format string
		Err    bool
		Out    string
	}{
		"json": {
			Format: "json",
			Out:    `{"x":1,"z":{"s":"str"},"y":2}`,
		},
		"yaml": {
			Format: "yaml",
			Out: `
				x: 1
				"y": 2
				z:
				  s: str
			`,
		},
		"cue": {
			Format: "cue",
			Out: `
				x: 1
				z: s: "str"
				y: 2
			`,
		},
		"path": {
			Format: "json",
			Path:   "z.s",
			Out:    `"str"`,
		},
	}
	ctx := cuecontext.New()
	val := ctx.CompileString(`
		x: *1 | int
		y: x + 1
		if y > 1 {
			z: s: "str"
		}
	`)
	for name, tt := range testcases {
		t.Run(name, func(t *testing.T) {
			bs, err := util.Print(val, util.WithFormat(tt.Format), util.WithPath(tt.Path))
			if tt.Err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, stringtools.TrimLeadingIndent(tt.Out), stringtools.TrimLeadingIndent(string(bs)))
		})
	}
}
