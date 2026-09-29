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
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/operators"

	"github.com/kubevela/pkg/cel/template"
)

// PropertyReferences returns every read an expression makes, against the shared
// permissive environment.
//
// The environment is the same for every caller resolving or scanning an
// expression, so building one per call site only invites them to differ.
func (e *Engine) PropertyReferences(expr string) ([]template.Reference, error) {
	return e.References(e.dyn, expr)
}

// References returns every read an expression makes.
//
// Walking the checked AST rather than the source text is what makes this exact:
// `source.cfg.data["image"]` and `source["cfg"].data.image` are the same read
// spelled two ways, and the parser has already normalised both into a select
// chain over an index.
//
// Reads inside a comprehension are included. A source read only in the untaken
// arm of a ternary is included too, deliberately: it still has to be resolved
// before the expression can be evaluated, and a value that might be substituted
// must count as sensitive whether or not this particular render reaches it.
func (e *Engine) References(env *cel.Env, expr string) ([]template.Reference, error) {
	c, err := e.compile(env, expr)
	if err != nil {
		return nil, err
	}
	ast := c.ast
	if err := e.checkQualifierCalls(ast.NativeRep().Expr()); err != nil {
		return nil, err
	}

	seen := map[string]template.Reference{}
	nav := celast.NavigateAST(ast.NativeRep())
	for _, n := range celast.MatchDescendants(nav, func(x celast.NavigableExpr) bool {
		// Only the outermost node of a chain: `source.cfg.meta.region` is one read,
		// not also `source.cfg` and `source.cfg.meta`. A parent written as a read
		// of its own, beside a child, is outermost in its own chain and counts.
		//
		// A bare identifier counts: `$(source)` reads the whole root, and root
		// validation has to see it to refuse it on a surface without that root.
		kind := x.Kind()
		return (kind == celast.SelectKind || kind == celast.CallKind || kind == celast.IdentKind) && !e.continuesChain(x)
	}) {
		root, path, ok := e.pathOf(n)
		if !ok || !e.isRoot[root] || shadowed(n, root) {
			continue
		}
		r := template.Reference{Root: root, Path: path, Defaulted: e.guarded(n, root, path)}
		if prev, dup := seen[r.String()]; !dup || (prev.Defaulted && !r.Defaulted) {
			seen[r.String()] = r
		}
	}

	out := make([]template.Reference, 0, len(seen))
	for _, r := range seen {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, nil
}

// continuesChain reports whether x is an inner link of a read chain: the operand
// of a select, the container of an index, or the receiver of a qualifier.
func (e *Engine) continuesChain(x celast.NavigableExpr) bool {
	p, ok := x.Parent()
	if !ok {
		return false
	}
	switch p.Kind() {
	case celast.SelectKind:
		return p.AsSelect().Operand().ID() == x.ID()
	case celast.CallKind:
		call := p.AsCall()
		if call.FunctionName() == operators.Index && len(call.Args()) == 2 {
			return call.Args()[0].ID() == x.ID()
		}
		if _, ok := e.qualifier(call); ok {
			return call.Target().ID() == x.ID()
		}
	}
	return false
}

// shadowed reports whether the root a read starts from is a comprehension's
// iteration variable of the same name rather than the root.
func shadowed(read celast.NavigableExpr, root string) bool {
	child := celast.Expr(read)
	for p, ok := read.Parent(); ok; p, ok = p.Parent() {
		if p.Kind() == celast.ComprehensionKind {
			comp := p.AsComprehension()
			inLoop := child.ID() == comp.LoopCondition().ID() || child.ID() == comp.LoopStep().ID()
			if inLoop && comp.IterVar() == root {
				return true
			}
		}
		child = p
	}
	return false
}

// guarded reports whether this specific read is defended against absence.
//
// Three things have to hold. Checking only the last is wrong in the unsafe
// direction:
//
//  1. The guard must test *this* path. `has(source.cfg.other) ? source.cfg.note
//     : "x"` defends nothing about note, and reading it still fails at render.
//  2. The read must sit in an arm, not in a condition - a condition is always
//     evaluated. Nesting matters: a read in the condition of an inner ternary is
//     unguarded even though that inner ternary sits in an outer one's arm.
//  3. The condition must prove the path present whenever the read's arm runs:
//     `has(x) ? x : d` and `!has(x) ? d : x` do; `has(x) ? d : x` does not.
//
// Both `cond ? a : b` and `has(x) && ...` count, because CEL's logical operators
// absorb an error from one side when the other side settles the result.
func (e *Engine) guarded(read celast.NavigableExpr, root string, path []string) bool {
	// A presence test on the read itself never fails.
	if read.Kind() == celast.SelectKind && read.AsSelect().IsTestOnly() {
		return true
	}
	child := celast.Expr(read)
	for p, ok := read.Parent(); ok; p, ok = p.Parent() {
		if p.Kind() == celast.CallKind {
			call := p.AsCall()
			args := call.Args()
			switch call.FunctionName() {
			case "_?_:_":
				if len(args) != 3 {
					break
				}
				// In the condition: not guarded by this ternary, and not by any
				// outer one either - the condition is evaluated regardless of
				// what encloses it.
				if args[0].ID() == child.ID() {
					return false
				}
				// The true arm runs when the condition holds, the false arm when
				// it does not.
				if e.proves(args[0], args[1].ID() == child.ID(), root, path) {
					return true
				}
			case "_&&_", "_||_":
				// && and || are duals, and only one absorbs the error each way
				// round. `has(x) && read(x)` is safe because a false has()
				// settles the conjunction, so the read matters only when the
				// other side is true; for ||, only when it is false.
				whenOtherIs := call.FunctionName() == "_&&_"
				for _, a := range args {
					if a.ID() != child.ID() && e.proves(a, whenOtherIs, root, path) {
						return true
					}
				}
			}
		}
		child = p
	}
	return false
}

// proves reports whether cond evaluating to value guarantees the path is
// present: a presence test for it, a negation, or a conjunction (or, when
// false, a disjunction) with a side that proves it.
func (e *Engine) proves(cond celast.Expr, value bool, root string, path []string) bool {
	if e.testsPath(cond, root, path) {
		return value
	}
	if cond.Kind() != celast.CallKind {
		return false
	}
	call := cond.AsCall()
	args := call.Args()
	switch call.FunctionName() {
	case operators.LogicalNot:
		return len(args) == 1 && e.proves(args[0], !value, root, path)
	case operators.LogicalAnd:
		// Both sides hold when it is true; either may not when it is false.
		return value && len(args) == 2 && (e.proves(args[0], true, root, path) || e.proves(args[1], true, root, path))
	case operators.LogicalOr:
		return !value && len(args) == 2 && (e.proves(args[0], false, root, path) || e.proves(args[1], false, root, path))
	}
	return false
}

// testsPath reports whether an expression is a presence test for this exact
// path.
//
// CEL spells that two ways, and both are needed. has(x.y) covers a declared field,
// but its macro rejects an index: has(m["k"]) does not compile. A map key is
// therefore tested with `"k" in m`, and that is the only form available when the
// key is not an identifier - which is every domain-prefixed label,
// context.appLabels["platform.io/team"] among them.
func (e *Engine) testsPath(expr celast.Expr, root string, path []string) bool {
	// has(x.y)
	if expr.Kind() == celast.SelectKind && expr.AsSelect().IsTestOnly() {
		r, p, ok := e.pathOf(expr)
		return ok && r == root && samePath(p, path)
	}
	// "k" in m - the container plus the key is the path being tested.
	if expr.Kind() != celast.CallKind {
		return false
	}
	call := expr.AsCall()
	if call.FunctionName() != operators.In && call.FunctionName() != operators.OldIn {
		return false
	}
	args := call.Args()
	if len(args) != 2 || args[0].Kind() != celast.LiteralKind {
		return false
	}
	key, ok := args[0].AsLiteral().Value().(string)
	if !ok {
		return false
	}
	r, p, ok := e.pathOf(args[1])
	return ok && r == root && samePath(append(append([]string{}, p...), key), path)
}

func samePath(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// pathOf flattens a select/index chain into its root identifier and path, over a
// plain Expr rather than a NavigableExpr, so it can be used inside a visitor.
func (e *Engine) pathOf(expr celast.Expr) (string, []string, bool) {
	var path []string
	cur := expr
	for {
		switch cur.Kind() {
		case celast.SelectKind:
			sel := cur.AsSelect()
			path = append([]string{sel.FieldName()}, path...)
			cur = sel.Operand()
		case celast.CallKind:
			call := cur.AsCall()
			if q, ok := e.qualifier(call); ok {
				path = append([]string{template.QualifierSegment(q)}, path...)
				cur = call.Target()
				continue
			}
			if call.FunctionName() != "_[_]" || len(call.Args()) != 2 {
				return "", nil, false
			}
			idx := call.Args()[1]
			if idx.Kind() != celast.LiteralKind {
				// A computed key: the container is what is read, and nothing
				// below it is known before evaluation.
				path = nil
				cur = call.Args()[0]
				continue
			}
			var segment string
			switch lit := idx.AsLiteral().Value().(type) {
			case string:
				segment = lit
			case int64:
				segment = strconv.FormatInt(lit, 10)
			case uint64:
				segment = strconv.FormatUint(lit, 10)
			default:
				return "", nil, false
			}
			path = append([]string{segment}, path...)
			cur = call.Args()[0]
		case celast.IdentKind:
			return cur.AsIdent(), path, true
		default:
			return "", nil, false
		}
	}
}

// qualifier reads a qualifier call whose argument is a literal. Anything else is
// not a qualifier a read can be resolved against before evaluation.
func (e *Engine) qualifier(call celast.CallExpr) (string, bool) {
	if !call.IsMemberFunction() || len(call.Args()) != 1 || call.Args()[0].Kind() != celast.LiteralKind {
		return "", false
	}
	arg, ok := call.Args()[0].AsLiteral().Value().(string)
	if !ok {
		return "", false
	}
	fn := call.FunctionName()
	if _, ok := e.qualifiers[fn]; !ok {
		return "", false
	}
	return template.Call(fn, arg), true
}

// checkQualifierCalls refuses what a read path cannot carry: a qualifier called
// on anything but a read of a root that declares it, or with anything but a
// literal string, which could only fail when evaluated; and a key containing a
// NUL character, which is how a qualifier is marked inside a read path.
func (e *Engine) checkQualifierCalls(expr celast.Expr) error {
	var err error
	celast.PostOrderVisit(expr, celast.NewExprVisitor(func(n celast.Expr) {
		if err != nil {
			return
		}
		if n.Kind() != celast.CallKind {
			return
		}
		call := n.AsCall()
		args := call.Args()
		switch fn := call.FunctionName(); {
		case (fn == operators.Index && len(args) == 2 && nulKey(args[1])) ||
			((fn == operators.In || fn == operators.OldIn) && len(args) == 2 && nulKey(args[0])):
			err = fmt.Errorf("a key containing a NUL character cannot be read")
		case call.IsMemberFunction() && e.declaredOn[fn] != nil:
			roots := e.declaredOn[fn]
			root, _, ok := e.pathOf(call.Target())
			if !ok || !contains(roots, root) {
				err = fmt.Errorf("%s() goes only on a %s read", fn, strings.Join(roots, " or "))
				return
			}
			if !e.placedAt(call.Target(), root) {
				place := root + strings.Repeat(".<name>", e.qualifiedAt[root])
				err = fmt.Errorf("%s go straight after %s: %s.%s(\"...\")",
					listOf(e.rootQualifiers[root]), place, place, fn)
				return
			}
			if _, ok := e.qualifier(call); !ok {
				err = fmt.Errorf("%s() takes a literal string, so the read is known before evaluation", fn)
				return
			}
			if nulKey(args[0]) {
				err = fmt.Errorf("a %s() argument containing a NUL character cannot be read", fn)
			}
		}
	}))
	return err
}

func nulKey(key celast.Expr) bool {
	if key.Kind() != celast.LiteralKind {
		return false
	}
	s, ok := key.AsLiteral().Value().(string)
	return ok && strings.ContainsRune(s, 0)
}

// placedAt reports whether a qualifier called on target is at its root's
// qualifier depth: after exactly that many literal segments, or after another
// qualifier that is. A computed segment names no entry a resolver could deliver.
func (e *Engine) placedAt(target celast.Expr, root string) bool {
	cur := target
	for cur.Kind() == celast.CallKind {
		call := cur.AsCall()
		if !call.IsMemberFunction() || e.declaredOn[call.FunctionName()] == nil {
			break
		}
		cur = call.Target()
	}
	depth := 0
	for {
		switch cur.Kind() {
		case celast.SelectKind:
			depth++
			cur = cur.AsSelect().Operand()
		case celast.CallKind:
			call := cur.AsCall()
			if call.FunctionName() != operators.Index || len(call.Args()) != 2 ||
				call.Args()[1].Kind() != celast.LiteralKind {
				return false
			}
			if _, ok := call.Args()[1].AsLiteral().Value().(string); !ok {
				return false
			}
			depth++
			cur = call.Args()[0]
		case celast.IdentKind:
			return cur.AsIdent() == root && depth == e.qualifiedAt[root]
		default:
			return false
		}
	}
}

// listOf joins names for a message: "a", "a and b", "a, b and c".
func listOf(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
