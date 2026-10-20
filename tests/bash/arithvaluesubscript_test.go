// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"testing"
)

// An arithmetic expression that arrives through a variable, still holding a
// `$name` inside a subscript, reads that name (#3303).
//
// `(( a[$key] ))` written out was always right: the expression is expanded
// before it is parsed, so `$key` is gone by the time the subscript is read.
// The case that was wrong is the one where `$key` survives that first
// expansion by arriving inside another parameter's value — `e='a[$key]';
// $(( $e ))` — and the subscript then still holds a dollar sign. That came to
// 0 at status 1 here.
//
// Measured 2026-09-16, each probe from a script file with standard input on
// /dev/null: bash 5.3.20, bash 3.2.57, zsh 5.9.2 and ksh93u+ 2012-08-01 all
// answer 5 on the association and the indexed element on the list, so this is
// a plain bug and not an axis. zsh answers 9 rather than 8 on the indexed row
// only because its arrays are 1-based, which is ArrayBaseIsZero's question and
// not this one.
func TestAnArithmeticValueResolvesTheNameInItsSubscript(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"an association through a value", `declare -A a; a[k]=5; key=k; e='a[$key]'; echo $(( $e ))`, "5"},
		{"an indexed element through a value", `a=(9 8 7); n=1; e='a[$n]'; echo $(( $e ))`, "8"},
		{"the double-paren spelling", `declare -A a; a[k]=5; key=k; e='a[$key]'; (( r = $e )); echo $r`, "5"},
		// The control: the ordinary written case, right before and after. A
		// test that only held this row would have passed on the bug.
		{"written out, the ordinary case", `declare -A a; a[k]=5; key=k; echo $(( a[$key] ))`, "5"},
		// The name is read once and its value used, not re-read as text: a
		// value holding another name's spelling is that spelling.
		{"the value is a key, not a name", `declare -A a; a[k]=5; a[key]=6; key=k; e='a[$key]'; echo $(( $e ))`, "5"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := answersRun(t, c.src); out != c.want+"\n" || st != 0 {
				t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// The line this change must not move: a `$` that arrived *inside* a
// subscript's brackets, carried by a value, is a character of the key and
// begins nothing.
//
// #3303 changes exactly the question of whether a `$` in arithmetic text is an
// expansion, and that is the seam #3047 closed as a P1: `k='$(cmd)'; a[$k]=V`
// used to run `cmd`, which is arbitrary command execution from data in any
// `counts[$key]=1`. So the resolution added for #3303 has to stay on the
// unmarked `$` a script wrote and never reach the marked one a value carried.
//
// Measured 2026-09-16 by effect, and the panel is NOT unanimous on every row:
//
//	row                                  bash 5.3  bash 3.2  zsh 5.9.2  ksh93u+
//	a[$k]=V            (assignment)      safe      safe      safe       safe
//	$(( a[$k] ))       (arithmetic read) safe      RUNS      RUNS       RUNS
//	e='a[$k]'; $(( $e ))                 safe      safe      safe       safe
//
// #3047's "every panel shell refuses" holds for the assignment and not for the
// arithmetic read, where three of the four references do execute the command.
// This file asserts bash 5.3's answer, which is the column this dialect is
// graded against; it says nothing about the zsh and ksh dialects, whose
// references run the second row.
//
// Asserted by effect, not by output: the file must not exist afterwards. An
// output check alone cannot tell "refused" from "ran and printed nothing".
func TestAValueInsideASubscriptStillRunsNothing(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "RAN")
	for _, c := range []struct{ name, src string }{
		{"an assignment subscript", `k='$(touch ` + marker + `)'; declare -a a; a[$k]=V`},
		{"an arithmetic subscript", `k='$(touch ` + marker + `)'; a=(1 2 3); : $(( a[$k] ))`},
		{"a value carrying the whole expression", `k='$(touch ` + marker + `)'; e='a[$k]'; a=(1 2 3); : $(( $e ))`},
	} {
		t.Run(c.name, func(t *testing.T) {
			_ = os.Remove(marker)
			answersRun(t, c.src)
			if _, err := os.Stat(marker); err == nil {
				t.Fatalf("%s ran the command carried in a value (%s exists) — this is #3047's arbitrary command execution returning", c.src, marker)
			}
		})
	}
}
