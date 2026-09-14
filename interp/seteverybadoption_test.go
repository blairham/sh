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
	// Both spellings, and the same answer to each: these rows mix names and
	// letters in one `set`, and what they measure is how many words get
	// reported rather than which spelling was harsher.
	sem.BadSetOptionNameFatal = fatal
	sem.BadSetOptionLetterFatal = fatal
	sem.FatalErrorStatusIsOne = No
	// The control column is the one that applies as it goes, so that
	// answering this axis No is the whole of the difference between the two
	// runs of every row below. Left Unspecified it would be a third reading
	// — bash's — and a row whose bad word has something in front of it would
	// stop on the unanswered axis rather than on the option. Every row here
	// happens to put its bad letter first, where the question is not asked at
	// all; saying it out loud is what keeps a later row from inheriting an
	// answer nobody chose.
	sem.SetValidatesOptionLettersFirst = No
	// One row below is written `-oNAME`, and a shell that has not answered
	// this would stop on the unanswered axis rather than on the option word.
	// No other row puts anything behind the `o`, so the answer reaches only
	// the row that asked for it.
	sem.SetOLetterAttachesItsName = Yes
	dg := Diagnostics{
		// The shell's name still stands in front of each sentence, which is
		// why every expectation below carries it: the assertion is the whole
		// rendered block and not a substring of it.
		Location:                     LocationNone,
		SetInvalidOptionLetter:       "set: %[2]s: unknown option",
		SetInvalidOptionName:         "set: %[1]s: bad option(s)",
		SetInvalidOptionNameStatus:   2,
		SetInvalidOptionLetterStatus: 2,
		BuiltinUsage:                 map[string]string{"set": "Usage: set [-abc]"},
		BuiltinUsageUnprefixed:       true,
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
	sem.BadSetOptionLetterFatal = No
	sem.FatalErrorStatusIsOne = No
	base := Diagnostics{
		Location:                     LocationNone,
		SetInvalidOptionLetter:       "set: %[2]s: unknown option",
		SetInvalidOptionName:         "set: %[1]s: bad option(s)",
		SetInvalidOptionNameStatus:   2,
		SetInvalidOptionLetterStatus: 2,
		BuiltinUsage:                 map[string]string{"set": "Usage: set [-abc]"},
		BuiltinUsageUnprefixed:       true,
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
	sem.BadSetOptionLetterFatal = No
	sem.FatalErrorStatusIsOne = No
	dg := Diagnostics{
		Location:                     LocationNone,
		SetInvalidOptionLetter:       "set: %[2]s: unknown option",
		SetInvalidOptionNameStatus:   2,
		SetInvalidOptionLetterStatus: 2,
		BuiltinUsage:                 map[string]string{"set": "Usage: set [-abc]"},
		BuiltinUsageUnprefixed:       true,
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

// The other half of the axis: the dialect that reports every bad option word
// applies **none** of them, names as well as letters.
//
// Measured 2026-09-13 across all seven columns with `command set` in front of
// the builtin, so that the columns where a refusal is fatal live long enough
// to be asked what they applied, and with `command set -e` alone as the
// control that the probe can see an option at all:
//
//	command set -e -Z             errexit off in ksh93 and the three bash
//	                              columns, on in dash and BusyBox ash
//	command set -eu -o zzznosuch  nounset and errexit **off** in ksh93,
//	                              **on** in every other column
//
// So the reach is what parts ksh93 from bash, and it is the reach the reports
// already imply: a shell that names every bad option word has by then read
// every option word. zsh takes neither probe — `command` does not reach its
// builtins — and was asked with a listing instead: `set -eu -o zzznosuch -o`
// there writes the option table with errexit and nounset both on.
//
// Two options are set with the bad word behind them, rather than one, because
// a row that checks only the option next to the refusal cannot tell "nothing
// was applied" from "the first one was". Nounset and noclobber rather than
// errexit, because errexit would end the script over the refusal's own status
// and take the line that reports the state with it.
func TestReportingEveryBadOptionAppliesNoneOfThem(t *testing.T) {
	for _, tc := range []struct {
		why, src     string
		every, first string
	}{
		{
			why:   "a bad letter behind a good one",
			src:   "set -u -Z\n",
			every: "-=\n",
			first: "-=u\n",
		},
		{
			// The row with no `preceded` gate on it. The two answers agree —
			// a shell that applies as it goes stops at the first bad word and
			// never reaches the `-u` either, which is dash's `set -Z -e`
			// leaving errexit off — but they did not agree before the fold:
			// the dialect that carries on *reading* past a bad word used to
			// carry on applying, so this was the one column where the option
			// behind the refusal came on.
			why:   "a good letter behind the bad one",
			src:   "set -Z -u\n",
			every: "-=\n",
			first: "-=\n",
		},
		{
			why:   "two good letters in one word, behind them a bad letter",
			src:   "set -uC -Z\n",
			every: "-=\n",
			first: "-=uC\n",
		},
		{
			// The half no letters-only pass can reach, and the reason this is
			// not a value of SetValidatesOptionLettersFirst: bash's first
			// pass knows the letter table and not the name table, so this row
			// is both letters there as well as in the shells that apply as
			// they go.
			why:   "two good letters in one word, behind them a bad `-o` name",
			src:   "set -uC -o zzznosuch\n",
			every: "-=\n",
			first: "-=uC\n",
		},
		{
			why:   "a bad `-o` name welded to its letter",
			src:   "set -uC -ozzznosuch\n",
			every: "-=\n",
			first: "-=uC\n",
		},
		{
			why:   "a good `-o` name in front of a bad letter",
			src:   "set -o nounset -Z\n",
			every: "-=\n",
			first: "-=u\n",
		},
		{
			// The control: no bad word, and the dialect that reads them all
			// applies them all. Without it every row above would pass in a
			// `set` that had quietly stopped working.
			why:   "nothing bad, so everything applies",
			src:   "set -uC\n",
			every: "-=uC\n",
			first: "-=uC\n",
		},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if got, _ := setAppliedRun(t, tc.src, Yes); !strings.HasSuffix(got, tc.every) {
				t.Errorf("reporting every bad word:\n got %q\nwant it to end %q", got, tc.every)
			}
			if got, _ := setAppliedRun(t, tc.src, No); !strings.HasSuffix(got, tc.first) {
				t.Errorf("applying as it goes:\n got %q\nwant it to end %q", got, tc.first)
			}
		})
	}
}

// setAppliedRun runs a `set` and then prints the option letters it left
// behind, with the refusal made survivable so that there is something to ask.
func setAppliedRun(t *testing.T, src string, every Answer) (string, int) {
	t.Helper()
	return setEveryRun(t, src+"echo \"-=$-\"\n", every, No)
}

// Nothing is listed either, which is the same fact from the other side: the
// applying loop is where a bare `-o` writes the option table, and a `set` that
// refused a word never reaches it. Measured — `set -o -Z` in ksh93, where a
// bare `-o` takes no next word, draws the refusal and its usage line and no
// table at all, and `set -e -Z -o` draws the same. Ours listed the whole table
// with `errexit on` in it, which is the partial application in plain sight.
func TestReportingEveryBadOptionListsNothingEither(t *testing.T) {
	got, _ := setEveryRun(t, "set -u -Z -o\n", Yes, No)
	if strings.Contains(got, "nounset") {
		t.Errorf("output %q, want no option table under a refused word", got)
	}
	// The control: the same words with nothing bad among them do list, so the
	// row above is a suppressed listing and not a listing that never worked.
	got, _ = setEveryRun(t, "set -u -o\n", Yes, No)
	if !strings.Contains(got, "nounset") {
		t.Errorf("output %q, want the option table", got)
	}
}

// The fold is a widening of one dialect's answer and not of the other's.
//
// bash validates the option **letters** first and its pass does not know the
// name table, so `command set -eu -o zzznosuch` is nounset and errexit on
// there where ksh93's is off — measured on the same day, in all three bash
// columns. A reading pass that had grown the names for everybody would put
// this row's answer where the measurement says it is not, which is exactly
// why the second half went to the axis only one dialect answers yes to rather
// than to SetValidatesOptionLettersFirst (#2670).
func TestValidatingTheLettersFirstStillDoesNotReachTheNames(t *testing.T) {
	sem := PosixSemantics()
	sem.SetReportsEveryBadOption = No
	sem.SetValidatesOptionLettersFirst = Yes
	sem.BadSetOptionNameFatal = No
	sem.BadSetOptionLetterFatal = No
	sem.FatalErrorStatusIsOne = No
	dg := Diagnostics{
		Location:                     LocationNone,
		SetInvalidOptionLetter:       "set: %[2]s: invalid option",
		SetInvalidOptionName:         "set: %[1]s: invalid option name",
		SetInvalidOptionNameStatus:   2,
		SetInvalidOptionLetterStatus: 2,
	}
	run := func(src string) string {
		t.Helper()
		var buf strings.Builder
		r := newTestRunner(t, &Runner{
			Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "sh",
		})
		f, err := syntax.Parse(src+"echo \"-=$-\"\n", syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	if got := run("set -uC -o zzznosuch\n"); !strings.HasSuffix(got, "-=uC\n") {
		t.Errorf("a bad name behind two good letters:\n got %q\nwant it to end %q", got, "-=uC\n")
	}
	// And the letter it does reach, so the row above is the reach and not a
	// pass that has stopped running.
	if got := run("set -uC -Z\n"); !strings.HasSuffix(got, "-=\n") {
		t.Errorf("a bad letter behind two good letters:\n got %q\nwant it to end %q", got, "-=\n")
	}
}
