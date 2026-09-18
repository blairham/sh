// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A subscript that will not evaluate in a builtin's operand is a complaint the
// **language** makes through that builtin, so the three sites that report one
// put the builtin's name aside first.
//
// That is two claims and they were made together: the name leaves the
// sentence, and the *location* falls back from the builtin's style to the
// shell's. One column makes only the first — see
// Diagnostics.BadSubscriptKeepsTheBuiltinsLocation, where the bare
// `$(( b c ))` is the control that proves the shell is not simply using one
// style everywhere. And in that same column `typeset` lost its name as well,
// where `unset` and `read` kept theirs (#3496).

// The location is kept where the dialect keeps it, at all three sites, and the
// sentence is the language's either way.
func TestABadSubscriptKeepsTheBuiltinsLocationWhereTheDialectDoes(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"unset", "a=(1 2 3)\nunset 'a[b c]'"},
		{"a store", "a=(1 2 3)\n{ read 'a[b c]'; } <<EOF\nY\nEOF"},
		{"a declaration", "a=(1 2 3)\ntypeset 'a[b c]'=v"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// LocationBracketLine is the style that makes the difference
			// visible: with the speaker cleared the report falls back to
			// Location, and with it kept the brackets stay.
			dg := Diagnostics{
				Location:                             LocationLineWord,
				BuiltinLocation:                      LocationBracketLine,
				BadSubscriptKeepsTheBuiltinsLocation: true,
			}
			out, _ := speakerRun(t, dg, c.src)
			if !strings.Contains(out, "[2]: ") {
				t.Errorf("out %q does not keep the builtin's location", out)
			}
			dg.BadSubscriptKeepsTheBuiltinsLocation = false
			out, _ = speakerRun(t, dg, c.src)
			if strings.Contains(out, "[2]: ") {
				t.Errorf("out %q keeps a location the dialect gives up", out)
			}
			if !strings.Contains(out, "line 2: ") {
				t.Errorf("out %q does not fall back to the shell's location", out)
			}
		})
	}
}

// The builtin's **name** is a separate question from its location, and the
// column that keeps the location puts the name in the sentence. `typeset`
// had no field to put it in at all, where `unset` and `read` had one each.
func TestADeclarationsBadSubscriptNamesTheBuiltinItWasHandedTo(t *testing.T) {
	for _, c := range []struct{ name, builtin, src string }{
		{"typeset", "typeset", "a=(1 2 3)\ntypeset 'a[b c]'=v"},
		{"readonly", "readonly", "a=(1 2 3)\nreadonly 'a[b c]'=v"},
		{"export", "export", "a=(1 2 3)\nexport 'a[b c]'=v"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := speakerRun(t, Diagnostics{DeclarationBadSubscript: "SPOKE-%[1]s %[2]s"}, c.src)
			if !strings.Contains(out, "SPOKE-"+c.builtin+" ") {
				t.Errorf("out %q does not name the builtin it was handed to", out)
			}
		})
	}
}

// Empty leaves the sentence to stand alone, which is what the two columns that
// name nobody do — so the field is not a way of always writing a name.
func TestADeclarationsBadSubscriptStandsAloneWhereTheDialectNamesNobody(t *testing.T) {
	out, _ := speakerRun(t, Diagnostics{}, "a=(1 2 3)\ntypeset 'a[b c]'=v")
	if strings.Contains(out, "typeset") {
		t.Errorf("out %q names a builtin where the dialect names none", out)
	}
	if !strings.Contains(out, "b c") {
		t.Errorf("out %q does not write the subscript's own sentence", out)
	}
}

// speakerRun answers what a subscripted operand needs and leaves the give-ups
// flat, so a row varies the location and the wording alone.
func speakerRun(t *testing.T, dg Diagnostics, src string) (string, int) {
	t.Helper()
	return optRunAs(t, func(s *Semantics) {
		arraySemantics(s)
		s.UnsetTakesASubscript = Yes
		s.TypesetTakesASubscript = Yes
		s.DeclarationTakesASubscript = Yes
		s.BadSubscriptToUnset = BadSubscriptReported
		s.BadSubscriptToAnOutputOperand = BadSubscriptReported
		s.BadSubscriptToADeclaration = BadSubscriptReported
		s.EmptyArithSubscript = EmptyArithSubscriptIsTheEmptyExpression
		// The `readonly` spelling walks past this one on its way to the
		// subscript, and an unanswered axis there would answer the row
		// instead of the field under test.
		s.ReadonlyElement = ReadonlyElementWritten
		s.FatalErrorStatusIsOne = Yes
	}, dg, src, RouteUnspecified)
}
