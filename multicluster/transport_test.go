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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransport(t *testing.T) {
	r := require.New(t)
	rt := &fakeRoundTripper{}

	// Test dynamic transport
	tp := NewTransport(rt)
	req := &http.Request{URL: &url.URL{Path: "/test-path"}}
	resp, err := tp.RoundTrip(req.WithContext(WithCluster(context.Background(), "example")))
	r.NoError(err)
	r.Equal("/apis/cluster.core.oam.dev/v1alpha1/clustergateways/example/proxy/test-path", resp.Request.URL.Path)
	resp, err = tp.RoundTrip(req.WithContext(WithCluster(context.Background(), Local)))
	r.NoError(err)
	r.Equal("/test-path", resp.Request.URL.Path)

	// Test static transport
	_rt := NewTransportWrapper(ForCluster("static"))(rt)
	resp, err = _rt.RoundTrip(req.WithContext(WithCluster(context.Background(), "example")))
	r.NoError(err)
	r.Equal("/apis/cluster.core.oam.dev/v1alpha1/clustergateways/static/proxy/test-path", resp.Request.URL.Path)

	// Test cancel request
	_rt.(*Transport).CancelRequest(req)
}

func TestEnableMultiCluster(t *testing.T) {
	r := require.New(t)
	config := &rest.Config{}
	// test it add RoundTripperWrapper to config and not wrap it twice
	err := EnableMultiCluster(config)
	r.NoError(err)
	err = EnableMultiCluster(config)
	r.NoError(err)
	req := &http.Request{URL: &url.URL{Path: "/test-path"}}
	rt := &fakeRoundTripper{}
	resp, err := config.WrapTransport(rt).RoundTrip(req.WithContext(WithCluster(context.Background(), "example")))
	r.NoError(err)
	r.Equal("/apis/cluster.core.oam.dev/v1alpha1/clustergateways/example/proxy/test-path", resp.Request.URL.Path)
}
