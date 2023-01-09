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
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"

	"github.com/kubevela/pkg/cue/cuex/providers/base64"
	"github.com/kubevela/pkg/cue/cuex/providers/http"
	"github.com/kubevela/pkg/cue/cuex/providers/kube"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/cue/util"
	"github.com/kubevela/pkg/util/singleton"
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
	bi := build.NewContext().NewInstance("", nil)
	bi.Imports = in.PackageManager.GetImports()
	f, err := parser.ParseFile("-", src)
	if err != nil {
		return cue.Value{}, err
	}
	if err = bi.AddSyntax(f); err != nil {
		return cue.Value{}, err
	}
	val := cuecontext.New().BuildInstance(bi)
	return in.Resolve(ctx, val)
}

// Resolve runs the resolve process by calling provider functions
func (in *Compiler) Resolve(ctx context.Context, value cue.Value) (cue.Value, error) {
	newValue := value
	executed := map[string]bool{}
	providers := in.PackageManager.GetProviders()
	for {
		if ddl, ok := ctx.Deadline(); ok && ddl.After(time.Now()) {
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
	compiler := &Compiler{
		PackageManager: cuexruntime.NewPackageManager(
			cuexruntime.WithInternalPackage{Package: base64.Package},
			cuexruntime.WithInternalPackage{Package: http.Package},
			cuexruntime.WithInternalPackage{Package: kube.Package},
		),
	}
	if EnableExternalPackageForDefaultCompiler {
		if err := compiler.LoadExternalPackages(context.Background()); err != nil {
			klog.Errorf("failed to load external packages for cuex default compiler: %s", err.Error())
		}
	}
	if EnableExternalPackageWatchForDefaultCompiler {
		go compiler.ListenExternalPackages(nil)
	}
	return compiler
})

var EnableExternalPackageForDefaultCompiler = true
var EnableExternalPackageWatchForDefaultCompiler = false

// AddFlags add flags for configuring cuex default compiler
func AddFlags(set *pflag.FlagSet) {
	set.BoolVarP(&EnableExternalPackageForDefaultCompiler, "cuex-enable-external-package", "", EnableExternalPackageForDefaultCompiler, "enable load external package for cuex default compiler")
	set.BoolVarP(&EnableExternalPackageWatchForDefaultCompiler, "cuex-enable-external-package-watch", "", EnableExternalPackageWatchForDefaultCompiler, "enable watch external package changes for cuex default compiler")
}

// CompileString use cuex default compiler to compile cue string
func CompileString(ctx context.Context, src string) (cue.Value, error) {
	return DefaultCompiler.Get().CompileString(ctx, src)
}
