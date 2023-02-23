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

package externalserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/emicklei/go-restful/v3"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/cue/cuex/externalserver"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

type val struct {
	V string `json:"v"`
}

func (in *val) MarshalJSON() ([]byte, error) {
	if in.V == "err_bar" {
		return nil, fmt.Errorf(in.V)
	}
	return json.Marshal(map[string]string{"v": in.V})
}

func foo(ctx context.Context, input *val) (*val, error) {
	if input.V == "err" {
		return nil, fmt.Errorf(input.V)
	}
	return &val{V: "foo"}, nil
}

func bar(ctx context.Context, input *val) (*val, error) {
	return &val{V: input.V + "_bar"}, nil
}

func TestExternalServer(t *testing.T) {
	server := externalserver.NewServer("/", map[string]externalserver.ServerProviderFn{
		"foo": externalserver.GenericServerProviderFn[val, val](foo),
		"bar": externalserver.GenericServerProviderFn[val, val](bar),
	})
	cmd := server.NewCommand()
	require.NoError(t, cmd.ParseFlags([]string{`--addr=:0`}))
	go func() {
		_ = cmd.Execute()
	}()
	time.Sleep(3 * time.Second)
	svr := httptest.NewTLSServer(server.Container)
	defer svr.Close()
	for name, tt := range map[string]struct {
		Path       string
		Input      string
		Output     string
		StatusCode int
	}{
		"foo": {
			Path:       "/foo",
			Input:      `{"v":"value"}`,
			Output:     `{"v":"foo"}`,
			StatusCode: 200,
		},
		"bar": {
			Path:       "/bar",
			Input:      `{"v":"value"}`,
			Output:     `{"v":"value_bar"}`,
			StatusCode: 200,
		},
		"bad-json": {
			Path:       "/foo",
			Input:      `{bad`,
			StatusCode: 400,
		},
		"bad-ret": {
			Path:       "/bar",
			Input:      `{"v":"err"}`,
			StatusCode: 500,
		},
		"foo-err": {
			Path:       "/foo",
			Input:      `{"v":"err"}`,
			StatusCode: 500,
		},
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := cuexruntime.DefaultClient.Get().Post(svr.URL+tt.Path, restful.MIME_JSON, bytes.NewReader([]byte(tt.Input)))
			require.NoError(t, err)
			require.Equal(t, tt.StatusCode, resp.StatusCode)
			if tt.StatusCode == http.StatusOK {
				bs, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, []byte(tt.Output), bs)
			}
		})
	}
}
