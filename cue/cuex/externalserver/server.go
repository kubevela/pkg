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

package externalserver

import (
	"context"
	"encoding/json"
	"github.com/kubevela/pkg/cue/cuex/runtime"
	"io"
	"net/http"

	"github.com/emicklei/go-restful/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/kubevela/pkg/util/cert"
)

// ServerProviderFn the function interface to process rest call
type ServerProviderFn interface {
	Call(request *restful.Request, response *restful.Response)
}

// GenericServerProviderFn generic function that implements ServerProviderFn interface
type GenericServerProviderFn[T any, U any] func(context.Context, *T) (*U, error)

// Call handle rest call for given request
func (fn GenericServerProviderFn[T, U]) Call(request *restful.Request, response *restful.Response) {
	ctx := runtime.ContextFromHeaders(request.Request)
	request.Request = request.Request.WithContext(ctx)
	bs, err := io.ReadAll(request.Request.Body)
	if err != nil {
		_ = response.WriteError(http.StatusBadRequest, err)
		return
	}
	params := new(T)
	if err = json.Unmarshal(bs, params); err != nil {
		_ = response.WriteError(http.StatusBadRequest, err)
		return
	}
	ret, err := fn(request.Request.Context(), params)
	if err != nil {
		_ = response.WriteError(http.StatusInternalServerError, err)
		return
	}
	if bs, err = json.Marshal(ret); err != nil {
		_ = response.WriteError(http.StatusInternalServerError, err)
		return
	}
	_, _ = response.Write(bs)
	return
}

const defaultAddr = ":8443"

// Server the external provider server
type Server struct {
	Fns       map[string]ServerProviderFn
	Container *restful.Container

	Addr     string
	TLS      bool
	CertFile string
	KeyFile  string
}

// ListenAndServe start the server
func (in *Server) ListenAndServe() (err error) {
	if in.TLS && (in.CertFile == "" || in.KeyFile == "") {
		in.CertFile, in.KeyFile, err = cert.GenerateDefaultSelfSignedCertificateLocally()
		if err != nil {
			return err
		}
	}
	svr := &http.Server{Addr: in.Addr, Handler: in.Container}
	if in.TLS {
		return svr.ListenAndServeTLS(in.CertFile, in.KeyFile)
	}
	return svr.ListenAndServe()
}

// AddFlags set flags
func (in *Server) AddFlags(set *pflag.FlagSet) {
	set.StringVarP(&in.Addr, "addr", "", in.Addr, "address of the server")
	set.BoolVarP(&in.TLS, "tls", "", in.TLS, "enable tls server")
	set.StringVarP(&in.CertFile, "cert-file", "", in.CertFile, "tls certificate path")
	set.StringVarP(&in.KeyFile, "key-file", "", in.KeyFile, "tls key path")
}

// NewCommand create start command
func (in *Server) NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			return in.ListenAndServe()
		},
	}
	in.AddFlags(cmd.Flags())
	return cmd
}

// NewServer create a server for serving as cuex external
func NewServer(path string, fns map[string]ServerProviderFn) *Server {
	container := restful.NewContainer()
	ws := &restful.WebService{}
	ws.Path(path).Consumes(restful.MIME_JSON).Produces(restful.MIME_JSON)
	for name, fn := range fns {
		ws.Route(ws.POST(name).To(fn.Call))
	}
	container.Add(ws)
	return &Server{
		Fns:       fns,
		Container: container,

		Addr: defaultAddr,
		TLS:  true,
	}
}
