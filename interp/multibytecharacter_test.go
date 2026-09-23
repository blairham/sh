// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// big5Alpha is U+03B1 as Big5 writes it: a lead byte and a backslash.
//
// Written as the bytes rather than as the letter, because the bytes are the
// subject. This is the encoding the *input* is in, which is what a locale names
// and what no Go literal of the character would produce.
const big5Alpha = "\xa3\x5c"

// runBig5 runs src with the read axis answered and a Big5 locale named in the
// environment.
func runBig5(t *testing.T, answer Answer, src string) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := *r.Semantics
		sem.ReadTakesAMultibyteCharacterWhole = answer
		r.Semantics = &sem
		r.Env = append(append([]string(nil), r.Env...), "LC_ALL=zh_TW.Big5")
	})
}

// `read` hands over the character it was given, and the backslash it is spelled
// with is not an escape.
//
// This is the sharpest shape of #4235: with the trail byte read as an escape,
// the separator behind the character is rescued from splitting, so `a` comes
// back holding a letter, a space and the *next* field's text, `b` holds what `c`
// should have, and `c` is never assigned. Nothing is reported at any status.
//
// The fields are printed with their bytes so the assertion is on what arrived
// rather than on what a terminal would make of it.
func TestReadTakesAMultibyteCharacterWhole(t *testing.T) {
	const src = "read a b c <<EOF\n" + big5Alpha + " b c\nEOF\n" +
		`printf '[%s][%s][%s]' "$a" "$b" "$c"`
	for _, c := range []struct {
		name   string
		answer Answer
		want   string
	}{
		// The character, then the two fields the separators made.
		{"whole", Yes, "[" + big5Alpha + "][b][c]"},
		// The corruption written out: the lead byte, the space the backslash
		// rescued, `b`, and the next field shifted down one name.
		{"a byte at a time", No, "[\xa3 b][c][]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBig5(t, c.answer, src)
			if out != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, c.want)
			}
		})
	}
}

// `read -r` is unmoved by the axis, because a raw read has no escape to be
// wrong about.
//
// It is the row that says this fix is about the escape and not about the
// character: whatever the axis answers, `-r` was already delivering the bytes
// as they came, and a change that made the reading depend on the encoding
// everywhere would show up here.
func TestARawReadIsUnmovedByTheAxis(t *testing.T) {
	const src = "read -r a b <<EOF\n" + big5Alpha + " b\nEOF\n" +
		`printf '[%s][%s]' "$a" "$b"`
	for _, answer := range []Answer{Yes, No} {
		out, st := runBig5(t, answer, src)
		if want := "[" + big5Alpha + "][b]"; out != want || st != 0 {
			t.Errorf("%v: got %q (status %d), want %q at 0", answer, out, st, want)
		}
	}
}

// A lead byte the input does not finish is a byte, and the delimiter behind it
// still ends the field.
//
// The shape every script holds: a line whose last character is a multibyte one,
// where the byte after the lead is the newline. A reader that took the pair on
// the strength of the lead alone would swallow the line ending and read on into
// the next line.
func TestALeadByteAtTheEndOfALineIsAByte(t *testing.T) {
	const src = "read a\nread b\n" + `printf '[%s][%s]' "$a" "$b"` + "\n"
	out, st := run(t, "{ "+src+" } <<EOF\n\xa3\nsecond\nEOF\n", func(r *Runner) {
		sem := *r.Semantics
		sem.ReadTakesAMultibyteCharacterWhole = Yes
		r.Semantics = &sem
		r.Env = append(append([]string(nil), r.Env...), "LC_ALL=zh_TW.Big5")
	})
	if want := "[\xa3][second]"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// The text of a program is the other reader, and it is a separate axis: the
// panel has two columns that answer the two of them differently.
//
// `eval` is the route this can be asked through in this package, and it is a
// real one — it parses at run time, with the runner's own locale and the
// runner's own dialect, which is exactly the pair the hook is built from. A
// word whose last character is spelled with a backslash runs into the next line
// under the byte reading: `x=` becomes a command, and the shell says so about a
// name nobody wrote.
func TestTheTextOfAProgramIsReadWhole(t *testing.T) {
	const src = "eval 'x=" + big5Alpha + "\n" + `printf "[%s]" "$x"` + "'"
	for _, c := range []struct {
		name   string
		answer Answer
		want   string
		status int
	}{
		{"whole", Yes, "[" + big5Alpha + "]", 0},
		// The trail byte escaped the newline, so the two lines became one: the
		// `printf` and its format are words of the assignment's line, and the
		// shell looks for a command by the name of the *format string*. The
		// diagnostic is asserted whole because the name in it is the evidence
		// — a script that says something nobody wrote, at a status that says
		// something ran.
		{"a byte at a time", No, "sh: [%s]: not found\n", 127},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := run(t, src, func(r *Runner) {
				sem := *r.Semantics
				sem.MultibyteCharacterIsReadWhole = c.answer
				r.Semantics = &sem
				r.Env = append(append([]string(nil), r.Env...), "LC_ALL=zh_TW.Big5")
			})
			if out != c.want || st != c.status {
				t.Errorf("got %q (status %d), want %q at %d", out, st, c.want, c.status)
			}
		})
	}
}

// Neither axis is reached where it cannot move the answer, which is what keeps
// a core whose answers are all "unanswered" from refusing ordinary text.
//
// A locale naming a multibyte encoding is set throughout: what keeps the
// question away is the bytes. ASCII is one character in every encoding there
// is, and a UTF-8 locale cannot hide a metacharacter inside a character at all.
func TestTheEncodingIsAskedAboutOnlyWhereItDecides(t *testing.T) {
	for _, c := range []struct {
		name, env, src, want string
	}{
		{
			"ASCII text under a Big5 locale", "LC_ALL=zh_TW.Big5",
			"read a b <<EOF\nx y\nEOF\n" + `printf '[%s][%s]' "$a" "$b"`,
			"[x][y]",
		},
		{
			"a character of a UTF-8 locale", "LC_ALL=en_US.UTF-8",
			"read a b <<EOF\né y\nEOF\n" + `printf '[%s][%s]' "$a" "$b"`,
			"[é][y]",
		},
		{
			"and the same bytes under a single-byte locale", "LC_ALL=C",
			"read a b <<EOF\n" + big5Alpha + " y\nEOF\n" + `printf '[%s][%s]' "$a" "$b"`,
			// The backslash is an escape here, and that is not this defect:
			// under a locale that names no multibyte encoding there is no
			// character for it to be part of.
			"[\xa3 y][]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Unspecified, which is the core's answer and the one that
			// complains: if either axis were consulted here the output would
			// carry a diagnostic and the status would be 2.
			out, st := run(t, c.src, func(r *Runner) {
				sem := *r.Semantics
				sem.ReadTakesAMultibyteCharacterWhole = Unspecified
				sem.MultibyteCharacterIsReadWhole = Unspecified
				r.Semantics = &sem
				r.Env = append(append([]string(nil), r.Env...), c.env)
			})
			if out != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, c.want)
			}
		})
	}
}
