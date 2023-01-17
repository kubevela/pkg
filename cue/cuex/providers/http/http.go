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

package http

import (
	"context"
	_ "embed"
	"io"
	"net/http"
	"strings"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
)

// DoParams params for do http request
// TODO: support timeout & tls
type DoParams struct {
	Method  string `json:"method"`
	URL     string `json:"url"`
	Request struct {
		Body string `json:"body"`
	} `json:"request"`
	Header  http.Header `json:"header"`
	Trailer http.Header `json:"trailer"`
}

// DoReturns returned struct for http response
type DoReturns struct {
	Response struct {
		Body       string      `json:"body"`
		Header     http.Header `json:"header"`
		Trailer    http.Header `json:"trailer"`
		StatusCode int         `json:"statusCode"`
	} `json:"response"`
}

// Do execute http request and process returned result
func Do(ctx context.Context, params *DoParams) (*DoReturns, error) {
	req, err := http.NewRequestWithContext(ctx, params.Method, params.URL, strings.NewReader(params.Request.Body))
	if err != nil {
		return nil, err
	}
	req.Header = params.Header
	req.Trailer = params.Trailer

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// parse response body and headers
	ret := &DoReturns{}
	ret.Response.Body = string(b)
	ret.Response.Header = resp.Header
	ret.Response.Trailer = resp.Trailer
	ret.Response.StatusCode = resp.StatusCode
	return ret, nil
}

// ProviderName .
const ProviderName = "http"

//go:embed http.cue
var template string

// Package .
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"do": cuexruntime.GenericProviderFn[DoParams, DoReturns](Do),
}))
