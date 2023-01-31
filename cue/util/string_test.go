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

func TestToString(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`
		// +usage=x
		x: y
		y: 5
		_z: 1`)
	s, err := util.ToString(v)
	require.NoError(t, err)
	require.Equal(t, stringtools.TrimLeadingIndent(`
		// +usage=x
		x:  5
		y:  5
		_z: 1
	`), s)
}

func TestToRawString(t *testing.T) {
	ctx := cuecontext.New()
	s := `
		import "strconv"

		// +usage=x
		param: {
			// test comment
			key:  *"key" | string
			val:  int & >=0
			loop: *[1, 2, 3] | [...int]
			if val > 1 {
				loop: [2, 4, 6]
			}
			r: [ for i in loop {
				strconv.FormatInt(i)
			}]
		}`
	v := ctx.CompileString(s)
	out, err := util.ToRawString(v)
	require.NoError(t, err)
	require.Equal(t, stringtools.TrimLeadingIndent(s), out)
}
