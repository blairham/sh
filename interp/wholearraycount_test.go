// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// runWholeArrayCount runs src under `set -u` with the axes an array read needs
// answered plus the one this suite is about, and a wording with a subject in
// it so that what a refusal *names* is readable from the output.
func runWholeArrayCount(t *testing.T, count WholeArrayCountPolicy, abandons Answer, bare bool, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.ArrayBaseIsZero = Yes
		sem.ArraysAreSparse = Yes
		sem.ArrayScalarIsTheWholeArray = No
		sem.ArrayLengthWithoutSubscriptIsCount = No
		sem.WholeArrayCount = count
		sem.WholeArrayCountRefusalAbandonsTheLine = abandons
		r.Semantics = &sem
		diag := Diagnostics{UnboundVariable: "%s: parameter not set"}
		diag.WholeArrayCountNamesTheBareName = bare
		r.Diagnostics = &diag
	})
}

// The counting answer, which is the base and is what this shell did for every
// dialect before the axis existed: `${#a[@]}` is a number whatever the name
// holds, and `set -u` has nothing to say about it.
//
// Measured 2026-09-18 on ksh93u+ 2012-08-01, `env -i HOME=… PATH=/usr/bin:/bin
// LC_ALL=C` from a script file under `set -u` with `echo after` on the line
// below — every row is a number and `after` runs (#3125).
func TestCountingAWholeArrayAsksNothingOfTheOption(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a name that holds nothing", `set -u; echo "[${#nope[@]}]"; echo after`},
		{"and under the star spelling", `set -u; echo "[${#nope[*]}]"; echo after`},
		{"an array holding elements", `a=(p q); set -u; echo "[${#a[@]}]"; echo after`},
		{"a name holding a string", `x=abc; set -u; echo "[${#x[@]}]"; echo after`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runWholeArrayCount(t, WholeArrayCountCounts, No, false, tc.src)
			if st != 0 || !strings.Contains(out, "after") {
				t.Errorf("got %q status %d, want a number and the next command run", out, st)
			}
		})
	}
}

// The reading that refuses a name holding **nothing at all**, and counts every
// name that holds something — so a string is a length rather than a refusal.
//
// Measured 2026-09-18 on zsh 5.9.2, `env -i HOME=… PATH=/usr/bin:/bin LC_ALL=C`
// from a script file under `set -u`: `${#a[@]}` on an absent name is
// `a[@]: parameter not set` and the shell stops, `x=abc; ${#x[@]}` is `3`, and
// `a=(); ${#a[@]}` is `0`. The last two rows are what draw the line, since a
// rule keyed on "holds no elements" would refuse both of them (#3125).
func TestACountRefusesANameHoldingNothing(t *testing.T) {
	out, st := runWholeArrayCount(t, WholeArrayCountRefusesANameHoldingNothing, No, false,
		`set -u; echo "[${#nope[@]}]"; echo after`)
	if want := "nope[@]: parameter not set"; !strings.Contains(out, want) {
		t.Errorf("got %q, want %q in it", out, want)
	}
	if st == 0 || strings.Contains(out, "after") {
		t.Errorf("got %q status %d, want the shell stopped", out, st)
	}
	for _, tc := range []struct{ name, src, want string }{
		{"an array assigned no elements is counted", `a=(); set -u; echo "[${#a[@]}]"; echo after`, "[0]\nafter\n"},
		{"and an array with elements", `a=(p q); set -u; echo "[${#a[@]}]"; echo after`, "[2]\nafter\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runWholeArrayCount(t, WholeArrayCountRefusesANameHoldingNothing, No, false, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
	// A name holding a string holds *something*, so this reading counts it
	// rather than refusing it — and **what** the number is belongs to the
	// axes that say whether a subscript on a string names a character, which
	// is why the row asserts that the option said nothing and not the count.
	// The dialect binary answering this way prints `3`, measured beside zsh
	// 5.9.2's own `3`.
	if out, st := runWholeArrayCount(t, WholeArrayCountRefusesANameHoldingNothing, No, false,
		`x=abc; set -u; echo "[${#x[@]}]"; echo after`); st != 0 || !strings.Contains(out, "after") {
		t.Errorf("a name holding a string = %q status %d, want it counted and the next command run", out, st)
	}
}

// The wider reading, which refuses every name that holds no **list** — a
// string among them — and which writes the bare name back rather than the
// subscript.
//
// Measured 2026-09-18 on bash 5.3.20 and bash 3.2.57 alike, `env -i HOME=…
// PATH=/usr/bin:/bin LC_ALL=C` from a script file under `set -u`: `${#a[@]}`
// on an absent name and `x=abc; ${#x[@]}` are both `x: unbound variable`,
// against `${x[@]}` on the same line answering `abc`. `a=(); ${#a[@]}` is `0`,
// which is the row that says an assigned-and-empty array is not what the
// refusal is about (#3125).
func TestACountRefusesANameHoldingNoList(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a name that holds nothing", `set -u; echo "[${#nope[@]}]"; echo after`, "nope: parameter not set"},
		{"and under the star spelling", `set -u; echo "[${#nope[*]}]"; echo after`, "nope: parameter not set"},
		{"a name holding a string", `x=abc; set -u; echo "[${#x[@]}]"; echo after`, "x: parameter not set"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runWholeArrayCount(t, WholeArrayCountRefusesANameHoldingNoArray, No, true, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q in it", out, tc.want)
			}
			if st == 0 || strings.Contains(out, "after") {
				t.Errorf("got %q status %d, want the line given up", out, st)
			}
		})
	}
	for _, tc := range []struct{ name, src, want string }{
		{"an array assigned no elements", `a=(); set -u; echo "[${#a[@]}]"; echo after`, "[0]\nafter\n"},
		{"an array with elements", `a=(p q); set -u; echo "[${#a[@]}]"; echo after`, "[2]\nafter\n"},
		{"the whole-array value, which is a different question", `x=abc; set -u; echo "[${x[@]}]"; echo after`, "[abc]\nafter\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runWholeArrayCount(t, WholeArrayCountRefusesANameHoldingNoArray, No, true, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// How much the refusal gives up, which is a second question and splits the two
// columns that refuse at all the other way round from their general answer.
//
// Measured 2026-09-18 from a script file under `set -u` with `a` never set,
// the refusal written as `echo "c=${#a[@]}"; echo SAME` and `echo "NEXT=$?"`
// on the line below: bash 5.3.20 and bash 3.2.57 print neither `c=` nor
// `SAME`, print `NEXT=1`, and exit 0; zsh 5.9.2 prints nothing after the
// sentence and exits 1. The control is `${#a}` on the same unset name, which
// ends the shell in all three (#3125).
func TestWhetherARefusedCountGivesUpTheLineOrTheShell(t *testing.T) {
	const src = "set -u\necho \"c=${#nope[@]}\"; echo SAME\necho NEXT\n"
	out, st := runWholeArrayCount(t, WholeArrayCountRefusesANameHoldingNoArray, Yes, true, src)
	if strings.Contains(out, "SAME") {
		t.Errorf("got %q, want the rest of the line given up", out)
	}
	if !strings.Contains(out, "NEXT") || st != 0 {
		t.Errorf("got %q status %d, want the next line run and a status of 0", out, st)
	}
	out, st = runWholeArrayCount(t, WholeArrayCountRefusesANameHoldingNoArray, No, true, src)
	if strings.Contains(out, "SAME") || strings.Contains(out, "NEXT") {
		t.Errorf("got %q, want the shell stopped", out)
	}
	if st == 0 {
		t.Errorf("status = 0, want a refusal")
	}
}
