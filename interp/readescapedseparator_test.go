// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What `read` does with an IFS whitespace character the line escaped at the
// very end of the value its last name takes.
//
// Without `-r` a backslash makes the character after it data, and `read`
// carries that as a mask into the splitter. The trim at the tail does not
// agree across the panel about whether the mask reaches it, and the split is
// three ways rather than two — see Semantics.ReadTrailingEscapedSeparator.
//
// The three answers are asserted over one table, so every row says what all
// three do with the same line. That is what makes the fourth row worth having:
// the two trimming readings part there and nowhere else, which is why one
// axis with three values and not one boolean (#1360).
func TestWhatReadDoesWithAnEscapedSeparatorAtTheEnd(t *testing.T) {
	for _, tc := range []struct {
		name, src                     string
		kept, fromARemainder, trimmed string
	}{
		{
			// Three fields for two names, so the last name takes a
			// remainder. The escaped space belongs to the field `c` closes.
			"a remainder ending in an escaped space",
			`printf 'a b c\\ \n' | { read x y; printf '[%s]' "$y"; }`,
			`[b c ]`, `[b c]`, `[b c]`,
		},
		{
			// The row that needs a third answer. The escaped space in the
			// middle joins `b` and `c` into one field, so the line holds one
			// field per name and the last name takes its own field — there
			// is no remainder to trim.
			"one field per name, the field ending in an escaped space",
			`printf 'a b\\ c\\ \n' | { read x y; printf '[%s]' "$y"; }`,
			`[b c ]`, `[b c ]`, `[b c]`,
		},
		{
			"one name and one field",
			`printf 'a\\ \n' | { read x; printf '[%s]' "$x"; }`,
			`[a ]`, `[a ]`, `[a]`,
		},
		{
			"one name over a line of two fields, so a remainder",
			`printf 'a b\\ \n' | { read x; printf '[%s]' "$x"; }`,
			`[a b ]`, `[a b]`, `[a b]`,
		},
		{
			// An escaped space with a separator in front of it is a field of
			// its own and is not content, so the reading that keeps a
			// field's own space still drops this one. A rule written about
			// the character before the space rather than about the field
			// would answer `[b ]` here.
			"an escaped space that is a field of its own",
			`printf 'a b \\ \n' | { read x y; printf '[%s]' "$y"; }`,
			`[b]`, `[b]`, `[b]`,
		},
		{
			// And the row that says it is the *field* and not the character:
			// two escaped spaces closing a field with content keep both.
			"two escaped spaces closing a field with content",
			`printf 'a x b\\ \\ \n' | { read x y; printf '[%s]' "$y"; }`,
			`[x b  ]`, `[x b]`, `[x b]`,
		},
		{
			// Unescaped trailing whitespace comes off under every reading —
			// the splitter never gave it to a field.
			"an unescaped closing run",
			`printf 'a b c   \n' | { read x y; printf '[%s]' "$y"; }`,
			`[b c]`, `[b c]`, `[b c]`,
		},
		{
			// An escaped space *inside* the remainder is text under every
			// reading, which is the control that says this is about the end.
			"an escaped space inside the remainder",
			`printf 'a b\\ c\n' | { read x y; printf '[%s]' "$y"; }`,
			`[b c]`, `[b c]`, `[b c]`,
		},
		{
			// `-r` has no mask for the trim to disagree about.
			"raw, so no mask at all",
			`printf 'a b c\\ \n' | { read -r x y; printf '[%s]' "$y"; }`,
			`[b c\]`, `[b c\]`, `[b c\]`,
		},
		{
			// The trim only ever takes whitespace, so an escaped
			// non-whitespace separator is never its business. Unanimous
			// across the panel, and it must stay that way.
			"an escaped non-whitespace separator",
			`printf 'a:b:c\\:\n' | { IFS=: read x y; printf '[%s]' "$y"; }`,
			`[b:c:]`, `[b:c:]`, `[b:c:]`,
		},
		{
			// And a non-default whitespace IFS splits exactly as the default
			// one does, so the answer is about the trim rather than about
			// which character it takes.
			"a tab for IFS and for the separators",
			`printf 'a\tb\tc\\\t\n' | { IFS='	' read x y; printf '[%s]' "$y"; }`,
			"[b\tc\t]", "[b\tc]", "[b\tc]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, side := range []struct {
				policy ReadTrailingEscapedSeparatorPolicy
				want   string
			}{
				{ReadTrailingEscapedSeparatorKept, tc.kept},
				{ReadTrailingEscapedSeparatorTrimmedFromARemainder, tc.fromARemainder},
				{ReadTrailingEscapedSeparatorTrimmed, tc.trimmed},
			} {
				sem := testSemantics()
				sem.ReadTrailingEscapedSeparator = side.policy
				out, st := run(t, tc.src, withSem(sem))
				if out != side.want || st != 0 {
					t.Errorf("%v: %s = %q (status %d), want %q at 0",
						side.policy, tc.src, out, st, side.want)
				}
			}
		})
	}
}

// The axis is asked where the readings leave different values behind, and
// nowhere else.
//
// Both halves are the placement. An unanswered vector has to report the row
// the panel divides on, and has to stay quiet on everything around it — a
// `read` with `-r`, one whose closing run was never escaped, and one whose
// escaped separator is not whitespace at all. Reporting there would refuse a
// line all six shells answer the same way.
func TestTheReadTrimAxisIsAskedOnlyAtAnEscapedClosingSeparator(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refused   bool
	}{
		{"a remainder ending in an escaped space", `printf 'a b c\\ \n' | { read x y; printf '[%s]' "$y"; }`, true},
		{"a field ending in an escaped space", `printf 'a b\\ \n' | { read x y; printf '[%s]' "$y"; }`, true},
		{"raw", `printf 'a b c\\ \n' | { read -r x y; printf '[%s]' "$y"; }`, false},
		{"an unescaped closing run", `printf 'a b c   \n' | { read x y; printf '[%s]' "$y"; }`, false},
		{"an escaped space inside the value", `printf 'a b\\ c\n' | { read x y; printf '[%s]' "$y"; }`, false},
		{"an escaped non-whitespace separator", `printf 'a:b:c\\:\n' | { IFS=: read x y; printf '[%s]' "$y"; }`, false},
		{"nothing escaped at all", `printf 'a b c\n' | { read x y; printf '[%s]' "$y"; }`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.ReadTrailingEscapedSeparator = ReadTrailingEscapedSeparatorUnspecified
			out, _ := run(t, tc.src, withSem(sem))
			// The refusal is what is asserted rather than the status: the
			// `printf` after it succeeds and the shell's status is its.
			said := strings.Contains(out, "escaped IFS whitespace character")
			if said != tc.refused {
				t.Fatalf("got %q, want the axis %s",
					out, map[bool]string{true: "reported", false: "not reported"}[tc.refused])
			}
		})
	}
}
