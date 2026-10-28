// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// runValuesPast runs src with the bad-name refusal fatal, the operands behind
// it carried, the append operand refused as a name, and the axis under test
// set — which is the one state in which the question can be reached at all.
// The names are read back from an EXIT trap, as runBadName reads them, since
// the refusal ends the script.
func runValuesPast(t *testing.T, src string, assigns Answer) (out string, status int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		s := permissive()
		s.BadNameToDeclarationFatal = Yes
		s.BadNameDeclaresTheOperandsAfterIt = Yes
		s.BadNameEndsTheDeclaringNotTheAssigning = assigns
		s.DeclarationNameOperands = PlainNamesOnly
		s.DeclarationTakesAnAppendOperand = No
		s.DeclaredNameWithoutValueIsEmpty = No
		r.Semantics = &s
	})
}

// Semantics.BadNameEndsTheDeclaringNotTheAssigning: the shell that carries a
// declaration's values past the bad name that ended its declaring, against
// the one that declares the operands behind it in full (#6510).
func TestWhetherABadNameLeavesTheValuesAssigned(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		assigns   Answer
		want      string
	}{
		// The refused `+=` operand itself lands as the statement it spells.
		{"append operand, assigned", `x=0; trap 'echo "x=$x"' EXIT; typeset x+=5`, Yes, "x=05\n"},
		{"append operand, refused whole", `x=0; trap 'echo "x=$x"' EXIT; typeset x+=5`, No, "x=0\n"},
		// A valued operand behind the refusal lands without the line's
		// letters: the integer letter does not reach it.
		{"letters not applied", `trap 'echo "[$z]"' EXIT; typeset -i ":" z=1+1`, Yes, "[1+1]\n"},
		{"letters applied", `trap 'echo "[$z]"' EXIT; typeset -i ":" z=1+1`, No, "[2]\n"},
		// And without the freeze.
		{"not frozen", `trap 'z=1; echo "z=$z"' EXIT; readonly ":" z=2`, Yes, "z=1\n"},
		{"frozen", `trap 'z=1; echo "z=$z"' EXIT; readonly ":" z=2`, No, "sh: z: readonly variable\n"},
		// A valueless operand behind the refusal declares nothing.
		{"valueless declares nothing", `trap 'z=1+1; echo "[$z]"' EXIT; typeset -i ":" z`, Yes, "[1+1]\n"},
		{"valueless declared", `trap 'z=1+1; echo "[$z]"' EXIT; typeset -i ":" z`, No, "[2]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runValuesPast(t, tc.src, tc.assigns)
			if !strings.HasSuffix(out, tc.want) || status == 0 {
				t.Errorf(
					"%s with the axis %v = %q (status %d), want it to end %q and fail",
					tc.src,
					tc.assigns,
					out,
					status,
					tc.want,
				)
			}
		})
	}
}

// The store comes first, so a store that refuses is the line's whole
// complaint: over a frozen name the append operand draws the freeze's
// sentence and the name is never judged. Where the values are not carried,
// the name is what is refused.
func TestAStoreRefusedAheadOfTheNameIsTheWholeComplaint(t *testing.T) {
	src := `readonly x=0; typeset x+=5; echo NOT-REACHED`
	out, status := runValuesPast(t, src, Yes)
	if want := "sh: x: readonly variable\n"; out != want || status == 0 {
		t.Errorf("assigned first: %q (status %d), want %q and a failure", out, status, want)
	}
	out, _ = runValuesPast(t, src, No)
	if !strings.Contains(out, "x+") || strings.Contains(out, "readonly variable") {
		t.Errorf("refused as a name: %q, want the name refused and no store attempted", out)
	}
	// And behind a plain bad name, the same: the frozen name's sentence and
	// nothing about the bad name.
	out, status = runValuesPast(t, `readonly x=0; typeset ":" x=5; echo NOT-REACHED`, Yes)
	if want := "sh: x: readonly variable\n"; out != want || status == 0 {
		t.Errorf("behind a bad name: %q (status %d), want %q and a failure", out, status, want)
	}
}

// The operands in front of the refusal are declared in full either way: the
// axis is about what is behind it.
func TestTheOperandsInFrontOfTheRefusalAreStillDeclared(t *testing.T) {
	for _, assigns := range []Answer{Yes, No} {
		out, _ := runValuesPast(t, `trap 'z=1; echo "z=$z"' EXIT; readonly z=2 ":"`, assigns)
		if want := "sh: z: readonly variable\n"; !strings.HasSuffix(out, want) {
			t.Errorf("with the axis %v = %q, want it to end %q", assigns, out, want)
		}
	}
}

// Asked at the disagreement and nowhere else: a line with no bad name, and a
// bad name with nothing behind it and no value of its own, reach no question
// — so an unanswered axis refuses nothing there.
func TestAnOrdinaryDeclarationAsksNothingAboutTheValuesBehindARefusal(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`export ok1=1 ok2=2; echo "[$ok1][$ok2]"`, "[1][2]\n"},
		{readBack + `export ok1=1 ":"`, "sh: export: `:': not a valid identifier\n[1][U]\n"},
	} {
		out, _ := runValuesPast(t, tc.src, Unspecified)
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}
