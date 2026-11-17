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

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
)

// RequestVars is the vars for http request. Headers and trailers take one
// string per name, as http.cue declares them.
// TODO: support timeout & tls
type RequestVars struct {
	Method  string `json:"method"`
	URL     string `json:"url"`
	Request struct {
		Body    string            `json:"body"`
		Header  map[string]string `json:"header"`
		Trailer map[string]string `json:"trailer"`
	} `json:"request"`
}

func toHeader(values map[string]string) http.Header {
	if values == nil {
		return nil
	}
	header := make(http.Header, len(values))
	for name, value := range values {
		header.Add(name, value)
	}
	return header
}

// ResponseVars is the vars for http response
type ResponseVars struct {
	Body       string      `json:"body"`
	Header     http.Header `json:"header"`
	Trailer    http.Header `json:"trailer"`
	StatusCode int         `json:"statusCode"`
}

// DoParams is the params for http request
type DoParams providers.Params[RequestVars]

// DoReturns returned struct for http response
type DoReturns providers.Returns[ResponseVars]

// NewRequest builds the HTTP request a vela/http call describes, so a provider
// that sends it through its own client builds it the same way.
func NewRequest(ctx context.Context, vars RequestVars) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, vars.Method, vars.URL, strings.NewReader(vars.Request.Body))
	if err != nil {
		return nil, err
	}
	req.Header = toHeader(vars.Request.Header)
	req.Trailer = toHeader(vars.Request.Trailer)
	return req, nil
}

// Do execute http request and process returned result
func Do(ctx context.Context, doParams *DoParams) (*DoReturns, error) {
	req, err := NewRequest(ctx, doParams.Params)
	if err != nil {
		return nil, err
	}

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
	return &DoReturns{
		Returns: ResponseVars{
			Body:       string(b),
			Header:     resp.Header,
			Trailer:    resp.Trailer,
			StatusCode: resp.StatusCode,
		},
	}, nil
}

// ProviderName .
const ProviderName = "http"

//go:embed http.cue
var template string

// Package .
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"do": cuexruntime.GenericProviderFn[DoParams, DoReturns](Do),
}))
