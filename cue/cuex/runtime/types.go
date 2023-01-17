/*
Copyright 2022 The KubeVela Authors.

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

package runtime

import (
	"context"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
)

// ProviderFn the function interface to process cue values
type ProviderFn interface {
	Call(context.Context, cue.Value) (cue.Value, error)
}

// Provider the interface to get provider function
type Provider interface {
	GetName() string
	GetProviderFn(do string) ProviderFn
}

// CUETemplater the interface for retrieving cue templates and imports
type CUETemplater interface {
	GetName() string
	GetPath() string
	GetTemplates() []string
	GetImports() []*build.Instance
}

// Package composed by Provider & CUETemplater, the atomic unit for engine to use
type Package interface {
	Provider
	CUETemplater
}
