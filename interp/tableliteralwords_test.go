// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A compound literal written with **bare words** landing on a table, which is
// not a kind change — the name is already the kind being declared — and which
// the panel answers two ways: pair the words off as key, value, key, value,
// or read the parentheses as an index array and refuse to put one in a table.
// See Semantics.BareElementsInATableLiteralEndTheScript and #2611.
func runTableWords(t *testing.T, src string, refuse bool) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.ArrayLiteral = true
		d.DeclarationUtilities = map[string]bool{"typeset": true}
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.BareElementsInATableLiteralEndTheScript = refuse
		r.Semantics = &sem
		d := CoreDiagnostics()
		if r.Diagnostics != nil {
			d = *r.Diagnostics
		}
		d.IndexArrayIntoATable = "cannot append index array to associative array %[1]s"
		r.Diagnostics = &d
	})
}

func TestBareWordsInATableLiteralAreAnAxis(t *testing.T) {
	// Both halves of the axis over the same source, which is the only way to
	// say that the refusal is a *choice*: the pairing column reads the same
	// two words as one key and one value.
	for _, tc := range []struct {
		name, src, paired, refused string
	}{
		{
			"a declaration carrying the literal",
			`typeset -A m=(alpha one); echo "st=$?"; echo "[${m[alpha]}]"; echo after`,
			"st=0\n[one]\nafter\n",
			"sh: cannot append index array to associative array m\n",
		},
		{
			// The same question reached by assignment rather than by the
			// utility. One site in this engine, and the row is what says so.
			"an assignment to a name already declared",
			`typeset -A m; m=(alpha one); echo "st=$?"; echo "[${m[alpha]}]"; echo after`,
			"st=0\n[one]\nafter\n",
			"sh: cannot append index array to associative array m\n",
		},
		{
			// An append never converts, however many elements the table has:
			// there is no index array to append to a table.
			"the append spelling",
			`typeset -A m=([k]=v); m+=(alpha one); echo "st=$?"; echo "[${m[alpha]}][${m[k]}]"`,
			"st=0\n[one][v]\n",
			"sh: cannot append index array to associative array m\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runTableWords(t, tc.src, false)
			if out != tc.paired || st != 0 {
				t.Errorf("pairing: = %q (status %d), want %q", out, st, tc.paired)
			}
			out, st = runTableWords(t, tc.src, true)
			// The status and the name in front of the sentence are the
			// core dialect's — what the refusing column writes is pinned
			// where the vector is, in dialect/ksh.
			if out != tc.refused || st != 2 {
				t.Errorf("refusing: = %q (status %d), want %q at 2", out, st, tc.refused)
			}
		})
	}
}

// What counts as a bare element is the **written** shape, and these are the
// two edges of it. A literal with no element at all is taken by the refusing
// column too — there is no index array in `()` for it to object to — and an
// element that expands to nothing is still an element, so it is refused
// although the pairing column has nothing to store for it.
func TestOnlyAWrittenBareElementIsOne(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{"no element written at all", `typeset -A m=(); echo "st=$?"; echo after`, "st=0\nafter\n", 0},
		{"every element keyed", `typeset -A m=([k]=v); echo "[${m[k]}]"`, "[v]\n", 0},
		{
			"an element that came to nothing is still one",
			`e=; typeset -A m=($e); echo after`,
			"sh: cannot append index array to associative array m\n", 2,
		},
		{
			// Measured: the elements are expanded and then judged, so what
			// an element's expansion *did* has already happened.
			"the elements are expanded before they are judged",
			`typeset -A m=($(echo SIDE)); echo after`,
			"sh: cannot append index array to associative array m\n", 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runTableWords(t, tc.src, true)
			if out != tc.want || st != tc.status {
				t.Errorf("= %q (status %d), want %q at %d", out, st, tc.want, tc.status)
			}
		})
	}
}

// The refusing answer has a second half, and a test that pinned only the
// complaint would leave it free: a **replacing** literal onto a table that
// already holds an element converts the name to an index array instead. The
// pairing column stores the same two words as one key and one value, so the
// row discriminates all three readings at once.
func TestTheRefusingAnswerConvertsAPopulatedTable(t *testing.T) {
	const src = `typeset -A m=([k]=v); m=(x y); echo "st=$?"; echo "[${m[0]}][${m[1]}][n=${#m[@]}]"`
	out, st := runTableWords(t, src, false)
	if want := "st=0\n[][][n=1]\n"; out != want || st != 0 {
		t.Errorf("pairing: = %q (status %d), want %q — a table under the key `x`", out, st, want)
	}
	out, st = runTableWords(t, src, true)
	if want := "st=0\n[x][y][n=2]\n"; out != want || st != 0 {
		t.Errorf("converting: = %q (status %d), want %q — an index array of two", out, st, want)
	}
}

// And the cell next to it, which is the refusing column's own oddity: the
// same replacing literal onto a table with **no** element complains.
func TestAnEmptyTableRefusesTheLiteralAPopulatedOneTakes(t *testing.T) {
	const src = `typeset -A m; m=(x y); echo after`
	out, st := runTableWords(t, src, true)
	if want := "sh: cannot append index array to associative array m\n"; out != want || st != 2 {
		t.Errorf("= %q (status %d), want %q at 2", out, st, want)
	}
}
