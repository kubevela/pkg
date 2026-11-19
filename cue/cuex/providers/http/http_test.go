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

package http_test

import (
	"context"
	"fmt"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/pkg/cue/cuex/providers/http"
)

func TestDo(t *testing.T) {
	svr := httptest.NewServer(nethttp.HandlerFunc(func(writer nethttp.ResponseWriter, request *nethttp.Request) {
		writer.WriteHeader(200)
	}))
	defer svr.Close()
	ret, err := http.Do(context.Background(), &http.DoParams{
		Params: http.RequestVars{
			Method: "GET", URL: svr.URL,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 200, ret.Returns.StatusCode)

	_, err = http.Do(context.Background(), &http.DoParams{
		Params: http.RequestVars{
			Method: "GET", URL: "https://localhost:9999/",
		},
	})
	require.Error(t, err)
}

func TestDoSendsRequestHeaders(t *testing.T) {
	svr := httptest.NewServer(nethttp.HandlerFunc(func(writer nethttp.ResponseWriter, request *nethttp.Request) {
		_, _ = writer.Write([]byte(request.Header.Get("X-Token") + "," + request.Header.Get("Content-Type")))
	}))
	defer svr.Close()
	src := fmt.Sprintf(`
import "vela/http"

r: http.#Do & {$params: {
	method: "POST"
	url:    %q
	request: header: {"X-Token": "abc", "Content-Type": "application/json"}
}}
`, svr.URL)
	v, err := cuex.NewCompilerWithInternalPackages(http.Package).CompileString(context.Background(), src)
	require.NoError(t, err)
	body, err := v.LookupPath(cue.ParsePath("r.$returns.body")).String()
	require.NoError(t, err)
	require.Equal(t, "abc,application/json", body)
}

func TestDoSendsCaseVariantHeaders(t *testing.T) {
	svr := httptest.NewServer(nethttp.HandlerFunc(func(writer nethttp.ResponseWriter, request *nethttp.Request) {
		values := request.Header.Values("X-Tag")
		sort.Strings(values)
		_, _ = writer.Write([]byte(strings.Join(values, ",")))
	}))
	defer svr.Close()
	src := fmt.Sprintf(`
import "vela/http"

r: http.#Do & {$params: {
	method: "GET"
	url:    %q
	request: header: {"X-Tag": "a", "x-tag": "b"}
}}
`, svr.URL)
	v, err := cuex.NewCompilerWithInternalPackages(http.Package).CompileString(context.Background(), src)
	require.NoError(t, err)
	body, err := v.LookupPath(cue.ParsePath("r.$returns.body")).String()
	require.NoError(t, err)
	require.Equal(t, "a,b", body)
}

func TestNewRequest(t *testing.T) {
	vars := http.RequestVars{Method: "PUT", URL: "http://example.com/x"}
	vars.Request.Body = "payload"
	vars.Request.Header = map[string]string{"X-Token": "abc"}
	vars.Request.Trailer = map[string]string{"X-Sum": "1"}
	req, err := http.NewRequest(context.Background(), vars)
	require.NoError(t, err)
	require.Equal(t, "PUT", req.Method)
	require.Equal(t, "http://example.com/x", req.URL.String())
	require.Equal(t, "abc", req.Header.Get("X-Token"))
	require.Equal(t, "1", req.Trailer.Get("X-Sum"))
	b, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, "payload", string(b))

	_, err = http.NewRequest(context.Background(), http.RequestVars{Method: "GET", URL: "://bad"})
	require.Error(t, err)
}

func TestNewRequestWithoutHeadersKeepsHeaderUsable(t *testing.T) {
	req, err := http.NewRequest(context.Background(), http.RequestVars{Method: "GET", URL: "http://example.com/"})
	require.NoError(t, err)
	require.NotNil(t, req.Header)
	req.Header.Set("X-Added", "later")
	require.Equal(t, "later", req.Header.Get("X-Added"))
}
