/*
Copyright 2022 The KubeVela Authors.

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
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"github.com/spf13/pflag"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	"github.com/kubevela/pkg/cue/cuex/providers/base64"
	cueext "github.com/kubevela/pkg/cue/cuex/providers/cue"
	"github.com/kubevela/pkg/cue/cuex/providers/http"
	"github.com/kubevela/pkg/cue/cuex/providers/kube"
	cuexutil "github.com/kubevela/pkg/cue/cuex/providers/util"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/cue/util"
	"github.com/kubevela/pkg/util/runtime"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/kubevela/pkg/util/slices"
)

const (
	doKey       = "#do"
	providerKey = "#provider"
	paramsKey   = "$params"
	// concurrencyKey is the attribute a template marks a set of calls with to
	// say they may run alongside each other: @concurrency(8)
	concurrencyKey = "concurrency"
	// orderKey is the attribute a template marks fields with to say what
	// order their calls run in: @step(1). Spelled the same in cue/util,
	// which is where the resolver sorts by it.
	orderKey = "step"
	// maxPerRenderKey caps how many of a function's calls one render may have
	// in flight: a ceiling on what a template asks for with @concurrency,
	// never a request on its own. It sits under #config, where the settings a
	// function declares about itself collect.
	//
	// It bounds one render, not the process. Renders run concurrently - the
	// application controller reconciles several at once - and each gets this
	// many, so a function declaring 8 can still see a multiple of that at the
	// far end. An endpoint needing a total limit has to hold one itself.
	maxPerRenderKey = "#config.maxPerRender"
)

// defaultConcurrency is what @concurrency asks for when it names no number.
// Unbounded would let a template open as many connections as it has calls.
var defaultConcurrency = goruntime.NumCPU()

// defaultMaxPerRender is the ceiling a function gets when it declares none:
// one call at a time, which is what the resolver has always done.
//
// Running calls together is safe only if the thing behind them says so, and
// that is not ours to assume. A provider function is marked in Go as safe to
// split, but splittable is not the same as safe to reorder: an endpoint may
// hold state, and vela/http covers POST and DELETE as readily as GET. So the
// number comes from the function's own CUE, written by whoever owns what is
// on the other end, and a function that says nothing runs as it did before.
var defaultMaxPerRender = 1

// parsed once: these were re-parsed for every node of every walk
var (
	doPath           = cue.ParsePath(doKey)
	providerPath     = cue.ParsePath(providerKey)
	paramsPath       = cue.ParsePath(paramsKey)
	maxPerRenderPath = cue.ParsePath(maxPerRenderKey)
)

// Compiler for compile cue strings into cue.Value
type Compiler struct {
	*cuexruntime.PackageManager

	// whether a package's own CUE holds a comprehension, keyed by its import
	// path. An external package is rebuilt on every informer resync, so keying
	// by the built instance would add an entry every few minutes for the life
	// of the process and hold the old parse tree with it.
	packageComprehensions sync.Map
	// packageRevealGraphs holds what each package's syntax says about which
	// calls can reveal another; see revealers.
	packageRevealGraphs sync.Map
}

// CompileString compile given cue string into cue.Value
func (in *Compiler) CompileString(ctx context.Context, src string) (cue.Value, error) {
	return in.CompileStringWithOptions(ctx, src)
}

// CompileConfig config for running compile process
type CompileConfig struct {
	// Optimise says which loops the prepass may answer before the build.
	Optimise                 OptimisePolicy
	ResolveProviderFunctions bool
	PreResolveMutators       []func(context.Context, string) (string, error)
	IntraResolveMutators     []*withIntraResolveMutation
	Data                     []*withData
}

// WithOptimise sets which loops the prepass may answer before the build.
//
// Without it a compile uses the default, which permits a loop at or over
// the threshold. Permits and not takes: a loop asking for @concurrency
// keeps it, and one whose iterations cannot be enumerated or accounted for
// is left alone whatever its length.
func WithOptimise(p OptimisePolicy) CompileOption { return withOptimise{p} }

type withOptimise struct{ policy OptimisePolicy }

func (in withOptimise) ApplyTo(cfg *CompileConfig) { cfg.Optimise = in.policy }

// NewCompileConfig create new CompileConfig
func NewCompileConfig(opts ...CompileOption) *CompileConfig {
	cfg := &CompileConfig{
		ResolveProviderFunctions: true,
		Optimise:                 DefaultOptimisePolicy,
		PreResolveMutators:       nil,
		IntraResolveMutators:     make([]*withIntraResolveMutation, 0),
	}
	for _, opt := range opts {
		opt.ApplyTo(cfg)
	}
	return cfg
}

// CompileOption options for compile cue string
type CompileOption interface {
	ApplyTo(*CompileConfig)
}

// WithExtraData fill the cue.Value before resolve
func WithExtraData(key string, data interface{}) CompileOption {
	return &withExtraData{
		key:  key,
		data: data,
	}
}

type withExtraData struct {
	key  string
	data interface{}
}

// ApplyTo .
func (in *withExtraData) ApplyTo(cfg *CompileConfig) {
	cfg.PreResolveMutators = append(cfg.PreResolveMutators, func(_ context.Context, template string) (string, error) {
		val, path := cuecontext.New().CompileString(""), cue.ParsePath(in.key)
		if runtime.IsNil(in.data) {
			val = val.FillPath(path, struct{}{})
		} else {
			val = val.FillPath(path, in.data)
		}
		data, err := util.ToString(val)
		return strings.Join([]string{template, data}, "\n"), err
	})
}

// WithData supplies a value for a field the template declares.
//
// WithExtraData reaches the template through its source: it builds the value,
// exports it to CUE text, and appends that to the template for the parser to
// read back. For a parameter block of any size that round trip costs several
// times the rest of the compile. WithData fills the value into the compiled
// cue.Value instead.
//
// A template that does not declare the field cannot be filled that way - the
// reference would not resolve and the build would fail - so those fall back to
// appending source, and cost what WithExtraData costs.
func WithData(key string, data interface{}) CompileOption {
	return &withData{
		key:  key,
		data: data,
	}
}

type withData struct {
	key  string
	data interface{}
}

// ApplyTo .
func (in *withData) ApplyTo(cfg *CompileConfig) {
	cfg.Data = append(cfg.Data, in)
}

// fill puts the data into an already built value.
func (in *withData) fill(val cue.Value) cue.Value {
	path := cue.ParsePath(in.key)
	if runtime.IsNil(in.data) {
		return val.FillPath(path, struct{}{})
	}
	return val.FillPath(path, in.data)
}

// source renders the data as CUE text, for a template that has nowhere to
// fill it. This is what WithExtraData does for every value.
func (in *withData) source() (string, error) {
	val := in.fill(cuecontext.New().CompileString(""))
	return util.ToString(val)
}

// declaredIn reports whether the file declares the field this data is for, and
// so whether the built value will have somewhere to put it.
func (in *withData) declaredIn(f *ast.File) bool {
	path := cue.ParsePath(in.key)
	if path.Err() != nil {
		return false
	}
	sels := path.Selectors()
	if len(sels) == 0 {
		return false
	}
	want := sels[0].String()
	if sels[0].LabelType() == cue.StringLabel {
		want = sels[0].Unquoted()
	}
	for _, decl := range f.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		if name, _, err := ast.LabelName(field.Label); err == nil && name == want {
			return true
		}
	}
	return false
}

// WithIntraResolveMutation - Allows to add a mutation function to the compile process.
// This runs after the initial parsing and before the resolution of provider functions (CueX).
// This is required when provider function resolution needs access to dynamically read values, such as from Config.
func WithIntraResolveMutation(name string, fn func(ctx context.Context, value cue.Value) (cue.Value, error)) CompileOption {
	return &withIntraResolveMutation{
		mutation: fn,
		name:     name,
	}
}

type withIntraResolveMutation struct {
	name     string
	mutation func(ctx context.Context, value cue.Value) (cue.Value, error)
}

// ApplyTo .
func (in *withIntraResolveMutation) ApplyTo(cfg *CompileConfig) {
	cfg.IntraResolveMutators = append(cfg.IntraResolveMutators, in)
}

var _ CompileOption = DisableResolveProviderFunctions{}

// DisableResolveProviderFunctions disable ResolveProviderFunctions
type DisableResolveProviderFunctions struct{}

// ApplyTo .
func (in DisableResolveProviderFunctions) ApplyTo(cfg *CompileConfig) {
	cfg.ResolveProviderFunctions = false
}

// CompileStringWithOptions compile given cue string with extra options
func (in *Compiler) CompileStringWithOptions(ctx context.Context, src string, opts ...CompileOption) (cue.Value, error) {
	var err error
	cfg := NewCompileConfig(opts...)
	bi := build.NewContext().NewInstance("", nil)
	imports := in.PackageManager.GetImports()
	bi.Imports = imports
	for _, mutator := range cfg.PreResolveMutators {
		if src, err = mutator(ctx, src); err != nil {
			return cue.Value{}, err
		}
	}
	f, err := parser.ParseFile("-", src, parser.ParseComments)
	if err != nil {
		return cue.Value{}, err
	}
	fills, f, src, err := partitionData(cfg.Data, src, f)
	if err != nil {
		return cue.Value{}, err
	}
	// The context this render builds in, made here because the prepass
	// reads in it too. It has to compile CUE's own packages for the
	// template regardless, and that is most of what reading a loop's
	// source costs, so a context of the prepass's own would pay for them
	// twice. What it reads in here are lists of keys and nothing from a
	// provider; the documents holding answers still get one context each,
	// which is what buildIn is for.
	cc := cuecontext.New()
	// Loops of calls answered before CUE is handed the template at all,
	// where the prepass can show it knows what an iteration reads.
	// Anything it cannot account for it leaves alone, so what follows is
	// the same work it always was, over fewer calls.
	//
	// Behind the same question the resolve is behind. A template that
	// imports no provider package has no call to answer, and most
	// definitions are that, so there is nothing there worth parsing the
	// file to find out.
	//
	// And not at all while anything is still to be put into the value.
	// Both a fill and a mutation land below, after this has run, so a call
	// reading what they change would be answered from what the template
	// says on its own. Where that is a default the answer comes out
	// concrete and wrong rather than declining, which is the one way this
	// can be wrong rather than merely slower.
	//
	// The walk below says the same thing about itself two blocks down: a
	// mutation replaces the value outright and cannot be read from here.
	// It is the same argument and it applies with more force here, since
	// this answers calls rather than deciding how much to look at.
	settled := len(fills) == 0 && len(cfg.IntraResolveMutators) == 0
	if settled && cfg.ResolveProviderFunctions && in.mayContainCalls(src, f, cfg, imports) {
		loops, answered, failed, took := in.prepassFile(ctx, f, imports, cfg.Optimise, cc, nil)
		if failed != nil {
			// A call the prepass made and that said no. The resolver
			// would report the same thing, having made the same call,
			// and letting it try would make every call in that loop a
			// second time.
			return cue.Value{}, failed
		}
		if took {
			// A prepass that declines everything behaves exactly like one
			// that works, so a controller has no way of telling which it is
			// doing without this.
			klog.V(2).InfoS("cuex answered provider calls before the render",
				"loops", loops, "calls", answered)
			// The rewrite put new values under fields the parser had
			// already resolved references to, and those references still
			// point at what used to be there. Writing the file out and
			// reading it back is what makes them point at what is there
			// now.
			if bs, ferr := format.Node(f); ferr == nil {
				if reparsed, perr := parser.ParseFile("-", string(bs), parser.ParseComments); perr == nil {
					f, src = reparsed, string(bs)
				}
			}
		}
	}
	if err = bi.AddSyntax(f); err != nil {
		return cue.Value{}, err
	}
	val := cc.BuildInstance(bi)
	for _, fill := range fills {
		val = fill.fill(val)
	}
	for _, irm := range cfg.IntraResolveMutators {
		klog.V(1).Infof("Applying Intra Resolve Mutation: %s", irm.name)
		result, err := irm.mutation(ctx, val)
		if err != nil {
			klog.V(1).ErrorS(err, "Couldn't apply Intra Resolve Mutation: %s", irm.name)
			return val, err
		}
		val = result
	}
	if cfg.ResolveProviderFunctions && in.mayContainCalls(src, f, cfg, imports) {
		scope := callScope(nil)
		if !carriesValueData(cfg) && len(cfg.IntraResolveMutators) == 0 {
			// Both can put a call where the syntax does not name it: a
			// mutation replaces the value outright, and a cue.Value filled in
			// can carry anything. Neither can be read from here, so neither
			// gets a narrowed walk.
			scope = scopeOf(f, imports)
		}
		var analysis *analysisBuild
		if len(cfg.IntraResolveMutators) == 0 {
			// A mutation may do anything, and doing it twice is not ours to
			// decide, so a mutated value is read where it is run.
			analysis = &analysisBuild{build: func() cue.Value {
				v := cuecontext.New().BuildInstance(bi)
				for _, fill := range fills {
					v = fill.fill(v)
				}
				return v
			}}
		}
		mayReveal := in.mayRevealCalls(f, cfg, imports)
		reveal := &revealers{}
		if mayReveal {
			reveal = in.revealersOf(f, cfg, imports)
		}
		return in.resolve(ctx, val, mayReveal, reveal, scope, analysis)
	}
	return val, nil
}

// carriesValueData reports whether any value being filled in is a cue.Value.
// A Go value fills ordinary fields, and a field named "#do" among them is a
// string label rather than a definition, so it is not a call. A cue.Value can
// carry anything.
func carriesValueData(cfg *CompileConfig) bool {
	for _, data := range cfg.Data {
		switch data.data.(type) {
		case cue.Value:
			return true
		case ast.Node:
			// Syntax is compiled as CUE rather than converted as data, so
			// a "#do" in it is a definition and the field it is filled
			// into is a call the file's own syntax never named. The scope
			// would leave that field out and the call would be dropped
			// without a word.
			return true
		}
	}
	return false
}

// mayRevealCalls reports whether the value can grow calls it does not have at
// the end of a build.
//
// A comprehension can: its guard may read what a call produced, so the field
// it writes does not exist until that call has run.
//
//	a: base64.#Encode & {$params: "seed"}
//	if a.$returns != "" {b: base64.#Encode & {$params: a.$returns}}
//
// Without one, and with results that fill ordinary fields, the calls a build
// produced are all the calls there are, and the resolver can stop as soon as
// it has run them rather than searching the value again to be told so.
//
// The template is not the only place one can be. A provider package's own CUE
// is built into the value too, and a definition there can put a call under a
// comprehension or a computed label, so the packages the template imports are
// read as well as the template.
func (in *Compiler) mayRevealCalls(f *ast.File, cfg *CompileConfig, imports []*build.Instance) bool {
	// A mutation replaces the value wholesale and a cue.Value can carry
	// anything, so neither can be read from here - the same two the search for
	// calls already gives up on.
	if len(cfg.IntraResolveMutators) > 0 || carriesValueData(cfg) {
		return true
	}
	if mayGrowArcs(f) {
		return true
	}
	for _, imported := range imports {
		if in.packageMayGrowArcs(imported) {
			return true
		}
	}
	return false
}

// packageGrowth is what was read from a package, kept with the instance it was
// read from so a rebuilt package is read again rather than answered from what
// the old one said.
type packageGrowth struct {
	instance *build.Instance
	found    bool
}

// packageMayGrowArcs reads a package once and remembers the answer. The
// template changes every compile; the packages rarely do.
func (in *Compiler) packageMayGrowArcs(imported *build.Instance) bool {
	if imported == nil {
		return true
	}
	path := imported.ImportPath
	if path != "" {
		if known, ok := in.packageComprehensions.Load(path); ok {
			if read := known.(packageGrowth); read.instance == imported {
				return read.found
			}
		}
	}
	found := false
	for _, file := range imported.Files {
		if mayGrowArcs(file) {
			found = true
			break
		}
	}
	if path != "" {
		in.packageComprehensions.Store(path, packageGrowth{instance: imported, found: found})
	}
	return found
}

// mayGrowArcs reports whether a file can produce fields the build did not,
// and so whether running the calls already found can reveal more.
//
// Two things do it. A comprehension makes its body once whatever it iterates
// or guards on is known, and a call's output is often what that is. A label
// that has to be computed - "\(x)" or (x) - makes no arc at all until x is
// concrete, so a call underneath one is invisible until the call it names has
// run. Both leave the value looking finished while work remains.
func mayGrowArcs(f *ast.File) bool {
	found := false
	ast.Walk(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Comprehension:
			found = true
		case *ast.Field:
			switch node.Label.(type) {
			case *ast.Interpolation, *ast.ParenExpr:
				found = true
			}
		}
		return !found
	}, nil)
	return found
}

// mayContainCalls reports whether the value could hold a provider call.
//
// Resolving walks the whole value, and on a template that renders a manifest
// that costs more than compiling it did. Most component and trait definitions
// call no provider at all, and should not pay for the search.
//
// A call is a field named #do, and there are four ways one can be there: it is
// written in the source, a provider package brings it in, something fills it
// into the value after the build, or a mutation puts it there. The first two
// are read here; the other two are only assumed, because what they will do is
// not knowable from here.
//
// Say yes when unsure. Saying no wrongly means a call never runs, and nothing
// reports it.
func (in *Compiler) mayContainCalls(src string, f *ast.File, cfg *CompileConfig, imports []*build.Instance) bool {
	if len(cfg.IntraResolveMutators) > 0 {
		return true
	}
	if carriesValueData(cfg) {
		return true
	}
	if strings.Contains(src, doKey) {
		return true
	}
	// Only provider packages are importable: CUE's own are builtin and never
	// reach bi.Imports, so nothing else can bring a #do in. The instances were
	// built for the file already, so they are read rather than asked for again
	// - asking allocates the whole package list, and asking inside the loop
	// allocated it once per import.
	if len(f.Imports) == 0 {
		return false
	}
	for _, spec := range f.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return true
		}
		for _, imported := range imports {
			if imported != nil && imported.ImportPath == path {
				return true
			}
		}
	}
	return false
}

// partitionData splits the data between what the built value can be filled
// with and what has to reach the template as source. Only the second kind
// needs the file parsed again, so a template that declares its fields - which
// is nearly all of them - is parsed once.
func partitionData(data []*withData, src string, f *ast.File) ([]*withData, *ast.File, string, error) {
	if len(data) == 0 {
		return nil, f, src, nil
	}
	var (
		fills    []*withData
		appended []string
	)
	for _, d := range data {
		if d.declaredIn(f) {
			fills = append(fills, d)
			continue
		}
		text, err := d.source()
		if err != nil {
			return nil, nil, "", err
		}
		appended = append(appended, text)
	}
	if len(appended) == 0 {
		return fills, f, src, nil
	}
	src = strings.Join(append([]string{src}, appended...), "\n")
	reparsed, err := parser.ParseFile("-", src, parser.ParseComments)
	if err != nil {
		return nil, nil, "", err
	}
	return fills, reparsed, src, nil
}

// Resolve runs the resolve process by calling provider functions.
//
// The value is walked to find the calls in it, and those are then run in
// dependency order without walking it again: a call still to run is looked up
// by its path rather than searched for. The walk is repeated only to find
// calls that did not exist when it last ran, which a comprehension guarded on
// another call's output can produce.
//
// The previous implementation walked the whole value and re-unified it at the
// root once per call.
func (in *Compiler) Resolve(ctx context.Context, value cue.Value) (cue.Value, error) {
	// a caller holding only the value has no syntax to read, so it can say
	// neither whether the value has a comprehension in it nor where a call
	// could be. Both are assumed the expensive way.
	return in.resolve(ctx, value, true, &revealers{all: true}, nil, nil)
}

// resolve is Resolve, told whether the value can grow calls it does not have
// yet. See mayRevealCalls.
func (in *Compiler) resolve(ctx context.Context, value cue.Value, mayReveal bool, reveal *revealers, scope callScope, analysis *analysisBuild) (cue.Value, error) {
	newValue := value
	executed := map[string]bool{}
	waitingFor := &callWaits{of: map[string][]string{}, analysis: analysis}
	providers := in.PackageManager.GetProviders()
	for {
		if pastDeadline(ctx) {
			return newValue, ResolveTimeoutErr{}
		}
		pending := pendingCalls(newValue, executed, scope)
		if len(pending) == 0 {
			break
		}
		next, opaque, err := in.runRound(ctx, newValue, providers, pending, executed, waitingFor, reveal)
		if err != nil {
			return next, err
		}
		newValue = next
		if !mayReveal && !opaque {
			// Nothing can have appeared. The template has no comprehension to
			// reveal a call, and what was placed came back as Go values, which
			// fill ordinary fields - a "#do" among them is a string label and
			// not a definition, so it is not a call either. Walking again
			// would only confirm that, and on a template that renders a
			// manifest the walk costs more than the calls did.
			break
		}
	}
	return newValue, nil
}

// pendingCall is a provider function call found in the value but not yet run.
type pendingCall struct {
	// path locates the call in the value, and keys it in the executed set.
	path cue.Path
	// fill is path with any field constraint dropped, so applying the result
	// materialises an optional field the way filling the value directly did.
	fill     cue.Path
	key      string
	value    cue.Value
	fn       string
	provider string
	// concurrency is how many of a template's calls it will have running at
	// once here, 1 unless it asked for more.
	concurrency int
	// maxPerRender is the ceiling the function's own definition declared with
	// #config.maxPerRender, or 0 where it declared none.
	maxPerRender int
}

// pendingCalls collects every call in the value that has not run yet, in the
// order the walk finds them, which is the order the resolver used to run them.
func pendingCalls(value cue.Value, executed map[string]bool, scope callScope) []pendingCall {
	var pending []pendingCall
	look := func(v cue.Value) (stop bool) {
		// The walk reaches every node, and most of them are the strings and
		// numbers a manifest is made of. #do is a field, so only something
		// that can hold fields can be a call, and asking that is cheaper than
		// looking a path up on a leaf that has none.
		//
		// Bottom is not a leaf for this purpose. A node whose contents
		// conflict reports it, and a call whose own bookkeeping a template
		// has contradicted is exactly such a node: skipping it would drop the
		// call and say nothing, which is worse than the conflict.
		if kind := v.IncompleteKind(); kind != cue.BottomKind && kind&cue.StructKind == 0 {
			return false
		}
		// checked before the path, which most nodes then never need
		fn, _ := v.LookupPath(doPath).String()
		if fn == "" {
			return false
		}
		path := v.Path()
		key := path.String()
		if executed[key] {
			return false
		}
		prd, _ := v.LookupPath(providerPath).String()
		pending = append(pending, pendingCall{
			path:         path,
			fill:         unconstrained(path),
			key:          key,
			value:        v,
			fn:           fn,
			provider:     prd,
			concurrency:  concurrencyAt(value, path),
			maxPerRender: declaredMaxPerRender(v),
		})
		return false
	}
	if scope == nil {
		util.Iterate(value, look)
		return pending
	}
	// The syntax named the fields a call could be under, so the rest of the
	// value - which is the manifest being rendered, and most of the nodes -
	// is left alone.
	under, ok := scope.fields(value)
	if !ok {
		util.Iterate(value, look)
		return pending
	}
	// The fields are in the order the file declares them, and a template
	// that asked for an order with @step wants that one. Iterate sorts a
	// value's own fields on its way down, which the walk from the root
	// passed through and this does not.
	for _, each := range util.ByStepOrder(under) {
		util.Iterate(each, look)
	}
	return pending
}

// fields is the values the scope names, as the value now stands, and whether
// they could be found at all.
//
// A field the scope names but the value has not got is simply not there to
// walk, which is what a template declaring something only under a
// comprehension looks like before the comprehension has anything to go on.
func (in callScope) fields(value cue.Value) ([]cue.Value, bool) {
	if len(in) == 0 {
		return nil, true
	}
	if out, ok := in.lookup(value); ok {
		return out, true
	}
	return in.enumerate(value)
}

// lookup finds the scope's fields by name.
//
// Enumerating the value's fields instead costs the whole template: asking a
// struct what its fields are finalises every one of them, and at the root
// every one of them is the manifest being rendered. Looking a name up reaches
// the one field.
//
// It reports false, so the caller enumerates, for a name it cannot spell as a
// selector or one the value has not got. Not got covers both a field that is
// genuinely absent and a label whose spelling this did not guess, and the two
// cannot be told apart from here: enumerating settles it either way.
func (in callScope) lookup(value cue.Value) ([]cue.Value, bool) {
	out := make([]cue.Value, 0, len(in))
	for _, name := range in {
		var sel cue.Selector
		switch {
		case strings.HasPrefix(name, "#"), strings.HasPrefix(name, "_#"):
			sel = cue.Def(name)
		case strings.HasPrefix(name, "_"):
			// a hidden label belongs to the package that wrote it, and the
			// syntax alone does not say which that is
			return nil, false
		default:
			sel = cue.Str(name)
		}
		field := value.LookupPath(cue.MakePath(sel))
		if !field.Exists() {
			return nil, false
		}
		out = append(out, field)
	}
	return out, true
}

// enumerate is lookup's fallback, reading the fields the value has rather than
// the names the syntax gave.
//
// The names come from the syntax and the selectors from the value, and the two
// spell some labels differently: a quoted field is my-field in the syntax and
// "my-field" as a selector. Both spellings are tried.
func (in callScope) enumerate(value cue.Value) ([]cue.Value, bool) {
	it, err := value.Fields(cue.Optional(true), cue.Hidden(true))
	if err != nil {
		return nil, false
	}
	named := make(map[string]bool, len(in))
	for _, name := range in {
		named[name] = true
	}
	out := make([]cue.Value, 0, len(in))
	for it.Next() {
		sel := it.Selector()
		if named[bareName(sel)] {
			out = append(out, it.Value())
			continue
		}
		if sel.LabelType() == cue.StringLabel && named[sel.Unquoted()] {
			out = append(out, it.Value())
		}
	}
	return out, true
}

// bareName is a selector's name with any constraint marker taken off.
//
// The names come from the syntax, which writes a field as _enc whether it
// is declared _enc, _enc? or _enc!, while a selector spells the optional
// one _enc?. Comparing the two as they stand matches a plain field and
// misses an optional one, and a call under a field this does not match is
// dropped without a word.
func bareName(sel cue.Selector) string {
	name := sel.String()
	if !sel.IsConstraint() {
		return name
	}
	return strings.TrimSuffix(strings.TrimSuffix(name, "?"), "!")
}

// concurrencyAt reads how many calls a template will have running at once
// here, from the nearest @concurrency attribute at or above the call.
//
// The attribute goes above the calls rather than on them, because by the time
// the value is walked the comprehension that wrote them is gone and only the
// struct it filled is left:
//
//	reads: {
//		for k in keys {"\(k)": kube.#Get & {$params: k}}
//	} @concurrency(8)
//
// It has to sit on a struct. A file-level attribute is not on one and reaches
// nothing, so a template marks each set of calls it wants run together, and
// calls outside them keep running one at a time.
func concurrencyAt(root cue.Value, path cue.Path) int {
	sels := path.Selectors()
	for n := len(sels); n >= 0; n-- {
		at := root
		if n > 0 {
			at = root.LookupPath(cue.MakePath(sels[:n]...))
		}
		if !at.Exists() {
			continue
		}
		attr := at.Attribute(concurrencyKey)
		if attr.Err() != nil {
			continue
		}
		// @concurrency() names no number and asks for the default. CUE reports
		// that as one empty argument rather than none, so the contents are
		// what distinguishes it.
		if strings.TrimSpace(attr.Contents()) == "" {
			return defaultConcurrency
		}
		asked, err := attr.Int(0)
		if err != nil || asked < 1 {
			// a number that cannot be read, or one asking for none. Neither is
			// a request to run calls together, and reading either as the
			// default would turn concurrency on where it was being turned off.
			return 1
		}
		return int(asked)
	}
	return 1
}

// pastDeadline reports that the resolve has run out of time, or been given up
// on. Both paths ask this, so a call that runs alongside others is no more
// free to start late than one that runs on its own.
func pastDeadline(ctx context.Context) bool {
	if ctx.Err() != nil {
		return true
	}
	ddl, ok := ctx.Deadline()
	return ok && ddl.Before(time.Now())
}

// runRound runs every call the walk found, in dependency order.
//
// Calls that read nothing another call produces run together; one that does
// waits until its dependency has run, and then reads it. Each level is applied
// before the next is judged, so a level's results are what make the next one
// ready.
//
// The walk is not repeated between levels. A call still to run is looked up
// again by its path, which is what reading the value as it now stands costs,
// rather than searching the whole value for it. Only a call that did not exist
// when the walk ran needs another one, and Resolve does that.
// Whatever a round managed before an error is kept in the value it returns.
// A caller looking at the value to see how far it got - which is the whole
// point of a deadline - should see the calls that did run.
//
// The round ends early after a call that can reveal another (see revealers),
// or whose result is a value its function built and may carry a call, so
// Resolve searches the value again before the next.
func (in *Compiler) runRound(
	ctx context.Context,
	value cue.Value,
	providers map[string]cuexruntime.Provider,
	pending []pendingCall,
	executed map[string]bool,
	waitingFor *callWaits,
	reveal *revealers,
) (cue.Value, bool, error) {
	remaining := pending
	// opaque means a round ran a function that builds its own cue.Value and
	// hands it back. What is in it is that function's business and could
	// include a definition, so the value has to be searched again. A result
	// that came from a Go value cannot hold one.
	opaque := false
	// the walk read the first level's calls already; later ones are read again
	// because an earlier level's results changed what they say
	for reread := false; len(remaining) > 0; reread = true {
		var (
			err         error
			levelOpaque bool
		)
		var stepped bool
		if value, remaining, levelOpaque, stepped, err = in.runLevel(ctx, value, providers, pending, remaining, executed, waitingFor, reread, reveal); err != nil {
			return value, opaque || levelOpaque, err
		}
		opaque = opaque || levelOpaque
		if stepped {
			break
		}
	}
	return value, opaque, nil
}

// runLevel runs the calls that are ready now and returns those that are not.
func (in *Compiler) runLevel(
	ctx context.Context,
	value cue.Value,
	providers map[string]cuexruntime.Provider,
	pending, remaining []pendingCall,
	executed map[string]bool,
	waitingFor *callWaits,
	reread bool,
	reveal *revealers,
) (cue.Value, []pendingCall, bool, bool, error) {
	stillPending := make(map[string]bool, len(remaining))
	for _, call := range remaining {
		stillPending[call.key] = true
	}
	level := remaining
	if reread {
		// Read each call as the value now stands: an earlier level's results
		// are what its input has been waiting for.
		level = make([]pendingCall, len(remaining))
		copy(level, remaining)
		for i := range level {
			if node := value.LookupPath(level[i].path); node.Exists() {
				level[i].value = node
			}
		}
	}
	// Readiness is settled before anything runs, only so the level knows how
	// many results it is about to collect. It has no side effects and cannot
	// fail, so working it out early changes nothing a caller can see - unlike
	// looking a provider up early, which would report a missing one before
	// calls ahead of it in the walk had run.
	runnable := make([]bool, len(level))
	some := false
	for i, call := range level {
		if ready(call, pending, stillPending, executed, waitingFor) {
			runnable[i] = true
			some = true
		}
	}
	if !some {
		// Every call is waiting on a value none of them can produce - a cycle,
		// or input that will never be filled. Run the first so the caller gets
		// the error the call itself reports, as it did when calls ran one at a
		// time.
		runnable[0] = true
	}

	var results []callResult
	var blocked []pendingCall
	opaque := false
	for i := 0; i < len(level); {
		if pastDeadline(ctx) {
			return applyResults(value, results), nil, opaque, false, ResolveTimeoutErr{}
		}
		call := level[i]
		fn, err := providerFn(providers, call)
		if err != nil {
			return applyResults(value, results), nil, opaque, false, err
		}
		if !runnable[i] {
			blocked = append(blocked, call)
			i++
			continue
		}
		if end, limit := concurrentRun(level, runnable, providers, i); end > i+1 {
			together, err := runTogether(ctx, providers, level[i:end], limit)
			// whatever ran is kept whether or not one of its siblings failed:
			// a call that reached a cluster has already changed it
			results = append(results, together...)
			for _, ran := range together {
				executed[ran.call.key] = true
			}
			if err != nil {
				return applyResults(value, results), nil, opaque, false, err
			}
			i = end
			for _, ran := range together {
				if ran.opaque || reveal.after(ran.call) {
					return applyResults(value, results), append(blocked, level[i:]...), opaque, true, nil
				}
			}
			continue
		}
		_, fromGo := fn.(cuexruntime.ResultProviderFn)
		if !fromGo {
			opaque = true
		}
		ret, err := callProvider(ctx, fn, call)
		if err != nil {
			return applyResults(value, results), nil, opaque, false, err
		}
		results = append(results, callResult{call: call, ret: ret, opaque: !fromGo})
		executed[call.key] = true
		i++
		if !fromGo || reveal.after(call) {
			return applyResults(value, results), append(blocked, level[i:]...), opaque, true, nil
		}
	}
	return applyResults(value, results), blocked, opaque, false, nil
}

// concurrentRun returns the end of the run of calls starting at start that
// can go together: runnable, asking for the same concurrency, and provided by
// a function that says it may run alongside itself. Calls a comprehension
// wrote are siblings, so they are next to each other here, and a run of them
// keeps its place among whatever else the level holds.
func concurrentRun(level []pendingCall, runnable []bool, providers map[string]cuexruntime.Provider, start int) (int, int) {
	limit := level[start].effectiveConcurrency()
	if limit <= 1 || !runsConcurrently(providers, level[start]) {
		return start + 1, 1
	}
	end := start + 1
	// calls only run together while they share a limit, so a level mixing
	// functions that cap differently splits rather than running all of them at
	// the loosest of the caps.
	for end < len(level) &&
		runnable[end] &&
		level[end].effectiveConcurrency() == limit &&
		runsConcurrently(providers, level[end]) {
		end++
	}
	return end, limit
}

// effectiveConcurrency is what a call actually runs at: what its template asked
// for, brought down to what its own function will fan out to. A function that
// declares nothing runs one call at a time however much a template asks for,
// and a declared ceiling never raises the template's number either.
func (in pendingCall) effectiveConcurrency() int {
	ceiling := in.maxPerRender
	if ceiling < 1 {
		ceiling = defaultMaxPerRender
	}
	if ceiling < in.concurrency {
		return ceiling
	}
	return in.concurrency
}

// declaredMaxPerRender reads the ceiling a call's own function declared. A
// function with no #config, or a #config not naming maxPerRender, declares
// none, so keys added there later cap nothing by accident.
//
// A ceiling that is there but cannot be read as a number above zero comes back
// as 1, one call at a time, rather than as no ceiling at all. A template can
// put a second maxPerRender on the call and leave the field conflicting, which
// compiles: reading that as "no limit" would hand the template a way to talk
// its way out of a limit the function set, which is the opposite of what the
// field is for.
func declaredMaxPerRender(v cue.Value) int {
	field := v.LookupPath(maxPerRenderPath)
	if !field.Exists() {
		return 0
	}
	max, err := field.Int64()
	if err != nil || max < 1 {
		return 1
	}
	return int(max)
}

func runsConcurrently(providers map[string]cuexruntime.Provider, call pendingCall) bool {
	fn, err := providerFn(providers, call)
	if err != nil {
		return false
	}
	_, ok := fn.(cuexruntime.ConcurrentProviderFn)
	return ok
}

// runTogether reads each call's parameters in turn and then runs the calls at
// once, up to limit of them.
//
// Reading comes first and on its own because CUE values built by one context
// cannot be read concurrently. What is left after that is Go values and the
// functions themselves, which is the part a template asked to overlap.
func runTogether(
	ctx context.Context,
	providers map[string]cuexruntime.Provider,
	calls []pendingCall,
	limit int,
) ([]callResult, error) {
	fns := make([]cuexruntime.ConcurrentProviderFn, len(calls))
	decoded := make([]any, len(calls))
	for i, call := range calls {
		fn, err := providerFn(providers, call)
		if err != nil {
			return nil, err
		}
		concurrent, ok := fn.(cuexruntime.ConcurrentProviderFn)
		if !ok {
			return nil, ProviderFnNotFoundErr{Provider: call.provider, Fn: call.fn}
		}
		fns[i] = concurrent
		if decoded[i], err = concurrent.Decode(call.value); err != nil {
			return nil, NewFunctionCallError(call.value, err)
		}
	}

	// limit workers taking calls in turn, rather than a goroutine per call
	// waiting for a slot: a thousand calls at eight at a time is eight
	// goroutines, not a thousand.
	workers := limit
	if workers > len(calls) {
		workers = len(calls)
	}
	rets := make([]any, len(calls))
	errs := make([]error, len(calls))
	// what actually ran, rather than what has no error against it: a call the
	// deadline stopped being reached has neither, and reporting that as a
	// result would mark it done and write its nothing into the value
	ran := make([]bool, len(calls))
	var (
		next     atomic.Int64
		ranShort atomic.Bool
		wg       sync.WaitGroup
	)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= len(calls) {
					return
				}
				// checked per call rather than once, so a group does not run
				// to the end of itself after the time for it has gone
				if pastDeadline(ctx) {
					ranShort.Store(true)
					return
				}
				rets[i], errs[i] = fns[i].Invoke(ctx, decoded[i])
				ran[i] = true
			}
		}()
	}
	wg.Wait()

	// Everything that ran is handed back, failure or not. These calls were in
	// flight together, so the ones that finished have already done whatever
	// they do, and dropping their results would lose that.
	results := make([]callResult, 0, len(calls))
	for i := range calls {
		if ran[i] && errs[i] == nil {
			results = append(results, callResult{call: calls[i], ret: rets[i]})
		}
	}
	// the first failure in the template's order, rather than whichever
	// happened to finish first, so the same template fails the same way twice
	for i, err := range errs {
		if err != nil {
			return results, NewFunctionCallError(calls[i].value, err)
		}
	}
	if ranShort.Load() {
		return results, ResolveTimeoutErr{}
	}
	return results, nil
}

// callProvider runs a call, asking for a Go value from any function that
// offers one.
//
// A result reaches the value as syntax either way, and the two roads there are
// not the same length. From a Go value it is a JSON encode and a read of the
// tokens. From a cue.Value the function first fills the result into the node
// it was given, and the resolver then reads the syntax back out of that node,
// building and taking apart a value neither of them wanted: for a pass of
// eight calls, eight times the work for the same answer.
func callProvider(ctx context.Context, fn cuexruntime.ProviderFn, call pendingCall) (any, error) {
	if returning, ok := fn.(cuexruntime.ResultProviderFn); ok {
		ret, err := returning.CallForResult(ctx, call.value)
		if err != nil {
			// the call's own node is what the error is about; a result form
			// has no value of its own to report
			return nil, NewFunctionCallError(call.value, err)
		}
		return ret, nil
	}
	ret, err := fn.Call(ctx, call.value)
	if err != nil {
		return nil, NewFunctionCallError(ret, err)
	}
	return ret, nil
}

// callResult is a call's result, waiting to be applied. It is a cue.Value
// from a ProviderFn, or a Go value from a ResultProviderFn; FillPath takes
// either.
type callResult struct {
	call pendingCall
	ret  any
	// opaque marks a result a NativeProviderFn built itself. What is in it is
	// that function's business and may include a definition, which is how one
	// provider's result carries another call.
	opaque bool
}

// applyResults writes a pass' results into the value.
//
// The whole returned value is applied, not just its $returns: a
// NativeProviderFn may write anywhere in the node.
//
// Several results are collected into one value and unified in a single step,
// which is most of what batching buys: unifying at the root is the expensive
// half of resolving, and this pays it once per pass instead of once per call.
//
// A single result is written straight into the value. Collecting it first
// would cost more than it saves, and a returned value carries its call's own
// $params, which may refer to fields the value has and a collection standing
// on its own does not.
func applyResults(value cue.Value, results []callResult) cue.Value {
	if len(results) == 1 {
		return value.FillPath(results[0].call.fill, results[0].ret)
	}
	// Filling each result into a collection in turn unifies that collection
	// again for every one of them, so a wide pass pays for its own results
	// repeatedly. The collection is built as syntax instead, in the order the
	// results came, and turned into a value once. Order is why this is syntax
	// and not a Go map: a map's keys come back sorted, which moves fields in
	// the rendered output.
	// A result that is already a cue.Value goes into a collection built as
	// syntax and turned into a value once. Filling each into a collection in
	// turn would unify that collection again for every one of them, so a wide
	// pass would pay for its own results repeatedly. Order is why this is
	// syntax and not a Go map: a map's keys come back sorted, which moves
	// fields in the rendered output.
	overlay := &overlayNode{}
	var rest []callResult
	writer := &answerWriter{cc: value.Context()}
	for _, result := range results {
		if expr, ok := writer.write(result.ret, result.opaque); ok && overlay.set(result.call.fill, expr) {
			continue
		}
		rest = append(rest, result)
	}
	if len(overlay.order) > 0 {
		value = value.Unify(value.Context().BuildExpr(overlay.expr()))
	}
	switch len(rest) {
	case 0:
		return value
	case 1:
		return value.FillPath(rest[0].call.fill, rest[0].ret)
	}
	// Go values have to be converted where they are filled, and there is no
	// syntax for them short of converting them twice. They are collected the
	// same way, one fill each, and unified once.
	collected := value.Context().CompileString("")
	for _, result := range rest {
		collected = collected.FillPath(result.call.fill, result.ret)
	}
	return value.Unify(collected)
}

// resultSyntax renders a result as an expression the overlay can hold.
// Anything that will not render is filled on its own instead.
//
// A Go value is converted here rather than where it is filled. Filling each
// into a collection in turn is what the overlay exists to avoid, and a pass
// that ran its calls together hands back Go values by the hundred.
//
// It is converted by filling, which is how a result reaches the value when its
// call ran on its own. Context.Encode is the other way to do it and does not
// agree: given a nil it drops the field where filling leaves top behind. A
// result must not depend on whether the template asked for its calls to run
// together, so the conversion has to be the one the rest of the resolver uses.
func resultSyntax(cc *cue.Context, ret any, opaque bool) (ast.Expr, bool) {
	return answerWriter{cc: cc}.syntax(ret, opaque)
}

// answerWriter turns what providers returned into syntax, holding the one
// blank value a Go result is filled into.
//
// The blank depends on nothing but the context, so it is the same value
// every time, and building it is a fifth of what writing an answer back
// costs: 3.4us and 5.5KB of 18.4us and 28.8KB. Both callers write a
// pass or a batch of answers in a loop, so both have somewhere to hold it.
type answerWriter struct {
	cc *cue.Context
	// blank is built on first use rather than up front, since a pass whose
	// results are all cue.Values never needs one.
	blank cue.Value
	built bool
}

func (w *answerWriter) fresh() (cue.Value, bool) {
	if w.cc == nil {
		return cue.Value{}, false
	}
	if !w.built {
		w.blank, w.built = w.cc.CompileString(""), true
	}
	return w.blank, w.blank.Err() == nil
}

func (w answerWriter) syntax(ret any, opaque bool) (ast.Expr, bool) {
	return (&w).write(ret, opaque)
}

func (w *answerWriter) write(ret any, opaque bool) (ast.Expr, bool) {
	val, isValue := ret.(cue.Value)
	if !isValue {
		// Filled, the way a result reaches the value when its call ran on
		// its own.
		//
		// Reading it as JSON and parsing that is quicker and does not
		// agree. JSON has one number, so a whole float comes back an int
		// and a []byte comes back the base64 string it encodes to, and a
		// template declaring $returns?: {num: float} then renders for one
		// call and conflicts for two. Nothing about a result may depend on
		// how many calls shared its pass.
		blank, ok := w.fresh()
		if !ok {
			return nil, false
		}
		val = blank.FillPath(cue.Path{}, ret)
		if val.Err() != nil {
			return nil, false
		}
	}
	// Final resolves references, so the collection stands on its own. It also
	// leaves out definitions, and a definition is how a result carries another
	// call - so a result that could hold one asks for them back. Only a native
	// provider's can: the rest are a node unified with a Go value, and a Go
	// value has no definitions in it. Asking for them everywhere would put
	// each call's own #do and #provider into the collection, which the value
	// already has, and a wide pass pays for that.
	opts := []cue.Option{cue.Final()}
	if opaque {
		opts = append(opts, cue.Definitions(true), cue.Hidden(true), cue.Optional(true))
	}
	switch node := val.Syntax(opts...).(type) {
	case *ast.File:
		return &ast.StructLit{Elts: node.Decls}, true
	case ast.Expr:
		return node, true
	default:
		return nil, false
	}
}

// overlayNode is a tree of results keyed by where they go, keeping the order
// they were added in.
type overlayNode struct {
	order []overlayKey
	kids  map[overlayKey]*overlayNode
	leaf  ast.Expr
}

// overlayKey is how a child is named in the collection. The kind is part of
// it because "_h" and _h are different fields that spell the same name, and
// keying by the name alone merges them: a result meant for the hidden one
// then lands in the quoted one, or the other way about, with nothing said.
type overlayKey struct {
	name   string
	hidden bool
}

// set places expr at path, reporting false for a path it cannot hold - a list
// index, a definition, or a field another result already claimed.
func (in *overlayNode) set(path cue.Path, expr ast.Expr) bool {
	sels := path.Selectors()
	if len(sels) == 0 {
		return false
	}
	for _, sel := range sels {
		if !overlayHolds(sel) {
			return false
		}
	}
	node := in
	for _, sel := range sels {
		if node.leaf != nil {
			return false
		}
		node = node.child(overlayKey{
			name:   overlayName(sel),
			hidden: sel.LabelType() == cue.HiddenLabel,
		})
	}
	if node.leaf != nil || len(node.order) > 0 {
		return false
	}
	node.leaf = expr
	return true
}

// overlayHolds reports whether a selector can be written as a label in the
// collection the overlay builds.
//
// A hidden field can, and has to be spelled as an identifier rather than
// quoted: a quoted "_h" is a different field from _h, so a result written
// that way lands beside the call instead of in it. Only a hidden name from
// the anonymous package is taken, because a hidden name belongs to the
// package that wrote it and a label built here belongs to no package. A
// template compiled from a file with no package clause, which is every
// template the resolver sees, is that one.
//
// Everything else the overlay cannot spell goes back to being filled one at a
// time, which is correct but costs a unify each.
func overlayHolds(sel cue.Selector) bool {
	switch sel.LabelType() {
	case cue.StringLabel:
		return true
	case cue.HiddenLabel:
		return sel.PkgPath() == "_"
	default:
		return false
	}
}

func overlayName(sel cue.Selector) string {
	if sel.LabelType() == cue.HiddenLabel {
		return sel.String()
	}
	return sel.Unquoted()
}

func (in *overlayNode) child(key overlayKey) *overlayNode {
	if in.kids == nil {
		in.kids = map[overlayKey]*overlayNode{}
	}
	if kid, ok := in.kids[key]; ok {
		return kid
	}
	kid := &overlayNode{}
	in.kids[key] = kid
	in.order = append(in.order, key)
	return kid
}

func (in *overlayNode) expr() ast.Expr {
	if in.leaf != nil {
		return in.leaf
	}
	lit := &ast.StructLit{}
	for _, key := range in.order {
		var label ast.Label = ast.NewString(key.name)
		if key.hidden {
			label = ast.NewIdent(key.name)
		}
		lit.Elts = append(lit.Elts, &ast.Field{
			Label: label,
			Value: in.kids[key].expr(),
		})
	}
	return lit
}

// unconstrained drops field constraints from a path, turning o? into o.
// Applying a result at a path that kept the constraint would leave the field
// optional instead of materialising it the way a direct fill did.
func unconstrained(path cue.Path) cue.Path {
	sels := path.Selectors()
	out := make([]cue.Selector, len(sels))
	for i, sel := range sels {
		out[i] = sel
		if !sel.IsConstraint() {
			continue
		}
		switch sel.LabelType() {
		case cue.StringLabel:
			out[i] = cue.Str(sel.Unquoted())
		case cue.HiddenLabel:
			// a hidden field is its own kind of label, and rebuilding it
			// as a string one makes a different field that spells the same
			out[i] = cue.Hid(bareName(sel), sel.PkgPath())
		}
	}
	return cue.MakePath(out...)
}

// ready reports whether a call can run now.
//
// Calls run in the order the template writes them, and a call moves later only
// for a call it reads. Following references finds most of those; it misses one
// reached by indexing a list a comprehension has yet to build, which leaves the
// call's parameters unresolved instead. Such a call waits for the calls written
// ahead of it, which are the only ones a template order can have meant it to
// read, and never for one after it: a later call can end the resolve, as a
// workflow step's wait does, and a call held behind it would never run. With
// nothing ahead of it left to run, it runs as it stands, and a parameter that
// is still missing is reported by the call.
func ready(call pendingCall, pending []pendingCall, stillPending map[string]bool, executed map[string]bool, waitingFor *callWaits) bool {
	if waitsForPeer(call, pending, stillPending, executed, waitingFor) {
		return false
	}
	return paramsResolved(call.value) || !earlierPending(call, pending, stillPending)
}

// earlierPending reports whether a call written ahead of call has yet to run.
func earlierPending(call pendingCall, pending []pendingCall, stillPending map[string]bool) bool {
	for _, peer := range pending {
		if peer.key == call.key {
			return false
		}
		if stillPending[peer.key] {
			return true
		}
	}
	return false
}

// paramsResolved reports whether a call's parameters have a value yet. A call
// with no $params has nothing to check here: a legacy call's top-level fields
// hold its outputs as well as its inputs, and those stay unset until it runs, so
// it is ordered by the calls it reads (callDependencies) alone.
//
// A NativeProviderFn is handed the parameters as a cue.Value and may mean to
// take them unresolved - a schema, or an open disjunction. Such a call waits
// for the calls written ahead of it and then runs in its place (see ready).
// Exempting native functions from this test would let one run before a call
// it reads through something the reference search cannot see, which is what
// the test is for.
func paramsResolved(v cue.Value) bool {
	params := v.LookupPath(paramsPath)
	if !params.Exists() {
		return true
	}
	return params.Validate(cue.Concrete(true)) == nil
}

// callWaits is what each call waits for, worked out the first time the call is
// seen and reused after: re-reading it every pass means re-reading a value that
// grows more expensive to inspect with every result applied to it.
type callWaits struct {
	of map[string][]string
	// analysis is where it is worked out, when the resolve has one.
	analysis *analysisBuild
}

// analysisBuild is a second build of the value a resolve started from, in a
// CUE context of its own, which working out what a call reads evaluates
// instead of the value calls run against. Following a call's references takes
// its inputs apart, and CUE keeps what that evaluates with the context: a
// result filled in afterwards can then come back bottom, and the calls after it
// are never found. kubevela/workflow's legacy op package did this to its
// share-cloud-resource and deploy-cloud-resource steps.
//
// It is built the first time a call is read, and holds no results: a read of
// a call that has run is settled without looking at its output, and the rest
// is the template as written.
type analysisBuild struct {
	build func() cue.Value
	value cue.Value
	built bool
}

// node is call as the analysis build has it. A resolve with no build of its
// own reads the call itself. A call the build does not have, revealed by a
// result since, is not read, and runs where it is written.
func (a *analysisBuild) node(call pendingCall) (cue.Value, bool) {
	if a == nil {
		return call.value, true
	}
	if !a.built {
		a.value, a.built = a.build(), true
	}
	node := a.value.LookupPath(call.path)
	return node, node.Exists()
}

// waitsForPeer reports whether the call reads output that another call has yet
// to produce. What a call reads is worked out once and remembered; each pass
// only rechecks whether those calls have run.
func waitsForPeer(call pendingCall, pending []pendingCall, stillPending map[string]bool, executed map[string]bool, waitingFor *callWaits) bool {
	waits, known := waitingFor.of[call.key]
	if !known {
		// a lone call has no peer to read, and no reason to build anything
		if len(pending) > 1 {
			if node, ok := waitingFor.analysis.node(call); ok {
				read := call
				read.value = node
				waits = callDependencies(read, pending, executed)
			}
		}
		waitingFor.of[call.key] = waits
	}
	for _, key := range waits {
		if stillPending[key] {
			return true
		}
	}
	return false
}

// callDependencies returns the keys of the calls whose output this call reads.
//
// A template rarely reads a call directly. It names the result once and reads
// that name, so following only the references in $params finds an ordinary
// field rather than the call behind it:
//
//	workload: resources.$returns.output
//	patch: util.#PatchK8sObject & {$params: value: workload}
//
// References are therefore followed through fields that are not themselves
// calls, and stop at ones that are - what a call holds is that call's business
// and reading into it here means reading whatever it has rendered.
//
// A call with no $params is written in the legacy style: its inputs are its own
// top-level fields, so those are what it reads.
//
//	wait: op.#ConditionalWait & {continue: req.$returns != _|_}
func callDependencies(call pendingCall, pending []pendingCall, executed map[string]bool) []string {
	if len(pending) < 2 {
		return nil // no peer to read, so nothing to read from one
	}
	var queue []reference
	if !collectInputReferences(call.value, &queue) {
		return peerKeys(call, pending)
	}
	waits := map[string]bool{}
	seen := map[string]bool{}
	for len(queue) > 0 {
		ref := queue[0]
		queue = queue[1:]
		if seen[ref.key] || ref.key == call.key {
			continue
		}
		seen[ref.key] = true
		if len(seen) > maxReferenceExpansion {
			return peerKeys(call, pending)
		}
		if owner, found := owningCall(ref.key, pending, call.key); found {
			waits[owner] = true
			continue
		}
		if owningExecutedCall(ref.key, executed) {
			// already produced, and its result is not worth reading into
			continue
		}
		target, at, found := nearestExisting(ref)
		if !found {
			continue
		}
		if _, hasDefault := target.Default(); hasDefault {
			// The target has not settled: it is a disjunction still standing
			// on its default, which a comprehension guarded by another call's
			// output may yet replace. That guard is not visible from here, so
			// the call waits for the calls written ahead of it, the only ones
			// a template order lets it read, rather than reading a default one
			// of them is about to change.
			//
			//	w: *false | bool
			//	if apply.$returns.value.spec.key != "" { w: true }
			//	wait: builtin.#ConditionalWait & {$params: continue: w}
			return earlierKeys(call, pending)
		}
		if key := at.String(); key != ref.key {
			// the exact path does not resolve yet, but an ancestor of it
			// does - queue that so it gets the same ownership check
			queue = append(queue, reference{root: ref.root, path: at, key: key})
			continue
		}
		if !collectReferences(target, &queue, 0) {
			return peerKeys(call, pending)
		}
	}
	if len(waits) == 0 {
		return nil
	}
	keys := make([]string, 0, len(waits))
	for key := range waits {
		keys = append(keys, key)
	}
	return keys
}

// collectInputReferences collects the references a call's inputs make: its
// $params, or for a legacy call with none, each of its own top-level fields,
// hidden ones included, since a provider may read those too. The
// fields and not the call's expression, which is the conjunction with the
// definition the call is made from, and reading into that definition reads the
// package it lives in rather than anything the call was given.
func collectInputReferences(call cue.Value, into *[]reference) bool {
	if params := call.LookupPath(paramsPath); params.Exists() {
		return collectReferences(params, into, 0)
	}
	it, err := call.Fields(cue.Optional(true), cue.Hidden(true))
	if err != nil {
		return false
	}
	for it.Next() {
		if !collectReferences(it.Value(), into, 1) {
			return false
		}
	}
	return true
}

// reference is a path some value reads, kept with the root it is relative to
// so it can be looked up again.
type reference struct {
	root cue.Value
	path cue.Path
	key  string
}

// nearestExisting resolves a reference, falling back to the nearest ancestor
// of it that resolves. A path reaching into output a call has yet to produce
// does not resolve, while the field the path starts from does - and that field
// is what names the call.
func nearestExisting(ref reference) (cue.Value, cue.Path, bool) {
	sels := ref.path.Selectors()
	for n := len(sels); n > 0; n-- {
		path := cue.MakePath(sels[:n]...)
		if v := ref.root.LookupPath(path); v.Exists() {
			return v, path, true
		}
	}
	return cue.Value{}, cue.Path{}, false
}

// owningCall reports the pending call that produces the given path, if any.
func owningCall(ref string, pending []pendingCall, self string) (string, bool) {
	for _, peer := range pending {
		if peer.key != self && isUnder(ref, peer.key) {
			return peer.key, true
		}
	}
	return "", false
}

func owningExecutedCall(ref string, executed map[string]bool) bool {
	for key := range executed {
		if isUnder(ref, key) {
			return true
		}
	}
	return false
}

// earlierKeys is the calls written ahead of call.
func earlierKeys(call pendingCall, pending []pendingCall) []string {
	var keys []string
	for _, peer := range pending {
		if peer.key == call.key {
			break
		}
		keys = append(keys, peer.key)
	}
	return keys
}

func peerKeys(call pendingCall, pending []pendingCall) []string {
	keys := make([]string, 0, len(pending))
	for _, peer := range pending {
		if peer.key != call.key {
			keys = append(keys, peer.key)
		}
	}
	return keys
}

const (
	// maxReferenceDepth bounds how far into a value references are looked for.
	maxReferenceDepth = 100
	// maxReferenceExpansion bounds how many references are followed for one
	// call. Reaching either makes the call wait for every peer, which costs a
	// pass and never runs it on input that may still change.
	maxReferenceExpansion = 1000
)

// collectReferences appends every path v reads to into, reporting false if it
// gave up before reaching the end. References hide inside conjunctions, so an
// expression's operands are searched as well as a struct's fields.
func collectReferences(v cue.Value, into *[]reference, depth int) bool {
	if depth > maxReferenceDepth {
		return false
	}
	if root, path := v.ReferencePath(); len(path.Selectors()) > 0 {
		*into = append(*into, reference{root: root, path: path, key: path.String()})
		// what the reference points at is reached by following it from the
		// queue, not by descending into it here
		return true
	}
	if op, args := v.Expr(); op != cue.NoOp || len(args) > 1 {
		for _, arg := range args {
			if !collectReferences(arg, into, depth+1) {
				return false
			}
		}
		return true
	}
	switch v.Kind() {
	case cue.StructKind:
		it, err := v.Fields(cue.Optional(true), cue.Hidden(true))
		if err != nil {
			return false
		}
		for it.Next() {
			if !collectReferences(it.Value(), into, depth+1) {
				return false
			}
		}
	case cue.ListKind:
		it, err := v.List()
		if err != nil {
			return false
		}
		for it.Next() {
			if !collectReferences(it.Value(), into, depth+1) {
				return false
			}
		}
	}
	return true
}

// isUnder reports whether the path ref is owner itself or sits inside it.
func isUnder(ref, owner string) bool {
	if !strings.HasPrefix(ref, owner) {
		return false
	}
	rest := ref[len(owner):]
	return rest == "" || rest[0] == '.' || rest[0] == '['
}

func providerFn(providers map[string]cuexruntime.Provider, call pendingCall) (cuexruntime.ProviderFn, error) {
	prd, found := providers[call.provider]
	if !found {
		return nil, ProviderNotFoundErr(call.provider)
	}
	fn := prd.GetProviderFn(call.fn)
	if fn == nil {
		return nil, ProviderFnNotFoundErr{Provider: call.provider, Fn: call.fn}
	}
	return fn, nil
}

// DefaultCompiler compiler for cuex to compile
var DefaultCompiler = singleton.NewSingleton[*Compiler](func() *Compiler {
	compiler := NewCompilerWithDefaultInternalPackages()
	if EnableExternalPackageForDefaultCompiler {
		if err := compiler.LoadExternalPackages(context.Background()); err != nil && !kerrors.IsNotFound(err) {
			klog.Errorf("failed to load external packages for cuex default compiler: %s", err.Error())
		}
	}
	if EnableExternalPackageWatchForDefaultCompiler {
		go compiler.ListenExternalPackages(nil)
	}
	return compiler
})

// NewCompilerWithInternalPackages create compiler with internal packages
func NewCompilerWithInternalPackages(packages ...cuexruntime.Package) *Compiler {
	return &Compiler{
		PackageManager: cuexruntime.NewPackageManager(
			slices.Map(packages, func(p cuexruntime.Package) cuexruntime.PackageManagerOption {
				return cuexruntime.WithInternalPackage{Package: p}
			})...,
		),
	}
}

// NewCompilerWithDefaultInternalPackages create compiler with default internal packages
func NewCompilerWithDefaultInternalPackages() *Compiler {
	return NewCompilerWithInternalPackages(
		base64.Package,
		http.Package,
		kube.Package,
		cueext.Package,
		cuexutil.Package,
	)
}

var (
	// EnableExternalPackageForDefaultCompiler .
	EnableExternalPackageForDefaultCompiler = true
	// EnableExternalPackageWatchForDefaultCompiler .
	EnableExternalPackageWatchForDefaultCompiler = false
)

// AddFlags add flags for configuring cuex default compiler
func AddFlags(set *pflag.FlagSet) {
	set.BoolVarP(&EnableExternalPackageForDefaultCompiler, "enable-external-cue-package", "", EnableExternalPackageForDefaultCompiler, "enable load external package for cuex default compiler")
	set.BoolVarP(&EnableExternalPackageWatchForDefaultCompiler, "list-watch-external-cue-package", "", EnableExternalPackageWatchForDefaultCompiler, "enable watch external package changes for cuex default compiler")
	set.BoolVarP(&cuexruntime.DefaultClientInsecureSkipVerify, "cuex-external-provider-insecure-skip-verify", "", cuexruntime.DefaultClientInsecureSkipVerify, "Set if the default external provider client of cuex should skip insecure verify")
}

// CompileString use cuex default compiler to compile cue string
func CompileString(ctx context.Context, src string) (cue.Value, error) {
	return DefaultCompiler.Get().CompileStringWithOptions(ctx, src)
}

// CompileStringWithOptions use cuex default compiler to compile cue string with options
func CompileStringWithOptions(ctx context.Context, src string, opts ...CompileOption) (cue.Value, error) {
	return DefaultCompiler.Get().CompileStringWithOptions(ctx, src, opts...)
}
