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

// `set` reports every option word it cannot use before it gives up, where the
// dialect says so, and stops at the first everywhere else.
//
// The whole line is compared and not a substring of it, which is the point of
// the axis: a refusal that reports one word is a *prefix* of one that reports
// two, so `strings.Contains` on the first sentence passes under both answers
// and could not tell them apart. What separates them is the second line and
// where the usage block sits, so the assertion is the rendered block, line
// endings included.
func setEveryRun(t *testing.T, src string, every, fatal Answer) (string, int) {
	t.Helper()
	sem := PosixSemantics()
	sem.SetReportsEveryBadOption = every
	sem.BadSetOptionNameFatal = fatal
	sem.FatalErrorStatusIsOne = No
	dg := Diagnostics{
		// The shell's name still stands in front of each sentence, which is
		// why every expectation below carries it: the assertion is the whole
		// rendered block and not a substring of it.
		Location:               LocationNone,
		SetInvalidOptionLetter: "set: %[2]s: unknown option",
		SetInvalidOptionName:   "set: %[1]s: bad option(s)",
		SetInvalidOptionStatus: 2,
		BuiltinUsage:           map[string]string{"set": "Usage: set [-abc]"},
		BuiltinUsageUnprefixed: true,
	}
	var buf strings.Builder
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "sh",
	})
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

func TestSetReportsEveryBadOptionWordOrStopsAtTheFirst(t *testing.T) {
	for _, tc := range []struct {
		why, src string
		every    string
		first    string
	}{
		{
			why:   "two bad letters written as separate words",
			src:   "set -q -z\n",
			every: "sh: set: q: unknown option\nsh: set: z: unknown option\nUsage: set [-abc]\n",
			first: "sh: set: q: unknown option\nUsage: set [-abc]\n",
		},
		{
			// The open question the issue left: a bundle is one word and
			// several letters, and the unit is the letter.
			why:   "two bad letters in one bundle",
			src:   "set -qz\n",
			every: "sh: set: q: unknown option\nsh: set: z: unknown option\nUsage: set [-abc]\n",
			first: "sh: set: q: unknown option\nUsage: set [-abc]\n",
		},
		{
			why:   "three of them, so it is every one and not two",
			src:   "set -q -z -y\n",
			every: "sh: set: q: unknown option\nsh: set: z: unknown option\nsh: set: y: unknown option\nUsage: set [-abc]\n",
			first: "sh: set: q: unknown option\nUsage: set [-abc]\n",
		},
		{
			why:   "long names count too",
			src:   "set -o nosuch -o alsobad\n",
			every: "sh: set: nosuch: bad option(s)\nsh: set: alsobad: bad option(s)\n",
			first: "sh: set: nosuch: bad option(s)\n",
		},
		{
			// Letters and names interleave in the order they were written,
			// which says one loop reports them rather than two passes.
			why:   "a name between two letters, in argument order",
			src:   "set -q -o nosuch -z\n",
			every: "sh: set: q: unknown option\nsh: set: nosuch: bad option(s)\nsh: set: z: unknown option\nUsage: set [-abc]\n",
			first: "sh: set: q: unknown option\nUsage: set [-abc]\n",
		},
		{
			why:   "one bad word answers the same either way",
			src:   "set -q\n",
			every: "sh: set: q: unknown option\nUsage: set [-abc]\n",
			first: "sh: set: q: unknown option\nUsage: set [-abc]\n",
		},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if got, _ := setEveryRun(t, tc.src, Yes, No); got != tc.every {
				t.Errorf("reporting every bad word:\n got %q\nwant %q", got, tc.every)
			}
			if got, _ := setEveryRun(t, tc.src, No, No); got != tc.first {
				t.Errorf("stopping at the first:\n got %q\nwant %q", got, tc.first)
			}
		})
	}
}

// The usage block is printed **once**, after all of them, and not one per
// word. Its own test because it is the half that does not fall out of
// "keep looping": the block is written by the same helper that writes each
// sentence, so a loop alone would repeat it.
func TestTheUsageBlockIsPrintedOnceAfterEveryRefusal(t *testing.T) {
	got, _ := setEveryRun(t, "set -q -z -y\n", Yes, No)
	if n := strings.Count(got, "Usage:"); n != 1 {
		t.Errorf("printed %d usage blocks, want exactly 1: %q", n, got)
	}
	if !strings.HasSuffix(got, "Usage: set [-abc]\n") {
		t.Errorf("output %q, want the usage block last", got)
	}
}

// The fatality ends the script **after** the reports rather than instead of
// the words behind the first. This is what the axis is for: three of the
// panel print one line because the refusal is fatal there and the loop never
// reaches the second, and the shell that answers yes here is fatal too and
// still prints them all.
func TestTheFatalityIsAppliedAfterEveryWordIsReported(t *testing.T) {
	const want = "sh: set: q: unknown option\nsh: set: z: unknown option\nUsage: set [-abc]\n"
	got, st := setEveryRun(t, "set -q -z\necho after\n", Yes, Yes)
	if got != want {
		t.Errorf("fatal, reporting every word:\n got %q\nwant %q", got, want)
	}
	if st != 2 {
		t.Errorf("status %d, want the refusal's 2", st)
	}
	// The control: not fatal, and `after` runs behind the same two lines.
	got, st = setEveryRun(t, "set -q -z\necho after\n", Yes, No)
	if got != want+"after\n" {
		t.Errorf("not fatal:\n got %q\nwant %q", got, want+"after\n")
	}
	if st != 0 {
		t.Errorf("status %d, want the echo's 0", st)
	}
}

// A `set` that reported a bad word sets nothing, which is the same in both
// answers and is what keeps "carry on reading the words" from becoming
// "carry on applying them".
func TestASetThatRefusedAWordChangesNothing(t *testing.T) {
	for _, every := range []Answer{Yes, No} {
		out, _ := setEveryRun(t, "set -q -z one two\necho \"n=$#\"\n", every, No)
		if !strings.HasSuffix(out, "n=0\n") {
			t.Errorf("every=%v: output %q, want no positional parameters set", every, out)
		}
	}
}

// The usage block is owed by the refusal that wanted one and not by the fact
// that something was refused. A dialect that prints none under a bad `-o`
// name gets none, even when it reports every word — which the one shell
// answering this axis cannot show, because it prints one under both.
func TestOnlyARefusalThatWantsAUsageBlockGetsOne(t *testing.T) {
	sem := PosixSemantics()
	sem.SetReportsEveryBadOption = Yes
	sem.BadSetOptionNameFatal = No
	sem.FatalErrorStatusIsOne = No
	base := Diagnostics{
		Location:               LocationNone,
		SetInvalidOptionLetter: "set: %[2]s: unknown option",
		SetInvalidOptionName:   "set: %[1]s: bad option(s)",
		SetInvalidOptionStatus: 2,
		BuiltinUsage:           map[string]string{"set": "Usage: set [-abc]"},
		BuiltinUsageUnprefixed: true,
	}
	withName := base
	withName.SetInvalidOptionNameUsage = true

	run := func(src string, dg Diagnostics) string {
		t.Helper()
		var buf strings.Builder
		r := newTestRunner(t, &Runner{
			Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "sh",
		})
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}

	const names = "set -o nosuch -o alsobad\n"
	want := "sh: set: nosuch: bad option(s)\nsh: set: alsobad: bad option(s)\n"
	if got := run(names, base); got != want {
		t.Errorf("names, no usage under a name:\n got %q\nwant %q", got, want)
	}
	if got := run(names, withName); got != want+"Usage: set [-abc]\n" {
		t.Errorf("names, usage under a name:\n got %q\nwant %q", got, want+"Usage: set [-abc]\n")
	}
	// A letter among them is enough to owe one, even where a name is not.
	mixed := "set -o nosuch -z\n"
	wantMixed := "sh: set: nosuch: bad option(s)\nsh: set: z: unknown option\nUsage: set [-abc]\n"
	if got := run(mixed, base); got != wantMixed {
		t.Errorf("a letter behind a name:\n got %q\nwant %q", got, wantMixed)
	}
}

// The axis is the *builtin's* and not the front end's.
//
// A shell's own option parse is a different surface with its own measured
// quirks — the shell that reports every bad word here answers `ksh -q -z`
// with a spurious `- : unknown option` between the two, and folds the whole
// of the rest of argv into a `-o` complaint — so it keeps stopping at the
// first word until those are settled. `reportsEveryBadSetOption` excludes
// the invocation and environment routes for that reason, and dropping either
// exclusion survived every test in the tree until this one.
func TestReportingEveryBadOptionDoesNotReachTheInvocationRoute(t *testing.T) {
	sem := PosixSemantics()
	sem.SetReportsEveryBadOption = Yes
	sem.BadSetOptionNameFatal = No
	sem.FatalErrorStatusIsOne = No
	dg := Diagnostics{
		Location:               LocationNone,
		SetInvalidOptionLetter: "set: %[2]s: unknown option",
		SetInvalidOptionStatus: 2,
		BuiltinUsage:           map[string]string{"set": "Usage: set [-abc]"},
		BuiltinUsageUnprefixed: true,
	}
	var buf strings.Builder
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "sh",
	})
	// The front end's way in, which is what sets atInvocation.
	r.SetOptionLetters("qz", true)
	got := buf.String()
	// One letter reported, not both: the invocation parse stops at the first
	// the way every dialect's builtin used to.
	if n := strings.Count(got, "unknown option"); n != 1 {
		t.Errorf("reported %d bad letters at an invocation, want 1: %q", n, got)
	}
	if strings.Contains(got, "z: unknown option") {
		t.Errorf("output %q, want only the first letter reported", got)
	}
}
