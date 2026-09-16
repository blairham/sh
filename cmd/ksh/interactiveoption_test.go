// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/testenv"
)

// This suite starts shells, so it runs in a home of its own — a shell reads
// startup files and writes history from the environment it was handed, and a
// suite that hands it the developer's environment is measuring the developer's
// dotfiles. internal/testenv is the guard; its own package comment is the
// argument for its shape.
func TestMain(m *testing.M) {
	if testenv.Assembled() {
		os.Exit(m.Run())
	}
	os.Exit(testenv.Run("cmd/ksh", m))
}

// `-o interactive` is this shell's second spelling of `-i`, and it prompts for
// it — #3221.
//
// The column that shows the name is not simply another way to spell a letter:
// this shell **refuses** it to a running script, and takes it on the command
// line that started the shell. Measured 2026-09-16 on ksh93u+ 2012-08-01 with
// the program on a pipe, so that nothing but the invocation could make the
// shell interactive: `ksh -o interactive` writes `$ `, runs the line and
// writes `$ ` again at status 0, while `set -o interactive` inside that same
// shell is `set: interactive: bad option(s)` at 2 in both directions. See
// Semantics.ImmovableOptionsSetAtInvocation, which is the split, and
// dialect/ksh's setoroster_test.go for the refusal that stays.
//
// The negative spelling is the shell's own `no` prefix over a name its `set
// -o` listing does not carry, and it composes with the sign the way zsh's
// does: `-o nointeractive` draws no prompt and `+o nointeractive` draws one,
// both measured on the same run.
//
// Against the binary's own shell value rather than the library's, because what
// this decides is a *route* and the route is the front end's.
func TestTheInteractiveOptionNameDrawsAPrompt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		argv    []string
		prompts bool
	}{
		{"the option alone", []string{"ksh", "-o", "interactive"}, true},
		{"the letter, for comparison", []string{"ksh", "-i"}, true},
		{"nothing written", []string{"ksh"}, false},
		{"the option under a plus", []string{"ksh", "+o", "interactive"}, false},
		{"the negative name", []string{"ksh", "-o", "nointeractive"}, false},
		{"the negative name under a plus", []string{"ksh", "+o", "nointeractive"}, true},
		{"the name attached to its letter", []string{"ksh", "-ointeractive"}, true},
		{"the letter then the option", []string{"ksh", "-i", "+o", "interactive"}, false},
		{"the option then the letter", []string{"ksh", "+o", "interactive", "-i"}, true},
		{"the letter then its plus", []string{"ksh", "-i", "+i"}, false},
		{"the plus then the letter", []string{"ksh", "+i", "-i"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			out, errs, code := prompt(t, "echo TYPED\n", tc.argv...)
			both := out + errs
			if code != 0 {
				t.Fatalf("status %d, out %q, stderr %q", code, out, errs)
			}
			if !strings.Contains(both, "TYPED") {
				t.Fatalf("said %q, want the typed line to have run under either answer", both)
			}
			if got := strings.Contains(both, "$ "); got != tc.prompts {
				t.Errorf("prompted = %v, want %v — said %q", got, tc.prompts, both)
			}
		})
	}
}

// And the state the four spellings write, which is the same one `-i` writes:
// `$-` gains `i` and the `set -o` listing says `interactive on`. Measured
// 2026-09-16 on ksh93u+, `ksh -o interactive -c 'echo $-'` reports `icmsBE`
// and `ksh +o interactive -c` reports `chsB`, byte for byte with `ksh -i -c`
// and a plain `ksh -c`.
func TestTheInteractiveOptionNameWritesTheStateTheLetterWrites(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		want bool
	}{
		{"the name", []string{"ksh", "-o", "interactive", "-c", "echo $-"}, true},
		{"the name under a plus", []string{"ksh", "+o", "interactive", "-c", "echo $-"}, false},
		{"the negative name", []string{"ksh", "-o", "nointeractive", "-c", "echo $-"}, false},
		{"the negative name under a plus", []string{"ksh", "+o", "nointeractive", "-c", "echo $-"}, true},
		{"the letter", []string{"ksh", "-i", "-c", "echo $-"}, true},
		{"the letter then its plus", []string{"ksh", "-i", "+i", "-c", "echo $-"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			out, errs, code := prompt(t, "", tc.argv...)
			if code != 0 {
				t.Fatalf("status %d, out %q, stderr %q", code, out, errs)
			}
			if got := strings.Contains(out, "i"); got != tc.want {
				t.Errorf("`i` in $- = %v, want %v — $- was %q", got, tc.want, strings.TrimSpace(out))
			}
		})
	}
}

// A script still cannot move the name, which is the half of the split that
// was already right and the half a grant at an invocation could quietly take
// away. Measured on ksh93u+: `set -o interactive` and `set -o nointeractive`
// are both `bad option(s)` at 2, word for word with a name it has never heard
// of, in a shell started with `-i` and in one started without.
func TestAScriptStillCannotMoveTheName(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
	}{
		{"the name on", "set -o interactive"},
		{"the name off", "set +o interactive"},
		{"the negative name on", "set -o nointeractive"},
		{"the negative name off", "set +o nointeractive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			out, errs, code := prompt(t, "", "ksh", "-c", tc.line)
			if code != 2 {
				t.Fatalf("status %d, want 2 — out %q, stderr %q", code, out, errs)
			}
			if !strings.Contains(errs, "bad option(s)") {
				t.Errorf("said %q, want `bad option(s)`", errs)
			}
		})
	}
}

// prompt drives the binary's own shell value with the typed lines on a pipe
// standing in for a person.
func prompt(t *testing.T, typed string, argv ...string) (out, errs string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	sh := shell()
	sh.SystemStartupDirectory = t.TempDir()
	sh.Stdout, sh.Stderr = &o, &e
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString(typed)
		_ = w.Close()
	}()
	t.Cleanup(func() { _ = r.Close() })
	sh.Stdin = r
	code = driver.MainArgs(sh, argv)
	return o.String(), e.String(), code
}
