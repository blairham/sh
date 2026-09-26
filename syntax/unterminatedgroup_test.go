// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// A pattern group the input runs out of is a **word** where the grammar says
// so, and unfinished input where it does not.
//
// [Dialect.UnterminatedPatternGroupIsAWord] is the whole of the difference,
// and the panel splits on it — measured 2026-09-26 from a script file. zsh
// 5.9.2 answers `print -r -- a(b` with `bad pattern: a(b`, which is a
// *runtime* complaint naming the word; bash 5.3.20 with `extglob` on answers
// its own spelling with `unexpected EOF while looking for matching )` and
// ksh93u+ with “syntax error: `(' unmatched“, both while parsing.
//
// The distinction is not cosmetic. A word the lexer refuses never reaches
// filename generation, and filename generation is where one shell's
// `setopt badpattern` is read — so the refusal has to be the matcher's for
// the option to be able to withhold it (#4645).
//
// The assertion is on the **spans** and not on whether the error is nil,
// because "it parsed" is true of a reading that threw the group away. The
// word has to arrive with its `(` still in it, since that is what the
// matcher refuses. The `|` in the wanted text is render's span boundary and
// not a character of the word: the group is its own span, which is how the
// printer knows to write the parentheses back live rather than escaped.
//
// Every case here opens its group **mid-word**, because a `(` at the front of
// one is the parser's question rather than the lexer's — a leading one opens
// a subshell unless the parser has said a word may begin with it, and this
// test drives the lexer on its own. The word-initial spelling is measured
// where the parser is in the loop, in dialect/zsh.
func TestAnUnterminatedPatternGroupIsAWordWhereTheGrammarSaysSo(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		src  string
		want string
	}{
		{"a group opened mid-word", "print -r -- a(b", "word(a|(b)"},
		{"an alternation nothing closes", "print -r -- a(b|c", "word(a|(b|c)"},
		{
			// The `@` is an ordinary character in front of a bare group
			// here, so this is the same one word rather than a quantified
			// group. It is in the table because it is the spelling the two
			// shells on the other side of the flag *do* have.
			"a quantifier's spelling in front of a bare group",
			"print -r -- a@(b", "word(a@|(b)",
		},
		{
			// The control: a group that closes is read as it always was,
			// and the flag must not reach it.
			"a group that closes is unchanged",
			"print -r -- a(b|c)", "word(a|(b|c))",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			word := Core()
			word.PatternAlternation = true
			word.UnterminatedPatternGroupIsAWord = true
			l := NewLexer(c.src, word)
			got := render(l.Tokens())
			if l.Err() != nil {
				t.Fatalf("refused with %v, want a word", l.Err())
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("tokens = %s, want %s in them", got, c.want)
			}
		})
	}
}

// And the other side of the flag, which is the falsifying half: the same
// input under the same grammar with the flag off is still refused, by name.
//
// Without this the test above would pass against a lexer that had simply
// stopped refusing everywhere — which is what the first draft of the change
// was, and what the bash and ksh columns would have paid for.
func TestAnUnterminatedPatternGroupIsUnfinishedInputWithoutTheFlag(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src string
		d         Dialect
	}{
		{"a bare group", "print -r -- a(b", func() Dialect {
			d := Core()
			d.PatternAlternation = true
			return d
		}()},
		{"a quantified group", "echo a@(b", func() Dialect {
			d := Core()
			d.ExtendedPattern = true
			return d
		}()},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			l := NewLexer(c.src, c.d)
			l.Tokens()
			if l.Err() == nil {
				t.Fatal("want a refusal: the group has no closing parenthesis")
			}
			if got := l.Err().Error(); !strings.Contains(got, "unterminated pattern group") {
				t.Errorf("refused with %q, want the group named", got)
			}
		})
	}
}
