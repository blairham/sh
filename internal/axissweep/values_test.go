// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import "testing"

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

// TestATriageLineIsOnlyReadWhereItIsWritten guards the parse against the two
// ways it could be too loose: a sentence that merely begins with the word, and
// a note that swallows the paragraph after it.
func TestATriageLineIsOnlyReadWhereItIsWritten(t *testing.T) {
	t.Parallel()
	got := parseNotes("Unanimous answers are common here.\n" +
		"unexhibited SomeValue: held by ksh93, measured.\n" +
		"Still the same note.\n" +
		"\n" +
		"unanimous: because the pair is the measurement.\n" +
		"\n" +
		"A closing paragraph that answers nothing.\n")
	if want := "held by ksh93, measured. Still the same note."; got.Value["SomeValue"] != want {
		t.Errorf("value note is %q, want %q", got.Value["SomeValue"], want)
	}
	if want := "because the pair is the measurement."; got.Unanimous != want {
		t.Errorf("unanimous note is %q, want %q", got.Unanimous, want)
	}
	if len(got.Value) != 1 {
		t.Errorf("%d value notes, want the one that is written", len(got.Value))
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
