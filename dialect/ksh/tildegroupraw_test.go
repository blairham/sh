// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// A `~(…)` group turns the rest of its word into the flavor's own text.
//
// `~(E)` ships and is honored, and two of the three metacharacters that make
// an extended regular expression *extended* were a syntax error here: the `(`
// of a group and the `|` of an alternation are the shell's characters too, and
// reading the `~(…)` prefix did not stop them ending the word.
//
// The rule is not "a `[[ ]]` operand reads parentheses as pattern characters".
// ksh93 refuses them **bare** exactly as this shell does — `[[ abcd == (ab)cd ]]`
// is “syntax error at line 1: `(' unexpected“ in both. What the group does is
// turn what follows it raw.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-19, each probe its own
// `env -i PATH=/usr/bin:/bin LC_ALL=C /bin/ksh -c …` with stdin on
// `/dev/null` (#3808).
func TestATildeGroupTurnsTheRestOfTheWordRaw(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		// Each row is a **pair**: the subject that matches and the subject
		// that does not. A probe that asked only "does the line come back 0"
		// cannot tell "the character is the pattern's" from "the character
		// was the shell's and the list happened to answer 0", and for `&`
		// and `;` it answers 0 either way.
		{
			name: "a group",
			src: `[[ abcd == ~(E)(ab)cd ]] && print -r -- Y || print -r -- N
[[ zzzz == ~(E)(ab)cd ]] && print -r -- Y || print -r -- N`,
			want: "Y\nN\n",
		},
		{
			name: "an alternation",
			src: `[[ zzz == ~(E)ab|zz ]] && print -r -- Y || print -r -- N
[[ qqq == ~(E)ab|zz ]] && print -r -- Y || print -r -- N`,
			want: "Y\nN\n",
		},
		{
			// The `&` row is the one that needs the pair most: were the `&`
			// the shell's, the line would be `[[ … ] ] &` then `b`, which is
			// 0 whatever the subject is. The second row coming back 1 — with
			// nothing on stderr — is what says the character reached the
			// pattern.
			name: "an ampersand",
			src: `[[ "a&b" == ~(E)a&b ]] && print -r -- Y || print -r -- N
[[ zzz == ~(E)a&b ]] && print -r -- Y || print -r -- N`,
			want: "Y\nN\n",
		},
		{
			// And the `;`, where a shell reading would have left `b` to be
			// looked up as a command at 127.
			name: "a semicolon",
			src: `[[ "a;b" == ~(E)a;b ]] && print -r -- Y || print -r -- N
[[ zzz == ~(E)a;b ]] && print -r -- Y || print -r -- N`,
			want: "Y\nN\n",
		},
		{
			name: "the redirection operators",
			src: `[[ "a<b" == ~(E)a<b ]] && print -r -- Y || print -r -- N
[[ "a>b" == ~(E)a>b ]] && print -r -- Y || print -r -- N
[[ zzz == ~(E)a>b ]] && print -r -- Y || print -r -- N`,
			want: "Y\nY\nN\n",
		},
		{
			// Nesting, so it is not a single-level scan.
			name: "a nested group",
			src: `[[ abcd == ~(E)(a(b))cd ]] && print -r -- Y || print -r -- N
[[ zzzz == ~(E)(a(b))cd ]] && print -r -- Y || print -r -- N`,
			want: "Y\nN\n",
		},
		{
			name: "several letters, then the text",
			src: `[[ abcd == ~(Ei)(AB)CD ]] && print -r -- Y || print -r -- N
[[ zzzz == ~(Ei)(AB)CD ]] && print -r -- Y || print -r -- N`,
			want: "Y\nN\n",
		},
		{
			// Mid-word. This row is about the **parse**: ksh93 answers 1 for
			// the first line and 0 for the second, and this shell answers 1
			// for both because a `~(…)` group that is not at the head of the
			// word reaches no modifier in the matcher — `[[ xabc == x~(E)a.c ]]`
			// is `N` here and `Y` there, on `main` as much as on this branch.
			// What changed is that neither line is a syntax error any more.
			name: "the group mid-word parses",
			src: `[[ abcd == x~(E)(ab)cd ]] && print -r -- Y || print -r -- N
[[ xabcd == x~(E)(ab)cd ]] && print -r -- Y || print -r -- N`,
			want: "N\nN\n",
		},
		{
			// A closing paren with nothing to close is text inside `[[ ]]`.
			// Written with the literal flavor because an unbalanced `)` is
			// not a regular expression RE2 will compile — which is a matcher
			// question and not this one.
			name: "an unmatched closer inside a condition",
			src: `[[ "a)b" == ~(F)a)b ]] && print -r -- Y || print -r -- N
[[ zzz == ~(F)a)b ]] && print -r -- Y || print -r -- N`,
			want: "Y\nN\n",
		},
		{
			// Outside a condition entirely: an ordinary word keeps the whole
			// of it as literal text, which is what the prefix is for.
			name: "an ordinary word",
			src:  `print -r -- ~(E)(ab)cd`,
			want: "~(E)(ab)cd\n",
		},
		{
			// A `case` arm, where the arm's own `)` still closes it: the
			// balanced group is taken whole and the unbalanced paren after it
			// is the grammar's.
			name: "a case arm",
			src: `case abcd in ~(E)(ab)cd) print -r -- Y;; *) print -r -- N;; esac
case zzzz in ~(E)(ab)cd) print -r -- Y;; *) print -r -- N;; esac`,
			want: "Y\nN\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// The controls, and they are what say the rule is the **group's** rather than
// the operand's. Every one of these answers the same in ksh93u+, measured in
// the same run, and every one of them answered this way before the raw text
// was read too.
func TestWithoutATildeGroupTheOperatorsAreStillTheShellS(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// A pattern group the grammar already knows, which is why this
			// one works and the bare one does not.
			name: "an @() group is unaffected",
			src: `[[ abcd == @(ab)cd ]] && print -r -- Y || print -r -- N
[[ zzzz == @(ab)cd ]] && print -r -- Y || print -r -- N`,
			want: "Y\nN\n",
		},
		{
			// The two characters no shell grammar wants, which is why
			// `a.c` and `a+b` working said nothing about the construct.
			name: "the characters only a regular expression means anything by",
			src: `[[ abcd == ~(E)a.c ]] && print -r -- Y || print -r -- N
[[ aab == ~(E)a+b ]] && print -r -- Y || print -r -- N`,
			want: "Y\nY\n",
		},

		{
			// And quoting and expansion are untouched.
			name: "quoting and expansion still happen",
			src: `[[ "a b" == ~(E)"a b" ]] && print -r -- Y || print -r -- N
v=b
[[ abc == ~(E)a${v}c ]] && print -r -- Y || print -r -- N
[[ abc == ~(E)a$(print -r -- b)c ]] && print -r -- Y || print -r -- N`,
			want: "Y\nY\nY\n",
		},
		{
			// An ordinary word with a `~` and no group is a `~` and no
			// group, and the `;` after it is still the shell's.
			name: "a bare tilde arms nothing",
			src:  `print -r -- a~b;print -r -- after`,
			want: "a~b\nafter\n",
		},
		{
			// And the state does not survive the word it was set in: the
			// `;` here is the shell's, one word after the group.
			name: "the next word is the shell's again",
			src:  `[[ abcd == ~(E)(ab)cd ]] && print -r -- Y;print -r -- after`,
			want: "Y\nafter\n",
		},
		{
			// An ordinary word's `;` is the shell's even **inside** the raw
			// reach, which is the boundary this rule is drawn at: the other
			// operators stop separating in a pattern comparison's right
			// operand and nowhere else.
			name: "an ordinary word after a group still ends at a semicolon",
			src:  `print -r -- ~(E)a;print -r -- after`,
			want: "~(E)a\nafter\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// And the three shapes that are a **refusal** in ksh93 and stay one here. They
// are asked of the parser rather than of the runner, because a run of a file
// that does not parse has nothing to compare.
//
// The first two are the rows that correct this issue's first framing: it is
// not that a `[[ ]]` operand reads parentheses and `|` as pattern characters.
// ksh93 refuses them bare in exactly the way this shell does, and what the
// `~(…)` group does is turn what follows it into the flavor's text.
func TestTheseStayRefusals(t *testing.T) {
	d := ksh.Dialect()
	for _, src := range []string{
		// `syntax error at line 1: `(' unexpected` in ksh93u+.
		`[[ abcd == (ab)cd ]]`,
		// `syntax error at line 1: `|' unexpected` there.
		`[[ ab == a|b ]]`,
		// An unquoted blank still ends the word, group or no group:
		// `syntax error at line 1: `cd' unexpected` there.
		`[[ abcd == ~(E)(ab) cd ]]`,
		// A `;` in a `case` arm is the shell's even after a group:
		// `syntax error at line 1: `;' unexpected` there.
		`case "a;b" in ~(E)a;b) print -r -- Y;; esac`,
		// And in the *left* operand, and as a unary operand, which is what
		// puts the boundary at the right operand of a pattern comparison.
		`[[ ~(E)a;b == "a;b" ]]`,
		`[[ -n ~(E)a;b ]]`,
		// And the state does not outlive the word it was set in. A second
		// comparison in the same condition is a second word, and its bare
		// `(` is the shell's again — ``syntax error at line 1: `(' unexpected``
		// in ksh93u+, and the row that fails if the flag is never cleared.
		`[[ abcd == ~(E)(ab)cd && ab == a(b) ]]`,
		// The same across a command boundary, where the word is a value.
		`x=~(E)(ab)cd; y=a(b)`,
	} {
		if _, err := syntax.Parse(src, d.On(syntax.RouteFromScriptFile)); err == nil {
			t.Errorf("%q parsed; ksh93u+ refuses it", src)
		}
	}
}
