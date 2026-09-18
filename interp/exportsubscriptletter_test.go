// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `export 'a[1]'` and `export 'a[1]'=v` write the element, and whether the
// **array** comes away carrying the export letter is the dialect's.
//
// Nothing reaches the environment in either column, so what this decides is
// the attribute and the listing behind it. One column records the letter and
// its child carries the value; the other leaves the array's own attributes
// alone, and the letter was landing there too (#3510).

// Both operand shapes, because they reach the question through one call and a
// dialect must not come to answer them differently.
func TestWhetherAnExportThroughASubscriptedOperandRecordsTheLetter(t *testing.T) {
	// The valueless shape reaches the letter through
	// ValuelessSubscriptedOperandWritesTheElement, which is where the operand
	// is finished and the caller records the attribute; the other two
	// policies hand the *base* back to the ordinary declaration below, which
	// is `export a` and not this question.
	for _, c := range []struct{ name, src string }{
		{"with a value", "a=(1 2 3)\nexport 'a[1]'=v\ntypeset -p a"},
		{"with no value", "a=(1 2 3)\nexport 'a[1]'\ntypeset -p a"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// The letters come back clustered, so the `x` is read out of
			// the cluster rather than looked for on its own.
			out, _ := exportLetterRun(t, Yes, c.src)
			if !strings.Contains(out, "declare -ax a=") {
				t.Errorf("out %q does not record the letter on the array", out)
			}
			out, _ = exportLetterRun(t, No, c.src)
			if !strings.Contains(out, "declare -a a=") {
				t.Errorf("out %q records a letter the dialect leaves off", out)
			}
			// The element is written either way: the answer moves the
			// attribute and must not move the value.
			if !strings.Contains(out, "a=") {
				t.Errorf("out %q did not list the array at all", out)
			}
		})
	}
}

// The element really is written under both answers, which is what keeps the
// row above about the attribute rather than about the store.
func TestAnExportThroughASubscriptedOperandStillWritesTheElement(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		out, _ := exportLetterRun(t, answer,
			"a=(1 2 3)\nexport 'a[1]'=v\n"+`echo "[${a[*]}]"`)
		if !strings.Contains(out, "[1 v 3]") {
			t.Errorf("answered %v: out %q, want the element written", answer, out)
		}
	}
}

// An axis nobody answered is refused by name rather than guessed at: one
// column's child carries the value and the other's does not, so neither
// answer stands in for the other.
func TestAnExportThroughASubscriptedOperandRefusesAnUnspecifiedAxis(t *testing.T) {
	out, _ := exportLetterRun(t, Unspecified, "a=(1 2 3)\nexport 'a[1]'=v")
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("out %q is not a refusal naming the axis", out)
	}
}

// exportLetterRun answers what a subscripted `export` operand needs and
// nothing else, so a row varies the letter alone.
func exportLetterRun(t *testing.T, answer Answer, src string) (string, int) {
	t.Helper()
	return optRunAs(t, func(s *Semantics) {
		arraySemantics(s)
		s.ArrayBaseIsZero = Yes
		s.DeclarationTakesASubscript = Yes
		s.TypesetTakesASubscript = Yes
		s.DeclareOptions = "aAfFgilnprux"
		s.ValuelessSubscriptedOperand = ValuelessSubscriptedOperandWritesTheElement
		s.ExportThroughASubscriptedOperandRecordsTheLetter = answer
		// A listing shape, because `typeset -p` is the only way to ask what
		// attributes a name came away with.
		s.DeclareListing = DeclareListingClustered
		s.DeclareValueQuoting = ListingQuoteAlwaysEscaped
		s.FatalErrorStatusIsOne = Yes
	}, Diagnostics{}, src, RouteUnspecified)
}
