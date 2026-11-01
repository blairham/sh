// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An expansion that fails in the text of an arithmetic construct leaves text
// that is not the expression the script wrote, and nothing evaluates it. The
// failed `$((1/0))` here leaves `b = 15 `, which reads cleanly: evaluating it
// stored 15, and a frozen name's subscript written the same way was refused
// once for the expansion and again for the store (#7189).
//
// arithTextSrc reads the status on the *same* line as the construct and again
// on the next, because on one line a give-up and a report print the same
// nothing.
const arithTextSrc = `(( b = 15 $((1/0)) )); echo "same=$?"` + "\n" +
	`echo "next=$?"` + "\n" +
	`echo "b=$b"`

// arithTextRun answers what the construct's own failure needs and varies
// whether a failed expansion in its text escapes it.
func arithTextRun(t *testing.T, escapes, statusIsTwo Answer, src string) (string, int) {
	t.Helper()
	return optRunAs(t, func(s *Semantics) {
		s.FailedExpansionEscapesAnArithmeticCommand = escapes
		s.ArithCommandErrorStatusIsTwo = statusIsTwo
		s.ArithCommandErrorIsFatal = No
		// And the line-give-up the escaping answer reaches, which is the
		// same door a compound command's heading goes through.
		s.FailedExpansionAbandonsTheLine = Yes
		s.FatalErrorStatusIsOne = Yes
	}, Diagnostics{}, src, RouteUnspecified)
}

// Unanimous, so asked under both answers: the arithmetic does not run.
func TestAFailedExpansionInAnArithmeticCommandStopsTheExpression(t *testing.T) {
	t.Parallel()
	for _, escapes := range []Answer{Yes, No} {
		out, _ := arithTextRun(t, escapes, Yes, arithTextSrc)
		if !strings.HasSuffix(out, "b=\n") {
			t.Errorf("escapes %v: got %q, want nothing stored in b", escapes, out)
		}
		// One complaint, the division's: what the expansion left behind is
		// not read, so nothing follows it to report on that.
		complaints := 0
		for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
			if !strings.HasPrefix(line, "same=") && !strings.HasPrefix(line, "next=") &&
				!strings.HasPrefix(line, "b=") {
				complaints++
			}
		}
		if complaints != 1 || !strings.Contains(out, "division") {
			t.Errorf("escapes %v: got %q, want the division reported and nothing else", escapes, out)
		}
	}
}

// What it ends is the axis. One answer gives it up as the expansion's
// failure, through the door a heading's takes; the other leaves the
// construct's own status with the line running on.
func TestWhatAFailedExpansionInAnArithmeticCommandEnds(t *testing.T) {
	t.Parallel()
	t.Run("given up as the expansion's", func(t *testing.T) {
		out, _ := arithTextRun(t, Yes, Yes, arithTextSrc)
		if strings.Contains(out, "same=") {
			t.Errorf("got %q, ran the rest of the line the give-up takes", out)
		}
		if !strings.Contains(out, "next=1\n") {
			t.Errorf("got %q, want the next line running with 1 behind it", out)
		}
	})
	t.Run("caught by the construct", func(t *testing.T) {
		out, _ := arithTextRun(t, No, Yes, arithTextSrc)
		if !strings.Contains(out, "same=2\n") || !strings.Contains(out, "next=0\n") {
			t.Errorf("got %q, want the construct's own 2 and the line carrying on", out)
		}
	})
	t.Run("unanswered", func(t *testing.T) {
		out, _ := arithTextRun(t, Unspecified, No, arithTextSrc)
		if !strings.Contains(out, "no dialect was chosen") {
			t.Errorf("got %q, want a refusal naming the axis", out)
		}
		if !strings.HasSuffix(out, "b=\n") {
			t.Errorf("got %q, want nothing stored in b", out)
		}
	})
}

// The expansion `$(( ))` makes the same stop, and asks no axis: what is left
// of its text is not read, so no second complaint follows the first and
// nothing is stored (#7189).
func TestAFailedExpansionInAnArithmeticExpansionStopsTheExpression(t *testing.T) {
	t.Parallel()
	const src = `echo $(( b = 15 $((1/0)) ))` + "\n" + `echo "b=$b"`
	for _, escapes := range []Answer{Yes, No} {
		out, _ := arithTextRun(t, escapes, Yes, src)
		lines := strings.SplitAfter(out, "\n")
		if len(lines) != 3 || !strings.Contains(lines[0], "division") || lines[1] != "b=\n" {
			t.Errorf("escapes %v: got %q, want one complaint and nothing stored in b", escapes, out)
		}
	}
}
