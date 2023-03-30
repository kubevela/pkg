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

package cue_test

import (
	"context"
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex/model/sets"
	"github.com/kubevela/pkg/cue/cuex/providers"
	cueprovider "github.com/kubevela/pkg/cue/cuex/providers/cue"
	"github.com/kubevela/pkg/util/stringtools"
)

func TestStrategyUnify(t *testing.T) {
	paramsTemplate := "{$params: {value: {%s}, patch: {%s}}}"
	testcases := map[string]struct {
		value  string
		patch  string
		expect string
		hasErr bool
	}{
		"test unify with normal patch": {
			value: `containers: [{name: "x1"},{name: "x2"},...]`,
			patch: `containers: [{name: "x1"},{name: "x2",image: "pause:0.1"}]`,
			expect: `
					containers: [{
						name: "x1"
					}, {
						name:  "x2"
						image: "pause:0.1"
					}]
`,
			hasErr: false,
		},
		"test unify with +patchKey tag": {
			value: `containers: [{name: "x1"},{name: "x2"},...]`,
			patch: `
				// +patchKey=name
				containers: [{name: "x2", image: "nginx:latest"}]
`,
			expect: `
					// +patchKey=name
					containers: [{
						name: "x1"
					}, {
						name:  "x2"
						image: "nginx:latest"
					}, ...]
`,
			hasErr: false,
		},
		"test unify with +patchStrategy=retainKeys tag": {
			value: `containers: [{name: "x1"},{name: "x2", image: "redis:latest"}]`,
			patch: `
					// +patchKey=name
					containers: [{
						name: "x2"
						// +patchStrategy=retainKeys
						image: "nginx:latest"
					}]
`,
			expect: `
					// +patchKey=name
					containers: [{
						name: "x1"
					}, {
						name: "x2"
						// +patchStrategy=retainKeys
						image: "nginx:latest"
					}, ...]
`,
			hasErr: false,
		},
		"test unify with conflicting error": {
			value: `containers: [{name: "x1"},{name: "x2"},...]`,
			patch: `containers: [{name: "x2"},{name: "x1"}]`,
			expect: `
					containers: [{
						name: _|_ // $returns.containers.0.name: conflicting values "x2" and "x1"
					}, {
						name: _|_ // $returns.containers.1.name: conflicting values "x1" and "x2"
					}]
`,
			hasErr: true,
		},
	}

	ctx := context.Background()
	cueCtx := cuecontext.New()
	for name, testcase := range testcases {
		t.Run(name, func(t *testing.T) {
			value := cueCtx.CompileString(fmt.Sprintf(paramsTemplate, testcase.value, testcase.patch))
			val, err := cueprovider.StrategyUnify(ctx, value)
			if testcase.hasErr {
				require.Error(t, err)
			}
			ret := val.LookupPath(cue.ParsePath(providers.ReturnsKey))
			retStr, err := sets.ToString(ret)
			require.NoError(t, err)
			assert.Equal(t, stringtools.TrimLeadingIndent(testcase.expect), stringtools.TrimLeadingIndent(retStr))
		})
	}
}
