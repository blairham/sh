// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A `zmodload -d` declaration is a **dependency** and not a record: the
// module it names is loaded first, a dependency that will not load takes the
// module with it, and a declaration that leads back to itself is refused.
// Before this the row was written and nothing read it, so a module loaded
// with everything it had declared still missing — the silent success the rest
// of zmodload.go is written to avoid, reached from the one direction the
// feature gate cannot see (#4740).
//
// Measured 2026-09-26 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`,
// aarch64-apple-darwin25.4.0), `-f` under `env -i PATH=/usr/bin:/bin`, one
// probe at a time with the module listing read afterwards.

func runDepend(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src+"\n")
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// The dependency is loaded **with** the module, and the listing afterwards is
// what says so. The control is the second half: without the declaration the
// same `zmodload` leaves one name behind rather than two, so the extra line
// is the declaration's doing and not the listing's.
func TestADeclaredDependencyIsLoadedWithTheModule(t *testing.T) {
	out, st := runDepend(t, "zmodload -d zsh/datetime zsh/stat\nzmodload zsh/datetime\nprint l=$?\nzmodload")
	want := "l=0\nzsh/datetime\nzsh/main\nzsh/stat\n"
	if out != want || st != 0 {
		t.Errorf("out = %q (status %d), want %q", out, st, want)
	}
	out, st = runDepend(t, "zmodload zsh/datetime\nprint l=$?\nzmodload")
	if want := "l=0\nzsh/datetime\nzsh/main\n"; out != want || st != 0 {
		t.Errorf("control = %q (status %d), want %q", out, st, want)
	}
}

// And transitively: a dependency's own dependency comes too.
func TestADependencyOfADependencyIsLoadedAsWell(t *testing.T) {
	out, st := runDepend(t, "zmodload -d zsh/datetime zsh/stat\nzmodload -d zsh/stat zsh/zpty\n"+
		"zmodload zsh/datetime\nprint l=$?\nzmodload")
	want := "l=0\nzsh/datetime\nzsh/main\nzsh/stat\nzsh/zpty\n"
	if out != want || st != 0 {
		t.Errorf("out = %q (status %d), want %q", out, st, want)
	}
}

// **A dependency that fails takes the module with it.** The listing is the
// half worth asserting: the status alone would pass for a shell that
// complained and loaded the module anyway.
func TestAFailedDependencyLeavesTheModuleUnloaded(t *testing.T) {
	out, st := runDepend(t, "zmodload -d zsh/datetime zsh/nosuchmod\nzmodload zsh/datetime\nprint l=$?\nzmodload")
	want := "zsh:2: failed to load module `zsh/nosuchmod': not implemented yet\nl=1\nzsh/main\n"
	if out != want || st != 0 {
		t.Errorf("out = %q (status %d), want %q", out, st, want)
	}
}

// The walk stops at the first dependency that will not load, so a good one
// declared behind a bad one is not loaded either — and a dependency loaded
// *before* the failure is left standing rather than rolled back. Both
// measured, and they are the pair that says the loading is a walk in
// declaration order rather than a set.
func TestTheDependencyWalkStopsAtTheFirstFailure(t *testing.T) {
	out, _ := runDepend(t, "zmodload -d zsh/datetime zsh/nosuchmod zsh/stat\n"+
		"zmodload zsh/datetime\nzmodload")
	want := "zsh:2: failed to load module `zsh/nosuchmod': not implemented yet\nzsh/main\n"
	if out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	out, _ = runDepend(t, "zmodload -d zsh/nosuchmod zsh/stat\nzmodload zsh/nosuchmod\nzmodload")
	want = "zsh:2: failed to load module `zsh/nosuchmod': not implemented yet\nzsh/main\nzsh/stat\n"
	if out != want {
		t.Errorf("a dependency of a failing module = %q, want %q", out, want)
	}
}

// A dependency already loaded is stepped over rather than loaded again.
func TestADependencyAlreadyLoadedIsNotLoadedTwice(t *testing.T) {
	out, st := runDepend(t, "zmodload zsh/stat\nzmodload -d zsh/datetime zsh/stat\n"+
		"zmodload zsh/datetime\nprint l=$?\nzmodload")
	want := "l=0\nzsh/datetime\nzsh/main\nzsh/stat\n"
	if out != want || st != 0 {
		t.Errorf("out = %q (status %d), want %q", out, st, want)
	}
}

// **A cycle ends the shell**, at 1, and the name it reports is the one
// `zmodload` was *asked for* rather than the module the walk came back to —
// `-d a b; -d b c; -d c a` then `zmodload b` is `;b`. The stray `;` is the
// reference's and is reproduced rather than tidied, because this sentence is
// what a script looking for the failure would match on.
//
// The `print never` is the assertion that matters: a shell that reported and
// carried on would print it.
func TestACircularDependencyEndsTheShell(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a module depending on itself",
			"zmodload -d zsh/datetime zsh/datetime\nzmodload zsh/datetime\nprint never",
			"zsh:2: circular dependencies for module ;zsh/datetime\n",
		},
		{
			"a cycle through three names",
			"zmodload -d a b\nzmodload -d b c\nzmodload -d c a\nzmodload b\nprint never",
			"zsh:4: circular dependencies for module ;b\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runDepend(t, c.src)
			if out != c.want {
				t.Errorf("out = %q, want %q", out, c.want)
			}
			if st != 1 {
				t.Errorf("status = %d, want 1", st)
			}
		})
	}
}

// Two letters that do **not** act on a declaration, measured: `-e` asks about
// the module and loads nothing, and an unload leaves the dependency standing.
// Without these rows a change that ran the walk from every letter would pass
// everything above.
func TestTheDependencyWalkIsOnlyTheLoadPath(t *testing.T) {
	out, st := runDepend(t, "zmodload -d zsh/datetime zsh/stat\nzmodload -e zsh/datetime\nprint e=$?\nzmodload")
	if want := "e=1\nzsh/main\n"; out != want || st != 0 {
		t.Errorf("-e = %q (status %d), want %q", out, st, want)
	}
	out, st = runDepend(t, "zmodload -d zsh/datetime zsh/stat\nzmodload zsh/datetime\n"+
		"zmodload -u zsh/datetime\nzmodload")
	if want := "zsh/main\nzsh/stat\n"; out != want || st != 0 {
		t.Errorf("-u = %q (status %d), want %q", out, st, want)
	}
}

// **This shell ships an empty dependency table**, where a fresh reference
// already has seven rows — `zsh/zutil: zsh/complete` among them.
//
// That is a decision and not an omission, and it is deliberately separable
// from the walk above: here a module's features are builtins and parameters
// the shell registers whatever is loaded, so an edge would carry no loading
// mechanism at all and would only propagate refusals. The row below is what
// keeps the two apart — a change that adopted the table would have to come
// and edit it.
func TestTheShippedDependencyTableIsNotAdopted(t *testing.T) {
	out, st := runDepend(t, "zmodload -d\nprint d=$?\nzmodload zsh/zutil\nzmodload")
	want := "d=0\nzsh/main\nzsh/zutil\n"
	if out != want || st != 0 {
		t.Errorf("out = %q (status %d), want %q", out, st, want)
	}
}
