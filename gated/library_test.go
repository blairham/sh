// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package gated_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// processImports are the packages through which a library reaches process
// state: os for the environment, the working directory and the three standard
// streams; os/exec and syscall for the process image; os/signal for the
// embedder's own signal handlers.
var processImports = map[string]bool{
	"os":        true,
	"os/exec":   true,
	"os/signal": true,
	"syscall":   true,
}

// TestThePackageTouchesNoProcessState holds this package to the rule the
// library packages of this module live under (AGENTS.md, "The core is a
// library, so it may not touch the process"). forbidigo enforces that rule for
// interp/, dialect/ and repl/ only, so for this directory the rule is checked
// here: no non-test file imports a package that reaches the process.
//
// It is stricter than forbidigo's list, and that is the point of checking an
// import rather than a call: this package has no business with os at all.
// Main is handed its streams, its arguments and its host name, which is what
// lets a consumer's cmd/sh be the only place those are read.
func TestThePackageTouchesNoProcessState(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	files := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files++
		f, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if processImports[p] {
				t.Errorf("%s imports %q: this package is a library and reaches no process state;\n"+
					"\ttake the value as a parameter, as Main takes its streams", name, p)
			}
		}
	}
	// A walk that read nothing passes over a broken tree and a clean one alike.
	if files < 5 {
		t.Fatalf("read %d non-test files; the walk is not looking at this package", files)
	}
}
