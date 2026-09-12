// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package coverage asks what a body of shell cases never mentions.
//
// A green suite says the cases somebody thought to write pass. It cannot tell
// a surface that is covered and correct from one nothing ever asked about, and
// this repository has been bitten by that twice in the same shape: `make
// axis-sweep` found 111 axis/dialect pairs nothing objected to (#2031), and
// `$'…'`, `+=`, C-style `for` and `function` were all broken while the feature
// matrix listed them as core — found by running scripts, because nothing in
// the corpus used them (#2293).
//
// So the question here is the denominator: enumerate the surface, then name
// every element no case mentions.
//
// # Total by construction, or it is worse than nothing
//
// Nothing in this package is a hand-kept list. The elements come from the tree
// itself:
//
//   - the **builtins** come from the dispatcher, through
//     [interp.Runner.BuiltinNames], and are asked of a runner the dialect
//     built — so a builtin a dialect registers or a prelude replaces is in
//     that dialect's surface and in no other's. Reading the literal in
//     `interp/builtin.go` would report a shell smaller than the one that runs.
//   - the **node kinds** are the types in `syntax` that have a `Pos` method,
//     read out of the source. A node added to the grammar joins the surface
//     without anybody remembering to add it.
//   - the **operator vocabularies** are the constants of every named integer
//     type that the tree actually carries — `Kind` on a redirection and on a
//     binary expression, `ParamOp` in a parameter expansion, `SpanKind` on a
//     word's spans. Which types those are is itself discovered, by reflection
//     over the node types, rather than listed.
//   - the **condition operators** are the package-level `map[string]bool`
//     tables in `syntax/cond.go`, plus the dialect-gated words in
//     `condUnaryOp`'s own switch.
//
// A hand-kept list is the failure this instrument exists to report, so it may
// not have one of its own.
//
// # What a mention is, and what it is not
//
// This counts **mentions**, and a mention is not coverage. A case that names
// `printf` and checks only that it exited 0 covers nothing, and this package
// scores it exactly as it scores a case that pins every conversion. The report
// says so in as many words, because a coverage number that overstates itself
// is worse than none: it retires the question.
//
// What it is good for is the other direction, which is not a proxy at all. An
// element with **zero** mentions is not covered by anything, and that is a
// fact rather than an estimate. The output is the work-list.
//
// # What is deliberately not in the denominator
//
// The `Semantics` axes. `make axis-sweep` already enumerates them and answers
// a strictly better question — it *moves* each one and reports what fails to
// object — and a count of cases mentioning an axis could not be computed at
// all, since an axis leaves no mark in the text. Listing them here with a
// made-up numerator would be the overstatement this package warns about.
package coverage

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Kinds of surface element. The report groups by these and they are printed
// as written.
const (
	KindBuiltin   = "builtin"
	KindNode      = "node kind"
	KindCondOp    = "condition operator"
	KindRedirOp   = "redirection operator"
	KindOperator  = "operator" // suffixed with the Go type: "operator (ParamOp)"
	kindOpPattern = "operator (%s)"
)

// Element is one thing the shell has that a case could ask about.
type Element struct {
	// Kind groups the element in the report.
	Kind string
	// Name is how it is spelled — a builtin's name, a node type, a constant.
	Name string
}

// OperatorKind names the group an operator constant of type t belongs to.
func OperatorKind(t string) string { return fmt.Sprintf(kindOpPattern, t) }

func (e Element) String() string { return e.Kind + " " + e.Name }

// byKindThenName is the order every listing uses.
func byKindThenName(es []Element) {
	sort.Slice(es, func(i, j int) bool {
		if es[i].Kind != es[j].Kind {
			return es[i].Kind < es[j].Kind
		}
		return es[i].Name < es[j].Name
	})
}

// sourceDir is where the `syntax` package's source is read from. It is found
// relative to this file at run time so the instrument works from any working
// directory, and is overridable for a test that wants a fixture.
var (
	sourceOnce sync.Once
	sourceDir  string
	sourceErr  error
	syntaxPkg  map[string]*ast.File
)

// SetSyntaxSourceDir points the source reader somewhere else. For tests.
func SetSyntaxSourceDir(dir string) {
	sourceOnce = sync.Once{}
	sourceDir = dir
}

func syntaxSource() (map[string]*ast.File, error) {
	sourceOnce.Do(readSyntaxSource)
	return syntaxPkg, sourceErr
}

func readSyntaxSource() {
	dir := sourceDir
	if dir == "" {
		_, self, _, ok := runtime.Caller(0)
		if !ok {
			sourceErr = fmt.Errorf("coverage: cannot locate this package's own source")
			return
		}
		// internal/coverage/surface.go -> the module root -> syntax.
		dir = filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(self))), "syntax")
	}
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		sourceErr = fmt.Errorf("coverage: reading %s: %w", dir, err)
		return
	}
	p, ok := pkgs["syntax"]
	if !ok {
		sourceErr = fmt.Errorf("coverage: %s holds no package named syntax", dir)
		return
	}
	syntaxPkg = p.Files
}

// NodeTypes is every type in `syntax` with a `Pos` method, which is what the
// package's own Node interface requires and so is the enumeration of the
// grammar's shapes.
//
// Read from the source rather than from a registry because there is no
// registry: a type joins the tree by having the method, and anything this
// package kept alongside would be the hand-written list it exists to report.
func NodeTypes() ([]string, error) {
	files, err := syntaxSource()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "Pos" || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			name := receiverTypeName(fn.Recv.List[0].Type)
			if name == "" || !ast.IsExported(name) {
				continue
			}
			seen[name] = true
		}
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("coverage: no node types found — the `Pos` method is how they are recognized, so this means the source was not read")
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

func receiverTypeName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return receiverTypeName(x.X)
	case *ast.Ident:
		return x.Name
	}
	return ""
}

// ConstantNames maps each named integer type declared in `syntax` to its
// constants, by value.
//
// iota is resolved the way the language does: a const block's expression list
// repeats, so a block of bare names counts up from zero. Only the shapes this
// package needs are understood — a bare name, an explicit integer, and
// `iota` — and anything else is an error rather than a silent gap, for the
// reason the package comment gives.
func ConstantNames() (map[string]map[int64]string, error) {
	files, err := syntaxSource()
	if err != nil {
		return nil, err
	}
	out := map[string]map[int64]string{}
	for _, f := range files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			typeName := ""
			next := int64(0)
			iotaBlock := false
			for i, s := range gd.Specs {
				vs, ok := s.(*ast.ValueSpec)
				if !ok {
					continue
				}
				if vs.Type != nil {
					id, ok := vs.Type.(*ast.Ident)
					if !ok {
						typeName = ""
						continue
					}
					typeName = id.Name
				}
				if typeName == "" {
					continue
				}
				switch {
				case len(vs.Values) == 1:
					v, isIota, ok := constValue(vs.Values[0])
					if !ok {
						// An expression this reader does not understand. The
						// numbering from here on would be a guess, so the
						// type is abandoned rather than half-recorded.
						delete(out, typeName)
						typeName = ""
						continue
					}
					if isIota {
						iotaBlock = true
						next = int64(i)
					} else {
						next = v
					}
				case len(vs.Values) == 0 && !iotaBlock && i > 0:
					// A repeated expression that was not iota: the value
					// does not advance and names would collide.
					continue
				}
				for _, n := range vs.Names {
					if n.Name == "_" {
						next++
						continue
					}
					if out[typeName] == nil {
						out[typeName] = map[int64]string{}
					}
					if _, taken := out[typeName][next]; !taken {
						out[typeName][next] = n.Name
					}
					next++
				}
			}
		}
	}
	return out, nil
}

func constValue(e ast.Expr) (v int64, isIota, ok bool) {
	switch x := e.(type) {
	case *ast.Ident:
		if x.Name == "iota" {
			return 0, true, true
		}
		return 0, false, false
	case *ast.BasicLit:
		if x.Kind != token.INT {
			return 0, false, false
		}
		n, err := strconv.ParseInt(x.Value, 0, 64)
		if err != nil {
			return 0, false, false
		}
		return n, false, true
	}
	return 0, false, false
}

// CondOperators is the words `[[ … ]]` accepts as tests: the package-level
// tables in `syntax/cond.go` and the dialect-gated words its own switch
// names.
//
// Every package-level `map[string]bool` in that file counts, rather than the
// two this was written against by name: a third table added beside them joins
// the surface without an edit here.
func CondOperators() ([]string, error) {
	files, err := syntaxSource()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for name, f := range files {
		if filepath.Base(name) != "cond.go" {
			continue
		}
		for _, d := range f.Decls {
			switch x := d.(type) {
			case *ast.GenDecl:
				if x.Tok != token.VAR {
					continue
				}
				for _, s := range x.Specs {
					vs, ok := s.(*ast.ValueSpec)
					if !ok || len(vs.Values) != 1 {
						continue
					}
					lit, ok := vs.Values[0].(*ast.CompositeLit)
					if !ok || !isStringBoolMap(lit.Type) {
						continue
					}
					for _, el := range lit.Elts {
						kv, ok := el.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						if s, ok := stringLit(kv.Key); ok {
							seen[s] = true
						}
					}
				}
			case *ast.FuncDecl:
				// The dialect-gated words, which are in a switch rather than
				// in a table because whether they are operators at all is a
				// grammar flag.
				if !strings.HasPrefix(x.Name.Name, "cond") || x.Body == nil {
					continue
				}
				ast.Inspect(x.Body, func(n ast.Node) bool {
					cc, ok := n.(*ast.CaseClause)
					if !ok {
						return true
					}
					for _, e := range cc.List {
						if s, ok := stringLit(e); ok && strings.HasPrefix(s, "-") {
							seen[s] = true
						}
					}
					return true
				})
			}
		}
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("coverage: no condition operators found in syntax/cond.go — the tables are how they are recognized, so this means the file was not read")
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out, nil
}

func isStringBoolMap(e ast.Expr) bool {
	m, ok := e.(*ast.MapType)
	if !ok {
		return false
	}
	k, ok := m.Key.(*ast.Ident)
	if !ok || k.Name != "string" {
		return false
	}
	v, ok := m.Value.(*ast.Ident)
	return ok && v.Name == "bool"
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// operatorTypes reports the named integer types the tree actually carries, by
// reflecting over the node types rather than by naming them.
//
// A type declared in `syntax` that no node holds is not part of the surface a
// case can mention — it would be an element with no way to reach it, and a
// permanent zero in the report is a false work item.
func operatorTypes(nodes []reflect.Type) map[string]bool {
	out := map[string]bool{}
	var walk func(reflect.Type, map[reflect.Type]bool)
	walk = func(t reflect.Type, seen map[reflect.Type]bool) {
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
			t = t.Elem()
		}
		if seen[t] {
			return
		}
		seen[t] = true
		if t.Kind() != reflect.Struct {
			return
		}
		for i := range t.NumField() {
			f := t.Field(i)
			ft := f.Type
			switch ft.Kind() {
			case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
				reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				if ft.PkgPath() != "" && ft.Name() != "" && ft.PkgPath() == syntaxPkgPath {
					out[ft.Name()] = true
				}
			default:
				walk(ft, seen)
			}
		}
	}
	seen := map[reflect.Type]bool{}
	for _, t := range nodes {
		walk(t, seen)
	}
	return out
}

const syntaxPkgPath = "github.com/blairham/sh/syntax"
