// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A call's assignment prefix makes a fresh, plain, exported scalar, whatever
// the name it displaces was holding. Measured 2026-09-21 on bash 5.3.20 from
// a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch
// HOME, and on bash 3.2.57 for every row that build can be asked — it has no
// `-A`, `-u` or `-l`, and answers the rest identically, so the two columns do
// not split (#4087).
func TestACallsPrefixMakesAFreshPlainScalar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const ff = `ff() { declare -p foo; }; `
	for _, row := range []struct{ name, src, want string }{
		{
			"over an indexed array",
			ff + `foo=(asdf fdsa); foo=bar ff`,
			`declare -x foo="bar"`,
		},
		{
			"over a table",
			ff + `declare -A foo=([k]=v); foo=bar ff`,
			`declare -x foo="bar"`,
		},
		{
			"over the integer letter",
			ff + `declare -i foo=7; foo=bar ff`,
			`declare -x foo="bar"`,
		},
		{
			"over the upper letter",
			ff + `declare -u foo=abc; foo=bar ff`,
			`declare -x foo="bar"`,
		},
		// The lower letter needs a word with a capital in it, or the fold
		// and the fresh cell answer alike.
		{
			"over the lower letter",
			ff + `declare -l foo=ABC; foo=bAr ff`,
			`declare -x foo="bAr"`,
		},
		{
			"over a one-element array",
			ff + `declare -a foo=(asdf); foo=bar ff`,
			`declare -x foo="bar"`,
		},
		// A prefix in front of a *builtin* is the same question, reached
		// through the other route: the command sees the fresh cell there too.
		{
			"in front of a builtin",
			`foo=(asdf fdsa); foo=bar declare -p foo`,
			`declare -x foo="bar"`,
		},
		{
			"in front of a special builtin",
			`declare -i foo=7; foo=bar eval 'declare -p foo'`,
			`declare -x foo="bar"`,
		},
		{
			"through command",
			`declare -i foo=7; foo=bar command eval 'declare -p foo'`,
			`declare -x foo="bar"`,
		},
	} {
		out, st := runBash(t, dir, row.src)
		if st != 0 {
			t.Errorf("%s: %s answered %d: %q", row.name, row.src, st, out)
			continue
		}
		if got := strings.TrimSpace(out); got != row.want {
			t.Errorf("%s: %s = %q, want %q", row.name, row.src, got, row.want)
		}
	}
}

// And it is a wrong *reading* under the overlay and not only a wrong listing,
// which is what makes this worth fixing rather than spelling differently. The
// three shapes below have no `declare -p` in them at all.
//
// The integer row is the sharpest: the prefix's word is read as an arithmetic
// expression where the displaced name carried the letter, so a call given
// `foo=bar` is handed `0` (#4087).
func TestACallsPrefixIsReadAsAPlainScalar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct{ name, src, want string }{
		{
			"the whole array is the prefix alone",
			`foo=(asdf fdsa); ff() { echo "[${foo[*]}]"; }; foo=bar ff`,
			"[bar]\n",
		},
		{
			"and its length is one",
			`foo=(asdf fdsa); ff() { echo "${#foo[@]}"; }; foo=bar ff`,
			"1\n",
		},
		{
			"the word is not arithmetic",
			`declare -i foo=7; ff() { echo "[$foo]"; }; foo=bar ff`,
			"[bar]\n",
		},
		{
			"and the case letter does not fold it",
			`declare -u foo=abc; ff() { echo "[$foo]"; }; foo=bar ff`,
			"[bar]\n",
		},
	} {
		out, st := runBash(t, dir, row.src)
		if out != row.want || st != 0 {
			t.Errorf("%s: %s = %q status %d, want %q", row.name, row.src, out, st, row.want)
		}
	}
}

// The take-back is untouched by all of that: what the name held comes back
// when the call returns, kind and letters and elements alike.
//
// This is the control the rows above need. A fresh cell that was not given
// back would pass every row of both tests above and would have thrown the
// name's array away for good.
func TestTheCallsPrefixStillGivesTheBindingBack(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const gg = `gg() { :; }; `
	for _, row := range []struct{ name, src, want string }{
		{
			"the array",
			gg + `foo=(asdf fdsa); foo=bar gg; declare -p foo`,
			`declare -a foo=([0]="asdf" [1]="fdsa")`,
		},
		{
			"the table",
			gg + `declare -A foo=([k]=v); foo=bar gg; declare -p foo`,
			`declare -A foo=([k]="v" )`,
		},
		{
			"the integer letter, and its value",
			gg + `declare -i foo=7; foo=bar gg; declare -p foo`,
			`declare -i foo="7"`,
		},
		{
			"the case letter, and its folded value",
			gg + `declare -u foo=abc; foo=bar gg; declare -p foo`,
			`declare -u foo="ABC"`,
		},
		// And through the builtin route, where the take-back is a different
		// piece of code answering the same question.
		{
			"after a builtin",
			`foo=(asdf fdsa); foo=bar declare -p foo >/dev/null; declare -p foo`,
			`declare -a foo=([0]="asdf" [1]="fdsa")`,
		},
	} {
		out, st := runBash(t, dir, row.src)
		if st != 0 {
			t.Errorf("%s: %s answered %d: %q", row.name, row.src, st, out)
			continue
		}
		if got := strings.TrimSpace(out); got != row.want {
			t.Errorf("%s: %s = %q, want %q", row.name, row.src, got, row.want)
		}
	}
}

// A declaration that **keeps** the prefix's entry keeps its *value*, written
// into the binding the prefix displaced through the ordinary assignment
// rules — not the fresh cell the command was shown.
//
// Measured 2026-09-21 on bash 5.3.20. The integer row is the one that says
// which of the two it is: the letter is back and `bar` is read through it, so
// the name is frozen at `0` rather than at `bar` (#4087).
func TestADeclarationKeepsThePrefixValueThroughTheDisplacedBinding(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct{ name, src, want string }{
		{
			"the array is back, with element zero written",
			`foo=(a b); foo=bar readonly foo; declare -p foo`,
			`declare -arx foo=([0]="bar" [1]="b")`,
		},
		{
			"the integer letter is back, and reads the word as arithmetic",
			`declare -i foo=7; foo=bar readonly foo; declare -p foo`,
			`declare -irx foo="0"`,
		},
	} {
		out, st := runBash(t, dir, row.src)
		if st != 0 {
			t.Errorf("%s: %s answered %d: %q", row.name, row.src, st, out)
			continue
		}
		if got := strings.TrimSpace(out); got != row.want {
			t.Errorf("%s: %s = %q, want %q", row.name, row.src, got, row.want)
		}
	}
}

// An **append** written as a prefix joins what the displaced binding was
// showing and lands in the fresh cell, which is the two halves of this in one
// row: `asdf` is element zero of the array the prefix displaced, and `declare
// -x` is the plain exported scalar it was joined into.
//
// Measured 2026-09-21 on bash 5.3.20. bash 3.2.57 answers `declare -x
// foo="bar"` for the same line — it empties the cell before the join rather
// than after — so this is the one row of #4087 where the two builds part, and
// this shell follows 5.3.
func TestAnAppendedPrefixJoinsTheDisplacedBindingIntoTheFreshCell(t *testing.T) {
	t.Parallel()
	out, st := runBash(t, t.TempDir(),
		`foo=(asdf fdsa); ff() { declare -p foo; }; foo+=bar ff`)
	const want = `declare -x foo="asdfbar"`
	if got := strings.TrimSpace(out); got != want || st != 0 {
		t.Errorf("= %q status %d, want %q", got, st, want)
	}
}

// A prefix over a **name reference** keeps the reference, because the fresh
// cell is made on the name the prefix's write *lands* on and nothing of the
// prefix lands on the reference itself. #4110 is the rest of that row — the
// export attribute belongs to the target too, and so does the take-back — and
// dialect/bash/namerefprefix_test.go holds it.
//
// Left here unchanged as the narrow guard it was written to be: whatever the
// fresh cell does, the letter is still on the name (#4087).
//
// bash 3.2.57 cannot be asked — `declare -n` is `invalid option` there, so the
// outer declaration never happens — which makes this a 5.3-only question and
// not a split between the two builds. #4087's discussion read it as a second
// column; it is not one.
func TestAPrefixOverANameReferenceIsNotTheFreshCellQuestion(t *testing.T) {
	t.Parallel()
	out, st := runBash(t, t.TempDir(),
		`target=T; declare -n foo=target; ff() { declare -p foo; }; foo=bar ff`)
	if st != 0 {
		t.Fatalf("answered %d: %q", st, out)
	}
	if got := strings.TrimSpace(out); !strings.Contains(got, "-n") {
		t.Errorf("= %q, want the name-reference letter still on it", got)
	}
}
