// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// Where a bare `(a|b)` opens, as the two option names decide it.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh`, run `-f` over a script file under `set -n`, with
// the options moved on the line before, 2026-09-27. Every `want` below is the
// reference's answer.
var bareGroupRows = []struct {
	name string
	src  string
	// refused with neither option, with `shglob`, and with both.
	neither, shGlob, both bool
}{
	{"a group where a condition's operand begins", "[[ a == (a|b) ]]\n", false, true, true},
	{"a group inside a condition's operand", "[[ ab == a(b|c) ]]\n", false, true, false},
	{"a group inside an argument", "echo x=(echo hi)\n", false, true, false},
	{"a group inside an arithmetic argument", "let a=(5 + 3)/2\n", false, true, false},
	// The controls. Neither holds an unquoted bare parenthesis, so an option
	// that had broken the word rules rather than narrowed them would show
	// here and in none of the rows above.
	{"no group at all", "[[ ab == a*c ]]\n", false, false, false},
	{"a quoted parenthesis", "echo 'a(b|c)'\n", false, false, false},
	{"a subshell where a command begins", "(echo hi)\n", false, false, false},
}

// TestTheOptionPairDecidesWhereABareGroupOpens is the pair's own row: three
// readings of one construct, reached by two names.
//
// **The second row is the discriminator.** Without it the pair would read as
// "the second name undoes the first for parentheses", which every other row
// here agrees with and which is false: the same two characters are refused
// where a word begins and taken inside one, in one shell in one state.
func TestTheOptionPairDecidesWhereABareGroupOpens(t *testing.T) {
	for _, row := range bareGroupRows {
		t.Run(row.name, func(t *testing.T) {
			for _, state := range []struct {
				name            string
				shGlob, kshGlob bool
				want            bool
			}{
				{"neither", false, false, row.neither},
				{"shglob", true, false, row.shGlob},
				{"shglob and kshglob", true, true, row.both},
				// `kshglob` alone changes nothing, which is measured: with
				// bare groups already opening anywhere there is nothing for
				// the narrowed reading to narrow.
				{"kshglob alone", false, true, row.neither},
			} {
				t.Run(state.name, func(t *testing.T) {
					r := caseListRunner(t)
					if code := setOption(r, "shglob", state.shGlob); code != 0 {
						t.Fatalf("setting shglob answered %d", code)
					}
					if code := setOption(r, "kshglob", state.kshGlob); code != 0 {
						t.Fatalf("setting kshglob answered %d", code)
					}
					// Both names read back whatever the other is doing, which
					// is what a script that set them needs and what the
					// derivation from the grammar has to preserve.
					if got := shGlobOn(r); got != state.shGlob {
						t.Errorf("shglob reads %v, want %v", got, state.shGlob)
					}
					if got := kshGlobOn(r); got != state.kshGlob {
						t.Errorf("kshglob reads %v, want %v", got, state.kshGlob)
					}
					if got := !parsesHere(t, r, row.src); got != state.want {
						t.Errorf("refused=%v, want %v", got, state.want)
					}
				})
			}
		})
	}
}

// TestAnEmulationReachesTheBareGroupGrammarThroughTheOptions is the
// emulation's row. The modes carry the pair because `shglob` and `kshglob` are
// both in emulationAlwaysReset and the per-emulation defaults hold `shglob` on
// under `sh` and `ksh`, `kshglob` on under `ksh` alone, and neither under
// `csh` — all measured before this change and unchanged by it.
func TestAnEmulationReachesTheBareGroupGrammarThroughTheOptions(t *testing.T) {
	for _, tc := range []struct {
		mode            string
		shGlob, kshGlob bool
	}{
		{"zsh", false, false},
		{"sh", true, false},
		{"ksh", true, true},
		{"csh", false, false},
	} {
		t.Run("emulate "+tc.mode, func(t *testing.T) {
			r := caseListRunner(t)
			applyEmulation(r, tc.mode, false)
			if got := shGlobOn(r); got != tc.shGlob {
				t.Fatalf("shglob reads %v, want %v", got, tc.shGlob)
			}
			if got := kshGlobOn(r); got != tc.kshGlob {
				t.Fatalf("kshglob reads %v, want %v", got, tc.kshGlob)
			}
			for _, row := range bareGroupRows {
				want := row.neither
				switch {
				case tc.shGlob && tc.kshGlob:
					want = row.both
				case tc.shGlob:
					want = row.shGlob
				}
				if got := !parsesHere(t, r, row.src); got != want {
					t.Errorf("%s: refused=%v, want %v", row.name, got, want)
				}
			}
		})
	}

	// And the mode held fixed while the option moves, which is what says the
	// option decides and the mode only sets it.
	t.Run("the option narrows inside the shell's own mode", func(t *testing.T) {
		r := caseListRunner(t)
		applyEmulation(r, "zsh", false)
		if code := setOption(r, "shglob", true); code != 0 {
			t.Fatalf("setting shglob answered %d", code)
		}
		if parsesHere(t, r, bareGroupRows[0].src) {
			t.Error("the group still opens, so the mode is what decides and not the option")
		}
	})

	t.Run("the option widens inside a narrowed mode", func(t *testing.T) {
		r := caseListRunner(t)
		applyEmulation(r, "sh", false)
		if code := setOption(r, "shglob", false); code != 0 {
			t.Fatalf("setting shglob answered %d", code)
		}
		if !parsesHere(t, r, bareGroupRows[0].src) {
			t.Error("the group is still refused, so the mode is what decides and not the option")
		}
	})
}
