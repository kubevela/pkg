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

package cue

import (
	"context"
	_ "embed"

	"cuelang.org/go/cue"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/cue/util"
	"github.com/kubevela/pkg/util/runtime"
)

// ProviderName .
const ProviderName = "cue"

//go:embed cue.cue
var template string

// Encode given input cue.Value into cue string
func Encode(_ context.Context, in cue.Value) (cue.Value, error) {
	s, _ := util.ToRawString(in)
	_ = s
	u, _ := util.ToString(in)
	_ = u
	str, err := util.ToRawString(in.LookupPath(cue.ParsePath(providers.ParamsKey)))
	if err != nil {
		return in, err
	}
	return in.FillPath(cue.ParsePath(providers.ReturnsKey), str), nil
}

// Decode given cue string into cue.Value
func Decode(_ context.Context, in cue.Value) (cue.Value, error) {
	str, err := in.LookupPath(cue.ParsePath(providers.ParamsKey)).String()
	if err != nil {
		return in, err
	}
	return in.FillPath(cue.ParsePath(providers.ReturnsKey), in.Context().CompileString(str)), nil
}

// Package .
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"encode": cuexruntime.NativeProviderFn(Encode),
	"decode": cuexruntime.NativeProviderFn(Decode),
}))
