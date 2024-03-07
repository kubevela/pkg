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
	"os"

	clustergatewayv1alpha1 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClientOptions the options for creating multi-cluster gatedClient
type ClientOptions struct {
	client.Options
	ClusterGateway             ClusterGatewayClientOptions
	DisableRemoteClusterClient bool
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
	constructor := NewRemoteClusterClient
	if options.DisableRemoteClusterClient {
		constructor = client.New
	}
	if len(options.ClusterGateway.URL) == 0 {
		return constructor(wrapped, options.Options)
	}
	var err error
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
	return constructor(wrapped, options.Options)
}

// DefaultClusterGatewayClientOptions the default ClusterGatewayClientOptions
var DefaultClusterGatewayClientOptions = ClusterGatewayClientOptions{}

// NewDefaultClient create default client with default DefaultClusterGatewayClientOptions
func NewDefaultClient(config *rest.Config, options client.Options) (client.Client, error) {
	return NewClient(config, ClientOptions{
		Options:                    options,
		ClusterGateway:             DefaultClusterGatewayClientOptions,
		DisableRemoteClusterClient: DefaultDisableRemoteClusterClient,
	})
}
