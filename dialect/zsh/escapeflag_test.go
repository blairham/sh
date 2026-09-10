// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The `(g:opts:)` flag's escape set, one option letter at a time.
//
// This is where the *set* belongs, because it is a measurement about this
// shell: interp asserts when the reader runs and over what text, with a
// two-letter reader of its own, and would pass on the alphabet while getting
// the letters wrong. Every row below was taken from zsh 5.9.2 on 2026-09-09.
//
// The table is deliberately widest where the vendor manual is thinnest.
// "Process escape sequences like the echo builtin when no options are given"
// is one sentence for twelve rows, and two of them — `\101` and `\0101` — are
// the ones an implementation gets wrong while looking right on the other ten.
func TestTheEscapeFlagReadsThisShellsEscapeSet(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, value, opts, want string
	}{
		// The eight nothing argues about, checked once so the rows below are
		// about what moves rather than about whether anything is read at all.
		{"a tab", `a\tb`, ``, "a\tb"},
		{"a newline", `a\nb`, ``, "a\nb"},
		{"a backslash", `a\\b`, ``, `a\b`},

		// The escape character has two spellings and they are not the same
		// question: the lowercase one is in the base set and the capital one
		// arrives with `e`.
		{"the lowercase escape character", `a\eb`, ``, "a\x1bb"},
		{"the capital spelling is text without the e option", `a\Eb`, ``, `a\Eb`},
		{"and the escape character with it", `a\Eb`, `e`, "a\x1bb"},

		// Hexadecimal and the two Unicode widths are in the base set.
		{"a hexadecimal byte", `a\x41b`, ``, "aAb"},
		{"which takes two digits and no more", `a\x415b`, ``, "aA5b"},
		{"a four-digit code point", `a\u0041b`, ``, "aAb"},
		{"fewer digits than four, ended by a character that is not one", `a\u4b-b`, ``, "aK-b"},
		{"an eight-digit code point", `a\U00000041b`, ``, "aAb"},

		// The two rows that decide whether `o` was understood. Without it
		// only `\0NNN` is octal and the zero introduces the escape rather
		// than counting as a digit; with it the backslash is followed by up
		// to three digits and no zero is needed — so the same five
		// characters mean different things in the two readings.
		{"octal needs a leading zero", `a\101b`, ``, `a\101b`},
		{"and with the o option it does not", `a\101b`, `o`, "aAb"},
		{"the leading zero is not one of the three digits", `a\0101b`, ``, "aAb"},
		{"and under o it is", `a\0101b`, `o`, "a\b1b"},
		{"a value above a byte is truncated", `a\400b`, `o`, "a\x00b"},
		{"and the largest one is not", `a\777b`, `o`, "a\xffb"},
		{"a digit outside octal is not an escape", `a\8b`, `o`, `a\8b`},

		// The `\M-x` family, which arrives with `e` and is text without it.
		{"a meta character is text without the e option", `a\M-Ab`, ``, `a\M-Ab`},
		{"and sets the high bit with it", `a\M-Ab`, `e`, "a\xc1b"},
		{"a control character", `a\C-Ab`, `e`, "a\x01b"},
		{"which does not care about case", `a\C-ab`, `e`, "a\x01b"},
		{"and takes the low five bits", `a\C-@b`, `e`, "a\x00b"},
		{"except for the one that is delete", `a\C-?b`, `e`, "a\x7fb"},
		{"the dash is optional", `a\MAb`, `e`, "a\xc1b"},
		{"for both of them", `a\CAb`, `e`, "a\x01b"},
		{"and the two nest", `a\M-\C-Ab`, `e`, "a\x81b"},

		// `^X`, which arrives with `c` and with nothing else.
		{"a caret is text without the c option", `a^Xb`, ``, `a^Xb`},
		{"and a control character with it", `a^Xb`, `c`, "a\x18b"},
		{"a caret is as good a target as a letter", `a^^^Ab`, `c`, "a\x1e\x01b"},
		{"a trailing caret is a caret", `a^`, `c`, "a^"},
		{"the caret reaches inside a meta escape", `a\M-^Ab`, `ec`, "a\x81b"},
		{"but not inside a control escape", `a\C-^Ab`, `ec`, "a\x1eAb"},

		// `\c`, the one row the manual is explicit about: it never ends the
		// output here, however the flag is written, so it is either text or
		// an escape this shell does not know.
		{"the truncating escape does not truncate", `a\cb`, ``, `a\cb`},
		{"nor under the octal option", `a\cb`, `o`, `a\cb`},
		{"nor under both of the print options", `a\cb`, `oe`, "acb"},

		// An escape this shell does not know, which is the same split.
		{"an unknown escape keeps its backslash", `a\qb`, ``, `a\qb`},
		{"and loses it under the e option", `a\qb`, `e`, "aqb"},
		{"a trailing backslash is a backslash", `a\`, ``, `a\`},
		{"whatever the options are", `a\`, `oec`, `a\`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "v='" + tc.value + "'; print -rn -- \"${(g:" + tc.opts + ":)v}\""
			out, st := runZsh(t, dir, src)
			if out != tc.want || st != 0 {
				t.Errorf("${(g:%s:)v} on %q = %q (status %d), want %q",
					tc.opts, tc.value, out, st, tc.want)
			}
		})
	}
}

// The option letters of two arguments union rather than the later one
// replacing the earlier, which the first row is the discriminating case for:
// the second argument is empty, so a reading that assigned would lose the `o`
// and answer `X\101Y`.
func TestTheEscapeFlagsArgumentsUnion(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"an empty argument takes nothing away", `${(g:o:g::)v}`, "aAb"},
		{"in either order", `${(g::g:o:)v}`, "aAb"},
		{"and two letters add up", `${(g:o:g:e:)w}`, "a\x1bb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `v='a\101b'; w='a\Eb'; print -rn -- "` + tc.src + `"`
			out, st := runZsh(t, dir, src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// Rule 13's half-step: the escape reading runs, and *then* the prompt-style
// formatting of the `(%)` family over what it produced.
//
// It is here rather than in interp because a runner nobody handed a prompt
// table has no `(%)` step to order against. The row discriminates: `\x25` is
// a `%`, so the reading is what makes the doubled `%` the prompt step then
// reduces, and reading the two the other way round leaves the `a%%b` that
// `${(g::)v}` alone answers.
func TestTheEscapeFlagRunsBeforeThePromptEscapes(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"the reading makes what the prompt step reduces", `${(%g::)v}`, "a%b"},
		{"and the reading alone leaves it doubled", `${(g::)v}`, "a%%b"},
		{"and the prompt step alone has nothing to reduce", `${(%)v}`, `a\x25\x25b`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `v='a\x25\x25b'; print -rn -- "` + tc.src + `"`
			out, st := runZsh(t, dir, src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// An option letter the flag does not have is an error in the flags at its own
// position, with this shell's wording — the same shape a `(Z)` option letter
// gets, and not the by-name refusal an unbuilt flag letter gets.
func TestTheEscapeFlagsBadOptionLetterIsThisShellsFlagsError(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `v=x; print -rn -- "${(g:x:)v}"`)
	want := "zsh:1: error in flags near position 6 in '${(g:x:)v}'\n"
	if out != want || st == 0 {
		t.Errorf("got %q status %d, want %q at a non-zero status", out, st, want)
	}
}
