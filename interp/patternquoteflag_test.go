// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"fmt"
	"strings"
	"testing"
)

// The `(b)` flag: the value's *pattern* metacharacters backslashed, so that
// the text matches itself when it is read as a pattern.
//
// Every row is a measurement on zsh 5.9.2 (Homebrew, arm64), 2026-09-10.
func TestThePatternQuotingFlag(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the pattern metacharacters are backslashed", `v='a b*c?d[e]'; printf "[%s]" "${(b)v}"`, `[a b\*c\?d\[e\]]`},
		// The discriminating row against `(q)`, and the reason the two are
		// not one function: the space is quoted there and not here.
		{"where (q) escapes the space as well", `v='a b*c?d[e]'; printf "[%s]" "${(q)v}"`, `[a\ b\*c\?d\[e\]]`},
		{"a value with none of them is itself", `v='plain-text_1/2:3'; printf "[%s]" "${(b)v}"`, "[plain-text_1/2:3]"},
		// The second difference from `(q)`, and it is the empty value: an
		// empty *pattern* is empty, where an empty shell word has to be
		// written `''`.
		{"an empty value stays empty", `v=""; printf "[%s]" "${(b)v}"`, "[]"},
		{"and so does a word branch that came to nothing", `y=""; printf "[%s]" "${(b)y:-$y}"`, "[]"},
		{"where (q) writes two quotes for it", `v=""; printf "[%s]" "${(q)v}"`, "['']"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, ordering, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The whole escape table, one character at a time, over the printable range.
//
// A table rather than a handful of rows because "which characters are
// special" *is* the flag: a probe on a value holding none of them cannot tell
// a correct escape from no escape at all, and a probe on `*` alone cannot
// tell this set from `(q)`'s, which is twice the size. The thirteen below
// were measured byte by byte across 1..127 and each is checked here in the
// middle of a value, at its start, at its end, alone and doubled — the
// position mattering because `(q)`'s own table has two entries that are
// special only at a value's start, and this one has none.
func TestWhatThePatternQuotingFlagEscapes(t *testing.T) {
	// Measured on zsh 5.9.2, 2026-09-10, over every byte from 1 to 127.
	const escaped = "#()*<>?[\\]^|~"
	// Everything else the printable range holds, plus the three whitespace
	// characters and two control bytes, all of which are written through as
	// themselves. `!` and `-` are the two that would be here by symmetry
	// with a bracket expression's own metacharacters and are measured out:
	// whatever would open the bracket is escaped already. The space is the
	// one that separates this flag from `(q)`.
	const literal = " \t\n\x01\x7f!\"$%&'+,-./0189:;=@ABZabz{}`"
	for _, c := range escaped + literal {
		want := string(c)
		if strings.ContainsRune(escaped, c) {
			want = `\` + want
		}
		for _, where := range []struct{ name, in, out string }{
			{"in the middle", "A" + string(c) + "Z", "A" + want + "Z"},
			{"at the start", string(c) + "Z", want + "Z"},
			{"at the end", "A" + string(c), "A" + want},
			{"alone", string(c), want},
			{"doubled", string(c) + string(c), want + want},
		} {
			t.Run(fmt.Sprintf("%q %s", c, where.name), func(t *testing.T) {
				src := "v=" + dollarSingle(where.in) + `; printf "[%s]" "${(b)v}"`
				out, st := runGrammar(t, src, ordering, nil)
				if want := "[" + where.out + "]"; out != want || st != 0 {
					t.Errorf("got %q (status %d), want %q at 0", out, st, want)
				}
			})
		}
	}
}

// dollarSingle writes s as a `$'…'` literal with every byte spelled in hex,
// so that a test's source carries no live quoting of its own.
func dollarSingle(s string) string {
	var b strings.Builder
	b.WriteString("$'")
	for i := 0; i < len(s); i++ {
		fmt.Fprintf(&b, "\\x%02x", s[i])
	}
	b.WriteString("'")
	return b.String()
}

// Where the escape sits in the group, and how it reaches an array.
//
// The compositions are the measurements; the placement is what they fix.
func TestWhereThePatternQuoteStepSits(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// An array is elementwise where the fields survive, and the join has
		// already run where they do not — so the separator the group
		// inserted is escaped along with everything else.
		{"an array is elementwise", `a=("x*y" "p q" "z"); printf "[%s]" "${(@b)a}"`, `[x\*y][p q][z]`},
		{"and joined it is one word", `a=("x*y" "p q" "z"); printf "[%s]" "${(b)a}"`, `[x\*y p q z]`},
		{"the separator is escaped with it", `a=("x*y" "p"); printf "[%s]" "${(bj:|:)a}"`, `[x\*y\|p]`},
		{"a separator with nothing special in it is not", `a=("x*y" "p"); printf "[%s]" "${(bj:,:)a}"`, `[x\*y,p]`},
		// The case conversion is ahead of it. So are the `(g)` reading and
		// the prompt escapes, neither of which has a row here because both
		// letters are answered by an escape set the dialect supplies and
		// this package holds nobody's — `(g)`'s row is in the corpus, and
		// the prompt escapes' measurement is in the spec: `${(b%):-%B*}`
		// with a `TERM` set is `ESC\[1m\*`, and a `(b)` that ran first would
		// have left the `[` the escape produced unmarked.
		{"the case conversion has run", `v='a*b'; printf "[%s]" "${(bU)v}"`, `[A\*B]`},
		// `(Q)` is rule 14's other half and runs behind this one, so the
		// pair is a round trip.
		{"the unquoting runs after it", `v='a*b'; printf "[%s]" "${(bQ)v}"`, "[a*b]"},
		// The padding is measured against the escaped text, which is what
		// says it runs behind: `a\*b` is four characters, so a width of
		// eight leaves four spaces and not five.
		{"the padding counts the escaped text", `v='a*b'; printf "[%s]" "${(bl:8:)v}"`, `[    a\*b]`},
		{"and truncates it the same way", `v='a*b'; printf "[%s]" "${(bl:3:)v}"`, `[\*b]`},
		// The split runs ahead of it, so what it escapes is each field.
		{"a split has already run", `v='a*b:c?d'; printf "[%s]" "${(@bs.:.)v}"`, `[a\*b][c\?d]`},
		{"a trim too", `v='a*b'; printf "[%s]" "${(b)v#a}"`, `[\*b]`},
		// And the length is asked of the value, not of the escape.
		{"the length is the value's own", `v='a*b'; printf "[%s]" "${(b)#v}"`, "[3]"},
		{"a word branch is escaped as a value is", `printf "[%s]" "${(b):-D*E}"`, `[D\*E]`},
		{"and an indirect one", `v='a*b'; n=v; printf "[%s]" "${(bP)n}"`, `[a\*b]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, ordering, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// `(~)` beside `(b)` is refused by name.
//
// The mark exempts an inserted join separator from the escape the rest of the
// result gets, and this flag is the one member of the quoting family the
// exemption does not survive: measured, `a=('x*y' 'p'); ${(~bj.|.)a}` is
// `x\*y\|p`, the bar escaped, where `${(~qj.|.)a}` is `x\*y|p` with the bar
// live. Carrying it would hold the join back past the escape and leave the
// bar live, which is the opposite answer.
func TestTheMarkedSeparatorBesideThePatternQuoteFlag(t *testing.T) {
	const src = `a=("x*y" "p"); printf "[%s]" "${(~bj.|.)a}"`
	const want = "sh: ${(~bj.|.)a}: the (~) expansion flag is not implemented beside the (b) flag\n"
	out, st := runGrammar(t, src, ordering, nil)
	if out != want || st == 0 {
		t.Errorf("got %q (status %d), want %q at nonzero", out, st, want)
	}
}
