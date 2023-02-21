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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cueprovider "github.com/kubevela/pkg/cue/cuex/providers/cue"
	"github.com/kubevela/pkg/cue/util"
	"github.com/kubevela/pkg/util/stringtools"
)

func TestEncode(t *testing.T) {
	ctx := context.Background()
	cctx := cuecontext.New()
	val := cctx.CompileString(`
		import "strconv"
		{
			$params: {
				// test comment
				key: *"key" | string
				val: int & >= 0
				loop: *[1, 2, 3] | [...int]
				if val > 1 {
					loop: [2, 4, 6]
				}
				r: [for i in loop {strconv.FormatInt(i)}]
			}
		}`)
	val, err := cueprovider.Encode(ctx, val)
	require.NoError(t, err)
	str, err := val.LookupPath(cue.ParsePath(providers.ReturnsKey)).String()
	require.NoError(t, err)
	require.Equal(t, stringtools.TrimLeadingIndent(`
		import "strconv"

		// test comment
		key:  *"key" | string
		val:  int & >=0
		loop: *[1, 2, 3] | [...int]
		if val > 1 {
			loop: [2, 4, 6]
		}
		r: [ for i in loop {
			strconv.FormatInt(i)
		}]`), strings.TrimSpace(str))
}

func TestDecode(t *testing.T) {
	ctx := context.Background()
	cctx := cuecontext.New()
	val := cctx.CompileString(`
		{
			$params: #"""
				import "strconv"				

				key: *"key" | string
				val: int & >= 0
				loop: *[1, 2, 3] | [...int]
				if val > 1 {
					loop: [2, 4, 6]
				}
				r: [for i in loop {strconv.FormatInt(i)}]
			"""#
		}`)
	val, err := cueprovider.Decode(ctx, val)
	require.NoError(t, err)
	ret := val.LookupPath(cue.ParsePath(providers.ReturnsKey))
	str, err := util.ToRawString(ret)
	require.NoError(t, err)
	require.Equal(t, stringtools.TrimLeadingIndent(`
		import "strconv"

		key:  *"key" | string
		val:  int & >=0
		loop: *[1, 2, 3] | [...int]
		if val > 1 {
			loop: [2, 4, 6]
		}
		r: [ for i in loop {
			strconv.FormatInt(i)
		}]`), strings.TrimSpace(str))
}

type X struct {
	A string                `json:"a"`
	B *runtime.RawExtension `json:"b"`
}

func TestT(t *testing.T) {
	x := &X{}
	err := json.Unmarshal([]byte(`{"a":"x","b":{"ex":"y"}}`), x)
	require.NoError(t, err)
	fmt.Println(string(x.B.Raw))
	fmt.Println(x.B.Object)
}
