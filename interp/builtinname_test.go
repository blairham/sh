// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// nameRun runs src with the panel's shared answer — a bad name is refused and
// is not fatal — so a test that cares about neither says neither.
func nameRun(t *testing.T, tweak func(*Semantics), dg Diagnostics, src string) (string, int) {
	t.Helper()
	return optRun(t, func(s *Semantics) {
		s.BadNameToDeclarationFatal = No
		s.BadNameToUnsetFatal = No
		if tweak != nil {
			tweak(s)
		}
	}, dg, src)
}

// The bug: an operand that is not a name was taken without a word, so
// `export -` exported a variable called `-` and reported success. Three of the
// four refuse it; the fourth refuses a different set, not none.
func TestAnOperandThatIsNotANameIsRefused(t *testing.T) {
	for _, builtin := range []string{"export", "readonly", "unset"} {
		for _, operand := range []string{"-", "1x", "a-b", "a b", "a.b", ""} {
			src := builtin + ` -- "` + operand + `"; echo "st=$?"`
			out, _ := nameRun(t, nil, Diagnostics{}, src)
			if !strings.Contains(out, "st=1") {
				t.Errorf("%s %q: said %q, want status 1", builtin, operand, out)
			}
			if !strings.Contains(out, builtin) {
				t.Errorf("%s %q: said %q, want the builtin named", builtin, operand, out)
			}
		}
	}
}

// A name still works, which is the half a refusal could break.
func TestTheNamesThatAreNamesStillWork(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`export _a1=2; echo "[$_a1]"`, "[2]"},
		{`readonly w=3; echo "[$w]"`, "[3]"},
		{`x=1; export x; unset x; echo "[${x-gone}]"`, "[gone]"},
		{`export a=1; echo "st=$?"`, "st=0"},
		// A subscripted operand is left alone: it splits the panel four ways
		// and is a question about arrays rather than about what a name is.
		{`unset "a[0]"; echo "st=$?"`, "st=0"},
	} {
		if out, _ := nameRun(t, nil, Diagnostics{}, c.src); !strings.Contains(out, c.want) {
			t.Errorf("%s: said %q, want %q", c.src, out, c.want)
		}
	}
}

// bash reports every bad operand and exports the ones that were names, rather
// than stopping at the first. The other three print one line because the first
// is fatal there, which falls out of that rather than needing its own rule.
func TestEveryBadOperandIsReportedAndTheGoodOnesAreKept(t *testing.T) {
	out, _ := nameRun(t, nil, Diagnostics{}, `export a=1 1x b=2 2y; echo "[$a][$b]"`)
	if n := strings.Count(out, "not a valid identifier"); n != 2 {
		t.Errorf("said %q, want two complaints, got %d", out, n)
	}
	if !strings.Contains(out, "1x") || !strings.Contains(out, "2y") {
		t.Errorf("said %q, want both bad operands named", out)
	}
	if !strings.Contains(out, "[1][2]") {
		t.Errorf("said %q, want the well-formed operands exported", out)
	}
}

// Where the dialect says so the script stops there, and `unset` is asked
// separately because ksh93 answers the two differently.
func TestABadNameCanEndTheScript(t *testing.T) {
	for _, c := range []struct {
		name  string
		tweak func(*Semantics)
		src   string
		stops bool
	}{
		{"export fatal", func(s *Semantics) { s.BadNameToDeclarationFatal = Yes }, `export 1x; echo after`, true},
		{"export not fatal", func(s *Semantics) { s.BadNameToDeclarationFatal = No }, `export 1x; echo after`, false},
		{"unset fatal", func(s *Semantics) { s.BadNameToUnsetFatal = Yes }, `unset 1x; echo after`, true},
		{"unset not fatal", func(s *Semantics) { s.BadNameToUnsetFatal = No }, `unset 1x; echo after`, false},
		// The split ksh93 has: `export` ends the script and `unset` does not.
		{"split, export", func(s *Semantics) {
			s.BadNameToDeclarationFatal, s.BadNameToUnsetFatal = Yes, No
		}, `export 1x; echo after`, true},
		{"split, unset", func(s *Semantics) {
			s.BadNameToDeclarationFatal, s.BadNameToUnsetFatal = Yes, No
		}, `unset 1x; echo after`, false},
	} {
		out, _ := nameRun(t, c.tweak, Diagnostics{}, c.src)
		if stopped := !strings.Contains(out, "after"); stopped != c.stops {
			t.Errorf("%s: said %q, want stops=%v", c.name, out, c.stops)
		}
	}
}

// What may stand where a name is wanted is per builtin, because zsh answers
// the two with sets that overlap only at `0`.
func TestWhatElseMayStandWhereANameIsWanted(t *testing.T) {
	for _, c := range []struct {
		name    string
		takes   NameOperands
		operand string
		refused bool
	}{
		{"plain refuses a special parameter", PlainNamesOnly, "?", true},
		{"plain refuses a positional", PlainNamesOnly, "12", true},
		{"specials take a special parameter", NamesAndSpecialParameters, "?", false},
		{"specials take the dash", NamesAndSpecialParameters, "-", false},
		{"specials take zero", NamesAndSpecialParameters, "0", false},
		// The digit that is not a special parameter: `export 0` is quiet in
		// zsh and `export 12` is not.
		{"specials refuse a positional", NamesAndSpecialParameters, "12", true},
		{"positionals take a positional", NamesAndPositionals, "12", false},
		{"positionals take zero", NamesAndPositionals, "0", false},
		// And the other direction: zsh's `unset` refuses what its `export`
		// takes.
		{"positionals refuse a special parameter", NamesAndPositionals, "?", true},
		// Neither set weakens what a name is.
		{"specials still refuse a non-name", NamesAndSpecialParameters, "a-b", true},
		{"positionals still refuse a non-name", NamesAndPositionals, "1x", true},
	} {
		out, _ := nameRun(t, func(s *Semantics) {
			s.DeclarationNameOperands, s.UnsetNameOperands = c.takes, c.takes
		}, Diagnostics{}, `export -- "`+c.operand+`"; echo "st=$?"`)
		if refused := !strings.Contains(out, "st=0"); refused != c.refused {
			t.Errorf("%s (%q): said %q, want refused=%v", c.name, c.operand, out, c.refused)
		}
	}
}

// The wording is per builtin, and the two dialects that split it split it in
// different places — ksh93 between `export` and the other two, zsh between
// `unset` and the other two.
func TestTheComplaintIsWordedPerBuiltin(t *testing.T) {
	dg := Diagnostics{BuiltinBadName: map[string]string{
		"export":   "%[1]s: %[2]s: is not an identifier",
		"readonly": "%[1]s: %[2]s: invalid variable name",
		"unset":    "%[2]s: invalid parameter name",
	}}
	for _, c := range []struct{ src, want string }{
		{`export 1x`, "export: 1x: is not an identifier"},
		{`readonly 1x`, "readonly: 1x: invalid variable name"},
		{`unset 1x`, "1x: invalid parameter name"},
	} {
		if out, _ := nameRun(t, nil, dg, c.src); !strings.Contains(out, c.want) {
			t.Errorf("%s: said %q, want %q", c.src, out, c.want)
		}
	}
	// A builtin with no entry falls back rather than borrowing another's.
	out, _ := nameRun(t, nil, Diagnostics{BuiltinBadName: map[string]string{
		"export": "%[1]s: %[2]s: is not an identifier",
	}}, `unset 1x`)
	if strings.Contains(out, "is not an identifier") {
		t.Errorf("said %q, want a builtin with no entry not to borrow another's", out)
	}
}

// An operand that starts with a digit is a different complaint in the one
// dialect that tells the two apart.
func TestALeadingDigitCanBeADifferentComplaint(t *testing.T) {
	dg := Diagnostics{
		BuiltinBadName:        map[string]string{"export": "not valid in this context: %[2]s"},
		BuiltinBadNameNumeric: map[string]string{"export": "not an identifier: %[2]s"},
	}
	if out, _ := nameRun(t, nil, dg, `export 1x`); !strings.Contains(out, "not an identifier: 1x") {
		t.Errorf("said %q, want the numeric wording", out)
	}
	if out, _ := nameRun(t, nil, dg, `export a-b`); !strings.Contains(out, "not valid in this context: a-b") {
		t.Errorf("said %q, want the ordinary wording", out)
	}
	// `1x` is a leading digit and `a-b` is not, which is the whole rule — a
	// digit anywhere else does not reach for the second wording.
	if out, _ := nameRun(t, nil, dg, `export a-1`); !strings.Contains(out, "not valid in this context: a-1") {
		t.Errorf("said %q, want a digit that does not lead to use the ordinary wording", out)
	}
	// With no numeric entry, both get the ordinary one.
	plain := Diagnostics{BuiltinBadName: map[string]string{"export": "not valid in this context: %[2]s"}}
	if out, _ := nameRun(t, nil, plain, `export 1x`); !strings.Contains(out, "not valid in this context: 1x") {
		t.Errorf("said %q, want the ordinary wording where there is no numeric one", out)
	}
}

// Two of the four quote back the whole operand and two name only the part in
// front of the `=`.
func TestTheOperandIsQuotedBackWholeOrNamedInPart(t *testing.T) {
	dg := Diagnostics{BuiltinBadName: map[string]string{"export": "%[1]s: %[2]s: bad"}}
	out, _ := nameRun(t, nil, dg, `export 1x=v`)
	if !strings.Contains(out, "export: 1x: bad") {
		t.Errorf("said %q, want the name alone", out)
	}
	if strings.Contains(out, "1x=v") {
		t.Errorf("said %q, want the value left out", out)
	}
	dg.BuiltinBadNameKeepsValue = true
	out, _ = nameRun(t, nil, dg, `export 1x=v`)
	if !strings.Contains(out, "export: 1x=v: bad") {
		t.Errorf("said %q, want the operand as written", out)
	}
}

// The status is the dialect's, and dash is the one that says 2.
func TestTheStatusIsTheDialects(t *testing.T) {
	if out, _ := nameRun(t, nil, Diagnostics{}, `export 1x; echo "st=$?"`); !strings.Contains(out, "st=1") {
		t.Errorf("said %q, want the default status of 1", out)
	}
	dg := Diagnostics{BuiltinBadNameStatus: 2}
	if out, _ := nameRun(t, nil, dg, `export 1x; echo "st=$?"`); !strings.Contains(out, "st=2") {
		t.Errorf("said %q, want the dialect's status", out)
	}
}

// A function name is not a variable name, and `unset -f` keeps its own laxer
// rule: bash takes `unset -f 1x` without a word where it refuses `unset -v 1x`.
func TestUnsetFKeepsItsOwnRule(t *testing.T) {
	out, _ := nameRun(t, nil, Diagnostics{}, `unset -f 1x; echo "st=$?"`)
	if !strings.Contains(out, "st=0") {
		t.Errorf("said %q, want `unset -f` to take a name a variable could not have", out)
	}
	if strings.Contains(out, "identifier") {
		t.Errorf("said %q, want no complaint about a function name", out)
	}
}

// An operand that is only spaces around a name is not a name — all four
// refuse `export " a "`, and a name check that trimmed first would take it.
func TestSurroundingSpaceIsNotTrimmedAway(t *testing.T) {
	for _, operand := range []string{" a ", " a", "a ", " "} {
		src := `export -- "` + operand + `"; echo "st=$?"`
		if out, _ := nameRun(t, nil, Diagnostics{}, src); !strings.Contains(out, "st=1") {
			t.Errorf("export %q: said %q, want it refused", operand, out)
		}
	}
	// And a name is still a name.
	if out, _ := nameRun(t, nil, Diagnostics{}, `export -- a; echo "st=$?"`); !strings.Contains(out, "st=0") {
		t.Errorf("said %q, want a plain name taken", out)
	}
}

// The empty operand is refused under every answer, including the one that
// takes any run of digits — an empty string is not a run of digits, and
// nothing but a test says so.
func TestTheEmptyOperandIsNeverAName(t *testing.T) {
	for _, takes := range []NameOperands{PlainNamesOnly, NamesAndSpecialParameters, NamesAndPositionals} {
		out, _ := nameRun(t, func(s *Semantics) {
			s.DeclarationNameOperands, s.UnsetNameOperands = takes, takes
		}, Diagnostics{}, `unset -- ""; echo "st=$?"`)
		if !strings.Contains(out, "st=1") {
			t.Errorf("%v: said %q, want the empty operand refused", takes, out)
		}
	}
}

// Where a bad name ends the script it ends it at the *first* one: dash and zsh
// print one line for `export 1x 2y` where bash prints two.
func TestAFatalBadNameStopsAtTheFirst(t *testing.T) {
	out, _ := nameRun(t, func(s *Semantics) { s.BadNameToDeclarationFatal = Yes },
		Diagnostics{}, `export 1x 2y; echo after`)
	if n := strings.Count(out, "not a valid identifier"); n != 1 {
		t.Errorf("said %q, want one complaint, got %d", out, n)
	}
	if strings.Contains(out, "2y") {
		t.Errorf("said %q, want the second operand never reached", out)
	}
	if strings.Contains(out, "after") {
		t.Errorf("said %q, want the script stopped", out)
	}
}

// A dialect may check nothing at all where it wants a name, and `unset -v`
// still gets a name checked — which is the only thing that tells the loose
// answer from no validation having been written.
func TestABareUnsetMayCheckNothingWhereUnsetVStillDoes(t *testing.T) {
	loose := func(s *Semantics) { s.UnsetNameOperands = AnythingIsAName }
	for _, c := range []struct {
		src     string
		refused bool
	}{
		{`unset 1x; echo "st=$?"`, false},
		{`unset "a b"; echo "st=$?"`, false},
		{`unset -- -; echo "st=$?"`, false},
		// The `-v` that asks for a variable gets a name.
		{`unset -v 1x; echo "st=$?"`, true},
		{`unset -v "a b"; echo "st=$?"`, true},
		// And a name is still a name either way.
		{`unset -v a1; echo "st=$?"`, false},
	} {
		out, _ := nameRun(t, loose, Diagnostics{}, c.src)
		if refused := !strings.Contains(out, "st=0"); refused != c.refused {
			t.Errorf("%s: said %q, want refused=%v", c.src, out, c.refused)
		}
	}
	// The loose answer belongs to `unset` alone: it must not leak into the
	// declarations, which is what one shared field would have done.
	out, _ := nameRun(t, loose, Diagnostics{}, `export 1x; echo "st=$?"`)
	if !strings.Contains(out, "st=1") {
		t.Errorf("said %q, want `export` to keep checking", out)
	}
	// And `-v` does not tighten a dialect that was never loose: zsh's `unset
	// -v 12` is quiet, like its bare form.
	out, _ = nameRun(t, func(s *Semantics) { s.UnsetNameOperands = NamesAndPositionals },
		Diagnostics{}, `unset -v 12; echo "st=$?"`)
	if !strings.Contains(out, "st=0") {
		t.Errorf("said %q, want -v to leave a dialect's own set alone", out)
	}
}

// With no dialect chosen the substrate refuses rather than picking a set.
func TestNoDialectMeansNoAnswer(t *testing.T) {
	out, _ := nameRun(t, func(s *Semantics) {
		s.DeclarationNameOperands = NameOperandsUnspecified
	}, Diagnostics{}, `export "?"`)
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("said %q, want the unspecified refusal", out)
	}
}

// TestASubscriptedOperandIsAskedPerBuiltin is why this is two fields rather
// than one: the same shell takes it for `unset` and refuses it for `export`.
func TestASubscriptedOperandIsAskedPerBuiltin(t *testing.T) {
	both := func(decl, unset Answer) func(*Semantics) {
		return func(s *Semantics) {
			s.DeclarationTakesASubscript = decl
			s.UnsetTakesASubscript = unset
			s.BadNameToDeclarationFatal = No
			s.BadNameToUnsetFatal = No
		}
	}

	// Refused for a declaration, taken for `unset` — bash's answer.
	out, _ := nameRun(t, both(No, Yes), Diagnostics{}, `export "a[0]"; echo "st=$?"`)
	if !strings.Contains(out, "st=1") {
		t.Errorf("export: said %q, want it refused", out)
	}
	out, _ = nameRun(t, both(No, Yes), Diagnostics{}, `unset "a[0]"; echo "st=$?"`)
	if !strings.Contains(out, "st=0") || strings.Contains(out, "not a valid") {
		t.Errorf("unset: said %q, want it taken", out)
	}

	// And the other way round, which no shell does — but the fields are
	// independent, and a test that only ever moved them together would pass
	// with one field just as well.
	out, _ = nameRun(t, both(Yes, No), Diagnostics{}, `export "a[0]"; echo "st=$?"`)
	if !strings.Contains(out, "st=0") || strings.Contains(out, "not a valid") {
		t.Errorf("export: said %q, want it taken", out)
	}
	out, _ = nameRun(t, both(Yes, No), Diagnostics{}, `unset "a[0]"; echo "st=$?"`)
	if !strings.Contains(out, "st=1") {
		t.Errorf("unset: said %q, want it refused", out)
	}
}

// TestARefusedSubscriptUsesTheOrdinaryBadNameWording, which is why two of
// the four need no new text for this at all: what they say about `export
// a[0]` is what they say about `export 1x`.
func TestARefusedSubscriptUsesTheOrdinaryBadNameWording(t *testing.T) {
	set := func(s *Semantics) {
		s.DeclarationTakesASubscript = No
		s.BadNameToDeclarationFatal = No
	}
	out, _ := nameRun(t, set, Diagnostics{}, `export "a[0]"`)
	if !strings.Contains(out, "a[0]") {
		t.Errorf("said %q, want the operand named", out)
	}
	if !strings.Contains(out, "not a valid identifier") {
		t.Errorf("said %q, want the ordinary bad-name wording", out)
	}
}

// TestAnOperandWithNoSubscriptAsksNothing keeps the question off the path
// every other operand takes.
func TestAnOperandWithNoSubscriptAsksNothing(t *testing.T) {
	out, _ := nameRun(t, nil, Diagnostics{}, `export ok=1; echo "[$ok]"`)
	if !strings.Contains(out, "[1]") {
		t.Errorf("said %q, want an ordinary operand to need no answer", out)
	}
	if strings.Contains(out, "disagree") {
		t.Errorf("said %q, want nothing asked", out)
	}
}

// TestADialectCanHaveItsOwnComplaintAboutASubscript, which is not its
// bad-name wording: two of the three that refuse a subscripted operand say
// what they say about any bad name, and the third has two messages of its
// own — one per builtin.
func TestADialectCanHaveItsOwnComplaintAboutASubscript(t *testing.T) {
	set := func(s *Semantics) {
		s.DeclarationTakesASubscript = No
		s.BadNameToDeclarationFatal = No
	}
	dg := Diagnostics{
		BuiltinBadName: map[string]string{"export": "export: %[2]s: not a name"},
		BuiltinBadSubscript: map[string]string{
			"export":   "%[3]s: about the subscript",
			"readonly": "%[2]s: about the element",
		},
	}

	// The base name, not the operand — which is the point of the third verb.
	out, _ := nameRun(t, set, dg, `export "a[0]"`)
	if !strings.Contains(out, "a: about the subscript") {
		t.Errorf("export: said %q, want the subscript complaint naming the base", out)
	}
	if strings.Contains(out, "not a name") {
		t.Errorf("export: said %q, want its own complaint and not the bad-name one", out)
	}

	// A different one for the other builtin, naming the whole operand.
	out, _ = nameRun(t, set, dg, `readonly "a[0]"`)
	if !strings.Contains(out, "a[0]: about the element") {
		t.Errorf("readonly: said %q, want the other complaint", out)
	}

	// And an operand that is bad for an ordinary reason still gets the
	// ordinary wording, which is what keeps this from swallowing that path.
	out, _ = nameRun(t, set, dg, `export 1x`)
	if !strings.Contains(out, "not a name") {
		t.Errorf("a plain bad name: said %q, want the bad-name wording", out)
	}
}

// TestNoComplaintOfItsOwnMeansTheOrdinaryOne, which is two of the three and
// is why they needed no new text at all.
func TestNoComplaintOfItsOwnMeansTheOrdinaryOne(t *testing.T) {
	set := func(s *Semantics) {
		s.DeclarationTakesASubscript = No
		s.BadNameToDeclarationFatal = No
	}
	out, _ := nameRun(t, set, Diagnostics{}, `export "a[0]"`)
	if !strings.Contains(out, "not a valid identifier") {
		t.Errorf("said %q, want the ordinary bad-name wording", out)
	}
}

// TestWhichSubscriptComplaintNamesTheBuiltin is a set because one shell
// answers it two ways: its message about the subscript carries no builtin in
// the location and its message about array elements does.
func TestWhichSubscriptComplaintNamesTheBuiltin(t *testing.T) {
	set := func(s *Semantics) {
		s.DeclarationTakesASubscript = No
		s.BadNameToDeclarationFatal = No
	}
	dg := Diagnostics{
		Location:               LocationTightLine,
		NamesBuiltinInLocation: true,
		BuiltinBadSubscript: map[string]string{
			"export":   "%[3]s: about the subscript",
			"readonly": "%[2]s: about the element",
		},
		SubscriptRefusalNamesBuiltin: map[string]bool{"readonly": true},
	}

	out, _ := nameRun(t, set, dg, `export "a[0]"`)
	if strings.Contains(out, ":export:") {
		t.Errorf("export: said %q, want no builtin in the location", out)
	}
	out, _ = nameRun(t, set, dg, `readonly "a[0]"`)
	if !strings.Contains(out, ":readonly:") {
		t.Errorf("readonly: said %q, want the builtin in the location", out)
	}
}

// TestASubscriptRefusalCanEndTheScript, under the same rule any bad name to a
// special builtin gets — the wording being the dialect's own does not make
// the fatality a different question.
func TestASubscriptRefusalCanEndTheScript(t *testing.T) {
	dg := Diagnostics{BuiltinBadSubscript: map[string]string{"export": "%[3]s: no"}}

	fatal := func(s *Semantics) {
		s.DeclarationTakesASubscript = No
		s.BadNameToDeclarationFatal = Yes
	}
	out, _ := nameRun(t, fatal, dg, `export "a[0]"; echo after`)
	if strings.Contains(out, "after") {
		t.Errorf("got %q, want the script to stop", out)
	}

	carry := func(s *Semantics) {
		s.DeclarationTakesASubscript = No
		s.BadNameToDeclarationFatal = No
	}
	out, _ = nameRun(t, carry, dg, `export "a[0]"; echo after`)
	if !strings.Contains(out, "after") {
		t.Errorf("got %q, want the script to carry on", out)
	}
}
