// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// An alias named `{` and the text written **touching** it.
//
// `{` at command position is a word by itself whatever follows it, so `{end`
// is the reserved word and then `end`. That makes it the one token the lexer
// finishes by a rule of its own rather than at a word boundary: every other
// word stops at a character that could not have continued it. Put an alias
// value in its place and the value touches the text after it — `alias \{=echo`
// with `{end` read as the single word `echoend`, which is the third root of
// #5139's "Aliasing reserved tokens" chunk.
//
// The reference separates the two with a **blank**, and with one only where
// none already stands. Measured 2026-09-30 against zsh 5.9.2 from
// /opt/homebrew/bin/zsh, each row `-fis` with the lines fed on stdin under
// `unsetopt PROMPT_SP; PROMPT=""; exec 2>&1`, and the value `echo` unless the
// row names another:
//
//	{end                              end
//	{x                                x
//	{"x"         value `print A`      A x
//	{$HOME       value `print A`      A /Users/bhamilton
//	{}x                               }x
//	{ x"         value `echo "`        x
//	{  x"        value `echo "`        x            the count is kept
//	{x"          value `echo "`        x            and supplied
//	{x           value `echo \`        x            a real blank: the
//	                                                 backslash escapes it
//	alias x='print XX'; {x            x             not `XX`
//	alias x='print XX'; {x  value `echo `   print XX
//
// **The last two rows are the pair that names what the blank is.** A value
// that ends in a blank expands the next word as well, which is an old and
// separate rule; this separator is not the value's own and does not. Both
// rows hold everything fixed but where the blank came from, which is the
// only way to tell the two apart — a grid that varied the following text
// instead would agree with either reading.
//
// A real blank rather than a bare word boundary is what the `echo \` row
// decides: a boundary would leave the backslash with nothing to escape.
func TestAnAliasNamedForTheOpenBraceIsSeparatedFromWhatTouchesIt(t *testing.T) {
	t.Parallel()
	d := zsh.Dialect()
	parse := func(t *testing.T, src string, pairs ...string) string {
		t.Helper()
		m := map[string]string{}
		for i := 0; i+1 < len(pairs); i += 2 {
			m[pairs[i]] = pairs[i+1]
		}
		p := syntax.NewParser(src, d)
		p.Aliases = func(name string) (string, bool) {
			v, ok := m[name]
			return v, ok
		}
		f := p.Parse()
		if err := p.Err(); err != nil {
			return "ERR: " + err.Error()
		}
		return syntax.Print(f)
	}
	for _, c := range []struct {
		name, src string
		aliases   []string
		want      string
	}{
		{
			// The chunk's own line. Two words, where the bug read one.
			"the text touching the brace", `{end`,
			[]string{"{", "echo"},
			`echo end`,
		},
		{
			// Nothing about the word `end`: any word at all.
			"any touching word", `{x`,
			[]string{"{", "echo"},
			`echo x`,
		},
		{
			// A quote is not a blank, so the separator goes in here too and
			// the value does not become the word `print A"x"`.
			"a quote touching the brace", `{"x"`,
			[]string{"{", "print A"},
			`print A "x"`,
		},
		{
			"a parameter touching the brace", `{$HOME`,
			[]string{"{", "print A"},
			`print A $HOME`,
		},
		{
			// `}` in a word is text, so the argument is the whole of `}x`.
			// This is also why `{}` on its own stays a parse error: it
			// becomes `echo }`, and a bare `}` is reserved in this dialect.
			"a close brace inside the touching word", `{}x`,
			[]string{"{", "echo"},
			`echo }x`,
		},
		{
			// The blank is a character, and the backslash escapes it. A
			// word boundary would have nothing here to escape.
			"a value ending in a backslash", `{x`,
			[]string{"{", `echo \`},
			`echo \ x`,
		},
		{
			// A construct the value leaves open still crosses the seam, and
			// now takes the separator with it.
			"a value leaving a quote open", `{x"`,
			[]string{"{", `echo "`},
			`echo " x"`,
		},
		{
			// Where a blank already stands, none is added: the blank the
			// open quote swallows is the one that was written.
			"a blank already standing after the brace", `{ x"`,
			[]string{"{", `echo "`},
			`echo " x"`,
		},
		{
			// And the count of them is kept rather than normalized.
			"two blanks already standing", `{  x"`,
			[]string{"{", `echo "`},
			`echo "  x"`,
		},
		{
			// The separator is not the value's own, so the word after it is
			// not itself expanded.
			"the touching word is not expanded", `{x`,
			[]string{"{", "echo", "x", "print XX"},
			`echo x`,
		},
		{
			// Where the blank *is* the value's, it is — the pair above.
			"a value of its own ending in a blank", `{x`,
			[]string{"{", "echo ", "x", "print XX"},
			`echo print XX`,
		},
		{
			// An ordinary word alias ends at a real word boundary, so
			// nothing is inserted and nothing changes.
			"an ordinary word alias", `q x`,
			[]string{"q", "echo", "x", "print XX"},
			`echo x`,
		},
		{
			// And the brace with no alias at all is still the reserved word.
			"the group with no alias", `{ print IN }`,
			nil, `{ print IN; }`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := parse(t, c.src, c.aliases...); got != c.want {
				t.Errorf("%q parsed to %q, want %q", c.src, got, c.want)
			}
		})
	}
}
