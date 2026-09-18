// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A sign flag is written even where the precision left no digits to sign.
//
// Run under CoreSemantics deliberately: every column on the panel writes it —
// bash 5.3.20, bash as sh, bash 3.2.57, zsh 5.9.2, ksh93u+, dash 0.5.12 and
// BusyBox ash 1.37.0, measured 2026-09-15 — so an implementation that reached
// an axis for this would be asking a question the panel does not have. C is
// two rules holding at once: a precision of 0 with a value of 0 produces no
// digits, and `+` or the space flag writes a sign for a signed conversion.
// Go's `fmt` applies the first and drops the second, which wrote nothing at
// all (#3024).
func TestASignSurvivesAPrecisionOfNought(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"a plus", `printf '[%+.0d]' 0`, "[+]"},
		{"a space", `printf '[% .0d]' 0`, "[ ]"},
		{"the other spelling of the verb", `printf '[%+.0i]' 0`, "[+]"},
		// A `.` with no digits after it is a precision of nought, which is
		// the same erasure written a second way.
		{"a bare point", `printf '[%+.d]' 0`, "[+]"},
		// No flag, so there is nothing left to write. This is what makes the
		// rows above about the *sign* rather than about the erasure.
		{"no flag at all", `printf '[%.0d]' 0`, "[]"},

		// The field, which a fix producing the sign without laying it out
		// would get wrong. The sign is one character padded to the width.
		{"padded to a width", `printf '[%+5.0d]' 0`, "[    +]"},
		{"and to the left", `printf '[%-+5.0d]' 0`, "[+    ]"},
		{"a space flag padded", `printf '[% 5.0d]' 0`, "[     ]"},
		// C ignores the `0` flag where an integer conversion states a
		// precision, and six of the seven columns do. ksh93 is the seventh
		// and pads with zeros there whatever the precision says, which is a
		// reading of its own and is filed rather than modeled (#3067) — so
		// this row is the six, and is asked here because the erasure is
		// where a stray zero fill would be most visible.
		{"the zero flag is ignored", `printf '[%+05.0d]' 0`, "[    +]"},

		// The controls: neither a nonzero value nor a precision that leaves
		// digits reaches the erasure, and neither does a float conversion.
		{"a nonzero value", `printf '[%+.0d]' 7`, "[+7]"},
		{"a precision that leaves digits", `printf '[%+.3d]' 0`, "[+000]"},
		{"a float conversion", `printf '[%+.0f]' 0`, "[+0]"},
		// An unsigned conversion has no sign at all, and every reference
		// drops the flag rather than writing one — see printfWithoutSignFlags.
		{"an unsigned conversion", `printf '[%+.0x]' 0`, "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			// C's reading of the `0` flag against a precision, which these
			// rows are not about: the one column that keeps the flag is
			// PrintfZeroFlagSurvivesAPrecision's and has its own case.
			sem.PrintfZeroFlagSurvivesAPrecision = No
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// C's `#` at a value of nought, both ways round.
//
// The axis is Semantics.PrintfAlternateFormAsksTheValue, and the second
// column is the half that a blanket "match C" would have broken: one
// reference writes `0x0` where the other six write `0`, and this shell
// already answered as that one does. Both readings are asserted here so that
// moving the axis moves the answer rather than only one of them being
// checked.
func TestTheAlternateFormAtNoughtIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name            string
		src             string
		value, digits   string
		unanimousInBoth bool
	}{
		// The two readings part on the hexadecimal: C writes the prefix for
		// a nonzero value only, the other reading writes it wherever there
		// are digits to put it in front of.
		{name: "a hexadecimal", src: `printf '[%#x]' 0`, value: "[0]", digits: "[0x0]"},
		{name: "a capital one", src: `printf '[%#X]' 0`, value: "[0]", digits: "[0X0]"},
		{name: "a precision that leaves digits", src: `printf '[%#.2x]' 0`, value: "[00]", digits: "[0x00]"},
		{name: "through a width", src: `printf '[%#5x]' 0`, value: "[    0]", digits: "[  0x0]"},
		{name: "through a zero fill", src: `printf '[%#05x]' 0`, value: "[00000]", digits: "[0x00000]"},
		// And on the octal, in the other direction: C raises the precision
		// until there is a leading zero, which is a zero to *write* where
		// the precision left no digits at all.
		{name: "an octal with nothing left", src: `printf '[%#.0o]' 0`, value: "[0]", digits: "[]"},
		{name: "and that one through a width", src: `printf '[%#5.0o]' 0`, value: "[    0]", digits: "[     ]"},

		// The controls. `%#.0x` is where the two readings agree — there is
		// no nonzero value and no digit for a prefix to go in front of — and
		// the nonzero rows are the ordinary alternate form, which is the
		// same in all seven columns. A table without these would have been
		// read as "one shell has no alternate form".
		{name: "no value and no digits", src: `printf '[%#.0x]' 0`, value: "[]", digits: "[]", unanimousInBoth: true},
		{name: "an octal nought is its own leading zero", src: `printf '[%#o]' 0`, value: "[0]", digits: "[0]", unanimousInBoth: true},
		{name: "a nonzero hexadecimal", src: `printf '[%#x]' 255`, value: "[0xff]", digits: "[0xff]", unanimousInBoth: true},
		{name: "a nonzero octal", src: `printf '[%#o]' 255`, value: "[0377]", digits: "[0377]", unanimousInBoth: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.unanimousInBoth == (tc.value != tc.digits) {
				t.Fatalf("the row says it is %v that the two readings agree, and they %v",
					tc.unanimousInBoth, tc.value == tc.digits)
			}
			for _, r := range []struct {
				name string
				a    Answer
				want string
			}{
				{"read off the value", Yes, tc.value},
				{"read off the digits", No, tc.digits},
			} {
				t.Run(r.name, func(t *testing.T) {
					sem := CoreSemantics()
					sem.PrintfAlternateFormAsksTheValue = r.a
					out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
					if out != r.want || st != 0 {
						t.Errorf("got %q status %d, want %q and 0", out, st, r.want)
					}
				})
			}
		})
	}
}

// The axis is asked where it decides the answer and nowhere else.
//
// An unanswered axis refuses, which is how the core reports a question the
// panel splits on — so this is also the assertion that a shell with no
// dialect is not quietly given one reading. The second half is the one worth
// having: a `#` on a *nonzero* value is `0xff` in all seven columns, and a
// conversion that asked anyway would refuse a construct nobody disagrees
// about.
func TestTheAlternateFormAxisIsAskedOnlyAtNought(t *testing.T) {
	for _, tc := range []struct {
		name     string
		src      string
		want     string
		refuses  bool
		wantStat int
	}{
		{name: "a nought", src: `printf '[%#x]' 0`, refuses: true, wantStat: 2},
		{name: "an octal nought with nothing left", src: `printf '[%#.0o]' 0`, refuses: true, wantStat: 2},
		{name: "a nonzero value", src: `printf '[%#x]' 255`, want: "[0xff]"},
		{name: "a nought with no alternate form", src: `printf '[%x]' 0`, want: "[0]"},
		// `#` means nothing on a `%d`, so a value of nought carrying one is
		// not this question either.
		{name: "an alternate form the verb has none of", src: `printf '[%#d]' 0`, want: "[0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if !tc.refuses {
				if out != tc.want || st != 0 {
					t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
				}
				return
			}
			if st != tc.wantStat {
				t.Errorf("status %d, want %d", st, tc.wantStat)
			}
			if !strings.Contains(out, "no dialect was chosen") {
				t.Errorf("the refusal does not say a dialect is missing: %q", out)
			}
		})
	}
}
