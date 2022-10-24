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

import "github.com/spf13/pflag"

// AddFlags add flags for multicluster
func AddFlags(set *pflag.FlagSet) {
	AddClusterGatewayClientFlags(set)
}

// AddClusterGatewayClientFlags add flags for default cluster-gateway client
func AddClusterGatewayClientFlags(set *pflag.FlagSet) {
	set.StringVarP(&DefaultClusterGatewayClientOptions.URL,
		"cluster-gateway-url", "", "",
		"Specify the redirect cluster-gateway url for multi-cluster requests. If empty, it uses the Kubernetes aggregated api to access.")
	set.StringVarP(&DefaultClusterGatewayClientOptions.CAFile,
		"cluster-gateway-ca-file", "", "",
		"Specify the CA file path for cluster-gateway.")
}
