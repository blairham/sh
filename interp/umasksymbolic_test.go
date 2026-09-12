// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
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

// The corners of the symbolic mask, where the panel does not agree. Each is
// asked only when the input reaches it, so an ordinary `u=rw` needs no
// answer from anybody — which is what these check as much as the answers.

// TestASecondOperatorInAClauseIsAsked covers both answers, and the "no" side
// leaves the mask alone rather than half-applying the first operator.
func TestASecondOperatorInAClauseIsAsked(t *testing.T) {
	out, _, held := umaskRun(t, 0o022, nil, `umask u+rw-x; umask`)
	if held != 0o122 {
		t.Errorf("mask %#o, want %#o with both operators applied (%q)", held, 0o122, out)
	}

	one := func(s *Semantics) { s.SymbolicMaskTakesMoreThanOneOperator = No }
	out, _, held = umaskRun(t, 0o022, one, `umask u+rw-x; umask`)
	if held != 0o022 {
		t.Errorf("mask %#o, want it left alone (%q)", held, out)
	}
	if !strings.Contains(out, "-") {
		t.Errorf("said %q, want the second operator named", out)
	}
}

// TestSettingWithNoWhoMeansAllThree. This was an axis until #2057, and the
// shape of how it stopped being one is worth keeping: zsh was recorded
// wanting a who before `=`, on the strength of `umask -- =w` written
// unquoted — which is not that operand in zsh at all. `=w` is that shell's
// `=cmd` expansion and arrives as `/usr/bin/w`, which is why the complaint
// named a `/` nobody had typed. Quoted, every column in the panel gives
// 0555.
func TestSettingWithNoWhoMeansAllThree(t *testing.T) {
	_, _, held := umaskRun(t, 0o022, nil, `umask -- "=w"`)
	if held != 0o555 {
		t.Errorf("mask %#o, want %#o", held, 0o555)
	}
}

// TestAWhoWithNoOperatorIsAsked: one dialect reads it as `=`, and one of the
// two that refuse it reaches for the complaint about a number instead.
func TestAWhoWithNoOperatorIsAsked(t *testing.T) {
	sets := func(s *Semantics) { s.SymbolicMaskWhoAloneSetsIt = Yes }
	_, _, held := umaskRun(t, 0o022, sets, `umask g`)
	if held != 0o072 {
		t.Errorf("mask %#o, want %#o — the group denied everything", held, 0o072)
	}

	out, _, held := umaskRun(t, 0o022, nil, `umask g`)
	if held != 0o022 {
		t.Errorf("mask %#o, want it left alone (%q)", held, out)
	}
	if out == "" {
		t.Error("said nothing, want a complaint")
	}
}

// TestTheSetuidAndStickyLettersAreTwoQuestions, because one dialect takes
// the first and refuses the second.
func TestTheSetuidAndStickyLettersAreTwoQuestions(t *testing.T) {
	_, _, held := umaskRun(t, 0o077, nil, `umask u=rs`)
	if held != 0o377 {
		t.Errorf("mask %#o, want %#o with `s` taken and worth nothing", held, 0o377)
	}

	noS := func(s *Semantics) { s.SymbolicMaskTakesTheSetuidLetter = No }
	out, _, held := umaskRun(t, 0o077, noS, `umask u=rs`)
	if held != 0o077 || !strings.Contains(out, "s") {
		t.Errorf("mask %#o out %q, want `s` refused and the mask left alone", held, out)
	}

	// And refusing `s` does not refuse `t`, nor the other way round.
	noT := func(s *Semantics) { s.SymbolicMaskTakesTheStickyLetter = No }
	if _, _, held := umaskRun(t, 0o077, noT, `umask u=rs`); held != 0o377 {
		t.Errorf("mask %#o, want `s` still taken when only `t` is refused", held)
	}
	if _, _, held := umaskRun(t, 0o077, noS, `umask u=rt`); held != 0o377 {
		t.Errorf("mask %#o, want `t` still taken when only `s` is refused", held)
	}
}

// TestAnOrdinaryClauseAsksNothing is the property that keeps these axes from
// reaching every script: a mask nobody disagrees about is read without any
// of them being consulted, so a shell with no answers still works.
func TestAnOrdinaryClauseAsksNothing(t *testing.T) {
	f, err := syntax.Parse(`umask u=rw,g=r,o=`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem := CoreSemantics()
	dg := Diagnostics{}
	held := 0o022
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	r.SetUmask = func(mask int) (int, error) { old := held; held = mask; return old, nil }
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "" {
		t.Errorf("said %q, want an ordinary clause to need no answer", buf.String())
	}
	// u=rw,g=r,o= allows 0640, so the mask is its complement.
	if held != 0o137 {
		t.Errorf("mask %#o, want %#o", held, 0o137)
	}
}

// TestAWhoWithNoOperatorCanReachForTheNumericComplaint: one dialect answers
// `umask g` with the wording it gives a number it could not read, and
// answers a bad *character* with a symbolic one — so which complaint it
// reaches for is not the same question as whether it refuses.
func TestAWhoWithNoOperatorCanReachForTheNumericComplaint(t *testing.T) {
	run := func(src string, numeric bool) string {
		t.Helper()
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		sem := permissive()
		sem.SymbolicMaskWhoAloneSetsIt = No
		dg := Diagnostics{
			UmaskBadMask:                     "umask: bad umask",
			UmaskBadSymbolicMode:             "umask: bad symbolic mode permission: %[2]s",
			UmaskBadSymbolicOperator:         "umask: bad symbolic mode operator: %[2]s",
			UmaskWhoAloneIsANumericComplaint: numeric,
		}
		held := 0o022
		r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
		r.SetUmask = func(mask int) (int, error) { old := held; held = mask; return old, nil }
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}

	if got := run(`umask g`, true); !strings.Contains(got, "bad umask") {
		t.Errorf("said %q, want the numeric complaint", got)
	}
	if got := run(`umask g`, false); !strings.Contains(got, "bad symbolic mode operator") {
		t.Errorf("said %q, want the symbolic complaint", got)
	}
	// And the flag reaches only that corner: a bad character still gets the
	// symbolic wording in the dialect that answers `umask g` numerically.
	if got := run(`umask u=q`, true); !strings.Contains(got, "bad symbolic") {
		t.Errorf("said %q, want a bad character worded symbolically", got)
	}
}
