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

package runtime

import (
	"context"
	"time"

	"cuelang.org/go/cue/build"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/kubevela/pkg/apis/cuex/v1alpha1"
	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/maps"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/kubevela/pkg/util/slices"
)

const defaultResyncPeriod = 5 * time.Minute

// PackageManager manages cue packages
type PackageManager struct {
	Internals *maps.SyncMap[string, Package]
	Externals *maps.SyncMap[string, Package]

	ResyncPeriod time.Duration
	StopCh       chan struct{}
}

type PackageManagerOption interface {
	ApplyTo(*PackageManager)
}

type WithResyncPeriod time.Duration

func (in WithResyncPeriod) ApplyTo(m *PackageManager) {
	m.ResyncPeriod = time.Duration(in)
}

type WithInternalPackage struct {
	Package
}

func (in WithInternalPackage) ApplyTo(m *PackageManager) {
	m.Internals.Set(in.GetPath(), in)
}

// NewPackageManager create PackageManager with given options
func NewPackageManager(opts ...PackageManagerOption) *PackageManager {
	m := &PackageManager{
		Internals:    maps.NewSyncMap[string, Package](),
		Externals:    maps.NewSyncMap[string, Package](),
		ResyncPeriod: defaultResyncPeriod,
	}
	for _, opt := range opts {
		opt.ApplyTo(m)
	}
	return m
}

func (in *PackageManager) getExternalPackageID(pkg *v1alpha1.Package) string {
	return "external://" + pkg.GetNamespace() + "/" + pkg.GetName()
}

func (in *PackageManager) setExternalPackage(pkg *v1alpha1.Package) {
	_id := in.getExternalPackageID(pkg)
	_pkg, err := NewExternalPackage(pkg)
	if err != nil {
		klog.Errorf("parse external package %s/%s failed: %s", pkg.Namespace, pkg.Name, err.Error())
		return
	}
	in.Externals.Set(_id, _pkg)
}

func (in *PackageManager) delExternalPackage(pkg *v1alpha1.Package) {
	_id := in.getExternalPackageID(pkg)
	in.Externals.Del(_id)
}

// LoadExternalPackages load all external packages
func (in *PackageManager) LoadExternalPackages(ctx context.Context) error {
	pkgs, err := singleton.DynamicClient.Get().Resource(v1alpha1.PackageGroupVersionResource).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, pkg := range pkgs.Items {
		_pkg := &v1alpha1.Package{}
		if err = apiruntime.DefaultUnstructuredConverter.FromUnstructured(pkg.Object, _pkg); err != nil {
			return err
		}
		in.setExternalPackage(_pkg)
	}
	return nil
}

// ListenExternalPackages start informer to listen external package changes
func (in *PackageManager) ListenExternalPackages(stopCh <-chan struct{}) {
	if stopCh == nil && in.StopCh == nil {
		in.StopCh = make(chan struct{})
		stopCh = in.StopCh
	}
	factory := dynamicinformer.NewDynamicSharedInformerFactory(
		singleton.DynamicClient.Get(), in.ResyncPeriod)
	informer := factory.ForResource(v1alpha1.PackageGroupVersionResource).Informer()
	defer runtime.HandleCrash()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if o, err := k8s.AsStructured[v1alpha1.Package](obj.(*unstructured.Unstructured)); err == nil {
				in.setExternalPackage(o)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if o, err := k8s.AsStructured[v1alpha1.Package](newObj.(*unstructured.Unstructured)); err == nil {
				in.setExternalPackage(o)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if o, err := k8s.AsStructured[v1alpha1.Package](obj.(*unstructured.Unstructured)); err == nil {
				in.delExternalPackage(o)
			}
		},
	})
	informer.Run(stopCh)
}

// LoadInternalPackages load given internal packages
func (in *PackageManager) LoadInternalPackages(pkgs ...Package) {
	for _, pkg := range pkgs {
		in.Internals.Set(pkg.GetName(), pkg)
	}
}

// GetPackages return all internal and external packages
func (in *PackageManager) GetPackages() []Package {
	return append(in.Internals.Values(), in.Externals.Values()...)
}

// GetImports return all build.Instances built by given packages
func (in *PackageManager) GetImports() []*build.Instance {
	return slices.Flatten(slices.Map(
		in.GetPackages(),
		func(p Package) []*build.Instance { return p.GetImports() },
	))
}

// GetProviders return all providers provisioned by given packages
func (in *PackageManager) GetProviders() map[string]Provider {
	m := map[string]Provider{}
	for _, pkg := range in.GetPackages() {
		m[pkg.GetName()] = pkg
	}
	return m
}
