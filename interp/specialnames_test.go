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

// filteredTables are the tables the special-name filter answers for. See
// interp/specialnames.go.
var filteredTables = map[string]bool{
	"kindFixed": true, "assignmentGuards": true, "assignmentActions": true,
	"dynamicWriters": true, "dynamicArrayWriters": true, "tied": true, "deferredParams": true,
}

// TestEveryWriteIntoAFilteredTableRecordsTheName holds the one property the
// filter's "no" rests on: every function that stores a name into one of the
// tables it answers for also records the name in it. A store that did not
// would let the plain store write past a guard, an action, a writer or a tie
// (#6423).
//
// The one exception is putBack, which restores a writer the same name was
// registered with before — so the name is recorded already — and is listed as
// such rather than left out of the walk.
func TestEveryWriteIntoAFilteredTableRecordsTheName(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	restores := map[string]bool{"putBack": true}
	writers := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(token.NewFileSet(), file, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			writes, records, putsBack := "", false, false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.AssignStmt:
					for _, lhs := range x.Lhs {
						if ix, ok := lhs.(*ast.IndexExpr); ok {
							if sel, ok := ix.X.(*ast.SelectorExpr); ok && filteredTables[sel.Sel.Name] {
								writes = sel.Sel.Name
							}
						}
					}
				case *ast.CallExpr:
					if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "add" {
						if inner, ok := sel.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "specialNames" {
							records = true
						}
					}
					if id, ok := x.Fun.(*ast.Ident); ok && restores[id.Name] && len(x.Args) > 0 {
						if u, ok := x.Args[0].(*ast.UnaryExpr); ok {
							if sel, ok := u.X.(*ast.SelectorExpr); ok && filteredTables[sel.Sel.Name] {
								putsBack = true
							}
						}
					}
				}
				return true
			})
			if writes == "" {
				continue
			}
			writers++
			if !records && !putsBack {
				t.Errorf("%s: %s writes a name into %s without recording it in specialNames — "+
					"call r.specialNames.add(name) beside the write", file, fn.Name.Name, writes)
			}
		}
	}
	// The seven registrations, so a walk that saw nothing is not a pass.
	if writers < len(filteredTables) {
		t.Errorf("found %d functions writing the filtered tables, want at least %d: the guard is looking in the wrong place",
			writers, len(filteredTables))
	}
}
