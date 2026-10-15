// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"context"
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
	out, errs := &strings.Builder{}, &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: errs,
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return out.String(), errs.String()
}
