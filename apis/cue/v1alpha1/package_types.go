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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Package is an extension for cuex engine
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:printcolumn:name="PATH",type=string,JSONPath=`.spec.path`
// +kubebuilder:printcolumn:name="PROTO",type=string,JSONPath=`.spec.provider.protocol`
// +kubebuilder:printcolumn:name="ENDPOINT",type=string,JSONPath=`.spec.provider.endpoint`
// +kubebuilder:resource:shortName={pkg,cpkg,cuepkg,cuepackage}
type Package struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PackageSpec `json:"spec"`
}

// PackageSpec the spec for Package
type PackageSpec struct {
	Path      string            `json:"path"`
	Provider  *Provider         `json:"provider,omitempty"`
	Templates map[string]string `json:"templates"`
}

// ProviderProtocol the protocol type for external Provider
type ProviderProtocol string

const (
	// ProtocolGRPC protocol type grpc for external Provider
	ProtocolGRPC ProviderProtocol = "grpc"
	// ProtocolHTTP protocol type http for external Provider
	ProtocolHTTP ProviderProtocol = "http"
)

// Provider the external Provider in Package for cuex to run functions
type Provider struct {
	Protocol ProviderProtocol `json:"protocol"`
	Endpoint string           `json:"endpoint"`
}

// PackageList list for Package
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PackageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Package `json:"items"`
}
