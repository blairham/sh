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
	//
	// Compared with the **re-drawn readings taken out**, because this shell's
	// listing re-reads a produced parameter and the real one does too:
	// measured 2026-09-14 on zsh 5.9.2, `f() { local y=2; typeset; }` and the
	// same function ending in `local` differ on their `RANDOM` line and on
	// nothing else that is not `$0` or `$_`. A byte comparison was true here
	// only while the listing had no produced parameter in it at all, so
	// keeping it would have pinned that absence rather than the claim (#2722).
	bare, _ := runZsh(t, dir, `x=1; f() { local y=2; local; }; f`)
	if redrawn(bare) != redrawn(out) {
		t.Errorf("`typeset` listed\n%q\nand `local` listed\n%q\nwant one listing for both words", out, bare)
	}
	// The control, so that the normalization above cannot be what makes the
	// two agree: the line really is there, and it really does carry a value.
	for _, listing := range []string{out, bare} {
		if !strings.Contains(listing, "integer 10 RANDOM=") {
			t.Errorf("listing %q has no produced RANDOM row, so the comparison above proves nothing", listing)
		}
	}
}

// redrawn blanks the reading of every produced parameter whose value is a new
// one on each read, so two listings of one shell can be compared for shape.
func redrawn(listing string) string {
	var b strings.Builder
	for _, line := range strings.SplitAfter(listing, "\n") {
		if name, _, cut := strings.Cut(line, "="); cut && strings.HasSuffix(name, "RANDOM") {
			b.WriteString(name + "=<redrawn>\n")
			continue
		}
		b.WriteString(line)
	}
	return b.String()
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
	// The three deferred names below are **read first**, which is the state
	// this test's own measurement was taken in — "with the modules loaded"
	// — and which it had no way to reach before the deferral was modeled.
	// A bare `typeset` in a shell that has referred to nothing writes
	// `undefined keymaps` for them, in the reference and here, and that row
	// is the pair below. See interp/deferredparam.go (#4923).
	out, st := runZsh(t, t.TempDir(), `: ${#keymaps} ${#widgets} ${#builtins}
		typeset`)
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
		// The kind word beside the readonly one, which the parameter had no
		// way to carry until a produced one could say how it lists —
		// measured 2026-09-12 after `zmodload zsh/datetime`, real zsh writes
		// `integer readonly EPOCHSECONDS` and `float readonly EPOCHREALTIME`
		// here, and this wrote `readonly` alone for both (#2451).
		"integer readonly EPOCHSECONDS\n",
		"float readonly EPOCHREALTIME\n",
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
	// And the other half of the pair, in a shell that has read nothing: the
	// same three names carry no kind and no freeze there, because the
	// parameter is not in being yet. Without this the read above reads as
	// tidiness rather than as the state it selects.
	unprimed, st := runZsh(t, t.TempDir(), `typeset`)
	if st != 0 {
		t.Fatalf("status = %d, out %q", st, unprimed)
	}
	for _, want := range []string{
		"undefined keymaps\n",
		"undefined widgets\n",
		"undefined builtins\n",
		// And the control that says the deferral is per name rather than
		// per module: these two are registered by modules that declare no
		// autoloadable parameter, so they are there from the start and
		// list with their attributes in the same run.
		"array readonly errnos\n",
		"association readonly langinfo\n",
	} {
		if !containsLine(unprimed, want) {
			t.Errorf("unprimed = %q, want the whole line %q", unprimed, want)
		}
	}
}
