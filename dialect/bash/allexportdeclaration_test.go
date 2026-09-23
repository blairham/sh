// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `set -a` marks a **declaration's** assignment for the environment as much as
// a plain one's, and what it does not mark is as measured as what it does.
//
// Measured 2026-09-22 on bash 5.3.20 under `LC_ALL=C` from a script file. The
// row that found it is `varenv.tests`, where `set -a; typeset FOOFOO=abcde;
// printenv FOOFOO` printed the value in bash and nothing here — so a script
// that exported through the option and assigned through the utility handed its
// children nothing (#4163).
func TestAllexportMarksADeclarationsAssignment(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src, want string }{
		{"typeset", `set -a; typeset F=x; declare -p F`, `declare -x F="x"`},
		{"declare", `set -a; declare F=x; declare -p F`, `declare -x F="x"`},
		{"readonly", `set -a; readonly F=x; declare -p F`, `declare -rx F="x"`},
		{"with a type letter", `set -a; declare -i F=3; declare -p F`, `declare -ix F="3"`},
		{"inside a function", `set -a; f(){ typeset F=x; declare -p F; }; f`, `declare -x F="x"`},
		{"the global letter", `set -a; declare -g F=x; declare -p F`, `declare -x F="x"`},
		// A plain assignment was already marked, and is here so the pair
		// cannot drift: the whole defect was one spelling doing it.
		{"a plain assignment", `set -a; F=x; declare -p F`, `declare -x F="x"`},
		// And an array is **not** marked, which is the row that says this is a
		// measurement rather than "every store".
		{"an array literal", `set -a; declare -a A=(1); declare -p A`, `declare -a A=([0]="1")`},
		{"a table literal", `set -a; declare -A M=([k]=v); declare -p M`, `declare -A M=([k]="v" )`},
		// Nor is a declaration with no value at all, in this shell — bash
		// marks that one and we do not; see the note below.
		{"off again", `set -a; set +a; typeset F=x; declare -p F`, `declare -- F="x"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, dir, c.src)
			if strings.TrimSpace(out) != c.want {
				t.Errorf("output %q, want %q", out, c.want)
			}
		})
	}
	// The environment a child is really handed, which is the half a listing
	// cannot show. A shell of our own rather than `printenv`, which these runs
	// have no PATH for — and an inner shell reads its environment the same way
	// the program would.
	// No pipeline and no external program: these runs have no PATH, and a
	// missing `grep` would read as a name that was not exported.
	out, _ := runBash(t, dir, `set -a; unset F; F=bar; typeset F=abcde; export -p`)
	wantWholeLines(t, out, `declare -x F="abcde"`)
}

// Two spellings bash marks and this shell does not, measured and written down
// rather than left to be found again: `local F=x` inside a function and a
// declaration carrying **no value** at all.
//
//	set -a; f(){ local F=x; declare -p F; }; f    declare -x F="x"    ours: declare --
//	set -a; declare F; declare -p F              declare -x F        ours: declare --
//
// Both are the same defect in a different store — `local`'s value does not reach
// the one this change fixed, and a valueless declaration stores nothing for it to
// reach — and neither is a line of the suite row that found the first. They are
// recorded here so the next reader measures rather than assumes (#4163).
func TestAllexportStillMissesTwoSpellings(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src, want string }{
		{"local with a value", `set -a; f(){ local F=x; declare -p F; }; f`, `declare -- F="x"`},
		{"a declaration with no value", `set -a; declare F; declare -p F`, `declare -- F`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, dir, c.src)
			if strings.TrimSpace(out) != c.want {
				t.Errorf("output %q, want %q — if this now matches bash's "+
					"`declare -x`, the gap is closed and the row should move", out, c.want)
			}
		})
	}
}
