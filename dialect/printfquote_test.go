// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// What each dialect's `%q` writes, byte by byte (#1707).
//
// The contract `%q` exists for is the round trip: what comes out has to read
// back as what went in. A backslash before a newline does not — a
// backslash-newline is a *line continuation*, so `a\<newline>b` read back is
// `ab` — which is what this shell wrote for every dialect before this, for
// the one character `%q` exists to make visible.
//
// Measured 2026-09-12, LC_ALL=C, with the value handed over as an argument.
// Three answers and no two alike, which is why the axis is an enumeration:
// bash moves the whole word into `$'…'` as soon as one byte cannot be written
// as itself, zsh wraps each such byte in a `$'…'` of its own, and ksh93 has
// three shapes and picks by what the value holds.
func TestEachDialectQuotesForReuse(t *testing.T) {
	for _, c := range []struct {
		value                string
		bash, zsh, ksh, dash string
	}{
		// The character the issue is about, and the one that shows the
		// whole-word rule: the space stops being escaped once the quotes
		// cover it.
		{"a\nb", `$'a\nb'`, `a$'\n'b`, `$'a\nb'`, ""},
		{"a b\nc", `$'a b\nc'`, `a\ b$'\n'c`, `$'a b\nc'`, ""},

		// The escapes each shell has a name for, and the ones it spells with
		// a number. ksh93 is the odd one twice over: no `\v`, and hex.
		{"a\tb", `$'a\tb'`, `a$'\t'b`, `$'a\tb'`, ""},
		{"a\vb", `$'a\vb'`, `a$'\v'b`, `$'a\x0bb'`, ""},
		{"a\x1bb", `$'a\Eb'`, `a$'\033'b`, `$'a\Eb'`, ""},
		{"a\x01b", `$'a\001b'`, `a$'\001'b`, `$'a\x01b'`, ""},
		{"a\xffb", `$'a\377b'`, `a$'\377'b`, `$'a\xffb'`, ""},

		// A value with nothing in it that needs an escape, where the three
		// still part company: ksh93 quotes the whole value where the other
		// two escape the byte.
		{"a b", `a\ b`, `a\ b`, `'a b'`, ""},
		{`a\b`, `a\\b`, `a\\b`, `'a\b'`, ""},
		{"a'b", `a\'b`, `a\'b`, `$'a\'b'`, ""},

		// The four printable bytes the two backslash dialects disagree
		// about, which is why bash cannot borrow zsh's table.
		{"a!b", `a\!b`, `a!b`, `a!b`, ""},
		{"a,b", `a\,b`, `a,b`, `a,b`, ""},
		{"a#b", `a#b`, `a\#b`, `'a#b'`, ""},
		{"=ab", `=ab`, `\=ab`, `'=ab'`, ""},

		// Nothing to quote, and nothing at all.
		{"abc", `abc`, `abc`, `abc`, ""},
		{"", `''`, `''`, `''`, ""},

		// A valid multibyte rune is written as itself in every column, which
		// is what keeps the byte rules from reaching a character.
		{"aĉb", "aĉb", "aĉb", "aĉb", ""},
	} {
		for _, d := range []struct{ name, want string }{
			{"bash", c.bash}, {"zsh", c.zsh}, {"ksh", c.ksh},
		} {
			p := presets[d.name]
			out, _, err := p.Combined(t, dialecttest.Base{
				// Through a variable rather than a positional parameter,
				// because the value holds bytes no source line can carry.
				Vars: map[string]string{"V": c.value},
			}, `printf '%q' "$V"`)
			if err != nil {
				t.Fatal(err)
			}
			if out != d.want {
				t.Errorf("%s: %q gave %q, want %q", d.name, c.value, out, d.want)
			}
		}
	}
}

// dash has no `%q` at all, which is the absence beside the three answers: the
// conversion stops the output where it stands, as any other unknown one does.
func TestOneDialectHasNoQuotingConversion(t *testing.T) {
	p := presets["dash"]
	out, st, err := p.Combined(t, dialecttest.Base{}, `printf '[%q]' abc; echo " st=$?"`)
	if err != nil {
		t.Fatal(err)
	}
	const want = "dash: 1: printf: %q: invalid directive\n[ st=2\n"
	if out != want || st != 0 {
		t.Errorf("said %q status %d, want %q", out, st, want)
	}
}
