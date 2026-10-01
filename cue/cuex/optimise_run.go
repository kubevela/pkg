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
	"context"
	"errors"
	"strings"
	"sync/atomic"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
)

// Running a loop of calls an iteration at a time, before CUE is given the
// template at all.
//
// The resolver expands the loop and then answers the calls it finds, so
// every call it has answered is in the value while it answers the rest,
// and the whole loop is in memory at once. This answers them one at a
// time, each in a file holding only what that iteration reads, and hands
// CUE a template with the answers already in it.
//
// It refuses far more readily than it works. Anything it cannot account
// for leaves the loop exactly as the template wrote it, for the resolver
// to handle as it always has, so the worst this can do is nothing.

// returnsKey is where a provider function's answer lands.
const returnsKey = "$returns"

// OptimiseStats counts what the prepass has done since the process
// started: how many loops it took and how many calls it answered, against
// how many loops it saw and left alone.
//
// A prepass that declines everything behaves exactly like one that works,
// so something has to say which it is doing. These are the counters a test
// reads to know its corpus proved anything, and the numbers worth putting
// on a controller.
var OptimiseStats struct {
	Loops, Calls, Declined atomic.Int64
}

// report is why each loop of one file was left alone, keyed by the field
// it sits under.
//
// By field, because a template's last loop is usually the comprehension
// that reads the answers rather than one of the loops that made them, so
// one reason for the file says why the wrong loop was refused.
//
// Carried down the call rather than kept in a package variable. A
// controller renders several applications at once, and a map written
// from every render is a concurrent map write, which ends the process
// rather than merely confusing a test.
type report struct {
	// why is the reason the loop under way was refused, set as it is
	// found and read when the loop gives up.
	why string
	// declined is the reason per field, for a test that needs to tell
	// one loop's refusal from another's.
	declined map[string]string
}

func (r *report) note(reason string) {
	if r != nil {
		r.why = reason
	}
}

func (r *report) took(name string) {
	if r != nil && r.declined != nil {
		delete(r.declined, name)
	}
}

func (r *report) refused(name string) {
	if r == nil {
		return
	}
	if r.declined == nil {
		r.declined = map[string]string{}
	}
	r.declined[name] = r.why
}

// optimised is what a prepass made of a template.
type optimised struct {
	// declined is why each loop it did not take was left alone.
	declined map[string]string
	// file is the template with each loop it took replaced by the answers.
	file *ast.File
	// loops is how many loops it took, and calls how many it answered, so a
	// caller can say whether it did anything.
	loops, calls int
}

// prepass answers what it can of a template's annotated loops. It reports
// false where it took nothing, in which case the template is unchanged and
// the resolver has everything still to do.
func (in *Compiler) prepass(
	ctx context.Context,
	src string,
	imports []*build.Instance,
	policy OptimisePolicy,
) (optimised, bool) {
	f, err := parser.ParseFile("-", src, parser.ParseComments)
	if err != nil {
		return optimised{}, false
	}
	rep := &report{declined: map[string]string{}}
	calls, failed, took := in.prepassFile(ctx, f, imports, policy, cuecontext.New(), rep)
	if failed != nil || !took {
		return optimised{declined: rep.declined}, false
	}
	return optimised{file: f, loops: 1, calls: calls, declined: rep.declined}, true
}

// prepassFile answers what it can of a file's loops, in place. It reports
// how many calls it answered and whether it took anything at all.
func (in *Compiler) prepassFile(
	ctx context.Context,
	f *ast.File,
	imports []*build.Instance,
	policy OptimisePolicy,
	reading *cue.Context,
	rep *report,
) (int, error, bool) {
	if !policy.Enabled {
		return 0, nil, false
	}
	// Loops come in waves. A second stage reads the answers of the first,
	// so its parameters are not concrete until the first has been
	// answered, and one pass over the file would take the first and
	// decline the rest. Passes are repeated while any make progress,
	// which is the same shape the resolver's rounds have and for the same
	// reason.
	total := 0
	for {
		n, failed, took := in.prepassRound(ctx, f, imports, policy, reading, rep)
		if failed != nil {
			return 0, failed, false
		}
		if !took {
			break
		}
		total += n
		// A round replaced what was under a field, and every reference the
		// parser resolved to it still points at what used to be there. The
		// next round copies those references into the documents it builds,
		// so the file is written out and read back before it runs again.
		bs, err := format.Node(onItsOwnLine(f))
		if err != nil {
			break
		}
		reparsed, err := parser.ParseFile("-", string(bs), parser.ParseComments)
		if err != nil {
			break
		}
		*f = *reparsed
	}
	if total == 0 {
		return 0, nil, false
	}
	// Answering a loop can be the last use of a provider package, and CUE
	// will not build a file that imports something it does not use.
	pruneImports(f)
	onItsOwnLine(f)
	return total, nil, true
}

// prepassRound answers what it can of a file's loops in one pass over it.
func (in *Compiler) prepassRound(
	ctx context.Context,
	f *ast.File,
	imports []*build.Instance,
	policy OptimisePolicy,
	reading *cue.Context,
	rep *report,
) (int, error, bool) {
	scope, ok := scopeOfFile(f)
	if !ok {
		return 0, nil, false
	}
	if asksForAnOrder(f) {
		// A step attribute says what order the calls run in, and the
		// resolver sorts by it. This walks the file, so it would answer
		// the loops in the order they happen to be written, which is not
		// the same thing and is not something to get wrong quietly. Four
		// of the six thousand CUE files in this workspace use one.
		rep.note("file:asksForAnOrder")
		return 0, nil, false
	}
	providers := in.PackageManager.GetProviders()

	loops, calls := 0, 0
	for _, decl := range f.Decls {
		field, isField := decl.(*ast.Field)
		if !isField {
			continue
		}
		comp, holdsLoop := loopOf(field)
		if !holdsLoop {
			continue
		}
		if asksToRunTogether(field) {
			// The template asked for these to run at once and this runs
			// them one after another. On a provider that formats a string
			// that is a fair trade for the memory; on one that talks to a
			// cluster it is the whole cost of the render, and the wall
			// time of a thousand reads is the client's rate limiter and
			// nothing else. A loop that asked keeps what it asked for.
			OptimiseStats.Declined.Add(1)
			continue
		}
		rep.note("")
		loopName, _, _ := ast.LabelName(field.Label)
		answers, n, failed, took := in.answerLoop(
			ctx, scope, comp, imports, providers, policy, reading, loopName, rep)
		if failed != nil {
			// The call was made and it said no. Handing the loop back
			// would make every call in it a second time.
			rebind(f)
			return 0, failed, false
		}
		if name, _, err := ast.LabelName(field.Label); err == nil {
			if took {
				rep.took(name)
			} else {
				rep.refused(name)
			}
		}
		if !took {
			OptimiseStats.Declined.Add(1)
			continue
		}
		OptimiseStats.Loops.Add(1)
		OptimiseStats.Calls.Add(int64(n))
		field.Value = answers
		loops++
		calls += n
	}
	// Building the iteration documents unbound the declarations they
	// borrowed, which are the template's own nodes.
	rebind(f)
	if loops == 0 {
		return 0, nil, false
	}
	return calls, nil, true
}

// pruneImports drops the imports nothing in the file refers to any more.
//
// A file keeps its imports twice: in Decls, which is what it formats to,
// and in Imports, which is what building it reads. Dropping one and not
// the other gives a file that reads correctly and will not build.
func pruneImports(f *ast.File) {
	used := map[string]bool{}
	for _, decl := range f.Decls {
		if _, isImport := decl.(*ast.ImportDecl); isImport {
			continue
		}
		for _, name := range freeIdents(decl) {
			used[name] = true
		}
	}
	kept := make([]ast.Decl, 0, len(f.Decls))
	for _, decl := range f.Decls {
		imp, isImport := decl.(*ast.ImportDecl)
		if !isImport {
			kept = append(kept, decl)
			continue
		}
		specs := make([]*ast.ImportSpec, 0, len(imp.Specs))
		for _, spec := range imp.Specs {
			if used[importName(spec)] {
				specs = append(specs, spec)
			}
		}
		if len(specs) == 0 {
			continue
		}
		imp.Specs = specs
		kept = append(kept, imp)
	}
	f.Decls = kept

	stillImported := make([]*ast.ImportSpec, 0, len(f.Imports))
	for _, spec := range f.Imports {
		if used[importName(spec)] {
			stillImported = append(stillImported, spec)
		}
	}
	f.Imports = stillImported
}

// asksToRunTogether reports whether a field carries @concurrency, by
// which a template says its calls may overlap.
func asksToRunTogether(field *ast.Field) bool {
	for _, attr := range field.Attrs {
		if name, _ := attr.Split(); name == concurrencyKey {
			return true
		}
	}
	return false
}

// asksForAnOrder reports whether anything in the file carries a step
// attribute, which is how a template says what order its calls run in.
func asksForAnOrder(f *ast.File) bool {
	for _, decl := range f.Decls {
		field, isField := decl.(*ast.Field)
		if !isField {
			continue
		}
		for _, attr := range field.Attrs {
			if name, _ := attr.Split(); name == orderKey {
				return true
			}
		}
	}
	return false
}

// answerLoop answers every iteration of one loop and returns them as the
// struct the loop would have produced. It reports false wherever it cannot
// be sure, which leaves the loop for the resolver.
func (in *Compiler) answerLoop(
	ctx context.Context,
	scope fileScope,
	comp *ast.Comprehension,
	imports []*build.Instance,
	providers map[string]cuexruntime.Provider,
	policy OptimisePolicy,
	reading *cue.Context,
	loopName string,
	rep *report,
) (*ast.StructLit, int, error, bool) {
	forClause, ok := onlyForClause(comp)
	if !ok {
		rep.note("loop:onlyForClause")
		return nil, 0, nil, false
	}
	source, ok := in.sourceValues(scope, forClause.Source, imports, reading)
	if !ok {
		rep.note("loop:sourceValues")
		return nil, 0, nil, false
	}
	if !policy.allows(len(source)) {
		rep.note("loop:policy")
		return nil, 0, nil, false
	}

	// Names the loop only ever reads an element of can be cut down to the
	// element this iteration reads, which is what keeps a stage reading
	// the stage before it from carrying all of its answers.
	//
	// Asked of the whole comprehension and not just its body, because the
	// clauses are evaluated in the document too: a condition reading a
	// name whole, where the body only indexes it, decides which iterations
	// there are at all and has to see the whole of it.
	narrowable := map[string][]ast.Expr{}
	for name := range scope.decls {
		if indices, only := indexedOnly(comp, name); only {
			narrowable[name] = indices
		}
	}

	answers := &ast.StructLit{}
	answered := 0
	size := policy.batch()
	for from := 0; from < len(source); from += size {
		to := from + size
		if to > len(source) {
			to = len(source)
		}
		got, failed, ok := in.answerBatch(
			ctx, scope, comp, source[from:to], imports, providers, len(source),
			narrowable, reading, loopName, rep)
		if failed != nil {
			return nil, 0, failed, false
		}
		if !ok {
			return nil, 0, nil, false
		}
		for _, a := range got {
			answers.Elts = append(answers.Elts, &ast.Field{
				Label: ast.NewString(a.key),
				Value: a.value,
			})
			answered++
		}
	}
	return answers, answered, nil, true
}

// answered is one iteration's key and what it answered.
type answeredCall struct {
	key   string
	value ast.Expr
}

// sourceValues is what a loop iterates over, as expressions that can be
// put back into a file. It reports false where the source is not something
// this can enumerate before the template has been built.
func (in *Compiler) sourceValues(
	scope fileScope, source ast.Expr, imports []*build.Instance,
	reading *cue.Context,
) ([]ast.Expr, bool) {
	names, needed, ok := scope.closureOf(freeIdents(source), nil)
	if !ok {
		return nil, false
	}
	file := &ast.File{}
	if len(needed) > 0 {
		file.Decls = append(file.Decls, &ast.ImportDecl{Specs: needed})
	}
	for _, name := range names {
		for _, field := range scope.decls[name] {
			file.Decls = append(file.Decls, field)
		}
	}
	file.Decls = append(file.Decls, &ast.Field{
		Label: ast.NewIdent(iterationField),
		Value: source,
	})

	v, ok := buildInside(reading, onItsOwnLine(file), imports)
	if !ok {
		return nil, false
	}
	list, err := v.LookupPath(cue.MakePath(cue.Hid(iterationField, "_"))).List()
	if err != nil {
		// a struct, or something not iterable before the render: leave it
		return nil, false
	}
	var out []ast.Expr
	for list.Next() {
		expr, ok := resultSyntax(nil, list.Value(), false)
		if !ok {
			return nil, false
		}
		out = append(out, expr)
	}
	return out, true
}

// answerIteration builds one iteration, calls its provider, and returns the
// key it was under and the answer to put there.
func (in *Compiler) answerBatch(
	ctx context.Context,
	scope fileScope,
	comp *ast.Comprehension,
	at []ast.Expr,
	imports []*build.Instance,
	providers map[string]cuexruntime.Provider,
	wide int,
	narrowable map[string][]ast.Expr,
	reading *cue.Context,
	loopName string,
	rep *report,
) ([]answeredCall, error, bool) {
	narrow := in.keysFor(scope, comp, at, imports, narrowable, reading)
	doc, built := iterationDoc(scope, comp, at, nil, wide, narrow, rep)
	if !built {
		return nil, nil, false
	}
	v, built := buildIn(doc, imports)
	if !built {
		rep.note("buildIn")
		return nil, nil, false
	}
	it, err := v.LookupPath(cue.MakePath(cue.Hid(iterationField, "_"))).Fields(cue.All())
	if err != nil {
		rep.note("fields")
		return nil, nil, false
	}

	// Every iteration is read before any of them is called, because a
	// loop is taken or left whole. One of them that cannot be answered
	// used to be found after the ones before it had already called their
	// provider, and the answers were then dropped and the loop handed
	// back, so the resolver called those providers again.
	//
	// A run can answer fewer than it was given: a condition the loop
	// carries excludes an iteration, and the template never wanted a
	// field for it either.
	var planned []plannedCall
	for it.Next() {
		key, node := it.Selector().Unquoted(), it.Value()
		p, ok := in.planOne(node, providers, loopPath(loopName, key), key, rep)
		if !ok {
			return nil, nil, false
		}
		planned = append(planned, p)
	}

	var out []answeredCall
	for _, p := range planned {
		answer, failed := in.runPlanned(ctx, p, rep)
		if failed != nil {
			return nil, failed, false
		}
		out = append(out, answeredCall{p.key, answer})
	}
	return out, nil, true
}

// plannedCall is an iteration that can be answered, with everything about
// it that can be settled before its provider runs.
type plannedCall struct {
	key    string
	call   pendingCall
	fn     cuexruntime.ProviderFn
	opaque bool
	params ast.Expr
}

// planOne reads what answering an iteration would need and reports false
// where it cannot be answered. It calls nothing, which is what lets a loop
// be declined for the cost of reading it.
func (in *Compiler) planOne(
	node cue.Value,
	providers map[string]cuexruntime.Provider,
	at cue.Path,
	key string,
	rep *report,
) (plannedCall, bool) {
	fn, _ := node.LookupPath(doPath).String()
	if fn == "" {
		// the loop makes something that is not a call, so there is nothing
		// here to answer and nothing to gain by taking it
		rep.note("notACall")
		return plannedCall{}, false
	}
	provider, _ := node.LookupPath(providerPath).String()
	call := pendingCall{
		path:     at,
		fill:     unconstrained(at),
		key:      at.String(),
		value:    node,
		fn:       fn,
		provider: provider,
	}
	if !paramsResolved(node) {
		// the parameters did not come out concrete on their own, so this
		// iteration reads something the document has not got
		rep.note("paramsNotConcrete")
		return plannedCall{}, false
	}
	provFn, err := providerFn(providers, call)
	if err != nil {
		rep.note("noProviderFn")
		return plannedCall{}, false
	}
	// What the resolver would have left at this path, which is the node a
	// provider hands back with the parameters it was called with beside
	// it. $params is an ordinary field and part of what a visible node
	// renders to, so dropping it changes the output of any template whose
	// loop is not hidden. #do and #provider are definitions and render to
	// nothing, so they are not put back.
	params, ok := resultSyntax(nil, node.LookupPath(cue.MakePath(cue.Str(paramsKey))), false)
	if !ok {
		rep.note("paramsLiteral")
		return plannedCall{}, false
	}
	// A provider that builds its own value can return a definition, and a
	// definition is how a result carries another call, so its answer is
	// written back whole. One handed a Go value cannot, and asking for
	// definitions there would put the call's own #do and #provider back
	// into the template.
	_, fromGo := provFn.(cuexruntime.ResultProviderFn)
	return plannedCall{
		key:    key,
		call:   call,
		fn:     provFn,
		opaque: !fromGo,
		params: params,
	}, true
}

// runPlanned makes the call and renders the answer to put in its place.
//
// Declining is not an option here: the provider has run, and handing the
// loop back would have the resolver run it again. So anything that goes
// wrong past this point is reported, which is what the resolver does with
// the same call.
func (in *Compiler) runPlanned(
	ctx context.Context,
	p plannedCall,
	rep *report,
) (ast.Expr, error) {
	call, at := p.call, p.call.path
	ret, err := callProvider(ctx, p.fn, call)
	if err != nil {
		// Reported, not declined. Declining hands the loop back and the
		// resolver runs every call in it again, including the ones that
		// already happened, so a render that fails halfway through a
		// hundred writes does them twice. The error is the one the
		// resolver would have given, same words and same path, because
		// it is the same call being made.
		rep.note("callFailed")
		// The error names where the call was, and where it was is the
		// document this built to answer it in. A template never wrote
		// that name and should not be shown it.
		if called, is := err.(FunctionCallError); is {
			called.Path = at.String()
			err = called
		}
		return nil, err
	}
	answer, ok := resultSyntax(call.value.Context(), ret, p.opaque)
	if !ok {
		rep.note("answerSyntax")
		return nil, FunctionCallError{
			Path: at.String(),
			Err:  errors.New("the provider's result cannot be written back as syntax"),
		}
	}
	// The answer already is the node, $returns and whatever else the
	// function wrote, so the parameters go into it rather than around it.
	filled, isStruct := answer.(*ast.StructLit)
	if !isStruct {
		rep.note("notAStruct")
		return nil, FunctionCallError{
			Path: at.String(),
			Err:  errors.New("the provider's result is not a struct"),
		}
	}
	// What is written back is an answered call, so it must not still read
	// as one asking to be made. An opaque answer is kept whole precisely
	// because a result can carry another call as a definition, and the
	// node's own #do and #provider are definitions too: left in, the
	// resolver finds them and makes every call a second time.
	filled.Elts = append([]ast.Decl{
		&ast.Field{Label: ast.NewString(paramsKey), Value: p.params},
	}, withoutCallLabels(filled.Elts)...)
	return filled, nil
}

// withoutCallLabels drops the #do and #provider a call names itself by,
// at the top level and nowhere deeper: a result that carries a call of its
// own keeps it.
func withoutCallLabels(elts []ast.Decl) []ast.Decl {
	out := make([]ast.Decl, 0, len(elts))
	for _, elt := range elts {
		if field, isField := elt.(*ast.Field); isField {
			if name, _, err := ast.LabelName(field.Label); err == nil &&
				(name == doKey || name == providerKey) {
				continue
			}
		}
		out = append(out, elt)
	}
	return out
}

// keysFor works out which elements of each narrowable name this iteration
// reads. A name it cannot work out is left out, which carries it whole and
// may then be declined for being too big: slower or refused, never wrong.
func (in *Compiler) keysFor(
	scope fileScope,
	comp *ast.Comprehension,
	at []ast.Expr,
	imports []*build.Instance,
	narrowable map[string][]ast.Expr,
	reading *cue.Context,
) map[string]map[string]bool {
	if len(narrowable) == 0 {
		return nil
	}
	forClause, ok := onlyForClause(comp)
	if !ok {
		return nil
	}
	loopVar := forClause.Value.Name

	// The keys most templates use are the iteration's own value written
	// out, and those need no evaluating. What is left over, if anything,
	// is worked out by building a document for it.
	out := map[string]map[string]bool{}
	rest := map[string][]ast.Expr{}
	for name, exprs := range narrowable {
		for _, idx := range exprs {
			keys, direct := make([]string, 0, len(at)), true
			for _, one := range at {
				key, ok := directKey(idx, loopVar, one)
				if !ok {
					direct = false
					break
				}
				keys = append(keys, key)
			}
			if !direct {
				rest[name] = append(rest[name], idx)
				continue
			}
			if out[name] == nil {
				out[name] = map[string]bool{}
			}
			for _, key := range keys {
				out[name][key] = true
			}
		}
	}
	if len(rest) == 0 {
		return out
	}

	narrowable = rest
	without := make(map[string]bool, len(narrowable))
	for name := range narrowable {
		without[name] = true
	}

	// every index expression in one document, so an iteration costs one
	// build to find its keys however many names it reads
	var indices []ast.Expr
	var order []string
	for name, exprs := range narrowable {
		for range exprs {
			order = append(order, name)
		}
		indices = append(indices, exprs...)
	}
	doc, ok := keysDoc(scope, comp, at, nil, indices, without)
	if !ok {
		return nil
	}
	v, ok := buildInside(reading, doc, imports)
	if !ok {
		return nil
	}
	// The keys document's body is a struct of one field rather than a
	// field with a computed label, so it merges into the holder and the
	// keys sit directly under it.
	// one list per iteration in the run, each holding that iteration's
	// keys in the order the index expressions were gathered
	perIteration, err := v.LookupPath(cue.MakePath(cue.Hid(iterationField, "_"))).Fields(cue.All())
	if err != nil {
		return nil
	}
	for perIteration.Next() {
		list, err := perIteration.Value().List()
		if err != nil {
			return nil
		}
		for i := 0; list.Next(); i++ {
			if i >= len(order) {
				return nil
			}
			key, err := list.Value().String()
			if err != nil {
				// an index that is not a string: this narrows by field
				// name and nothing else
				return nil
			}
			name := order[i]
			if out[name] == nil {
				out[name] = map[string]bool{}
			}
			out[name][key] = true
		}
	}
	return out
}

// loopPath is where the template put a call, which is what an error
// about it should name. The document it was answered in is this
// package's business and not something to report.
func loopPath(loopName, key string) cue.Path {
	if loopName == "" {
		return cue.MakePath(cue.Str(key))
	}
	if strings.HasPrefix(loopName, "#") || strings.HasPrefix(loopName, "_#") {
		return cue.MakePath(cue.Def(loopName), cue.Str(key))
	}
	if strings.HasPrefix(loopName, "_") {
		return cue.MakePath(cue.Hid(loopName, "_"), cue.Str(key))
	}
	return cue.MakePath(cue.Str(loopName), cue.Str(key))
}

// rebind binds a file's identifiers to the file they are in.
//
// The documents an iteration is built from hold declarations lifted out
// of the template, the same nodes and not copies, so unbinding one to
// build it also unbinds them in the template. Left like that the
// template no longer resolves, and the failure is a value that builds
// and will not marshal: "reference set to unknown node in AST". So the
// template is bound again after anything has been built from it.
func rebind(f *ast.File) {
	unbind(f)
	astutil.Resolve(f, func(_ token.Pos, _ string, _ ...interface{}) {})
}

// unbind forgets what the parser bound a file's identifiers to, so they
// can be bound again to the file they are in now.
func unbind(node ast.Node) {
	ast.Walk(node, func(n ast.Node) bool {
		if id, is := n.(*ast.Ident); is {
			id.Scope = nil
			id.Node = nil
		}
		return true
	}, nil)
}

// buildIn builds a file against the provider packages, in a context of its
// own so what it leaves behind goes with it.
//
// For the documents that hold answers. A context retains everything built
// in it, so one per run is what keeps the answers of a run from piling up,
// and that is the whole point of running in batches.
//
// Written out and read back first. These files are assembled from
// declarations taken out of the template and, where a field was narrowed
// to the elements an iteration reads, from nodes made here; either way the
// identifiers in them resolve to whatever the parser bound them to in the
// file they came from, which is not in the file being built. Reading the
// text back is what binds them to what is actually there.
func buildIn(file *ast.File, imports []*build.Instance) (cue.Value, bool) {
	return buildInside(cuecontext.New(), file, imports)
}

// buildInside is the same against a context the caller keeps.
//
// For the documents that hold no answers: what a loop iterates over, and
// the keys an iteration reads. A fresh context has to compile every package
// the document names before it can read it, and CUE's own count: a loop
// written over list.Range costs ninety microseconds to read in a new
// context and four in one that has read a loop already. The resolver pays
// that once for the render, so a context per loop is how the prepass came
// to cost more than it saved on a template with several small loops.
//
// Safe to share only because these documents hold a list of keys and
// nothing from a provider. Anything carrying answers wants buildIn.
func buildInside(cc *cue.Context, file *ast.File, imports []*build.Instance) (cue.Value, bool) {
	// The identifiers in these files were bound by the parser to nodes in
	// the template they came from, and narrowing replaced some of those
	// nodes outright. Resolving binds what is unbound and leaves what is
	// bound alone, so the old bindings are cleared first or they survive
	// and point at nothing this file has.
	//
	// The cheap way of doing what writing the file out and reading it back
	// does, which is a format and a parse for every iteration.
	unbind(file)
	astutil.Resolve(file, func(_ token.Pos, _ string, _ ...interface{}) {})
	bi := build.NewContext().NewInstance("", nil)
	bi.Imports = imports
	if err := bi.AddSyntax(file); err != nil {
		return cue.Value{}, false
	}
	v := cc.BuildInstance(bi)
	if v.Err() != nil {
		return cue.Value{}, false
	}
	return v, true
}
