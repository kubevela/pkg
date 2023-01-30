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

package server_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/emicklei/go-restful/v3"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/server"

	"github.com/kubevela/pkg/cue/cuex"
	cueserver "github.com/kubevela/pkg/cue/server"
)

func TestRegisterGenericAPIServer(t *testing.T) {
	s := &server.GenericAPIServer{Handler: &server.APIServerHandler{
		GoRestfulContainer: restful.NewContainer(),
	}}
	cueserver.RegisterGenericAPIServer(s)
}

type FakeResponseWriter struct {
	bytes.Buffer
	StatusCode int
	Bad        bool
}

func (in *FakeResponseWriter) Header() http.Header {
	return map[string][]string{}
}

func (in *FakeResponseWriter) Write(i []byte) (int, error) {
	if in.Bad {
		return 0, io.ErrUnexpectedEOF
	}
	return in.Buffer.Write(i)
}

func (in *FakeResponseWriter) WriteHeader(statusCode int) {
	in.StatusCode = statusCode
}

var _ http.ResponseWriter = &FakeResponseWriter{}

type BadReader struct{}

func (in *BadReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

var _ io.Reader = &BadReader{}

func TestHandleRequest(t *testing.T) {
	cases := map[string]struct {
		Body       []byte
		Path       string
		Format     string
		BadWriter  bool
		UseCuex    bool
		StatusCode int
		Output     []byte
	}{
		"bad-request": {
			Body:       nil,
			StatusCode: http.StatusBadRequest,
		},
		"compile-error": {
			Body:       []byte(`bad-key: bad value`),
			StatusCode: http.StatusBadRequest,
		},
		"write-error": {
			Body:       []byte(`x: y: z: 5`),
			BadWriter:  true,
			StatusCode: http.StatusInternalServerError,
		},
		"good": {
			Body:       []byte(`x: y: z: 5`),
			Path:       "x.y",
			StatusCode: http.StatusOK,
			Output:     []byte(`{"z":5}`),
		},
		"cuex": {
			Body: []byte(`
				import "vela/base64"
				x: y: base64.#Encode & { $params: "example" }
			`),
			Path:       "x.y.$returns",
			UseCuex:    true,
			StatusCode: http.StatusOK,
			Output:     []byte(`"ZXhhbXBsZQ=="`),
		},
		"yaml-format": {
			Body:       []byte(`x: y: z: 5`),
			Path:       "x",
			Format:     "application/yaml",
			StatusCode: http.StatusOK,
			Output:     []byte("\"y\":\n  z: 5\n"),
		},
		"cue-format": {
			Body:       []byte(`x: y: z: 5`),
			Path:       "x.y",
			Format:     "application/cue",
			StatusCode: http.StatusOK,
			Output:     []byte("{\n\tz: 5\n}"),
		},
	}
	cueServer := cueserver.NewCompileServer(func(ctx context.Context, s string) (cue.Value, error) {
		return cuecontext.New().CompileString(s), nil
	})
	cuexServer := cueserver.NewCompileServer(cuex.NewCompilerWithDefaultInternalPackages().CompileString)
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			var body io.Reader = bytes.NewReader(tt.Body)
			if tt.Body == nil {
				body = &BadReader{}
			}
			raw, err := http.NewRequest("", "?path="+tt.Path, body)
			require.NoError(t, err)
			request := restful.NewRequest(raw)
			request.Request.Header.Set(restful.HEADER_Accept, tt.Format)
			writer := &FakeResponseWriter{Bad: tt.BadWriter}
			response := restful.NewResponse(writer)
			if tt.UseCuex {
				cuexServer.Handle(request, response)
			} else {
				cueServer.Handle(request, response)
			}
			require.Equal(t, tt.StatusCode, writer.StatusCode)
			if tt.StatusCode == http.StatusOK {
				require.Equal(t, tt.Output, writer.Bytes())
			}
		})
	}
}
