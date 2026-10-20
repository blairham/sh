// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An empty bracket expression **compiles** in this shell and matches nothing,
// where the other five leave it unterminated. We called it unterminated too
// and refused the pattern (#4644).
//
// Measured 2026-09-26 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* —
// run `-f -c` in the directory emptyBracketDir builds.

// emptyBracketDir holds the names that tell the two readings apart:
//
//	a  z  ~   one character each, so a set of one character can name them
//	]         the member `[]]` holds under the reading every column shares
//	a]        two characters, which nothing here should match
func emptyBracketDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"a", "z", "~", "]", "a]"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// **The controls come first.** A bracket that closes later reads the `]`
// written first as an ordinary member here exactly as it does everywhere
// else, so these rows are what say the rule is keyed on the *bracket* and not
// on the `]`: an implementation that closed on the first `]` would get all
// four wrong.
func TestABracketThatClosesLaterIsAnOrdinarySetHere(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a one-member set", `print -r -- []]`, "]\n"},
		{"a two-member set", `print -r -- []a]`, "] a\n"},
		{"a four-member set", `print -r -- []a~b]`, "] a ~\n"},
		{"a negated set holding it", `print -r -- [!]]`, "a z ~\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, emptyBracketDir(t), c.src+"\n")
			if out != c.want {
				t.Errorf("stdout = %q, want %q (stderr %q)", out, c.want, errs)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// And the rows that part from the panel: a bracket the member reading cannot
// close is the empty set here rather than an unterminated one.
//
// `[]` holds nothing, so it matches nothing and the word is a **miss** — `no
// matches found`, not `bad pattern`, which is the distinction #4630 settled
// and this one inherits. `[!]` is its negation and matches any one character,
// so it lists every one-character name in the directory.
func TestAnEmptyBracketCompilesAndMatchesNothing(t *testing.T) {
	for _, c := range []struct {
		name, src, want, err string
		status               int
	}{
		{"the empty set misses", `print -r -- []`, "", "no matches found: []", 1},
		{"with a literal after it", `print -r -- []a`, "", "no matches found: []a", 1},
		{
			"the negated empty set matches any one character",
			`print -r -- [!]`, "] a z ~\n", "", 0,
		},
		{"a caret negates it too", `print -r -- [^]`, "] a z ~\n", "", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, emptyBracketDir(t), c.src+"\n")
			if out != c.want {
				t.Errorf("stdout = %q, want %q (stderr %q)", out, c.want, errs)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
			if c.err != "" && !strings.Contains(errs, c.err) {
				t.Errorf("stderr = %q, want %q in it", errs, c.err)
			}
			if c.err == "" && errs != "" {
				t.Errorf("stderr = %q, want nothing", errs)
			}
		})
	}
}

// The same reading on the surfaces that are not filename generation, so the
// verdict is the pattern's rather than the walk's.
func TestAnEmptyBracketOnTheOtherSurfaces(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// The empty set matches nothing, so the prefix is not trimmed —
			// where a shell reading `[]` as two literal characters would
			// take them.
			"a prefix trim", `w="[]abc"; print -r -- ${w#[]}`, "[]abc\n",
		},
		{
			// And the negated empty set takes exactly one character.
			"a prefix trim that matches", `w="zabc"; print -r -- ${w#[!]}`, "abc\n",
		},
		{
			"a case arm",
			`case z in ([]) print E;; ([!]) print N;; (*) print O;; esac`, "N\n",
		},
		{
			"a condition operand",
			`[[ z == [!] ]] && print YES || print NO`, "YES\n",
		},
		{
			// And a group holding one: the arm that matches still matches,
			// which is what a walk that could not close `[]` used to cost.
			"an arm beside an empty set",
			`w="ab"; print -r -- ${w#([]|a)}`, "b\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, emptyBracketDir(t), c.src+"\n")
			if out != c.want {
				t.Errorf("stdout = %q, want %q (stderr %q)", out, c.want, errs)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0 (stderr %q)", st, errs)
			}
		})
	}
}

// The field that is exactly `[` is still not this question, in either
// direction — it is what keeps `[ a = a ]` running the test builtin, and it
// is the row that says the empty bracket did not widen the refusal.
func TestABareBracketIsStillNotAPatternHere(t *testing.T) {
	out, st, errs := runZshSplit(t, emptyBracketDir(t), "print -r -- [\n")
	if out != "[\n" || st != 0 {
		t.Errorf("got %q status %d, want the bare bracket at 0 (stderr %q)", out, st, errs)
	}
}
