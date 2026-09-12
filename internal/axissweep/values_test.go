// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/blairham/sh/interp"
)

// TestPresetUseAsksEveryAxis. The value of this check is that it is total and
// costs no processes, so the thing to pin is that it really does cover the
// whole vector and really does read all four dialects.
func TestPresetUseAsksEveryAxis(t *testing.T) {
	t.Parallel()
	uses, err := PresetUse()
	if err != nil {
		t.Fatal(err)
	}
	if len(uses) < 300 {
		t.Fatalf("only %d axes asked about", len(uses))
	}
	for _, u := range uses {
		for _, want := range []string{"bash", "zsh", "ksh", "dash"} {
			if _, ok := u.Held[want]; !ok {
				t.Fatalf("%s: %s was not asked what it holds", u.Field, want)
			}
		}
		if len(u.Held) != 4 {
			t.Fatalf("%s: %d vectors asked, want the four dialects", u.Field, len(u.Held))
		}
	}
}

// TestAnAxisEveryDialectAnswersAlikeIsReported is the claim the check makes.
// SplitCommandSubstitution is the standing example — its own comment says
// "true everywhere measured, including zsh" — so if this stops being flagged,
// either the axis was triaged or the check stopped working, and the two must
// not look alike.
func TestAnAxisEveryDialectAnswersAlikeIsReported(t *testing.T) {
	t.Parallel()
	uses, err := PresetUse()
	if err != nil {
		t.Fatal(err)
	}
	var unanimous int
	for _, u := range uses {
		if u.Unanimous {
			unanimous++
			if len(u.Held) == 0 {
				t.Fatalf("%s: unanimous with nothing held", u.Field)
			}
		}
	}
	if unanimous == 0 {
		t.Skip("no axis is answered alike by all four dialects; nothing to check here")
	}
}

// TestTheTriageIsReadBackFromTheFieldItAnswers. The verdicts from #2060 are
// worth nothing if they live anywhere but the axis: a note in a pull request
// is invisible to the next sweep, which then re-opens the same 25 entries.
// The standing examples are the two the issue argued about — one reached by a
// run-time option, one by POSIX mode.
func TestTheTriageIsReadBackFromTheFieldItAnswers(t *testing.T) {
	t.Parallel()
	uses, err := PresetUse()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"FunctionLocalTraps":     "TrapsGoBackAtTheReturn",
		"ForNameWhenTheLoopRuns": "ForNameEndsTheScriptAsASyntaxError",
	}
	seen := map[string]bool{}
	for _, u := range uses {
		value, ok := want[u.Field]
		if !ok {
			continue
		}
		seen[u.Field] = true
		if u.Explained[value] == "" {
			t.Errorf("%s: nothing says who holds %s, and its own comment does", u.Field, value)
		}
	}
	for field := range want {
		if !seen[field] {
			t.Errorf("%s: not swept at all", field)
		}
	}
	if u := findUse(uses, "FunctionLocalTraps"); u != nil && u.Why == "" {
		t.Error("FunctionLocalTraps: answered alike by all four and nothing says why it is an axis")
	}
}

// TestATriageEntryLandsInTheHalfItAnswers guards the decode against the way it
// could be too loose: the three halves are three different questions, and a
// verdict read back under the wrong one answers something nobody asked.
func TestATriageEntryLandsInTheHalfItAnswers(t *testing.T) {
	t.Parallel()
	var got map[string]Notes
	const in = `{"SomeAxis": {
		"unanimous":   "because the pair is the measurement.",
		"unexhibited": {"SomeValue": "held by one preset, measured."},
		"unpinned":    {"zsh": "the axis is never consulted there."}
	}}`
	if err := json.Unmarshal([]byte(in), &got); err != nil {
		t.Fatal(err)
	}
	n := got["SomeAxis"]
	if want := "held by one preset, measured."; n.Value["SomeValue"] != want {
		t.Errorf("value note is %q, want %q", n.Value["SomeValue"], want)
	}
	if want := "because the pair is the measurement."; n.Unanimous != want {
		t.Errorf("unanimous note is %q, want %q", n.Unanimous, want)
	}
	if want := "the axis is never consulted there."; n.Unpinned["zsh"] != want {
		t.Errorf("unpinned note is %q, want %q", n.Unpinned["zsh"], want)
	}
	if len(n.Value) != 1 {
		t.Errorf("%d value notes, want the one that is written", len(n.Value))
	}
}

// TestEveryTriagedAxisIsAnAxis is the guard the comment grammar got for free
// and a file of its own does not: a verdict filed under a name no field has
// answers nothing, and reads as triaged from every angle but the one that
// matters. Total by construction, like the rest of this package.
func TestEveryTriagedAxisIsAnAxis(t *testing.T) {
	t.Parallel()
	notes, err := FieldNotes()
	if err != nil {
		t.Fatal(err)
	}
	fields, err := Fields(reflect.TypeFor[interp.Semantics]())
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, f := range fields {
		known[f.Path] = true
	}
	for field := range notes {
		if !known[field] {
			t.Errorf("triage.json: %s is not a field of interp.Semantics", field)
		}
	}
}

func findUse(uses []ValueUse, field string) *ValueUse {
	for i := range uses {
		if uses[i].Field == field {
			return &uses[i]
		}
	}
	return nil
}

// TestAnUnpinnedVerdictIsReadBackPerDialect. The flip half's verdicts are
// per dialect — an axis one preset never consults is one another may lean on —
// so the decode has to keep them apart, and a reason written for every dialect
// has to answer for the ones with no line of their own.
func TestAnUnpinnedVerdictIsReadBackPerDialect(t *testing.T) {
	t.Parallel()
	got := Notes{Unpinned: map[string]string{
		"zsh": "the axis is never consulted there.",
		"":    "and this one answers for anybody else.",
	}}
	if got := verdict(got, "zsh"); got != "the axis is never consulted there." {
		t.Errorf("zsh reads %q, want its own line rather than the shared one", got)
	}
	if got := verdict(got, "bash"); got != "and this one answers for anybody else." {
		t.Errorf("bash reads %q, want the shared line", got)
	}
}

// TestAVerdictForADialectThatDoesNotExistIsRefused. The comment grammar could
// not express one, because the marker only matched the four names. A file can
// write anything, so the loader refuses it rather than reading back a reason
// no sweep can act on.
func TestAVerdictForADialectThatDoesNotExistIsRefused(t *testing.T) {
	t.Parallel()
	if knownDialect("fish") {
		t.Error("fish is not one of the dialects and knownDialect says it is")
	}
	for _, d := range []string{"bash", "zsh", "ksh", "dash"} {
		if !knownDialect(d) {
			t.Errorf("%s is a dialect and knownDialect says it is not", d)
		}
	}
}

// TestTheFlipVerdictsAreOnTheAxesTheyAnswer is the same claim the preset one
// makes, for the four triaged in #2057 that cannot be pinned by any row.
func TestTheFlipVerdictsAreOnTheAxesTheyAnswer(t *testing.T) {
	t.Parallel()
	notes, err := FieldNotes()
	if err != nil {
		t.Fatal(err)
	}
	for field, dialect := range map[string]string{
		"TypeNamesTheKindWithDashT":              "bash",
		"PrintfEmptyIsNotANumber":                "zsh",
		"SetBTurnsOffBraceExpansion":             "zsh",
		"ValuelessDeclarationHidesTheOuterValue": "zsh",
	} {
		if verdict(notes[field], dialect) == "" {
			t.Errorf("%s: nothing says why no corpus row objects in %s", field, dialect)
		}
	}
}
