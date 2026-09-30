// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A DEBUG action that turns ERR_EXIT **on** skips the command it fired for,
// and spends the option doing it.
//
// Two effects and one rule, which `C05debug.ztst`'s "Skip line from DEBUG
// trap" asserts together: the line does not run, and the line after it sees
// the option back off. Measured 2026-09-30 on zsh 5.9.2.
func TestADebugActionArmingErrExitSkipsTheCommand(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := "fn() {\n" +
		"  setopt localtraps localoptions debugbeforecmd\n" +
		"  trap '(( LINENO == 4 )) && setopt errexit' DEBUG\n" +
		"  print $LINENO three\n" +
		"  print $LINENO four\n" +
		"  print $LINENO five\n" +
		"  [[ -o errexit ]] && print 'ERREXIT is still set'\n" +
		"}\n" +
		"fn\nprint \"status=$?\"\n"
	out, _ := runZsh(t, dir, src)
	want := "3 three\n5 five\nstatus=1\n"
	if out != want {
		t.Errorf("= %q, want %q", out, want)
	}
	// Named separately so a failure says which half broke rather than only
	// that the transcript moved.
	if strings.Contains(out, "4 four") {
		t.Error("line 4 ran, so the arming did not skip the command")
	}
	if strings.Contains(out, "ERREXIT is still set") {
		t.Error("ERR_EXIT survived the skip, so the option was not spent")
	}
}

// And what the skipped command leaves behind: status 0, with the option off.
//
// A row of its own because the transcript above cannot show it — the status
// the *function* reports comes from a later line, so a skip that left a
// different status would still produce the same three lines.
func TestASkippedCommandLeavesZeroAndTheOptionOff(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := "fn() {\n" +
		"  setopt localtraps localoptions debugbeforecmd\n" +
		"  trap '(( LINENO == 4 )) && setopt errexit' DEBUG\n" +
		"  print $LINENO three\n" +
		"  print $LINENO four\n" +
		"  print \"after: st=$? errexit=$([[ -o errexit ]] && print on || print off)\"\n" +
		"}\n" +
		"fn\n"
	out, _ := runZsh(t, dir, src)
	const want = "3 three\nafter: st=0 errexit=off\n"
	if out != want {
		t.Errorf("= %q, want %q", out, want)
	}
}

// **The arming decides and not the action's status**, which is the
// neighboring rule and the control that keeps this one from being "a DEBUG
// action can skip a command". A body of `false` touches nothing and skips
// nothing here.
func TestADebugActionsStatusAloneSkipsNothingHere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := "g() {\n" +
		"  setopt localtraps localoptions debugbeforecmd\n" +
		"  trap '(( LINENO == 3 )) && false' DEBUG\n" +
		"  print a\n" +
		"  print b\n" +
		"}\n" +
		"g\nprint \"status=$?\"\n"
	out, _ := runZsh(t, dir, src)
	const want = "a\nb\nstatus=0\n"
	if out != want {
		t.Errorf("= %q, want %q — a non-zero action skips nothing in this shell", out, want)
	}
}

// An ordinary DEBUG trap never reaches the question, which is what keeps a
// strict core from refusing by name over a rule the script does not use.
// Asserted because the guard is invisible otherwise: every row above arms
// the option, so none of them would notice it going away.
func TestAnOrdinaryDebugTrapDoesNotReachTheArmingQuestion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := "h() {\n" +
		"  setopt localtraps localoptions debugbeforecmd\n" +
		"  trap 'print D' DEBUG\n" +
		"  print one\n" +
		"}\n" +
		"h\n"
	out, st := runZsh(t, dir, src)
	if st != 0 || !strings.Contains(out, "one") {
		t.Errorf("= %q (status %d), want the command to have run", out, st)
	}
	if strings.Contains(out, "disagree here") {
		t.Errorf("= %q, want no axis refusal for a trap that arms nothing", out)
	}
}

// And the arming has to be an arming: with `errexit` **already on** before
// the trap fires, an ordinary action skips nothing.
//
// The row that says the rule is about the option *changing* rather than about
// its state. A reading that fired whenever `errexit` was on during a DEBUG
// firing would skip every command of an `errexit` script that has a trap set
// — which is a very loud bug, and one no other row here would catch, because
// every one of them starts with the option off.
//
// Measured 2026-09-30 on zsh 5.9.2: both commands run and the function
// reports 0.
func TestErrExitAlreadyOnSkipsNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := "fn() {\n" +
		"  setopt localtraps localoptions debugbeforecmd errexit\n" +
		"  trap 'true' DEBUG\n" +
		"  print a\n" +
		"  print b\n" +
		"}\n" +
		"fn\nprint \"status=$?\"\n"
	out, _ := runZsh(t, dir, src)
	const want = "a\nb\nstatus=0\n"
	if out != want {
		t.Errorf("= %q, want %q — the option was already on, so nothing was armed", out, want)
	}
}
