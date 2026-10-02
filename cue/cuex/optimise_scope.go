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

package cuex

import (
	"cuelang.org/go/cue/ast"
)

// What an expression reads from outside itself.
//
// A prepass evaluates one iteration of a loop somewhere small, and to do
// that it has to know what that iteration reaches for. Getting this wrong
// is the whole risk of the idea: a name left out is an error and shows
// itself, but a name resolved to the wrong thing is an answer that looks
// fine and is not.
//
// So this is deliberately blunt. It reports every identifier an expression
// mentions that the expression does not itself bind, and nothing decides
// later that one of them did not matter. Over-reporting costs a larger
// document to evaluate; under-reporting costs a wrong answer.

// freeIdents is the identifiers an expression reads from its surroundings,
// in the order they are first met. Anything bound within the expression, by
// a let or a comprehension, is not one of them.
func freeIdents(node ast.Node) []string {
	f := &identScan{seen: map[string]bool{}}
	f.walk(node, map[string]bool{})
	return f.order
}

type identScan struct {
	seen  map[string]bool
	order []string
}

func (f *identScan) add(name string) {
	// _ is the blank identifier in a comprehension, never a reference
	if name == "" || name == "_" || f.seen[name] {
		return
	}
	f.seen[name] = true
	f.order = append(f.order, name)
}

// walk reports the free identifiers of node, given the names already bound
// around it. bound is copied wherever a construct adds to it, so a binding
// in one branch does not leak into its siblings.
func (f *identScan) walk(node ast.Node, bound map[string]bool) {
	switch n := node.(type) {
	case nil:
		return

	case *ast.Ident:
		if !bound[n.Name] {
			f.add(n.Name)
		}

	case *ast.SelectorExpr:
		// only the root of a.b.c is a reference; b and c are labels
		f.walk(n.X, bound)

	case *ast.Field:
		// A label is not a reference, except where it is computed: an
		// interpolated or bracketed label reads what it interpolates.
		switch label := n.Label.(type) {
		case *ast.Interpolation:
			f.walk(label, bound)
		case *ast.ListLit:
			// a pattern constraint, [string]: ..., binds nothing here
			f.walk(label, bound)
		}
		f.walk(n.Value, bound)

	case *ast.LetClause:
		// the bound name is visible after this clause, not inside its own
		// expression
		f.walk(n.Expr, bound)

	case *ast.Comprehension:
		// Each clause can bind names the ones after it see, and the body
		// sees all of them. The source of a for or if is read in the scope
		// that has the clauses before it.
		inner := copyBound(bound)
		for _, clause := range n.Clauses {
			switch c := clause.(type) {
			case *ast.ForClause:
				f.walk(c.Source, inner)
				inner = copyBound(inner)
				if c.Key != nil {
					inner[c.Key.Name] = true
				}
				if c.Value != nil {
					inner[c.Value.Name] = true
				}
			case *ast.IfClause:
				f.walk(c.Condition, inner)
			case *ast.LetClause:
				f.walk(c.Expr, inner)
				inner = copyBound(inner)
				inner[c.Ident.Name] = true
			default:
				f.walk(clause, inner)
			}
		}
		f.walk(n.Value, inner)

	case *ast.StructLit:
		// A let inside a struct is visible to the whole struct, including
		// fields written before it, so the lets are collected first.
		inner := copyBound(bound)
		for _, elt := range n.Elts {
			if let, ok := elt.(*ast.LetClause); ok {
				inner[let.Ident.Name] = true
			}
		}
		for _, elt := range n.Elts {
			f.walk(elt, inner)
		}

	case *ast.File:
		inner := copyBound(bound)
		for _, decl := range n.Decls {
			if let, ok := decl.(*ast.LetClause); ok {
				inner[let.Ident.Name] = true
			}
		}
		for _, decl := range n.Decls {
			f.walk(decl, inner)
		}

	case *ast.Alias:
		// X=expr binds X for what follows; the expression itself is read here
		f.walk(n.Expr, bound)

	default:
		// everything else is read through, children in order
		ast.Walk(node, func(child ast.Node) bool {
			if child == node {
				return true
			}
			f.walk(child, bound)
			return false
		}, nil)
	}
}

func copyBound(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in)+2)
	for k := range in {
		out[k] = true
	}
	return out
}
