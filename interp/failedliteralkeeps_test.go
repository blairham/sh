// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// failedKeepSem is the core vector with what a failed literal keeps
// answered: the subscripted reading off, so that the two readings here are
// the only ones that can keep anything.
func failedKeepSem(tableAppend, letter Answer) Semantics {
	s := elementOrderSem(No)
	s.FailedSubscriptedLiteralKeepsWhatItStored = No
	s.FailedTableAppendKeepsThePairsItAdded = tableAppend
	s.FailedOperandLeavesTheLettersKind = letter
	s.BareElementsInATableLiteralAreEachOneValue = Yes
	return s
}

// tableReadBack lists a table's keys and values in a fixed order, so a row
// does not depend on the order a table keeps.
const tableReadBack = `trap 'for k in j k i l q; do [ "${m[$k]+set}" ] && printf "%s=%s " "$k" "${m[$k]}"; done; echo' EXIT
typeset -A m
m[k]=old
`

// A bare append onto a table that fails keeps the pairs it had added, plain
// pairs included, and a key whose value never came is dropped — where the
// axis says so. Nothing else that fails keeps anything (#6704).
func TestAFailedTableAppendKeepsThePairsItAddedIsTheAxis(t *testing.T) {
	for _, c := range []struct{ src, keeps, keepsNothing string }{
		{"m+=(j new i $((1/0)))\n", "j=new k=old ", "k=old "},
		{"m+=(j new i $(echo q) l $((1/0)))\n", "j=new k=old i=q ", "k=old "},
		{"m+=(j new i '' $((1/0)))\n", "j=new k=old i= ", "k=old "},
		{"m+=([k]=new [i]=$((1/0)) [l]=z)\n", "k=new ", "k=old "},
		// Not an append, and not a bare one: nothing is kept either way.
		{"m=([j]=new [k]=$((1/0)))\n", "k=old ", "k=old "},
		{"typeset -A m+=([j]=new [i]=$((1/0)))\n", "k=old ", "k=old "},
	} {
		for _, keeps := range []Answer{Yes, No} {
			out, _ := run(t, tableReadBack+c.src, withSem(failedKeepSem(keeps, No)))
			want := c.keepsNothing
			if keeps == Yes {
				want = c.keeps
			}
			if got := trapReadBack(out); got != want {
				t.Errorf("%q, keeps=%v: read back %q, want %q", c.src, keeps, got, want)
			}
		}
	}
	// And an append onto an indexed array keeps nothing under either answer.
	for _, keeps := range []Answer{Yes, No} {
		out, _ := run(t, "trap 'echo \"${a[*]}\"' EXIT\na=(1 2)\na+=([5]=x [6]=$((1/0)))\n",
			withSem(failedKeepSem(keeps, No)))
		if got := trapReadBack(out); got != "1 2" {
			t.Errorf("an indexed append, keeps=%v: read back %q, want %q", keeps, got, "1 2")
		}
	}
}

// A declaration whose container letter re-creates the name leaves it of that
// kind and empty when its literal fails, where the axis says so; with no
// letter, or with `+=`, the name keeps what it held either way (#6705).
func TestAFailedOperandLeavesTheLettersKindIsTheAxis(t *testing.T) {
	for _, c := range []struct{ src, starts, keeps string }{
		{"a=(1 2)\ntypeset -a a=(5 $((1/0)))\n", "0 unset", "2 1"},
		{"a=(1 2)\ntypeset -A a=(5 $((1/0)))\n", "0 unset", "2 1"},
		{"a=x\ntypeset -a a=(5 $((1/0)))\n", "0 unset", "1 x"},
		{"a=(1 2)\ntypeset a=(5 $((1/0)))\n", "2 1", "2 1"},
		{"a=(1 2)\ntypeset -a a+=(5 $((1/0)))\n", "2 1", "2 1"},
	} {
		for _, letter := range []Answer{Yes, No} {
			src := "trap 'echo \"${#a[@]} ${a[0]-unset}\"' EXIT\n" + c.src
			out, _ := run(t, src, withSem(failedKeepSem(No, letter)))
			want := c.keeps
			if letter == Yes {
				want = c.starts
			}
			if got := trapReadBack(out); got != want {
				t.Errorf("%q, the letter starts over=%v: read back %q, want %q", c.src, letter, got, want)
			}
		}
	}
}

// With both axes unanswered, a literal that expands asks nothing, and a
// failed one asks only on the road its axis is about.
func TestALiteralThatExpandsAsksNothingOfWhatAFailureKeeps(t *testing.T) {
	sem := failedKeepSem(Unspecified, Unspecified)
	for _, src := range []string{
		tableReadBack + "m+=(j new i q)\n",
		"a=(1 2)\ntypeset -a a=(5 6)\necho \"${a[*]}\"\n",
		"a=(1 2)\na+=(3 $((1/0)))\n",
		"a=(1 2)\ntypeset a=(5 $((1/0)))\n",
	} {
		if out, _ := run(t, src, withSem(sem)); strings.Contains(out, "disagree") {
			t.Errorf("%q: said %q, want no question", src, out)
		}
	}
	for _, c := range []struct{ src, question string }{
		{tableReadBack + "m+=(j new i $((1/0)))\n", "keeping the pairs it had added"},
		{"a=(1 2)\ntypeset -a a=(5 $((1/0)))\n", "re-creating a name whose literal failed"},
	} {
		if out, _ := run(t, c.src, withSem(sem)); !strings.Contains(out, c.question) {
			t.Errorf("%q: said %q, want the question", c.src, out)
		}
	}
}

// trapReadBack is the last line a run wrote, which is where the exit trap's
// read-back lands.
func trapReadBack(out string) string {
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	return lines[len(lines)-1]
}

// A bare literal that fails over a name that is not an array makes it one
// where the axis says so — an empty one where the name was unset, the scalar
// at element zero where it held one — and leaves the name as it was where it
// does not. A name already an array, and a declaration's operand, end the
// same under either answer (#7078).
func TestAFailedBareLiteralStillMakesTheNameAnArrayIsTheAxis(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, makes, leaves string }{
		{"unset b\nb=(1 $((1/0)) 3)\n", `declare -a b=()`, "absent"},
		{"b=s\nb=(1 $((1/0)) 3)\n", `declare -a b=([0]="s")`, `declare -- b="s"`},
		{"unset b\nb+=(1 $((1/0)))\n", `declare -a b=()`, "absent"},
		{"b=s\nb+=(1 $((1/0)))\n", `declare -a b=([0]="s")`, `declare -- b="s"`},
		{"b=s\nb=(1 ${u?boom} 3)\n", `declare -a b=([0]="s")`, `declare -- b="s"`},
		{"b=(7)\nb=(1 $((1/0)) 3)\n", `declare -a b=([0]="7")`, `declare -a b=([0]="7")`},
		{"unset b\ntypeset -a b=(1 $((1/0)))\n", "absent", "absent"},
	} {
		for _, makes := range []Answer{Yes, No} {
			sem := failedBareSem(makes)
			src := "trap 'typeset -p b 2>/dev/null || echo absent' EXIT\n" + c.src
			out, _ := run(t, src, withSem(sem))
			want := c.leaves
			if makes == Yes {
				want = c.makes
			}
			if got := trapReadBack(out); got != want {
				t.Errorf("%q, makes an array=%v: read back %q, want %q", c.src, makes, got, want)
			}
		}
	}
}

// Left unanswered, the axis is asked only of a bare literal that failed over
// a name that is not an array.
func TestAFailedBareLiteralAsksOnlyOverANameThatIsNotAnArray(t *testing.T) {
	t.Parallel()
	sem := failedBareSem(Unspecified)
	const question = "still making its name an array"
	for _, src := range []string{
		"b=s\nb=(1 2)\n",
		"b=(7)\nb=(1 $((1/0)))\n",
		"b=(7)\nb+=(1 $((1/0)))\n",
		"b=s\ntypeset -a b=(1 $((1/0)))\n",
	} {
		if out, _ := run(t, src, withSem(sem)); strings.Contains(out, question) {
			t.Errorf("%q: said %q, want no question", src, out)
		}
	}
	for _, src := range []string{
		"b=s\nb=(1 $((1/0)))\n",
		"unset b\nb+=(1 $((1/0)))\n",
	} {
		if out, _ := run(t, src, withSem(sem)); !strings.Contains(out, question) {
			t.Errorf("%q: said %q, want the question", src, out)
		}
	}
}

// failedBareSem is failedKeepSem with the bare literal's answer chosen, and
// a listing to read the kind back with: an unset name and an empty array
// expand alike, and so do a scalar and an array of one.
func failedBareSem(makes Answer) Semantics {
	s := failedKeepSem(No, No)
	s.FailedBareLiteralStillMakesTheNameAnArray = makes
	s.DeclareListing = DeclareListingClustered
	s.DeclareValueQuoting = ListingQuoteAlwaysDouble
	s.DeclarePrintReportsAMissingName = Yes
	return s
}
