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
	"cuelang.org/go/cue/build"

	"github.com/kubevela/pkg/apis/cuex/v1alpha1"
	"github.com/kubevela/pkg/cue/util"
	"github.com/kubevela/pkg/util/maps"
)

const (
	// VelaPrefix default prefix for internal package names
	VelaPrefix = "vela/"
)

type internalPackage struct {
	name     string
	template string
	imp      *build.Instance
	fns      *maps.SyncMap[string, ProviderFn]
}

func (in *internalPackage) GetProviderFn(do string) ProviderFn {
	fn, _ := in.fns.Get(do)
	return fn
}

func (in *internalPackage) GetName() string {
	return in.name
}

func (in *internalPackage) GetPath() string {
	return VelaPrefix + in.name
}

func (in *internalPackage) GetTemplates() []string {
	return []string{in.template}
}

func (in *internalPackage) GetImports() []*build.Instance {
	return []*build.Instance{in.imp}
}

var _ Package = &internalPackage{}

// NewInternalPackage create package based on given functions
func NewInternalPackage(name string, template string, fns map[string]ProviderFn) (Package, error) {
	pkg := &internalPackage{name: name, template: template}
	bi, err := util.BuildImport(pkg.GetPath(), map[string]string{"-": template})
	if err != nil {
		return nil, err
	}
	pkg.imp = bi
	pkg.fns = maps.NewSyncMapFrom(fns)
	return pkg, nil
}

type externalPackage struct {
	src     *v1alpha1.Package
	imports []*build.Instance
}

func (in *externalPackage) GetProviderFn(do string) ProviderFn {
	if in.src.Spec.Provider == nil {
		return nil
	}
	fn := ExternalProviderFn(*in.src.Spec.Provider)
	return &fn
}

func (in *externalPackage) GetName() string {
	return in.src.Name
}

func (in *externalPackage) GetPath() string {
	return in.src.Spec.Path
}

func (in *externalPackage) GetTemplates() []string {
	return maps.Values(in.src.Spec.Templates)
}

func (in *externalPackage) GetImports() []*build.Instance {
	return in.imports
}

var _ Package = &externalPackage{}

// NewExternalPackage create Package based on given CRD object
func NewExternalPackage(src *v1alpha1.Package) (Package, error) {
	pkg := &externalPackage{src: src}
	bi, err := util.BuildImport(pkg.GetPath(), src.Spec.Templates)
	if err != nil {
		return nil, err
	}
	pkg.imports = []*build.Instance{bi}
	return pkg, nil
}
