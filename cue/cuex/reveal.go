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
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
)

// revealers says which calls can reveal another once they have run, so a round
// stops after one and the value is searched again before the next call: a call
// written ahead of the rest runs ahead of them, and a call that ends the
// resolve, such as a workflow step's wait, must not run before a call that a
// comprehension writes once an earlier call has answered.
//
// A call reveals one where a comprehension whose body can hold a call, or a
// label computed for a field that can, reads the call's result. That is read
// from the syntax by name: what the comprehension's clauses or the label name,
// and what the fields so named read in turn. Names are not scoped, so two
// fields sharing a name are taken for one another, which costs a search and
// never an order. A call under any field so named reveals.
type revealers struct {
	// all is set where the syntax cannot say: every call reveals.
	all   bool
	names map[string]bool
}

// after reports whether the value must be searched again once call has run.
func (r *revealers) after(call pendingCall) bool {
	if r == nil {
		return false
	}
	if r.all {
		return true
	}
	for _, sel := range call.path.Selectors() {
		if name, ok := selectorName(sel); ok && r.names[name] {
			return true
		}
	}
	return false
}

func selectorName(sel cue.Selector) (string, bool) {
	switch sel.LabelType() {
	case cue.StringLabel:
		return sel.Unquoted(), true
	case cue.DefinitionLabel, cue.HiddenLabel, cue.HiddenDefinitionLabel:
		return sel.String(), true
	}
	return "", false
}

// revealGraph is what one file's syntax says: the names a revealing clause or
// label reads, and what each field reads, by name.
type revealGraph struct {
	seeds map[string]bool
	reads map[string]map[string]bool
}

func newRevealGraph() *revealGraph {
	return &revealGraph{seeds: map[string]bool{}, reads: map[string]map[string]bool{}}
}

func (g *revealGraph) merge(other *revealGraph) {
	for name := range other.seeds {
		g.seeds[name] = true
	}
	for name, reads := range other.reads {
		into := g.reads[name]
		if into == nil {
			into = map[string]bool{}
			g.reads[name] = into
		}
		for read := range reads {
			into[read] = true
		}
	}
}

// close follows what the seeds read, by name, to everything they depend on.
func (g *revealGraph) close() *revealers {
	names := map[string]bool{}
	queue := make([]string, 0, len(g.seeds))
	for name := range g.seeds {
		queue = append(queue, name)
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if names[name] {
			continue
		}
		names[name] = true
		for read := range g.reads[name] {
			queue = append(queue, read)
		}
	}
	return &revealers{names: names}
}

// readReveals reads one file.
func readReveals(f *ast.File) *revealGraph {
	g := newRevealGraph()
	ast.Walk(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Field:
			label := node.Label
			if alias, ok := label.(*ast.Alias); ok {
				// X=name: reading X reads the field
				g.addReads(alias.Ident.Name, node.Value)
				if expr, ok := alias.Expr.(ast.Label); ok {
					label = expr
				}
			}
			if name, _, err := ast.LabelName(label); err == nil && name != "" {
				g.addReads(name, node.Value)
			}
			switch label := label.(type) {
			case *ast.Interpolation, *ast.ParenExpr:
				if mayHoldCall(node.Value) {
					g.addSeeds(label)
				}
			}
		case *ast.LetClause:
			g.addReads(node.Ident.Name, node.Expr)
		case *ast.Comprehension:
			if mayHoldCall(node.Value) {
				for _, clause := range node.Clauses {
					g.addSeeds(clause)
				}
			}
		}
		return true
	}, nil)
	return g
}

func (g *revealGraph) addSeeds(n ast.Node) {
	for name := range identifiers(n) {
		g.seeds[name] = true
	}
}

func (g *revealGraph) addReads(name string, n ast.Node) {
	reads := g.reads[name]
	if reads == nil {
		reads = map[string]bool{}
		g.reads[name] = reads
	}
	for read := range identifiers(n) {
		reads[read] = true
	}
}

// identifiers is every name an expression reads: the start of each reference,
// and each field a selector goes through.
func identifiers(n ast.Node) map[string]bool {
	names := map[string]bool{}
	ast.Walk(n, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok {
			names[ident.Name] = true
		}
		return true
	}, nil)
	return names
}

// mayHoldCall reports whether syntax can produce a provider call: a call is a
// definition's, so syntax that names no definition and no #do produces none.
func mayHoldCall(n ast.Node) bool {
	found := false
	ast.Walk(n, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && strings.HasPrefix(ident.Name, "#") {
			found = true
		}
		if field, ok := n.(*ast.Field); ok {
			if name, _, err := ast.LabelName(field.Label); err == nil && name == doKey {
				found = true
			}
		}
		return !found
	}, nil)
	return found
}

// revealersOf reads the template and the packages it imports. Where a mutation
// or a filled cue.Value can put a call the syntax does not show, every call
// reveals.
func (in *Compiler) revealersOf(f *ast.File, cfg *CompileConfig, imports []*build.Instance) *revealers {
	if len(cfg.IntraResolveMutators) > 0 || carriesValueData(cfg) {
		return &revealers{all: true}
	}
	g := readReveals(f)
	for _, imported := range imports {
		read, ok := in.packageReveals(imported)
		if !ok {
			return &revealers{all: true}
		}
		g.merge(read)
	}
	return g.close()
}

// packageRevealGraph is a package's reading, kept with the instance it was read
// from, as packageGrowth is.
type packageRevealGraph struct {
	instance *build.Instance
	graph    *revealGraph
}

// packageReveals reads a package once and remembers it.
func (in *Compiler) packageReveals(imported *build.Instance) (*revealGraph, bool) {
	if imported == nil {
		return nil, false
	}
	path := imported.ImportPath
	if path != "" {
		if known, ok := in.packageRevealGraphs.Load(path); ok {
			if read := known.(packageRevealGraph); read.instance == imported {
				return read.graph, true
			}
		}
	}
	g := newRevealGraph()
	for _, file := range imported.Files {
		g.merge(readReveals(file))
	}
	if path != "" {
		in.packageRevealGraphs.Store(path, packageRevealGraph{instance: imported, graph: g})
	}
	return g, true
}
