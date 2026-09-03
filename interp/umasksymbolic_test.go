// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The bug: a symbolic mask was read as an octal one, failed, and was reported
// as a number out of range. All four accept `u=rwx,g=,o=` and all four make it
// 0077.
func TestASymbolicMaskIsAccepted(t *testing.T) {
	for _, c := range []struct {
		start int
		expr  string
		want  int
	}{
		// The form the issue was filed on, and the one scripts write.
		{0o022, "u=rwx,g=,o=", 0o077},
		{0o022, "a=", 0o777},
		{0o022, "a=rwx", 0o000},
		{0o022, "u=", 0o722},
		{0o022, "ugo=r", 0o333},
		{0o022, "u=r,g=w,o=x", 0o356},
		{0o022, "a+rwx", 0o000},
		{0o022, "o-rwx", 0o027},
		{0o000, "a-w", 0o222},
		{0o022, "u+w", 0o022},
		{0o077, "g+r", 0o037},
		// An omitted who is all three, not the owner: `umask 022; umask -w`
		// is 222 in all four.
		{0o022, "+w", 0o000},
		{0o022, "-w", 0o222},
		{0o022, "=w", 0o555},
		// Clauses are applied left to right, so a later one sees the earlier.
		{0o022, "a=r,+w", 0o111},
		// More than one operator in a clause.
		{0o022, "u+rw-x", 0o122},
		// `s` and `t` are taken and are worth nothing — a mask has no setuid
		// or sticky bit to deny — so the `r` still lands.
		{0o077, "u=rs", 0o377},
	} {
		out, _, held := umaskRun(t, c.start, nil, "umask -- "+c.expr)
		if held != c.want {
			t.Errorf("umask %04o; umask %s: mask %04o, want %04o (%s)", c.start, c.expr, held, c.want, out)
		}
	}
}

// The octal spelling still works, which is the half this could break.
func TestTheOctalSpellingStillWorks(t *testing.T) {
	for _, c := range []struct {
		arg  string
		want int
	}{{"022", 0o022}, {"0022", 0o022}, {"777", 0o777}, {"0", 0}, {"7", 7}} {
		if _, _, held := umaskRun(t, 0o077, nil, "umask "+c.arg); held != c.want {
			t.Errorf("umask %s: mask %04o, want %04o", c.arg, held, c.want)
		}
	}
}

// Which spelling is decided by the first character rather than by trying one
// and falling back — `1x` is a bad *number* and `-1` is a bad *mode*, and the
// two get different complaints in the dialects that word them differently.
func TestTheFirstCharacterDecidesWhichSpelling(t *testing.T) {
	for _, c := range []struct {
		arg     string
		numeric bool
	}{
		{"1x", true},
		{"8", true},
		{"0888", true},
		{"999", true},
		{"-1", false},
		{"u=q", false},
		{"zz", false},
	} {
		out, _, _ := umaskSymbolicRun(t, 0o022, Diagnostics{
			UmaskBadMask:         "umask: %[1]s: bad number",
			UmaskBadSymbolicMode: "umask: %[1]s: bad mode",
		}, "umask -- "+c.arg)
		want := "bad mode"
		if c.numeric {
			want = "bad number"
		}
		if !strings.Contains(out, want) {
			t.Errorf("umask %s: said %q, want %q", c.arg, out, want)
		}
	}
}

// Two of the four say whether it wanted an operator or a permission; the other
// two name the whole argument and never reach the question.
func TestTheComplaintMaySayWhichKindOfCharacter(t *testing.T) {
	both := Diagnostics{
		UmaskBadSymbolicMode:     "umask: `%[2]s': invalid symbolic mode character",
		UmaskBadSymbolicOperator: "umask: `%[2]s': invalid symbolic mode operator",
	}
	if out, _, _ := umaskSymbolicRun(t, 0o022, both, "umask -- u=q"); !strings.Contains(out, "`q': invalid symbolic mode character") {
		t.Errorf("said %q, want the permission wording naming q", out)
	}
	if out, _, _ := umaskSymbolicRun(t, 0o022, both, "umask -- zz"); !strings.Contains(out, "`z': invalid symbolic mode operator") {
		t.Errorf("said %q, want the operator wording naming z", out)
	}
	// A dialect with no operator wording uses the one it has for both, and
	// names the whole argument rather than a character.
	one := Diagnostics{UmaskBadSymbolicMode: "umask: Illegal mode: %[1]s"}
	for _, arg := range []string{"u=q", "zz"} {
		out, _, _ := umaskSymbolicRun(t, 0o022, one, "umask -- "+arg)
		if !strings.Contains(out, "Illegal mode: "+arg) {
			t.Errorf("umask %s: said %q, want the whole argument named", arg, out)
		}
	}
}

// A mask it could not read leaves the old one alone.
func TestARefusedMaskChangesNothing(t *testing.T) {
	for _, arg := range []string{"u=q", "zz", "8", "-1"} {
		if _, _, held := umaskRun(t, 0o022, nil, "umask -- "+arg); held != 0o022 {
			t.Errorf("umask %s: mask became %04o, want it untouched", arg, held)
		}
	}
}

// zsh writes a C octal literal with a minimum of three digits, which is not
// "three digits": the leading zero comes back as soon as the owner group
// denies anything.
func TestTheShorterFormIsAnOctalLiteralNotThreeDigits(t *testing.T) {
	for _, c := range []struct {
		mask int
		want string
	}{
		{0o022, "022"},
		{0o077, "077"},
		{0, "000"},
		{7, "007"},
		{0o111, "0111"},
		{0o333, "0333"},
		{0o777, "0777"},
	} {
		out, _, _ := umaskRun(t, c.mask, func(s *Semantics) { s.UmaskPrintsFourDigits = No }, "umask")
		if strings.TrimSpace(out) != c.want {
			t.Errorf("mask %04o: printed %q, want %q", c.mask, strings.TrimSpace(out), c.want)
		}
	}
	// And the four-digit answer is unchanged by any of that.
	out, _, _ := umaskRun(t, 0o022, func(s *Semantics) { s.UmaskPrintsFourDigits = Yes }, "umask")
	if strings.TrimSpace(out) != "0022" {
		t.Errorf("printed %q, want 0022", strings.TrimSpace(out))
	}
}

// The symbolic form round-trips: what `-S` prints can be given back.
func TestWhatDashSPrintsCanBeGivenBack(t *testing.T) {
	for _, mask := range []int{0, 0o022, 0o077, 0o111, 0o333, 0o777, 0o027} {
		out, _, _ := umaskRun(t, mask, nil, "umask -S")
		spelled := strings.TrimSpace(out)
		if _, _, held := umaskRun(t, 0o777, nil, "umask -- "+spelled); held != mask {
			t.Errorf("umask -S of %04o said %q, which reads back as %04o", mask, spelled, held)
		}
	}
}
