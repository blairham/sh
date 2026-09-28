// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// The three option names of #4817 that reach the **grammar**, and the rows
// that say so.
//
// Every `want` here is the reference's own answer, measured on zsh 5.9.2
// (aarch64-apple-darwin25.4.0) at `/opt/homebrew/bin/zsh`, run `-f` over a
// script file under `set -n` with the option moved on the line after an
// `emulate`, 2026-09-27 — and then re-measured against this shell's binary
// side by side, every row agreeing in both states.
//
// The modes reach all three through the option table rather than through a
// per-mode grammar: each name is in emulationAlwaysReset and each has a row in
// emulationDefaults. That is what the emulation test below checks, and the two
// subtests after it are what say **the option decides and the mode only sets
// it** — a grid over three modes agrees with a mode-keyed table on every cell,
// which is the shape a wide grid keyed on the wrong noun has.
type optionGrammarRow struct {
	name string
	src  string
	// refusedOff and refusedOn are the reference's verdicts with the option
	// off and on. A row where they are equal is a control.
	refusedOff, refusedOn bool
}

// ignoreBracesRows: the name stops brace expansion *and* takes away the two
// readings that make a bare brace a reserved word.
var ignoreBracesRows = []optionGrammarRow{
	{"a group closing with no separator", "{ echo A }\n", false, true},
	{"a group closing with no blank either", "{ echo A}; echo st=$?\n", false, true},
	// The row that goes the other way, and the reason this is one reading
	// rather than a rule about groups: with the name on, a `}` that ends a
	// word is text and the line parses where the shell's own mode refuses it.
	{"a close brace ending an argument", "echo A}; echo st=$?\n", true, false},
	{"a group opening with no blank", "{print A; }\n", false, true},
	{"a brace-spelled case closed with a brace", "case x { x) echo hit;; }\n", false, true},
	{"an `in` case closed with a brace", "case x in x) echo one;; }\n", false, true},
	// The controls. The first is the sharpest: the `{` still *opens* a case
	// body with the name on, so what the name took away is the closer and not
	// the opener — a wiring that moved both refuses this line.
	{"a brace-spelled case closed with esac", "case x { x) echo hit;; esac\n", false, false},
	{"a quoted close brace", "echo A\\}\n", false, false},
	{"a close brace in an assignment's value", "x=a}\n", false, false},
	{"a brace expansion", "echo {a,b}\n", false, false},
}

// multiFuncDefRows: the name moves the **parenthesized** spelling's name list
// and leaves the `function` keyword's alone.
var multiFuncDefRows = []optionGrammarRow{
	{"several names before the parens", "a b () { echo \"[$0]\"; }\n", true, false},
	{"a name list of any words", "echo hi () { printf x; }\n", true, false},
	{"a redirection before the parens", "a b >out () { echo \"[$0]\"; }\n", true, false},
	// The controls. The keyword rows are the split — the list after
	// `function` survives the name that takes the other one away — and the
	// one-name row says this is not the parenthesized form going altogether.
	{"one name before the parens", "a () { echo \"[$0]\"; }\n", false, false},
	{"several names after the keyword", "function a b { echo \"[$0]\"; }\n", false, false},
	{"one name after the keyword", "function a { echo \"[$0]\"; }\n", false, false},
}

// shortLoopsRows: the keyword's body is the one place this name reaches that
// is not a loop's, and there only a brace group will do with it off.
var shortLoopsRows = []optionGrammarRow{
	{"no body at all", "function a\n", true, false},
	{"a body of one command", "function a; echo B\n", true, false},
	{"a subshell body", "function a; ( echo B )\n", true, false},
	{"a compound body", "function a; if true; then :; fi\n", true, false},
	{"an and-or list body", "function a; echo X && echo Y\n", true, false},
	// The controls. The first is the separator, which outlives the optional
	// body — a wiring that took the whole of `FunctionKeywordBodyIsOptional`
	// refuses it. The last two are the parenthesized spelling, which keeps
	// its one-command body in every state.
	{"a separator before a brace body", "function a; { echo B; }\n", false, false},
	{"a brace body", "function a { echo B; }\n", false, false},
	{"a one-command body after parens", "f() echo hi\n", false, false},
	{"a brace body after parens", "f() { echo hi; }\n", false, false},
}

func optionMovesTheGrammar(t *testing.T, name string, rows []optionGrammarRow) {
	t.Helper()
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			for _, on := range []bool{false, true} {
				want := row.refusedOff
				if on {
					want = row.refusedOn
				}
				r := caseListRunner(t)
				if code := setOption(r, name, on); code != 0 {
					t.Fatalf("setting %s to %v answered %d", name, on, code)
				}
				if got := !parsesHere(t, r, row.src); got != want {
					t.Errorf("with %s %v the line is refused=%v, want %v", name, on, got, want)
				}
			}
		})
	}
}

// TestIgnoreBracesReachesTheBraceGrammar is the option's own row.
func TestIgnoreBracesReachesTheBraceGrammar(t *testing.T) {
	optionMovesTheGrammar(t, "ignorebraces", ignoreBracesRows)
}

// TestMultiFuncDefReachesTheParenthesizedNameList is the option's own row.
func TestMultiFuncDefReachesTheParenthesizedNameList(t *testing.T) {
	optionMovesTheGrammar(t, "multifuncdef", multiFuncDefRows)
}

// TestShortLoopsReachesTheFunctionKeywordsBody is the option's own row.
func TestShortLoopsReachesTheFunctionKeywordsBody(t *testing.T) {
	optionMovesTheGrammar(t, "shortloops", shortLoopsRows)
}

// emulationGrammarOptions is each name's state after each emulation, from
// emulationDefaults. `emulate zsh` is the table's own default.
var emulationGrammarOptions = []struct {
	name string
	rows []optionGrammarRow
	// on is the state in the four modes, in the order below.
	zsh, sh, ksh, csh bool
}{
	{"ignorebraces", ignoreBracesRows, false, true, false, false},
	{"multifuncdef", multiFuncDefRows, true, false, false, false},
	{"shortloops", shortLoopsRows, true, false, false, true},
}

// TestAnEmulationReachesTheseGrammarsThroughTheOptions is the emulation's row:
// every mode answers every snippet the way its own default for the name does.
func TestAnEmulationReachesTheseGrammarsThroughTheOptions(t *testing.T) {
	for _, o := range emulationGrammarOptions {
		t.Run(o.name, func(t *testing.T) {
			for _, tc := range []struct {
				mode string
				on   bool
			}{
				{"zsh", o.zsh}, {"sh", o.sh}, {"ksh", o.ksh}, {"csh", o.csh},
			} {
				t.Run("emulate "+tc.mode, func(t *testing.T) {
					r := caseListRunner(t)
					applyEmulation(r, tc.mode, false)
					for _, row := range o.rows {
						want := row.refusedOff
						if tc.on {
							want = row.refusedOn
						}
						if got := !parsesHere(t, r, row.src); got != want {
							t.Errorf("%s: refused=%v, want %v", row.name, got, want)
						}
					}
				})
			}
		})
	}
}

// TestTheOptionAndNotTheModeDecidesTheseGrammars holds the mode fixed and
// moves the name, in both directions.
//
// Without these two, every assertion above is equally well explained by a
// table keyed on the mode — and one of those gets `setopt shglob` inside
// `emulate zsh` wrong, silently, which is the failure this repository has been
// caught by more than once.
func TestTheOptionAndNotTheModeDecidesTheseGrammars(t *testing.T) {
	for _, o := range emulationGrammarOptions {
		// A row the name actually moves, so the assertion cannot pass by
		// pointing at a control.
		var moving optionGrammarRow
		for _, row := range o.rows {
			if row.refusedOff != row.refusedOn {
				moving = row
				break
			}
		}
		if moving.src == "" {
			t.Fatalf("%s: no row moves, so this test has nothing to look at", o.name)
		}
		t.Run(o.name+" inside the shell's own mode", func(t *testing.T) {
			r := caseListRunner(t)
			applyEmulation(r, "zsh", false)
			if code := setOption(r, o.name, !o.zsh); code != 0 {
				t.Fatalf("setting %s answered %d", o.name, code)
			}
			want := moving.refusedOff
			if !o.zsh {
				want = moving.refusedOn
			}
			if got := !parsesHere(t, r, moving.src); got != want {
				t.Errorf("%s: refused=%v, want %v — the mode is what decides and not the option",
					moving.name, got, want)
			}
		})
		t.Run(o.name+" inside sh's mode", func(t *testing.T) {
			r := caseListRunner(t)
			applyEmulation(r, "sh", false)
			if code := setOption(r, o.name, !o.sh); code != 0 {
				t.Fatalf("setting %s answered %d", o.name, code)
			}
			want := moving.refusedOff
			if !o.sh {
				want = moving.refusedOn
			}
			if got := !parsesHere(t, r, moving.src); got != want {
				t.Errorf("%s: refused=%v, want %v — the mode is what decides and not the option",
					moving.name, got, want)
			}
		})
	}
}

// optionState reads one name back off the shell, through the same table
// `setopt` and `[[ -o … ]]` read it from.
func namedOptionState(t *testing.T, r *interp.Runner, name string) bool {
	t.Helper()
	o, inverted, ok := resolveOptionName(normalizeOption(name))
	if !ok {
		t.Fatalf("no such option: %s", name)
	}
	return o.get(r) != inverted
}

// TestTheseNamesReadBackWhatTheyWereSet is the other half of a name that is no
// longer merely remembered: a script has to be able to ask.
func TestTheseNamesReadBackWhatTheyWereSet(t *testing.T) {
	for _, o := range emulationGrammarOptions {
		t.Run(o.name, func(t *testing.T) {
			for _, on := range []bool{true, false, true} {
				r := caseListRunner(t)
				if code := setOption(r, o.name, on); code != 0 {
					t.Fatalf("setting %s answered %d", o.name, code)
				}
				if got := namedOptionState(t, r, o.name); got != on {
					t.Errorf("%s reads %v after being set %v", o.name, got, on)
				}
			}
		})
	}
}
