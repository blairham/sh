// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An attribute over a value that is *compound* — an array or a keyed table.
// Three questions, each with its own field, and tests that name axes and
// letters rather than shells:
//
//   - CompoundAttribute, what an attribute arriving over a standing compound
//     value makes of it, which has three answers where the scalar question
//     has two;
//   - CompoundElementsGoThroughTheAttribute, whether a later write to one
//     element is folded;
//   - ArrayLiteralAssignmentStartsTheNameOver, whether replacing a whole
//     array re-creates the name.

// withCompound is declRun's setter for a dialect with the three letters, the
// table letter, and the compound answers a case names.
func withCompound(p CompoundAttributePolicy, elems, startsOver Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.DeclareOptions = "aAgilprux"
		s.LocalOptions = "aAilprux"
		s.DeclareListing = DeclareListingExportSpelled
		s.BareDeclarationListing = DeclareListingPlainAssignment
		s.DeclaredNameWithoutValueIsEmpty = Yes
		s.AttributeRereadsTheValueItFinds = Yes
		s.CompoundAttribute = p
		s.CompoundElementsGoThroughTheAttribute = elems
		s.ArrayLiteralAssignmentStartsTheNameOver = startsOver
		s.ArrayScalarIsTheWholeArray = No
		s.ArrayNameWithoutSubscriptIsTheList = No
	}
}

// The three answers, over an indexed array and over a keyed table, read the
// three ways that tell them apart: the elements, the length, and — for the
// table — what a child is told.
func TestACompoundValueMeetingANewAttribute(t *testing.T) {
	for _, tc := range []struct {
		name       string
		p          CompoundAttributePolicy
		arr, assoc string
	}{
		{"keeps the elements", CompoundAttributeKeepsTheElements, "[a b] n=2\n", "[v] n=1\n"},
		{"folds every element", CompoundAttributeFoldsEveryElement, "[0 0] n=2\n", "[0] n=1\n"},
		// The table row reads `[0]` rather than empty because reading a
		// *subscript* off a scalar is its own question and not this one —
		// what this asserts is that the compound is gone and the name is
		// holding the fresh scalar the declared type gives it.
		{"replaces it with a scalar", CompoundAttributeReplacesItWithAScalar, "[0] n=1\n", "[0] n=1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, `arr=(a b)
typeset -i arr
echo "[${arr[@]}] n=${#arr[@]}"`, withCompound(tc.p, Yes, No), Diagnostics{})
			if out != tc.arr || st != 0 || errs != "" {
				t.Errorf("over an array = %q (stderr %q, status %d), want %q", out, errs, st, tc.arr)
			}
			out, errs, st = declRun(t, `typeset -A m
m[k]=v
typeset -i m
echo "[${m[k]}] n=${#m[@]}"`, withCompound(tc.p, Yes, No), Diagnostics{})
			if out != tc.assoc || st != 0 || errs != "" {
				t.Errorf("over a table = %q (stderr %q, status %d), want %q", out, errs, st, tc.assoc)
			}
		})
	}
}

// The answer that replaces leaves a **fresh** scalar and not a fold of
// anything the array held: three arrays whose contents fold three different
// ways all leave the same value, which is what a fresh declaration of the
// name would leave.
func TestTheScalarAnArrayIsReplacedWithIsAFreshOne(t *testing.T) {
	out, errs, st := declRun(t, `a=(7 8); typeset -i a; echo "1 [$a]"
b=(x y); typeset -i b; echo "2 [$b]"
c=(0x10 9); typeset -i c; echo "3 [$c]"`,
		withCompound(CompoundAttributeReplacesItWithAScalar, Yes, No), Diagnostics{})
	want := "1 [0]\n2 [0]\n3 [0]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("got %q (stderr %q, status %d), want %q — the same value from three different arrays",
			out, errs, st, want)
	}
}

// The letter matters for the replacing answer and for that one only: a case
// letter changes no kind, so there is nothing to replace the compound with
// and the elements stand.
func TestOnlyATypeLetterReplacesACompoundWithAScalar(t *testing.T) {
	out, errs, st := declRun(t, `brr=(a b)
typeset -u brr
echo "[${brr[@]}] n=${#brr[@]}"`,
		withCompound(CompoundAttributeReplacesItWithAScalar, Yes, No), Diagnostics{})
	want := "[a b] n=2\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a case letter under the replacing answer = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// A declaration that says nothing about a value never reaches the question,
// which is what keeps a bare table declaration free of a dialect: markAssoc
// puts the empty table there before the question is asked, so every
// `typeset -A` reached it until this was guarded.
func TestADeclarationWithNoTypeLetterNeedsNoCompoundAnswer(t *testing.T) {
	set := func(s *Semantics) {
		withCompound(CompoundAttributeUnspecified, Yes, No)(s)
	}
	out, errs, st := declRun(t, `typeset -A m
m[k]=v
typeset -x m
echo "[${m[k]}]"`, set, Diagnostics{})
	if out != "[v]\n" || st != 0 || errs != "" {
		t.Errorf("got %q (stderr %q, status %d), want %q with no axis asked", out, errs, st, "[v]\n")
	}
}

// An unanswered axis refuses rather than guessing: one answer leaves the
// array alone, one rewrites every element and one leaves no array at all.
func TestAnUnansweredCompoundAxisIsRefused(t *testing.T) {
	out, errs, st := declRun(t, `arr=(a b)
typeset -i arr
echo "st=$?"
echo "[${arr[@]}]"`, withCompound(CompoundAttributeUnspecified, Yes, No), Diagnostics{})
	if !strings.Contains(errs, "an attribute added to a name already holding an array") {
		t.Errorf("stderr = %q, want the axis named", errs)
	}
	want := "st=2\n[a b]\n"
	if out != want || st != 0 {
		t.Errorf("out = %q (status %d), want %q with the array left standing", out, st, want)
	}
}

// A later write to one element, which is a different question from the
// reach-back above: the answer that keeps a standing array's elements alone
// still folds what is written to one of them.
func TestAnElementWriteGoesThroughTheNamesAttribute(t *testing.T) {
	for _, tc := range []struct {
		name  string
		elems Answer
		want  string
	}{
		{"folded", Yes, "1 [1 7]\n2 [AB EF]\n3 [14]\n"},
		{"not folded", No, "1 [1 3+4]\n2 [ab ef]\n3 [7+7]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, `typeset -ia a=(1 2); a[1]=3+4; echo "1 [${a[@]}]"
typeset -ua q=(ab cd); q[1]=ef; echo "2 [${q[@]}]"
typeset -A m; typeset -i m; m[k]=7+7; echo "3 [${m[k]}]"`,
				withCompound(CompoundAttributeKeepsTheElements, tc.elems, No), Diagnostics{})
			if out != tc.want || st != 0 || errs != "" {
				t.Errorf("got %q (stderr %q, status %d), want %q", out, errs, st, tc.want)
			}
		})
	}
}

// An append and a declaration's own array literal are the same write and go
// through the same fold — the two spellings that must not be mistaken for the
// replacement below.
func TestAnAppendAndADeclarationsOwnLiteralAreFolded(t *testing.T) {
	out, errs, st := declRun(t, `typeset -ia d=(5+5 6+6); echo "1 [${d[@]}]"
typeset -ia e; e+=(7+7); echo "2 [${e[@]}]"
typeset -ia f=(1); f+=(8+8); echo "3 [${f[@]}]"`,
		withCompound(CompoundAttributeKeepsTheElements, Yes, Yes), Diagnostics{})
	want := "1 [10 12]\n2 [14]\n3 [1 16]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("got %q (stderr %q, status %d), want %q — neither spelling re-creates the name",
			out, errs, st, want)
	}
}

// Replacing a whole array: a new name in one answer and new elements in the
// other. The listing says which, and the element write *after* it is the
// load-bearing half — it says the attribute is gone rather than merely
// bypassed by the assignment that replaced the elements.
func TestAWholeArrayAssignmentReCreatesTheName(t *testing.T) {
	for _, tc := range []struct {
		name       string
		startsOver Answer
		want       string
	}{
		{"re-created", Yes, "typeset -a z=( \"5+5\" \"6+6\" )\n[3+4 6+6]\n"},
		{"replaced in place", No, "typeset -ai z=( \"10\" \"12\" )\n[7 12]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, `typeset -ia z=(1)
z=(5+5 6+6)
typeset -p z
z[0]=3+4
echo "[${z[@]}]"`,
				withCompound(CompoundAttributeKeepsTheElements, Yes, tc.startsOver), Diagnostics{})
			if out != tc.want || st != 0 || errs != "" {
				t.Errorf("got %q (stderr %q, status %d), want %q", out, errs, st, tc.want)
			}
		})
	}
}

// The bounds on that one, and it is a fact about the *spelling*: a keyed
// literal keeps the attribute, and so does the first array literal a declared
// name receives — the name has to have something to start over.
func TestWhatDoesNotReCreateTheName(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a keyed literal",
			"typeset -A m; typeset -i m; m[k]=1\nm=([j]=2+2)\nm[k]=3+3\necho \"[${m[k]}][${m[j]}]\"",
			"[6][4]\n",
		},
		{
			"the first literal a declared name receives",
			"typeset -ia b\nb=(5+5 6+6)\necho \"[${b[@]}]\"",
			"[10 12]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, tc.src,
				withCompound(CompoundAttributeKeepsTheElements, Yes, Yes), Diagnostics{})
			if out != tc.want || st != 0 || errs != "" {
				t.Errorf("got %q (stderr %q, status %d), want %q", out, errs, st, tc.want)
			}
		})
	}
}

// A declaration's own operand keeps the attribute *even over an array the
// name is already holding*, which is the row that makes the Operand guard
// load-bearing rather than a shortcut for a name with nothing in it.
// Measured, and both shells that can be asked agree: `typeset -ia d=(1);
// typeset -ia d=(5+5 6+6)` folds to `10 12` and keeps the letter.
func TestADeclarationsOwnLiteralNeverReCreatesTheName(t *testing.T) {
	out, errs, st := declRun(t, `typeset -ia d=(1)
typeset -ia d=(5+5 6+6)
typeset -p d
d[0]=3+4
echo "[${d[@]}]"`,
		withCompound(CompoundAttributeKeepsTheElements, Yes, Yes), Diagnostics{})
	want := "typeset -ai d=( \"10\" \"12\" )\n[7 12]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("got %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// All three letters go, not only the integer one: the shell that re-creates
// the name loses `-u` and `-l` with it, so a later element write is not
// folded either. Measured — `typeset -ua g=(a); g=(bb cc)` lists as
// `typeset -a g=(bb cc)` and the `dd` after it stays lower-case.
func TestReCreatingTheNameLosesEveryTypeLetter(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"upper", "typeset -ua g=(a)\ng=(bb cc)\ng[0]=dd\necho \"[${g[@]}]\"", "[dd cc]\n"},
		{"lower", "typeset -la h=(A)\nh=(BB CC)\nh[0]=DD\necho \"[${h[@]}]\"", "[DD CC]\n"},
		{"integer", "typeset -ia z=(1)\nz=(5+5)\nz[0]=3+4\necho \"[${z[@]}]\"", "[3+4]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, tc.src,
				withCompound(CompoundAttributeKeepsTheElements, Yes, Yes), Diagnostics{})
			if out != tc.want || st != 0 || errs != "" {
				t.Errorf("got %q (stderr %q, status %d), want %q", out, errs, st, tc.want)
			}
		})
	}
}

// An array carrying none of the three letters never reaches the question, so
// an ordinary `a=(x y)` over an ordinary array needs no dialect at all. Left
// unanswered here on purpose: a guard that asked anyway would refuse every
// array replacement in a preset that has not answered it.
func TestAnOrdinaryArrayReplacementNeedsNoDialect(t *testing.T) {
	set := func(s *Semantics) {
		withCompound(CompoundAttributeKeepsTheElements, Yes, Unspecified)(s)
	}
	out, errs, st := declRun(t, `a=(1 2)
a=(3 4)
echo "[${a[@]}] st=$?"`, set, Diagnostics{})
	want := "[3 4] st=0\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("got %q (stderr %q, status %d), want %q with no axis asked", out, errs, st, want)
	}
}
