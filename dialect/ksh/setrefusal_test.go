// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

func refuseInScript(t *testing.T, src string) string {
	t.Helper()
	f, err := syntax.Parse(src, ksh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var errs strings.Builder
	sem, diag := ksh.Semantics(), ksh.Diagnostics()
	r := &interp.Runner{
		Stdout: &strings.Builder{}, Stderr: &errs,
		Semantics: &sem, Diagnostics: &diag, Name: "/bin/ksh",
	}
	ksh.Apply(r)
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return errs.String()
}

func refuseAtInvocation(t *testing.T, apply func(*interp.Runner)) string {
	t.Helper()
	var errs strings.Builder
	sem, diag := ksh.Semantics(), ksh.Diagnostics()
	r := &interp.Runner{
		Stdout: &strings.Builder{}, Stderr: &errs,
		Semantics: &sem, Diagnostics: &diag, Name: "/bin/ksh",
	}
	ksh.Apply(r)
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

// TestKshKeepsTheSetLettersItHasAndThisShellDoesNot: ten letters ksh93 has —
// including `-A`, which assigns an array rather than switching anything.
func TestKshKeepsTheSetLettersItHasAndThisShellDoesNot(t *testing.T) {
	if got, want := ksh.Diagnostics().UnimplementedOptionLetters["set"], "bkprstABGH"; got != want {
		t.Errorf("UnimplementedOptionLetters[set] = %q, want %q", got, want)
	}
	for _, l := range "bkprstABGH" {
		src := "set -" + string(l) + "\n"
		if got := refuseInScript(t, src); !strings.Contains(got, "is not implemented yet") {
			t.Errorf("%q said %q, want it called missing rather than unknown", src, got)
		}
	}
}
