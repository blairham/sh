// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A reference aimed at the whole of an array is read two ways, and which one
// is decided by the **spelling**: `$r` splices the array and `${r}` is a
// scalar read of the reference whose value is the elements joined.
//
// Not an axis. Only one column in the panel has a letter that aims a
// reference at `a[@]` at all, so this is the core's reading rather than a
// disagreement to record — see Runner.namerefSplicesItsArray for the
// measurement and Runner.bracedWholeArrayReference for the two things the
// scalar reading decides that the splice does not.
//
// The AST already carries the distinction: syntax.ParamExpr.Bare was added
// for a diagnostic that turns on the same spelling, so nothing new had to be
// recorded to ask the question.

// The fields, which are the half a reader sees first: three elements one at a
// time under the bare spelling and one joined field under the braced one.
func TestABracedWholeArrayReferenceIsOneField(t *testing.T) {
	out, st := runNameref(t, "a=(aa bb cc)\ntypeset -n r=a[@]\n"+
		"printf '<%s>' \"$r\"\necho\nprintf '<%s>' \"${r}\"\necho")
	want := "<aa><bb><cc>\n<aa bb cc>\n"
	if out != want || st != 0 {
		t.Errorf("the two spellings = %q (status %d), want %q", out, st, want)
	}
}

// Unquoted, both spellings come to the same fields — the join is put straight
// back apart by splitting — which is the control that says the difference is
// the *join* and not a second reading of the array.
func TestAnUnquotedWholeArrayReferenceSplitsEitherSpelling(t *testing.T) {
	out, st := runNameref(t, "a=(aa bb cc)\ntypeset -n r=a[@]\n"+
		"printf '<%s>' $r\necho\nprintf '<%s>' ${r}\necho")
	want := "<aa><bb><cc>\n<aa><bb><cc>\n"
	if out != want || st != 0 {
		t.Errorf("the unquoted spellings = %q (status %d), want %q", out, st, want)
	}
}

// The separator is the **target's** own, and it is the separator the written
// spellings already have: a `a[*]` target joins on IFS unconditionally, and a
// `a[@]` target asks Semantics.UnsplitAtListJoinsOnIFS, which is the axis
// `"${a[@]}"` read into one field asks. Moved here, so the row is the axis's
// rather than a second rule beside it.
//
// One row of the shell this was measured from is measured and not matched, and
// it is named in Runner.bracedWholeArrayReference rather than asserted here:
// there a `a[@]` target joins on IFS when the reference is read as a *word*
// and on a space when it is read into an assignment, so that answer is
// context-dependent and this one is not.
func TestABracedWholeArrayReferenceJoinsTheTargetsOwnWay(t *testing.T) {
	const src = "IFS=-\na=(aa bb)\ntypeset -n r=a[@]\ntypeset -n s=a[*]\n" +
		"printf '<%s>' \"${r}\"\necho\nprintf '<%s>' \"${s}\"\necho"
	for _, tc := range []struct {
		joins Answer
		want  string
	}{
		{Yes, "<aa-bb>\n<aa-bb>\n"},
		{No, "<aa bb>\n<aa-bb>\n"},
	} {
		sem := namerefAimSemantics()
		sem.ArrayBaseIsZero = Yes
		sem.UnsplitAtListJoinsOnIFS = tc.joins
		out, st := runNamerefWith(t, sem, src)
		if out != tc.want || st != 0 {
			t.Errorf("UnsplitAtListJoinsOnIFS=%v: the two targets = %q (status %d), want %q",
				tc.joins, out, st, tc.want)
		}
	}
}

// An operator applies to that joined string and not to the elements, which is
// the discriminator that says the braced spelling is not `a[*]` either: a
// range subscript over `[*]` takes *elements*, and this takes characters.
func TestAnOperatorOnABracedWholeArrayReferenceReadsTheJoin(t *testing.T) {
	out, st := runNameref(t, "a=(aa bb cc)\ntypeset -n r=a[@]\n"+
		"echo \"[${r:0:2}]\"\necho \"[${a[*]:0:2}]\"")
	want := "[aa]\n[aa bb]\n"
	if out != want || st != 0 {
		t.Errorf("the range subscript = %q (status %d), want %q", out, st, want)
	}
}

// The length is the exception, and it is measured rather than conceded: the
// element **count** under either spelling, where the join is seven characters
// long.
func TestTheLengthOfAWholeArrayReferenceIsTheCountEitherSpelling(t *testing.T) {
	out, st := runNameref(t, "a=(aaa bbb)\ntypeset -n r=a[@]\necho \"[${#r}]\"")
	want := "[2]\n"
	if out != want || st != 0 {
		t.Errorf("the length = %q (status %d), want %q", out, st, want)
	}
}

// And the set-ness the two spellings part on: a list with no elements is
// **unset** through the braced reference, where one holding a single empty
// element is set. Neither follows from the written spelling, which is what
// makes this the reference's own rule.
func TestAnEmptyArrayIsUnsetThroughABracedReference(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"no elements", "a=()\ntypeset -n r=a[@]\necho \"[${r-D}]\"", "[D]\n"},
		{"no elements, alternate", "a=()\ntypeset -n r=a[@]\necho \"[${r+S}]\"", "[]\n"},
		{"one empty element", "a=(\"\")\ntypeset -n r=a[@]\necho \"[${r+S}]\"", "[S]\n"},
		{"one element", "a=(x)\ntypeset -n r=a[@]\necho \"[${r+S}]\"", "[S]\n"},
		{"never set", "typeset -n r=a[@]\necho \"[${r+S}]\"", "[]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNameref(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.name, out, st, tc.want)
			}
		})
	}
}

// The same set-ness under `set -u`, which is the row this whole split was
// filed for (#3125): the braced spelling refuses and names the **reference**,
// where the bare one is silent because the splice of an array nobody set is
// not an unbound parameter.
func TestNounsetReachesABracedWholeArrayReferenceOnly(t *testing.T) {
	sem := namerefAimSemantics()
	sem.ArrayBaseIsZero = Yes
	run := func(src string) (string, string, int) {
		t.Helper()
		out, st := runNamerefWith(t, sem, "set -u\n"+src)
		before, after, _ := strings.Cut(out, "\x00")
		return before, after, st
	}
	out, _, st := run("typeset -n r=a[@]\necho \"[$r]\"\necho OK")
	if out != "[]\nOK\n" || st != 0 {
		t.Errorf("the bare spelling = %q (status %d), want it silent", out, st)
	}
	out, _, st = run("typeset -n r=a[@]\necho \"[${r}]\"\necho OK")
	if strings.Contains(out, "OK") || st == 0 {
		t.Errorf("the braced spelling = %q (status %d), want the refusal", out, st)
	}
	out, _, st = run("a=()\ntypeset -n r=a[@]\necho \"[${r}]\"\necho OK")
	if strings.Contains(out, "OK") || st == 0 {
		t.Errorf("a list with no elements = %q (status %d), want the refusal", out, st)
	}
	out, _, st = run("a=(\"\")\ntypeset -n r=a[@]\necho \"[${r}]\"\necho OK")
	if out != "[]\nOK\n" || st != 0 {
		t.Errorf("one empty element = %q (status %d), want it silent", out, st)
	}
}
