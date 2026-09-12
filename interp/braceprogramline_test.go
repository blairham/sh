// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// The dialect that reports an unmatched `$( )` at the line after the input's
// last reports an unmatched `${ cmd;}` there too, and an unmatched `${x}` at
// the brace. Same opener, same sentence, and only the line moves.
//
// Measured 2026-09-12 on bash 5.3.15 over a two-line file whose second line
// is `echo after`:
//
//	echo ${ echo hi   line 3
//	echo ${x          line 1
//
// bash 3.2.57 has the construct and answers 1 for both, which is a third
// answer with no dialect here to hold it. ksh93 reaches a different diagnosis
// and dash has no such form (#1425).
func TestAnUnmatchedBraceHoldingAProgramIsBlamedWhereTheInputRanOut(t *testing.T) {
	d := syntax.Core()
	d.CurrentShellSubstitution = true

	atEnd := Diagnostics{UnmatchedReportedAtOpener: true, CmdSubstUnmatchedAtEnd: true}
	atOpener := Diagnostics{UnmatchedReportedAtOpener: true}

	// Written without a trailing newline, so the line the input *ended* on
	// and the line *after* the input's last are different numbers and the
	// flag has something to move. bash answers 3 for the command form and 1
	// for the parameter form on this exact text, measured.
	for _, tc := range []struct {
		src            string
		end, otherwise int
	}{
		{"echo ${ echo hi\necho after", 3, 2},
		{"echo ${ echo hi # cmt }\necho after", 3, 2},
		// The control: the parameter form is blamed at the brace by the
		// same dialect, which is what says the difference belongs to the
		// form and not to the two characters that opened it.
		{"echo ${x\necho after", 1, 1},
	} {
		_, err := syntax.Parse(tc.src, d)
		if err == nil {
			t.Fatalf("%q: parsed cleanly, want a refusal", tc.src)
		}
		if got := atEnd.ParseFailureLine(err); got != tc.end {
			t.Errorf("%q: line = %d, want %d", tc.src, got, tc.end)
		}
		// A dialect without the flag answers a program-holding construct
		// the way it answers `$( )`: the line the input ran out on. The
		// parameter form is unmoved either way, which is what keeps this
		// from being a rule about the brace.
		if got := atOpener.ParseFailureLine(err); got != tc.otherwise {
			t.Errorf("%q without the flag: line = %d, want %d", tc.src, got, tc.otherwise)
		}
	}
}
