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
func TestKshRefusesAnInvocationOptionWithItsOwnUsageLine(t *testing.T) {
	const usage = "Usage: ksh [-cilrsDEabefhkmnprtuvxBCGH] [-R file] [-o[option]] [arg ...]\n"
	got := refuseAtInvocation(t, func(r *interp.Runner) { r.SetOptionLetters("q", true) })
	if want := "/bin/ksh: -q: unknown option\n" + usage; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	named := refuseAtInvocation(t, func(r *interp.Runner) { r.SetNamedOption("zzznosuch", true) })
	if want := "/bin/ksh: zzznosuch: bad option(s)\n" + usage; named != want {
		t.Errorf("got %q, want %q", named, want)
	}
}

// TestKshKeepsTheSetLettersItHasAndThisShellDoesNot: nine letters ksh93 has
// and this shell does not.
//
// `-A` has left the list. It assigns an array rather than switching anything,
// and it is implemented — see Semantics.SetArrayLetter. A letter both claimed
// and refused is dead data that says the opposite of what the shell does, and
// dialect.TestNoLetterIsBothImplementedAndNot is the invariant for the
// builtins whose letters are an optstring; `set`'s are a switch, so this is
// the row that has to be kept honest by hand.
func TestKshKeepsTheSetLettersItHasAndThisShellDoesNot(t *testing.T) {
	if got, want := ksh.Diagnostics().UnimplementedOptionLetters["set"], "bkprstBGH"; got != want {
		t.Errorf("UnimplementedOptionLetters[set] = %q, want %q", got, want)
	}
	for _, l := range "bkprstBGH" {
		src := "set -" + string(l) + "\n"
		if got := refuseInScript(t, src); !strings.Contains(got, "is not implemented yet") {
			t.Errorf("%q said %q, want it called missing rather than unknown", src, got)
		}
	}
}
