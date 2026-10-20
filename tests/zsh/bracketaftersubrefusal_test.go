// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A bracket a `[:name:]` left open — `[[:alpha:]` — is a bad pattern in this
// shell, and it was refused only when the matcher happened to walk as far as
// the bracket. So *whether the shell refused depended on the value*, which is
// not a distinction the reference draws (#4659).
//
// Measured 2026-09-26 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — run `-f -c`; `go version -m` says *not a Go
// executable* for it.
//
// **Every row uses a subject the pattern's first character already rules
// out**, which is the half the lazy verdict could not produce. The first
// test's control is the same pattern with that character taken off: it
// reaches the matcher, and it was refused before this change and after it —
// so a suite written only that way passed throughout.
func TestABracketAClassLeftOpenIsRefusedOnEverySurface(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a prefix trim", `v=zzz; print -r -- ${v#x[[:alpha:]}`, "bad pattern: x[[:alpha:]"},
		{"a suffix trim", `v=zzz; print -r -- ${v%[[:alpha:]y}`, "bad pattern: [[:alpha:]y"},
		{"an element filter", `v=zzz; print -r -- ${v:#x[[:alpha:]}`, "bad pattern: x[[:alpha:]"},
		{
			"a case arm",
			`case zzz in (x[[:alpha:]) print ONE;; (*) print STAR;; esac`,
			"bad pattern: x[[:alpha:]",
		},
		{
			"a condition operand",
			`[[ zzz == x[[:alpha:] ]] && print YES || print NO`,
			"bad pattern: x[[:alpha:]",
		},
		{"a filename pattern", `print -r -- x[[:alpha:]`, "bad pattern: x[[:alpha:]"},
		{
			// A class in the middle of the set, and two of them, are the
			// same shape reached two other ways.
			"a class after an ordinary member",
			`v=zzz; print -r -- ${v#x[q[:alpha:]}`, "bad pattern: x[q[:alpha:]",
		},
		{
			"two classes",
			`v=zzz; print -r -- ${v#x[[:alpha:][:digit:]}`, "bad pattern: x[[:alpha:][:digit:]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, errs := runZshSplit(t, t.TempDir(), c.src+"\nprint after\n")
			if !strings.Contains(errs, c.want) {
				t.Errorf("stderr = %q, want %q in it", errs, c.want)
			}
			if out != "" {
				t.Errorf("stdout = %q, want nothing — the shell leaves", out)
			}
		})
	}
}

// The control that says the instrument fires on both sides: the same pattern
// against a subject its first character does not rule out reaches the matcher
// and is refused there, as it always was.
func TestTheSameBracketIsRefusedWhenTheMatcherReachesIt(t *testing.T) {
	out, _, errs := runZshSplit(t, t.TempDir(), "v=zzz\nprint -r -- ${v#[[:alpha:]}\nprint after\n")
	if !strings.Contains(errs, "bad pattern: [[:alpha:]") {
		t.Errorf("stderr = %q, want the refusal", errs)
	}
	if out != "" {
		t.Errorf("stdout = %q, want nothing", out)
	}
}

// And the patterns that **do** compile, which keep the verdict from becoming
// "any pattern with a class in it".
//
// The last two are this shell's own: it reads no collating element, so
// `[[.a.]` closes at the `]` it can see — measured, `${v#x[[.a.]}` is `zzz`
// at status 0 there — and a `\[` opens no bracket at all.
func TestAClassThatClosesItsBracketIsNotRefused(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a closed class", `v=azzz; print -r -- ${v#[[:alpha:]]}`, "zzz\n"},
		{"a bracket closed after the class", `v=zzz; print -r -- ${v#x[[:alpha:]]}`, "zzz\n"},
		{"a collating delimiter this shell does not read", `v=zzz; print -r -- ${v#x[[.a.]}`, "zzz\n"},
		{"an escaped bracket", `v=zzz; print -r -- ${v#x\[[:alpha:]}`, "zzz\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, t.TempDir(), c.src+"\n")
			if out != c.want {
				t.Errorf("stdout = %q, want %q (stderr %q)", out, c.want, errs)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
			if errs != "" {
				t.Errorf("stderr = %q, want nothing", errs)
			}
		})
	}
}

// `unsetopt badpattern` spares the word on its way to the filesystem here
// exactly as it spares a plain unterminated bracket, which is what says this
// refusal went through the one switch rather than around it.
func TestBadPatternStillSparesAFilenamePatternAClassLeftOpen(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(),
		"unsetopt badpattern\nprint -r -- x[[:alpha:]\n")
	if want := "x[[:alpha:]\n"; out != want {
		t.Errorf("stdout = %q, want %q (stderr %q)", out, want, errs)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	// The control for the row above: with the option on the same word is
	// refused, so the row is the option and not a shell that stopped looking.
	_, st, errs = runZshSplit(t, t.TempDir(), "setopt badpattern\nprint -r -- x[[:alpha:]\n")
	if !strings.Contains(errs, "bad pattern: x[[:alpha:]") || st != 1 {
		t.Errorf("with the option on: stderr = %q status %d", errs, st)
	}
}
