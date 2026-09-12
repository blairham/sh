// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// A `;` the dialect stepped over becomes the innermost unclosed thing an
// input that ran out names, in place of the keyword that would otherwise be
// named.
//
// Measured on ksh93u+ 2026-09-12, `-n` over a script file ending where it is
// shown. It is the shell that names an innermost keyword, so it is the only
// column the difference can be seen in.
//
// #1207 filed this as the `;` that admitted an empty *and-or* operand. It is
// not: `{ ; ` has no and-or in it and answers the same way, so what does it is
// the step-over (#1207).
func TestASteppedOverSeparatorIsTheInnermostThingNamed(t *testing.T) {
	d := Core()
	d.SeparatorWhereACommandBelongs = OneSeparatorExceptAfterABarOrBeforeACondition
	d.AbsentAndOrOperandIsAnEmptyCommand = true
	d.CoprocPipeOperator = true

	for _, tc := range []struct{ src, innermost string }{
		// A terminator is not this, which is the control: the `;` has to
		// have been standing where a *command* belonged.
		{"{ :", "{"},
		{"{ : ;", "{"},
		{"{ ;", ";"},
		// More input after it does not clear it.
		{"{ ; :", ";"},
		{"( ;", ";"},
		{"if :; then ;", ";"},
		{"while :; do ;", ";"},
		{"x() { ; ", ";"},
		{"{ false || ;", ";"},
		{"{ false && ;", ";"},
		{"{ : |& ;", ";"},
		// A clause does not displace it, where an ordinary clause keyword
		// displaces the one before it.
		{"if false || ; then", ";"},
		// Its construct closing does, which is what scopes it.
		{"{ ; : ; } ; if :; then", "then"},
		// And a `case` arm is the exception, measured rather than assumed.
		{"case x in x) ;", "case"},
		{"case x in x) false || ;", "case"},
		// The ordinary answers, unmoved.
		{"if :; then", "then"},
		{"while :; do", "do"},
		{"if :", "if"},
		{"if :; then : ; :", "then"},
	} {
		_, err := Parse(tc.src, d)
		var se *Error
		if !errors.As(err, &se) || se.Kind != ErrUnterminated {
			t.Errorf("%q: %v, want an ErrUnterminated", tc.src, err)
			continue
		}
		if se.Innermost != tc.innermost {
			t.Errorf("%q: innermost %q, want %q", tc.src, se.Innermost, tc.innermost)
		}
	}
}

// It is deliberately diagnostic-only: the shell that would name it has no
// open-state prompt escape, so what it draws a continuation prompt for cannot
// be measured, and guessing that half is how a wrong rule gets into the
// tables. So the separator never appears in what a prompt is drawn from.
func TestASteppedOverSeparatorNeverReachesTheOpenState(t *testing.T) {
	d := Core()
	d.SeparatorWhereACommandBelongs = OneSeparatorExceptAfterABarOrBeforeACondition
	p := NewParser("{ ; ", d)
	p.Parse()
	for _, o := range p.Open() {
		if o.Word == ";" {
			t.Fatalf("Open() = %+v, want no separator in it", p.Open())
		}
	}
	// The construct it is inside is still there, so this is the separator
	// being absent rather than the state being empty.
	if len(p.Open()) == 0 {
		t.Fatal("Open() is empty, want the brace group still open")
	}
}
