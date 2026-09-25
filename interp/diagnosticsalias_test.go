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

// Nothing may write through what diag hands back.
//
// diag returns the runner's *own* diagnostics vector by pointer rather than a
// copy of it, because the vector is 9112 bytes and the shell reads a field of
// it several thousand times in a startup — a copy per read, and a second copy
// to zero the value the copy was written over, was about a sixth of the whole
// run. The cost of that is an alias: a write through the pointer does not
// change one message, it changes the shell for the rest of the run.
//
// This is exactly the kind of rule that holds until somebody adds a line, and
// the line compiles — `d.Location = LocationNameOnly` was already in the tree
// when the return type changed, and it went on compiling and stopped meaning
// what it said. So it is checked rather than agreed. A caller that needs to
// move an axis takes a copy first: `d := *r.diag()`, which is what
// locationPrefixNamed does and why.
func TestNothingWritesThroughTheDiagnostics(t *testing.T) {
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	read := 0
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		read++
		for _, pos := range writesThroughDiag(f) {
			t.Errorf("%s: writes through the pointer diag returns; take a copy with *r.diag() first",
				fset.Position(pos))
		}
	}
	// The control. A glob that matched nothing, or a package that moved,
	// reports no violations in exactly the words a clean tree does.
	if read < 50 {
		t.Fatalf("read %d files of interp, want the whole package: this test proved nothing", read)
	}
}

// writesThroughDiag reports every assignment that reaches a field of the
// value diag returned — either straight off the call, or through a name the
// call was assigned to without being dereferenced.
func writesThroughDiag(f *ast.File) []token.Pos {
	var found []token.Pos
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}
		// The names this function bound to the pointer itself. A `*r.diag()`
		// is a copy and is not one of them, which is the whole distinction
		// this test is about.
		aliases := map[string]bool{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for i, rhs := range as.Rhs {
				if !isDiagCall(rhs) || i >= len(as.Lhs) {
					continue
				}
				if id, ok := as.Lhs[i].(*ast.Ident); ok {
					aliases[id.Name] = true
				}
			}
			return true
		})
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			var lhs []ast.Expr
			switch s := n.(type) {
			case *ast.AssignStmt:
				lhs = s.Lhs
			case *ast.IncDecStmt:
				lhs = []ast.Expr{s.X}
			default:
				return true
			}
			for _, e := range lhs {
				sel, ok := e.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				// `r.diag().Field = …`, which compiles and is the sharpest
				// form of this, and `d.Field = …` for a bound name.
				if isDiagCall(sel.X) {
					found = append(found, sel.Pos())
					continue
				}
				if id, ok := sel.X.(*ast.Ident); ok && aliases[id.Name] {
					found = append(found, sel.Pos())
				}
			}
			return true
		})
		return false
	})
	return found
}

// isDiagCall reports whether e is a call of the diag method on anything.
func isDiagCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "diag"
}

// The guard's own three cases. A checker that cannot fire is a checker that
// reports a clean tree in the same words as a broken one, which is the
// failure this repository has made often enough to write down.
func TestTheDiagnosticsGuardCatchesAWriteThroughABoundName(t *testing.T) {
	requireViolations(t, 1, `package interp
func f(r *Runner) {
	d := r.diag()
	d.Location = LocationNameOnly
	_ = d
}`)
}

// The sharpest form, because it needs no name and reads like an ordinary
// field write.
func TestTheDiagnosticsGuardCatchesAWriteStraightOffTheCall(t *testing.T) {
	requireViolations(t, 1, `package interp
func f(r *Runner) { r.diag().Location = LocationNameOnly }`)
}

// And the form that is correct, which has to stay quiet or the guard would
// forbid the very thing it tells a caller to do.
func TestTheDiagnosticsGuardAllowsACopy(t *testing.T) {
	requireViolations(t, 0, `package interp
func f(r *Runner) {
	d := *r.diag()
	d.Location = LocationNameOnly
	_ = d
}`)
}

func requireViolations(t *testing.T, want int, src string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(writesThroughDiag(f)); got != want {
		t.Errorf("%d violations, want %d", got, want)
	}
}
