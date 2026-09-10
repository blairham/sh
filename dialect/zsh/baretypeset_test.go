// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
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

// A produced parameter is listed by its attributes and **not** by its value,
// which is what `hide` does in the shell this models and what every table in
// this listing that the shell generates on each read needs.
//
// Measured 2026-09-09 against zsh 5.9.2: with the modules loaded, a bare
// `typeset` writes `array readonly errnos`, `association readonly widgets`
// and `array readonly epochtime` — the attribute words, the name, and no `=`
// at all. The same shell shows it for a name a script hides itself:
// `typeset -H hhh=v; typeset -aH hha=(1 2); typeset` writes `hhh` and
// `array hha`.
//
// The `=` is the whole assertion and it is asserted as an absence, because
// the failure it guards is one of size and one of *time*. Writing the values
// put a hundred and seven error names, a fifty-five-key locale table and
// every widget this shell has into the middle of the listing — and one of
// them is a clock, so the same listing asked for twice differed from itself
// in its own nanoseconds, which is a test that fails for no reason anybody
// can act on (#1618).
func TestABareListingWritesAProducedParametersAttributesAndNotItsValue(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset`)
	if st != 0 {
		t.Fatalf("status = %d, out %q", st, out)
	}
	for _, want := range []string{
		"array readonly errnos\n",
		"array readonly keymaps\n",
		"array readonly epochtime\n",
		"association readonly langinfo\n",
		"association readonly widgets\n",
		"association readonly sysparams\n",
		"association readonly builtins\n",
		"readonly EPOCHSECONDS\n",
	} {
		if !containsLine(out, want) {
			t.Errorf("out = %q, want the whole line %q", out, want)
		}
	}
	for _, never := range []string{"errnos=", "langinfo=", "widgets=", "epochtime=", "EPOCHSECONDS="} {
		if strings.Contains(out, never) {
			t.Errorf("out = %q, want no %q in it — a produced value is not state a listing carries", out, never)
		}
	}
}
