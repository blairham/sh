// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// What the editor writes for a line, with the runs applied.
//
// Whole strings including the escape bytes, because that is the whole claim: a
// test that stripped the sequences would pass for an editor that drew the line
// plainly, which is the failure this seam is most likely to have.
func TestARunIsDrawnWithItsStyleAndAReset(t *testing.T) {
	for _, c := range []struct {
		name string
		line string
		runs []Highlight
		want string
	}{
		{
			name: "one run in the middle",
			line: "echo one two",
			runs: []Highlight{{Start: 5, End: 8, Style: "\x1b[31m"}},
			want: "echo \x1b[31mone\x1b[0m two",
		},
		{
			name: "two runs, in the order they start",
			line: "echo one two",
			runs: []Highlight{{Start: 9, End: 12, Style: "\x1b[32m"}, {Start: 0, End: 4, Style: "\x1b[31m"}},
			want: "\x1b[31mecho\x1b[0m one \x1b[32mtwo\x1b[0m",
		},
		{
			name: "a run reaching the end of the line",
			line: `echo "open`,
			runs: []Highlight{{Start: 5, End: 10, Style: "\x1b[31m"}},
			want: "echo \x1b[31m\"open\x1b[0m",
		},
		{
			name: "a second run overlapping the first is dropped",
			line: "echo one two",
			runs: []Highlight{{Start: 0, End: 8, Style: "\x1b[31m"}, {Start: 5, End: 12, Style: "\x1b[32m"}},
			want: "\x1b[31mecho one\x1b[0m two",
		},
		{
			name: "a backwards run is dropped rather than a panic",
			line: "echo one",
			runs: []Highlight{{Start: 5, End: 2, Style: "\x1b[31m"}},
			want: "echo one",
		},
		{
			name: "a run off the end of the line is dropped",
			line: "echo one",
			runs: []Highlight{{Start: 5, End: 99, Style: "\x1b[31m"}},
			want: "echo one",
		},
		{
			name: "a run with no style is dropped",
			line: "echo one",
			runs: []Highlight{{Start: 0, End: 4}},
			want: "echo one",
		},
		{
			name: "a run over a character wider than one byte",
			line: "echo 日本",
			runs: []Highlight{{Start: 5, End: 11, Style: "\x1b[31m"}},
			want: "echo \x1b[31m日本\x1b[0m",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := &editor{
				line:        []rune(c.line),
				highlighter: HighlighterFunc(func(string) []Highlight { return c.runs }),
			}
			if got := e.styled(); got != c.want {
				t.Errorf("the editor writes\n got %q\nwant %q", got, c.want)
			}
		})
	}
}

// No highlighter, and a highlighter with nothing to say, both draw the line as
// it stands and copy nothing.
func TestALineWithoutRunsIsDrawnAsItStands(t *testing.T) {
	plain := &editor{line: []rune("echo one")}
	if got := plain.styled(); got != "echo one" {
		t.Errorf("with no highlighter the editor writes %q, want %q", got, "echo one")
	}
	quiet := &editor{
		line:        []rune("echo one"),
		highlighter: HighlighterFunc(func(string) []Highlight { return nil }),
	}
	if got := quiet.styled(); got != "echo one" {
		t.Errorf("with no runs the editor writes %q, want %q", got, "echo one")
	}
}

// A highlighter is asked about the line as it stands, in bytes.
func TestAHighlighterIsAskedAboutTheLine(t *testing.T) {
	var asked []string
	e := &editor{
		line: []rune("echo 日本"),
		highlighter: HighlighterFunc(func(line string) []Highlight {
			asked = append(asked, line)
			return nil
		}),
	}
	e.styled()
	if len(asked) != 1 || asked[0] != "echo 日本" {
		t.Errorf("the highlighter was asked %q, want one call with %q", asked, "echo 日本")
	}
}

// What UnclosedQuote colors: the word an open quotation has swallowed, from
// where the word begins to the end of the line.
func TestUnclosedQuoteColorsWhatTheQuotationSwallowed(t *testing.T) {
	u := UnclosedQuote{Style: "\x1b[31m"}
	for _, c := range []struct{ name, line, want string }{
		{"a closed line is not colored", `echo "one two"`, `echo "one two"`},
		{"an empty line is not colored", "", ""},
		{"a line with no quotes at all", "echo one two", "echo one two"},
		{"a double quote left open", `echo "one two`, "echo \x1b[31m\"one two\x1b[0m"},
		{"a single quote left open", `echo 'one two`, "echo \x1b[31m'one two\x1b[0m"},
		// The word rather than the line: what is before the quotation is not
		// inside it, and coloring it would say the shell is confused about
		// text it has already read.
		{"only the word carrying it", `cp a.txt "b c`, "cp a.txt \x1b[31m\"b c\x1b[0m"},
		// A quote closed and another opened is one open quotation.
		{"a second quotation left open", `echo "one" "two`, "echo \"one\" \x1b[31m\"two\x1b[0m"},
		// An escaped quote is a character, not a quotation.
		{"an escaped quote opens nothing", `echo \"one`, `echo \"one`},
		// Bytes and not runes, which a run computed in runes would get wrong.
		{"a quotation after a wide character", `echo 日本 "x`, "echo 日本 \x1b[31m\"x\x1b[0m"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := &editor{line: []rune(c.line), highlighter: u}
			if got := e.styled(); got != c.want {
				t.Errorf("the editor writes\n got %q\nwant %q", got, c.want)
			}
		})
	}
}

// An empty style is a highlighter that is off, which is what every real shell
// is: measured, none of bash, zsh, dash or ksh93 colors a line as it is typed.
func TestUnclosedQuoteWithNoStyleColorsNothing(t *testing.T) {
	if got := (UnclosedQuote{}).Highlight(`echo "open`); got != nil {
		t.Errorf("an unstyled highlighter returned %v, want nothing", got)
	}
}

// The number that decided the seam's shape, kept where it can go stale loudly.
//
// #803 asked for the re-parse cost on a realistic line before the design, on
// the grounds that it decides everything. It does, and the answer was that a
// full parse per keystroke costs single-digit microseconds — so the seam hands
// over the line and nothing else: no cached tree, no incremental lexer, no
// invalidation. See highlight.go for the recorded figures.
func BenchmarkReparsingALineOnEveryKeystroke(b *testing.B) {
	for _, c := range []struct{ name, line string }{
		{"short", "ls -la"},
		{"typical", "git log --oneline --graph -20 | head -n 5 > /tmp/out.txt"},
		{"long", `for f in $(find . -name '*.go'); do grep -n "TODO" "$f" >> "$HOME/t"; done`},
		{"very-long", strings.Repeat("echo one two three | cat ; ", 20)},
	} {
		b.Run(c.name, func(b *testing.B) {
			for range b.N {
				p := syntax.NewParser(c.line+"\n", syntax.Core())
				for {
					if _, ok := p.NextLine(); !ok {
						break
					}
				}
				_ = p.Err()
			}
		})
	}
}

// The colored line reaches a real terminal, escape bytes and all, and the
// cursor still lands where the characters are.
//
// Both halves matter and only the terminal can be asked about the second. The
// editor places the cursor by counting the *runes* of the line, so a redraw
// that wrote the escape sequences and then counted them would leave the cursor
// several columns to the right of where the person is typing — and a test that
// stripped ANSI before looking would pass either way.
func TestAColoredLineReachesTheTerminalAndTheCursorStillLandsRight(t *testing.T) {
	s := newSessionWith(t, func(sh *Shell) {
		sh.Highlighter = UnclosedQuote{Style: "\x1b[31m"}
	})

	// A quotation left open, then closed, then `^A` to the start of the line
	// and the missing letter typed there. The command only runs if the cursor
	// was where the editor said it was: `cho "one two"` is not a command, and
	// the `e` has to land in front of it and nowhere else.
	s.typeLine("cho \"one two\"\x01e\n")
	waitFor(t, s.ran, "one two", "the command's output")

	// The redraw for the moment the quotation was open, exactly: the erase,
	// the prompt, and the line with the unclosed word in red. `cho "one` is
	// what had been typed when the `t` of `two` had not yet arrived.
	const want = "\r\x1b[K[1]cho \x1b[31m\"one\x1b[0m"
	if got := s.screen.String(); !strings.Contains(got, want) {
		t.Errorf("the open quotation was never drawn in red.\nwant a draw of %q\ngot %q", want, got)
	}
	// And once it was closed, the same line is drawn plainly.
	const closed = "\r\x1b[K[1]cho \"one two\""
	if got := s.screen.String(); !strings.Contains(got, closed) {
		t.Errorf("the closed line was never drawn plainly.\nwant a draw of %q\ngot %q", closed, got)
	}
	s.end()
}
