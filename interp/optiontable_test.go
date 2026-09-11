// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A dialect whose option namespace is its own supplies the whole of what
// `set -o` and `set +o` write, and what `set -o name` moves (#1080).
//
// The seam is Runner.SetOptionTable, and these tests name it rather than a
// shell — a table of five made-up names, so what is asserted is that the rows
// come from the dialect and not that any particular shell has any particular
// option. Without it, a shell whose `set -o` names are its own listed the
// substrate's shared two dozen instead: a plausible table, at status 0, in a
// vocabulary that shell does not use for output.

// tableOption is one entry of the miniature dialect's namespace.
type tableOption struct {
	name  string
	on    bool
	fixed bool
}

// optionTableRunner is a shell with the five names below and nothing else.
// The table is held on the closure rather than on the Runner, which is what a
// dialect does with its own store.
func optionTableRunner(t *testing.T, src string, dg Diagnostics, sem *Semantics) (string, int) {
	t.Helper()
	table := []*tableOption{
		{name: "noalpha", on: false},
		{name: "beta", on: true},
		{name: "gamma", on: false},
		{name: "nodelta", on: false, fixed: true},
		{name: "epsilon", on: false, fixed: true},
	}
	find := func(name string) *tableOption {
		for _, o := range table {
			if o.name == name {
				return o
			}
		}
		return nil
	}
	var buf strings.Builder
	s := PosixSemantics()
	if sem != nil {
		s = *sem
	}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &s, Diagnostics: &dg, Name: "sh"})
	r.SetOptionTable(
		func(*Runner) []ListedOption {
			rows := make([]ListedOption, 0, len(table))
			for _, o := range table {
				rows = append(rows, ListedOption{Name: o.name, On: o.on})
			}
			return rows
		},
		func(_ *Runner, name string, on bool) (moved, known bool) {
			o := find(name)
			if o == nil {
				return false, false
			}
			if o.fixed {
				// Granted where it is already where it is asked to be, and
				// refused otherwise — with nothing said, because the sentence
				// is the substrate's on all three routes.
				return o.on == on, true
			}
			o.on = on
			return true, true
		},
	)
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return buf.String(), st
}

// notFatal is the answer these cases need for everything except the one that
// is about ending the script: a refusal that stopped the shell could not
// print what it reported.
func notFatal() *Semantics {
	s := PosixSemantics()
	s.BadSetOptionNameFatal = No
	return &s
}

// TestTheDialectsOwnNamesAreWhatSetOListsThem: both listings, by byte. The
// order is the table's and not sorted here, because the spelling and the
// order are one measurement a dialect makes and this package has no way to
// re-derive either.
func TestTheDialectsOwnNamesAreWhatSetOLists(t *testing.T) {
	out, st := optionTableRunner(t, "set -o\n", Diagnostics{}, nil)
	want := "noalpha        off\n" +
		"beta           on\n" +
		"gamma          off\n" +
		"nodelta        off\n" +
		"epsilon        off\n"
	if out != want || st != 0 {
		t.Errorf("stdout = %q status %d, want exactly %q at 0", out, st, want)
	}

	out, st = optionTableRunner(t, "set +o\n", Diagnostics{}, nil)
	want = "set +o noalpha\nset -o beta\nset +o gamma\nset +o nodelta\nset +o epsilon\n"
	if out != want || st != 0 {
		t.Errorf("stdout = %q status %d, want exactly %q at 0", out, st, want)
	}
}

// TestTheSubstratesOwnNamesAreGoneFromTheListing: the whole of the bug. A
// shell with a table of its own must not still write the shared names — a
// listing is a capture surface, and one holding the wrong vocabulary at
// status 0 is the silent kind of wrong answer.
func TestTheSubstratesOwnNamesAreGoneFromTheListing(t *testing.T) {
	out, _ := optionTableRunner(t, "set +o\n", Diagnostics{}, nil)
	for _, name := range []string{"allexport", "errexit", "noclobber", "xtrace", "nounset"} {
		if strings.Contains(out, name) {
			t.Errorf("stdout = %q, want no mention of the substrate's %s", out, name)
		}
	}
}

// TestSetOMovesTheDialectsOwnName: and the listing shows it afterwards, which
// is what makes the two halves one namespace rather than two.
func TestSetOMovesTheDialectsOwnName(t *testing.T) {
	out, st := optionTableRunner(t, "set -o gamma\necho \"st=$?\"\nset +o\n", Diagnostics{}, nil)
	want := "st=0\nset +o noalpha\nset -o beta\nset -o gamma\nset +o nodelta\nset +o epsilon\n"
	if out != want || st != 0 {
		t.Errorf("stdout = %q status %d, want exactly %q at 0", out, st, want)
	}

	// And back off, so the direction is read rather than assumed.
	out, _ = optionTableRunner(t, "set +o beta\nset +o\n", Diagnostics{}, nil)
	if !strings.Contains(out, "set +o beta\n") {
		t.Errorf("stdout = %q, want `beta` off afterwards", out)
	}
}

// TestANameTheDialectDoesNotHaveIsAnInvalidName: known false is the "typo"
// answer, worded and scored by the dialect exactly as it is without a table.
func TestANameTheDialectDoesNotHaveIsAnInvalidName(t *testing.T) {
	dg := Diagnostics{SetInvalidOptionName: "set: no such option: %[1]s", SetInvalidOptionStatus: 1}
	out, st := optionTableRunner(t, "set -o zzznosuch\necho \"st=$?\"\n", dg, notFatal())
	if want := "sh: set: no such option: zzznosuch\nst=1\n"; out != want {
		t.Errorf("output = %q, want exactly %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want the script to have carried on to the echo", st)
	}
}

// TestANameTheDialectWillNotMoveIsADifferentAnswer: known true and moved
// false, which is a shell that is missing something rather than a typo — and
// the two must not be one sentence, because a script can act on the
// difference.
func TestANameTheDialectWillNotMoveIsADifferentAnswer(t *testing.T) {
	dg := Diagnostics{
		SetInvalidOptionName:   "set: no such option: %[1]s",
		SetImmovableOptionName: "set: can't change option: %[1]s",
		SetInvalidOptionStatus: 1,
	}
	out, _ := optionTableRunner(t, "set -o epsilon\necho \"st=$?\"\n", dg, notFatal())
	if want := "sh: set: can't change option: epsilon\nst=1\n"; out != want {
		t.Errorf("output = %q, want exactly %q", out, want)
	}

	// Asking a fixed name for the state it is already in is granted and
	// silent, which is the same bargain the substrate's own table strikes.
	out, _ = optionTableRunner(t, "set +o epsilon\necho \"st=$?\"\n", dg, notFatal())
	if want := "st=0\n"; out != want {
		t.Errorf("output = %q, want exactly %q", out, want)
	}
}

// TestAnImmovableNameEndsTheScriptWhereTheDialectSaysSo: the refusal is the
// dialect's on every axis a refused `set -o` already has, fatality included.
func TestAnImmovableNameEndsTheScriptWhereTheDialectSaysSo(t *testing.T) {
	dg := Diagnostics{SetImmovableOptionName: "set: can't change option: %[1]s", SetInvalidOptionStatus: 1}
	fatal := PosixSemantics()
	fatal.BadSetOptionNameFatal = Yes
	out, st := optionTableRunner(t, "set -o epsilon\necho after\n", dg, &fatal)
	if want := "sh: set: can't change option: epsilon\n"; out != want {
		t.Errorf("output = %q, want exactly %q — nothing after it", out, want)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// TestTheDialectsTableDoesNotSwallowTheSubstratesOwnSeam: the recursion
// guard, and it is load-bearing rather than defensive. A dialect's entries
// are written in terms of the substrate's — its `err_exit` moves the
// substrate's `errexit` through ApplyNamedOption — so if that seam went
// through the table as well, the dialect's own option builtin would call back
// into the table it was called from and never return.
func TestTheDialectsTableDoesNotSwallowTheSubstratesOwnSeam(t *testing.T) {
	var buf strings.Builder
	sem := PosixSemantics()
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "sh"})
	moves := 0
	r.SetOptionTable(
		func(*Runner) []ListedOption { return nil },
		func(*Runner, string, bool) (bool, bool) { moves++; return false, false },
	)
	// A substrate name, through the seam a dialect's option builtin uses.
	if code := r.ApplyNamedOption("errexit", true); code != 0 {
		t.Errorf("ApplyNamedOption = %d, want 0 — the substrate's own table answers it", code)
	}
	if moves != 0 {
		t.Errorf("the dialect's mover was asked %d times, want 0", moves)
	}
	if on, known := r.NamedOption("errexit"); !on || !known {
		t.Errorf("NamedOption = %v %v, want it on and known", on, known)
	}
}

// TestAShellWithNoTableIsUnchanged: three of the four presets install none,
// and every one of the cases above has to be a no-op for them.
func TestAShellWithNoTableIsUnchanged(t *testing.T) {
	out, st := run(t, "set +o\n", withExtras)
	if !strings.Contains(out, "set +o allexport\n") || !strings.Contains(out, "set +o errexit\n") {
		t.Errorf("stdout = %q, want the substrate's own names listed", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}
