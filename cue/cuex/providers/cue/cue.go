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

	"github.com/kubevela/pkg/cue/cuex/model/sets"
	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
)

// ProviderName .
const ProviderName = "cue"

//go:embed cue.cue
var template string

// StrategyUnify unifies values by using a strategic patching approach.
func StrategyUnify(_ context.Context, in cue.Value) (cue.Value, error) {
	params := in.LookupPath(cue.ParsePath(providers.ParamsKey))
	base := params.LookupPath(cue.ParsePath("value"))
	patcher := params.LookupPath(cue.ParsePath("patch"))
	res, err := sets.StrategyUnify(base, patcher)
	return in.FillPath(cue.ParsePath(providers.ReturnsKey), res), err
}

// Package .
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"strategyUnify": cuexruntime.NativeProviderFn(StrategyUnify),
}))
