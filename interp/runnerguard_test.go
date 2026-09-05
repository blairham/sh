// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bareMarker exempts one Runner literal from the rule below.
//
// A comment rather than a list of blessed files, because the exemption has to
// sit where the reader is: a file named somewhere else is a rule with an
// invisible hole in it, and the next person to copy the line copies the hole.
// It is greppable, and there should be very few.
const bareMarker = "testrunner:bare"

// TestEveryRunnerInATestComesFromTheHelper.
//
// The helper gives a Runner a working directory, a temporary directory and a
// registered CleanUp, and every one of those is a thing the suite got wrong by
// leaving it out: relative redirects wrote into the checkout, and process
// substitutions left a directory each in /tmp — thousands of them, because
// only CleanUp removes one and no test had reason to call it.
//
// Converting the 192 literals that existed fixed those. This is what stops the
// 193rd from putting them back, and it is the half worth having: a sweep is
// undone by the next test somebody writes, and an unwrapped Runner literal
// looks like every other line on the page. The failure it prevents is silent
// in both directions — the test passes either way — so a reviewer is the only
// other thing that could catch it, and reviewers do not count directories in
// /tmp.
//
// It also found seven the sweep missed, which is the argument in miniature: the
// sweep went looking for one spelling and these files use the other, importing
// the package by name where most dot-import it. A rule enforced by reading the
// syntax tree does not care how the type is spelled.
//
// It reads this package's own test sources, which is the only source it may
// read and the reason this is a test rather than a linter: the rule is about
// what a Runner in *these* tests must have, not about Go.
func TestEveryRunnerInATestComesFromTheHelper(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 100 {
		t.Fatalf("found %d test files, want the whole package — the guard is looking in the wrong place", len(files))
	}

	var bare []string
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		found, err := bareRunners(file, src)
		if err != nil {
			t.Fatal(err)
		}
		bare = append(bare, found...)
	}

	if len(bare) != 0 {
		t.Errorf("%d Runner(s) built without newTestRunner:", len(bare))
		for _, where := range bare {
			t.Errorf("\t%s", where)
		}
		t.Errorf("Wrap the literal — newTestRunner(t, &Runner{…}) — so it gets a "+
			"directory of its own, a TMPDIR of its own and a registered CleanUp. "+
			"If an unset field is the subject of the test, say so in a comment "+
			"containing %q on the line above.", bareMarker)
	}
}

// TestTheGuardCatchesWhatItIsFor.
//
// A guard is worth what it detects, and a guard over a tree that already obeys
// the rule reports nothing whether it works or not — so the passing run above
// is not evidence. These are the cases, written out rather than waited for:
// the two spellings it has to catch, the wrapper it has to accept, and the
// marker it has to honor.
//
// The last one is the one to keep honest in both directions. A marker that
// exempted the whole file, or the rest of it, would be a rule anyone could
// switch off by writing a comment once at the top.
func TestTheGuardCatchesWhatItIsFor(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"a bare literal is caught", "r := &Runner{}", 1},
		{"the qualified spelling too", "r := &interp.Runner{Name: \"sh\"}", 1},
		{"fields do not hide it", "r := &Runner{\n\tStdout: nil,\n\tDir: \"x\",\n}", 1},
		{"the wrapper is accepted", "r := newTestRunner(t, &Runner{})", 0},
		{"the wrapper is accepted qualified", "r := newTestRunner(t, &interp.Runner{})", 0},
		{"a marker on the line above exempts", "// " + bareMarker + "\nr := &Runner{}", 0},
		{"a marker beside it exempts", "r := &Runner{} // " + bareMarker, 0},
		{"a marker two lines up does not", "// " + bareMarker + "\n\nr := &Runner{}", 1},
		// An ordinary comment is not a marker. Without this, a guard that
		// treated any comment as an excuse would pass every case above and
		// exempt most of the package, since nearly every construction here
		// has a sentence over it explaining what it is for.
		{"an ordinary comment above does not exempt", "// a Runner for the test below\nr := &Runner{}", 1},
		{"an ordinary comment beside it does not exempt", "r := &Runner{} // the shell", 1},
		{"a marker naming something else does not", "// nolint:something\nr := &Runner{}", 1},
		{"a marker does not exempt the next one", "// " + bareMarker + "\nr := &Runner{}\nr2 := &Runner{}", 1},
		{"a value rather than a pointer is not the shape", "var r Runner", 0},
		{"another type is not it", "r := &Buffer{}", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "package interp_test\nfunc f() {\n" + tc.body + "\n}\n"
			got, err := bareRunners("case.go", []byte(src))
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.want {
				t.Errorf("found %v, want %d", got, tc.want)
			}
		})
	}
}

// bareRunners names every Runner literal in one file that the helper is not
// holding and no marker excuses.
//
// Split out from the test so the test above can hand it a source of its own.
// A guard that can only be pointed at the tree it guards is a guard whose own
// failure looks exactly like success.
func bareRunners(name string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// The lines a marker comment ends on. A literal is exempt if one sits
	// directly above it or beside it, which is where a reader looks.
	exempt := map[int]bool{}
	for _, group := range f.Comments {
		if strings.Contains(group.Text(), bareMarker) {
			exempt[fset.Position(group.End()).Line] = true
		}
	}

	// Every Runner literal the helper is already holding. Collected first so
	// the walk below is "is this one of those" rather than one that has to
	// know its own parent.
	held := map[ast.Node]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, ok := call.Fun.(*ast.Ident); !ok || id.Name != "newTestRunner" {
			return true
		}
		for _, arg := range call.Args {
			held[arg] = true
		}
		return true
	})

	var bare []string
	ast.Inspect(f, func(n ast.Node) bool {
		unary, ok := n.(*ast.UnaryExpr)
		if !ok || unary.Op != token.AND || !isRunnerLiteral(unary.X) {
			return true
		}
		if held[ast.Node(unary)] {
			return true
		}
		line := fset.Position(unary.Pos()).Line
		if exempt[line] || exempt[line-1] {
			return true
		}
		bare = append(bare, fset.Position(unary.Pos()).String())
		return true
	})
	return bare, nil
}

// isRunnerLiteral says whether an expression is a Runner composite literal,
// spelled either way: this package's tests dot-import interp, and the
// in-package ones name it directly, so both `Runner{}` and `interp.Runner{}`
// have to count.
func isRunnerLiteral(x ast.Expr) bool {
	lit, ok := x.(*ast.CompositeLit)
	if !ok {
		return false
	}
	switch typ := lit.Type.(type) {
	case *ast.Ident:
		return typ.Name == "Runner"
	case *ast.SelectorExpr:
		return typ.Sel.Name == "Runner"
	}
	return false
}
