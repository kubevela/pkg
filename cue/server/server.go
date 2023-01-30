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
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

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
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "read request body error: %s", err.Error())
		return
	}
	val, err := in.fn(r.Context(), string(bs))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "compile cue error: %s", err.Error())
		return
	}
	if path := r.URL.Query().Get(paramKeyPath); len(path) > 0 {
		val = val.LookupPath(cue.ParsePath(path))
	}
	switch r.Header.Get(restful.HEADER_Accept) {
	case mimeCue:
		w.Header().Set(restful.HEADER_ContentEncoding, mimeCue)
		s, e := util.ToString(val)
		bs, err = []byte(s), e
	case mimeYaml:
		w.Header().Set(restful.HEADER_ContentEncoding, mimeYaml)
		if bs, err = val.MarshalJSON(); err == nil {
			bs, err = yaml.JSONToYAML(bs)
		}
	default:
		w.Header().Set(restful.HEADER_ContentEncoding, restful.MIME_JSON)
		bs, err = val.MarshalJSON()
	}
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "content encode error: %s", err.Error())
		return
	}
	if _, err = fmt.Fprint(w, string(bs)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		klog.Errorf("unexpected error when writing response: %s", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
	return
}

// Handle rest request
func (in *CompileServer) Handle(request *restful.Request, response *restful.Response) {
	in.ServeHTTP(response, request.Request)
}
