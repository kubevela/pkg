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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/kubevela/pkg/cel/template"
)

// A Plan is a properties tree parsed once: every expression in it and every read
// each one makes. Building one calls no resolver and evaluates nothing, so the
// same plan serves admission, dependency planning and evaluation.
type Plan struct {
	engine *Engine
	tree   interface{}
	exprs  []Expression
	reads  map[string][]Read
	// readsOf is the roots each expression reads, each once.
	readsOf map[string][]string
}

// An Expression is one $( ) expression in a tree.
type Expression struct {
	// Property is where it sits, as template.Walk names it.
	Property string
	// Expr is the text inside $( ).
	Expr string
	// Whole reports that it is the property's entire value, so it keeps its type.
	Whole bool
	Reads []Read
}

// A Checker validates every read of one root in a plan against what the host
// knows, such as the resource being admitted and the definitions it names. It
// judges reads without their values: resolving them is a Resolver's job.
type Checker interface {
	Check(reads []Read) []CheckError
}

// CheckerFunc adapts a function to a Checker.
type CheckerFunc func(reads []Read) []CheckError

// Check calls f.
func (f CheckerFunc) Check(reads []Read) []CheckError { return f(reads) }

// A CheckError is one fault in a tree, and where it is.
type CheckError struct {
	Property string
	// Expr is the expression at fault, when the fault is not about one read.
	Expr string
	// Read is the read at fault, when there is one.
	Read Read
	Err  error
}

func (c CheckError) Error() string {
	parts := make([]string, 0, 3)
	if c.Property != "" {
		parts = append(parts, c.Property)
	}
	switch {
	case c.Read.Root != "":
		parts = append(parts, c.Read.String())
	case c.Expr != "":
		parts = append(parts, "$("+c.Expr+")")
	}
	return strings.Join(append(parts, c.Err.Error()), ": ")
}

// Unwrap exposes the underlying error.
func (c CheckError) Unwrap() error { return c.Err }

// CheckErrors is every fault found in one pass.
type CheckErrors []CheckError

func (c CheckErrors) Error() string {
	msgs := make([]string, len(c))
	for i, e := range c {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

// Unwrap exposes each fault, so errors.Is and errors.As see through the list.
func (c CheckErrors) Unwrap() []error {
	out := make([]error, len(c))
	for i, e := range c {
		out[i] = e
	}
	return out
}

// Plan parses every expression in a tree and collects what each reads.
//
// Every expression that does not parse or compile is reported, each with its
// property, as CheckErrors.
func (e *Engine) Plan(tree interface{}) (*Plan, error) {
	p := &Plan{engine: e, tree: tree, reads: map[string][]Read{}, readsOf: map[string][]string{}}
	var faults CheckErrors
	//nolint:errcheck // the visitor records faults rather than stopping
	_ = template.Walk(tree, "", func(at, raw string) error {
		parsed, err := template.Parse(raw)
		if err != nil {
			faults = append(faults, CheckError{Property: at, Err: err})
			return nil
		}
		for _, f := range parsed.Fragments {
			if !f.IsExpr() {
				continue
			}
			refs, err := e.PropertyReferences(f.Expr)
			if err != nil {
				faults = append(faults, CheckError{Property: at, Expr: f.Expr, Err: err})
				continue
			}
			x := Expression{Property: at, Expr: f.Expr, Whole: parsed.Whole()}
			for _, r := range refs {
				read := Read{Reference: r, Property: at}
				x.Reads = append(x.Reads, read)
				p.reads[r.Root] = append(p.reads[r.Root], read)
				if !contains(p.readsOf[f.Expr], r.Root) {
					p.readsOf[f.Expr] = append(p.readsOf[f.Expr], r.Root)
				}
			}
			p.exprs = append(p.exprs, x)
		}
		return nil
	})
	if len(faults) > 0 {
		sort.SliceStable(faults, func(i, j int) bool { return faults[i].Property < faults[j].Property })
		return nil, faults
	}
	// Walk visits a map in no fixed order; a property's own expressions keep
	// theirs.
	sort.SliceStable(p.exprs, func(i, j int) bool { return p.exprs[i].Property < p.exprs[j].Property })
	for root := range p.reads {
		rs := p.reads[root]
		sort.SliceStable(rs, func(i, j int) bool {
			if rs[i].Property != rs[j].Property {
				return rs[i].Property < rs[j].Property
			}
			return rs[i].String() < rs[j].String()
		})
	}
	return p, nil
}

// PlanJSON plans a JSON document, such as a resource's properties, and keeps
// the plan for the document's bytes: the same properties reach admission and
// every render, and a plan is only ever read once built. A document that is
// not JSON, or holds an expression that will not compile, is not kept.
func (e *Engine) PlanJSON(doc []byte) (*Plan, error) {
	key := sha256.Sum256(doc)
	if hit, ok := e.plans.Get(key); ok {
		if p, ok := hit.(*Plan); ok {
			return p, nil
		}
	}
	var tree interface{}
	if err := json.Unmarshal(doc, &tree); err != nil {
		return nil, err
	}
	p, err := e.Plan(tree)
	if err != nil {
		return nil, err
	}
	e.plans.Add(key, p)
	return p, nil
}

// Reads returns every read of root, sorted by property.
func (p *Plan) Reads(root string) []Read {
	return append([]Read(nil), p.reads[root]...)
}

// Expressions returns every expression, sorted by property.
func (p *Plan) Expressions() []Expression {
	return append([]Expression(nil), p.exprs...)
}

// Check is admission for the tree: every read's root must be one of permitted,
// and each read root with a Checker has it called once with all of its reads.
// Every fault is reported, reads of roots outside permitted first.
func (p *Plan) Check(permitted []string, checkers map[string]Checker) []CheckError {
	var out []CheckError
	for _, root := range p.engine.roots {
		if len(p.reads[root]) == 0 || contains(permitted, root) {
			continue
		}
		quoted := make([]string, len(permitted))
		for i, r := range permitted {
			quoted[i] = strconv.Quote(r)
		}
		for _, r := range p.reads[root] {
			out = append(out, CheckError{Property: r.Property, Read: r,
				Err: fmt.Errorf("%q cannot be read here; this surface permits %s", root, strings.Join(quoted, ", "))})
		}
	}
	for _, root := range p.engine.roots {
		checker, ok := checkers[root]
		if !ok || checker == nil || len(p.reads[root]) == 0 || !contains(permitted, root) {
			continue
		}
		out = append(out, checker.Check(p.Reads(root))...)
	}
	return out
}

// Eval resolves and evaluates the tree.
//
// Each read root's resolver is called once with all of its reads, in the order
// the roots were declared; a root nothing reads is not resolved. The first
// resolver error ends the evaluation, so a host declares its cheapest roots,
// and those most likely to wait, first. Only then is every expression
// evaluated, against the permissive environment. A leaf with no expression is
// returned byte-identical, and the tree is left alone.
func (p *Plan) Eval(ctx context.Context, resolvers map[string]Resolver, opts TreeOptions) (interface{}, error) {
	e := p.engine
	activation := map[string]interface{}{}
	unknown := map[string]bool{}
	for _, root := range e.roots {
		rs := p.reads[root]
		if len(rs) == 0 {
			continue
		}
		resolver, ok := resolvers[root]
		if !ok || resolver == nil {
			return nil, fmt.Errorf("%s: nothing resolves %q here", rs[0].String(), root)
		}
		v, err := resolver.Resolve(ctx, p.Reads(root))
		if err != nil {
			return nil, err
		}
		if _, ok := v.(unknownValue); ok {
			unknown[root] = true
			continue
		}
		activation[root] = v
	}

	normalised := normaliseInput(activation)
	return template.Map(p.tree, "", func(_, raw string) (interface{}, error) {
		return interpolate(raw, func(expr string, whole bool) (interface{}, error) {
			for _, root := range p.readsOf[expr] {
				if !unknown[root] {
					continue
				}
				if opts.Unknown == nil {
					return nil, fmt.Errorf("%s has no value in this render", expr)
				}
				return opts.Unknown(expr, whole)
			}
			v, err := e.run(e.dyn, expr, normalised)
			if err != nil && opts.OnEvalError != nil {
				if reported := opts.OnEvalError(expr, p.readsOf[expr], err); reported != nil {
					err = reported
				}
			}
			return v, err
		})
	})
}

// EvalTree is Plan followed by Eval, for a tree evaluated once.
func (e *Engine) EvalTree(ctx context.Context, tree interface{}, resolvers map[string]Resolver,
	opts TreeOptions) (interface{}, error) {
	p, err := e.Plan(tree)
	if err != nil {
		return nil, err
	}
	return p.Eval(ctx, resolvers, opts)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
