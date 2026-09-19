// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"slices"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A bare `~` is the home the shell holds *now*, and it does not move when
// something starts a child.
//
// The second half is the whole point of the test. One build of one column
// answers a bare `~` from a copy of `HOME` that its own assignments do not
// reach, and refreshes that copy as a side effect of building an environment
// for a child — so `~` there is one answer before an external command on the
// line and a different one after it. Measured 2026-09-19, `env -i
// PATH=/usr/bin:/bin LC_ALL=C HOME=/orig`, `HOME=/h; echo ~`:
//
//	bash 5.3.20          /orig     and /h once a child has been built
//	bash as sh           /orig     the same binary, the same answer
//	bash 3.2.57          /h
//	zsh 5.9.2            /h
//	ksh93u+              /h
//	dash 0.5.12          /h
//	BusyBox ash 1.37.0   /h
//
// Six of the seven read the variable, and the seventh disagrees with its own
// older build, so this is recorded rather than modeled — see
// docs/spec/grammar/expansion.md and #3484. What is pinned here is that the
// answer is the variable and that it is *stable*: a guard against arriving at
// the divergent reading by accident, in which `HOME=/x; cd ~` would go to the
// old home for as long as a script runs nothing but builtins.
func TestABareTildeReadsTheCurrentHomeBeforeAndAfterAChild(t *testing.T) {
	dir := t.TempDir()
	// Three reads, and the order is what makes them discriminate. One before
	// the assignment fixes what the shell started with, one after it, and one
	// after a child has been built. The three readings give three different
	// triples, so no two of them can be confused:
	//
	//	the variable, read every time   dir, /h, /h     what this shell does
	//	frozen at startup               dir, dir, dir
	//	a cache a child refreshes       dir, dir, /h
	//
	// A probe with only the last two lines cannot tell any of them apart,
	// which is how the first draft of this test passed a runner mutated to
	// freeze the home it first read.
	out, st := run(t, "echo ~\nHOME=/h\necho ~\n/usr/bin/true\necho ~\n", func(r *Runner) {
		sem := CoreSemantics()
		r.Semantics, r.Dir = &sem, dir
		r.Vars = map[string]string{"HOME": dir, "PATH": "/usr/bin:/bin"}
	})
	if st != 0 {
		t.Fatalf("status = %d, want 0; out = %q", st, out)
	}
	want := []string{dir, "/h", "/h"}
	if got := strings.Split(strings.TrimSpace(out), "\n"); !slices.Equal(got, want) {
		t.Errorf("the three tildes read %q, want %q", got, want)
	}
}
