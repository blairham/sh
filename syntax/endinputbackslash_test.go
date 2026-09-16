// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// A backslash the input ends immediately after is a word, not a refusal.
//
// This shell refused it — `input ends after a backslash`, status 2, which in
// a script is fatal and discards everything after the line. No shell in the
// panel refuses: bash 5.3, bash as `sh`, bash 3.2, dash, ksh93u+, zsh 5.9.2,
// zsh as `sh` and BusyBox ash all run the line at status 0. That half is a
// correction with no axis in it (#2680).
//
// What the backslash *becomes* is the axis, and the three readings are
// [EndOfInputBackslash]'s. Measured 2026-09-13 over `-c` and over a script
// file with no trailing newline, which answer alike.
//
// The assertions are on the spans rather than on whether the line parses,
// because every reading parses. A test that asked only for a nil error would
// pass all three and kill nothing.
func TestABackslashTheInputEndsAfterIsAWord(t *testing.T) {
	t.Parallel()
	literal := Core()
	dropped := Core()
	dropped.BackslashAtEndOfInput = EndOfInputBackslashIsDropped
	atStart := Core()
	atStart.BackslashAtEndOfInput = EndOfInputBackslashIsLiteralOnlyAtAWordStart

	for _, c := range []struct {
		name                              string
		src                               string
		literal, dropped, onlyAtWordStart string
	}{
		{
			// The backslash inside a word. bash 5.3, dash and ash keep it
			// and print `[x\]`; bash 3.2, ksh93 and zsh print `[x]`.
			"inside a word",
			`printf x\`,
			`word(printf) word(x|\\)`, `word(printf) word(x|\)`, `word(printf) word(x|\)`,
		},
		{
			// The backslash as the whole word, and the row ksh93 parts from
			// the shells it agreed with above: `printf "[%s]" \` is `[\]`
			// there and `[]` in zsh. Without this row a reading of ksh93 as
			// "drop it, like zsh" scores the same as the truth.
			"the whole word",
			`printf a \`,
			`word(printf) word(a) word(\\)`, `word(printf) word(a) word(\)`, `word(printf) word(a) word(\\)`,
		},
		{
			// Having read *anything* is what ends the start, and not the
			// character in front of the backslash: ksh93 drops it after a
			// quoted run as readily as after a bare one — `printf "[%s]"
			// "x"\` is `[x]` there. So the flag is the word's emptiness.
			"after a quoted run",
			`printf "x"\`,
			`word(printf) word("x"|\\)`, `word(printf) word("x"|\)`, `word(printf) word("x"|\)`,
		},
		{
			// An escaped backslash is read first, so the trailing one is no
			// longer at a start — `printf "[%s]" \\\` is `[x\]`-shaped in
			// ksh93 and keeps both in bash 5.3.
			"after an escaped backslash",
			`printf \\\`,
			`word(printf) word(\\|\\)`, `word(printf) word(\\|\)`, `word(printf) word(\\|\)`,
		},
	} {
		for _, d := range []struct {
			name string
			d    Dialect
			want string
		}{
			{"literal", literal, c.literal},
			{"dropped", dropped, c.dropped},
			{"only-at-a-word-start", atStart, c.onlyAtWordStart},
		} {
			l := NewLexer(c.src, d.d)
			got := render(l.Tokens())
			if l.Err() != nil {
				t.Errorf("%s/%s: %v", c.name, d.name, l.Err())
				continue
			}
			if l.Incomplete() {
				t.Errorf("%s/%s: reported incomplete; the panel runs it", c.name, d.name)
			}
			if got != d.want {
				t.Errorf("%s/%s: got %s, want %s", c.name, d.name, got, d.want)
			}
		}
	}
}

// The word survives the reading that drops the backslash, which is the
// difference between zsh and bash 3.2 and the reason the dropping reading
// still produces a span.
//
// `printf "[%s]" a \` is `[a][]` in zsh 5.9.2 and `[a]` in bash 3.2: zsh keeps
// an empty field where 3.2 loses the word with the backslash. The format is
// reused rather than doubled on purpose — `printf "[%s][%s]" a \` prints
// `[a][]` in both, because two conversions fill a missing operand with the
// empty string and cannot tell an empty word from an absent one. No dialect
// preset is bash 3.2, so the fourth reading is recorded in the corpus and not
// given a value — but an implementation that returned *no* span would be bash
// 3.2's, silently, and this is what says so.
func TestTheDroppedBackslashStillLeavesAWord(t *testing.T) {
	t.Parallel()
	d := Core()
	d.BackslashAtEndOfInput = EndOfInputBackslashIsDropped
	toks := NewLexer(`printf a \`, d).Tokens()
	words := 0
	for _, tk := range toks {
		if tk.Kind == TokWord {
			words++
		}
	}
	if words != 3 {
		t.Fatalf("got %d words, want 3 — the empty one is a field in zsh", words)
	}
	last := toks[2]
	if lit := last.Literal(); lit != "" {
		t.Errorf("last word is %q, want empty", lit)
	}
	if !last.IsQuoted() {
		t.Error("the empty word has to report as quoted, or nothing keeps it a field")
	}
}

// The nesting the older substitution exists for, which is where the refusal
// was found.
//
// “ `echo \\` “ unescapes to `echo \` — a backslash the *inner* input ends
// after — so the inner parse met the same end of input and the refusal came
// back out as the whole line's. The construct in the issue is one layer
// further: “ `echo \\`echo n\\“ “ is that substitution, then the literal
// text, then an empty one.
//
// The `$( )` spelling is the control. It needs no escaping to nest and its
// body is taken whole rather than unescaped, so a change to the older form's
// unescaping must not reach it — `"$(echo \`+"`"+`echo n\`+"`"+`)"` keeps
// both backslashes in all eight columns.
func TestTheOlderSubstitutionNestsThroughItsOwnEscaping(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"a body that ends in an escaped backslash",
			"echo `echo \\\\`",
			`word(echo) word(cmd{echo \})`,
		},
		{
			"the same inside double quotes",
			"echo \"`echo \\\\`\"",
			`word(echo) word("cmd{echo \})`,
		},
		{
			"an escaped backquote still opens a nested body",
			"echo `echo \\`echo n\\``",
			"word(echo) word(cmd{echo `echo n`})",
		},
		{
			// The control: `$( )` hands its body on unchanged, so the
			// backslashes are still there for the inner parse to read.
			"the newer spelling unescapes nothing",
			"echo \"$(echo \\`echo n\\`)\"",
			"word(echo) word(\"cmd{echo \\`echo n\\`})",
		},
	} {
		l := NewLexer(c.src, Core())
		got := render(l.Tokens())
		if l.Err() != nil {
			t.Errorf("%s: %v", c.name, l.Err())
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

// An unterminated pattern group says so, and used to say something else.
//
// The group scan reached the same end of input and reported the backslash,
// which got in front of the refusal that was actually due: the group has no
// `)`. Nothing in the panel names a backslash here — zsh 5.9.2 answers `bad
// pattern: @(a` and ksh93u+ runs the word. The two scans now read one rule
// from one place, which is what keeps this from drifting back.
func TestAnUnterminatedGroupIsNotBlamedOnItsBackslash(t *testing.T) {
	t.Parallel()
	d := Core()
	d.ExtendedPattern = true
	l := NewLexer(`echo @(a\`, d)
	l.Tokens()
	if l.Err() == nil {
		t.Fatal("want a refusal: the group has no closing parenthesis")
	}
	if got := l.Err().Error(); !strings.Contains(got, "unterminated pattern group") {
		t.Errorf("refused with %q, want the group named", got)
	}
}
