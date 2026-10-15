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

// The other dialect that counts holds one number for a pipeline's last
// element and nothing for a redirected group, measured 2026-10-15 against
// ksh93u+ (#6503). `{v}` numbers are this shell's own table, so the rows are
// exact numbers rather than positions in the kernel's free list.
func TestTheLastPipelineElementHoldsANumber(t *testing.T) {
	for _, c := range []struct {
		own  OwnDescriptorNumbers
		src  string
		want string
	}{
		{LastPipelineElementsInputTakesANumber, `echo x | { exec {v}</dev/null; echo $v; }`, "11"},
		{LastPipelineElementsInputTakesANumber, `true | true | { exec {v}</dev/null; echo $v; }`, "11"},
		{LastPipelineElementsInputTakesANumber, `echo x | ( exec {v}</dev/null; echo $v )`, "11"},
		{LastPipelineElementsInputTakesANumber, `{ exec {v}</dev/null; echo $v; } </dev/null`, "10"},
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

// A command substitution's body in the dialect that holds the last pipeline
// element's input, measured 2026-10-15 against ksh93u+ (#6558): one number
// held for the body, and one more once something needs the body's output as
// a descriptor. Exact `{v}` numbers from a base of 10.
func TestACommandSubstitutionBodyHoldsANumber(t *testing.T) {
	const ksh = LastPipelineElementsInputTakesANumber
	for _, c := range []struct {
		own  OwnDescriptorNumbers
		src  string
		want string
	}{
		{ksh, `exec {v}</dev/null; echo $v`, "10"},
		{ksh, `echo $(exec {v}</dev/null; echo $v)`, "11"},
		{ksh, "echo `exec {v}</dev/null; echo $v`", "11"},
		{ksh, `echo $(echo $(exec {v}</dev/null; echo $v))`, "11"},
		{ksh, `echo $( (exec {v}</dev/null; echo $v) )`, "11"},
		{ksh, `exec {a}</dev/null; echo $a $(exec {v}</dev/null; echo $v)`, "10 12"},
		{ksh, `echo x | { echo $(exec {v}</dev/null; echo $v); }`, "12"},
		// The body's output, needed as a descriptor.
		{ksh, `echo $(/bin/sh -c :; exec {v}</dev/null; echo $v)`, "12"},
		{ksh, `echo $(/bin/sh -c :; /bin/sh -c :; exec {v}</dev/null; echo $v)`, "12"},
		{ksh, `echo $(exec {v}</dev/null; /bin/sh -c :; exec {w}</dev/null; echo $v $w)`, "11 13"},
		{ksh, `echo $( (/bin/sh -c :); exec {v}</dev/null; echo $v)`, "12"},
		{ksh, `echo $(echo x | read y; exec {v}</dev/null; echo $v)`, "12"},
		{ksh, `echo $(true >/dev/null; exec {v}</dev/null; echo $v)`, "12"},
		{ksh, `echo $(echo hi 2>&1; exec {v}</dev/null; echo $v)`, "hi 12"},
		{ksh, `echo $(true 2>/dev/null; exec {v}</dev/null; echo $v)`, "11"},
		{ksh, `echo $(true </dev/null; exec {v}</dev/null; echo $v)`, "11"},
		// Seen by a body nested in it; a nested body's own is gone with it.
		{ksh, `echo $(/bin/sh -c :; echo $(exec {v}</dev/null; echo $v))`, "12"},
		{ksh, `echo $(echo $(/bin/sh -c :); exec {v}</dev/null; echo $v)`, "11"},
		// A process substitution's body holds none of it, but one made
		// inside a command substitution's body holds that body's numbers,
		// and making it is a fork, which needs the output.
		{ksh, `cat <(exec {v}</dev/null; echo $v)`, "10"},
		{ksh, `echo $(cat <(exec {v}</dev/null; echo $v))`, "12"},
		{ksh, `echo $(: <(:); exec {w}</dev/null; echo $w)`, "12"},
		{ksh, `echo $(cat <(echo $(/bin/sh -c :; exec {v}</dev/null; echo $v)))`, "13"},
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
