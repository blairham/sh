// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// A subscript's flag group is read in two positions and `shglob` takes it off
// one of them.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh`, run `-f` over a script file under `set -n`, with
// the options moved on the line before, 2026-09-27.
var subscriptFlagRows = []struct {
	name                  string
	src                   string
	neither, shGlob, both bool
}{
	{"a flag group in a command word", "b[(r)y]=Q\n", false, true, false},
	{"another letter, same position", "b[(i)x]=Q\n", false, true, false},
	{"a flag group on a string", "s[(r)l]=Q\n", false, true, false},
	// The group in the other position, which does not move: with `shglob`
	// alone the line above is a parse error in the reference and this one
	// still runs the search.
	{"a flag group in a substitution", "echo ${b[(r)y]}\n", false, false, false},
	// The controls: an ordinary subscript, and the blanks a command word's
	// subscript still holds in every state.
	{"an ordinary subscript", "b[2]=Q\n", false, false, false},
	{"blanks inside a command word's subscript", "b[a b]=Q\n", false, false, false},
}

func TestTheOptionPairDecidesACommandWordsSubscriptFlagGroup(t *testing.T) {
	for _, row := range subscriptFlagRows {
		t.Run(row.name, func(t *testing.T) {
			for _, state := range []struct {
				name            string
				shGlob, kshGlob bool
				want            bool
			}{
				{"neither", false, false, row.neither},
				{"shglob", true, false, row.shGlob},
				{"shglob and kshglob", true, true, row.both},
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
					if got := !parsesHere(t, r, row.src); got != state.want {
						t.Errorf("refused=%v, want %v", got, state.want)
					}
				})
			}
		})
	}
}

// And through the emulations, which is the route #4819's subscript cases take.
func TestAnEmulationReachesACommandWordsSubscriptFlagGroup(t *testing.T) {
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
			for _, row := range subscriptFlagRows {
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
}
