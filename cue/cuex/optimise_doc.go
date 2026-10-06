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
	"cuelang.org/go/cue/token"
)

// onItsOwnLine puts every declaration of an assembled file where the
// formatter can see it should start a line.
//
// The declarations are nodes lifted out of the template, and they carry
// where they sat in it. Two imports that were lines apart there come out
// side by side here, which does not parse, and a file this cannot read
// back is a file it cannot build.
func onItsOwnLine(f *ast.File) *ast.File {
	for _, decl := range f.Decls {
		ast.SetRelPos(decl, token.Newline)
		imp, is := decl.(*ast.ImportDecl)
		if !is {
			continue
		}
		if len(imp.Specs) == 1 {
			// import "list", which has to stay on the one line
			imp.Lparen = token.NoPos
			imp.Rparen = token.NoPos
			ast.SetRelPos(imp.Specs[0], token.Blank)
			continue
		}
		// more than one, so the bracketed form, one to a line
		imp.Lparen = token.Blank.Pos()
		imp.Rparen = token.Newline.Pos()
		for _, spec := range imp.Specs {
			ast.SetRelPos(spec, token.Newline)
		}
	}
	return f
}

// One iteration of a loop, as a file of its own.
//
// The loop is left exactly as the template wrote it and its source is
// replaced by a list of one. Everything else about the comprehension, its
// body, its conditions, the names it binds, is carried over untouched, so
// the iteration means in the small document what it meant in the file. The
// alternative is to pull the call apart and rebuild the parameters from its
// pieces, which is more code and more ways to be wrong about what a
// template said.
//
// What comes back is a file holding the imports and declarations that
// iteration reads, and one field with one call in it. Reading that call's
// $params is the point of the exercise.

// iterationField is where the one call lands, named so nothing a template
// could write collides with it.
const iterationField = "_cuex_iteration"

// keysIndexVar names the position within a run, so the keys of each
// iteration in it can be told apart.
const keysIndexVar = "_cuex_at"

// iterationDoc builds the file for a run of iterations. It reports false
// where the loop is a shape this does not handle, which is a reason to
// leave the loop to the resolver and never a reason to guess.
//
// A run rather than one, because a document costs something to build
// whatever is in it: a context, the provider packages, the declarations
// the body reads. Paid once for one iteration that is most of the cost;
// paid once for a hundred it is nothing. What the run is chosen to be
// decides the peak, since that is how many calls are in memory at once,
// and it is the only thing that does.
func iterationDoc(
	scope fileScope,
	comp *ast.Comprehension,
	at []ast.Expr,
	skip map[string]bool,
	wide int,
	narrow map[string]map[string]bool,
	rep *report,
) (*ast.File, bool) {
	forClause, ok := onlyForClause(comp)
	if !ok {
		rep.note("doc:onlyForClause")
		return nil, false
	}

	// what the body and the surviving clauses read, less what the loop
	// binds for itself
	bound := copyBound(skip)
	if forClause.Value != nil {
		bound[forClause.Value.Name] = true
	}
	seeds := freeIdents(comp.Value)
	for _, clause := range comp.Clauses {
		if clause == forClause {
			continue
		}
		seeds = append(seeds, freeIdents(clause)...)
	}
	names, imports, ok := scope.closureOf(seeds, bound)
	if !ok {
		rep.note("doc:closure")
		return nil, false
	}
	if carriesAWholeLoop(scope, names, wide, narrow) {
		rep.note("doc:carriesAWholeLoop")
		return nil, false
	}

	// the same comprehension, over one element
	pinned := &ast.Comprehension{
		Clauses: make([]ast.Clause, 0, len(comp.Clauses)),
		Value:   comp.Value,
	}
	for _, clause := range comp.Clauses {
		if clause == forClause {
			pinned.Clauses = append(pinned.Clauses, &ast.ForClause{
				Key:    forClause.Key,
				Value:  forClause.Value,
				Source: ast.NewList(at...),
			})
			continue
		}
		pinned.Clauses = append(pinned.Clauses, clause)
	}

	file := &ast.File{}
	if len(imports) > 0 {
		specs := make([]*ast.ImportSpec, len(imports))
		copy(specs, imports)
		file.Decls = append(file.Decls, &ast.ImportDecl{Specs: specs})
	}
	for _, name := range names {
		decls := scope.decls[name]
		if keys, narrowing := narrow[name]; narrowing {
			cut, ok := narrowTo(decls, keys)
			if !ok {
				rep.note("doc:narrowTo")
				return nil, false
			}
			decls = cut
		}
		for _, field := range decls {
			file.Decls = append(file.Decls, field)
		}
	}
	file.Decls = append(file.Decls, &ast.Field{
		Label: ast.NewIdent(iterationField),
		Value: &ast.StructLit{Elts: []ast.Decl{pinned}},
	})
	return onItsOwnLine(file), true
}

// keysDoc is the file that says which elements of the narrowable names
// this iteration reads: the same loop, pinned the same way, with a list of
// the index expressions in place of the body.
//
// The names being narrowed are left out of it, so it stays small however
// many answers they hold. An index that reads one of them cannot be worked
// out this way, and the loop is declined rather than guessed at.
func keysDoc(
	scope fileScope,
	comp *ast.Comprehension,
	at []ast.Expr,
	skip map[string]bool,
	indices []ast.Expr,
	without map[string]bool,
) (*ast.File, bool) {
	forClause, ok := onlyForClause(comp)
	if !ok {
		return nil, false
	}
	bound := copyBound(skip)
	if forClause.Value != nil {
		bound[forClause.Value.Name] = true
	}
	var seeds []string
	for _, idx := range indices {
		seeds = append(seeds, freeIdents(idx)...)
	}
	for _, name := range seeds {
		if without[name] {
			// the key depends on what is being narrowed away, so there is
			// no small document that can work it out
			return nil, false
		}
	}
	names, imports, ok := scope.closureOf(seeds, bound)
	if !ok {
		return nil, false
	}

	// keyed by the iteration, since a run of them answers at once
	pinned := &ast.Comprehension{Value: &ast.StructLit{Elts: []ast.Decl{
		&ast.Field{
			Label: &ast.Interpolation{Elts: []ast.Expr{
				&ast.BasicLit{Kind: token.STRING, Value: `"\(`},
				ast.NewIdent(keysIndexVar),
				&ast.BasicLit{Kind: token.STRING, Value: `)"`},
			}},
			Value: ast.NewList(indices...),
		},
	}}}
	for _, clause := range comp.Clauses {
		if clause == forClause {
			pinned.Clauses = append(pinned.Clauses, &ast.ForClause{
				Key:    ast.NewIdent(keysIndexVar),
				Value:  forClause.Value,
				Source: ast.NewList(at...),
			})
			continue
		}
		if _, isIf := clause.(*ast.IfClause); isIf {
			// a condition can read what is being narrowed away, and an
			// iteration it excludes has no keys to find anyway
			continue
		}
		pinned.Clauses = append(pinned.Clauses, clause)
	}

	file := &ast.File{}
	if len(imports) > 0 {
		specs := make([]*ast.ImportSpec, len(imports))
		copy(specs, imports)
		file.Decls = append(file.Decls, &ast.ImportDecl{Specs: specs})
	}
	for _, name := range names {
		if without[name] {
			continue
		}
		for _, field := range scope.decls[name] {
			file.Decls = append(file.Decls, field)
		}
	}
	file.Decls = append(file.Decls, &ast.Field{
		Label: ast.NewIdent(iterationField),
		Value: &ast.StructLit{Elts: []ast.Decl{pinned}},
	})
	return onItsOwnLine(file), true
}

// onlyForClause reports the comprehension's single for, and false where
// there is not exactly one or where it binds a key.
//
// A key is declined rather than handled: over a list it is the position,
// and a list of one would say every iteration was the first. That is the
// kind of wrong that does not announce itself, so the loop goes to the
// resolver instead.
func onlyForClause(comp *ast.Comprehension) (*ast.ForClause, bool) {
	var found *ast.ForClause
	for _, clause := range comp.Clauses {
		f, is := clause.(*ast.ForClause)
		if !is {
			continue
		}
		if found != nil {
			return nil, false
		}
		found = f
	}
	if found == nil || found.Key != nil || found.Value == nil {
		return nil, false
	}
	return found, true
}

// loopOf reports the comprehension a field holds, where the field holds one
// and nothing else.
func loopOf(field *ast.Field) (*ast.Comprehension, bool) {
	s, ok := field.Value.(*ast.StructLit)
	if !ok || len(s.Elts) != 1 {
		return nil, false
	}
	comp, ok := s.Elts[0].(*ast.Comprehension)
	return comp, ok
}

// carriesAWholeLoop reports whether the closure would put a whole answered
// loop into every iteration's document.
//
// A stage that reads the stage before it reads one element of it, but the
// closure carries names and not elements, so without narrowing the
// document gets all of them: n documents of n answers, which is quadratic
// and slower than leaving the loop alone by a wide margin. Where the
// elements an iteration reads are known the name is cut down to those and
// this does not apply; where they are not, the loop is declined.
//
// wide is how many iterations this loop has, so the comparison is against
// the work being saved rather than against a number picked in advance.
func carriesAWholeLoop(scope fileScope, names []string, wide int, narrow map[string]map[string]bool) bool {
	if wide < 2 {
		return false
	}
	for _, name := range names {
		if _, narrowing := narrow[name]; narrowing {
			continue
		}
		for _, field := range scope.decls[name] {
			s, isStruct := field.Value.(*ast.StructLit)
			if isStruct && len(s.Elts) >= wide {
				return true
			}
		}
	}
	return false
}
