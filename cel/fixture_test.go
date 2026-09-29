/*
Copyright 2026 The KubeVela Authors.

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
package cel

import (
	"context"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/google/cel-go/cel"
	apiservercel "k8s.io/apiserver/pkg/cel"

	"github.com/kubevela/pkg/cel/template"
)

// fixture is an engine with roots and qualifiers named as the first host names
// them, so these tests read like the expressions authors write. Nothing about
// them is special to the engine.
var fixture = func() *Engine {
	e, err := NewEngine(
		Root{Name: "source"},
		Root{Name: "context"},
		Root{Name: "component", QualifiedAt: 1, Qualifiers: []Qualifier{
			{Name: "cluster"},
			{Name: "namespace"},
			{Name: "placements", Result: cel.ListType(cel.DynType)},
		}},
	)
	if err != nil {
		panic(err)
	}
	return e
}()

func dynEnv() (*cel.Env, error) { return fixture.DynEnv(), nil }

// typedEnv types source from binding schemas and context from field types.
func typedEnv(sources map[string]cue.Value, ctx map[string]*apiservercel.DeclType) (*cel.Env, error) {
	return fixture.TypedEnv("", func() (map[string]*apiservercel.DeclType, error) {
		src := map[string]*apiservercel.DeclField{}
		for name, schema := range sources {
			src[name] = apiservercel.NewDeclField(name, DeclType(schema, "test.source."+name), true, nil, nil)
		}
		c := map[string]*apiservercel.DeclField{}
		for name, t := range ctx {
			c[name] = apiservercel.NewDeclField(name, t, true, nil, nil)
		}
		return map[string]*apiservercel.DeclType{
			"source":  apiservercel.NewObjectType("test.source", src),
			"context": apiservercel.NewObjectType("test.context", c),
		}, nil
	})
}

// typedEnvText is typedEnv from schemas written as CUE text.
func typedEnvText(schemas map[string]string) (*cel.Env, error) {
	cc := cuecontext.New()
	sources := map[string]cue.Value{}
	for name, text := range schemas {
		v := cc.CompileString(text)
		if v.Err() != nil {
			return nil, fmt.Errorf("schema %q: %w", name, v.Err())
		}
		sources[name] = v
	}
	return typedEnv(sources, nil)
}

func evalTree(v interface{}, resolved map[string]map[string]interface{}, ctx map[string]interface{}) (interface{}, error) {
	sources := map[string]interface{}{}
	for name, values := range resolved {
		sources[name] = values
	}
	return fixture.EvalTree(context.Background(), v, map[string]Resolver{
		"source":  Static(sources),
		"context": Static(ctx),
	}, TreeOptions{})
}

// undefendedReads is the absent-read rule over source: reads an optional path
// may leave missing, with no guard.
func undefendedReads(env *cel.Env, expr string, optional func(template.Reference) bool) ([]template.Reference, error) {
	refs, err := fixture.References(env, expr)
	if err != nil {
		return nil, err
	}
	var out []template.Reference
	for _, r := range refs {
		if r.Root == "source" && !r.Defaulted && optional(r) {
			out = append(out, r)
		}
	}
	return out, nil
}

// validateTree is admission's root check through a plan: the first fault, or nil.
func validateTree(e *Engine, tree interface{}, roots ...string) error {
	p, err := e.Plan(tree)
	if err != nil {
		return err
	}
	if faults := p.Check(roots, nil); len(faults) > 0 {
		return faults[0]
	}
	return nil
}
