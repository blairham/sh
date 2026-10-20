// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A `-m` operand is a pattern that selects names out of a table, and a
// pattern this engine cannot read is a **refusal** there rather than a miss:
// there is no subject for it to miss against, so "no names" is exactly what a
// correct pattern with nothing to pick out also says, and a script that
// mistyped a bracket was told its table was empty at 0 (#4741).
//
// Measured against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* —
// run `-f` under `env -i PATH=/usr/bin:/bin`, 2026-09-26.
//
// The control that made the gap look half-closed is in the grid below:
// `unhash -m '['` already exited 1 before this change, by the *other* route —
// nothing was removed — so the status alone said the rule was there at one of
// the surfaces when it was at none of them.

// The surfaces, each with its own name in the location. The complaint is
// `bad pattern : [` with a **space before the colon**, which is not the `bad
// pattern: [a` the same shell writes for a glob: two sentences one byte
// apart, and sharing one string would tie a change to either to the other.
func TestABadSelectionPatternIsRefusedAtEverySurface(t *testing.T) {
	for _, c := range []struct {
		name, src, complaint string
	}{
		{"typeset -m", `typeset -m '['`, "zsh:typeset:1: bad pattern : ["},
		{"typeset +fm", `typeset +fm '['`, "zsh:typeset:1: bad pattern : ["},
		{"typeset -fm", `typeset -fm '['`, "zsh:typeset:1: bad pattern : ["},
		{"declare -m", `declare -m '['`, "zsh:declare:1: bad pattern : ["},
		{"functions -m", `functions -m '['`, "zsh:functions:1: bad pattern : ["},
		{"functions +m", `functions +m '['`, "zsh:functions:1: bad pattern : ["},
		{"hash -m", `hash -m '['`, "zsh:hash:1: bad pattern : ["},
		{"hash -dm", `hash -dm '['`, "zsh:hash:1: bad pattern : ["},
		{"unhash -m", `unhash -m '['`, "zsh:unhash:1: bad pattern : ["},
		{"alias -m", `alias -m '['`, "zsh:alias:1: bad pattern : ["},
		{"unalias -m", `unalias -m '['`, "zsh:unalias:1: bad pattern : ["},
		{"unfunction -m", `unfunction -m '['`, "zsh:unfunction:1: bad pattern : ["},
		{"unset -m", `unset -m '['`, "zsh:unset:1: bad pattern : ["},
		{"unset -f -m", `unset -f -m '['`, "zsh:unset:1: bad pattern : ["},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, t.TempDir(), c.src+"\n")
			if out != "" {
				t.Errorf("stdout = %q, want nothing", out)
			}
			if st != 1 {
				t.Errorf("status = %d, want 1", st)
			}
			if !strings.Contains(errs, c.complaint) {
				t.Errorf("stderr = %q, want %q in it", errs, c.complaint)
			}
		})
	}
}

// The other half of the rule, and the half a refusal written for every `-m`
// operand would break: a pattern that is merely **unmatched** is silent, at
// whatever status that surface already gives a miss. Without these rows a
// mutant that refused every pattern would pass the grid above.
func TestAnUnmatchedSelectionPatternIsNotRefused(t *testing.T) {
	for _, c := range []struct {
		name, src string
		status    int
	}{
		// A listing asked a question and got the answer "none".
		{"typeset -m", `typeset -m 'zznosuch*'`, 0},
		{"functions -m", `functions -m 'zznosuch*'`, 0},
		{"hash -m", `hash -m 'zznosuch*'`, 0},
		{"alias -m", `alias -m 'zznosuch*'`, 0},
		// A removal that removed nothing has not done what it was asked, so
		// these are 1 — by their own route and not by a refusal.
		{"unhash -m", `unhash -m 'zznosuch*'`, 1},
		{"unalias -m", `unalias -m 'zznosuch*'`, 1},
		{"unfunction -m", `unfunction -m 'zznosuch*'`, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, t.TempDir(), c.src+"\n")
			if out != "" {
				t.Errorf("stdout = %q, want nothing", out)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
			if errs != "" {
				t.Errorf("stderr = %q, want nothing — the pattern is readable", errs)
			}
		})
	}
}

// What counts as unreadable, measured one operand at a time. Two rows here
// are the reason the scan is its own and not the matcher's:
//
//   - `[]` and `[]a` are **valid**, so the POSIX rule that the first `]`
//     after the bracket is an ordinary character does not hold for this
//     question — though it does hold for matching, in the same shell.
//   - `[[:alpha:]` is **invalid**, so a `[:name:]` inside a bracket is one
//     element whose own `]` does not close the bracket around it.
//
// And `a)b` is refused here while the same word is ordinary text at a
// matching surface, which is why a close with nothing open is a rule of its
// own rather than the mirror of the unterminated-group test.
func TestWhichSelectionPatternsAreRefused(t *testing.T) {
	for _, c := range []struct {
		pattern string
		refused bool
	}{
		{`[`, true},
		{`[a`, true},
		{`a[b`, true},
		{`[a-`, true},
		{`[!`, true},
		{`[[:alpha:]`, true},
		{`[]`, false},
		{`[]a`, false},
		{`[!]`, false},
		{`[)]`, false},
		{`[(]`, false},
		{`[[:alpha:]]`, false},
		{`a\[b`, false},
		{`\[`, false},
		{`a(b`, true},
		{`(`, true},
		{`)`, true},
		{`a)b`, true},
		{`(a|b`, true},
		{`(a))`, true},
		{`[a](b`, true},
		{`(a|b)`, false},
		{`((a))`, false},
		{`[a](b)`, false},
		{`a\(b`, false},
		{`a\)b`, false},
		{`*`, false},
		{`x`, false},
		{`(#i)a`, false},
		{`<1-3>`, false},
		{`{a,b}`, false},
	} {
		t.Run(c.pattern, func(t *testing.T) {
			src := "typeset -m " + singleQuoted(c.pattern) + "\n"
			// Standard output is deliberately not asserted: `*` is a legal
			// pattern that picks out every parameter in the shell, so a row
			// that demanded silence would be asking about the table rather
			// than about the pattern.
			_, st, errs := runZshSplit(t, t.TempDir(), src)
			refused := strings.Contains(errs, "bad pattern : ")
			if refused != c.refused {
				t.Errorf("stderr = %q status %d, want refused = %v", errs, st, c.refused)
			}
			if want := map[bool]int{true: 1, false: 0}[c.refused]; st != want {
				t.Errorf("status = %d, want %d", st, want)
			}
		})
	}
}

// singleQuoted spells a pattern as a shell word that reaches the builtin
// unchanged, which matters here because every case is punctuation.
func singleQuoted(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// **Every other operand on the line is still answered.** The refusal is per
// operand rather than per command — measured, `typeset -m 'zq*' '[' 'zr*'`
// writes both parameters and the complaint and exits 1 — so a change that
// gave up the whole line on the first bad pattern would be wrong in the
// direction that loses output.
func TestOneBadSelectionPatternDoesNotDiscardTheGoodOnes(t *testing.T) {
	src := "zq=1\nzr=2\ntypeset -m 'zq*' '[' 'zr*'\n"
	out, st, errs := runZshSplit(t, t.TempDir(), src)
	if want := "zq=1\nzr=2\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
	if !strings.Contains(errs, "bad pattern : [") {
		t.Errorf("stderr = %q, want the complaint in it", errs)
	}
}

// A refused line writes **nothing**, where a line with no `-m` operand at all
// writes the whole table. The two are a pair: dropping the early return that
// separates them turns `typeset -m '['` into a dump of every parameter in the
// shell, which is a listing at 1 and reads as an answer.
func TestARefusedSelectionLineDoesNotListTheWholeTable(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(), "zq=1\ntypeset -m '['\n")
	if out != "" {
		t.Errorf("stdout = %q, want nothing at all", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
	if !strings.Contains(errs, "bad pattern : [") {
		t.Errorf("stderr = %q, want the complaint in it", errs)
	}
	// The positive control: the same builtin with no `-m` does write names,
	// so the row above is not passing because the listing is broken.
	out, st, errs = runZshSplit(t, t.TempDir(), "zq=1\ntypeset -m 'zq*'\n")
	if want := "zq=1\n"; out != want || st != 0 || errs != "" {
		t.Errorf("control: out = %q status %d stderr %q, want %q 0 and nothing", out, st, errs, want)
	}
}

// The refusal is not the `BAD_PATTERN` option: that one decides whether
// *filename generation* spares a word it cannot compile, and it does not
// reach a selection pattern. Measured in both states, `typeset -m '['` is the
// complaint at 1 either way — the pair that says this rule is not keyed on
// the option a reader would reach for first.
func TestTheBadPatternOptionDoesNotReachASelectionPattern(t *testing.T) {
	for _, state := range []string{"setopt badpattern", "unsetopt badpattern"} {
		t.Run(state, func(t *testing.T) {
			out, st, errs := runZshSplit(t, t.TempDir(), state+"\ntypeset -m '['\n")
			if out != "" || st != 1 || !strings.Contains(errs, "bad pattern : [") {
				t.Errorf("out = %q status %d stderr = %q, want the complaint at 1", out, st, errs)
			}
		})
	}
}
