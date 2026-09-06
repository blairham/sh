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
			f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
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
		}
	}
	// A walk that read nothing passes for the wrong reason.
	if files < 100 {
		t.Fatalf("the walk read %d Go files across interp/ and syntax/; it should be reading hundreds", files)
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
