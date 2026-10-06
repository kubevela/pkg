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
	"cuelang.org/go/cue/token"
)

// Carrying one element of a field instead of all of it.
//
// A stage that reads the stage before it reads one answer of it, but a
// closure carries names rather than elements, so every iteration is given
// every answer the earlier stage gave. That is n documents of n answers,
// and it made a chain of four stages fifteen times slower than leaving it
// alone.
//
// Where a name is only ever read as name[something], the something can be
// worked out for the iteration in hand and the document given that one
// element. Where the name is read any other way, even once, it is carried
// whole: a field read as a whole is a field whose whole value matters, and
// there is no narrowing that is safe.

// indexedOnly reports the index expressions a name is read with, and false
// where the name is read any other way.
//
// Every mention has to be accounted for. A bare reference, a selector, a
// name passed somewhere this cannot see into: any of them means the whole
// value is wanted, and narrowing would hand over less than the template
// asked for.
func indexedOnly(node ast.Node, name string) ([]ast.Expr, bool) {
	var indices []ast.Expr
	indexed := map[*ast.Ident]bool{}
	ast.Walk(node, func(n ast.Node) bool {
		if ix, is := n.(*ast.IndexExpr); is {
			if id, isIdent := ix.X.(*ast.Ident); isIdent && id.Name == name {
				indexed[id] = true
				indices = append(indices, ix.Index)
			}
		}
		return true
	}, nil)

	mentions := 0
	ast.Walk(node, func(n ast.Node) bool {
		if id, is := n.(*ast.Ident); is && id.Name == name {
			mentions++
			if !indexed[id] {
				// read some other way, so the whole of it is wanted
				indices = nil
			}
		}
		return true
	}, nil)
	if indices == nil || mentions != len(indexed) {
		return nil, false
	}
	return indices, true
}

// narrowTo is the declarations of a name cut down to the keys given, where
// every declaration of it is a struct and every key is in one of them.
//
// It reports false where a key is not there, which is a template reading
// something that does not exist. That is the resolver's error to report,
// with its own path and its own words, so this stands aside.
func narrowTo(decls []*ast.Field, keys map[string]bool) ([]*ast.Field, bool) {
	found := map[string]bool{}
	out := make([]*ast.Field, 0, len(decls))
	for _, decl := range decls {
		s, isStruct := decl.Value.(*ast.StructLit)
		if !isStruct {
			return nil, false
		}
		kept := &ast.StructLit{}
		for _, elt := range s.Elts {
			field, isField := elt.(*ast.Field)
			if !isField {
				// an embedding or a comprehension still in there: this
				// cannot say which keys it makes
				return nil, false
			}
			label, _, err := ast.LabelName(field.Label)
			if err != nil {
				return nil, false
			}
			if !keys[label] {
				continue
			}
			found[label] = true
			kept.Elts = append(kept.Elts, field)
		}
		// The declaration as the template wrote it, with only its value cut
		// down. Built from the label alone it lost whether the field was
		// optional and any attribute on it, and an optional field carried
		// over as a required one is concrete in the document where the
		// template never said it was.
		narrowed := *decl
		narrowed.Value = kept
		out = append(out, &narrowed)
	}
	for key := range keys {
		if !found[key] {
			return nil, false
		}
	}
	return out, true
}

// directKey is the key an index expression comes to for one iteration,
// where that can be said without evaluating anything.
//
// "\(i)" over a loop bound to i is how nearly every template keys a loop
// by what it is looping over, and it is the iteration's own value written
// out. Recognising it saves building a document per iteration purely to be
// told that.
//
// Anything else returns false and is worked out the general way.
func directKey(index ast.Expr, loopVar string, at ast.Expr) (string, bool) {
	interp, is := index.(*ast.Interpolation)
	if !is || len(interp.Elts) != 3 {
		return "", false
	}
	opening, isLit := interp.Elts[0].(*ast.BasicLit)
	if !isLit || opening.Value != `"\(` {
		return "", false
	}
	closing, isLit := interp.Elts[2].(*ast.BasicLit)
	if !isLit || closing.Value != `)"` {
		return "", false
	}
	id, isIdent := interp.Elts[1].(*ast.Ident)
	if !isIdent || id.Name != loopVar {
		return "", false
	}
	return literalText(at)
}

// literalText is what a literal reads as inside an interpolation: a number
// as written, a string without its quotes.
func literalText(at ast.Expr) (string, bool) {
	lit, is := at.(*ast.BasicLit)
	if !is {
		return "", false
	}
	switch lit.Kind {
	case token.INT, token.FLOAT:
		return lit.Value, true
	case token.STRING:
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			return "", false
		}
		return s, true
	default:
		return "", false
	}
}
