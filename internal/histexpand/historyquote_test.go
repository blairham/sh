// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package histexpand_test

import (
	"testing"

	"github.com/blairham/sh/internal/histexpand"
)

// The two things a **script** asks of the engine that a prompt never could:
// a line that begins inside a quote an earlier line opened, and an entry with
// a newline in it.

func list(lines ...string) histexpand.List {
	return histexpand.List{Lines: lines, First: 1}
}

// A line handed over mid-quote is scanned from that state, and the scanner
// still closes the quote where the line closes it.
//
// Measured on bash 5.3.20 from a script file: `echo 'a` / `!!` / `b'` prints
// the two characters, `echo "a` / `!!` / `b"` expands them, and `echo 'a` /
// `b' !!` expands the reference *after* the quote ends.
func TestALineThatBeginsInsideAQuote(t *testing.T) {
	for _, c := range []struct {
		name string
		in   histexpand.Quote
		line string
		want string
	}{
		{"unquoted expands", histexpand.Unquoted, "echo !!", "echo echo AAA"},
		{"inside single quotes nothing expands", histexpand.InSingleQuotes, "!!", "!!"},
		{"inside double quotes it still expands", histexpand.InDoubleQuotes, "!!", "echo AAA"},
		{"the closing quote lets the rest expand", histexpand.InSingleQuotes, "b' !!", "b' echo AAA"},
		{"a closing double quote likewise", histexpand.InDoubleQuotes, `b" !!`, `b" echo AAA`},
		// A `'` inside a double-quoted string opens nothing, which is the
		// reason the double-quoted state has to be carried at all rather than
		// treated as the same as no state.
		{"an apostrophe inside double quotes opens nothing", histexpand.InDoubleQuotes, "it's !!", "it's echo AAA"},
		// And `^old^new^` is a quick substitution only outside a quote. A
		// body line of a continued string that happens to start with the
		// character is text.
		{"a quick substitution needs an unquoted start", histexpand.InSingleQuotes, "^AAA^BBB^", "^AAA^BBB^"},
		{"and is one outside a quote", histexpand.Unquoted, "^AAA^BBB^", "echo BBB"},
	} {
		t.Run(c.name, func(t *testing.T) {
			res, err := histexpand.ExpandIn(c.line, c.in, list("echo AAA"), histexpand.Default)
			if err != nil {
				t.Fatalf("%v", err)
			}
			if res.Line != c.want {
				t.Errorf("expanded %q from %v to %q, want %q", c.line, c.in, res.Line, c.want)
			}
		})
	}
}

// A reference with no word designator is the entry's own text. Measured:
// `echo   spaced    words` recalled by `!!` comes back with its spacing, and
// by `!!:*` as words joined with one space each.
//
// It matters far beyond spacing once a script is filling the list, because a
// script's entries hold whole multi-line commands: rebuilding one out of its
// words turns a here-document into a row of words that runs something else.
func TestAReferenceWithNoDesignatorIsTheEntryItself(t *testing.T) {
	entry := "cat <<EOD\nbody\nEOD\n"
	res, err := histexpand.Expand("echo !!", list(entry), histexpand.Default)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if res.Line != "echo "+entry {
		t.Errorf("expanded to %q, want the entry verbatim", res.Line)
	}
	spaced, err := histexpand.Expand("echo !!", list("echo   spaced    words"), histexpand.Default)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if spaced.Line != "echo echo   spaced    words" {
		t.Errorf("expanded to %q, want the spacing kept", spaced.Line)
	}
	words, err := histexpand.Expand("echo !!:*", list("echo   spaced    words"), histexpand.Default)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if words.Line != "echo spaced words" {
		t.Errorf("expanded to %q, want the words joined singly", words.Line)
	}
}
