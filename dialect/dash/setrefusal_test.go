// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
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

// What dash says about a `set` option it does not have, measured 2026-09-05.
// The dash is written into the sentence rather than echoed: `set +q` is
// refused as `-q` here, where bash and ksh93 write back the sign they were
// given. No usage line follows either spelling (#598).
func TestDashRefusesASetOptionInItsOwnWords(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"set -q\n", "dash: 1: set: Illegal option -q\n"},
		{"set +q\n", "dash: 1: set: Illegal option -q\n"},
		{"set -o zzznosuch\n", "dash: 1: set: Illegal option -o zzznosuch\n"},
	} {
		if got := refuseInScript(t, c.src); got != c.want {
			t.Errorf("%q said %q, want %q", c.src, got, c.want)
		}
	}
}

// At an invocation dash says the same sentence with nothing naming `set`, and
// keeps the line it has not reached yet — the nought its script diagnostics
// already write there, and the one thing in the panel that writes a number
// at all before anything has been read.
func TestDashRefusesAnInvocationOptionAtLineNought(t *testing.T) {
	got := refuseAtInvocation(t, func(r *interp.Runner) { r.SetOptionLetters("q", true) })
	if want := "dash: 0: Illegal option -q\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	named := refuseAtInvocation(t, func(r *interp.Runner) { r.SetNamedOption("zzznosuch", true) })
	if want := "dash: 0: Illegal option -o zzznosuch\n"; named != want {
		t.Errorf("got %q, want %q", named, want)
	}
}

// TestDashKeepsTheSetLettersItHasAndThisShellDoesNot: `-b`, `-i`, `-s` and
// the three about its line editor and end-of-file are dash's own.
func TestDashKeepsTheSetLettersItHasAndThisShellDoesNot(t *testing.T) {
	if got, want := dash.Diagnostics().UnimplementedOptionLetters["set"], "bisEIV"; got != want {
		t.Errorf("UnimplementedOptionLetters[set] = %q, want %q", got, want)
	}
	for _, l := range "bisEIV" {
		src := "set -" + string(l) + "\n"
		if got := refuseInScript(t, src); !strings.Contains(got, "is not implemented yet") {
			t.Errorf("%q said %q, want it called missing rather than illegal", src, got)
		}
	}
}
