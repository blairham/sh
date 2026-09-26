// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// MAGIC_EQUAL_SUBST: with it on, the first unquoted `=` in a command argument
// splits the word and everything after it gets the tilde treatment an
// assignment's value already gets, so `configure --prefix=~/opt` reaches the
// program as a path. Off — zsh's default and this dialect's — the two
// characters go through as written.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh`, under `-f -c` with `HOME=/Users/testhome`,
// 2026-09-25. `go version -m` on that path reports *not a Go executable* and
// on this shell's own binary reports `github.com/blairham/sh/cmd/zsh`, so the
// two are different programs. The option was `recorded(…)` here until #4548:
// accepted, reported back correctly by every listing, and acted on by nothing.
//
// The whole table below was run in both states of the option in the reference
// and every row is that binary's answer.
func TestMagicEqualSubstExpandsTheValueOfAnyEqualsWord(t *testing.T) {
	for _, tc := range []struct{ name, word, on, off string }{
		// The issue's own reduction.
		{"a bare tilde", `a=~`, "a=/Users/testhome", "a=~"},
		{"a path", `a=~/x`, "a=/Users/testhome/x", "a=~/x"},
		// The rows a rule about the *shape* of an assignment cannot reach.
		// This is the common one in a real script.
		{"a long option", `--opt=~/x`, "--opt=/Users/testhome/x", "--opt=~/x"},
		{"a short option", `-o=~`, "-o=/Users/testhome", "-o=~"},
		{"a name starting with a digit", `1a=~`, "1a=/Users/testhome", "1a=~"},
		{"a dash in the name", `a-b=~`, "a-b=/Users/testhome", "a-b=~"},
		{"a dot in the name", `a.b=~`, "a.b=/Users/testhome", "a.b=~"},
		{"a slash in the name", `a/b=~`, "a/b=/Users/testhome", "a/b=~"},
		{"a colon in the name", `a:b=~`, "a:b=/Users/testhome", "a:b=~"},
		{"a tilde as the name", `~=~`, "~=/Users/testhome", "~=~"},
		// The shape's own rows still move, through the same `=`.
		{"an underscore name", `_a=~`, "_a=/Users/testhome", "_a=~"},
		{"a name with a digit in it", `A9=~`, "A9=/Users/testhome", "A9=~"},
		{"an append", `a+=~`, "a+=/Users/testhome", "a+=~"},
		// **The first `=`, not the last.** The pair that parts the two
		// readings: `a=b=~` is left alone under the option because the value
		// the first `=` opens is `b=~`, whose tilde heads nothing; put a
		// colon in front of that tilde and the same first `=` expands it.
		{"a second equals", `a=b=~`, "a=b=~", "a=b=~"},
		{"a colon past the second equals", `a=b=~:~`, "a=b=~:/Users/testhome", "a=b=~:~"},
		// The value is the assignment's value, colons and all.
		{"a colon segment", `PATH=a:~/b`, "PATH=a:/Users/testhome/b", "PATH=a:~/b"},
		{"two segments", `a=~:~`, "a=/Users/testhome:/Users/testhome", "a=~:~"},
		{"a colon opening the value", `a=:~`, "a=:/Users/testhome", "a=:~"},
		{"a colon closing it", `a=~:`, "a=/Users/testhome:", "a=~:"},
		{"a tilde in the middle", `a=x~`, "a=x~", "a=x~"},
		{"a doubled tilde", `a=~~`, "a=~~", "a=~~"},
		{"the directory stack", `a=~+`, "a=/Users/testhome/w", "a=~+"},
		// A colon-tilde in **front** of the `=` moves once the word has
		// qualified, and never on its own. The pair, not either row alone.
		{"a colon tilde before the equals", `a:~/b=~`, "a:/Users/testhome/b=/Users/testhome", "a:~/b=~"},
		{"the same word with nothing to qualify it", `a:~/b=c`, "a:~/b=c", "a:~/b=c"},
		// **The `=`'s own quoting decides.** Four rows that hold the word's
		// text still and move where the quotes fall: the name may be quoted
		// or produced by an expansion and the word still qualifies; the `=`
		// itself may not, and neither may the tilde.
		{"a quoted name", `'--opt'=~`, "--opt=/Users/testhome", "--opt=~"},
		{"a double quoted name", `"a"=~`, "a=/Users/testhome", "a=~"},
		{"an empty quoted head", `""a=~`, "a=/Users/testhome", "a=~"},
		{"a name from an expansion", `$e=~`, "--p=/Users/testhome", "--p=~"},
		{"a quoted equals", `a'='~`, "a=~", "a=~"},
		{"a quoted tilde", `a='~'`, "a=~", "a=~"},
		{"an escaped tilde", `a=\~`, "a=~", "a=~"},
		{"the whole word quoted", `'a=~'`, "a=~", "a=~"},
		// A quoted `=` is not the splitter and the next unquoted one is.
		{"a quoted equals then a plain one", `"a=b"c=~`, "a=bc=/Users/testhome", "a=bc=~"},
		{"the same the other way round", `a"=b"c=~`, "a=bc=/Users/testhome", "a=bc=~"},
		// A word with no `=` at all is untouched in both states, which keeps
		// this from reading as "the option expands tildes".
		{"no equals at all", `a:~/b`, "a:~/b", "a:~/b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, st := range []struct {
				verb string
				want string
			}{{"setopt", tc.on}, {"unsetopt", tc.off}} {
				got := magicEqualRun(t, st.verb+" magicequalsubst\nprint -r -- "+tc.word)
				if got != st.want {
					t.Errorf("%s magicequalsubst; print -r -- %s = %q, want %q",
						st.verb, tc.word, got, st.want)
				}
			}
		})
	}
}

// The three rows the issue measured as already correct, which is what says
// this change reached the *argument* road and left the assignment road alone.
// All three agree with the reference in both states of the option, and they
// did before #4548 too.
func TestMagicEqualSubstLeavesTheAssignmentRoadsAlone(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a plain assignment", `v=~; print -r -- $v`},
		{"a typeset operand", `typeset w=~; print -r -- $w`},
		{"an export operand", `export e2=~; print -r -- $e2`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, verb := range []string{"setopt", "unsetopt"} {
				got := magicEqualRun(t, verb+" magicequalsubst\n"+tc.src)
				if got != "/Users/testhome" {
					t.Errorf("%s magicequalsubst; %s = %q, want %q",
						verb, tc.src, got, "/Users/testhome")
				}
			}
		})
	}
}

// The option is one state read in one place, so a subshell that moves it does
// not move the parent's — the guarantee setAxis carries and recorded(…) never
// had to. And the listings keep reporting what they reported before, since the
// state is now read off the axis rather than out of the recorded store.
func TestMagicEqualSubstIsOneStateReadInOnePlace(t *testing.T) {
	got := magicEqualRun(t, `
print -r -- before: $(print -r -- a=~)
(setopt magicequalsubst; print -r -- inside: a=~)
print -r -- after: $(print -r -- a=~)
setopt magicequalsubst
[[ -o magicequalsubst ]] && print -r -- cond: on || print -r -- cond: off
setopt | while IFS= read -r l; do case $l in magicequalsubst) print -r -- listed: $l;; esac; done
unsetopt magicequalsubst
[[ -o magicequalsubst ]] && print -r -- cond2: on || print -r -- cond2: off
`)
	wantWholeLines(t, got,
		"before: a=~",
		"inside: a=/Users/testhome",
		"after: a=~",
		"cond: on",
		"listed: magicequalsubst",
		"cond2: off",
	)
}

// magicEqualRun runs src with a home directory the rows above can name and a
// working directory for `~+`, and returns the output with its trailing
// newline removed.
//
// HOME is a variable of the runner rather than the process's, so the rows say
// what they mean on any machine; `e` holds `--p` for the one row whose name
// comes out of an expansion, chosen so that the name the expansion produces is
// one the assignment shape would refuse.
func magicEqualRun(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	out, _, err := preset.Combined(t, dialecttest.Base{
		Dir: dir + "/w",
		Vars: map[string]string{
			"PATH": dir,
			"HOME": "/Users/testhome",
			"PWD":  "/Users/testhome/w",
			"e":    "--p",
		},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return strings.TrimRight(out, "\n")
}
