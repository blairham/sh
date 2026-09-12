// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const dialectPrefix = "github.com/blairham/sh/dialect/"

// TestNothingHereIsAShell, including the tests.
//
// The substrate defines the questions and a dialect answers them. That the
// *code* obeys is structural — nothing under interp/ or syntax/ has ever
// imported a shell — but the tests did, and the direction of that dependency
// is the same inversion said quietly: `interp`'s tests could not compile
// unless `dialect/bash` did, and an edit to bash's preset changed what the
// substrate asserted about itself. Twelve files, seeded from bash's whole
// answer sheet, two of them while claiming in their own headers to name axes
// and never a shell (#491).
//
// So the rule covers the tests too, and is checked over the source rather than
// left to review. What replaced the seed is in vector_test.go: the standard's
// preset plus the axes these tests reach, and each of those is here because
// taking it out turns a test red.
//
// The syntax package is walked as well. Nothing there has ever imported a
// dialect either, and the cheapest moment to say so is before the first one
// does.
//
// **Imports were not the whole rule, and the guard used to think they were.**
// It parsed with parser.ImportsOnly, so an identifier named after a shell was
// invisible to it — and two survived that way. `bash := testSemantics()`, in
// interp_test.go and bgpanic_test.go, named a value that is the *standard's*
// preset plus the axes these tests reach and has never been bash's, so a
// reader who trusted the name read an assertion about a shell where there was
// none (#1300). An import creates a dependency and a name does not, but the
// name is what a reader uses to decide whether a test is in the right package.
//
// Only `bash`, `zsh` and `ksh`. `dash` is deliberately absent: it is also the
// **hyphen** throughout this codebase — `cd -`, a lone `-` as an option — so a
// guard that included it would fire on correct code and be turned off within a
// week. Comments are out of scope too, and by construction: 518 of the 560
// shell mentions in these tests are comments recording what was measured
// against which binary, and a differential project that cannot say which shell
// produced an expectation is worse off, not better.
func TestNothingHereIsAShell(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	files := 0
	for _, pkg := range []string{"interp", "syntax"} {
		dir := filepath.Join(root, pkg)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
		fset := token.NewFileSet()
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
				continue
			}
			files++
			path := filepath.Join(dir, e.Name())
			// A whole parse, not parser.ImportsOnly: the declaration check
			// below needs the bodies, and that is the point of it.
			f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parsing %s: %v", path, err)
			}
			for _, imported := range importPaths(t, f) {
				if strings.HasPrefix(imported, dialectPrefix) {
					t.Errorf("%s/%s imports %s.\n"+
						"\tThe substrate asks the questions and a dialect answers them, so a\n"+
						"\ttest here that borrows a shell's answer sheet has inverted that —\n"+
						"\tand a preset edit then changes what the substrate asserts.\n"+
						"\tBuild the vector locally: see testSemantics in vector_test.go.",
						pkg, e.Name(), imported)
				}
			}
			for _, d := range shellNamedDeclarations(f) {
				t.Errorf("%s/%s:%d declares %s named %q.\n"+
					"\tNothing here holds a shell's answer, so nothing here should be\n"+
					"\tnamed for one — a reader who trusts the name reads this as an\n"+
					"\tassertion about that shell, which is the inversion this test is\n"+
					"\tabout. Name the value: `sem`, `d`, whatever it holds.\n"+
					"\tA test that really is about a shell's answer belongs in dialect/%s.",
					pkg, e.Name(), fset.Position(d.pos).Line, d.kind, d.name, strings.ToLower(d.name))
			}
		}
	}
	// A walk that read nothing passes for the wrong reason.
	if files < 100 {
		t.Fatalf("the walk read %d Go files across interp/ and syntax/; it should be reading hundreds", files)
	}
}

// shellNames is the set the declaration check fires on. See the note on
// TestNothingHereIsAShell for why `dash` is not in it.
var shellNames = map[string]bool{"bash": true, "zsh": true, "ksh": true}

// declaration is one identifier the check objected to.
type declaration struct {
	pos  token.Pos
	kind string
	name string
}

// shellNamedDeclarations reports every identifier f *declares* whose name is a
// shell's.
//
// Declarations only, never uses. A bare ast.Ident walk would also match the
// `Sel` of a selector expression and a composite literal's field keys, so a
// struct field somebody else named would be reported against the file that
// merely mentions it — and a guard that fires on code its own package cannot
// fix is a guard that gets deleted. Every case below is a site where this
// package chose the name.
func shellNamedDeclarations(f *ast.File) []declaration {
	var found []declaration
	add := func(id *ast.Ident, kind string) {
		if id == nil || !shellNames[strings.ToLower(id.Name)] {
			return
		}
		found = append(found, declaration{pos: id.Pos(), kind: kind, name: id.Name})
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if x.Tok != token.DEFINE {
				return true
			}
			for _, lhs := range x.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					add(id, "a local")
				}
			}
		case *ast.ValueSpec:
			for _, id := range x.Names {
				add(id, "a variable or constant")
			}
		case *ast.TypeSpec:
			add(x.Name, "a type")
		case *ast.FuncDecl:
			add(x.Name, "a function")
		case *ast.Field:
			// Struct fields, parameters, results and receivers all arrive
			// here, and all four are this package's own naming.
			for _, id := range x.Names {
				add(id, "a field or parameter")
			}
		case *ast.RangeStmt:
			if x.Tok != token.DEFINE {
				return true
			}
			for _, e := range []ast.Expr{x.Key, x.Value} {
				if id, ok := e.(*ast.Ident); ok {
					add(id, "a range variable")
				}
			}
		case *ast.LabeledStmt:
			add(x.Label, "a label")
		}
		return true
	})
	return found
}

// TestTheShellNameGuardCatchesOneAndLeavesTheRestAlone is the other half of
// the declaration check, and the half that decides whether anyone keeps it.
//
// The source below holds one of every shape the check reports and one of every
// shape it must not: `dash` in the three spellings this codebase really uses
// for the hyphen, a *use* of a name declared elsewhere, and a field key on
// somebody else's struct. A detector that fired on those would be switched off
// long before it caught anything.
func TestTheShellNameGuardCatchesOneAndLeavesTheRestAlone(t *testing.T) {
	t.Parallel()
	const src = `package interp_test

type zsh struct{ bash int }            // line 3: a type, and a field

func ksh(bash int) {}                  // line 5: a function, and a parameter

func f() {
	bash := 1                      // line 8: a local
	var zsh = 2                    // line 9: a variable
	for ksh := range 3 {           // line 10: a range variable
		_ = ksh
	}
	_ = bash
	_ = zsh

	// None of the rest is a declaration of a shell's name.
	dash := false                  // the hyphen, which is what dash is here
	_ = dash
	other.bash = 1                 // a use, on somebody else's struct
	_ = point{bash: 2}             // a field key, likewise
	_ = someone.zsh()              // a call, likewise
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x_test.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var lines []int
	for _, d := range shellNamedDeclarations(f) {
		lines = append(lines, fset.Position(d.pos).Line)
	}
	want := []int{3, 3, 5, 5, 8, 9, 10}
	if len(lines) != len(want) {
		t.Fatalf("reported lines %v, want %v", lines, want)
	}
	for i, l := range lines {
		if l != want[i] {
			t.Errorf("reported line %d at position %d, want %d", l, i, want[i])
		}
	}
}

// TestTheShellImportGuardCatchesOne hands the check a deliberate violation.
//
// Without it a guard that had stopped looking would be indistinguishable from
// a tree with nothing to find, which is the failure mode of every detector.
func TestTheShellImportGuardCatchesOne(t *testing.T) {
	t.Parallel()
	const src = `package interp_test

import (
	"testing"

	sh "github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

var _ = testing.T{}
var _ = syntax.Core
var _ = sh.Dialect
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x_test.go", src, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, imported := range importPaths(t, f) {
		if strings.HasPrefix(imported, dialectPrefix) {
			found++
		}
	}
	if found != 1 {
		t.Errorf("the check found %d shell imports in a file with one", found)
	}
}

// importPaths reads one file's import paths, unquoted.
func importPaths(t *testing.T, f *ast.File) []string {
	t.Helper()
	out := make([]string, 0, len(f.Imports))
	for _, spec := range f.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("import path %s: %v", spec.Path.Value, err)
		}
		out = append(out, path)
	}
	return out
}

// moduleRoot walks up from the test's directory to the go.mod, so the walk
// does not depend on where the test was run from.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test's directory")
		}
		dir = parent
	}
}
