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
	"context"
	"fmt"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"

	"github.com/kubevela/pkg/cue/cuex"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
)

func TestAddFlags(t *testing.T) {
	set := pflag.NewFlagSet("-", 0)
	cuex.AddFlags(set)
}

func TestCompile(t *testing.T) {
	compiler := cuex.NewCompilerWithDefaultInternalPackages()
	ctx := context.Background()
	str := `
		import "vela/base64"
		parameter: input: string
		_enc: base64.#Encode & { $params: parameter.input }
		output: _enc.$returns
	`
	val, err := compiler.CompileStringWithOptions(ctx, str, cuex.WithExtraData("parameter.input", "example"))
	require.NoError(t, err)
	s, err := val.LookupPath(cue.ParsePath("output")).String()
	require.NoError(t, err)
	require.Equal(t, "ZXhhbXBsZQ==", s)

	val, err = compiler.CompileStringWithOptions(ctx, str, cuex.WithExtraData("parameter", &kuberuntime.RawExtension{Raw: []byte(`{"input": "example"}`)}))
	require.NoError(t, err)
	s, err = val.LookupPath(cue.ParsePath("output")).String()
	require.NoError(t, err)
	require.Equal(t, "ZXhhbXBsZQ==", s)

	val, err = compiler.CompileStringWithOptions(ctx, str, cuex.DisableResolveProviderFunctions{})
	require.NoError(t, err)
	_, err = val.LookupPath(cue.ParsePath("output")).String()
	require.Error(t, err)
}

func TestResolve(t *testing.T) {
	compiler := &cuex.Compiler{
		PackageManager: cuexruntime.NewPackageManager(
			cuexruntime.WithInternalPackage{
				Package: runtime.Must(cuexruntime.NewInternalPackage("test", "", map[string]cuexruntime.ProviderFn{
					"err": cuexruntime.GenericProviderFn[int, int](func(ctx context.Context, t *int) (*int, error) {
						return nil, fmt.Errorf("err")
					}),
					"timeout": cuexruntime.GenericProviderFn[int, int](func(ctx context.Context, t *int) (*int, error) {
						time.Sleep(time.Second)
						return t, nil
					}),
				})),
			}),
	}
	ctx := context.Background()
	cctx := cuecontext.New()

	for name, tt := range map[string]struct {
		Input string
		Error error
	}{
		"provider-not-found": {
			Input: `x: {
				#do: "fn"
				#provider: "unknown"
			}`,
			Error: cuex.ProviderNotFoundErr("unknown"),
		},
		"provider-fn-not-found": {
			Input: `x: {
				#do: "unknown"
				#provider: "test"
			}`,
			Error: cuex.ProviderFnNotFoundErr{Provider: "test", Fn: "unknown"},
		},
		"provider-fn-call-error": {
			Input: `x: {
				#do: "err"
				#provider: "test"
			}`,
			Error: cuex.FunctionCallError{Path: "x", Value: `x: {
				#do: "err"
				#provider: "test"
			}`, Err: fmt.Errorf("err")},
		},
		"provider-fn-timeout-error": {
			Input: `x: {
				#do: "timeout"
				#provider: "test"
			}`,
			Error: cuex.ResolveTimeoutErr{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			defer cancel()
			v := cctx.CompileString(tt.Input)
			_, err := compiler.Resolve(_ctx, v)
			require.Error(t, tt.Error, err)
		})
	}
}
