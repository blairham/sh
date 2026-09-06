// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// setFlagged parses src under a dialect with the set-test flag on and returns
// the first parameter expansion.
func setFlagged(t *testing.T, src string) *ParamExpr {
	t.Helper()
	d := Core()
	d.ParamSetTestFlag = true
	return firstParam(t, src, d)
}

// What the flag reads, and what name it may carry.
func TestParamSetTestFlagReadsANameOrAPositional(t *testing.T) {
	for _, tc := range []struct {
		src  string
		name string
	}{
		{`echo ${+x}`, "x"},
		{`echo ${+_x1}`, "_x1"},
		{`echo ${+PATH}`, "PATH"},
		{`echo ${+0}`, "0"},
		{`echo ${+1}`, "1"},
		{`echo ${+10}`, "10"},
	} {
		e := setFlagged(t, tc.src)
		if !e.SetTest || e.Name != tc.name || e.Bad {
			t.Errorf("%s: SetTest=%v name=%q bad=%v, want true %q false",
				tc.src, e.SetTest, e.Name, e.Bad, tc.name)
		}
		if e.Op != ParamNone || e.Length {
			t.Errorf("%s: op=%v length=%v, want a plain reference",
				tc.src, e.Op, e.Length)
		}
	}
	// And the plain expansion is untouched: nothing carries the flag by
	// accident.
	if e := setFlagged(t, `echo ${x}`); e.SetTest {
		t.Error("${x}: SetTest set on an expansion with no `+`")
	}
}

// Every parameter that is not a name or a positional is refused, which is the
// measured answer and not the one "is it set" suggests.
func TestParamSetTestFlagRefusesEverythingElse(t *testing.T) {
	for _, src := range []string{
		`echo ${+}`,   // nothing at all
		`echo ${+@}`,  //
		`echo ${+*}`,  //
		`echo ${+#}`,  //
		`echo ${+?}`,  //
		`echo ${+$}`,  //
		`echo ${+!}`,  //
		`echo ${+-}`,  //
		`echo ${++x}`, // not a doubled spelling: there is none
		`echo ${+#x}`, // a length may not follow it
		`echo ${+~x}`, // the tilde precedes it, never follows
		`echo ${+ x}`, // no space between the flag and the name
	} {
		if e := setFlagged(t, src); !e.Bad || e.Src == "" {
			t.Errorf("%s: read as %+v, want a bad substitution carrying Src", src, e)
		}
	}
}

// The refusal is deferred to the run wherever the dialect defers, which is
// every dialect that has this flag: `${+?}` in a branch never taken is no
// error at all. The parse-time dialects do not have the flag, and there a
// leading `+` is not a name, so the expansion is unreadable for the older
// reason and is still named by its token.
func TestParamSetTestFlagOffIsUnreadable(t *testing.T) {
	if e := firstParam(t, `echo ${+x}`, Core()); !e.Bad || e.Src != "+x" {
		t.Errorf(`${+x} in the core: %+v, want Bad with Src "+x"`, e)
	}
	d := Core()
	d.BadSubstitutionAtParseTime = true
	_, err := Parse(`echo ${+x}`, d)
	if err == nil {
		t.Fatal(`${+x} under a parse-time dialect: no error`)
	}
	if tokenOf(err) != "+" {
		t.Errorf("error = %v (token %q), want the token `+` named", err, tokenOf(err))
	}
}

// The subscript is still read after the flag — `${+functions[f]}` is the
// idiom the construct is reached for — and so is an operator, which the
// interpreter then answers with the value rather than the count.
func TestParamSetTestFlagKeepsTheRestOfTheGrammar(t *testing.T) {
	d := Core()
	d.ParamSetTestFlag = true
	d.ArraySubscript = true
	e := firstParam(t, `echo ${+a[2]}`, d)
	if !e.SetTest || e.Name != "a" || e.Index == nil {
		t.Errorf("${+a[2]}: SetTest=%v name=%q index=%v, want true \"a\" a subscript",
			e.SetTest, e.Name, e.Index)
	}
	for _, tc := range []struct {
		src string
		op  ParamOp
	}{
		{`echo ${+x#p}`, ParamTrimPrefix},
		{`echo ${+x:-d}`, ParamDefault},
		{`echo ${+x=d}`, ParamAssign},
	} {
		e := firstParam(t, tc.src, d)
		if !e.SetTest || e.Op != tc.op || e.Bad {
			t.Errorf("%s: SetTest=%v op=%v bad=%v, want true %v false",
				tc.src, e.SetTest, e.Op, e.Bad, tc.op)
		}
	}
}

// The tilde run comes first and the `+` second, and the flag group before
// both. That order is measured; the reverses are not shapes at all.
func TestParamSetTestFlagSitsAfterTheTildeAndTheGroup(t *testing.T) {
	d := Core()
	d.ParamSetTestFlag = true
	d.ParamTildeFlag = true
	d.ParamSplitFlag = true
	d.ParamExpansionFlags = true
	e := firstParam(t, `echo ${~+x}`, d)
	if !e.SetTest || e.TildeFlags != 1 || e.Name != "x" {
		t.Errorf(`${~+x}: SetTest=%v tildes=%d name=%q, want true 1 "x"`,
			e.SetTest, e.TildeFlags, e.Name)
	}
	e = firstParam(t, `echo ${(U)+x}`, d)
	if !e.SetTest || e.Flags != "U" || e.Name != "x" {
		t.Errorf(`${(U)+x}: SetTest=%v flags=%q name=%q, want true "U" "x"`,
			e.SetTest, e.Flags, e.Name)
	}
	// The `=` flag shares the tilde's slot, so it precedes the `+` as well,
	// and the two are interchangeable within the slot.
	for _, src := range []string{`echo ${=+x}`, `echo ${~=+x}`, `echo ${=~+x}`} {
		if e := firstParam(t, src, d); !e.SetTest || e.Name != "x" || e.Bad {
			t.Errorf("%s: SetTest=%v name=%q bad=%v, want true \"x\" false",
				src, e.SetTest, e.Name, e.Bad)
		}
	}
	// A tilde, an equals or a group written *after* the `+` leaves no name.
	for _, src := range []string{`echo ${+~x}`, `echo ${+=x}`, `echo ${+(U)x}`} {
		if e := firstParam(t, src, d); !e.Bad {
			t.Errorf("%s: read as %+v, want a bad substitution", src, e)
		}
	}
	// And the flag does not relax the empty name the way the tilde and the
	// group do: `${~}` and `${(U)}` read, `${~+}` and `${(U)+}` do not.
	for _, src := range []string{`echo ${~+}`, `echo ${(U)+}`} {
		if e := firstParam(t, src, d); !e.Bad {
			t.Errorf("%s: read as %+v, want a bad substitution", src, e)
		}
	}
	if e := firstParam(t, `echo ${~}`, d); e.Bad {
		t.Error("${~}: read as a bad substitution, want the empty name")
	}
}

// `${#+x}` is not this construct: the `#` is read first, so what follows is
// the length's business and the flag never reaches it.
func TestParamSetTestFlagIsNotTheLengthsAlternate(t *testing.T) {
	d := Core()
	d.ParamSetTestFlag = true
	e := firstParam(t, `echo ${#+x}`, d)
	if e.SetTest {
		t.Errorf("${#+x}: SetTest set, want the `#` read first (%+v)", e)
	}
}

// The span round-trips as written, because the printer writes it back raw.
func TestParamSetTestFlagRoundTrips(t *testing.T) {
	d := Core()
	d.ParamSetTestFlag = true
	d.ArraySubscript = true
	for _, src := range []string{
		"echo ${+x}", "echo ${+0}", "echo ${+a[2]}", "echo x${+x}y",
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
