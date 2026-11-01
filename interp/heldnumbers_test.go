// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The numbers a shell holds for its own plumbing are in the numbers it
// publishes, under Semantics.OwnDescriptors (#6473).
//
// Every row was measured against zsh 5.9.2 run as `zsh -fc` on 2026-10-15 and
// is written as positions in the list of numbers free from the base up, so
// that `{ echo <(true) <(true); } </dev/null` reading `1 2` says the shell
// stepped over exactly one number — zsh's `12 13` against the `11 12` it
// answers with no redirection. The rows vary one thing at a time: which
// stream, whether it was open, how many redirections name it, whether the
// redirection copies a source, where in a pipeline the command stands, and
// whether a fork comes between.
func TestTheShellsOwnDescriptorsAreInItsNumbers(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	free := freeFdsAlong(t, 10, 200, 8)
	for _, c := range []struct {
		src  string
		want []int
	}{
		// The control: nothing redirected, nothing held.
		{`echo <(true) <(true) <(true)`, []int{0, 1, 2}},
		// One stream saved, then two; and only a stream that was open.
		{`{ echo <(true) <(true) <(true); } </dev/null`, []int{1, 2, 3}},
		{`{ echo <(true) <(true); } 2>/dev/null`, []int{1, 2}},
		{`{ echo <(true) <(true); } </dev/null 2>/dev/null`, []int{2, 3}},
		{`{ echo <(true) <(true); } 3</dev/null`, []int{0, 1}},
		{`exec 3</dev/null; { echo <(true) <(true); } 3</dev/null`, []int{1, 2}},
		// Once per stream, not once per redirection.
		{`{ echo <(true) <(true); } 2>/dev/null 2>/dev/null`, []int{1, 2}},
		// A function call is the same as a group; `exec`'s list is not saved.
		{`f() { echo <(true) <(true); }; f </dev/null`, []int{1, 2}},
		{`exec 0</dev/null; echo <(true) <(true)`, []int{0, 1}},
		// A duplication's source is held while the save is taken.
		{`{ echo <(true) <(true) <(true); } 2>&1`, []int{0, 2, 3}},
		// The pipes' reading ends, and the last element's own input.
		{`echo x | echo <(true)`, []int{1}},
		{`true | true | echo <(true)`, []int{2}},
		{`echo x | { echo <(true) <(true) <(true); }`, []int{0, 2, 3}},
		{`echo x | { echo <(true) <(true) <(true); } 2>/dev/null`, []int{2, 3, 4}},
		{`true | true | { echo <(true) <(true); }`, []int{1, 3}},
		{`echo x | { echo y | { echo <(true) <(true); }; }`, []int{0, 3}},
		// A fork keeps the pipes' ends and drops the saved copies, and a
		// fork's own list saves nothing.
		{`{ ( echo <(true) <(true) ); } </dev/null`, []int{0, 1}},
		{`true | true | { ( echo <(true) <(true) ); }`, []int{1, 2}},
		{`true | true | ( echo <(true) <(true) )`, []int{0, 1}},
		{`{ echo <(true) <(true); } </dev/null & wait`, []int{0, 1}},
		// And a substitution's body holds none of it: zsh 5.9.2 answers
		// `true | true | cat <(echo <(true))` from the bottom of its range,
		// and the same with `read` for `cat`. Written with `read` because a
		// program started here holds a pipe os/exec made for it on the
		// lowest free number while it runs, which made this row answer one
		// high about half the time on Linux — that is #6543's, not this
		// row's.
		{`true | true | read x < <(echo <(true)); echo $x`, []int{0}},
		// A substitution a redirection reads from is a source too; one it
		// writes into is made after the save.
		{`{ echo <(true) <(true); } < <(true)`, []int{0, 2}},
		{`{ echo <(true) <(true); } 3< <(true)`, []int{0, 1}},
		{`exec 3>/dev/null; { echo <(true) <(true); } 3> >(true)`, []int{1, 2}},
		// And every substitution of the list is made before any of its
		// saves, so one written after a save is on its number first (#6505).
		{`{ echo <(true) <(true); } < <(true) 2>/dev/null`, []int{2, 3}},
		{`{ echo <(true) <(true); } 2>/dev/null < <(true)`, []int{0, 3}},
		{`{ echo <(true) <(true); } < <(true) 3< <(true)`, []int{0, 1}},
		{`echo x | { echo <(true) <(true); } 2>/dev/null < <(true)`, []int{0, 3}},
		// A writing substitution is on the second number, its reading end
		// having taken the first and given it back: alone it holds nothing
		// a save steps over, after another save it holds the number between.
		{`{ echo <(true) <(true); } 2> >(true)`, []int{1, 2}},
		{`{ echo <(true) <(true); } 2> >(true) </dev/null`, []int{2, 3}},
		{`{ echo <(true) <(true); } </dev/null 2> >(true)`, []int{1, 3}},
		{`{ echo <(true) <(true); } </dev/null 2>/dev/null 3> >(true)`, []int{1, 3}},
		{`{ echo <(true) <(true); } </dev/null 2>&1 3> >(true)`, []int{1, 2}},
	} {
		t.Run(c.src, func(t *testing.T) {
			got := substNumbers(t, runOwnDescriptors(t, SavedStreamsAndPipeReadEndsTakeNumbers, c.src))
			var want []int
			for _, i := range c.want {
				want = append(want, free[i])
			}
			if !sameFds(got, want) {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}

// A simple command as a forked pipeline element holds one more number from
// its first process substitution's pipe until its words are expanded,
// measured against zsh 5.9.2 (#6530): after that pipe's first end, so a
// writing substitution steps over it as well; seen by a command substitution
// in a later word; given back before a function body runs and before the
// command's own list. A case of its own, in a quiet table of its own, because
// a writing substitution's body that is still finishing is in the table the
// next case counts from (#6701).
func TestAForkedElementsFirstSubstitutionHoldsANumber(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	free := freeFdsAlong(t, 10, 200, 4)
	for _, c := range []struct {
		src  string
		want []int
	}{
		{`echo <(true) <(true) <(true) | read -r x; echo $x`, []int{0, 2, 3}},
		{`echo >(true) >(true) | read -r x; echo $x`, []int{2, 3}},
		{`echo <(true) >(true) | read -r x; echo $x`, []int{0, 3}},
		{`{ echo <(true) <(true); } | read -r x; echo $x`, []int{0, 1}},
		{`echo <(true) $(exec {v}</dev/null; echo $v) | read -r x; echo $x`, []int{0, 2}},
		{`f() { echo <(true) "$@"; }; f <(true) <(true) | read -r x; echo $x`, []int{1, 0, 2}},
		{`f() { echo "$@" $v; }; f <(true) <(true) {v}</dev/null | read -r x; echo $x`, []int{0, 2, 1}},
	} {
		t.Run(c.src, func(t *testing.T) {
			got := substNumbers(t, runOwnDescriptors(t, SavedStreamsAndPipeReadEndsTakeNumbers, c.src))
			var want []int
			for _, i := range c.want {
				want = append(want, free[i])
			}
			if !sameFds(got, want) {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}

// And `exec {v}` steps over the same numbers, because it is the same
// allocation: zsh 5.9.2 answers 12 for both of these, where 11 is free with
// nothing held.
func TestADescriptorVariableStepsOverTheShellsOwn(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	free := freeFdsAlong(t, 10, 200, 4)
	for _, src := range []string{
		`{ exec {v}</dev/null; echo $v; } </dev/null`,
		`: <(true) {v}</dev/null; echo $v`,
	} {
		t.Run(src, func(t *testing.T) {
			got := strings.TrimSpace(runOwnDescriptors(t, SavedStreamsAndPipeReadEndsTakeNumbers, src))
			if want := itoa(free[1]); got != want {
				t.Errorf("got %s, want %s", got, want)
			}
		})
	}
}

// The other answer counts nothing, which is what keeps the rows above from
// being a property of the allocator rather than of the axis.
func TestTheShellsOwnDescriptorsTakeNoNumberWhereTheDialectSaysSo(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	free := freeFdsAlong(t, 10, 200, 4)
	for _, src := range []string{
		`{ echo <(true) <(true); } </dev/null`,
		`echo x | { echo <(true) <(true); }`,
		`true | true | echo <(true) <(true)`,
	} {
		t.Run(src, func(t *testing.T) {
			got := substNumbers(t, runOwnDescriptors(t, OwnDescriptorsTakeNoNumber, src))
			if want := free[:2]; !sameFds(got, want) {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}

// A list's substitutions are made before its first redirection is applied,
// so one whose redirection is never reached has still been started: zsh 5.9.2
// prints `ran` for this, bash 5.3.20 and ksh93u+ do not. A command
// substitution is not made early — `>$(echo ran)` after the failure prints
// nothing in all three.
func TestARedirectionListsSubstitutionsAreMadeFirst(t *testing.T) {
	for _, c := range []struct {
		own OwnDescriptorNumbers
		src string
		ran bool
	}{
		{SavedStreamsAndPipeReadEndsTakeNumbers, `: </nonexistent > >(echo ran); wait`, true},
		{SavedStreamsAndPipeReadEndsTakeNumbers, `: </nonexistent >$(echo ran >&2; echo /dev/null)`, false},
		{OwnDescriptorsTakeNoNumber, `: </nonexistent > >(echo ran); wait`, false},
	} {
		t.Run(c.own.String()+": "+c.src, func(t *testing.T) {
			out, errs := runOwnDescriptorsAllowingErrors(t, c.own, c.src)
			if ran := strings.Contains(out+errs, "ran"); ran != c.ran {
				t.Errorf("ran: got %v, want %v (stdout %q, stderr %q)", ran, c.ran, out, errs)
			}
			if !strings.Contains(errs, "nonexistent") {
				t.Errorf("stderr %q, want the failed open", errs)
			}
		})
	}
}

// The other dialect that counts holds one number for a pipeline's last
// element and nothing for a redirected group, measured 2026-10-15 against
// ksh93u+ (#6503). `{v}` numbers are this shell's own table, so the rows are
// exact numbers rather than positions in the kernel's free list.
func TestTheLastPipelineElementHoldsANumber(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		own  OwnDescriptorNumbers
		src  string
		want string
	}{
		{LastPipelineElementsInputTakesANumber, `echo x | { exec {v}</dev/null; echo $v; }`, "11"},
		{LastPipelineElementsInputTakesANumber, `true | true | { exec {v}</dev/null; echo $v; }`, "11"},
		// The parentheses hold one more of their own where they are not the
		// last command. See TestAnUnforkedSubshellHoldsANumber.
		{LastPipelineElementsInputTakesANumber, `echo x | ( exec {v}</dev/null; echo $v )`, "12"},
		// Not the last command of a `-c` string here, so the group saves
		// its input. See TestTheLastCommandSavesNothing.
		{LastPipelineElementsInputTakesANumber, `{ exec {v}</dev/null; echo $v; } </dev/null`, "11"},
		{LastPipelineElementsInputTakesANumber, `{ exec {v}</dev/null; } | :; exec {w}</dev/null; echo $w`, "10"},
		// Held while the element runs and let go of after, and only where
		// the shell's input was open.
		{LastPipelineElementsInputTakesANumber, `echo x | { exec {v}</dev/null; echo $v; }; exec {w}</dev/null; echo $w`, "11 10"},
		{LastPipelineElementsInputTakesANumber, `exec 0<&-; echo x | { exec {v}</dev/null; echo $v; }`, "10"},
		// A group holding nothing but another pipeline holds it once.
		{LastPipelineElementsInputTakesANumber, `echo x | { echo y | { exec {v}</dev/null; echo $v; }; }`, "11"},
		{LastPipelineElementsInputTakesANumber, `echo x | { echo y | { exec {v}</dev/null; echo $v; }; :; }`, "12"},
		{LastPipelineElementsInputTakesANumber, `echo x | { ! echo y | { exec {v}</dev/null; echo $v; }; }; :`, "12"},
		{OwnDescriptorsTakeNoNumber, `echo x | { exec {v}</dev/null; echo $v; }`, "10"},
	} {
		t.Run(c.own.String()+": "+c.src, func(t *testing.T) {
			sem := permissive()
			sem.FirstAllocatedDescriptor = AllocateDescriptorsFromTen
			sem.OwnDescriptors = c.own
			sem.LastPipelineElementInCurrentShell = Yes
			out, st := run(t, c.src, withSem(sem))
			if got := strings.Join(strings.Fields(out), " "); got != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q", got, st, c.want)
			}
		})
	}
}

// The dialect that holds a number for the last pipeline element's input
// saves each stream a redirection list replaces, and a move's source, except
// on the last command a `-c` string runs, measured 2026-10-16 against ksh93u+
// (#6814, #6747, #6637). last says the text is the whole of a `-c` string's
// last part. Exact `{v}` numbers from a base of 10. See
// Runner.tailPutsNothingBack and redirHolds.moved.
func TestTheLastCommandSavesNothing(t *testing.T) {
	const g = `{ exec {v}</dev/null; echo $v; }`
	for _, c := range []struct {
		src  string
		last bool
		want string
	}{
		// Each open stream a list replaces, once, and a `{v}` later in the
		// same list steps over it.
		{`true </dev/null {v}</dev/null; echo $v`, true, "11"},
		{`true 2>/dev/null {v}</dev/null; echo $v`, true, "11"},
		{`true </dev/null </dev/null {v}</dev/null; echo $v`, true, "11"},
		{`true </dev/null 2>/dev/null {v}</dev/null; echo $v`, true, "12"},
		{`true {v}</dev/null </dev/null {w}</dev/null; echo $v $w`, true, "10 12"},
		{`true 3</dev/null {v}</dev/null; echo $v`, true, "10"},
		{`exec 3</dev/null; true 3</dev/null {v}</dev/null; echo $v`, true, "11"},
		// No copy of a duplication's source; a move's source is saved.
		{`exec 4</dev/null 9</dev/null; true 4<&9 {v}</dev/null; echo $v`, true, "11"},
		{`exec 4</dev/null 9</dev/null; true 4<&9- {v}</dev/null; echo $v`, true, "12"},
		{`exec 9</dev/null; true 1>&9- {v}</dev/null; echo $v`, true, "12"},
		{`exec 9</dev/null; true 4<&9- {v}</dev/null; echo $v`, true, "11"},
		{`exec 9</dev/null; true {v}<&9-; echo $v`, true, "11"},
		{`exec 9</dev/null; true {v}<&9-; true {w}</dev/null; echo $v $w`, true, "11 10"},
		{`exec 9</dev/null; true {v}<&9; echo $v`, true, "10"},
		// `exec` saves nothing.
		{`exec </dev/null {v}</dev/null; echo $v`, true, "10"},
		{`exec 9</dev/null; exec {v}<&9-; echo $v`, true, "10"},
		// And the last command saves nothing, however it is reached.
		{g + ` </dev/null`, true, "10"},
		{g + ` </dev/null`, false, "11"},
		{g + ` </dev/null; :`, true, "11"},
		{`: ; ` + g + ` </dev/null`, true, "10"},
		{`true && ` + g + ` </dev/null`, true, "10"},
		{g + ` </dev/null && :`, true, "11"},
		{`! ` + g + ` 2>/dev/null`, true, "11"},
		{`if true; then ` + g + ` </dev/null; fi`, true, "10"},
		{`case a in a) ` + g + ` 2>/dev/null;; esac`, true, "10"},
		{`{ ` + g + ` </dev/null; }`, true, "10"},
		{`{ ` + g + ` </dev/null; } 2>/dev/null; :`, true, "12"},
		{`for i in 1; do ` + g + ` </dev/null; done`, true, "11"},
		{`f() { ` + g + ` </dev/null; }; f`, true, "11"},
		{`f() { exec {v}</dev/null; echo $v; }; f 2>/dev/null`, true, "10"},
		{`f() { exec {v}</dev/null; echo $v; }; f 2>/dev/null; :`, true, "11"},
		{`eval '` + g + ` </dev/null'`, true, "10"},
		{`eval '` + g + ` </dev/null'; :`, true, "11"},
		{`echo x | ` + g + ` 2>/dev/null`, true, "11"},
		{`echo x | ` + g + ` 2>/dev/null; :`, true, "12"},
		{`echo x | { ` + g + ` 2>/dev/null; }`, true, "11"},
		{`! echo x | ` + g + ` 2>/dev/null`, true, "12"},
		{`( ` + g + ` 2>/dev/null )`, true, "10"},
		{`( ` + g + ` 2>/dev/null; : )`, true, "11"},
		{g + ` 2>/dev/null | cat; :`, true, "10"},
		{`trap 'echo t' EXIT; ` + g + ` 2>/dev/null`, true, "10 t"},
	} {
		t.Run(fmt.Sprintf("last=%v: %s", c.last, c.src), func(t *testing.T) {
			sem := permissive()
			sem.FirstAllocatedDescriptor = AllocateDescriptorsFromTen
			sem.OwnDescriptors = LastPipelineElementsInputTakesANumber
			sem.LastPipelineElementInCurrentShell = Yes
			sem.FdVariableOutlivesTheCommand = Yes
			sem.RedirectsUseEveryTarget = No
			sem.FdMove = FdMoveRelocates
			out, _ := run(t, c.src, func(r *Runner) {
				r.Semantics = &sem
				r.LastPart = c.last
			})
			if got := strings.Join(strings.Fields(out), " "); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// A fork started inside a command substitution's body holds one number more:
// a pipeline element in itself while it runs, a background job in the body
// from then on, once. Measured 2026-10-16 against ksh93u+ (#6640). Exact
// `{v}` numbers from a base of 10. See Runner.forkInABody.
func TestAForkInABodyHoldsANumber(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`echo $({ exec {v}</dev/null; echo $v; } | cat)`, "13"},
		{`echo $({ exec {v}</dev/null; echo $v; } | cat; exec {w}</dev/null; echo $w)`, "13 12"},
		{`echo $(echo x | { exec {v}</dev/null; echo $v; } | cat)`, "13"},
		{`echo $(echo x | { exec {v}</dev/null; echo $v; })`, "13"},
		{`echo $(true & wait; exec {v}</dev/null; echo $v)`, "13"},
		{`echo $(true & true & wait; exec {v}</dev/null; echo $v)`, "13"},
		{`echo $({ exec {v}</dev/null; echo $v; } & wait)`, "13"},
		{`echo $(echo $(true & wait; exec {v}</dev/null; echo $v))`, "13"},
		{`echo $(true &); exec {v}</dev/null; echo $v`, "10"},
		{`( true & wait; exec {v}</dev/null; echo $v ); :`, "11"},
		{`( { exec {v}</dev/null; echo $v; } | cat ); :`, "11"},
	} {
		t.Run(c.src, func(t *testing.T) {
			sem := permissive()
			sem.FirstAllocatedDescriptor = AllocateDescriptorsFromTen
			sem.OwnDescriptors = LastPipelineElementsInputTakesANumber
			sem.LastPipelineElementInCurrentShell = Yes
			sem.FdVariableOutlivesTheCommand = Yes
			sem.RedirectsUseEveryTarget = No
			out, _ := run(t, c.src, func(r *Runner) {
				r.Semantics = &sem
				r.LastPart = true
			})
			if got := strings.Join(strings.Fields(out), " "); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// The parentheses the dialect runs without a fork hold one number while they
// run, their own list saves as a group's does, an `exec` inside them or inside
// a command substitution's body saves for as long as the body lasts, and a
// fork's last command is a last command. Measured 2026-10-16 against ksh93u+
// (#6638, #6639). Exact `{v}` numbers from a base of 10. See
// Runner.enterParentheses, redirHolds.saveForTheBody and Runner.handForkTail.
func TestAnUnforkedSubshellHoldsANumber(t *testing.T) {
	const g = `{ exec {v}</dev/null; echo $v; }`
	for _, c := range []struct {
		src  string
		last bool
		want string
	}{
		{`( exec {v}</dev/null; echo $v )`, true, "10"},
		{`( exec {v}</dev/null; echo $v ); :`, true, "11"},
		{`( exec {v}</dev/null; echo $v )`, false, "11"},
		{`( ( exec {v}</dev/null; echo $v ) ); :`, true, "11"},
		{`echo $( ( exec {v}</dev/null; echo $v ) )`, true, "11"},
		{`( exec {v}</dev/null; echo $v ) | cat; :`, true, "10"},
		{`( exec {v}</dev/null; echo $v ) & wait; :`, true, "10"},
		{`( ( exec {v}</dev/null; echo $v ) ) | cat; :`, true, "10"},
		{`( ( exec {v}</dev/null; echo $v ); : ) | cat; :`, true, "11"},
		{`echo x | ( exec {v}</dev/null; echo $v ); :`, true, "12"},
		{`echo x | ( exec {v}</dev/null; echo $v )`, true, "11"},
		{`( exec {v}</dev/null; echo $v ) 2>/dev/null; :`, true, "12"},
		{`( exec {v}</dev/null; echo $v ) 3</dev/null; :`, true, "11"},
		{`( exec {v}</dev/null; exec {w}</dev/null; echo $v $w ); :`, true, "11 12"},
		// An `exec` in such a body saves where the stream is open or is
		// where an open lands, once, never standard output, and the body's
		// end gives it back.
		{`( exec 2>/dev/null; exec {v}</dev/null; echo $v ); :`, true, "12"},
		{`( exec 2>/dev/null; exec 2>/dev/null; exec {v}</dev/null; echo $v ); :`, true, "12"},
		{`( exec 0</dev/null; exec 2>/dev/null; exec {v}</dev/null; echo $v ); :`, true, "13"},
		{`( exec {v}</dev/null; exec 2>/dev/null; exec {w}</dev/null; echo $v $w ); :`, true, "11 13"},
		{`( exec 0<&-; exec {v}</dev/null; echo $v ); :`, true, "12"},
		{`( exec 3</dev/null; exec {v}</dev/null; echo $v ); :`, true, "12"},
		{`( exec 3<&-; exec {v}</dev/null; echo $v ); :`, true, "11"},
		{`( exec 4</dev/null; exec {v}</dev/null; echo $v ); :`, true, "11"},
		{`exec 3</dev/null; ( exec 4</dev/null; exec {v}</dev/null; echo $v ); :`, true, "12"},
		{`exec 5</dev/null; ( exec 5<&-; exec {v}</dev/null; echo $v ); :`, true, "12"},
		{`( exec 4</dev/null; exec 4<&-; exec {v}</dev/null; echo $v ); :`, true, "11"},
		{`( exec >/dev/null; exec {v}</dev/null; echo $v >&2 ); :`, true, "11"},
		{`( exec 2>/dev/null ); exec {v}</dev/null; echo $v`, true, "10"},
		{`echo $(exec 2>/dev/null; exec {v}</dev/null; echo $v)`, true, "12"},
		{`echo $(exec 2>&1; exec {v}</dev/null; echo $v)`, true, "13"},
		{`echo $(exec {v}</dev/null; exec 2>/dev/null; exec {w}</dev/null; echo $v $w)`, true, "11 13"},
		// A fork's last command saves nothing.
		{`{ ` + g + ` 2>/dev/null; } | cat; :`, true, "10"},
		{`{ ` + g + ` 2>/dev/null; :; } | cat; :`, true, "11"},
		{`{ ` + g + ` 2>/dev/null; } & wait; :`, true, "10"},
		{`( ` + g + ` 2>/dev/null ); :`, true, "12"},
	} {
		t.Run(fmt.Sprintf("last=%v: %s", c.last, c.src), func(t *testing.T) {
			sem := permissive()
			sem.FirstAllocatedDescriptor = AllocateDescriptorsFromTen
			sem.OwnDescriptors = LastPipelineElementsInputTakesANumber
			sem.LastPipelineElementInCurrentShell = Yes
			sem.FdVariableOutlivesTheCommand = Yes
			sem.RedirectsUseEveryTarget = No
			sem.FdMove = FdMoveRelocates
			out, _ := run(t, c.src, func(r *Runner) {
				r.Semantics = &sem
				r.LastPart = c.last
			})
			if got := strings.Join(strings.Fields(out), " "); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// The dialect that saves per redirection, measured 2026-10-15 against bash
// 5.3.20 (#6502), beside the per-stream answer on the two shapes that tell
// them apart. Exact `{v}` numbers from a base of 10.
func TestEachRedirectionSavesItsStream(t *testing.T) {
	const (
		each   = EachRedirectionSavesItsStreams
		stream = SavedStreamsAndPipeReadEndsTakeNumbers
	)
	for _, c := range []struct {
		own  OwnDescriptorNumbers
		src  string
		want string
	}{
		{each, `{ exec {v}</dev/null; echo $v; } </dev/null`, "11"},
		{each, `{ exec {v}</dev/null; echo $v; } 2>/dev/null 2>/dev/null`, "12"},
		{stream, `{ exec {v}</dev/null; echo $v; } 2>/dev/null 2>/dev/null`, "11"},
		// No copy of a duplication's source.
		{each, `{ exec {v}</dev/null; echo $v; } 2>&1`, "11"},
		{stream, `{ exec {v}</dev/null; echo $v; } 2>&1`, "10"},
		// Only a stream open when its redirection is reached.
		{each, `{ exec {v}</dev/null; echo $v; } 3</dev/null`, "10"},
		{each, `exec 3</dev/null; { exec {v}</dev/null; echo $v; } 3</dev/null`, "11"},
		{each, `{ exec {v}</dev/null; echo $v; } 3</dev/null 3</dev/null`, "11"},
		{each, `{ exec {v}</dev/null; echo $v; } &>/dev/null; echo $v`, "12"},
		{each, `f() { exec {v}</dev/null; echo $v; } 2>/dev/null; f </dev/null`, "12"},
		// Given back with the command, and none in a fork's own list.
		{each, `{ exec {v}</dev/null; echo $v; } </dev/null; exec {w}</dev/null; echo $w`, "11 10"},
		{each, `( exec {v}</dev/null; echo $v ) </dev/null`, "10"},
		{each, `{ exec {v}</dev/null; echo $v >&2; } </dev/null | :`, "10"},
		{each, `{ exec {v}</dev/null; echo $v; } </dev/null & wait`, "10"},
		{OwnDescriptorsTakeNoNumber, `{ exec {v}</dev/null; echo $v; } 2>/dev/null 2>/dev/null`, "10"},
	} {
		t.Run(c.own.String()+": "+c.src, func(t *testing.T) {
			sem := permissive()
			sem.FirstAllocatedDescriptor = AllocateDescriptorsFromTen
			sem.OwnDescriptors = c.own
			// The second of two redirections of one stream replaces the
			// first, as it does in the shell measured.
			sem.RedirectsUseEveryTarget = No
			out, st := run(t, c.src, withSem(sem))
			if got := strings.Join(strings.Fields(out), " "); got != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q", got, st, c.want)
			}
		})
	}
}

// runOwnDescriptors runs src the way the dialect that counts its own
// descriptors would: substitutions and `{v}` from the base up, the base at
// 10, and a pipeline's last element in the shell itself.
func runOwnDescriptors(t *testing.T, own OwnDescriptorNumbers, src string) string {
	t.Helper()
	out, errs := runOwnDescriptorsAllowingErrors(t, own, src)
	if errs != "" {
		t.Fatalf("stderr: %s", errs)
	}
	return out
}

// runOwnDescriptorsAllowingErrors is runOwnDescriptors for a script that is
// meant to complain, answering what it wrote to each stream.
func runOwnDescriptorsAllowingErrors(t *testing.T, own OwnDescriptorNumbers, src string) (string, string) {
	t.Helper()
	return runOwnDescriptorsReading(t, own, src, nil)
}

// runOwnDescriptorsReading is runOwnDescriptorsAllowingErrors with the
// shell's input given, where stdin is not nil.
func runOwnDescriptorsReading(t *testing.T, own OwnDescriptorNumbers, src string, stdin io.Reader) (string, string) {
	t.Helper()
	d := syntax.Core()
	d.ProcessSubstitution = true
	d.FdVariableRedirections = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	sem.SubstitutionEndPlacement = SubstitutionEndsWhereAnyDescriptorGoes
	sem.FirstAllocatedDescriptor = AllocateDescriptorsFromTen
	sem.OwnDescriptors = own
	sem.LastPipelineElementInCurrentShell = Yes
	sem.FdVariableOutlivesTheCommand = Yes
	// Answered only so that a row starting a background job can be run at
	// all: a substitution's body asks it the moment there is a job.
	sem.SubshellJobTable = SubshellJobsCleared
	out, errs := &strings.Builder{}, &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdin: stdin, Stdout: out, Stderr: errs,
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return out.String(), errs.String()
}

// A `{name}` whose target is this command's process substitution takes the
// substitution's own number, and the other end's number is the next pick's,
// measured 2026-10-15 against zsh 5.9.2 (#6789). In a quiet table, because
// the substitution's number is the kernel's and the name's is this shell's,
// and only there do the two count from the same place. See
// Runner.substitutionANameTakes. From a base of 10 here, where zsh's is 11.
func TestANameTakesItsSubstitutionsNumber(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	for _, c := range []struct{ src, want string }{
		// The two without a name first, while no earlier row's body is
		// still holding a descriptor this process counts.
		{`: < <(true) {w}</dev/null; echo $w`, "10"},
		{`: > >(read -r x) {w}</dev/null; echo $w`, "11"},
		// What `exec` opens outlives the run in this process, so each row
		// closes it again for the rows after it.
		{`exec {v}> >(read -r x); exec {w}</dev/null {x}</dev/null; echo $v $w $x; exec {v}>&- {w}<&- {x}<&-`, "11 10 12"},
		{`exec {v}< <(true); exec {w}</dev/null {x}</dev/null; echo $v $w $x; exec {v}<&- {w}<&- {x}<&-`, "10 11 12"},
		{`: {v}> >(read -r x) {w}</dev/null; echo $v $w`, "11 10"},
		{`: {v}< <(true) {w}</dev/null; echo $v $w`, "10 11"},
		{`: {w}</dev/null {v}< <(true); echo $v $w`, "10 11"},
		{`exec {v}< <(echo hi); read x <&$v; echo $x; exec {v}<&-`, "hi"},
	} {
		t.Run(c.src, func(t *testing.T) {
			out := runOwnDescriptors(t, SavedStreamsAndPipeReadEndsTakeNumbers, c.src)
			if got := strings.Join(strings.Fields(out), " "); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// A command substitution's body in the dialect that holds the last pipeline
// element's input, measured 2026-10-15 against ksh93u+ (#6558): one number
// held for the body, and one more once something needs the body's output as
// a descriptor. Exact `{v}` numbers from a base of 10.
func TestACommandSubstitutionBodyHoldsANumber(t *testing.T) {
	const lastInput = LastPipelineElementsInputTakesANumber
	for _, c := range []struct {
		own  OwnDescriptorNumbers
		src  string
		want string
	}{
		{lastInput, `exec {v}</dev/null; echo $v`, "10"},
		{lastInput, `echo $(exec {v}</dev/null; echo $v)`, "11"},
		{lastInput, "echo `exec {v}</dev/null; echo $v`", "11"},
		{lastInput, `echo $(echo $(exec {v}</dev/null; echo $v))`, "11"},
		{lastInput, `echo $( (exec {v}</dev/null; echo $v) )`, "11"},
		{lastInput, `exec {a}</dev/null; echo $a $(exec {v}</dev/null; echo $v)`, "10 12"},
		{lastInput, `echo x | { echo $(exec {v}</dev/null; echo $v); }`, "12"},
		// The body's output, needed as a descriptor.
		{lastInput, `echo $(/bin/sh -c :; exec {v}</dev/null; echo $v)`, "12"},
		{lastInput, `echo $(/bin/sh -c :; /bin/sh -c :; exec {v}</dev/null; echo $v)`, "12"},
		{lastInput, `echo $(exec {v}</dev/null; /bin/sh -c :; exec {w}</dev/null; echo $v $w)`, "11 13"},
		{lastInput, `echo $( (/bin/sh -c :); exec {v}</dev/null; echo $v)`, "12"},
		{lastInput, `echo $(echo x | read y; exec {v}</dev/null; echo $v)`, "12"},
		{lastInput, `echo $(true >/dev/null; exec {v}</dev/null; echo $v)`, "12"},
		{lastInput, `echo $(echo hi 2>&1; exec {v}</dev/null; echo $v)`, "hi 12"},
		{lastInput, `echo $(true 2>/dev/null; exec {v}</dev/null; echo $v)`, "11"},
		{lastInput, `echo $(true </dev/null; exec {v}</dev/null; echo $v)`, "11"},
		// Seen by a body nested in it; a nested body's own is gone with it.
		{lastInput, `echo $(/bin/sh -c :; echo $(exec {v}</dev/null; echo $v))`, "12"},
		{lastInput, `echo $(echo $(/bin/sh -c :); exec {v}</dev/null; echo $v)`, "11"},
		// A process substitution's body holds none of it, but one made
		// inside a command substitution's body holds that body's numbers,
		// and making it is a fork, which needs the output.
		{lastInput, `cat <(exec {v}</dev/null; echo $v)`, "10"},
		{lastInput, `echo $(cat <(exec {v}</dev/null; echo $v))`, "12"},
		{lastInput, `echo $(: <(:); exec {w}</dev/null; echo $w)`, "12"},
		{lastInput, `echo $(cat <(echo $(/bin/sh -c :; exec {v}</dev/null; echo $v)))`, "13"},
		{OwnDescriptorsTakeNoNumber, `echo $(/bin/sh -c :; exec {v}</dev/null; echo $v)`, "10"},
	} {
		t.Run(c.own.String()+": "+c.src, func(t *testing.T) {
			sem := permissive()
			sem.FirstAllocatedDescriptor = AllocateDescriptorsFromTen
			sem.OwnDescriptors = c.own
			sem.LastPipelineElementInCurrentShell = Yes
			out, st := run(t, c.src, withSem(sem))
			if got := strings.Join(strings.Fields(out), " "); got != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q", got, st, c.want)
			}
		})
	}
}

// A stream redirected again in one list joins its targets or sources in the
// dialect that joins them, and the join holds numbers until the list has
// applied — measured 2026-10-15 against zsh 5.9.2 (#6529), whose base is 11,
// so every number here is one below zsh's. And `exec`'s list saves as any
// list does, given back when it has applied, in both dialects that save.
func TestAJoinHoldsNumbersWhileItsListApplies(t *testing.T) {
	t.Parallel()
	const (
		stream = SavedStreamsAndPipeReadEndsTakeNumbers
		each   = EachRedirectionSavesItsStreams
	)
	for _, c := range []struct {
		own   OwnDescriptorNumbers
		joins Answer
		src   string
		want  string
	}{
		// Sources: two, then one each, after the save.
		{stream, Yes, `: </dev/null </dev/null {v}</dev/null; echo $v`, "13"},
		{stream, Yes, `: </dev/null </dev/null </dev/null {v}</dev/null; echo $v`, "15"},
		{stream, Yes, `: 3</dev/null 3</dev/null {v}</dev/null; echo $v`, "12"},
		{stream, Yes, `: </dev/null <&0 {v}</dev/null; echo $v`, "13"},
		// Targets: three, then one each.
		{stream, Yes, `: >/dev/null >/dev/null {v}</dev/null; echo $v`, "14"},
		{stream, Yes, `: >/dev/null >/dev/null >/dev/null {v}</dev/null; echo $v`, "15"},
		{stream, Yes, `: >/dev/null >&2 {v}</dev/null; echo $v`, "14"},
		// A join of sources owes one more after the next number taken.
		{stream, Yes, `: </dev/null </dev/null {v}</dev/null {w}</dev/null; echo $v $w`, "13 15"},
		{stream, Yes, `: </dev/null </dev/null 2>&1 {v}</dev/null {w}</dev/null; echo $v $w`, "13 16"},
		{stream, Yes, `: </dev/null </dev/null 3</dev/null {v}</dev/null {w}</dev/null; echo $v $w`, "13 15"},
		{stream, Yes, `: 3</dev/null 3</dev/null 3</dev/null {v}</dev/null {w}</dev/null; echo $v $w`, "14 15"},
		{stream, Yes, `: </dev/null </dev/null >/dev/null >/dev/null {v}</dev/null; echo $v`, "18"},
		// Copying a joined stream ends the join; a `{v}`'s number is not
		// the one owed.
		{stream, Yes, `: </dev/null </dev/null 3<&0 {v}</dev/null {w}</dev/null; echo $v $w`, "11 12"},
		{stream, Yes, `: >/dev/null >/dev/null 2>&1 {v}</dev/null {w}</dev/null; echo $v $w`, "11 13"},
		{stream, Yes, `: </dev/null </dev/null {v}<&0 </dev/null {w}</dev/null; echo $v $w`, "11 14"},
		{stream, Yes, `: </dev/null </dev/null {v}<&2 {w}</dev/null; echo $v $w`, "13 15"},
		{stream, Yes, `: </dev/null </dev/null {x}</dev/null 3<&0 {v}</dev/null {w}</dev/null; echo $x $v $w`, "13 11 12"},
		{stream, Yes, `: </dev/null </dev/null {v}</dev/null; exec {v}<&-; exec {a}</dev/null {b}</dev/null {c}</dev/null {d}</dev/null; echo $d`, "13"},
		{stream, Yes, `{ exec {a}</dev/null {b}</dev/null {c}</dev/null {d}</dev/null; echo $d; } </dev/null </dev/null 2>&1`, "14"},
		// Only while the list applies; the pipe is a source of the stream.
		{stream, Yes, `{ exec {a}</dev/null; echo $a; } </dev/null </dev/null`, "11"},
		{stream, Yes, `: </dev/null </dev/null {v}</dev/null; exec {w}</dev/null; echo $v $w`, "13 10"},
		{stream, Yes, `echo x | : </dev/null {v}</dev/null; echo $v`, "13"},
		{stream, No, `: </dev/null </dev/null {v}</dev/null; echo $v`, "11"},
		// `exec`'s list saves while it applies.
		{stream, Yes, `exec </dev/null {v}</dev/null; exec {w}</dev/null; echo $v $w`, "11 10"},
		{stream, Yes, `exec 2>&1 {v}</dev/null {w}</dev/null; echo $v $w`, "10 12"},
		{each, No, `exec </dev/null {v}</dev/null; exec {w}</dev/null; echo $v $w`, "11 10"},
		{each, No, `exec 2>&1 {v}</dev/null {w}</dev/null; echo $v $w`, "11 12"},
		{OwnDescriptorsTakeNoNumber, Yes, `exec </dev/null </dev/null {v}</dev/null; echo $v`, "10"},
	} {
		t.Run(c.own.String()+": "+c.src, func(t *testing.T) {
			sem := permissive()
			sem.FirstAllocatedDescriptor = AllocateDescriptorsFromTen
			sem.OwnDescriptors = c.own
			sem.RedirectsUseEveryTarget = c.joins
			sem.LastPipelineElementInCurrentShell = Yes
			out, st := run(t, c.src, withSem(sem))
			if got := strings.Join(strings.Fields(out), " "); got != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q", got, st, c.want)
			}
		})
	}
}

// A coprocess's ends in the placement that takes the four lowest free numbers
// and keeps the first and the fourth, and what its body holds, measured
// 2026-10-17 against zsh 5.9.2 (#6661). From a base of 10 here, where zsh's
// is 11, and in a quiet table, because a substitution's number is the
// kernel's. See CoprocEndsFirstAndFourthFromTheBase.
func TestACoprocessHoldsTheFirstAndFourth(t *testing.T) {
	if measuredInAQuietDescriptorTable(t) {
		return
	}
	// The rows with a substitution first, while no earlier row's coprocess
	// holds a descriptor this process counts; every body ends by itself.
	for _, c := range []struct{ src, want string }{
		{`coproc echo <(true) <(true); read -r l <&${COPROC[0]}; echo $l`, "/dev/fd/14 /dev/fd/15"},
		{`coproc C { echo <(true) <(true); }; read -r l <&${C[0]}; echo $l`, "/dev/fd/11 /dev/fd/12"},
		{`coproc C { :; }; exec {a}</dev/null {b}</dev/null {c}</dev/null; echo $a $b $c`, "11 12 14"},
		{`exec {x}</dev/null; coproc C { :; }; exec {a}</dev/null {b}</dev/null; echo $x $a $b`, "10 12 13"},
		{`coproc C { :; }; coproc C { :; }; exec {a}</dev/null {b}</dev/null {c}</dev/null; echo $a $b $c`, "11 12 14"},
		{`coproc C { exec {a}</dev/null {b}</dev/null; echo $a $b; }; read -r l <&${C[0]}; echo $l`, "11 12"},
		{`coproc C { exec {a}</dev/null; echo $a; } </dev/null; read -r l <&${C[0]}; echo $l`, "11"},
	} {
		t.Run(c.src, func(t *testing.T) {
			d := syntax.Core()
			d.Coproc, d.CoprocName = true, true
			d.ProcessSubstitution, d.FdVariableRedirections = true, true
			f, err := syntax.Parse(c.src, d)
			if err != nil {
				t.Fatal(err)
			}
			sem := PosixSemantics()
			sem.CoprocEndsInAnArray = Yes
			sem.CoprocessEndPlacement = CoprocEndsFirstAndFourthFromTheBase
			sem.SubstitutionEndPlacement = SubstitutionEndsWhereAnyDescriptorGoes
			sem.FirstAllocatedDescriptor = AllocateDescriptorsFromTen
			sem.OwnDescriptors = SavedStreamsAndPipeReadEndsTakeNumbers
			sem.FdVariableOutlivesTheCommand = Yes
			sem.SubshellJobTable = SubshellJobsCleared
			sem.RedirectTargetIsAnOrdinaryWord = No
			out := &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
				Stdout: out, Stderr: out, Env: testPATH(),
			})
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(strings.Fields(out.String()), " "); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
