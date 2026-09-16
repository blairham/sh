// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package coverage

import (
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"sort"
	"strings"

	"github.com/blairham/sh/syntax"
)

// OperatorTypes is every named type a node carries that has constants of its
// own — the operator vocabularies.
//
// Derived rather than named: a field of a node, whose declared type is a type
// this package found constants for. So `Kind` is in because a redirection and
// a binary expression each hold one, `ParamOp` because a parameter expansion
// does, and a type declared in `syntax` that no node holds is out, because a
// case has no way to mention it and a permanent zero in a report is a false
// work item.
//
// And a type that [syntax.Dialect] also carries is out, because its constants
// are not something a case spells. They are a grammar's *reading*: the same
// text `"${v-'a}b'}"` is read three ways by three dialects, and which way is
// decided by the dialect, not by the case. A node that keeps such a value
// keeps a copy of the reading — `ParamExpr.RawTailRead` is the one today, and
// the parse writes it only under a dialect whose POSIX mode moves the reading.
// Walking that field produced two wrong answers at once (#3258): a field the
// parse never set holds the type's zero, so `BraceQuoteProtectsAPatternOnly`
// read as mentioned under four dialects that never wrote it, and
// `BraceQuoteProtectsNothing` — zsh's reading, and never written to a node by
// any parse — was a permanent zero nobody could retire. A reflection walk
// cannot tell an unset field from one set to the zero, so the fix is not to
// ask it to. These readings are the same kind of fact as the Semantics axes
// the package comment already leaves out: they leave no mark in the text, and
// the corpus rows each Dialect field cites are what measure them.
//
// Derived, not listed: a type joins the exclusion by being the declared type
// of a Dialect field, so a reading added tomorrow and copied onto a node
// stays out of the surface without an edit here.
func OperatorTypes() ([]string, error) {
	files, err := syntaxSource()
	if err != nil {
		return nil, err
	}
	consts, err := ConstantNames()
	if err != nil {
		return nil, err
	}
	nodes, err := NodeTypes()
	if err != nil {
		return nil, err
	}
	isNode := map[string]bool{}
	for _, n := range nodes {
		isNode[n] = true
	}
	readings, err := ReadingTypes()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, f := range files {
		for _, st := range structTypes(f, func(name string) bool { return isNode[name] }) {
			for _, fld := range st.Fields.List {
				id, ok := fld.Type.(*ast.Ident)
				if !ok || readings[id.Name] {
					continue
				}
				if _, has := consts[id.Name]; has {
					seen[id.Name] = true
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

// ReadingTypes is every named type [syntax.Dialect] declares a field of —
// the grammar readings, which [OperatorTypes] leaves out.
//
// An empty answer is an error rather than an empty set: it can only mean the
// struct was not found, and an empty exclusion would put the readings back
// into the surface without a word.
func ReadingTypes() (map[string]bool, error) {
	files, err := syntaxSource()
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, f := range files {
		for _, st := range structTypes(f, func(name string) bool { return name == "Dialect" }) {
			for _, fld := range st.Fields.List {
				if id, ok := fld.Type.(*ast.Ident); ok && ast.IsExported(id.Name) {
					out[id.Name] = true
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("coverage: syntax.Dialect declares no field of a named type — the struct is how the readings are recognized, so this means the source was not read")
	}
	return out, nil
}

// structTypes is every struct type declared in f whose name want accepts.
func structTypes(f *ast.File, want func(string) bool) []*ast.StructType {
	var out []*ast.StructType
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, s := range gd.Specs {
			ts, ok := s.(*ast.TypeSpec)
			if !ok || !want(ts.Name.Name) {
				continue
			}
			if st, ok := ts.Type.(*ast.StructType); ok && st.Fields != nil {
				out = append(out, st)
			}
		}
	}
	return out
}

// Surface is every element of one dialect's surface.
//
// builtins is what that dialect's runner reports, which is why it is a
// parameter rather than something this package reaches for: the surface is
// per dialect, and a builtin a dialect registers belongs to it alone.
func Surface(builtins []string) ([]Element, error) {
	var out []Element
	for _, b := range builtins {
		out = append(out, Element{Kind: KindBuiltin, Name: b})
	}
	nodes, err := NodeTypes()
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		out = append(out, Element{Kind: KindNode, Name: n})
	}
	ops, err := OperatorTypes()
	if err != nil {
		return nil, err
	}
	consts, err := ConstantNames()
	if err != nil {
		return nil, err
	}
	for _, t := range ops {
		for value, name := range consts[t] {
			kind, ok := operatorElementKind(t, value)
			if !ok {
				continue
			}
			out = append(out, Element{Kind: kind, Name: name})
		}
	}
	conds, err := CondOperators()
	if err != nil {
		return nil, err
	}
	for _, c := range conds {
		out = append(out, Element{Kind: KindCondOp, Name: c})
	}
	byKindThenName(out)
	return out, nil
}

// operatorElementKind says which group a constant belongs in, and whether it
// is part of the surface at all.
//
// Every operator type but one is wholly reachable: each `ParamOp` is an
// operator a parameter expansion can hold, each `SpanKind` a shape a word's
// span can be. `Kind` is the exception and is not an operator type at all —
// it is the lexer's entire token vocabulary, and most of it never reaches a
// node. Listing `TokSemicolon` as an element nothing mentions would be a
// permanent zero and a work item nobody can do.
//
// So the subset is taken from the shell's own answer rather than from a list
// here: [syntax.Kind.IsRedirect] is what the parser asks when it lifts a
// redirection out of a word list, so an operator added to that switch joins
// this surface with it. The two list operators a BinaryExpr carries are left
// out on purpose — `&&` and `||` are the whole of them and the BinaryExpr
// node kind already stands for the shape.
func operatorElementKind(typeName string, value int64) (string, bool) {
	if typeName != "Kind" {
		return OperatorKind(typeName), true
	}
	if syntax.Kind(value).IsRedirect() {
		return KindRedirOp, true
	}
	return "", false
}

// Mentions counts, for one parsed file, every surface element its text
// reaches.
//
// A *mention* and not a test of the element: see the package comment. What it
// is reliable for is the zero.
func Mentions(f *syntax.File, isBuiltin func(string) bool, into map[Element]int) error {
	consts, err := ConstantNames()
	if err != nil {
		return err
	}
	nodes, err := NodeTypes()
	if err != nil {
		return err
	}
	isNode := map[string]bool{}
	for _, n := range nodes {
		isNode[n] = true
	}
	conds, err := CondOperators()
	if err != nil {
		return err
	}
	isCondOp := make(map[string]bool, len(conds))
	for _, c := range conds {
		isCondOp[c] = true
	}
	ops, err := OperatorTypes()
	if err != nil {
		return err
	}
	// Only the vocabularies the surface holds. A field of any other named
	// type is a reading the grammar copied onto the node (see
	// [OperatorTypes]), and recording its value as a mention — even of an
	// element no report lists — is the false mention #3258 found, one lookup
	// away from being printed.
	opConsts := make(map[string]map[int64]string, len(ops))
	for _, o := range ops {
		opConsts[o] = consts[o]
	}
	m := &mentioner{consts: opConsts, isNode: isNode, isBuiltin: isBuiltin, isCondOp: isCondOp, into: into}
	m.value(reflect.ValueOf(f), map[uintptr]bool{})
	return nil
}

type mentioner struct {
	consts    map[string]map[int64]string
	isNode    map[string]bool
	isBuiltin func(string) bool
	isCondOp  map[string]bool
	into      map[Element]int
}

func (m *mentioner) value(v reflect.Value, seen map[uintptr]bool) {
	if !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return
		}
		if v.Kind() == reflect.Pointer {
			// A tree is acyclic, but a shared child would otherwise be
			// counted twice and a cycle would not return at all.
			p := v.Pointer()
			if seen[p] {
				return
			}
			seen[p] = true
		}
		m.value(v.Elem(), seen)
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			m.value(v.Index(i), seen)
		}
	case reflect.Struct:
		t := v.Type()
		if t.PkgPath() == syntaxPkgPath && m.isNode[t.Name()] {
			m.into[Element{Kind: KindNode, Name: t.Name()}]++
		}
		if t.PkgPath() == syntaxPkgPath && t.Name() == "SimpleCmd" {
			m.command(v)
		}
		for i := range t.NumField() {
			f := t.Field(i)
			ft := f.Type
			if ft.PkgPath() == syntaxPkgPath && ft.Name() != "" && isInteger(ft.Kind()) {
				if names, ok := m.consts[ft.Name()]; ok {
					val := asInt(v.Field(i))
					if name, ok := names[val]; ok {
						if kind, ok := operatorElementKind(ft.Name(), val); ok {
							m.into[Element{Kind: kind, Name: name}]++
						}
					}
				}
				continue
			}
			if ft.Kind() == reflect.String && f.Name == "Op" {
				// The condition and arithmetic operators are words rather
				// than constants, so the value *is* the name.
				if s := v.Field(i).String(); s != "" {
					m.into[Element{Kind: KindCondOp, Name: s}]++
				}
				continue
			}
			m.value(v.Field(i), seen)
		}
	}
}

// command records the builtins a simple command names.
//
// The command word only. A builtin's name appearing as an argument is not the
// case asking about the builtin — `echo read` says nothing about `read` — and
// counting it would be the overstatement this package is otherwise careful
// about.
func (m *mentioner) command(v reflect.Value) {
	args := v.FieldByName("Args")
	if !args.IsValid() || args.Len() == 0 {
		return
	}
	name, ok := literalWord(args.Index(0))
	if !ok {
		return
	}
	if m.isBuiltin(name) {
		m.into[Element{Kind: KindBuiltin, Name: name}]++
	}
	if name != "test" && name != "[" {
		return
	}
	// `[ -f x ]` and `[[ -f x ]]` ask about the same operator, and only the
	// second parses into a node with an Op field. Counting the builtin's
	// operand words too is what keeps a corpus written in the portable
	// spelling from reading as though it had never tested a file at all.
	for i := 1; i < args.Len(); i++ {
		w, ok := literalWord(args.Index(i))
		if !ok || !m.isCondOp[w] {
			continue
		}
		m.into[Element{Kind: KindCondOp, Name: w}]++
	}
}

// literalWord reports a word that is one unquoted literal span, which is the
// only shape whose text is known before the shell runs.
func literalWord(v reflect.Value) (string, bool) {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return "", false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return "", false
	}
	parts := v.FieldByName("Parts")
	if !parts.IsValid() {
		parts = v.FieldByName("Spans")
	}
	if !parts.IsValid() {
		return "", false
	}
	var b strings.Builder
	for i := range parts.Len() {
		p := parts.Index(i)
		kind := p.FieldByName("Kind")
		val := p.FieldByName("Value")
		if !kind.IsValid() || !val.IsValid() {
			return "", false
		}
		if asInt(kind) != 0 {
			// Not a plain literal span: the text is not known until it runs.
			return "", false
		}
		b.WriteString(val.String())
	}
	if b.Len() == 0 {
		return "", false
	}
	return b.String(), true
}

func isInteger(k reflect.Kind) bool {
	switch k {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	}
	return false
}

func asInt(v reflect.Value) int64 {
	if isInteger(v.Kind()) {
		switch v.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return int64(v.Uint())
		default:
			return v.Int()
		}
	}
	return -1
}

// ErrNoSurface is returned when the enumeration came back empty, which can
// only mean the source was not read.
var ErrNoSurface = fmt.Errorf("coverage: the surface came back empty")
