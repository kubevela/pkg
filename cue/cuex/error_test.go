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

package cuex_test

import (
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
)

func TestErrors(t *testing.T) {
	require.Equal(t, "provider a not found", cuex.ProviderNotFoundErr("a").Error())
	require.Equal(t, "function a not found in provider x", cuex.ProviderFnNotFoundErr{Provider: "x", Fn: "a"}.Error())

	v := cuecontext.New().CompileString(`a: b: "c"`).LookupPath(cue.ParsePath("a.b"))
	e := cuex.NewFunctionCallError(v, fmt.Errorf("err"))
	require.Equal(t, `function call error for a.b: err (value: "c")`, e.Error())

	require.Equal(t, "cuex compile resolve timeout", cuex.ResolveTimeoutErr{}.Error())
}

func TestFunctionCallErrorMessage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      cuex.FunctionCallError
		expected string
	}{
		{
			name:     "path and value present",
			err:      cuex.FunctionCallError{Path: "a.b", Value: `"c"`, Err: fmt.Errorf("err")},
			expected: `function call error for a.b: err (value: "c")`,
		},
		{
			name:     "empty path drops the for clause",
			err:      cuex.FunctionCallError{Value: `"c"`, Err: fmt.Errorf("err")},
			expected: `function call error: err (value: "c")`,
		},
		{
			name:     "empty value drops the value clause",
			err:      cuex.FunctionCallError{Path: "a.b", Err: fmt.Errorf("err")},
			expected: "function call error for a.b: err",
		},
		{
			name:     "empty path and value",
			err:      cuex.FunctionCallError{Err: fmt.Errorf("err")},
			expected: "function call error: err",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.err.Error())
		})
	}
}

func TestNewFunctionCallErrorOmitsUnrenderableValue(t *testing.T) {
	// A value that cannot be rendered must be left out of the message rather
	// than replaced by the formatting failure, which reads as though the
	// failure came from the value itself.
	e := cuex.NewFunctionCallError(cue.Value{}, fmt.Errorf("boom"))
	require.Empty(t, e.Value)
	require.NotContains(t, e.Error(), "cue/format")
	require.Equal(t, "function call error: boom", e.Error())
}
