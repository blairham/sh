// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An empty key on a keyed table is a key like any other in two of the three
// columns that have the attribute, and it took the parser to reach one at all.
//
// `m[""]=4` parsed with **no subscript**, because the empty quoted span the
// brackets held was dropped as a zero-length piece of text — so the assignment
// arrived as a plain `m=4` and stored under the key `0`. That is the store
// half; the read half already resolved the key correctly, which is why the
// pair `assign then read` came back empty and looked like a lookup fault
// (#1938).
func TestAnEmptyKeyIsAKey(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"written as an empty quotation",
			`typeset -A m; m[""]=4; printf '[%s]' "${m[""]}"`,
			`[4]`,
		},
		{
			"reached through a parameter",
			`typeset -A m; w=; m[$w]=4; printf '[%s]' "${m[$w]}"`,
			`[4]`,
		},
		{
			// The two spellings name the same key where the subscript is a
			// quoting context, so the second assignment replaces the first
			// and the table holds one element.
			"the two spellings are one key",
			`typeset -A m; w=; m[""]=4; m[$w]=5; printf '[%s][%s]' "${#m[@]}" "${m[""]}"`,
			`[1][5]`,
		},
		{
			// The control that says this is emptiness and not blankness: one
			// space is a key, and a different one.
			"a blank key is a key of its own",
			`typeset -A m; m[""]=4; m[" "]=7; printf '[%s][%s][%s]' "${#m[@]}" "${m[""]}" "${m[" "]}"`,
			`[2][4][7]`,
		},
		{
			// And that the empty key is not element zero, which is what the
			// parser gap made of it.
			"the empty key is not the key zero",
			`typeset -A m; m[""]=4; printf '[%s]' "${m[0]}"`,
			`[]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.EmptyAssociativeKeyIsAnError = No
			// Reading one is a question of its own, and this file is about
			// the store — see TestAnEmptyKeyReadMayBeReported (#1972).
			sem.EmptyAssociativeKeyIsReportedWhenRead = No
			sem.SubscriptIsAQuotingContext = Yes
			out, st := run(t, tc.src, withSem(sem))
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// Where the subscript is *not* a quoting context, `m[""]` is a two-character
// key and the empty one is reached only through a parameter.
//
// The same axis one construct over, and the row that says the two columns
// which store an empty key are not storing the same thing.
func TestAnEmptyQuotationIsAKeyOfItsOwnWhereQuotesStay(t *testing.T) {
	const src = `typeset -A m; w=; m[""]=4; m[$w]=5; printf '[%s][%s][%s]' "${#m[@]}" "${m[""]}" "${m[$w]}"`
	sem := testSemantics()
	sem.EmptyAssociativeKeyIsAnError = No
	sem.EmptyAssociativeKeyIsReportedWhenRead = No
	sem.SubscriptIsAQuotingContext = No
	out, st := run(t, src, withSem(sem))
	if want := `[2][4][5]`; out != want || st != 0 {
		t.Errorf("%s = %q (status %d), want %q at 0", src, out, st, want)
	}
}

// And the column that refuses to store under an empty key at all.
func TestAnEmptyKeyMayBeRefused(t *testing.T) {
	for _, tc := range []struct{ name, src, subject string }{
		{"written as an empty quotation", `typeset -A m; m[""]=4`, `m[""]`},
		{"written as an empty single quotation", `typeset -A m; m['']=4`, `m['']`},
		// The subject is the subscript as it was written and not what it came
		// to, which is why the diagnostic goes through the printer.
		{"reached through a parameter", `typeset -A m; w=; m[$w]=4`, `m[$w]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.EmptyAssociativeKeyIsAnError = Yes
			sem.SubscriptIsAQuotingContext = Yes
			// On the next *line*: the refusal gives up the rest of the
			// command list, which is the same thing a refused reassignment
			// does in the column that reaches either — so a `;` here would
			// assert nothing about the table.
			src := tc.src + "\nprintf '[%s]' \"${#m[@]}\"\n"
			out, _ := run(t, src, withSem(sem))
			if !strings.Contains(out, tc.subject) {
				t.Errorf("%s = %q, want the subscript named as %q", src, out, tc.subject)
			}
			if !strings.Contains(out, "[0]") {
				t.Errorf("%s = %q, want the table left empty", src, out)
			}
			// And the rest of the list the refusal stood in is given up.
			gave, _ := run(t, tc.src+`; printf '[ran]'`, withSem(sem))
			if strings.Contains(gave, "[ran]") {
				t.Errorf("%s = %q, want the rest of the list given up", tc.src, gave)
			}
		})
	}
}

// The axis is read where a key came out empty and nowhere else.
func TestTheEmptyKeyAxisIsAskedOnlyOverAnEmptyKey(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refused   bool
	}{
		{"an empty key", `typeset -A m; m[""]=4`, true},
		{"an empty key through a parameter", `typeset -A m; w=; m[$w]=4`, true},
		{"an ordinary key", `typeset -A m; m[k]=4`, false},
		{"a blank key", `typeset -A m; m[" "]=4`, false},
		{"reading an empty key", `typeset -A m; printf '[%s]' "${m[""]}"`, false},
		{"an indexed name", `a=(1 2); a[0]=4`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.EmptyAssociativeKeyIsAnError = Unspecified
			sem.EmptyAssociativeKeyIsReportedWhenRead = No
			sem.SubscriptIsAQuotingContext = Yes
			out, _ := run(t, tc.src, withSem(sem))
			said := strings.Contains(out, "empty key on a keyed table")
			if said != tc.refused {
				t.Fatalf("got %q, want the axis %s", out,
					map[bool]string{true: "reported", false: "not reported"}[tc.refused])
			}
		})
	}
}
