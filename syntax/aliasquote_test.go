// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A quote an alias body opens, and one it leaves open (#2685).
//
// Alias substitution replaces the alias word with the alias *text* in the
// input the shell is reading, and lexing carries on over the join. Both
// halves of that were wrong here and in opposite directions, because the
// body was lexed on its own and its lexer started and ended at the body's
// edges — so a quote could not enter it and could not leave it.
//
//	written                              seven columns   here, before
//	alias q='echo "'  then  q hello"     ` hello`, 0     refused, 2
//	alias a='echo "x' then  a            refused         `x`, 0
//
// Unanimous in dash, bash 5.3, that binary as `sh`, bash 3.2, ksh93, zsh and
// BusyBox ash, both ways round.

// The construct the body opens reaches the rest of the line, whichever
// construct it is. It is a property of the lexer crossing the seam rather
// than of quoting, so the whole family is here: the two quotes, both
// spellings of a command substitution, and `$'`.
func TestAConstructAnAliasBodyOpensReachesTheRestOfTheLine(t *testing.T) {
	for _, c := range []struct {
		name  string
		alias syntax.Aliases
		src   string
		want  string
	}{
		{
			"a double quote",
			table("q", `echo "`), `q hello"`, `echo " hello"`,
		},
		{
			"a single quote",
			table("q", "echo '"), "q hello'", "echo ' hello'",
		},
		{
			// The body's last token is a word that had already begun, so
			// the join is inside a word rather than at its start: `a"b` and
			// ` c"` are one word and not two.
			"a quote opened part way through a word",
			table("q", `echo a"b`), `q c"`, `echo a"b c"`,
		},
		{
			"a command substitution",
			table("q", "echo $("), "q echo hi)", "echo $( echo hi)",
		},
		{
			"the older spelling of one",
			table("q", "echo `"), "q echo hi`", "echo ` echo hi`",
		},
		{
			// An arithmetic substitution, which is a third scanner again —
			// its closer is two characters and its body is an expression
			// rather than a program.
			"an arithmetic substitution",
			table("q", "echo $(("), "q 1+1 ))", "echo $(( 1+1 ))",
		},
		{
			"dollar-single quotes",
			table("q", `echo $'`), `q a'`, `echo $' a'`,
		},
		{
			// The seam is crossed once and the input goes on being the
			// input: the words after the carried one are read where they
			// really stand.
			"the words after the carried one are still read",
			table("q", `echo "`), `q one" two three`, `echo " one" two three`,
		},
		{
			// A quote may span lines, and the one that closes it may be on
			// any later line of the file. Nothing about the seam stops at
			// the end of the alias word's line.
			"a quote closed on a later line",
			table("q", `echo "`), "q one\ntwo\"\necho after", "echo \" one\ntwo\"\necho after",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := parsed(t, c.alias, c.src); got != c.want {
				t.Errorf("%q came to %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// The control, and it is the one that says the carry is about a construct
// that is still *open*: a body whose quoting closes inside it takes nothing
// from the input, and the word after the alias word is a word of its own.
func TestABodyThatClosesItsOwnQuotesTakesNothingFromTheInput(t *testing.T) {
	for _, c := range []struct {
		name  string
		alias syntax.Aliases
		src   string
		want  string
	}{
		{"a closed double quote", table("q", `echo "a"`), `q b`, `echo "a" b`},
		{"a closed single quote", table("q", "echo 'a'"), "q b", "echo 'a' b"},
		{"a closed substitution", table("q", "echo $(echo a)"), "q b", "echo $(echo a) b"},
		{"no quoting at all", table("q", "echo a"), "q b", "echo a b"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := parsed(t, c.alias, c.src); got != c.want {
				t.Errorf("%q came to %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// The second half, and the worse one: a construct nothing closes is an
// unterminated construct rather than a silence. Running it meant closing a
// quote the script never closed and running a command the author did not
// write — `alias a='echo "x'` then `a` printed `x` here and is a refusal in
// every column of the panel.
func TestAConstructNothingClosesIsUnterminated(t *testing.T) {
	for _, c := range []struct {
		name  string
		alias syntax.Aliases
		src   string
		open  string
	}{
		{"a double quote", table("a", `echo "x`), "a", `"`},
		{"a single quote", table("a", "echo 'x"), "a", "'"},
		{"a command substitution", table("a", "echo $(echo x"), "a", "$("},
		{
			// The rest of the input is swallowed by the construct rather
			// than run, which is what every column does: `echo two` here is
			// inside the quote.
			"the rest of the input is inside it",
			table("a", `echo "x`), "a\necho two\n", `"`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := syntax.NewParser(c.src, syntax.Core())
			p.Aliases = c.alias
			p.Parse()
			if p.Err() == nil {
				t.Fatalf("%q with %q parsed, want an unterminated construct", c.src, c.name)
			}
			if !p.Incomplete() {
				t.Errorf("%q: not reported incomplete, so a prompt would refuse it rather than ask for more", c.src)
			}
			got := p.Open()
			if len(got) == 0 || got[len(got)-1].Word != c.open {
				t.Errorf("%q: open %v, want the innermost to be %q", c.src, got, c.open)
			}
		})
	}
}

// Where the refusal is reported, which is the pair of lines every dialect
// words its diagnostic from: the opener is named where the alias *word*
// stood, because the body is text the program never held, and the end is
// where the input really ran out.
//
// Measured on a five-line file whose alias word is on line 4: bash 5.3, that
// binary as `sh`, bash 3.2 and ksh93 all blame line 4, and zsh and dash both
// blame line 6 — the line after the last. Both numbers come from here.
func TestTheUnterminatedBodyIsBlamedAtTheAliasWordAndAtTheEnd(t *testing.T) {
	const src = "echo zero\necho one\nalias a='echo \"x'\na\necho two\n"
	p := syntax.NewParser(src, syntax.Core())
	p.Aliases = table("a", `echo "x`)
	p.Parse()
	var se *syntax.Error
	if !errors.As(p.Err(), &se) {
		t.Fatalf("err %v, want a syntax.Error", p.Err())
	}
	if se.Pos.Line != 4 {
		t.Errorf("opener at line %d, want 4 — the line the alias word is on", se.Pos.Line)
	}
	if se.EofLine != 6 {
		t.Errorf("EofLine %d, want 6 — the line the input ran out on", se.EofLine)
	}
}

// The input goes on being the input after the seam: what the joined reading
// took is skipped in the input's own lexer, lines and columns and all, so
// everything read afterwards is at its real position.
//
// Asked with a refusal three lines past the carry, because a position is only
// checkable where something names it. A carry that consumed the right number
// of *bytes* and lost the newlines inside them passes every test above and
// fails this one.
func TestAfterTheSeamTheInputIsReadAtItsRealPosition(t *testing.T) {
	const src = "alias q='echo \"'\nq one\ntwo\"\nthree\n)\n"
	p := syntax.NewParser(src, syntax.Core())
	p.Aliases = table("q", `echo "`)
	p.Parse()
	var se *syntax.Error
	if !errors.As(p.Err(), &se) {
		t.Fatalf("err %v, want the stray parenthesis refused", p.Err())
	}
	if se.Pos.Line != 5 {
		t.Errorf("blamed line %d, want 5 — the line the `)` is written on", se.Pos.Line)
	}
	if se.Pos.Col != 1 {
		t.Errorf("blamed column %d, want 1", se.Pos.Col)
	}
}

// A blank the body ends with is inside the construct the body opened, so it
// is not the trailing blank that makes the next word eligible for expansion
// in turn. `alias q='echo "x '` used as `q b"` is `x  b` in all seven
// columns and never the expansion of `b`.
func TestABlankInsideAnOpenConstructMakesNoNextWordEligible(t *testing.T) {
	got := parsed(t, table("q", `echo "x `, "b", "BEE"), `q b"`)
	if want := `echo "x  b"`; got != want {
		t.Errorf("came to %q, want %q", got, want)
	}
	// The control on it: the same trailing blank outside a quote does make
	// the next word eligible, which is the rule behind `alias sudo='sudo '`.
	got = parsed(t, table("q", `echo "x" `, "b", "BEE"), `q b`)
	if want := `echo "x" BEE`; got != want {
		t.Errorf("with the quote closed, came to %q, want %q", got, want)
	}
}

// The one question inside the seam the panel does not agree on: whether the
// word *past* the construct is offered to the table in turn.
//
// bash offers it — `alias c='CEE'; alias q='echo "x '` used as `q b" c` is
// `x  b CEE` in bash 5.3, that binary as `sh`, and bash 3.2 — and dash,
// ksh93, zsh and BusyBox ash offer nothing, because what follows the value is
// inside the quote. The core refuses what the panel disagrees about, so the
// field is off there; dialect/aliasquote_test.go says which preset holds
// which value.
func TestTheAxisDecidesWhetherTheBlankReachesPastTheConstruct(t *testing.T) {
	for _, c := range []struct {
		on   bool
		want string
	}{
		{false, `echo "x  b" c`},
		{true, `echo "x  b" CEE`},
	} {
		d := syntax.Core()
		d.AliasTrailingBlankReachesPastAnOpenConstruct = c.on
		p := syntax.NewParser(`q b" c`, d)
		p.Aliases = table("q", `echo "x `, "c", "CEE")
		f := p.Parse()
		if err := p.Err(); err != nil {
			t.Fatalf("on=%v: did not parse: %v", c.on, err)
		}
		if got := strings.TrimSpace(syntax.Print(f)); got != c.want {
			t.Errorf("on=%v: came to %q, want %q", c.on, got, c.want)
		}
	}
}

// The seam is crossed only where the text after the alias word is the
// *input's*. A body expanded from inside another body's expansion has that
// body's remaining tokens in front of it, and a token keeps no text to be
// read again — so the carry is declined there rather than reading past
// tokens that stand between. See #2709, which has the panel rows for the
// nested arrangement.
func TestTheCarryIsDeclinedWhereTheRestIsAnotherBodysTokens(t *testing.T) {
	got := parsed(t, table("a", "b x", "b", `echo "`), "a\necho after")
	if !strings.Contains(got, "echo after") {
		t.Errorf("came to %q: the input after the expansion was swallowed, so the pending token was read as if it were the input", got)
	}
}
