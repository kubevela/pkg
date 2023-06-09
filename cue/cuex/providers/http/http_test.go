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
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

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
