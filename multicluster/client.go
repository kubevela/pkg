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
	"os"

	clustergatewayv1alpha1 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// gatedClient use base client to handle hub cluster requests and
// use gateway client to do managed cluster requests
type gatedClient struct {
	base    client.Client
	gateway client.Client
	writer  *gatedStatusWriter
}

// gatedStatusWriter use base writer to handle hub cluster requests and
// use gateway writer to do managed cluster requests
type gatedStatusWriter struct {
	base    client.StatusWriter
	gateway client.StatusWriter
}

var _ client.Client = &gatedClient{}
var _ client.StatusWriter = &gatedStatusWriter{}

func (m *gatedClient) getClientFor(ctx context.Context) client.Client {
	if cluster, exists := ClusterFrom(ctx); !exists || IsLocal(cluster) {
		return m.base
	}
	return m.gateway
}

func (m *gatedStatusWriter) getWriterFor(ctx context.Context) client.StatusWriter {
	if cluster, exists := ClusterFrom(ctx); !exists || IsLocal(cluster) {
		return m.base
	}
	return m.gateway
}

func (m *gatedClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return m.getClientFor(ctx).Get(ctx, key, obj)
}

func (m *gatedClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return m.getClientFor(ctx).List(ctx, list, opts...)
}

func (m *gatedClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return m.getClientFor(ctx).Create(ctx, obj, opts...)
}

func (m *gatedClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return m.getClientFor(ctx).Delete(ctx, obj, opts...)
}

func (m *gatedClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return m.getClientFor(ctx).Update(ctx, obj, opts...)
}

func (m *gatedClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return m.getClientFor(ctx).Patch(ctx, obj, patch, opts...)
}

func (m *gatedClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return m.getClientFor(ctx).DeleteAllOf(ctx, obj, opts...)
}

func (m *gatedClient) Status() client.StatusWriter {
	return m.writer
}

func (m *gatedClient) Scheme() *runtime.Scheme {
	return m.base.Scheme()
}

func (m *gatedClient) RESTMapper() meta.RESTMapper {
	return m.base.RESTMapper()
}

func (m *gatedStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return m.getWriterFor(ctx).Update(ctx, obj, opts...)
}

func (m *gatedStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return m.getWriterFor(ctx).Patch(ctx, obj, patch, opts...)
}

// ClientOptions the options for creating multi-cluster gatedClient
type ClientOptions struct {
	client.Options
	ClusterGateway ClusterGatewayClientOptions
}

// ClusterGatewayClientOptions the options for creating the gateway client
type ClusterGatewayClientOptions struct {
	// URL the url for cluster-gateway. If empty, multi-cluster request will use
	// the Kubernetes aggregated api.
	URL string
	// CAFile the CA file for cluster-gateway. If neither ClusterGatewayURL nor
	// ClusterGatewayCAFile is empty, the CA file will be used when accessing
	// cluster-gateway.
	CAFile string
}

// NewClient create a multi-cluster client for handling multi-cluster requests
// If ClusterGatewayURL is not set, the client will use the Kubernetes
// aggregated api directly. All multi-cluster requests will be directed to
// the hub Kubernetes APIServer. The managed cluster requests will be redirected
// from the Kubernetes APIServer to cluster-gateway.
// If ClusterGatewayURL is set, the client will directly call cluster-gateway
// for managed cluster requests, instead of calling the hub Kubernetes
// APIServer.
func NewClient(config *rest.Config, options ClientOptions) (client.Client, error) {
	wrapped := rest.CopyConfig(config)
	wrapped.Wrap(NewTransportWrapper())
	if len(options.ClusterGateway.URL) == 0 {
		return client.New(wrapped, options.Options)
	}
	base, err := client.New(config, options.Options)
	if err != nil {
		return nil, err
	}
	wrapped.Host = options.ClusterGateway.URL
	if len(options.ClusterGateway.CAFile) > 0 {
		if wrapped.CAData, err = os.ReadFile(options.ClusterGateway.CAFile); err != nil {
			return nil, err
		}
	} else {
		wrapped.CAData = nil
		wrapped.Insecure = true
	}
	if options.Options.Scheme != nil {
		// no err will be returned here
		_ = clustergatewayv1alpha1.AddToScheme(options.Options.Scheme)
	}
	gateway, err := client.New(wrapped, options.Options)
	if err != nil {
		return nil, err
	}
	return &gatedClient{
		base:    base,
		gateway: gateway,
		writer: &gatedStatusWriter{
			base:    base.Status(),
			gateway: gateway.Status(),
		},
	}, nil
}

// DefaultClusterGatewayClientOptions the default ClusterGatewayClientOptions
var DefaultClusterGatewayClientOptions = ClusterGatewayClientOptions{}

// NewDefaultClient create default client with default DefaultClusterGatewayClientOptions
func NewDefaultClient(config *rest.Config, options client.Options) (client.Client, error) {
	return NewClient(config, ClientOptions{
		Options:        options,
		ClusterGateway: DefaultClusterGatewayClientOptions,
	})
}
