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

package multicluster

import (
	"context"
	"k8s.io/client-go/rest"
	"net/http"
	"net/url"
	"path"
	"strings"

	clustergatewayconfig "github.com/oam-dev/cluster-gateway/pkg/config"
	knet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/client-go/transport"
	"k8s.io/utils/pointer"

	"github.com/kubevela/pkg/util/net"
)

// Transport the transport for multi-cluster request
type Transport struct {
	// delegate the underlying RoundTripper
	delegate http.RoundTripper

	// cluster the proxy target. If empty, the target will be determined from
	// request context dynamically.
	cluster *string
}

// TransportOption option for creating transport
type TransportOption interface {
	ApplyToTransport(*Transport)
}

// ForCluster create transport for specified cluster
type ForCluster string

// ApplyToTransport .
func (op ForCluster) ApplyToTransport(t *Transport) {
	t.cluster = pointer.String(string(op))
}

// NewTransport create a transport instance for handling multi-cluster request
func NewTransport(rt http.RoundTripper) *Transport {
	return &Transport{delegate: rt}
}

// NewTransportWrapper create a WrapperFunc for wrapping RoundTripper with
// multi-cluster transport
func NewTransportWrapper(options ...TransportOption) transport.WrapperFunc {
	return func(rt http.RoundTripper) http.RoundTripper {
		t := NewTransport(rt)
		for _, op := range options {
			op.ApplyToTransport(t)
		}
		return t
	}
}

// EnableMultiCluster enable config to access other cluster through cluster-gateway.
// This can be called multiple times without worrying the duplicated wrapping
func EnableMultiCluster(config *rest.Config, options ...TransportOption) error {
	rt := &fakeRoundTripper{}
	wrapFunc := config.WrapTransport
	if wrapFunc == nil {
		config.Wrap(NewTransportWrapper(options...))
		return nil
	}
	tripper := wrapFunc(rt)
	mockReq := &http.Request{URL: &url.URL{Path: "/test-path"}}
	mockReq = mockReq.WithContext(context.WithValue(context.Background(), clusterKey, "non-local"))
	response, err := tripper.RoundTrip(mockReq)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(response.Request.URL.Path, "/apis/cluster.core.oam.dev/v1alpha1/clustergateways") {
		config.Wrap(NewTransportWrapper())
	}
	return nil
}

var _ http.RoundTripper = &Transport{}
var _ knet.RoundTripperWrapper = &Transport{}

// formatProxyURL will format the request API path by the cluster gateway resources rule
func formatProxyURL(cluster, originalPath string) string {
	originalPath = strings.TrimPrefix(originalPath, "/")
	return path.Clean(strings.Join([]string{
		"/apis",
		clustergatewayconfig.MetaApiGroupName,
		clustergatewayconfig.MetaApiVersionName,
		clustergatewayconfig.MetaApiResourceName,
		cluster,
		"proxy",
		originalPath,
	}, "/"))
}

// getClusterFor get cluster for incoming request. If cluster set in transport,
// it will return the pre-set cluster. Otherwise, it will find cluster in
// context.
func (t *Transport) getClusterFor(req *http.Request) string {
	if t.cluster != nil {
		return *t.cluster
	}
	cluster, _ := ClusterFrom(req.Context())
	return cluster
}

// RoundTrip is the main function for the re-write API path logic
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	cluster := t.getClusterFor(req)
	if !IsLocal(cluster) {
		req = req.Clone(req.Context())
		req.URL.Path = formatProxyURL(cluster, req.URL.Path)
	}
	return t.delegate.RoundTrip(req)
}

// CancelRequest will try cancel request with the inner round tripper
func (t *Transport) CancelRequest(req *http.Request) {
	net.TryCancelRequest(t.WrappedRoundTripper(), req)
}

// WrappedRoundTripper can get the wrapped RoundTripper
func (t *Transport) WrappedRoundTripper() http.RoundTripper {
	return t.delegate
}

type fakeRoundTripper struct{}

func (rt *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{Request: req}, nil
}

func (rt *fakeRoundTripper) CancelRequest(req *http.Request) {}
