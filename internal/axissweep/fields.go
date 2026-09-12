// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package axissweep asks, of every axis in interp.Semantics, whether anything
// fails when it is moved.
//
// An axis exists to record a *measured disagreement between real shells*. So
// an axis nothing objects to is not merely an untested code path: it asserts
// a fact nobody checked, and the next reader takes it as evidence that the
// measurement happened. Four were found vacuous in a single day (#2031), each
// discovered by accident, by an agent editing the line next to it. Nobody had
// ever swept the struct.
//
// # Total by construction
//
// This is the third instance of a shape the tree has already recorded twice —
// #1416, a reflect.Map walk over one struct while the tables behind a slice
// escaped it, and #1808, a boundary guard that read only the packages holding
// a Boundary while every dialect escaped it. Both were a guarantee that was
// *checked* rather than *enumerated*, and so was only ever as wide as the set
// it walked.
//
// So nothing here is hand-kept. The fields come from reflection over the
// struct, and the values a field may take come from the constants declared in
// the source, parsed. A field whose type this package does not know how to
// move is an **error**, never a skip: a sweep that quietly passes over a field
// reports it as "nothing objected", which is a bug filed against something
// that was never touched — the one misreading that would make the instrument
// worse than nothing.
package axissweep

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Field is one axis: a dotted path into the struct and the type it holds.
type Field struct {
	// Path is the field name, or the dotted route to it through a struct
	// the vector holds by value.
	Path string
	// Type is the declared type's name, as written — "Answer", "bool",
	// "TrapLocality". Used to find the constants that type declares.
	Type string
	// Kind is what reflection can do with it.
	Kind reflect.Kind
	// Doc is the field's leading comment, first sentence, so a report can
	// say what the axis claims without the reader opening the file.
	Doc string
}

// Value is one legal setting of a field.
type Value struct {
	// Name is how the source spells it — a constant name, `true`, a quoted
	// string. It is what a report prints.
	Name string
	// Literal is what internal/axismutate parses: a decimal for any integer
	// kind, `true`/`false`, or the string itself.
	Literal string
	// Unspecified marks the "no shell has been chosen here" constant, which
	// nearly every answer type declares and names for what it is.
	//
	// A flip to or from it is a different question from a flip between two
	// answers. Moving a specified axis to Unspecified makes the shell refuse
	// wherever it is consulted, so it measures whether the axis is *reached*
	// — which almost everything is. Whether the corpus can tell Yes from No
	// is the question that found the four vacuous axes, and only a flip
	// between two specified values asks it.
	Unspecified bool
}

// Fields enumerates every axis of a settings struct, in declaration order.
//
// Structs held by value are walked into rather than treated as one field:
// StartupFileOptions.Login is an axis in its own right, and a sweep that
// moved the struct as a unit could not say which of its fields nothing
// objected to.
func Fields(t reflect.Type) ([]Field, error) {
	docs, err := fieldDocs()
	if err != nil {
		return nil, err
	}
	var out []Field
	var walk func(t reflect.Type, prefix string) error
	walk = func(t reflect.Type, prefix string) error {
		if t.Kind() != reflect.Struct {
			return fmt.Errorf("%s: want a struct, got %s", prefix, t.Kind())
		}
		for i := range t.NumField() {
			f := t.Field(i)
			if f.PkgPath != "" {
				return fmt.Errorf("%s%s: unexported, and nothing outside the package can move it", prefix, f.Name)
			}
			path := prefix + f.Name
			if f.Type.Kind() == reflect.Struct {
				if err := walk(f.Type, path+"."); err != nil {
					return err
				}
				continue
			}
			out = append(out, Field{
				Path: path,
				Type: typeName(f.Type),
				Kind: f.Type.Kind(),
				Doc:  docs[path],
			})
		}
		return nil
	}
	if err := walk(t, ""); err != nil {
		return nil, err
	}
	return out, nil
}

// typeName is the type as the source writes it, without the package
// qualifier: reflection says "interp.Answer" and the constants are declared
// against "Answer".
func typeName(t reflect.Type) string {
	if t.Name() == "" {
		return t.String()
	}
	return t.Name()
}

// Values are the settings a field may legally take, other than the one it
// holds now.
//
// Where the type is a named integer, they are every constant the source
// declares of that type — which is the enumeration being derived rather than
// kept, so an axis that gains a fourth answer tomorrow is swept with four.
// bool has two. A string has no enumerable set, so it gets two probes that
// are certainly different from whatever it holds: empty, which is how every
// string axis here spells "this shell has no such thing", and a word no shell
// has a meaning for.
//
// An unrecognized kind is an error. See the package comment.
func Values(f Field, current reflect.Value) ([]Value, error) {
	var all []Value
	switch f.Kind {
	case reflect.Bool:
		all = []Value{{Name: "false", Literal: "false"}, {Name: "true", Literal: "true"}}
	case reflect.String:
		all = []Value{{Name: `""`, Literal: ""}, {Name: strconv.Quote(stringProbe), Literal: stringProbe}}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var err error
		all, err = intValues(f)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%s: nothing here knows the legal values of a %s", f.Path, f.Kind)
	}
	held := literalOf(current)
	var out []Value
	for _, v := range all {
		if v.Literal != held {
			out = append(out, v)
		}
	}
	return out, nil
}

// stringProbe is a word no shell gives a meaning to, so a string axis moved
// to it has certainly been moved.
const stringProbe = "axis-sweep-probe"

// intValues are the constants the source declares of a named integer type,
// or — for a plain int, of which the vector holds one — a small fixed set
// that certainly includes a value the field does not hold.
func intValues(f Field) ([]Value, error) {
	if f.Type == "int" {
		return []Value{
			{Name: "0", Literal: "0"},
			{Name: "1", Literal: "1"},
			{Name: "2", Literal: "2"},
		}, nil
	}
	consts, err := typeConstants()
	if err != nil {
		return nil, err
	}
	named := consts[f.Type]
	if len(named) == 0 {
		return nil, fmt.Errorf("%s: no constants of type %s are declared in %s, so the sweep cannot enumerate its values", f.Path, f.Type, sourceDir)
	}
	return named, nil
}

func literalOf(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	default:
		return ""
	}
}

// At walks a dotted path into a struct value.
func At(v reflect.Value, path string) (reflect.Value, error) {
	for _, name := range strings.Split(path, ".") {
		if v.Kind() != reflect.Struct {
			return reflect.Value{}, fmt.Errorf("%s: %q is not inside a struct", path, name)
		}
		v = v.FieldByName(name)
		if !v.IsValid() {
			return reflect.Value{}, fmt.Errorf("%s: no field named %q", path, name)
		}
	}
	return v, nil
}

// sourceDir is the package whose declarations are read. It is a path relative
// to the module root, and the sweep is run from there.
var sourceDir = "interp"

// SetSourceDir points the constant and comment scans at another checkout,
// which only a test needs.
func SetSourceDir(dir string) { sourceDir = dir }

// The three scans below are each done once and shared. They are behind a
// sync.Once rather than a nil check because the callers are tests that run in
// parallel, and a lazily filled package-level map is a data race however
// obviously idempotent the work is.
var (
	parseOnce   sync.Once
	parsedFiles map[string]*ast.File
	parseErr    error
)

func sourceFiles() (map[string]*ast.File, error) {
	parseOnce.Do(readSource)
	return parsedFiles, parseErr
}

func readSource() {
	// One file at a time rather than parser.ParseDir, which is deprecated
	// for a reason that would bite here: it does not read build tags, so it
	// groups files into packages by guesswork. Everything this needs is in
	// one package's ordinary source, and the alternative it points at —
	// golang.org/x/tools/go/packages — would make a build-tool dependency
	// direct for a constant scan.
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		parseErr = fmt.Errorf("reading %s: %w", sourceDir, err)
		return
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(sourceDir, name)
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			parseErr = fmt.Errorf("reading %s: %w", path, err)
			return
		}
		files[path] = f
	}
	if len(files) == 0 {
		parseErr = fmt.Errorf("no Go files under %s", sourceDir)
		return
	}
	parsedFiles = files
}

var (
	constOnce  sync.Once
	constCache map[string][]Value
	constErr   error
)

// typeConstants collects, per named type, the constants the source declares
// of it.
//
// A const block carries its type down: a spec with no type of its own repeats
// the one above it, which is how `iota` blocks are written throughout, so the
// scan has to carry it too. The zero value of each type is marked, because
// every answer type here declares one and means the same thing by it.
func typeConstants() (map[string][]Value, error) {
	constOnce.Do(readConstants)
	return constCache, constErr
}

func readConstants() {
	files, err := sourceFiles()
	if err != nil {
		constErr = err
		return
	}
	out := map[string][]Value{}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, decl := range files[name].Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			typeName := ""
			index := 0
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				if vs.Type != nil {
					if id, ok := vs.Type.(*ast.Ident); ok {
						typeName = id.Name
					} else {
						typeName = ""
					}
					index = 0
				} else if len(vs.Values) > 0 {
					// A fresh expression with no type: not part of an iota
					// run this scan can count, so stop attributing.
					typeName = ""
				}
				if typeName == "" {
					continue
				}
				for _, id := range vs.Names {
					if id.Name != "_" {
						out[typeName] = append(out[typeName], Value{
							Name:        id.Name,
							Literal:     strconv.Itoa(index),
							Unspecified: strings.Contains(id.Name, "Unspecified"),
						})
					}
					index++
				}
			}
		}
	}
	constCache = out
}

var (
	docOnce  sync.Once
	docCache map[string]string
	docErr   error

	noteOnce  sync.Once
	noteCache map[string]Notes
	noteErr   error
)

// Notes are the triage recorded for one axis.
//
// The preset lists are not verdicts and never were — they are two questions
// asked of the struct, and each entry has to be answered by measuring the
// panel again. An answer that lives only in a pull request is one the next
// sweep cannot see, so it is written down where the sweep can read it back:
// triage.json, keyed by field path.
//
//	"SomeAxis": {
//	  "unanimous":   "why the axis records something even so",
//	  "unexhibited": {"SomeConstant": "who holds it, and what measured that"},
//	  "unpinned":    {"zsh": "why no corpus row objects when it moves"}
//	}
//
// It is a file of its own rather than a line in the field's doc comment
// because every one of these verdicts names a shell, and a comment under
// interp or syntax may not: the core defines the questions and never says who
// answers them which way. The names belong beside the instrument that needs
// them, which is here.
//
// A field with no entry is **untriaged**, which is the state worth reporting:
// it separates the axes somebody has re-measured from the ones nobody has
// looked at yet, and those two used to be spelled identically.
type Notes struct {
	// Unanimous is why an axis every dialect answers alike is an axis
	// anyway.
	Unanimous string `json:"unanimous,omitempty"`
	// Value is, per legal value no dialect holds, who does hold it.
	Value map[string]string `json:"unexhibited,omitempty"`
	// Unpinned is, per dialect, why the graded corpus does not object when
	// this axis moves — keyed by dialect name, or by "" for a reason that
	// holds for every dialect.
	//
	// The verdict a flip needs is not the one a preset list needs. There the
	// question was who holds a value; here it is which of two opposite fixes
	// applies, because an unpinned axis is *either* a missing corpus row or
	// a disagreement that is not there. A row landed answers it by making
	// the pair disappear from the list; what stays needs a standing reason,
	// and the reason worth writing is the one that says the corpus cannot
	// reach it rather than that nobody has got to it.
	Unpinned map[string]string `json:"unpinned,omitempty"`
}

// fieldDocs is each field's leading comment, trimmed to its first sentence.
func fieldDocs() (map[string]string, error) {
	docOnce.Do(readDocs)
	return docCache, docErr
}

// FieldNotes is the triage recorded for each field, by field path.
//
// The triage lives in triage.json rather than in the field's own doc comment,
// because what it records is *which shell holds a value no preset holds* — and
// naming a shell is the one thing a comment in interp or syntax may not do.
// The instrument that needs those names keeps them, next to itself.
func FieldNotes() (map[string]Notes, error) {
	noteOnce.Do(readNotes)
	return noteCache, noteErr
}

//go:embed triage.json
var triageJSON []byte

func readNotes() {
	var out map[string]Notes
	if err := json.Unmarshal(triageJSON, &out); err != nil {
		noteErr = fmt.Errorf("reading triage.json: %w", err)
		return
	}
	// A verdict keyed by a dialect that does not exist answers for nobody,
	// and silently reading it back would make the flip half report a
	// reason no sweep can act on. The comment grammar this replaced could
	// not express one; the file can, so it is refused here instead.
	for field, n := range out {
		for d := range n.Unpinned {
			if d == "" {
				continue
			}
			if !knownDialect(d) {
				noteErr = fmt.Errorf("triage.json: %s: unpinned verdict for %q, which is not one of the dialects", field, d)
				return
			}
		}
	}
	noteCache = out
}

// knownDialect reports whether a name is one the presets answer for. The
// triage file names a dialect only in the unpinned half, where the question is
// which shell's corpus rows fail to object.
func knownDialect(name string) bool {
	_, ok := dialectPresets()[name]
	return ok
}

func readDocs() {
	files, err := sourceFiles()
	if err != nil {
		docErr = err
		return
	}
	out := map[string]string{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			prefix := ""
			if ts.Name.Name != "Semantics" {
				prefix = ts.Name.Name + "."
			}
			for _, field := range st.Fields.List {
				doc := firstSentence(field.Doc.Text())
				for _, id := range field.Names {
					out[prefix+id.Name] = doc
					// Nested structs are recorded under both the outer
					// path and their own type name; the outer one wins
					// because Fields asks for it.
					if prefix == "" {
						out[id.Name] = doc
					}
				}
			}
			return true
		})
	}
	docCache = out
}

func firstSentence(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}
