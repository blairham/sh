// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package depsurface_test

import (
	"os/exec"
	"strings"
	"testing"
)

// runtimeDeps is every module outside this one that the shipped binary
// links, and the whole list.
//
// It is empty, and that is the point. "This module has no runtime
// dependencies at all" is written in internal/policy/alias.go as an argument
// — it is why a platform alias is a compile-time table and not a lookup —
// and an argument resting on a property nothing checks is an argument that
// quietly stops being true.
//
// It was briefly not empty. #2045 needed Unicode decomposition data to close
// a sandbox escape, took `golang.org/x/text/unicode/norm` to get it, and the
// table came back into the repository a day later as internal/unorm — the
// same answer internal/eastasian had already reached for East Asian Width.
// The list is kept rather than deleted because a guard that exists only
// while it has something to hold is a guard that will be missing next time.
//
// An entry here costs a deliberate edit and a sentence saying why, which is
// the price a dependency should cost a substrate.
var runtimeDeps = []string{}

// TestTheDependencySurfaceIsPinned fails when the shipped binary starts
// linking something new.
//
// In a package of its own rather than beside the command, because asking
// this question means running the go tool, and the cmd/sh suite gives itself
// a scratch $HOME and fails the run if anything writes into it — which the
// go build cache and its telemetry promptly do. The guard is right and the
// test was in the wrong room.
//
// Asked of `cmd/sh` rather than of go.mod, because go.mod is not the
// question: this repository's go.mod is mostly `tool` directives for the
// linter and the formatter, every one of them marked indirect, and none of
// them reaches the binary. What matters is what a user runs, which is what
// `go list -deps` of the command answers.
func TestTheDependencySurfaceIsPinned(t *testing.T) {
	t.Parallel()
	out, err := exec.Command("go", "list", "-deps", "github.com/blairham/sh/cmd/sh").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	allowed := make(map[string]bool, len(runtimeDeps))
	for _, d := range runtimeDeps {
		allowed[d] = true
	}
	var unexpected []string
	for _, pkg := range strings.Fields(string(out)) {
		if !external(pkg) || strings.HasPrefix(pkg, "github.com/blairham/sh") {
			continue
		}
		if !allowedUnder(pkg, allowed) {
			unexpected = append(unexpected, pkg)
		}
	}
	if len(unexpected) > 0 {
		t.Errorf("the shipped binary links modules this list does not name:\n  %s\n\n"+
			"If that is deliberate, add it to runtimeDeps with the reason. If it is not,\n"+
			"it arrived through an import somebody did not mean to add.",
			strings.Join(unexpected, "\n  "))
	}
}

// external distinguishes a module path from a standard-library one. The
// standard library has no dot in its first component and a module path
// always does, since it begins with a domain — which is the same test the
// go command itself uses to tell them apart.
func external(pkg string) bool {
	first, _, _ := strings.Cut(pkg, "/")
	return strings.Contains(first, ".")
}

func allowedUnder(pkg string, allowed map[string]bool) bool {
	for mod := range allowed {
		if pkg == mod || strings.HasPrefix(pkg, mod+"/") {
			return true
		}
	}
	return false
}
