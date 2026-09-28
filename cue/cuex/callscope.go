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
	"strconv"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
)

// callScope is the set of the value's own fields that could hold a provider
// call, read from the syntax before anything is evaluated.
//
// Finding calls means walking the value, and that walk is most of a resolve.
// Nearly all of what it walks is the manifest being rendered, which declares
// no calls at all: in a definition with one call, an eighth of the nodes are
// under the field holding it and the rest are the workload. The syntax says
// which fields those are without evaluating anything, so the walk can leave
// the rest alone.
//
// A nil scope means walk everything, and that is the answer whenever the
// syntax cannot settle it. Getting this wrong loses a call and reports
// nothing, so every doubt is resolved by looking at more rather than less.
// The names are in the order the file declares them, which is the order the
// walk used to find the calls under them and so the order they run in.
type callScope []string

// scopeOf reads the fields that could hold a call.
//
// It gives up, returning nil, on anything that could put a call somewhere the
// syntax does not name: a file that embeds an expression at the top level
// merges it into the root, and a field whose label has to be computed has no
// name to match against a path until it is evaluated.
func scopeOf(f *ast.File, imports []*build.Instance) callScope {
	providers := providerImports(f, imports)
	decls, ok := topLevel(f)
	if !ok {
		return nil
	}
	named := make([]bool, len(decls))
	for i, d := range decls {
		named[i] = holdsACall(d.value, providers)
	}
	return scopeFrom(decls, named)
}

// topLevelDecl is a field the file declares at its top level, which is the
// only place the scope can name.
type topLevelDecl struct {
	name  string
	value ast.Expr
}

// topLevel reads the file's top-level fields, reporting false where something
// there cannot be named.
func topLevel(f *ast.File) ([]topLevelDecl, bool) {
	var decls []topLevelDecl
	for _, d := range f.Decls {
		field, ok := d.(*ast.Field)
		if !ok {
			// A package clause, an import, a comment: none of them hold a
			// field. Anything else at the top level is an embedding or a let,
			// which lands somewhere this cannot name.
			switch d.(type) {
			case *ast.Package, *ast.ImportDecl, *ast.CommentGroup, *ast.Attribute:
				continue
			default:
				return nil, false
			}
		}
		name, _, err := ast.LabelName(field.Label)
		if err != nil || name == "" {
			return nil, false
		}
		decls = append(decls, topLevelDecl{name, field.Value})
	}
	return decls, true
}

// scopeFrom grows the fields that name a call into the fields that hold one,
// and returns them in the order the file declares them.
//
// A field that copies one holding a call holds the same call, and its own
// syntax names no provider to say so. A definition is the case that matters:
// it is not run where it is declared, so:
//
//	#tpl: nothing.#Do & {$params: "a"}
//	second: #tpl
//
// leaves second as the only call there is. Repeated until nothing more is
// reached, since a copy can be copied.
func scopeFrom(decls []topLevelDecl, named []bool) callScope {
	holds := map[string]bool{}
	for i, d := range decls {
		if named[i] {
			holds[d.name] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, d := range decls {
			if holds[d.name] || !mentions(d.value, holds) {
				continue
			}
			holds[d.name] = true
			changed = true
		}
	}
	scope := callScope{}
	for _, d := range decls {
		if holds[d.name] {
			scope = append(scope, d.name)
		}
	}
	return scope
}

// mentions reports whether a subtree takes any of the given names as a value
// in itself, rather than reading a field out of one.
//
// The difference decides how much of the template gets walked. A field that
// copies a call is a call:
//
//	second: #tpl
//
// A field that reads a call's output is not, and nearly every template has
// one, because producing output is what the calls are for:
//
//	encoded: KEY: _enc.$returns
//
// Counting the second as a call puts the field holding it in the scope, and
// then the manifest that reads that field, and then there is no scope left.
func mentions(node ast.Node, names map[string]bool) bool {
	// the identifiers a selector or an index reads through, which are being
	// looked into rather than taken
	readThrough := map[*ast.Ident]bool{}
	ast.Walk(node, func(n ast.Node) bool {
		switch expr := n.(type) {
		case *ast.SelectorExpr:
			if id := rootOf(expr.X); id != nil {
				readThrough[id] = true
			}
		case *ast.IndexExpr:
			if id := rootOf(expr.X); id != nil {
				readThrough[id] = true
			}
		}
		return true
	}, nil)

	found := false
	ast.Walk(node, func(n ast.Node) bool {
		if found {
			return false
		}
		if id, ok := n.(*ast.Ident); ok && names[id.Name] && !readThrough[id] {
			found = true
		}
		return !found
	}, nil)
	return found
}

// rootOf is the identifier a selector or index chain starts from.
func rootOf(e ast.Expr) *ast.Ident {
	for {
		switch expr := e.(type) {
		case *ast.Ident:
			return expr
		case *ast.SelectorExpr:
			e = expr.X
		case *ast.IndexExpr:
			e = expr.X
		default:
			return nil
		}
	}
}

// providerImports is what the file calls the provider packages it imports.
//
// A call written as base64.#Encode names no #do of its own: the #do is in the
// package, and what the file holds is the name it imported it under. Only
// provider packages are importable, since CUE's own are builtin and never
// reach the instances, so any import that matches one is enough to look at.
func providerImports(f *ast.File, imports []*build.Instance) map[string]bool {
	if len(f.Imports) == 0 {
		return nil
	}
	named := map[string]bool{}
	for _, spec := range f.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		found := false
		for _, imported := range imports {
			if imported != nil && imported.ImportPath == path {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		if spec.Name != nil {
			named[spec.Name.Name] = true
			continue
		}
		named[lastSegment(path)] = true
	}
	return named
}

func lastSegment(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

// holdsACall reports whether a subtree could produce a provider call: a #do
// written into it, or a mention of a provider package it could come from.
//
// Both are looked for anywhere underneath, comprehensions and computed labels
// included, because wherever the syntax puts a call the value puts it under
// the same field.
func holdsACall(node ast.Node, providers map[string]bool) bool {
	found := false
	ast.Walk(node, func(n ast.Node) bool {
		if found {
			return false
		}
		switch node := n.(type) {
		case *ast.Field:
			if name, _, err := ast.LabelName(node.Label); err == nil && name == doKey {
				found = true
			}
		case *ast.Ident:
			if providers[node.Name] {
				found = true
			}
		}
		return !found
	}, nil)
	return found
}
