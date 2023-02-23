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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"cuelang.org/go/cue"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kubevela/pkg/apis/cue/v1alpha1"
	"github.com/kubevela/pkg/cue/cuex/providers"
	"github.com/kubevela/pkg/util/singleton"
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
type ExternalProviderFn struct {
	v1alpha1.Provider
	Fn string
}

// DefaultClientInsecureSkipVerify set if the default external provider client
// use insecure-skip-verify
var DefaultClientInsecureSkipVerify = true

// DefaultClient client for dealing requests
var DefaultClient = singleton.NewSingleton(func() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: DefaultClientInsecureSkipVerify},
		},
	}
})

// FunctionHeaderKey http header for recording cuex provider function
const FunctionHeaderKey = "CueX-External-Provider-Function"

// Call dial external endpoints by passing the json data of the input parameter,
// then fill back returned values
func (in *ExternalProviderFn) Call(ctx context.Context, value cue.Value) (cue.Value, error) {
	params := value.LookupPath(cue.ParsePath(providers.ParamsKey))
	bs, err := params.MarshalJSON()
	if err != nil {
		return value, err
	}
	switch in.Protocol {
	case v1alpha1.ProtocolHTTP, v1alpha1.ProtocolHTTPS:
		req, err := http.NewRequest(http.MethodPost, in.Endpoint, bytes.NewReader(bs))
		if err != nil {
			return value, err
		}
		req.Header.Set("Content-Type", runtime.ContentTypeJSON)
		req.Header.Set(FunctionHeaderKey, in.Fn)
		resp, err := DefaultClient.Get().Do(req.WithContext(ctx))
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
	return value.FillPath(cue.ParsePath(providers.ReturnsKey), ret), nil
}

var _ ProviderFn = NativeProviderFn(nil)

// NativeProviderFn native function that implements ProviderFn interface
type NativeProviderFn func(context.Context, cue.Value) (cue.Value, error)

// Call .
func (fn NativeProviderFn) Call(ctx context.Context, value cue.Value) (cue.Value, error) {
	return fn(ctx, value)
}
