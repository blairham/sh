// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A pattern this shell will not compile is refused wherever it is used as a
// pattern — including as the **operand of a parameter expansion**, which is
// where it was not (#4646). `${v#[a}` stripped `[a` as two literal characters
// and reported success: a plausible value at status 0, which is the one shape
// a script cannot notice.
//
// Measured against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* —
// run `-f -c`, 2026-09-26.
//
// This is a different question from #4630's `badpattern`, and the rows below
// hold the proof: every one of them answers the same way in **both** states of
// that option, because the option is read at filename generation and reaches
// nothing here. The `unsetopt badpattern` row is what says so.

// TestAnOperandThisShellWillNotCompileIsRefused walks every pattern-taking
// operator a parameter expansion has. The `:#` element filter is in the list
// as a **control**: it took a different route to the matcher and was already
// right, so a grid without it could not say whether a change had moved the
// broken surfaces or merely broken the working one.
func TestAnOperandThisShellWillNotCompileIsRefused(t *testing.T) {
	for _, c := range []struct {
		name, src string
		wantErr   string
	}{
		{"trim a short prefix", `v='[abc'; print -r -- ${v#[a}`, "bad pattern: [a"},
		{"trim a long prefix", `v='[abc'; print -r -- ${v##[a}`, "bad pattern: [a"},
		{"trim a short suffix", `v='bc[a'; print -r -- ${v%[a}`, "bad pattern: [a"},
		{"trim a long suffix", `v='bc[a'; print -r -- ${v%%[a}`, "bad pattern: [a"},
		{"substitute once", `v='[abc'; print -r -- ${v/[a/x}`, "bad pattern: [a"},
		{"substitute everywhere", `v='[abc'; print -r -- ${v//[a/x}`, "bad pattern: [a"},
		{"substitute anchored at the front", `v='[abc'; print -r -- ${v/#[a/x}`, "bad pattern: [a"},
		{"substitute anchored at the end", `v='bc[a'; print -r -- ${v/%[a/x}`, "bad pattern: [a"},
		{"delete rather than replace", `v='[abc'; print -r -- ${v/[a}`, "bad pattern: [a"},
		{"a searching trim", `v='x[abc'; print -r -- ${(S)v#[a}`, "bad pattern: [a"},
		{"a projected trim", `v='[abc'; print -r -- ${(MB)v#[a}`, "bad pattern: [a"},
		// The lone bracket, which the *filesystem* spares so that `[ a = a ]`
		// keeps working. There is no command name here to protect, and the
		// reference refuses it: `${v#[}` is `bad pattern: [` at 1.
		{"a lone bracket", `v='[abc'; print -r -- ${v#[}`, "bad pattern: ["},
		// The control that was already right, reached through matchPatternR.
		{"the element filter", `v='[a'; print -r -- ${v:#[a}`, "bad pattern: [a"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src+`; print -r -- AFTER`)
			if !strings.Contains(out, c.wantErr) {
				t.Errorf("got %q, want it to report %q", out, c.wantErr)
			}
			if strings.Contains(out, "AFTER") {
				t.Errorf("the script should end over the pattern, got %q", out)
			}
			if st != 1 {
				t.Errorf("status = %d, want 1", st)
			}
		})
	}
}

// TestTheRefusalIsAboutCompilingAndNotAboutMatching is the pair the wrong-noun
// trap here is killed by: a pattern that will not compile and a pattern that
// simply misses leave the **same value** behind and differ only in status, so
// a probe reading the value cannot tell them apart.
//
// The second row is the one an implementation raising the refusal from inside
// the matcher cannot produce. `x[a` is ruled out by its first character before
// any bracket is read — and the span walk skips candidate pieces the pattern's
// edge literals could not fill, so on this subject the matcher is not called
// at all. The reference refuses it just the same, which is what makes this a
// question about the pattern rather than about the match.
func TestTheRefusalIsAboutCompilingAndNotAboutMatching(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"a pattern that misses", `v=zzz; print -r -- ${v#q}`, "zzz\n", 0},
		{
			"a pattern that will not compile, on a value it could not match",
			`v=zzz; print -r -- ${v#[a}`, "bad pattern: [a", 1,
		},
		{
			"a pattern whose first character already misses",
			`v=zzz; print -r -- ${v#x[a}`, "bad pattern: x[a", 1,
		},
		{"a bracket that closes", `v='[abc'; print -r -- ${v#[ab]}`, "[abc\n", 0},
		{
			"a bracket a class left open",
			`v='[abc'; print -r -- ${v#[[:alpha:]}`, "bad pattern: [[:alpha:]", 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if !strings.Contains(out, c.want) {
				t.Errorf("got %q, want %q", out, c.want)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
		})
	}
}

// TestAQuotedBracketIsNotAPattern is the other half of the pair above, and it
// is what keeps the refusal from swallowing ordinary text. A bracket that was
// quoted or escaped, and a value used as a pattern without `${~…}`, are all
// characters rather than a bracket expression — measured, each is stripped and
// reported at status 0.
func TestAQuotedBracketIsNotAPattern(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"quoted", `v='[abc'; print -r -- ${v#"["}`, "abc\n"},
		{"escaped", `v='[abc'; print -r -- ${v#\[}`, "abc\n"},
		{"a value, which is text without the tilde", `v='[abc'; p='[a'; print -r -- ${v#$p}`, "bc\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q status 0", out, st, c.want)
			}
		})
	}
	// And the tilde is what turns the same value into a pattern, so the
	// refusal arrives through it. The pair holds the *text* fixed and moves
	// only whether it is read as a pattern.
	out, st := runZsh(t, t.TempDir(), `v='[abc'; p='[a'; print -r -- ${v#${~p}}`)
	if !strings.Contains(out, "bad pattern: [a") || st != 1 {
		t.Errorf("got %q status %d, want a refusal at 1", out, st)
	}
}

// TestTheOperandRefusalIsNotTheGlobbingOption is why this is not #4630.
// `badpattern` is read at filename generation and nowhere else, so every row
// above answers the same way in both of its states — measured on the
// reference, which reports `bad pattern: [a` for `${v#[a}` with the option off
// as well as on.
//
// `emulate sh` and `emulate ksh` are here for the same reason: both reach the
// option without anybody typing `setopt`, and neither moves this surface.
func TestTheOperandRefusalIsNotTheGlobbingOption(t *testing.T) {
	for _, prefix := range []string{
		"",
		"unsetopt badpattern; ",
		"setopt badpattern; ",
		"emulate sh; ",
		"emulate ksh; ",
	} {
		out, st := runZsh(t, t.TempDir(), prefix+`v='[abc'; print -r -- ${v#[a}; print -r -- AFTER`)
		if !strings.Contains(out, "bad pattern: [a") || strings.Contains(out, "AFTER") || st != 1 {
			t.Errorf("%q: got %q status %d, want a refusal at 1", prefix, out, st)
		}
	}
	// The control that says the option is not simply inert: with it off, a
	// malformed glob on its way to the filesystem is still handed back whole.
	if out, st := runZsh(t, t.TempDir(), `unsetopt badpattern; print -r -- [a`); out != "[a\n" || st != 0 {
		t.Errorf("globbing still moves with the option: got %q status %d", out, st)
	}
}

// TestHowFarTheOperandRefusalReaches pins what a script can do about it, which
// is the half that decides whether a caller can defend against this at all.
// Measured on the reference, `-c`: an `||` does not catch it, a function body
// ends the whole script, and an `eval` or a subshell contains it and leaves 1
// behind.
func TestHowFarTheOperandRefusalReaches(t *testing.T) {
	dir := t.TempDir()
	t.Run("an or-list does not catch it", func(t *testing.T) {
		out, st := runZsh(t, dir, `v='[abc'; print -r -- ${v#[a} || print -r -- CAUGHT; print -r -- AFTER`)
		if strings.Contains(out, "CAUGHT") || strings.Contains(out, "AFTER") || st != 1 {
			t.Errorf("got %q status %d", out, st)
		}
	})
	t.Run("a function body ends the script", func(t *testing.T) {
		out, st := runZsh(t, dir, `v='[abc'; f() { print -r -- ${v#[a}; }; f; print -r -- AFTER`)
		if strings.Contains(out, "AFTER") || st != 1 {
			t.Errorf("got %q status %d", out, st)
		}
	})
	t.Run("an eval contains it", func(t *testing.T) {
		out, st := runZsh(t, dir, `v='[abc'; eval 'print -r -- ${v#[a}'; print -r -- "AFTER $?"`)
		if !strings.Contains(out, "AFTER 1") || st != 0 {
			t.Errorf("got %q status %d, want the script to run on", out, st)
		}
	})
	t.Run("a subshell contains it", func(t *testing.T) {
		out, st := runZsh(t, dir, `v='[abc'; ( print -r -- ${v#[a} ); print -r -- "AFTER $?"`)
		if !strings.Contains(out, "AFTER 1") || st != 0 {
			t.Errorf("got %q status %d, want the script to run on", out, st)
		}
	})
}
