// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
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

// What zsh says about a `set` option it does not have, measured 2026-09-05 on
// 5.9.2. `set` is named in the location rather than in the sentence, as it is
// for every message here, and the dash is written into the wording: `set +q`
// is refused as `-q`, which dash also does and bash and ksh93 do not (#598).
func TestZshRefusesASetOptionInItsOwnWords(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"set -q\n", "zsh:set:1: bad option: -q\n"},
		{"set +q\n", "zsh:set:1: bad option: -q\n"},
		{"set -o zzznosuch\n", "zsh:set:1: no such option: zzznosuch\n"},
	} {
		if got := refuseInScript(t, c.src); got != c.want {
			t.Errorf("%q said %q, want %q", c.src, got, c.want)
		}
	}
}

// At an invocation the location goes entirely — no builtin, and no line,
// where the run-time form writes `zsh:set:1:`. No usage block follows either
// spelling; zsh is the shell in the panel that prints none anywhere.
func TestZshRefusesAnInvocationOptionWithNoLocationAtAll(t *testing.T) {
	got := refuseAtInvocation(t, func(r *interp.Runner) { r.SetOptionLetters("q", true) })
	if want := "zsh: bad option: -q\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	named := refuseAtInvocation(t, func(r *interp.Runner) { r.SetNamedOption("zzznosuch", true) })
	if want := "zsh: no such option: zzznosuch\n"; named != want {
		t.Errorf("got %q, want %q", named, want)
	}
}

// TestZshKeepsTheSetLettersItHasAndThisShellDoesNot: zsh gives a letter to
// far more of its options than the rest of the panel — measured, it refuses
// only b, c, j, q and z of the fifty-two and has the other forty-seven — so
// nearly every letter this shell does not implement is one zsh really has.
func TestZshKeepsTheSetLettersItHasAndThisShellDoesNot(t *testing.T) {
	// `-A` is not in the list: it assigns an array and is implemented — see
	// Semantics.SetArrayLetter. A letter both claimed and refused is dead
	// data that says the opposite of what the shell does.
	//
	// `-t` is not in it either, and for the other reason: this shell has the
	// letter and refuses to *move* it, which is a different sentence from one
	// we have not built — see Diagnostics.ImmovableOptionLetters and
	// TestZshRefusesTheOneCommandLetterInItsOwnWords.
	// `-p` is not in it either, and for a third reason: it is the short
	// spelling of `privileged`, a name this shell's own option table records,
	// so both directions of the letter are granted at 0 — which is what real
	// zsh answers. See Semantics.SetHasThePrivilegedLetter (#2412).
	const missing = "dgiklrswyBDEFGHIJKLMNOPQRSTUVWXYZ"
	if got := zsh.Diagnostics().UnimplementedOptionLetters["set"]; got != missing {
		t.Errorf("UnimplementedOptionLetters[set] = %q, want %q", got, missing)
	}
	for _, l := range missing {
		src := "set -" + string(l) + "\n"
		if got := refuseInScript(t, src); !strings.Contains(got, "is not implemented yet") {
			t.Errorf("%q said %q, want it called missing rather than bad", src, got)
		}
	}
	// The five zsh refuses itself get zsh's own words.
	for _, l := range "bcjqz" {
		src := "set -" + string(l) + "\n"
		want := "zsh:set:1: bad option: -" + string(l) + "\n"
		if got := refuseInScript(t, src); got != want {
			t.Errorf("%q said %q, want %q", src, got, want)
		}
	}
}
