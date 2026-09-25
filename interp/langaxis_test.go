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

// The two axes a runner adjusts are not lang's to answer, and nothing may
// write through what it returns.
//
// lang hands back the runner's own dialect by pointer, where dialect returns
// a copy with ArithPrecedence and CharacterWidth set from the runner's
// current state. Every other axis reads the same either way, which is what
// makes the pointer safe and what makes it fast: a startup on a real ~/.zshrc
// asked 630,000 of these questions, and one function asked seven of them by
// copying 384 bytes seven times.
//
// Both halves of that are invisible if they go wrong. Reading
// `r.lang().ArithPrecedence` compiles and returns the dialect's order rather
// than the one `setopt c_precedences` moved it to — a wrong answer, not a
// crash. And a runner with no dialect of its own is handed a package-level
// value that every other such runner shares, so a write through the pointer
// would reach all of them.
func TestNothingReadsAnAdjustedAxisOffLang(t *testing.T) {
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
		for _, bad := range langMisuses(f) {
			t.Errorf("%s: %s", fset.Position(bad.pos), bad.why)
		}
	}
	if read < 50 {
		t.Fatalf("read %d files of interp, want the whole package: this test proved nothing", read)
	}
}

type langMisuse struct {
	pos token.Pos
	why string
}

// adjustedAxes are the fields dialect fills in from the runner and lang does
// not. Named here rather than derived, because the point is to fail when one
// is added to dialect and not to this list — a reader adding an adjustment
// has to come here, which is the only moment anybody is thinking about it.
var adjustedAxes = map[string]bool{"ArithPrecedence": true, "CharacterWidth": true}

// langMisuses reports every use of lang that is not a read of an
// unadjusted axis.
//
// The rule is stated that way round on purpose, and it is the second version
// of this check. The first named the two things that were obviously wrong —
// reading an adjusted axis, writing through the pointer — and a third slipped
// between them: `r.lang().On(route)`, a *method* on Dialect that returns a
// copy, which parsed a sourced file with a dialect carrying neither the
// runner's character width nor its arithmetic order. It compiled, it read
// like every other line here, and it broke a multibyte file's reading.
//
// So anything that is not `r.lang().SomeAxis` as a value is a misuse, and a
// caller wanting the dialect itself — to hand to a parser, to call a method
// on, to copy — takes dialect and its two adjustments with it.
func langMisuses(f *ast.File) []langMisuse {
	var found []langMisuse
	// Every lang call, and the ones reached as a plain field read. What is
	// in the first and not the second is a use of the value itself.
	calls := map[token.Pos]bool{}
	fieldRead := map[token.Pos]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if e, ok := n.(ast.Expr); ok && isLangCall(e) {
			calls[n.Pos()] = true
		}
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && isLangCall(sel.X) {
				found = append(found, langMisuse{sel.Pos(), "calls " + sel.Sel.Name + " on lang, which hands back a dialect the runner has not adjusted; use dialect"})
				// Counted as reached so it is reported once rather than
				// twice, as a method call and as a loose value.
				fieldRead[sel.X.Pos()] = true
			}
			return true
		}
		if sel, ok := n.(*ast.SelectorExpr); ok && isLangCall(sel.X) {
			fieldRead[sel.X.Pos()] = true
			if adjustedAxes[sel.Sel.Name] {
				found = append(found, langMisuse{sel.Pos(), "reads " + sel.Sel.Name + " off lang, which does not adjust it; use dialect"})
			}
		}
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
			if sel, ok := e.(*ast.SelectorExpr); ok && isLangCall(sel.X) {
				found = append(found, langMisuse{sel.Pos(), "writes through lang, whose value other runners share"})
			}
		}
		return true
	})
	for pos := range calls {
		if !fieldRead[pos] {
			found = append(found, langMisuse{pos, "uses the dialect lang returns rather than reading one axis off it; use dialect"})
		}
	}
	return found
}

func isLangCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "lang"
}

// The guard's own cases, so that a clean run means the checker looked rather
// than that it could not fire.
func TestTheLangGuardCatchesAnAdjustedRead(t *testing.T) {
	requireLangMisuses(t, 1, `package interp
func f(r *Runner) bool { return r.lang().ArithPrecedence == 0 }`)
}

func TestTheLangGuardCatchesAWriteThrough(t *testing.T) {
	requireLangMisuses(t, 1, `package interp
func f(r *Runner) { r.lang().TildeGroup = true }`)
}

// The shape that got through the first version of this guard and broke a
// multibyte file's reading.
func TestTheLangGuardCatchesAMethodCall(t *testing.T) {
	requireLangMisuses(t, 1, `package interp
func f(r *Runner) Dialect { return r.lang().On(0) }`)
}

// And the value taken loose, which is the same mistake without the method.
func TestTheLangGuardCatchesTheValueTakenLoose(t *testing.T) {
	requireLangMisuses(t, 1, `package interp
func f(r *Runner) Dialect { return *r.lang() }`)
}

func TestTheLangGuardAllowsAnOrdinaryRead(t *testing.T) {
	requireLangMisuses(t, 0, `package interp
func f(r *Runner) bool { return r.lang().TildeGroup }`)
}

func requireLangMisuses(t *testing.T, want int, src string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(langMisuses(f)); got != want {
		t.Errorf("%d misuses, want %d", got, want)
	}
}
