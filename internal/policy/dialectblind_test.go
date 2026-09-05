// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The gate is dialect-blind, and this is the half of that claim a test can
// hold on its own: nothing this package reaches, at any depth inside the
// module, is a shell.
//
// Blindness is already true by type — Gate.Allow is handed a context and an
// interp.Action, and an Action carries a kind, a path, an argument vector, a
// write flag, a PID and a signal, with no route from any of them to which
// shell is running. But "there is no route today" is a statement about the
// code, and the way it stops being true is somebody importing dialect/bash to
// ask one question. The behavioral half — one script, one policy, every
// dialect, the same denials — lives with the binary that has a -dialect flag.

const modulePath = "github.com/blairham/sh"

func TestNothingThisPackageReachesIsAShell(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	seen := map[string]bool{}
	var visit func(pkg string)
	visit = func(pkg string) {
		if seen[pkg] {
			return
		}
		seen[pkg] = true
		for _, imported := range importsOf(t, root, pkg) {
			if !strings.HasPrefix(imported, modulePath+"/") {
				// Outside the module is the standard library, which this
				// module's only other dependencies are tools rather than
				// code. Nothing there is a shell.
				continue
			}
			rel := strings.TrimPrefix(imported, modulePath+"/")
			if strings.HasPrefix(rel, "dialect/") {
				t.Errorf("%s imports %s: a policy that can ask which shell it is inside is not dialect-blind",
					pkg, imported)
				continue
			}
			visit(rel)
		}
	}
	visit("internal/policy")
	// A sanity check on the walk itself: a test that silently visited nothing
	// would pass for the wrong reason, which is the failure mode of every
	// assertion made over a traversal.
	if !seen["interp"] {
		t.Fatalf("the import walk never reached interp; it visited %d packages", len(seen))
	}
}

// importsOf reads the non-test imports of one package in this module.
func importsOf(t *testing.T, root, pkg string) []string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(pkg))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var out []string
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, spec := range f.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("import path %s in %s: %v", spec.Path.Value, name, err)
			}
			out = append(out, path)
		}
	}
	return out
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
