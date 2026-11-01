// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

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

// Runner.visibleDiagnostic renders a whole sentence and keeps only the newline
// that ends it, which is right only while no diagnostic writes two lines in
// one call: a newline between two sentences would come out as `\n`. This
// reads every call to the writers that route through it and refuses a
// format literal with a newline anywhere but at its end.
func TestNoDiagnosticWritesTwoLinesInOneCall(t *testing.T) {
	t.Parallel()
	writers := map[string]bool{
		"diagf": true, "Diagnosef": true, "DiagnoseAsTheShellf": true,
		"diagLine": true, "diagLineNamed": true, "fatal": true,
	}
	var files []string
	for _, dir := range []string{".", "../dialect/zsh"} {
		matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, matches...)
	}
	checked := 0
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !writers[sel.Sel.Name] {
				return true
			}
			for _, arg := range call.Args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				checked++
				if i := strings.IndexByte(s, '\n'); i >= 0 && i != len(s)-1 {
					t.Errorf("%s: %s writes %q, two lines in one call", fset.Position(lit.Pos()), sel.Sel.Name, s)
				}
				break
			}
			return true
		})
	}
	if checked < 100 {
		// The walk found too few calls to be reading the writers at all.
		t.Fatalf("checked %d format literals, want the several hundred these writers have", checked)
	}
}
