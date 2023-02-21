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

package server

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"cuelang.org/go/cue"
	"github.com/emicklei/go-restful/v3"

	"github.com/kubevela/pkg/cue/util"
)

// CompileFn function for compile
type CompileFn func(context.Context, string) (cue.Value, error)

const (
	paramKeyPath = "path"
	mimeYaml     = "application/yaml"
	mimeCue      = "application/cue"
)

// CompileServer server for compile cue value
type CompileServer struct {
	fn CompileFn
}

// NewCompileServer create CompileServer
func NewCompileServer(fn CompileFn) *CompileServer {
	return &CompileServer{fn: fn}
}

// ServeHTTP .
func (in *CompileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bs, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("read request body error: %s", err.Error()), http.StatusBadRequest)
		return
	}
	val, err := in.fn(r.Context(), string(bs))
	if err != nil {
		http.Error(w, fmt.Sprintf("compile cue error: %s", err.Error()), http.StatusBadRequest)
		return
	}
	var options []util.PrintOption
	if path := r.URL.Query().Get(paramKeyPath); len(path) > 0 {
		options = append(options, util.WithPath(path))
	}
	switch r.Header.Get(restful.HEADER_Accept) {
	case mimeCue:
		w.Header().Set(restful.HEADER_ContentEncoding, mimeCue)
		options = append(options, util.WithFormat(util.PrintFormatCue))
	case mimeYaml:
		w.Header().Set(restful.HEADER_ContentEncoding, mimeYaml)
		options = append(options, util.WithFormat(util.PrintFormatYaml))
	default:
		w.Header().Set(restful.HEADER_ContentEncoding, restful.MIME_JSON)
		options = append(options, util.WithFormat(util.PrintFormatJson))
	}
	bs, err = util.Print(val, options...)
	if err != nil {
		http.Error(w, fmt.Sprintf("content encode error: %s", err.Error()), http.StatusBadRequest)
		return
	}
	if _, err = w.Write(bs); err != nil {
		http.Error(w, fmt.Sprintf("unexpected error when writing response: %s", err.Error()), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	return
}

// Handle rest request
func (in *CompileServer) Handle(request *restful.Request, response *restful.Response) {
	in.ServeHTTP(response, request.Request)
}
