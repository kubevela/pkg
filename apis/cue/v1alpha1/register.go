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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apiruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	// Group .
	Group = "cue.oam.dev"
	// Version .
	Version = "v1alpha1"
)

// GroupVersion .
var GroupVersion = schema.GroupVersion{Group: Group, Version: Version}

// PackageResource resource name for Package
const PackageResource = "packages"

// PackageGroupVersionResource GroupVersionResource for Package
var PackageGroupVersionResource = GroupVersion.WithResource(PackageResource)

func init() {
	apiruntime.Must(AddToScheme(scheme.Scheme))
}

// AddToScheme .
var AddToScheme = func(scheme *runtime.Scheme) error {
	metav1.AddToGroupVersion(scheme, GroupVersion)
	scheme.AddKnownTypes(GroupVersion, &Package{}, &PackageList{})
	return nil
}
