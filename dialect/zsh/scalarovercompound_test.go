// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A scalar assigned over a name holding an array or a table replaces the name
// outright: the value is the whole of it and the compound is gone.
//
// Measured against zsh 5.9.2 (2026-09-09), line for line. `typeset -p` is the
// instrument, because it is the only read that separates the two hypotheses:
// `$v` answers `x` whether the array was replaced or the value merely landed
// on its first element, since a plain reference to a one-element array spells
// the same thing. Real zsh's `${(t)v}` says `scalar` at each of these points
// and would be the sharper instrument still — it is not implemented here yet
// (#1657), so the listing stands in for it.
//
// bash and ksh93 keep the array and write element 0, which is why this is
// Semantics.ScalarAssignedOverACompoundReplacesTheName and not a rule.
func TestAScalarAssignedOverAnArrayLeavesAPlainScalar(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `v=(a b c)
v=x
print -r -- "assign: [$v] n=${#v}"
typeset -p v
typeset -a w
w=(p q)
w=y
typeset -p w
typeset -A m
m=(k1 v1)
m=z
typeset -p m`)
	want := "assign: [x] n=1\ntypeset v=x\ntypeset w=y\ntypeset m=z\n"
	if out != want || st != 0 {
		t.Errorf("scalar over a compound = %q (status %d), want %q", out, st, want)
	}
}

// The loop variable of a `for` is set by that same rule, which is the whole
// reason the axis lives at the store rather than at the assignment statement.
//
// This shell's own `compinit` is what found it: `_i_line` is filled as an
// array by `IFS=$' \t' read -rA _i_line < $_i_file` while it walks the
// completion files, and then reused as the scalar of the loop that rebinds the
// eight standard completion widgets. All eight passes saw the last file read
// instead of the widget name, so `zle -C $_i_line .$_i_line _main_complete`
// complained about the same invalid widget eight times and none of the eight
// was bound (#1645).
//
// The body's read is asserted and not only the listing afterwards: a fix at
// the end of the loop would leave every pass wrong and the last line right.
func TestAForLoopReplacesAnArrayItsVariableWasHolding(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `v=(a b c)
for v in x y z; do print -rn -- "[$v n=${#v}]"; done
print
typeset -p v`)
	want := "[x n=1][y n=1][z n=1]\ntypeset v=z\n"
	if out != want || st != 0 {
		t.Errorf("for over an array name = %q (status %d), want %q", out, st, want)
	}
}

// And the compinit shape itself, in miniature: a name filled as an array by
// `read -A` and then reused as a loop variable. The first listing is what
// makes the second one mean something — without it a loop that never saw an
// array would pass too.
func TestANameReadAsAnArrayIsReplacedByALaterLoop(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `line=()
print -r -- "one two three" | read -rA line
typeset -p line
for line in alpha beta; do print -rn -- "[$line]"; done
print
typeset -p line`)
	want := "typeset -a line=( one two three )\n[alpha][beta]\ntypeset line=beta\n"
	if out != want || st != 0 {
		t.Errorf("read -A then a loop = %q (status %d), want %q", out, st, want)
	}
}

// `read` into a name holding an array collapses it the same way, and so does
// an assigning expansion — the two other spellings that reach the store
// without going through an assignment statement.
func TestReadAndAnAssigningExpansionReplaceAnArray(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `a=(1 2 3)
read a <<< "zz"
typeset -p a
b=(1 2 3)
print -r -- "subst: ${b::=x y}"
typeset -p b`)
	want := "typeset a=zz\nsubst: x y\ntypeset b='x y'\n"
	if out != want || st != 0 {
		t.Errorf("read and ::= over an array = %q (status %d), want %q", out, st, want)
	}
}

// A prefix assignment is taken back whole, array and all — it is transient in
// this shell exactly as it is elsewhere, and the name that was an array before
// the command is an array after it.
func TestAPrefixAssignmentLeavesTheArrayStanding(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `a=(p q)
a=x true
typeset -p a`)
	want := "typeset -a a=( p q )\n"
	if out != want || st != 0 {
		t.Errorf("prefix over an array = %q (status %d), want %q", out, st, want)
	}
}
