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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"cuelang.org/go/cue"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kubevela/pkg/apis/cue/v1alpha1"
)

var _ ProviderFn = GenericProviderFn[any, any](nil)

// GenericProviderFn generic function that implements ProviderFn interface
type GenericProviderFn[T any, U any] func(context.Context, *T) (*U, error)

// Call marshal value into json and decode into underlying function input
// parameters, then fill back the returned output value
func (fn GenericProviderFn[T, U]) Call(ctx context.Context, value cue.Value) (cue.Value, error) {
	params := new(T)
	bs, err := value.MarshalJSON()
	if err != nil {
		return value, err
	}
	if err = json.Unmarshal(bs, params); err != nil {
		return value, err
	}
	ret, err := fn(ctx, params)
	if err != nil {
		return value, err
	}
	return value.FillPath(cue.ParsePath(""), ret), nil
}

var _ ProviderFn = (*ExternalProviderFn)(nil)

// ExternalProviderFn external provider that implements ProviderFn interface
type ExternalProviderFn v1alpha1.Provider

// Call dial external endpoints by passing the json data of the input parameter,
// then fill back returned values
func (in *ExternalProviderFn) Call(ctx context.Context, value cue.Value) (cue.Value, error) {
	bs, err := value.MarshalJSON()
	if err != nil {
		return value, err
	}
	switch in.Protocol {
	case v1alpha1.ProtocolHTTP:
		req, err := http.NewRequest(http.MethodPost, in.Endpoint, bytes.NewReader(bs))
		if err != nil {
			return value, err
		}
		req.Header.Set("Content-Type", runtime.ContentTypeJSON)
		resp, err := http.DefaultClient.Do(req.WithContext(ctx))
		if err != nil {
			return value, err
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if bs, err = io.ReadAll(resp.Body); err != nil {
			return value, err
		}
	default:
		return value, fmt.Errorf("protocol %s not supported yet", in.Protocol)
	}
	ret := &map[string]any{}
	if err = json.Unmarshal(bs, ret); err != nil {
		return value, err
	}
	return value.FillPath(cue.ParsePath(""), ret), nil
}
