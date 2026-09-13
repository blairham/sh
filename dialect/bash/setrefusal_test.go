// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// refuseInScript runs src and reports what reached standard error.
func refuseInScript(t *testing.T, src string) string {
	t.Helper()
	f := preset.Parse(t, src)
	var errs strings.Builder
	r := preset.Runner(dialecttest.Base{Stdout: &strings.Builder{}, Stderr: &errs})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return errs.String()
}

// refuseAtInvocation is the other route: the front end applying an option the
// shell was started with, before anything has been read.
func refuseAtInvocation(t *testing.T, apply func(*interp.Runner)) string {
	t.Helper()
	var errs strings.Builder
	r := preset.Runner(dialecttest.Base{Stdout: &strings.Builder{}, Stderr: &errs})
	apply(r)
	return errs.String()
}

// What bash says about a `set` option it does not have, measured 2026-09-05
// on bash 5.3.15 with a scratch HOME and no startup files. `-q` is one of the
// four letters (`-q`, `-j`, `-z`, `-A`) every shell in the panel refuses.
//
// The usage line under the letter and not under the name is bash's own shape
// rather than an oversight: the letter is refused the way any builtin's bad
// letter is, and the name earns a sentence of its own (#598).
func TestBashRefusesASetOptionInItsOwnWords(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{
			"set -q\n",
			"bash: line 1: set: -q: invalid option\n" +
				"set: usage: set [-abefhkmnptuvxBCEHPT] [-o option-name] [--] [-] [arg ...]\n",
		},
		// The sign is echoed back, which dash and zsh do not do.
		{
			"set +q\n",
			"bash: line 1: set: +q: invalid option\n" +
				"set: usage: set [-abefhkmnptuvxBCEHPT] [-o option-name] [--] [-] [arg ...]\n",
		},
		{"set -o zzznosuch\n", "bash: line 1: set: zzznosuch: invalid option name\n"},
	} {
		if got := refuseInScript(t, c.src); got != c.want {
			t.Errorf("%q said %q, want %q", c.src, got, c.want)
		}
	}
}

// At an invocation bash names itself instead of `set` and prints its whole
// shell usage block — and for the long spelling only, hands the refusal to
// the builtin and writes its own name where `set` would stand, location and
// all. Two shapes from one shell, both measured.
func TestBashRefusesAnInvocationOptionWithItsOwnUsageBlock(t *testing.T) {
	got := refuseAtInvocation(t, func(r *interp.Runner) { r.SetOptionLetters("q", true) })
	first, rest, _ := strings.Cut(got, "\n")
	if want := "bash: -q: invalid option"; first != want {
		t.Errorf("first line %q, want %q", first, want)
	}
	for _, w := range []string{
		"Usage:\tbash [GNU long option] [option] ...\n",
		"\tbash [GNU long option] [option] script-file ...\n",
		"GNU long options:\n\t--debug\n",
		"Shell options:\n\t-ilrsD or -c command or -O shopt_option\t\t(invocation only)\n",
		"\t-abefhkmnptuvxBCEHPT or -o option\n",
	} {
		if !strings.Contains(rest, w) {
			t.Errorf("the usage block %q is missing %q", rest, w)
		}
	}
	if strings.Contains(got, "set:") {
		t.Errorf("said %q, want nothing naming the builtin at an invocation", got)
	}

	named := refuseAtInvocation(t, func(r *interp.Runner) { r.SetNamedOption("zzznosuch", true) })
	if want := "bash: line 0: bash: zzznosuch: invalid option name\n"; named != want {
		t.Errorf("got %q, want %q", named, want)
	}
}

// TestBashKeepsTheSetLettersItHasAndThisShellDoesNot: `set -b` is bash's, and
// this shell not having it is a different answer from bash refusing it.
//
// `-t` is deliberately absent: the option behind it is built, so the letter
// belongs to the accepted set and not to this one. A letter in both tables is
// the pairing that broke in #1709 — listed by one and refused by the other.
// `-B` left for the same reason in #1856: brace expansion really switches off
// now, so the letter is accepted and its old line here would be dead data.
//
// `-p` left in #2412 and is the third shape: the letter is the short spelling
// of `privileged`, which this shell answers through the `set -o` table like
// any other name it does not implement — `set +p` is granted because the
// shell is already in the state it asks for, and `set -p` is refused by the
// name rather than by the letter. A letter routed to a name must not also be
// listed here, or the two would give different sentences for one question.
func TestBashKeepsTheSetLettersItHasAndThisShellDoesNot(t *testing.T) {
	if got, want := bash.Diagnostics().UnimplementedOptionLetters["set"], "bkrHP"; got != want {
		t.Errorf("UnimplementedOptionLetters[set] = %q, want %q", got, want)
	}
	for _, l := range "bkrHP" {
		src := "set -" + string(l) + "\n"
		if got := refuseInScript(t, src); !strings.Contains(got, "is not implemented yet") {
			t.Errorf("%q said %q, want it called missing rather than invalid", src, got)
		}
	}
}
