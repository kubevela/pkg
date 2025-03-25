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

package runtime_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/apis/cue/v1alpha1"
	"github.com/kubevela/pkg/cue/cuex/providers"
	"github.com/kubevela/pkg/cue/cuex/runtime"
)

type value struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

func TestGenericProviderFn(t *testing.T) {
	fn := func(ctx context.Context, v *value) (*value, error) {
		if v.Input == "?" {
			return nil, fmt.Errorf("invalid input")
		}
		v.Output = strings.ToUpper(v.Input)
		return v, nil
	}
	prd := runtime.GenericProviderFn[value, value](fn)

	// test normal
	v := cuecontext.New().CompileString(`{
		input: "value"
		output?: string
	}`)
	out, err := prd.Call(context.Background(), v)
	require.NoError(t, err)
	_v := &value{}
	require.NoError(t, out.Decode(_v))
	require.Equal(t, _v.Output, "VALUE")

	// test invalid input
	badInput := cuecontext.New().CompileString(`what?`)
	_, err = prd.Call(context.Background(), badInput)
	require.Error(t, err)

	// test invalid value input
	badValueInput := cuecontext.New().CompileString(`{input: 5}`)
	_, err = prd.Call(context.Background(), badValueInput)
	require.Error(t, err)

	// test invalid output
	badOutput := cuecontext.New().CompileString(`{
		input: "?"
		output?: string
	}`)
	_, err = prd.Call(context.Background(), badOutput)
	require.Error(t, err)
}

func newTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		bs, _ := io.ReadAll(request.Body)
		v := &value{}
		_ = json.Unmarshal(bs, v)
		switch v.Input {
		case "?":
			_, _ = writer.Write([]byte(`?`))
			return
		case "-":
			writer.WriteHeader(500)
			return
		}
		v.Output = strings.ToUpper(v.Input)
		bs, _ = json.Marshal(v)
		_, _ = writer.Write(bs)
	}))
}

func TestExternalProviderFn(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	// test normal
	prd := runtime.ExternalProviderFn{
		Provider: v1alpha1.Provider{
			Protocol: v1alpha1.ProtocolHTTP,
			Endpoint: server.URL,
		},
	}
	v := cuecontext.New().CompileString(`{
		$params: input: "value"
		$returns?: {
			output?: string
			...
		}
	}`)
	out, err := prd.Call(context.Background(), v)
	require.NoError(t, err)
	_v := &value{}
	require.NoError(t, out.LookupPath(cue.ParsePath(providers.ReturnsKey)).Decode(_v))
	require.Equal(t, "VALUE", _v.Output)

	// test invalid input
	badInput := cuecontext.New().CompileString(`what?`)
	_, err = prd.Call(context.Background(), badInput)
	require.Error(t, err)

	// test invalid output
	badOutput := cuecontext.New().CompileString(`{
		$params: input: "?"
		$returns?: {
			output?: string
			...
		}
	}`)
	_, err = prd.Call(context.Background(), badOutput)
	require.Error(t, err)

	// test bad response
	badResp := cuecontext.New().CompileString(`{
		$params: input: "-"
		$returns?: {
			output?: string
			...
		}
	}`)
	_, err = prd.Call(context.Background(), badResp)
	require.Error(t, err)

	// test invalid protocol
	prd = runtime.ExternalProviderFn{
		Provider: v1alpha1.Provider{
			Protocol: "-",
			Endpoint: server.URL,
		},
	}
	_, err = prd.Call(context.Background(), v)
	require.Error(t, fmt.Errorf("protocol - not supported yet"), err)

	// test bad endpoint
	prd = runtime.ExternalProviderFn{
		Provider: v1alpha1.Provider{
			Protocol: v1alpha1.ProtocolHTTP,
			Endpoint: "?",
		},
	}
	_, err = prd.Call(context.Background(), v)
	require.Error(t, err)
}

func TestNativeProviderFn(t *testing.T) {
	fn := func(_ context.Context, in cue.Value) (cue.Value, error) {
		params := in.LookupPath(cue.ParsePath(providers.ParamsKey))
		return in.FillPath(cue.ParsePath(providers.ReturnsKey), params), nil
	}

	ctx := context.Background()
	val := cuecontext.New().CompileString(`{
		$params: "s"
		$returns?: string
	}`)
	ret, _ := runtime.NativeProviderFn(fn).Call(ctx, val)
	s, err := ret.LookupPath(cue.ParsePath(providers.ReturnsKey)).String()
	require.NoError(t, err)
	require.Equal(t, "s", s)
}

func TestProviderCustomHeader(t *testing.T) {
	headerVal := "123"
	headers := map[string]string{
		"x-api-key": headerVal,
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		apiKeyFromHeader := request.Header.Get("x-api-key")
		require.Equal(t, headerVal, apiKeyFromHeader)
		writer.WriteHeader(200)
		writer.Write([]byte("{}"))
	}))
	defer server.Close()

	prd := runtime.ExternalProviderFn{
		Provider: v1alpha1.Provider{
			Protocol: v1alpha1.ProtocolHTTP,
			Endpoint: server.URL,
			Header:   headers,
		},
	}
	v := cuecontext.New().CompileString(`{
		$params: input: "value"
		$returns?: {
			output?: string
			...
		}
	}`)
	_, err := prd.Call(context.Background(), v)
	require.NoError(t, err, "call to ExternalProviderFn failed")
}
