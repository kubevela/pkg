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
