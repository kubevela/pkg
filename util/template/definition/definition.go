/*
Copyright 2023 The KubeVela Authors.

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

package definition

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/kubevela/pkg/meta"
	"github.com/kubevela/pkg/util/template"
)

// TemplateLoader loads a template from a Definition.
type TemplateLoader struct {
	cli client.Client
}

type systemNamespaceContextKey int

const (
	definitionNamespace systemNamespaceContextKey = iota
)

// NewTemplateLoader creates a new template loader for definition
func NewTemplateLoader(ctx context.Context, cli client.Client) template.Loader[*loadConfig, *compileConfig] {
	return &TemplateLoader{
		cli: cli,
	}
}

type loadConfig struct {
	typ string
}

// WithType adds a definition type to the loader.
func WithType(typ string) template.LoadOption[*loadConfig] {
	return &withType{
		typ: typ,
	}
}

type withType struct {
	typ string
}

// ApplyTo .
func (in *withType) ApplyTo(cfg *loadConfig) {
	cfg.typ = in.typ
}

type compileConfig struct {
}

type definitionTemplate struct {
	template string
}

func (t *definitionTemplate) Compile(opts ...template.CompileOption[*compileConfig]) string {
	return t.template
}

// LoadTemplate loads the main template from the definition.
func (l *TemplateLoader) LoadTemplate(ctx context.Context, name string, opts ...template.LoadOption[*loadConfig]) (template.Template[*compileConfig], error) {
	cfg := &loadConfig{}
	for _, opt := range opts {
		opt.ApplyTo(cfg)
	}
	if cfg.typ == "" {
		return nil, fmt.Errorf("definition type is required")
	}

	def := &v1alpha1.Definition{}
	ns := NamespaceFrom(ctx)
	if err := l.cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: fmt.Sprintf("%s-%s", cfg.typ, name)}, def); err != nil {
		if errors.IsNotFound(err) {
			// fallback to name without type
			if err := l.cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, def); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	if def.Spec.Type != cfg.typ {
		return nil, fmt.Errorf("definition %s is not of type %s", name, cfg.typ)
	}
	// only support main.cue as the entry point of main template currently
	if template, ok := def.Spec.Templates["main.cue"]; ok {
		return &definitionTemplate{template: template}, nil
	}
	return nil, fmt.Errorf("main.cue not found in %s", def.Name)
}

// NamespaceFrom returns the namespace from context
func NamespaceFrom(ctx context.Context) string {
	ns, _ := ctx.Value(definitionNamespace).(string)
	if ns == "" {
		ns = meta.NamespaceVelaSystem
	}
	return ns
}

// WithNamespace returns a context with namespace
func WithNamespace(ctx context.Context, ns string) context.Context {
	return context.WithValue(ctx, definitionNamespace, ns)
}
