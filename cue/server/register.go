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

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/emicklei/go-restful/v3"
	"k8s.io/apiserver/pkg/server"

	"github.com/kubevela/pkg/cue/cuex"
)

const (
	cuePath     = "/cue"
	cuexPath    = "/cuex"
	compilePath = "/compile"
)

// RegisterGenericAPIServer register cue & cuex compile path to apiserver
func RegisterGenericAPIServer(server *server.GenericAPIServer) *server.GenericAPIServer {
	server = RegisterCueServerToGenericAPIServer(server)
	server = RegisterCuexServerToGenericAPIServer(server)
	return server
}

// RegisterCueServerToGenericAPIServer register cue compile path to apiserver
func RegisterCueServerToGenericAPIServer(server *server.GenericAPIServer) *server.GenericAPIServer {
	ws := &restful.WebService{}
	ws.Path(cuePath)
	ws.Route(ws.POST(compilePath).To(
		NewCompileServer(func(ctx context.Context, s string) (cue.Value, error) {
			return cuecontext.New().CompileString(s), nil
		}).Handle))
	server.Handler.GoRestfulContainer.Add(ws)
	return server
}

// RegisterCuexServerToGenericAPIServer register cuex compile path to apiserver
func RegisterCuexServerToGenericAPIServer(server *server.GenericAPIServer) *server.GenericAPIServer {
	ws := &restful.WebService{}
	ws.Path(cuexPath)
	ws.Route(ws.POST(compilePath).To(NewCompileServer(cuex.CompileString).Handle))
	server.Handler.GoRestfulContainer.Add(ws)
	return server
}
