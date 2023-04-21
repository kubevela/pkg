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

package cuex

import (
	"context"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"github.com/spf13/pflag"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	"github.com/kubevela/pkg/cue/cuex/providers/base64"
	cueext "github.com/kubevela/pkg/cue/cuex/providers/cue"
	"github.com/kubevela/pkg/cue/cuex/providers/http"
	"github.com/kubevela/pkg/cue/cuex/providers/kube"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/cue/util"
	"github.com/kubevela/pkg/util/runtime"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/kubevela/pkg/util/slices"
)

const (
	doKey       = "#do"
	providerKey = "#provider"
)

// Compiler for compile cue strings into cue.Value
type Compiler struct {
	*cuexruntime.PackageManager
}

// CompileString compile given cue string into cue.Value
func (in *Compiler) CompileString(ctx context.Context, src string) (cue.Value, error) {
	return in.CompileStringWithOptions(ctx, src)
}

// CompileConfig config for running compile process
type CompileConfig struct {
	ResolveProviderFunctions bool
	PreResolveMutators       []func(context.Context, string) (string, error)
}

// NewCompileConfig create new CompileConfig
func NewCompileConfig(opts ...CompileOption) *CompileConfig {
	cfg := &CompileConfig{
		ResolveProviderFunctions: true,
		PreResolveMutators:       nil,
	}
	for _, opt := range opts {
		opt.ApplyTo(cfg)
	}
	return cfg
}

// CompileOption options for compile cue string
type CompileOption interface {
	ApplyTo(*CompileConfig)
}

// WithExtraData fill the cue.Value before resolve
func WithExtraData(key string, data interface{}) CompileOption {
	return &withExtraData{
		key:  key,
		data: data,
	}
}

type withExtraData struct {
	key  string
	data interface{}
}

// ApplyTo .
func (in *withExtraData) ApplyTo(cfg *CompileConfig) {
	cfg.PreResolveMutators = append(cfg.PreResolveMutators, func(_ context.Context, template string) (string, error) {
		val, path := cuecontext.New().CompileString(""), cue.ParsePath(in.key)
		if runtime.IsNil(in.data) {
			val = val.FillPath(path, struct{}{})
		} else {
			val = val.FillPath(path, in.data)
		}
		data, err := util.ToString(val)
		return strings.Join([]string{template, data}, "\n"), err
	})
}

var _ CompileOption = DisableResolveProviderFunctions{}

// DisableResolveProviderFunctions disable ResolveProviderFunctions
type DisableResolveProviderFunctions struct{}

// ApplyTo .
func (in DisableResolveProviderFunctions) ApplyTo(cfg *CompileConfig) {
	cfg.ResolveProviderFunctions = false
}

// CompileStringWithOptions compile given cue string with extra options
func (in *Compiler) CompileStringWithOptions(ctx context.Context, src string, opts ...CompileOption) (cue.Value, error) {
	var err error
	cfg := NewCompileConfig(opts...)
	bi := build.NewContext().NewInstance("", nil)
	bi.Imports = in.PackageManager.GetImports()
	for _, mutator := range cfg.PreResolveMutators {
		if src, err = mutator(ctx, src); err != nil {
			return cue.Value{}, err
		}
	}
	f, err := parser.ParseFile("-", src, parser.ParseComments)
	if err != nil {
		return cue.Value{}, err
	}
	if err = bi.AddSyntax(f); err != nil {
		return cue.Value{}, err
	}
	val := cuecontext.New().BuildInstance(bi)
	if cfg.ResolveProviderFunctions {
		return in.Resolve(ctx, val)
	}
	return val, nil
}

// Resolve runs the resolve process by calling provider functions
func (in *Compiler) Resolve(ctx context.Context, value cue.Value) (cue.Value, error) {
	newValue := value
	executed := map[string]bool{}
	providers := in.PackageManager.GetProviders()
	for {
		if ddl, ok := ctx.Deadline(); ok && ddl.Before(time.Now()) {
			return newValue, ResolveTimeoutErr{}
		}
		var next *cue.Value
		// 1. find the next to execute
		util.Iterate(newValue, func(v cue.Value) (stop bool) {
			_, done := executed[v.Path().String()]
			fn, _ := v.LookupPath(cue.ParsePath(doKey)).String()
			if !done && fn != "" {
				next = &v
				return true
			}
			return false
		})
		if next == nil {
			break
		}
		// 2. execute
		fn, _ := next.LookupPath(cue.ParsePath(doKey)).String()
		prdName, _ := next.LookupPath(cue.ParsePath(providerKey)).String()
		prd, found := providers[prdName]
		if !found {
			return newValue, ProviderNotFoundErr(prdName)
		}
		f := prd.GetProviderFn(fn)
		if f == nil {
			return newValue, ProviderFnNotFoundErr{Provider: prdName, Fn: fn}
		}
		val, err := f.Call(ctx, *next)
		if err != nil {
			return newValue, NewFunctionCallError(val, err)
		}
		newValue = newValue.FillPath(next.Path(), val)
		executed[next.Path().String()] = true
	}
	return newValue, nil
}

// DefaultCompiler compiler for cuex to compile
var DefaultCompiler = singleton.NewSingleton[*Compiler](func() *Compiler {
	compiler := NewCompilerWithDefaultInternalPackages()
	if EnableExternalPackageForDefaultCompiler {
		if err := compiler.LoadExternalPackages(context.Background()); err != nil && !kerrors.IsNotFound(err) {
			klog.Errorf("failed to load external packages for cuex default compiler: %s", err.Error())
		}
	}
	if EnableExternalPackageWatchForDefaultCompiler {
		go compiler.ListenExternalPackages(nil)
	}
	return compiler
})

// NewCompilerWithInternalPackages create compiler with internal packages
func NewCompilerWithInternalPackages(packages ...cuexruntime.Package) *Compiler {
	return &Compiler{
		PackageManager: cuexruntime.NewPackageManager(
			slices.Map(packages, func(p cuexruntime.Package) cuexruntime.PackageManagerOption {
				return cuexruntime.WithInternalPackage{Package: p}
			})...,
		),
	}
}

// NewCompilerWithDefaultInternalPackages create compiler with default internal packages
func NewCompilerWithDefaultInternalPackages() *Compiler {
	return NewCompilerWithInternalPackages(
		base64.Package,
		http.Package,
		kube.Package,
		cueext.Package,
	)
}

var (
	// EnableExternalPackageForDefaultCompiler .
	EnableExternalPackageForDefaultCompiler = true
	// EnableExternalPackageWatchForDefaultCompiler .
	EnableExternalPackageWatchForDefaultCompiler = false
)

// AddFlags add flags for configuring cuex default compiler
func AddFlags(set *pflag.FlagSet) {
	set.BoolVarP(&EnableExternalPackageForDefaultCompiler, "enable-external-cue-package", "", EnableExternalPackageForDefaultCompiler, "enable load external package for cuex default compiler")
	set.BoolVarP(&EnableExternalPackageWatchForDefaultCompiler, "list-watch-external-cue-package", "", EnableExternalPackageWatchForDefaultCompiler, "enable watch external package changes for cuex default compiler")
	set.BoolVarP(&cuexruntime.DefaultClientInsecureSkipVerify, "cuex-external-provider-insecure-skip-verify", "", cuexruntime.DefaultClientInsecureSkipVerify, "Set if the default external provider client of cuex should skip insecure verify")
}

// CompileString use cuex default compiler to compile cue string
func CompileString(ctx context.Context, src string) (cue.Value, error) {
	return DefaultCompiler.Get().CompileStringWithOptions(ctx, src)
}

// CompileStringWithOptions use cuex default compiler to compile cue string with options
func CompileStringWithOptions(ctx context.Context, src string, opts ...CompileOption) (cue.Value, error) {
	return DefaultCompiler.Get().CompileStringWithOptions(ctx, src, opts...)
}
