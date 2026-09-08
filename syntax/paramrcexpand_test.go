// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// rcExpandFlagged parses src under a dialect with the rc-expand flag on and
// returns the first parameter expansion.
func rcExpandFlagged(t *testing.T, src string) *ParamExpr {
	t.Helper()
	d := Core()
	d.ParamRcExpandFlag = true
	return firstParam(t, src, d)
}

// The count is what is read, not a bool, because parity is the meaning.
func TestParamRcExpandFlagCounts(t *testing.T) {
	for _, tc := range []struct {
		src   string
		count int
		name  string
	}{
		{`echo ${x}`, 0, "x"},
		{`echo ${^x}`, 1, "x"},
		{`echo ${^^x}`, 2, "x"},
		{`echo ${^^^x}`, 3, "x"},
		{`echo ${^^^^x}`, 4, "x"},
		{`echo ${^}`, 1, ""},
		{`echo ${^^}`, 2, ""},
		{`echo ${^@}`, 1, "@"},
		{`echo ${^1}`, 1, "1"},
	} {
		e := rcExpandFlagged(t, tc.src)
		if e.RcExpandFlags != tc.count || e.Name != tc.name {
			t.Errorf("%s: RcExpandFlags=%d name=%q, want %d %q",
				tc.src, e.RcExpandFlags, e.Name, tc.count, tc.name)
		}
		if e.Bad {
			t.Errorf("%s: read as a bad substitution", tc.src)
		}
	}
}

// The operator and the subscript are still read after the run: the flag is a
// prefix on the expansion and not a different expansion.
func TestParamRcExpandFlagKeepsTheRestOfTheGrammar(t *testing.T) {
	d := Core()
	d.ParamRcExpandFlag = true
	d.ArraySubscript = true
	for _, tc := range []struct {
		src    string
		count  int
		name   string
		op     ParamOp
		length bool
	}{
		{`echo ${^x#p}`, 1, "x", ParamTrimPrefix, false},
		{`echo ${^x:-d}`, 1, "x", ParamDefault, false},
		{`echo ${^x=d}`, 1, "x", ParamAssign, false},
		{`echo ${^#x}`, 1, "x", ParamNone, true},
		{`echo ${^^#x}`, 2, "x", ParamNone, true},
		{`echo ${^a[1]}`, 1, "a", ParamNone, false},
	} {
		e := firstParam(t, tc.src, d)
		if e.RcExpandFlags != tc.count || e.Name != tc.name ||
			e.Op != tc.op || e.Length != tc.length {
			t.Errorf("%s: spreads=%d name=%q op=%v length=%v, want %d %q %v %v",
				tc.src, e.RcExpandFlags, e.Name, e.Op, e.Length,
				tc.count, tc.name, tc.op, tc.length)
		}
	}
	if e := firstParam(t, `echo ${^a[1]}`, d); e.Index == nil {
		t.Error("${^a[1]}: the subscript was not read")
	}
	// A `#` in front of the run is not this flag: `${#^x}` is a bad
	// substitution in the shell that has the construct, because the run is
	// read in front of the `#` and nothing behind it is a flag.
	if e := firstParam(t, `echo ${#^x}`, d); e.RcExpandFlags != 0 {
		t.Errorf(`${#^x}: RcExpandFlags=%d, want 0 — the run precedes the "#"`,
			e.RcExpandFlags)
	}
}

// The three flag characters share one slot and are interchangeable within it,
// which is measured — and each is counted on its own, because parity is per
// flag and not per character.
func TestParamRcExpandFlagSharesTheSlot(t *testing.T) {
	d := Core()
	d.ParamRcExpandFlag = true
	d.ParamSplitFlag = true
	d.ParamTildeFlag = true
	for _, tc := range []struct {
		src                     string
		spreads, splits, tildes int
		name                    string
	}{
		{`echo ${^=x}`, 1, 1, 0, "x"},
		{`echo ${=^x}`, 1, 1, 0, "x"},
		{`echo ${^~x}`, 1, 0, 1, "x"},
		{`echo ${~^x}`, 1, 0, 1, "x"},
		{`echo ${^~=^x}`, 2, 1, 1, "x"},
		{`echo ${=~^x}`, 1, 1, 1, "x"},
	} {
		e := firstParam(t, tc.src, d)
		if e.RcExpandFlags != tc.spreads || e.SplitFlags != tc.splits ||
			e.TildeFlags != tc.tildes || e.Name != tc.name {
			t.Errorf("%s: spreads=%d splits=%d tildes=%d name=%q, want %d %d %d %q",
				tc.src, e.RcExpandFlags, e.SplitFlags, e.TildeFlags, e.Name,
				tc.spreads, tc.splits, tc.tildes, tc.name)
		}
	}
}

// The flag group comes first and the run second, and the set test behind it
// is still read. That order is measured, and the reverse is a bad
// substitution rather than a different order.
func TestParamRcExpandFlagFollowsTheFlagGroup(t *testing.T) {
	d := Core()
	d.ParamRcExpandFlag = true
	d.ParamExpansionFlags = true
	d.ParamSetTestFlag = true
	e := firstParam(t, `echo ${(U)^x}`, d)
	if e.Flags != "U" || e.RcExpandFlags != 1 || e.Name != "x" {
		t.Errorf(`${(U)^x}: flags=%q spreads=%d name=%q, want "U" 1 "x"`,
			e.Flags, e.RcExpandFlags, e.Name)
	}
	// A group after the run leaves no name, which is the bad substitution
	// the shell reports for it.
	if e := firstParam(t, `echo ${^(U)x}`, d); !e.Bad {
		t.Errorf(`${^(U)x}: read as %+v, want a bad substitution`, e)
	}
	// The set test stands behind the run and not in front of it, measured:
	// `${^+x}` reads and `${+^x}` does not.
	if e := firstParam(t, `echo ${^+x}`, d); e.RcExpandFlags != 1 || !e.SetTest || e.Name != "x" {
		t.Errorf(`${^+x}: spreads=%d settest=%v name=%q, want 1 true "x"`,
			e.RcExpandFlags, e.SetTest, e.Name)
	}
	if e := firstParam(t, `echo ${+^x}`, d); !e.Bad {
		t.Errorf(`${+^x}: read as %+v, want a bad substitution`, e)
	}
}

// Without the flag a leading `^` is not a name, so the expansion is
// unreadable — and *which* way it is unreadable is the split
// BadSubstitutionAtParseTime already records.
func TestParamRcExpandFlagOffIsUnreadable(t *testing.T) {
	if e := firstParam(t, `echo ${^x}`, Core()); !e.Bad || e.Src != "^x" {
		t.Errorf(`${^x} in the core: %+v, want Bad with Src "^x"`, e)
	}
	d := Core()
	d.BadSubstitutionAtParseTime = true
	_, err := Parse(`echo ${^x}`, d)
	if err == nil {
		t.Fatal(`${^x} under a parse-time dialect: no error`)
	}
	pe, ok := err.(*Error)
	if !ok || pe.Token != "^" {
		t.Errorf("error = %v (token %q), want the token `^` named", err, tokenOf(err))
	}
}

// The case-conversion `^` of another grammar follows the name, so the two
// cannot be confused: with only that dialect's flag on, `${x^}` is the case
// change and `${^x}` is unreadable.
func TestParamRcExpandFlagDoesNotCollideWithCaseChange(t *testing.T) {
	d := Core()
	d.ParamCaseChange = true
	e := firstParam(t, `echo ${x^}`, d)
	if e.RcExpandFlags != 0 || e.Name != "x" {
		t.Errorf(`${x^}: spreads=%d name=%q, want 0 "x"`, e.RcExpandFlags, e.Name)
	}
	if e := firstParam(t, `echo ${^x}`, d); !e.Bad {
		t.Errorf(`${^x} without the flag: read as %+v, want a bad substitution`, e)
	}
}

// The span round-trips as written, because the printer writes it back raw.
func TestParamRcExpandFlagRoundTrips(t *testing.T) {
	d := Core()
	d.ParamRcExpandFlag = true
	d.ParamSplitFlag = true
	d.ParamTildeFlag = true
	for _, src := range []string{
		"echo ${^x}", "echo ${^^x}", "echo ${^x#p}", "echo x${^x}y",
		"echo ${^=x}", "echo ${~^x}",
	} {
		f, err := Parse(src, d)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		if got := Print(f); got != src {
			t.Errorf("round trip of %q = %q", src, got)
		}
	}
}
