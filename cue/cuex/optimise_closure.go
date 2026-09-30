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
)

// Everything an expression needs, gathered from the file around it.
//
// freeIdents says what an expression reads. This follows those names to
// their declarations, and those declarations' names to theirs, until
// nothing new is reached. What comes back is enough of the file to
// evaluate the expression and no more.
//
// Two rules keep it from being quietly wrong.
//
// A name is every declaration of it, not the first. CUE unifies a file, so
// cfg written twice is one field with both halves, and carrying one half
// gives parameters that are concrete and short of what the template said.
//
// A name that cannot be accounted for stops the whole thing. Not a
// declaration, not an import, not something CUE declares itself: then this
// does not know what it is, and guessing is exactly the failure this is
// built to avoid.

// predeclared are the names CUE provides, which need no declaration
// carrying and are not a reason to decline.
var predeclared = map[string]bool{
	"bool": true, "int": true, "float": true, "string": true,
	"bytes": true, "number": true, "null": true, "_": true,
	"true": true, "false": true,
	"len": true, "close": true, "and": true, "or": true,
	"div": true, "mod": true, "quo": true, "rem": true,
}

// fileScope is a file's top level, indexed so a name can be looked up.
type fileScope struct {
	// decls holds every declaration of a name, in the order written.
	decls map[string][]*ast.Field
	// order is the names in the order the file first declares them, so what
	// is carried keeps the file's shape.
	order []string
	// imports are by the name they are referred to as, which is the alias
	// where there is one.
	imports map[string]*ast.ImportSpec
}

// scopeOfFile indexes a file's top level. It reports false where the top
// level holds something that cannot be named, since a closure over a file
// with an unnamed part in it cannot be shown to be complete.
func scopeOfFile(f *ast.File) (fileScope, bool) {
	s := fileScope{
		decls:   map[string][]*ast.Field{},
		imports: map[string]*ast.ImportSpec{},
	}
	for _, d := range f.Decls {
		switch decl := d.(type) {
		case *ast.Package, *ast.CommentGroup, *ast.Attribute:
			continue
		case *ast.ImportDecl:
			for _, spec := range decl.Specs {
				s.imports[importName(spec)] = spec
			}
		case *ast.Field:
			name, _, err := ast.LabelName(decl.Label)
			if err != nil || name == "" {
				return fileScope{}, false
			}
			if _, seen := s.decls[name]; !seen {
				s.order = append(s.order, name)
			}
			s.decls[name] = append(s.decls[name], decl)
		default:
			// an embedding or a top-level let: it contributes to the file
			// in a way this cannot name, so no closure over it is provable
			return fileScope{}, false
		}
	}
	return s, true
}

// importName is what an import is referred to as: its alias where it has
// one, otherwise the last element of its path.
func importName(spec *ast.ImportSpec) string {
	if spec.Name != nil {
		return spec.Name.Name
	}
	path, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		return ""
	}
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

// closureOf follows seeds to their declarations and those to theirs. It
// returns the names to carry in the file's own order, the imports they
// need, and false where a name cannot be accounted for.
//
// skip is the names not to follow: the loop variables, which are bound per
// iteration rather than declared, and the field being replaced.
func (s fileScope) closureOf(seeds []string, skip map[string]bool) ([]string, []*ast.ImportSpec, bool) {
	wanted := map[string]bool{}
	var order []string
	var imports []*ast.ImportSpec
	seenImport := map[string]bool{}

	queue := append([]string(nil), seeds...)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		switch {
		case skip[name], predeclared[name], wanted[name]:
			continue
		}
		if spec, isImport := s.imports[name]; isImport {
			if !seenImport[name] {
				seenImport[name] = true
				imports = append(imports, spec)
			}
			continue
		}
		fields, declared := s.decls[name]
		if !declared {
			// Not a declaration, not an import, not CUE's own. This does
			// not know what it is and will not carry a file it cannot show
			// to be complete.
			return nil, nil, false
		}
		wanted[name] = true
		order = append(order, name)
		// every declaration of the name, not the first
		for _, field := range fields {
			queue = append(queue, freeIdents(field.Value)...)
		}
	}

	// back into the order the file declares them, so what is carried reads
	// like the file it came from
	inFileOrder := make([]string, 0, len(order))
	for _, name := range s.order {
		if wanted[name] {
			inFileOrder = append(inFileOrder, name)
		}
	}
	return inFileOrder, imports, true
}
