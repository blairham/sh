// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// `ignoreclosebraces` — the closing half of the pair `ignorebraces` moves.
//
// **This row cannot be graded by `make emulate-sweep`.** No emulation moves
// the name: it is in emulationAlwaysReset with no entry in emulationDefaults,
// so every mode puts it back off and not one corpus snippet changes with it.
// The sweep read 0 for these prefixes before this change and reads 0 after,
// which is the shape an instrument with nothing in front of it has. So the
// cases are here, and what *would* show the difference is stated before the
// change rather than after it: the two names disagree about
// `{print A; }` and `{print A}` and about what survives an
// `unsetopt ignorebraces`, and this shell answered five of those nine rows
// wrongly in two separate states.
//
// Every `want` is the reference's own answer, measured on zsh 5.9.2
// (aarch64-apple-darwin25.4.0) at `/opt/homebrew/bin/zsh`, `-f` over a script
// file under `set -n`, with the names moved on the line after `emulate zsh`,
// 2026-09-28.
var closeBraceRows = []struct {
	name string
	src  string
	// refused with neither name, with `ignorebraces`, with
	// `ignoreclosebraces`, and with both.
	neither, ib, icb, both bool
}{
	{"a group closing with no separator", "{ echo A }\n", false, true, true, true},
	{"a group closing with no blank either", "{ echo A}\n", false, true, true, true},
	{"a close brace ending an argument", "echo A}\n", true, false, false, false},
	{"a case closed with a brace", "case x in x) echo o;; }\n", false, true, true, true},
	// **The two discriminators, and they point opposite ways.** The first is
	// the *opening* reading alone, which the closing name leaves standing.
	// The second needs both readings at once, so it parses under the name
	// that takes both away — three ordinary words — and is refused under the
	// name that opens a group nothing then closes.
	{"a group opening with no blank", "{print A; }\n", false, true, false, true},
	{"a group opening and closing with no blank", "{print A}\n", false, false, true, false},
	// The controls. None of them holds a bare brace either reading can
	// reach, so a wiring that had broken the word rules rather than narrowed
	// them would show here and in none of the rows above.
	{"a quoted close brace", "echo A\\}\n", false, false, false, false},
	{"a close brace in an assignment's value", "x=a}\n", false, false, false, false},
	{"a brace expansion", "echo {a,b}\n", false, false, false, false},
}

// closeBraceState is one of the four states the pair has, and the fifth below
// is the one that says the state cannot be derived from the grammar.
func closeBraceWant(row int, ib, icb bool) bool {
	r := closeBraceRows[row]
	switch {
	case ib && icb:
		return r.both
	case ib:
		return r.ib
	case icb:
		return r.icb
	}
	return r.neither
}

// TestTheTwoBraceNamesAreNotSynonyms is the pair's own row: four states of
// two names over nine snippets, and the two rows that would be wrong
// whichever synonym a one-name wiring picked.
func TestTheTwoBraceNamesAreNotSynonyms(t *testing.T) {
	for i, row := range closeBraceRows {
		t.Run(row.name, func(t *testing.T) {
			for _, st := range []struct {
				name    string
				ib, icb bool
			}{
				{"neither", false, false},
				{"ignorebraces", true, false},
				{"ignoreclosebraces", false, true},
				{"both", true, true},
			} {
				t.Run(st.name, func(t *testing.T) {
					r := caseListRunner(t)
					if code := setOption(r, "ignorebraces", st.ib); code != 0 {
						t.Fatalf("setting ignorebraces answered %d", code)
					}
					if code := setOption(r, "ignoreclosebraces", st.icb); code != 0 {
						t.Fatalf("setting ignoreclosebraces answered %d", code)
					}
					// Both names read back whatever the other is doing, which
					// is what a script that set them needs.
					if got := namedOptionState(t, r, "ignorebraces"); got != st.ib {
						t.Errorf("ignorebraces reads %v, want %v", got, st.ib)
					}
					if got := namedOptionState(t, r, "ignoreclosebraces"); got != st.icb {
						t.Errorf("ignoreclosebraces reads %v, want %v", got, st.icb)
					}
					want := closeBraceWant(i, st.ib, st.icb)
					if got := !parsesHere(t, r, row.src); got != want {
						t.Errorf("refused=%v, want %v", got, want)
					}
				})
			}
		})
	}
}

// TestTheClosingNameSurvivesTheOtherBeingTakenOff is the row that says the
// state has to be kept beside the name rather than read back off the grammar.
//
// With both names on the two grammar fields are in exactly the state
// `ignorebraces` alone leaves them, so nothing in the dialect could say which
// of the two an `unsetopt ignorebraces` should leave behind. Measured, it
// leaves this one.
func TestTheClosingNameSurvivesTheOtherBeingTakenOff(t *testing.T) {
	for i, row := range closeBraceRows {
		t.Run(row.name, func(t *testing.T) {
			r := caseListRunner(t)
			for _, name := range []string{"ignorebraces", "ignoreclosebraces"} {
				if code := setOption(r, name, true); code != 0 {
					t.Fatalf("setting %s answered %d", name, code)
				}
			}
			if code := setOption(r, "ignorebraces", false); code != 0 {
				t.Fatalf("unsetting ignorebraces answered %d", code)
			}
			if got := namedOptionState(t, r, "ignoreclosebraces"); !got {
				t.Fatal("ignoreclosebraces was taken off with the other name")
			}
			// The grammar the closing name alone leaves, which is the `icb`
			// column and **not** the `neither` one.
			if got := !parsesHere(t, r, row.src); got != closeBraceWant(i, false, true) {
				t.Errorf("refused=%v, want the closing name's own answer %v",
					got, closeBraceWant(i, false, true))
			}
		})
	}
}

// TestTheClosingNameLeavesBraceExpansionAlone is the other half of what
// separates the two names: one of them is the substrate's `braceexpand`
// inverted and the other is not.
//
// Measured — `setopt ignoreclosebraces; echo {a,b}` is `a b` and the same
// line under `ignorebraces` is `{a,b}`.
func TestTheClosingNameLeavesBraceExpansionAlone(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ib, icb bool
		expands bool
	}{
		{"neither", false, false, true},
		{"the closing name alone", false, true, true},
		{"the name that owns the expansion", true, false, false},
		{"both", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := caseListRunner(t)
			if code := setOption(r, "ignorebraces", tc.ib); code != 0 {
				t.Fatalf("setting ignorebraces answered %d", code)
			}
			if code := setOption(r, "ignoreclosebraces", tc.icb); code != 0 {
				t.Fatalf("setting ignoreclosebraces answered %d", code)
			}
			on, _ := r.NamedOption("braceexpand")
			if on != tc.expands {
				t.Errorf("braceexpand reads %v, want %v", on, tc.expands)
			}
		})
	}
}

// And the name is no longer one this shell merely remembers, which is the
// bargain it used to strike: the state is real and something reads it.
func TestTheClosingNameIsNotRecordedOnly(t *testing.T) {
	o, _, ok := resolveOptionName("ignoreclosebraces")
	if !ok {
		t.Fatal("ignoreclosebraces is not in the table at all")
	}
	if o.recorded {
		t.Error("still marked recorded, but the grammar reads it")
	}
	if o.set == nil {
		t.Error("cannot be moved, so `setopt ignoreclosebraces` would refuse")
	}
}

// TestThePresetCarriesAllThreeBraceReadings: the third reading is masked by
// the second in the preset's own state — the reserved reading implies the
// lexing — so a preset that left it off would still parse every line above
// correctly until an option moved. This is the row that reads it directly,
// and it is here because a mutant that cleared the preset line survived
// everything else in this package.
func TestThePresetCarriesAllThreeBraceReadings(t *testing.T) {
	r := caseListRunner(t)
	open, closing, endsWord := r.BraceReservedWordReadings()
	if !open || !closing || !endsWord {
		t.Errorf("open=%v closing=%v endsWord=%v, want all three of this shell's brace readings",
			open, closing, endsWord)
	}
}
