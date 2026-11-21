/*
Copyright 2026 The KubeVela Authors.

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

package util

import (
	_ "embed"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
)

// ProviderName is the cuex provider name; templates import it as "vela/util".
const ProviderName = "util"

//go:embed util.cue
var template string

// Package is the registered internal cuex package for util.
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"truncate":    cuexruntime.GenericProviderFn[TruncateParams, TruncateReturns](Truncate),
	"timeparse":   cuexruntime.GenericProviderFn[TimeParseParams, TimeParseReturns](TimeParse),
	"timeadd":     cuexruntime.GenericProviderFn[TimeAddParams, TimeAddReturns](TimeAdd),
	"timediff":    cuexruntime.GenericProviderFn[TimeDiffParams, TimeDiffReturns](TimeDiff),
	"timecompare": cuexruntime.GenericProviderFn[TimeCompareParams, TimeCompareReturns](TimeCompare),
}))
