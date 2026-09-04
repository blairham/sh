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

// `trap` took no options at all before these: `trap -p` set a trap whose
// action was the word `-p`, and the shell failed to run it later, when the
// trap fired. These name the axis rather than the shell.

// trapRun runs src with the given answers and returns stdout, stderr and the
// status, because the interesting cases differ in all three.
func trapRun(t *testing.T, src string, set func(*Semantics), dg Diagnostics) (string, string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.TrapParsesOptions = Yes
	sem.TrapOneArgumentIsACondition = Yes
	sem.TrapReportsAnUnknownSingleCondition = Yes
	sem.TrapSingleUnknownConditionIsUsage = No
	// A spelling for the tests that are not about spelling. The ones that
	// are override it.
	sem.TrapQuoting = ListingQuoteAlwaysEscaped
	if set != nil {
		set(&sem)
	}
	var out, errs bytes.Buffer
	dir := t.TempDir()
	r := &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

// TestATrapOptionIsReadOrTakenAsTheAction is the axis that decides whether
// there are options here at all, and both directions are load-bearing: the
// "no" answer is a real dialect's, not a fallback.
func TestATrapOptionIsReadOrTakenAsTheAction(t *testing.T) {
	read := func(s *Semantics) { s.TrapPrintsWithP = Yes }
	out, errs, st := trapRun(t, "trap 'echo hi' INT\ntrap -p", read, Diagnostics{})
	if !strings.Contains(out, "trap -- 'echo hi' INT") {
		t.Errorf("read as an option: stdout %q, want the trap printed", out)
	}
	if errs != "" || st != 0 {
		t.Errorf("read as an option: stderr %q, status %d, want neither", errs, st)
	}

	// Taken as the action instead: nothing is printed, and the word becomes
	// a trap on a condition that is never named, so nothing runs either.
	// The dialect that answers no here also says nothing about a word that
	// is not a condition, which is what makes `trap -p` silent rather than
	// merely unprinted.
	notRead := func(s *Semantics) {
		s.TrapParsesOptions = No
		s.TrapReportsAnUnknownSingleCondition = No
	}
	out, _, st = trapRun(t, "trap 'echo hi' INT\ntrap -p", notRead, Diagnostics{})
	if strings.Contains(out, "trap --") {
		t.Errorf("taken as the action: stdout %q, want nothing printed", out)
	}
	if st != 0 {
		t.Errorf("taken as the action: status %d, want 0", st)
	}
}

// TestAnUnknownTrapOptionGoesThroughTheSharedComplaint keeps trap on the
// same path every other builtin's bad option takes, rather than growing a
// second spelling of the same message.
func TestAnUnknownTrapOptionGoesThroughTheSharedComplaint(t *testing.T) {
	dg := Diagnostics{
		BuiltinBadOption: "%[1]s: %[2]s: invalid option",
		BuiltinUsage:     map[string]string{"trap": "trap: usage: trap ..."},
	}
	_, errs, st := trapRun(t, "trap -Q INT", nil, dg)
	if !strings.Contains(errs, "trap: -Q: invalid option") {
		t.Errorf("stderr = %q, want the shared complaint", errs)
	}
	if !strings.Contains(errs, "trap: usage: trap ...") {
		t.Errorf("stderr = %q, want the usage line from the same table", errs)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
}

// TestPrintingATrapNamesOnlyWhatWasAsked covers -p with and without
// conditions, and the bare form the two dialects reach differently.
func TestPrintingATrapNamesOnlyWhatWasAsked(t *testing.T) {
	yes := func(s *Semantics) { s.TrapPrintsWithP = Yes; s.TrapPrintsBareWithP = Yes }

	out, _, _ := trapRun(t, "trap 'echo hi' INT\ntrap : EXIT\ntrap -p INT", yes, Diagnostics{})
	if !strings.Contains(out, "INT") {
		t.Errorf("stdout = %q, want the condition that was asked for", out)
	}
	if strings.Contains(out, "EXIT") {
		t.Errorf("stdout = %q, want only the condition that was asked for", out)
	}

	// -P is the action alone, with nothing around it.
	out, _, _ = trapRun(t, "trap 'echo hi' INT\ntrap -P INT", yes, Diagnostics{})
	if strings.TrimSpace(out) != "echo hi" {
		t.Errorf("bare = %q, want the action alone", out)
	}

	// ksh93 reaches that output through -p with an operand instead.
	bare := func(s *Semantics) { s.TrapPrintsWithP = Yes; s.TrapPrintsBareWithConditions = Yes }
	out, _, _ = trapRun(t, "trap 'echo hi' INT\ntrap -p INT", bare, Diagnostics{})
	if strings.TrimSpace(out) != "echo hi" {
		t.Errorf("bare with conditions = %q, want the action alone", out)
	}
	// And only with an operand: the bare listing is still the full one.
	out, _, _ = trapRun(t, "trap 'echo hi' INT\ntrap -p", bare, Diagnostics{})
	if !strings.Contains(out, "trap --") {
		t.Errorf("bare without conditions = %q, want the whole line", out)
	}
}

// TestBarePrintingInsistsOnACondition is the one option that refuses to
// guess: printing all of them is what -p is for.
func TestBarePrintingInsistsOnACondition(t *testing.T) {
	yes := func(s *Semantics) { s.TrapPrintsBareWithP = Yes }
	out, errs, st := trapRun(t, "trap 'echo hi' INT\ntrap -P", yes,
		Diagnostics{TrapBarePrintNeedsCondition: "trap: -P needs one"})
	if out != "" {
		t.Errorf("stdout = %q, want nothing printed", out)
	}
	if !strings.Contains(errs, "trap: -P needs one") || st != 2 {
		t.Errorf("stderr %q status %d, want the refusal and 2", errs, st)
	}
}

// TestOneArgumentIsAConditionOrIsRefused is the short spelling of
// `trap - condition`, and the dialect that refuses the form entirely.
func TestOneArgumentIsAConditionOrIsRefused(t *testing.T) {
	yes := func(s *Semantics) { s.TrapPrintsWithP = Yes }
	out, errs, st := trapRun(t, "trap 'echo hi' INT\ntrap INT\ntrap -p", yes, Diagnostics{})
	if strings.Contains(out, "echo hi") {
		t.Errorf("stdout = %q, want the trap put back", out)
	}
	if errs != "" || st != 0 {
		t.Errorf("stderr %q status %d, want neither", errs, st)
	}

	refuse := func(s *Semantics) { s.TrapOneArgumentIsACondition = No }
	out, errs, st = trapRun(t, "trap INT\necho after", refuse,
		Diagnostics{TrapConditionRequired: "trap: condition(s) required"})
	if !strings.Contains(errs, "trap: condition(s) required") {
		t.Errorf("stderr = %q, want the refusal", errs)
	}
	if strings.Contains(out, "after") {
		t.Errorf("stdout = %q, want the refusal to end the script", out)
	}
	if st == 0 {
		t.Error("status = 0, want the refusal to fail")
	}
}

// TestTheOneWordThatIsNotAConditionIsAnsweredThreeWays is why two axes were
// needed rather than one: saying nothing, naming the word, and printing the
// usage are three different answers to the same input.
func TestTheOneWordThatIsNotAConditionIsAnsweredThreeWays(t *testing.T) {
	dg := Diagnostics{
		TrapBadSignal: "trap: %[1]s: bad trap",
		BuiltinUsage:  map[string]string{"trap": "trap: usage: trap ..."},
	}

	_, errs, st := trapRun(t, "trap notacondition", nil, dg)
	if !strings.Contains(errs, "trap: notacondition: bad trap") || st != 1 {
		t.Errorf("naming the word: stderr %q status %d", errs, st)
	}

	usage := func(s *Semantics) { s.TrapSingleUnknownConditionIsUsage = Yes }
	_, errs, st = trapRun(t, "trap notacondition", usage, dg)
	if !strings.Contains(errs, "trap: usage: trap ...") {
		t.Errorf("printing the usage: stderr = %q", errs)
	}
	if strings.Contains(errs, "bad trap") {
		t.Errorf("printing the usage: stderr = %q, want the word not named", errs)
	}
	if st != 2 {
		t.Errorf("printing the usage: status = %d, want 2", st)
	}

	silent := func(s *Semantics) { s.TrapReportsAnUnknownSingleCondition = No }
	_, errs, st = trapRun(t, "trap notacondition", silent, dg)
	if errs != "" || st != 0 {
		t.Errorf("saying nothing: stderr %q status %d, want neither", errs, st)
	}
}

// TestASignalCanBePrintedWithItsPrefix is bash's spelling, and EXIT never
// takes it because EXIT is not a signal.
func TestASignalCanBePrintedWithItsPrefix(t *testing.T) {
	yes := func(s *Semantics) { s.TrapPrintsWithP = Yes }
	out, _, _ := trapRun(t, "trap 'echo hi' INT\ntrap : EXIT\ntrap -p", yes,
		Diagnostics{TrapPrintsSignalPrefix: "SIG"})
	if !strings.Contains(out, "SIGINT") {
		t.Errorf("stdout = %q, want the prefix", out)
	}
	if strings.Contains(out, "SIGEXIT") {
		t.Errorf("stdout = %q, want EXIT left alone", out)
	}
}

// TestDashDashEndsTheOptions is unanimous, and the case that needs it is an
// action that looks like one: `trap -- -p EXIT` sets a trap that runs `-p`.
func TestDashDashEndsTheOptions(t *testing.T) {
	yes := func(s *Semantics) { s.TrapPrintsWithP = Yes }
	out, _, st := trapRun(t, "trap -- -p INT\ntrap -p", yes, Diagnostics{})
	if !strings.Contains(out, "trap -- '-p' INT") {
		t.Errorf("stdout = %q, want the word taken as the action", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// TestListingSignalsIsTheSameListingKillPrints, which is deliberately the
// plain one: the real shells number the table and lay it out in columns, and
// the table is not the same on two operating systems.
func TestListingSignalsIsTheSameListingKillPrints(t *testing.T) {
	yes := func(s *Semantics) { s.TrapListsSignalsWithL = Yes }
	out, errs, st := trapRun(t, "trap -l", yes, Diagnostics{})
	if !strings.Contains(out, "INT") || !strings.Contains(out, "TERM") {
		t.Errorf("stdout = %q, want the signal names", out)
	}
	if errs != "" || st != 0 {
		t.Errorf("stderr %q status %d, want neither", errs, st)
	}

	// And the letter is refused where the dialect does not have it, rather
	// than listing anyway.
	no := func(s *Semantics) { s.TrapListsSignalsWithL = No }
	out, _, st = trapRun(t, "trap -l", no, Diagnostics{BuiltinBadOption: "%[1]s: %[2]s: invalid option"})
	if strings.Contains(out, "INT") {
		t.Errorf("refused: stdout = %q, want nothing listed", out)
	}
	if st != 2 {
		t.Errorf("refused: status = %d, want 2", st)
	}
}

// TestExitIsNotASignalWhenPrintedByName is the guard the prefix needs, and
// it only shows through `-p EXIT`: the listing that prints every trap writes
// EXIT itself and never goes through the naming at all.
func TestExitIsNotASignalWhenPrintedByName(t *testing.T) {
	yes := func(s *Semantics) { s.TrapPrintsWithP = Yes }
	out, _, _ := trapRun(t, "trap : EXIT\ntrap -p EXIT", yes,
		Diagnostics{TrapPrintsSignalPrefix: "SIG"})
	if !strings.Contains(out, "EXIT") {
		t.Errorf("stdout = %q, want the condition named", out)
	}
	if strings.Contains(out, "SIGEXIT") {
		t.Errorf("stdout = %q, want EXIT left alone", out)
	}
}

// TestOneArgumentPutsEXITBackToo, which is a separate path from putting a
// signal back: EXIT is not in the signal table and is kept on its own.
func TestOneArgumentPutsEXITBackToo(t *testing.T) {
	yes := func(s *Semantics) { s.TrapPrintsWithP = Yes }
	out, _, _ := trapRun(t, "trap 'echo ran' EXIT\ntrap EXIT\ntrap -p\necho end", yes, Diagnostics{})
	if strings.Contains(out, "trap --") {
		t.Errorf("stdout = %q, want the EXIT trap put back", out)
	}
	if strings.Contains(out, "ran") {
		t.Errorf("stdout = %q, want the action never to run", out)
	}
	if !strings.Contains(out, "end") {
		t.Errorf("stdout = %q, want the script to carry on", out)
	}
}

// TestATrapActionIsSpelledTheWayThisDialectListsValues, which is not always
// the way its aliases are spelled: the style is shared vocabulary and the
// choice of style is per builtin.
func TestATrapActionIsSpelledTheWayThisDialectListsValues(t *testing.T) {
	for _, tc := range []struct {
		name  string
		style ListingQuotingStyle
		want  string
	}{
		{"always quoted", ListingQuoteAlwaysEscaped, "trap -- ':' INT"},
		{"always quoted, doubled", ListingQuoteAlwaysDoubled, "trap -- ':' INT"},
		{"bare when it can be", ListingQuoteWhenNeededDollar, "trap -- : INT"},
		{"bare, and no dollar form", ListingQuoteWhenNeededPlain, "trap -- : INT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := func(s *Semantics) { s.TrapPrintsWithP = Yes; s.TrapQuoting = tc.style }
			out, _, _ := trapRun(t, "trap : INT\ntrap -p", set, Diagnostics{})
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("stdout = %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// TestTheTwoStylesThatQuoteAlwaysDifferOnAnEmbeddedQuote, which is the only
// thing that tells them apart — and the reason a one-word action cannot.
func TestTheTwoStylesThatQuoteAlwaysDifferOnAnEmbeddedQuote(t *testing.T) {
	run := func(style ListingQuotingStyle) string {
		set := func(s *Semantics) { s.TrapPrintsWithP = Yes; s.TrapQuoting = style }
		out, _, _ := trapRun(t, "trap \"echo it's\" INT\ntrap -p", set, Diagnostics{})
		return strings.TrimSpace(out)
	}
	escaped, doubled := run(ListingQuoteAlwaysEscaped), run(ListingQuoteAlwaysDoubled)
	if !strings.Contains(escaped, `it'\''s`) {
		t.Errorf("escaped = %q, want the backslash spelling", escaped)
	}
	if !strings.Contains(doubled, `it'"'"'s`) {
		t.Errorf("doubled = %q, want the double-quoted spelling", doubled)
	}
	if escaped == doubled {
		t.Error("the two styles produced the same text, so neither is being chosen")
	}

	// And the style that reaches for `$'...'` does, where the plain one
	// never does — the difference zsh's two builtins showed.
	dollar := run(ListingQuoteWhenNeededDollar)
	if !strings.HasPrefix(dollar, "trap -- $'") {
		t.Errorf("dollar = %q, want the dollar form", dollar)
	}
	if plain := run(ListingQuoteWhenNeededPlain); strings.Contains(plain, "$'") {
		t.Errorf("plain = %q, want no dollar form", plain)
	}
}

// TestATrapAndAnAliasAreAskedSeparately is the whole reason for the second
// field: one dialect answers the two differently.
func TestATrapAndAnAliasAreAskedSeparately(t *testing.T) {
	set := func(s *Semantics) {
		s.TrapPrintsWithP = Yes
		s.TrapQuoting = ListingQuoteWhenNeededPlain
		s.AliasQuoting = ListingQuoteAlwaysEscaped
	}
	out, _, _ := trapRun(t, "trap : INT\ntrap -p", set, Diagnostics{})
	if strings.Contains(out, "':'") {
		t.Errorf("stdout = %q, want trap to use its own answer, not the alias one", out)
	}
}
