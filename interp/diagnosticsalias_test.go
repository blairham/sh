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
// move an axis takes a copy first: `d := r.diag().Clone()`, which is what
// locationPrefixNamed does and why.
func TestNothingWritesThroughTheDiagnostics(t *testing.T) {
	t.Parallel()
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
			t.Errorf("%s: writes through the pointer diag returns; take a copy with r.diag().Clone() first",
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
	t.Parallel()
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
	t.Parallel()
	requireViolations(t, 1, `package interp
func f(r *Runner) { r.diag().Location = LocationNameOnly }`)
}

// And the form that is correct, which has to stay quiet or the guard would
// forbid the very thing it tells a caller to do.
func TestTheDiagnosticsGuardAllowsACopy(t *testing.T) {
	t.Parallel()
	requireViolations(t, 0, `package interp
func f(r *Runner) {
	d := r.diag().Clone()
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

// Nor may a method of Diagnostics write its receiver, with four exceptions.
//
// The methods take the table by pointer so that a function calling one on an
// error path does not hold a 10 KB copy on its frame whether or not that
// branch runs (#6610). The table a method is called on is therefore the
// caller's — the runner's own, or the front end's — and a method that moves a
// style on it moves it for the rest of the run, exactly as a write through
// diag does above. Three methods are *for* moving the table in place and are
// called where a route is settled; keepBodyLocation is their helper and runs
// on the copy a route has just made. Every other method that needs to move a
// style takes a copy: `n := d.Clone()`, as ForScript does.
//
// The check is the same shape as the one above because the hazard is the same
// shape: the line compiles, and `d.Location = LocationNone` inside a value
// method meant what it said until the receiver became a pointer.
func TestADiagnosticsMethodWritesOnlyACopy(t *testing.T) {
	t.Parallel()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	methods := 0
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
		n, writes := receiverWrites(f)
		methods += n
		for _, pos := range writes {
			t.Errorf("%s: a Diagnostics method writes its receiver; take a copy with d.Clone() first", fset.Position(pos))
		}
	}
	// The control: the table has dozens of methods, and a glob that matched
	// nothing would report a clean package in the same words.
	if methods < 60 {
		t.Fatalf("saw %d Diagnostics methods, want the whole table: this test proved nothing", methods)
	}
}

// movesTheTableInPlace names the Diagnostics methods whose job is to write
// the table they are called on.
var movesTheTableInPlace = map[string]bool{
	"LocateCommandStringAsAPrompt": true,
	"LocateScriptAsAPrompt":        true,
	"keepBodyLocation":             true,
}

// receiverWrites counts the methods of *Diagnostics in f and reports every
// assignment in one of them, outside movesTheTableInPlace, whose target is a
// field of the receiver.
func receiverWrites(f *ast.File) (methods int, found []token.Pos) {
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || fn.Body == nil {
			continue
		}
		star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if id, ok := star.X.(*ast.Ident); !ok || id.Name != "Diagnostics" {
			continue
		}
		methods++
		if movesTheTableInPlace[fn.Name.Name] || len(fn.Recv.List[0].Names) == 0 {
			continue
		}
		recv := fn.Recv.List[0].Names[0].Name
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
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == recv && !rebound(fn.Body, recv, sel.Pos()) {
					found = append(found, sel.Pos())
				}
			}
			return true
		})
	}
	return methods, found
}

// rebound reports whether name was assigned a Clone of itself before pos in
// body: `d = d.Clone()` makes every later write a write on the copy.
func rebound(body *ast.BlockStmt, name string, pos token.Pos) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || as.Pos() >= pos {
			return true
		}
		for i, l := range as.Lhs {
			id, ok := l.(*ast.Ident)
			if !ok || id.Name != name || i >= len(as.Rhs) {
				continue
			}
			if call, ok := as.Rhs[i].(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Clone" {
					found = true
				}
			}
		}
		return true
	})
	return found
}

// The guard's own cases: a write through the receiver fires, a write on a
// copy does not, and the methods that move the table in place are allowed.
func TestTheMethodGuardCatchesAReceiverWrite(t *testing.T) {
	t.Parallel()
	requireReceiverWrites(t, 1, `package interp
func (d *Diagnostics) f() { d.Location = LocationNone }`)
}

func TestTheMethodGuardAllowsACopy(t *testing.T) {
	t.Parallel()
	requireReceiverWrites(t, 0, `package interp
func (d *Diagnostics) f() string {
	n := d.Clone()
	n.Location = LocationNone
	return n.prefix("", "", false, 1)
}
func (d *Diagnostics) g() {
	d = d.Clone()
	d.Location = LocationNone
}
func (d *Diagnostics) keepBodyLocation() { d.bodyLocation = d.Location }`)
}

func requireReceiverWrites(t *testing.T, want int, src string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, got := receiverWrites(f); len(got) != want {
		t.Errorf("%d receiver writes, want %d", len(got), want)
	}
}
