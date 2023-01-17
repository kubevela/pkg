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

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
)

type Var struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

func Encode(ctx context.Context, v *Var) (*Var, error) {
	v.Output = base64.StdEncoding.EncodeToString([]byte(v.Input))
	return v, nil
}

func Decode(ctx context.Context, v *Var) (*Var, error) {
	o, err := base64.StdEncoding.DecodeString(v.Input)
	if err == nil {
		v.Output = string(o)
	}
	return v, err
}

const ProviderName = "base64"

//go:embed base64.cue
var template string

var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"encode": cuexruntime.GenericProviderFn[Var, Var](Encode),
	"decode": cuexruntime.GenericProviderFn[Var, Var](Decode),
}))
