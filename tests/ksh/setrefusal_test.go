// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// refuseInScript runs src and reports what reached standard error.
func refuseInScript(t *testing.T, src string) string {
	t.Helper()
	f := preset.Parse(t, src)
	var errs strings.Builder
	r := preset.Runner(dialecttest.Base{Stdout: &strings.Builder{}, Stderr: &errs, Name: "/bin/ksh"})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return errs.String()
}

// refuseAtInvocation is the other route: the front end applying an option the
// shell was started with, before anything has been read.
// The name is the path, because this shell writes `$0` as it was invoked
// and these cases are about the exact sentence.

func refuseAtInvocation(t *testing.T, apply func(*interp.Runner)) string {
	t.Helper()
	var errs strings.Builder
	r := preset.Runner(dialecttest.Base{Stdout: &strings.Builder{}, Stderr: &errs, Name: "/bin/ksh"})
	apply(r)
	return errs.String()
}

// What ksh93 says about a `set` option it does not have, measured 2026-09-05
// on 93u+ 2012-08-01. It echoes back the sign it was asked with, as bash
// does, and it is the one shell that repeats `set`'s usage line under the
// long spelling as well as under the letter (#598).
func TestKshRefusesASetOptionInItsOwnWords(t *testing.T) {
	const usage = "Usage: set [-sabefhkmnprtuvxBCGH] [-A name] [-o[option]] [arg ...]\n"
	for _, c := range []struct{ src, want string }{
		{"set -q\n", "/bin/ksh: set: -q: unknown option\n" + usage},
		{"set +q\n", "/bin/ksh: set: +q: unknown option\n" + usage},
		{"set -o zzznosuch\n", "/bin/ksh: set: zzznosuch: bad option(s)\n" + usage},
	} {
		if got := refuseInScript(t, c.src); got != c.want {
			t.Errorf("%q said %q, want %q", c.src, got, c.want)
		}
	}
}

// At an invocation ksh93 stops naming `set` and prints the *shell's* usage
// line instead of the builtin's — naming itself there by the last element of
// the word it was invoked by, where bash spells the whole path. Measured
// through a link named `myksh`, which is what it called itself.
//
// The *sentence* above that block names itself the same way, which this test
// asserted the other way round until it was measured. Invoked as `/bin/ksh`,
// 93u+ 2012-08-01 writes `ksh: -Z: unknown option` and `ksh: zzznosuch: bad
// option(s)` — and `/bin/ksh: /nosuch.sh: not found` for a script it cannot
// open, from the same invocation with no line read either. So the base name
// is the *option refusal's* and not the route's: it is the AST option reader
// speaking rather than the shell. See
// Diagnostics.InvocationOptionRefusalNamesTheBase.
func TestKshRefusesAnInvocationOptionWithItsOwnUsageLine(t *testing.T) {
	const usage = "Usage: ksh [-cilrsDEabefhkmnprtuvxBCGH] [-R file] [-o[option]] [arg ...]\n"
	got := refuseAtInvocation(t, func(r *interp.Runner) { r.SetOptionLetters("q", true) })
	if want := "ksh: -q: unknown option\n" + usage; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	named := refuseAtInvocation(t, func(r *interp.Runner) { r.SetNamedOption("zzznosuch", true) })
	if want := "ksh: zzznosuch: bad option(s)\n" + usage; named != want {
		t.Errorf("got %q, want %q", named, want)
	}
	// The long spelling: the same sentence under a different block, the one
	// that names no letters because the spelling refused has none.
	long := refuseAtInvocation(t, func(r *interp.Runner) { r.SetLongOption("zzznosuch") })
	if want := "ksh: zzznosuch: bad option(s)\nUsage: ksh [ options ] [arg ...]\n"; long != want {
		t.Errorf("got %q, want %q", long, want)
	}
}

// TestKshKeepsTheSetLettersItHasAndThisShellDoesNot: the letters ksh93 has
// and this shell does not.
//
// `-A` has left the list. It assigns an array rather than switching anything,
// and it is implemented — see Semantics.SetArrayLetter. `-t` has left it for
// the same reason: the shell really does stop after one command now, and the
// letter is the only spelling ksh93 has for that option — see
// Semantics.SetHasTheTLetter. `-B` left in #1856, brace expansion having
// become a switch this shell really holds. A letter both claimed
// and refused is dead data that says the opposite of what the shell does, and
// dialect.TestNoLetterIsBothImplementedAndNot is the invariant for the
// builtins whose letters are an optstring; `set`'s are a switch, so this is
// the row that has to be kept honest by hand. `-H` left in #3093: ksh93's
// `-H` is `set -o histexpand`, and this shell has the expander behind it.
// `-k` left in #3095 for the same reason again: the keyword option is built
// and `set -o keyword` grants it under its long name, so a letter listed here
// would refuse what the name gives. `-G` left in #3152, by the other road:
// `set -o globstar` is wired to the walk and the letter goes through this
// dialect's own letter table, which wins over the shared reading because `G`
// is a letter two shells spell different options with — zsh's is `nullglob`.
// `-s` left when the sort was built: it orders the operands rather than
// switching anything — see Semantics.SetSLetterSortsTheOperands.
func TestKshKeepsTheSetLettersItHasAndThisShellDoesNot(t *testing.T) {
	// `-p` left in #2412: it is the short spelling of `privileged`, which
	// this shell answers through the `set -o` table, so `set +p` is granted
	// and `set -p` is refused by the name. A letter routed to a name must
	// not also be listed here.
	// `-r` left in #4205, when this shell's restricted mode was built: the
	// letter enters it and `set +r` leaves it again, so a letter listed here
	// would refuse what Semantics.SetHasTheRestrictedLetter grants. The pairing
	// is the same one `-t`, `-B`, `-H`, `-k` and `-G` have each broken once, and
	// there is a *third* table beside these two — Runner.hasSetLetter, the
	// validating pass, where the letter was still unknown after the axis said
	// yes and the refusal read `set: -r: unknown option`.
	if got, want := ksh.Diagnostics().UnimplementedOptionLetters["set"], "b"; got != want {
		t.Errorf("UnimplementedOptionLetters[set] = %q, want %q", got, want)
	}
	for _, l := range "b" {
		src := "set -" + string(l) + "\n"
		if got := refuseInScript(t, src); !strings.Contains(got, "is not implemented yet") {
			t.Errorf("%q said %q, want it called missing rather than unknown", src, got)
		}
	}
}
