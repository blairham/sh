// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// `shortloops` is what decides whether a body standing where `do … done`
// would may be written as one command or left out altogether — and it leaves
// the **brace**-spelled body alone, which is why
// [syntax.Dialect.ShortFormBody] is a flag of its own rather than the whole
// of `ShortForm` (#4887).
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh`, `-f` over a script file under `set -n`, with the
// option moved on the line after the `emulate`, 2026-09-27. Every `want` is
// the reference's answer, and every row was measured in all three of that
// shell's emulations and answers alike in each.
var shortBodyRows = []struct {
	name string
	src  string
	// refused with the option on and with it off.
	on, off bool
}{
	// A body of one command, in each construct the family reaches. The
	// separator in front of it is optional and is not what decides, which is
	// why the same loop is here written both ways.
	{"a for's list and one command", "for i in a b; echo $i\n", false, true},
	{"a for's parenthesized list and one command", "for i (a b) echo $i\n", false, true},
	{"a select's parenthesized list and one command", "select o (a b) :\n", false, true},
	{"a count loop and one command", "repeat 2 echo x\n", false, true},
	{"an if whose condition ended itself", "if (( 1 )) echo hi\n", false, true},
	{"a while with a separator", "while (( 0 )); :\n", false, true},
	{"a while with none", "while (( 0 )) :\n", false, true},
	{"an until", "until (( 1 )); :\n", false, true},
	// And the body omitted altogether.
	{"a for with a name and no list", "for i\n", false, true},
	{"a for with a list and no body", "for i in a b\n", false, true},
	{"a while with nothing after it", "while\n", false, true},
	{"a while with a condition and no body", "while true\n", false, true},
	{"an until with nothing after it", "until\n", false, true},
	// The brace spelling, which the option does not reach. These are the
	// rows that say the name is narrower than the flag it used to be wired
	// to would have been: an option written straight onto `ShortForm` takes
	// all of them away with the thirteen above.
	{"an if with a brace body", "if (( 1 )) { echo A; }\n", false, false},
	{"an if with a brace body and a redundant fi", "if (( 1 )) { echo A; } fi\n", false, false},
	{"a while with a brace body", "while (( 0 )) { :; }\n", false, false},
	{"an until with a brace body", "until (( 1 )) { :; }\n", false, false},
	{"a count loop with a brace body", "repeat 2 { echo x; }\n", false, false},
	{"a select with a brace body", "select o (a b) { :; }\n", false, false},
	{"a for with a parenthesized list and a brace body", "for i (a b) { echo $i; }\n", false, false},
	{"a for with a list and a brace body", "for i in a b; { echo $i; }\n", false, false},
	// Nor the parenthesized item list, which the long body reaches in both
	// states, nor the long form itself.
	{"a parenthesized list with a keyword body", "for i (a b); do echo $i; done\n", false, false},
	{"the long form", "for i in a b; do echo $i; done\n", false, false},
	// The control on the other side: an ordinary command is untouched, and a
	// stop word where a command belongs stays refused, so an option that had
	// moved the grammar at large rather than this one production would show
	// here and nowhere else.
	{"an ordinary command", "echo hi\n", false, false},
	{"a stop word where a command belongs", "{ fi; }\n", true, true},
}

func TestTheOptionDecidesAShortBodyWrittenWithoutBraces(t *testing.T) {
	for _, row := range shortBodyRows {
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
					if got := r.ShortFormBodyIsOneCommandOrNone(); got != state.on {
						t.Errorf("the reading is %v, want %v", got, state.on)
					}
					if got := !parsesHere(t, r, row.src); got != state.want {
						t.Errorf("refused=%v, want %v", got, state.want)
					}
				})
			}
		})
	}
}

// refusedUnderCshjunkieloops are the rows `emulate csh` refuses although its
// `shortloops` is on: every loop whose body is one command or none, because
// the mode turns `cshjunkieloops` on too and a loop's body must then be closed
// by `end`. Measured 2026-10-02 on zsh 5.9.2, each row in a script file after
// `emulate csh` and `setopt noexec`, with the stop-word row as the positive
// control; the `if` rows parse there, the option being about loops (#5155).
var refusedUnderCshjunkieloops = map[string]bool{
	"a for's list and one command":                  true,
	"a for's parenthesized list and one command":    true,
	"a select's parenthesized list and one command": true,
	"a count loop and one command":                  true,
	"a while with a separator":                      true,
	"a while with none":                             true,
	"an until":                                      true,
	"a for with a name and no list":                 true,
	"a for with a list and no body":                 true,
	"a while with nothing after it":                 true,
	"a while with a condition and no body":          true,
	"an until with nothing after it":                true,
}

// And through the emulations, which is the route #4817's `core` rows take:
// `shortloops` is in emulationAlwaysReset and its per-emulation default is
// off under `sh` and `ksh` and on under `csh`.
//
// Every row answers the same way in every mode with the option held, which is
// the measurement rather than a consequence of the wiring — except that `csh`
// moves a second option, see refusedUnderCshjunkieloops.
func TestAnEmulationReachesAShortBodyThroughTheOption(t *testing.T) {
	for _, tc := range []struct {
		mode string
		on   bool
	}{{"zsh", true}, {"sh", false}, {"ksh", false}, {"csh", true}} {
		t.Run("emulate "+tc.mode, func(t *testing.T) {
			r := caseListRunner(t)
			applyEmulation(r, tc.mode, false)
			if got := r.ShortFormBodyIsOneCommandOrNone(); got != tc.on {
				t.Fatalf("the reading is %v, want %v", got, tc.on)
			}
			for _, row := range shortBodyRows {
				want := row.off
				if tc.on {
					want = row.on
				}
				if tc.mode == "csh" && refusedUnderCshjunkieloops[row.name] {
					// `emulate csh` turns `cshjunkieloops` on as well, and a
					// loop's body then has to be `end`-closed — see
					// TestCshjunkieloopsClosesALoopBodyWithEnd.
					want = true
				}
				if got := !parsesHere(t, r, row.src); got != want {
					t.Errorf("%s: refused=%v, want %v", row.name, got, want)
				}
			}
		})
	}
}
