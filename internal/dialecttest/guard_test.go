// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialecttest_test

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

const interpPath = "github.com/blairham/sh/interp"

// TestEveryRunnerUnderDialectIsToldWhichShellItIs is the half of #860 that
// outlives the patch.
//
// Adding Runner.Dialect to the nineteen files that were missing it fixes those
// nineteen; it does nothing about the twentieth, which will be written by
// copying one of them, and the nineteen exist because that is exactly how the
// first one spread. So the rule is enforced over the source rather than
// asserted once per file: every hand-built interp.Runner anywhere under
// dialect/ names the field, or this fails with the file and line.
//
// A behavioral check cannot do this job. The omission is silent by
// construction — nil means the core, the snippet still runs, the status is
// still a number — and #849 established that every one of the nineteen passed
// with the field and without it. A test that runs the suite twice would
// therefore report nothing. What is wrong is visible only in the source, so
// the source is what is read.
//
// Runner (in this package) is the way out: a runner built through a Preset
// cannot lack the field, and the failure message points there rather than
// asking for another correct call site.
func TestEveryRunnerUnderDialectIsToldWhichShellItIs(t *testing.T) {
	t.Parallel()
	root := filepath.Join(moduleRoot(t), "dialect")
	fset := token.NewFileSet()
	files := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		files++
		f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			t.Errorf("parsing %s: %v", path, perr)
			return nil
		}
		for _, pos := range runnersMissingDialect(f) {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("dialect/%s:%d builds an interp.Runner without Dialect.\n"+
				"\tNil means the core, so every nested parse — eval, command substitution,\n"+
				"\ta sourced file, a trap body — runs as a shell this suite is not about.\n"+
				"\tSet Dialect, or build the runner with dialecttest.Preset.Runner.",
				rel, fset.Position(pos).Line)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A walk that silently visited nothing passes for the wrong reason, which
	// is the failure mode of every assertion made over a traversal.
	if files < 20 {
		t.Fatalf("the walk read %d Go files under dialect/; it should be reading dozens", files)
	}
}

// runnersMissingDialect reports the position of every composite literal in f
// that builds an interp.Runner without naming the Dialect field.
//
// The import is resolved rather than assumed: a file that spells the package
// something other than `interp` is still building the same struct, and one
// that has its own type called Runner is not.
func runnersMissingDialect(f *ast.File) []token.Pos {
	local := ""
	for _, spec := range f.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != interpPath {
			continue
		}
		local = "interp"
		if spec.Name != nil {
			local = spec.Name.Name
		}
	}
	if local == "" || local == "_" || local == "." {
		// No import to resolve against, or one this check cannot follow.
		// A dot-import of interp would defeat it; nothing does that, and
		// the guard below on the file count would not notice, so say so.
		return nil
	}
	var missing []token.Pos
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := lit.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Runner" {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != local {
			return true
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				// A positional literal names no fields at all, so it
				// cannot have named this one. It also will not compile
				// against a struct this size; report it either way.
				continue
			}
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Dialect" {
				return true
			}
		}
		missing = append(missing, lit.Lbrace)
		return true
	})
	return missing
}

// TestTheGuardCatchesADeliberateOmission hands the detector a violation and
// checks that it says so.
//
// Without this, a detector that reported nothing would be indistinguishable
// from a clean tree — which is the same shape of hole the thing it guards
// against is, and the reason #890's home-directory guard is a named directory
// that must stay empty rather than a sweep with an exemption.
func TestTheGuardCatchesADeliberateOmission(t *testing.T) {
	t.Parallel()
	const src = `package x

import (
	"github.com/blairham/sh/interp"
	sh "github.com/blairham/sh/syntax"
)

type Runner struct{ Dialect *sh.Dialect }

func f() {
	_ = &interp.Runner{Stdout: nil}                  // line 11: the omission
	_ = interp.Runner{Dialect: nil}                  // fine: names the field
	_ = &Runner{}                                    // fine: not interp's
	_ = &interp.Runner{                              // line 14: nested, still missing
		Semantics: nil,
	}
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var lines []int
	for _, pos := range runnersMissingDialect(f) {
		lines = append(lines, fset.Position(pos).Line)
	}
	want := []int{11, 14}
	if len(lines) != len(want) {
		t.Fatalf("reported lines %v, want %v", lines, want)
	}
	for i, l := range lines {
		if l != want[i] {
			t.Errorf("reported line %d, want %d", l, want[i])
		}
	}
}

// TestTheGuardFollowsARenamedImport: the file it is protecting could spell the
// import anything, and a check that matched the literal text `interp.Runner`
// would be silently blind to the one that did.
func TestTheGuardFollowsARenamedImport(t *testing.T) {
	t.Parallel()
	const src = `package x

import ip "github.com/blairham/sh/interp"

func f() { _ = &ip.Runner{Name: "bash"} }
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(runnersMissingDialect(f)); got != 1 {
		t.Errorf("reported %d omissions under a renamed import, want 1", got)
	}
}

// moduleRoot walks up from the test's directory to the go.mod, so the walk
// above does not depend on where the test was run from.
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
