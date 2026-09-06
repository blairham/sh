// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// tildeFlagged parses src under a dialect with the tilde flag on and returns
// the first parameter expansion.
func tildeFlagged(t *testing.T, src string) *ParamExpr {
	t.Helper()
	d := Core()
	d.ParamTildeFlag = true
	return firstParam(t, src, d)
}

// The count is what is read, not a bool, because parity is the meaning.
func TestParamTildeFlagCounts(t *testing.T) {
	for _, tc := range []struct {
		src   string
		count int
		name  string
	}{
		{`echo ${x}`, 0, "x"},
		{`echo ${~x}`, 1, "x"},
		{`echo ${~~x}`, 2, "x"},
		{`echo ${~~~x}`, 3, "x"},
		{`echo ${~~~~x}`, 4, "x"},
		{`echo ${~}`, 1, ""},
		{`echo ${~~}`, 2, ""},
		{`echo ${~@}`, 1, "@"},
		{`echo ${~1}`, 1, "1"},
	} {
		e := tildeFlagged(t, tc.src)
		if e.TildeFlags != tc.count || e.Name != tc.name {
			t.Errorf("%s: TildeFlags=%d name=%q, want %d %q",
				tc.src, e.TildeFlags, e.Name, tc.count, tc.name)
		}
		if e.Bad {
			t.Errorf("%s: read as a bad substitution", tc.src)
		}
	}
}

// The operator and the subscript are still read after the tildes: the flag is
// a prefix on the expansion and not a different expansion.
func TestParamTildeFlagKeepsTheRestOfTheGrammar(t *testing.T) {
	d := Core()
	d.ParamTildeFlag = true
	d.ArraySubscript = true
	for _, tc := range []struct {
		src    string
		count  int
		name   string
		op     ParamOp
		length bool
	}{
		{`echo ${~x#p}`, 1, "x", ParamTrimPrefix, false},
		{`echo ${~x:-d}`, 1, "x", ParamDefault, false},
		{`echo ${~#x}`, 1, "x", ParamNone, true},
		{`echo ${~~#x}`, 2, "x", ParamNone, true},
		{`echo ${~a[1]}`, 1, "a", ParamNone, false},
	} {
		e := firstParam(t, tc.src, d)
		if e.TildeFlags != tc.count || e.Name != tc.name ||
			e.Op != tc.op || e.Length != tc.length {
			t.Errorf("%s: tildes=%d name=%q op=%v length=%v, want %d %q %v %v",
				tc.src, e.TildeFlags, e.Name, e.Op, e.Length,
				tc.count, tc.name, tc.op, tc.length)
		}
	}
	if e := firstParam(t, `echo ${~a[1]}`, d); e.Index == nil {
		t.Error("${~a[1]}: the subscript was not read")
	}
}

// The flag group comes first and the tildes second. That order is measured,
// and the reverse is not a shape at all.
func TestParamTildeFlagFollowsTheFlagGroup(t *testing.T) {
	d := Core()
	d.ParamTildeFlag = true
	d.ParamExpansionFlags = true
	e := firstParam(t, `echo ${(U)~x}`, d)
	if e.Flags != "U" || e.TildeFlags != 1 || e.Name != "x" {
		t.Errorf(`${(U)~x}: flags=%q tildes=%d name=%q, want "U" 1 "x"`,
			e.Flags, e.TildeFlags, e.Name)
	}
	// A group after the tildes leaves no name, which is the bad substitution
	// the shell reports for it.
	if e := firstParam(t, `echo ${~(U)x}`, d); !e.Bad {
		t.Errorf(`${~(U)x}: read as %+v, want a bad substitution`, e)
	}
	// And a `#` in front of the tildes is not the length: `${#~g}` is a bad
	// substitution where `${~#g}` is a count.
	if e := firstParam(t, `echo ${#~g}`, d); !e.Bad {
		t.Errorf(`${#~g}: read as %+v, want a bad substitution`, e)
	}
}

// Without the flag a leading tilde is not a name, so the expansion is
// unreadable — and *which* way it is unreadable is the split
// BadSubstitutionAtParseTime already records.
func TestParamTildeFlagOffIsUnreadable(t *testing.T) {
	if e := firstParam(t, `echo ${~x}`, Core()); !e.Bad || e.Src != "~x" {
		t.Errorf(`${~x} in the core: %+v, want Bad with Src "~x"`, e)
	}
	d := Core()
	d.BadSubstitutionAtParseTime = true
	_, err := Parse(`echo ${~x}`, d)
	if err == nil {
		t.Fatal(`${~x} under a parse-time dialect: no error`)
	}
	pe, ok := err.(*Error)
	if !ok || pe.Token != "~" {
		t.Errorf("error = %v (token %q), want the token `~` named", err, tokenOf(err))
	}
}

// tokenOf is the token an *Error names, or "" for anything else.
func tokenOf(err error) string {
	if pe, ok := err.(*Error); ok {
		return pe.Token
	}
	return ""
}

// The span round-trips as written, because the printer writes it back raw.
func TestParamTildeFlagRoundTrips(t *testing.T) {
	d := Core()
	d.ParamTildeFlag = true
	for _, src := range []string{
		"echo ${~x}", "echo ${~~x}", "echo ${~x#p}", "echo x${~x}y",
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
