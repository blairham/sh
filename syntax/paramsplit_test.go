// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// splitFlagged parses src under a dialect with the split flag on and returns
// the first parameter expansion.
func splitFlagged(t *testing.T, src string) *ParamExpr {
	t.Helper()
	d := Core()
	d.ParamSplitFlag = true
	return firstParam(t, src, d)
}

// The count is what is read, not a bool, because parity is the meaning.
func TestParamSplitFlagCounts(t *testing.T) {
	for _, tc := range []struct {
		src   string
		count int
		name  string
	}{
		{`echo ${x}`, 0, "x"},
		{`echo ${=x}`, 1, "x"},
		{`echo ${==x}`, 2, "x"},
		{`echo ${===x}`, 3, "x"},
		{`echo ${====x}`, 4, "x"},
		{`echo ${=}`, 1, ""},
		{`echo ${==}`, 2, ""},
		{`echo ${=@}`, 1, "@"},
		{`echo ${=1}`, 1, "1"},
	} {
		e := splitFlagged(t, tc.src)
		if e.SplitFlags != tc.count || e.Name != tc.name {
			t.Errorf("%s: SplitFlags=%d name=%q, want %d %q",
				tc.src, e.SplitFlags, e.Name, tc.count, tc.name)
		}
		if e.Bad {
			t.Errorf("%s: read as a bad substitution", tc.src)
		}
	}
}

// The operator and the subscript are still read after the run: the flag is a
// prefix on the expansion and not a different expansion.
func TestParamSplitFlagKeepsTheRestOfTheGrammar(t *testing.T) {
	d := Core()
	d.ParamSplitFlag = true
	d.ArraySubscript = true
	for _, tc := range []struct {
		src    string
		count  int
		name   string
		op     ParamOp
		length bool
	}{
		{`echo ${=x#p}`, 1, "x", ParamTrimPrefix, false},
		{`echo ${=x:-d}`, 1, "x", ParamDefault, false},
		// The one pairing that could have collided, and does not: the `=`
		// in front is the flag and the `=` behind the name is still the
		// assignment operator.
		{`echo ${=x=d}`, 1, "x", ParamAssign, false},
		{`echo ${=#x}`, 1, "x", ParamNone, true},
		{`echo ${==#x}`, 2, "x", ParamNone, true},
		{`echo ${=a[1]}`, 1, "a", ParamNone, false},
	} {
		e := firstParam(t, tc.src, d)
		if e.SplitFlags != tc.count || e.Name != tc.name ||
			e.Op != tc.op || e.Length != tc.length {
			t.Errorf("%s: splits=%d name=%q op=%v length=%v, want %d %q %v %v",
				tc.src, e.SplitFlags, e.Name, e.Op, e.Length,
				tc.count, tc.name, tc.op, tc.length)
		}
	}
	if e := firstParam(t, `echo ${=a[1]}`, d); e.Index == nil {
		t.Error("${=a[1]}: the subscript was not read")
	}
}

// The two flag characters share one slot and are interchangeable within it,
// which is measured — and each is counted on its own, because parity is per
// flag and not per character.
func TestParamSplitFlagSharesTheSlotWithTheTilde(t *testing.T) {
	d := Core()
	d.ParamSplitFlag = true
	d.ParamTildeFlag = true
	for _, tc := range []struct {
		src            string
		splits, tildes int
		name           string
	}{
		{`echo ${=~x}`, 1, 1, "x"},
		{`echo ${~=x}`, 1, 1, "x"},
		{`echo ${=~=x}`, 2, 1, "x"},
		{`echo ${~=~x}`, 1, 2, "x"},
	} {
		e := firstParam(t, tc.src, d)
		if e.SplitFlags != tc.splits || e.TildeFlags != tc.tildes || e.Name != tc.name {
			t.Errorf("%s: splits=%d tildes=%d name=%q, want %d %d %q",
				tc.src, e.SplitFlags, e.TildeFlags, e.Name,
				tc.splits, tc.tildes, tc.name)
		}
	}
}

// The flag group comes first and the run second, and the `#` behind it is
// still the length. That order is measured, and the reverse is a different
// expansion rather than a different order.
func TestParamSplitFlagFollowsTheFlagGroup(t *testing.T) {
	d := Core()
	d.ParamSplitFlag = true
	d.ParamExpansionFlags = true
	e := firstParam(t, `echo ${(U)=x}`, d)
	if e.Flags != "U" || e.SplitFlags != 1 || e.Name != "x" {
		t.Errorf(`${(U)=x}: flags=%q splits=%d name=%q, want "U" 1 "x"`,
			e.Flags, e.SplitFlags, e.Name)
	}
	// A group after the run leaves no name, which is the bad substitution
	// the shell reports for it.
	if e := firstParam(t, `echo ${=(U)x}`, d); !e.Bad {
		t.Errorf(`${=(U)x}: read as %+v, want a bad substitution`, e)
	}
	// And an `=` behind the `#` is not the flag: `${#=word}` is the
	// assignment on `$#` in the shell that has the construct, so the run is
	// read in front of the `#` and nothing behind it is a flag. That
	// expansion is a gap here either way — the point of the row is that the
	// flag did not silently claim it.
	e = firstParam(t, `echo ${#=x}`, d)
	if e.SplitFlags != 0 {
		t.Errorf(`${#=x}: SplitFlags=%d, want 0 — the run precedes the "#"`, e.SplitFlags)
	}
}

// Without the flag a leading `=` is not a name, so the expansion is
// unreadable — and *which* way it is unreadable is the split
// BadSubstitutionAtParseTime already records.
func TestParamSplitFlagOffIsUnreadable(t *testing.T) {
	if e := firstParam(t, `echo ${=x}`, Core()); !e.Bad || e.Src != "=x" {
		t.Errorf(`${=x} in the core: %+v, want Bad with Src "=x"`, e)
	}
	d := Core()
	d.BadSubstitutionAtParseTime = true
	_, err := Parse(`echo ${=x}`, d)
	if err == nil {
		t.Fatal(`${=x} under a parse-time dialect: no error`)
	}
	pe, ok := err.(*Error)
	if !ok || pe.Token != "=" {
		t.Errorf("error = %v (token %q), want the token `=` named", err, tokenOf(err))
	}
}

// The span round-trips as written, because the printer writes it back raw.
func TestParamSplitFlagRoundTrips(t *testing.T) {
	d := Core()
	d.ParamSplitFlag = true
	d.ParamTildeFlag = true
	for _, src := range []string{
		"echo ${=x}", "echo ${==x}", "echo ${=x#p}", "echo x${=x}y",
		"echo ${=~x}", "echo ${~=x}",
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
