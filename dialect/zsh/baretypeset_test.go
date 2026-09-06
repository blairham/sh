// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// A bare `typeset` writes the same parameter table a bare `local` writes:
// every parameter the shell has, each as its attribute words and then the
// assignment. Measured 2026-09-06 on 5.9.2, inside a function and out.
//
// The other word was left when the `local` spelling was built, because the
// corpus graded `local` — so `typeset` with nothing after it wrote nothing at
// all, which is the one answer no shell gives.
func TestABareTypesetIsTheSameListingAsABareLocal(t *testing.T) {
	if got := zsh.Semantics().BareTypesetListing; got != interp.BareLocalListsEveryParameter {
		t.Errorf("BareTypesetListing = %v, want BareLocalListsEveryParameter", got)
	}
	dir := t.TempDir()
	src := `x=1; f() { local y=2; typeset; }; f`
	out, st := runZsh(t, dir, src)
	if st != 0 {
		t.Fatalf("status = %d, out %q", st, out)
	}
	for _, want := range []string{"local y=2\n", "x=1\n"} {
		if !containsLine(out, want) {
			t.Errorf("out = %q, want the line %q — the running function's local and the global alike", out, want)
		}
	}
	// And the two words agree, which is the whole of the claim.
	bare, _ := runZsh(t, dir, `x=1; f() { local y=2; local; }; f`)
	if bare != out {
		t.Errorf("`typeset` listed\n%q\nand `local` listed\n%q\nwant one listing for both words", out, bare)
	}
}

func containsLine(out, line string) bool {
	for i := 0; i+len(line) <= len(out); i++ {
		if out[i:i+len(line)] == line && (i == 0 || out[i-1] == '\n') {
			return true
		}
	}
	return false
}
