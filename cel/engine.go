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
	"regexp"
	"sort"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/google/cel-go/ext"
	apiservercel "k8s.io/apiserver/pkg/cel"
	"k8s.io/utils/lru"

	"github.com/kubevela/pkg/cel/template"
)

// A Root is a top-level name an expression may read. Everything an expression
// reads hangs off a declared root, so the sandbox is "these names and nothing
// else" rather than a denylist.
type Root struct {
	Name string
	// Qualifiers are the member calls a read of this root may make to select
	// among values delivered with it, such as peer.db.at("east").
	Qualifiers []Qualifier
	// QualifiedAt is how many path segments a qualifier call follows: 1 puts it
	// straight after an entry's name, as peer.db.at("east"). Further qualifiers
	// may follow it directly.
	QualifiedAt int
}

// A Qualifier is a member call taking one literal string, answered by looking
// up what the caller delivered under QualifiedKey. Which value it names is
// settled before evaluation, so the call performs no I/O.
type Qualifier struct {
	Name string
	// Result is the call's type; nil is dyn.
	Result *cel.Type
}

// QualifiedKey holds, inside a delivered value, what each qualifier call on it
// returns, keyed by template.Call. A chained call nests the same way:
// at("a").in("b") is the "in:b" entry inside the "at:a" entry.
const QualifiedKey = "$q"

const (
	// programCacheSize bounds the distinct expressions kept compiled.
	programCacheSize = 2048
	// typedEnvCacheSize bounds the distinct typed environments kept.
	typedEnvCacheSize = 256
	// checkedPerEnvSize bounds the checked ASTs kept for one typed environment.
	checkedPerEnvSize = 256
	// planCacheSize bounds the JSON documents kept planned.
	planCacheSize = 1024
)

// An Engine compiles and evaluates expressions over a fixed set of roots.
//
// It is immutable once built and safe for concurrent use; lru.Cache locks
// internally.
type Engine struct {
	roots      []string
	isRoot     map[string]bool
	qualifiers map[string]Qualifier
	// declaredOn is the roots declaring each qualifier.
	declaredOn map[string][]string
	// rootQualifiers is each root's qualifier names, in declared order.
	rootQualifiers map[string][]string
	qualifiedAt    map[string]int
	// functions is what every environment offers besides its roots.
	functions []cel.EnvOption
	dyn       *cel.Env
	// compiled holds programs for dyn alone, keyed by expression text.
	compiled *lru.Cache
	typed    *lru.Cache
	// checked holds, for each typed environment typed still holds, the
	// checked ASTs OutputType has made in it, keyed by expression text. An
	// environment's entry goes when typed evicts it.
	checkedMu sync.Mutex
	checked   map[*cel.Env]*lru.Cache
	// plans holds plans of JSON documents, keyed by a digest of their bytes.
	plans *lru.Cache
}

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// NewEngine builds an engine for the roots given.
//
// A qualifier compiles on any receiver, since a read below a root is dyn, so
// References refuses one called on anything but a read of a root declaring it.
// Two roots may declare the same qualifier only with the same result type.
func NewEngine(roots ...Root) (*Engine, error) {
	e := &Engine{
		isRoot:     map[string]bool{},
		qualifiers: map[string]Qualifier{},
		declaredOn: map[string][]string{},

		rootQualifiers: map[string][]string{},
		qualifiedAt:    map[string]int{},
		compiled:       lru.New(programCacheSize),
		checked:        map[*cel.Env]*lru.Cache{},
		plans:          lru.New(planCacheSize),
	}
	e.typed = lru.NewWithEvictionFunc(typedEnvCacheSize, func(_ lru.Key, v interface{}) {
		if env, ok := v.(*cel.Env); ok {
			e.checkedMu.Lock()
			delete(e.checked, env)
			e.checkedMu.Unlock()
		}
	})
	base, err := cel.NewEnv(append([]cel.EnvOption{cel.Variable("x", cel.DynType)}, libraries()...)...)
	if err != nil {
		return nil, err
	}
	for _, r := range roots {
		if !identifier.MatchString(r.Name) {
			return nil, fmt.Errorf("root %q is not an identifier", r.Name)
		}
		if e.isRoot[r.Name] {
			return nil, fmt.Errorf("root %q is declared twice", r.Name)
		}
		if r.QualifiedAt < 0 {
			return nil, fmt.Errorf("root %q has a negative QualifiedAt", r.Name)
		}
		e.isRoot[r.Name] = true
		e.roots = append(e.roots, r.Name)
		e.qualifiedAt[r.Name] = r.QualifiedAt
		own := map[string]bool{}
		for _, q := range r.Qualifiers {
			if !identifier.MatchString(q.Name) {
				return nil, fmt.Errorf("qualifier %q on root %q is not an identifier", q.Name, r.Name)
			}
			if own[q.Name] {
				return nil, fmt.Errorf("qualifier %q is declared twice on root %q", q.Name, r.Name)
			}
			own[q.Name] = true
			if taken(base, q.Name) {
				return nil, fmt.Errorf("qualifier %q is already a function or macro in every environment", q.Name)
			}
			if prev, ok := e.qualifiers[q.Name]; ok && resultOf(prev).String() != resultOf(q).String() {
				return nil, fmt.Errorf("qualifier %q is declared as both %s and %s", q.Name, resultOf(prev), resultOf(q))
			}
			e.qualifiers[q.Name] = q
			e.declaredOn[q.Name] = append(e.declaredOn[q.Name], r.Name)
			e.rootQualifiers[r.Name] = append(e.rootQualifiers[r.Name], q.Name)
		}
	}
	e.functions = append(libraries(), e.qualifierFunctions()...)

	opts := make([]cel.EnvOption, 0, len(e.roots))
	for _, name := range e.roots {
		opts = append(opts, cel.Variable(name, cel.MapType(cel.StringType, cel.DynType)))
	}
	dyn, err := cel.NewEnv(append(opts, e.functions...)...)
	if err != nil {
		return nil, err
	}
	e.dyn = dyn
	return e, nil
}

// DynEnv is the environment for evaluation, where every root is an open map.
//
// Static typing is an admission concern: by the time a render happens the
// values exist, and admission has already refused anything that would not type.
// CEL selects a map key with `.`, so an expression reads the same here as against
// a declared object.
func (e *Engine) DynEnv() *cel.Env { return e.dyn }

// TypedEnv is an environment where the roots in decls carry those types and the
// rest stay open maps. It is kept under key, and decls is only called on a miss;
// an empty key is never kept.
//
// The key must capture everything decls depends on: two callers sharing a key
// with different schemas would each type-check against the other's.
func (e *Engine) TypedEnv(key string, decls func() (map[string]*apiservercel.DeclType, error)) (*cel.Env, error) {
	if key != "" {
		if hit, ok := e.typed.Get(key); ok {
			if env, ok := hit.(*cel.Env); ok {
				return env, nil
			}
		}
	}
	d, err := decls()
	if err != nil {
		return nil, err
	}
	env, err := e.typedEnv(d)
	if err != nil {
		return nil, err
	}
	if key != "" {
		e.checkedMu.Lock()
		e.checked[env] = lru.New(checkedPerEnvSize)
		e.checkedMu.Unlock()
		e.typed.Add(key, env)
	}
	return env, nil
}

func (e *Engine) typedEnv(decls map[string]*apiservercel.DeclType) (*cel.Env, error) {
	for name := range decls {
		if !e.isRoot[name] {
			return nil, fmt.Errorf("%q is not a root of this engine", name)
		}
	}
	var declared []*apiservercel.DeclType
	var vars []cel.EnvOption
	for _, name := range e.roots {
		if d, ok := decls[name]; ok {
			declared = append(declared, d)
			vars = append(vars, cel.Variable(name, d.CelType()))
			continue
		}
		vars = append(vars, cel.Variable(name, cel.MapType(cel.StringType, cel.DynType)))
	}
	// The provider is what resolves a named object type when a field is
	// selected off it; declaring the variable alone is not enough.
	provider := apiservercel.NewDeclTypeProvider(declared...)
	opts, err := provider.EnvOptions(types.NewEmptyRegistry())
	if err != nil {
		return nil, fmt.Errorf("declaring root types: %w", err)
	}
	opts = append(opts, vars...)
	return cel.NewEnv(append(opts, e.functions...)...)
}

// libraries is the function set every environment offers.
//
// Only pure, total libraries: Strings for text handling, Lists for slice.
// Encoders and Sets have no established use, and Bindings introduces
// `cel.bind`, which would let an expression grow into a small program.
//
// cel.OptionalTypes() is not offered: it is incompatible with the
// DeclTypeProvider a typed environment is built on ("custom types not supported
// by provider"), so a default is written with has():
//
//	has(source.cfg.note) ? source.cfg.note : "none"
func libraries() []cel.EnvOption {
	return []cel.EnvOption{ext.Strings(), ext.Lists()}
}

func (e *Engine) qualifierFunctions() []cel.EnvOption {
	names := make([]string, 0, len(e.qualifiers))
	for name := range e.qualifiers {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]cel.EnvOption, 0, len(names))
	for _, name := range names {
		fn := name
		out = append(out, cel.Function(fn,
			cel.MemberOverload("qualifier_"+fn, []*cel.Type{cel.DynType, cel.StringType}, resultOf(e.qualifiers[fn]),
				cel.BinaryBinding(func(recv, arg ref.Val) ref.Val {
					s, _ := arg.Value().(string)
					return delivered(recv, template.Call(fn, s))
				}))))
	}
	return out
}

// taken reports whether base already resolves name in any of the shapes a call
// can take, as a function, a member function or a macro. cel-go does not list an
// environment's functions, so each shape is compiled.
func taken(base *cel.Env, name string) bool {
	for _, shape := range []string{
		`%[1]s(x)`, `%[1]s(x, "a")`, `%[1]s(x.y)`,
		`x.%[1]s()`, `x.%[1]s("a")`, `x.%[1]s("a", "b")`, `x.%[1]s(v, v)`,
	} {
		if _, iss := base.Compile(fmt.Sprintf(shape, name)); iss == nil || iss.Err() == nil {
			return true
		}
	}
	return false
}

func resultOf(q Qualifier) *cel.Type {
	if q.Result == nil {
		return cel.DynType
	}
	return q.Result
}

// delivered looks a qualifier call up in what was delivered with its receiver.
func delivered(recv ref.Val, call string) ref.Val {
	fn, arg := template.SplitCall(call)
	notDelivered := types.NewErr("read .%s(%q) was not delivered to this render", fn, arg)
	m, ok := recv.(traits.Mapper)
	if !ok {
		return notDelivered
	}
	calls, found := m.Find(types.String(QualifiedKey))
	if !found {
		return notDelivered
	}
	cm, ok := calls.(traits.Mapper)
	if !ok {
		return notDelivered
	}
	v, found := cm.Find(types.String(call))
	if !found {
		return notDelivered
	}
	return v
}

// compiled is one expression's compilation: the AST References walks and the
// program Eval runs. Both are read-only once built.
type compiled struct {
	ast *cel.Ast
	prg cel.Program
}

// compile returns expr compiled against env, reusing an earlier compilation
// where it can.
//
// Only the engine's own permissive environment is cached. A typed environment
// declares each root's real shape, so the same text compiles to a different
// result there, and a dyn-typed program would disable its target-type check.
// Failures are not cached, so an error's text reflects the call that made it.
func (e *Engine) compile(env *cel.Env, expr string) (*compiled, error) {
	cacheable := env == e.dyn
	if cacheable {
		if hit, ok := e.compiled.Get(expr); ok {
			if c, ok := hit.(*compiled); ok {
				return c, nil
			}
		}
	}
	ast, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, iss.Err()
	}
	prg, err := env.Program(ast)
	if err != nil {
		return nil, err
	}
	c := &compiled{ast: ast, prg: prg}
	if cacheable {
		e.compiled.Add(expr, c)
	}
	return c, nil
}
