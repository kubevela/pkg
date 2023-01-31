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

package util

import (
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/token"
)

// ToString stringify cue.Value with reference resolved
func ToString(v cue.Value, opts ...cue.Option) (string, error) {
	opts = append([]cue.Option{cue.Final(), cue.Docs(true), cue.All()}, opts...)
	return toString(v, opts...)
}

// ToRawString stringify cue.Value without resolving references
func ToRawString(v cue.Value, opts ...cue.Option) (string, error) {
	opts = append([]cue.Option{cue.Raw(), cue.Docs(true), cue.All()}, opts...)
	return toString(v, opts...)
}

func toString(v cue.Value, opts ...cue.Option) (string, error) {
	node := v.Syntax(opts...)
	node = _format(node)
	bs, err := format.Node(node, format.Simplify())
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bs)), nil
}

func _format(n ast.Node) ast.Node {
	switch x := n.(type) {
	case *ast.StructLit:
		var decls []ast.Decl
		for _, elt := range x.Elts {
			if _, ok := elt.(*ast.Ellipsis); ok {
				continue
			}
			decls = append(decls, elt)
		}
		return &ast.File{Decls: decls}
	case ast.Expr:
		ast.SetRelPos(x, token.NoSpace)
		return &ast.File{Decls: []ast.Decl{&ast.EmbedDecl{Expr: x}}}
	default:
		return x
	}
}
