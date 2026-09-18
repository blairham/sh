// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A subscript that will not evaluate **inside `(( ))`** is the subscript's
// failure and not the construct's, and two things followed from reading it as
// the construct's: the `((: ` prefix went in front of a sentence that already
// names the subscript, and the give-up every other bracketed site makes was
// never made, so the rest of the line ran with the construct's status behind
// it.
//
// `$(( a[b c] ))` and `(( a[b c] ))` are the same subscript in the same
// arithmetic one construct apart, and they answered differently — which is the
// tell (#3507).

// arithSubSrc reads the status on the *same* line as the construct and again
// on the next, because on one line a give-up and a report print the same
// nothing.
const arithSubSrc = "a=(1 2 3)\n" +
	`(( a[b c] )); echo "same=$?"` + "\n" +
	`echo "next=$?"` + "\n" +
	"echo end"

// arithOwnSrc is the control: the construct's **own** arithmetic, which every
// column words through the construct where it words one at all.
const arithOwnSrc = `(( b c )); echo "same=$?"` + "\n" +
	`echo "next=$?"` + "\n" +
	"echo end"

// The sentence names no construct, which is unanimous and so is not an axis.
func TestASubscriptsFailureInsideAnArithmeticCommandIsNotTheConstructs(t *testing.T) {
	dg := Diagnostics{ArithErrorNamesTheConstruct: true}
	out, _ := arithSubRun(t, No, dg, arithSubSrc)
	if strings.Contains(out, "((: ") {
		t.Errorf("out %q blames the construct for a subscript's failure", out)
	}
	// And the construct keeps the prefix for its own arithmetic, which is
	// what makes the line above a distinction rather than a deletion.
	out, _ = arithSubRun(t, No, dg, arithOwnSrc)
	if !strings.Contains(out, "((: ") {
		t.Errorf("out %q dropped the construct from its own failure", out)
	}
}

// How much it gives up is the axis. One column gives up the line, as it does
// wherever a bad subscript is written; the others let the construct catch it
// and answer with its own status and fatality.
func TestWhatABadSubscriptInsideAnArithmeticCommandGivesUp(t *testing.T) {
	t.Run("given up as the subscript's", func(t *testing.T) {
		out, _ := arithSubRun(t, Yes, Diagnostics{}, arithSubSrc)
		if strings.Contains(out, "same=") {
			t.Errorf("out %q ran the rest of the line the give-up takes", out)
		}
		if !strings.Contains(out, "next=1") || !strings.Contains(out, "end") {
			t.Errorf("out %q, want the next line running with 1 behind it", out)
		}
	})
	t.Run("caught by the construct", func(t *testing.T) {
		out, _ := arithSubRun(t, No, Diagnostics{}, arithSubSrc)
		if !strings.Contains(out, "same=1") {
			t.Errorf("out %q, want the construct's own status on the same line", out)
		}
		if !strings.Contains(out, "next=0") || !strings.Contains(out, "end") {
			t.Errorf("out %q, want the line to have carried on", out)
		}
	})
}

// The **C-style `for` header** is the same measurement one construct over and
// goes through the same door, which is the whole reason there is a door: a
// copy that answered only `(( ))` is how this repository keeps re-finding the
// same bug.
func TestABadSubscriptInAForHeaderIsGivenUpTheSameWay(t *testing.T) {
	const src = "a=(1 2 3)\n" +
		`for (( i=a[b c]; i<1; i++ )); do echo x; done; echo "same=$?"` + "\n" +
		`echo "next=$?"` + "\n" +
		"echo end"
	dg := Diagnostics{ArithErrorNamesTheConstruct: true}
	out, _ := arithSubRun(t, Yes, dg, src)
	if strings.Contains(out, "((: ") {
		t.Errorf("out %q blames the header for a subscript's failure", out)
	}
	if strings.Contains(out, "same=") {
		t.Errorf("out %q ran the rest of the line the give-up takes", out)
	}
	if !strings.Contains(out, "next=1") {
		t.Errorf("out %q, want the next line running with 1 behind it", out)
	}
	// The header's own arithmetic keeps the prefix, as the command's does.
	out, _ = arithSubRun(t, Yes, dg,
		`for (( i=b c; i<1; i++ )); do echo x; done`+"\n"+`echo "next=$?"`)
	if !strings.Contains(out, "((: ") {
		t.Errorf("out %q dropped the construct from the header's own failure", out)
	}
}

// An axis nobody answered is refused by name rather than guessed at: one
// column takes the line away and two leave it running.
func TestABadSubscriptInsideAnArithmeticCommandRefusesAnUnspecifiedAxis(t *testing.T) {
	out, _ := arithSubRun(t, Unspecified, Diagnostics{}, arithSubSrc)
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("out %q is not a refusal naming the axis", out)
	}
	if !strings.Contains(out, "same=2") {
		t.Errorf("out %q does not leave the refusal's status behind", out)
	}
}

// arithSubRun answers what a subscript inside `(( ))` needs and leaves the
// construct's own status and fatality flat, so a row varies the give-up alone.
func arithSubRun(t *testing.T, escapes Answer, dg Diagnostics, src string) (string, int) {
	t.Helper()
	return optRunAs(t, func(s *Semantics) {
		arraySemantics(s)
		s.BadSubscriptEscapesAnArithmeticCommand = escapes
		// The construct's own answers, pinned so that a row is about the
		// subscript rather than about what `(( ))` does with any failure.
		s.ArithCommandErrorIsFatal = No
		s.ArithCommandErrorStatusIsTwo = No
		// And the line-give-up the escaping answer reaches, which is the
		// same door every other bracketed site goes through.
		s.FailedExpansionAbandonsTheLine = Yes
		s.FatalErrorStatusIsOne = Yes
	}, dg, src, RouteUnspecified)
}
