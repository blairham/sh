// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestEveryCommandsDoorDefersOnlyWhatTheCompilerInlines holds the shape that
// keeps the runtime's deferred-call machinery off the path every command in
// the shell takes (#6423).
//
// A function's deferred calls are compiled inline only while it has at most
// eight of them, none in a loop, and no more than fifteen of them counted once
// per return statement. Past that, every return runs the runtime's machinery —
// including a return taken before anything was deferred — and `command` and
// `applyRedirs` were both past it, so a `(( ))` paid it twice on its way
// through and a loop of them spent about a tenth of its time there. Checked
// with `go build -gcflags=-d=defer`, which says "open-coded" or
// "stack-allocated" per site. The split that fixed it is easy to undo by
// adding one more deferred call to the wrong function, and nothing else would
// say so: the shell behaves the same and is slower.
//
// So each function on that path is held to one of two shapes: it defers
// nothing at all, or what it defers is within the compiler's limits.
func TestEveryCommandsDoorDefersOnlyWhatTheCompilerInlines(t *testing.T) {
	t.Parallel()
	// What every command runs, and so must defer nothing.
	deferNothing := map[string]bool{
		"commandAt":   true,
		"dispatch":    true,
		"applyRedirs": true,
	}
	// What defers, and must stay where the compiler inlines it.
	inlined := map[string]bool{
		"command":                     true,
		"commandPuttingTheLineBack":   true,
		"commandLeavingItsHeaderLine": true,
		"dispatchHoldingAJobSlot":     true,
		"underRuntime":                true,
		"simple":                      true,
	}
	seen := map[string]bool{}
	fset := token.NewFileSet()
	for _, file := range []string{"runner.go", "redirect.go"} {
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil {
				continue
			}
			name := fn.Name.Name
			if !deferNothing[name] && !inlined[name] {
				continue
			}
			seen[name] = true
			defers, returns, inLoop := deferShape(fn.Body)
			if deferNothing[name] && defers > 0 {
				t.Errorf("%s defers %d call(s); it is on every command's path and must defer nothing — "+
					"put what it defers in a function of its own that only the commands needing it enter", name, defers)
			}
			if inlined[name] && (defers > 8 || defers*returns > 15 || inLoop) {
				t.Errorf("%s has %d deferred call(s) and %d return(s)%s: past what the compiler inlines "+
					"(at most 8, none in a loop, and at most 15 counted once per return)",
					name, defers, returns, map[bool]string{true: ", one in a loop"}[inLoop])
			}
		}
	}
	for name := range deferNothing {
		if !seen[name] {
			t.Errorf("found no method %s: the guard is looking in the wrong place", name)
		}
	}
	for name := range inlined {
		if !seen[name] {
			t.Errorf("found no method %s: the guard is looking in the wrong place", name)
		}
	}
}

// deferShape counts the defer and return statements in a function body, and
// says whether any defer sits in a loop. A function literal's body is its own
// function and is not counted.
func deferShape(body *ast.BlockStmt) (defers, returns int, inLoop bool) {
	var walk func(n ast.Node, loop bool)
	walk = func(n ast.Node, loop bool) {
		ast.Inspect(n, func(m ast.Node) bool {
			switch x := m.(type) {
			case *ast.FuncLit:
				return false
			case *ast.DeferStmt:
				defers++
				if loop {
					inLoop = true
				}
			case *ast.ReturnStmt:
				returns++
			case *ast.ForStmt:
				if m != n {
					walk(x.Body, true)
					return false
				}
			case *ast.RangeStmt:
				if m != n {
					walk(x.Body, true)
					return false
				}
			}
			return true
		})
	}
	walk(body, false)
	if returns == 0 {
		// Falling off the end is a return of its own.
		returns = 1
	}
	return defers, returns, inLoop
}
