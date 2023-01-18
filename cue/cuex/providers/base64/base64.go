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

package base64

import (
	"context"
	"encoding/base64"

	_ "embed"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
)

// Params .
type Params providers.Params[string]

// Returns .
type Returns providers.Returns[string]

// Encode .
func Encode(ctx context.Context, params *Params) (*Returns, error) {
	return &Returns{
		Returns: base64.StdEncoding.EncodeToString([]byte(params.Params)),
	}, nil
}

// Decode .
func Decode(ctx context.Context, params *Params) (*Returns, error) {
	o, err := base64.StdEncoding.DecodeString(params.Params)
	if err != nil {
		return nil, err
	}
	return &Returns{
		Returns: string(o),
	}, nil
}

// ProviderName .
const ProviderName = "base64"

//go:embed base64.cue
var template string

// Package .
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"encode": cuexruntime.GenericProviderFn[Params, Returns](Encode),
	"decode": cuexruntime.GenericProviderFn[Params, Returns](Decode),
}))
