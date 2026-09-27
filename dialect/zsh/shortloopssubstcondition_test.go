// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// A `$( … )` body whose read stopped at the closing parenthesis with an `if`
// short of its `then` is left to the moment it runs here, and `shortloops` is
// what decides it.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh`, `-f` over a script file under `set -n`, with the
// option moved on the line after the `emulate`, 2026-09-27. Every `want` is
// the reference's answer.
var unfinishedConditionRows = []struct {
	name string
	body string
	// refused with the option on and with it off.
	on, off bool
}{
	{"the keyword alone", "if", false, true},
	{"a condition with nothing after it", "if true", false, true},
	{"a condition left on an operator", "if true &&", false, true},
	{"a condition that is itself an if", "if if true", false, true},
	{"an elif's condition", "if true; then :; elif", false, true},
	// The `then` is the line: this is the same construct one keyword later
	// and it does not move with the option, in the reference or here.
	{"a then already read", "if true; then", true, true},
	// The control on the other side: a body with nothing wrong in it is
	// untouched in both states.
	{"a body that parses", "echo hi", false, false},
}

func TestTheOptionDecidesAnUnfinishedConditionInASubstitution(t *testing.T) {
	for _, row := range unfinishedConditionRows {
		t.Run(row.name, func(t *testing.T) {
			for _, state := range []struct {
				name string
				on   bool
				want bool
			}{{"shortloops", true, row.on}, {"noshortloops", false, row.off}} {
				t.Run(state.name, func(t *testing.T) {
					r := caseListRunner(t)
					if code := setOption(r, "shortloops", state.on); code != 0 {
						t.Fatalf("setting shortloops answered %d", code)
					}
					if got := shortLoopsOn(r); got != state.on {
						t.Errorf("shortloops reads %v, want %v", got, state.on)
					}
					src := "echo b; v=$(" + row.body + "); echo a\n"
					if got := !parsesHere(t, r, src); got != state.want {
						t.Errorf("refused=%v, want %v", got, state.want)
					}
				})
			}
		})
	}
}

// And through the emulations, which is the route the two `subst/` cases of
// #4819 take: `shortloops` is in emulationAlwaysReset and its per-emulation
// default is off under `sh` and `ksh` and on under `csh`, all measured before
// this change.
func TestAnEmulationReachesAnUnfinishedConditionThroughTheOption(t *testing.T) {
	for _, tc := range []struct {
		mode string
		on   bool
	}{{"zsh", true}, {"sh", false}, {"ksh", false}, {"csh", true}} {
		t.Run("emulate "+tc.mode, func(t *testing.T) {
			r := caseListRunner(t)
			applyEmulation(r, tc.mode, false)
			if got := shortLoopsOn(r); got != tc.on {
				t.Fatalf("shortloops reads %v, want %v", got, tc.on)
			}
			for _, row := range unfinishedConditionRows {
				want := row.off
				if tc.on {
					want = row.on
				}
				src := "echo b; v=$(" + row.body + "); echo a\n"
				if got := !parsesHere(t, r, src); got != want {
					t.Errorf("%s: refused=%v, want %v", row.name, got, want)
				}
			}
		})
	}

	// The mode held fixed and the option moved, which is what says the option
	// decides and the mode only sets it.
	t.Run("the option settles inside the shell's own mode", func(t *testing.T) {
		r := caseListRunner(t)
		applyEmulation(r, "zsh", false)
		if code := setOption(r, "shortloops", false); code != 0 {
			t.Fatalf("setting shortloops answered %d", code)
		}
		if parsesHere(t, r, "echo b; v=$(if); echo a\n") {
			t.Error("the body is still deferred, so the mode is what decides and not the option")
		}
	})

	t.Run("the option defers inside a narrowed mode", func(t *testing.T) {
		r := caseListRunner(t)
		applyEmulation(r, "sh", false)
		if code := setOption(r, "shortloops", true); code != 0 {
			t.Fatalf("setting shortloops answered %d", code)
		}
		if !parsesHere(t, r, "echo b; v=$(if); echo a\n") {
			t.Error("the body is still refused, so the mode is what decides and not the option")
		}
	})
}

// And the rest of the closer's population, which settles here now and did
// not before: a refusal on the closing parenthesis is the body's like any
// other, and the rows above are the one shape carved back out of that.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh`, `-f` over a script file under `set -n`,
// 2026-09-27: every body below is refused **with the line** there under all
// three modes, and every one of them was taken here until #4859.
func TestTheRestOfTheClosersPopulationSettlesTheReadHere(t *testing.T) {
	for _, body := range []string{
		"for", "case x", "{", "select", "repeat", "echo |",
		// And the three `if` shapes the option does **not** reach, which are
		// what says the flag above is narrower than the rule it sits in: an
		// `if` past its `then`, and an unfinished `if` inside something else.
		"if true; then :; else", "if true; then if", "while true; do if",
	} {
		t.Run(body, func(t *testing.T) {
			r := caseListRunner(t)
			src := "echo b; v=$(" + body + "); echo a\n"
			if parsesHere(t, r, src) {
				t.Errorf("$(%s) still reads as a closed substitution", body)
			}
			// And the option does not move it, which is what keeps the two
			// rules apart: `shortloops` reaches the unfinished condition and
			// nothing else on the closer.
			for _, on := range []bool{true, false} {
				if code := setOption(r, "shortloops", on); code != 0 {
					t.Fatalf("setting shortloops answered %d", code)
				}
				if parsesHere(t, r, src) {
					t.Errorf("$(%s) reads as closed with shortloops=%v", body, on)
				}
			}
		})
	}
}
