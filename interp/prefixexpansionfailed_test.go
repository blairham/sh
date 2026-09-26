// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// prefixFailRun runs src with the two axes that decide how far a failed
// assignment prefix unwinds set to the values the caller names.
//
// `bad math` is the arithmetic complaint, so a test can tell the failure's own
// sentence from anything the command would have printed.
func prefixFailRun(t *testing.T, src string, fatality PrefixRefusalFatalityPolicy, abandons Answer) (string, int) {
	t.Helper()
	sem := permissive()
	sem.PrefixRefusalFatality = fatality
	sem.FailedExpansionAbandonsTheLine = abandons
	sem.FatalErrorStatusIsOne = Yes
	dg := Diagnostics{ArithOperandExpected: "bad math"}
	var out bytes.Buffer
	dir := t.TempDir()
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "sh", Vars: map[string]string{"PATH": dir},
	})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), st
}

// A command whose assignment prefix could not be expanded does not run.
//
// It ran. `a=$(( } )) echo RAN` wrote the arithmetic complaint, then `RAN`,
// and reported 0 — so a script went on with a command that had been told its
// environment could not be computed, and `||` never fired. The same expression
// in an ordinary assignment and in an ordinary word was right all along, which
// is what says the bug was the prefix path: the check a command's words go
// through runs before the prefix is walked, so nothing had failed yet when it
// looked (#4675).
//
// **The noun is the expansion and not the assignment**, and TestAFrozenPrefix…
// below is the pair that holds one fixed and moves the other.
//
// Asserted across every value of both axes, because none of them is a reason
// to run the command: what they decide is how far the give-up reaches.
func TestACommandWhosePrefixCouldNotBeExpandedDoesNotRun(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`a=$(( } )) echo RAN`,
		`a=$(( } )) :`,
		`f() { echo RAN; }; a=$(( } )) f`,
		`a=$(( } )) /bin/echo RAN`,
		`a=$(( } )) command /bin/echo RAN`,
	} {
		for _, fatality := range []PrefixRefusalFatalityPolicy{
			PrefixRefusalNeverFatal,
			PrefixRefusalAlwaysFatal,
			PrefixRefusalFatalOnASpecialBuiltinOrFunction,
			PrefixRefusalFatalOnACommandThisShellRuns,
		} {
			for _, abandons := range []Answer{Yes, No} {
				out, _ := prefixFailRun(t, src+"\n", fatality, abandons)
				if !strings.Contains(out, "bad math") {
					t.Errorf("%s [%v/%v]: said %q, want the failure reported",
						src, fatality, abandons, out)
				}
				if strings.Contains(out, "RAN") {
					t.Errorf("%s [%v/%v]: said %q, want the command left unrun",
						src, fatality, abandons, out)
				}
			}
		}
	}
}

// And a prefix that expands cleanly still runs its command, with the value in
// front of it — the control that says the door above is opened by a failure
// and not by having a prefix at all.
func TestACleanPrefixStillRunsItsCommand(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, want string }{
		{`a=ok echo RAN`, "RAN"},
		{`f() { echo "[$a]"; }; a=ok f`, "[ok]"},
		{`a=ok /bin/echo RAN`, "RAN"},
		{`a=$(( 1 + 1 )) echo RAN`, "RAN"},
	} {
		out, st := prefixFailRun(t, c.src+"\n", PrefixRefusalAlwaysFatal, No)
		if !strings.Contains(out, c.want) || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
	}
}

// How far the give-up reaches is the three outcomes the panel has, and they
// are read off the two axes rather than written per command kind.
//
// The pairing is a snippet on **two lines**, because on one line ending the
// line and ending the shell print the same nothing — the shape #1171 was
// missed for.
func TestHowFarAFailedPrefixExpansionUnwinds(t *testing.T) {
	t.Parallel()
	const src = "a=$(( } )) echo RAN; echo SAME\necho \"after st=$?\"\n"
	for _, c := range []struct {
		name     string
		fatality PrefixRefusalFatalityPolicy
		abandons Answer
		same     bool
		after    string
		status   int
	}{
		{
			// The whole line goes, and the next one runs and reads 1.
			"the line is given up and the next one runs",
			PrefixRefusalNeverFatal, Yes, false, "after st=1", 0,
		},
		{
			// The shell ends, so neither the rest of the line nor the next
			// one is reached.
			"or the shell ends",
			PrefixRefusalAlwaysFatal, No, false, "", 1,
		},
		{
			// Or the command alone is given up: the rest of the line runs,
			// and `$?` on the next line is that command's own success.
			"or the command alone goes and the rest of the line runs",
			PrefixRefusalNeverFatal, No, true, "after st=0", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := prefixFailRun(t, src, c.fatality, c.abandons)
			if strings.Contains(out, "RAN") {
				t.Errorf("said %q, want the command left unrun", out)
			}
			if got := strings.Contains(out, "SAME"); got != c.same {
				t.Errorf("said %q, want the rest of the line run=%v", out, c.same)
			}
			if c.after == "" {
				if strings.Contains(out, "after") {
					t.Errorf("said %q, want nothing after it", out)
				}
			} else if !strings.Contains(out, c.after) {
				t.Errorf("said %q, want %q in it", out, c.after)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
		})
	}
}

// Which of the three a command gets is read from the command word, which is
// the whole reason the fatality axis is asked here: the two policies that
// name a kind answer differently for a builtin, a function and an external.
func TestTheGiveUpIsReadFromTheCommandWord(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		src   string
		ends  bool
		kinds PrefixRefusalFatalityPolicy
	}{
		{`a=$(( } )) echo RAN`, false, PrefixRefusalFatalOnASpecialBuiltinOrFunction},
		{`a=$(( } )) :`, true, PrefixRefusalFatalOnASpecialBuiltinOrFunction},
		{`f() { :; }; a=$(( } )) f`, true, PrefixRefusalFatalOnASpecialBuiltinOrFunction},
		{`a=$(( } )) /bin/echo RAN`, false, PrefixRefusalFatalOnASpecialBuiltinOrFunction},

		{`a=$(( } )) echo RAN`, true, PrefixRefusalFatalOnACommandThisShellRuns},
		{`a=$(( } )) :`, true, PrefixRefusalFatalOnACommandThisShellRuns},
		{`f() { :; }; a=$(( } )) f`, true, PrefixRefusalFatalOnACommandThisShellRuns},
		{`a=$(( } )) /bin/echo RAN`, false, PrefixRefusalFatalOnACommandThisShellRuns},
	} {
		out, st := prefixFailRun(t, c.src+"\necho AFTER\n", c.kinds, No)
		if ended := !strings.Contains(out, "AFTER"); ended != c.ends {
			t.Errorf("%s [%v] = %q, want the shell ending=%v", c.src, c.kinds, out, c.ends)
		}
		if c.ends && st != 1 {
			t.Errorf("%s [%v]: status = %d, want 1", c.src, c.kinds, st)
		}
	}
}

// A give-up that is the command's alone is **catchable**, which is what says
// the unwinding was taken back rather than merely renumbered. The fatal
// readings are not, and the pair is one snippet asked twice.
func TestOnlyACommandsOwnGiveUpIsCatchable(t *testing.T) {
	t.Parallel()
	const src = "a=$(( } )) /bin/echo RAN || echo CAUGHT\necho AFTER\n"
	if out, _ := prefixFailRun(t, src, PrefixRefusalFatalOnACommandThisShellRuns, No); !strings.Contains(out, "CAUGHT") {
		t.Errorf("said %q, want the give-up caught by ||", out)
	}
	if out, _ := prefixFailRun(t, src, PrefixRefusalAlwaysFatal, No); strings.Contains(out, "CAUGHT") {
		t.Errorf("said %q, want a fatal give-up to escape ||", out)
	}
}

// Nothing behind the entry that failed is expanded, which is unanimous in the
// panel and is asserted with the side effect written where it can be **seen**:
// `b=$(echo SIDE)` proves nothing, because a substitution's output is captured
// whether or not the entry ran.
func TestTheEntriesBehindAFailedPrefixAreNotExpanded(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`a=$(( } )) b=$(echo SIDE >&2) echo RAN`,
		`a=$(( } )) b=$(echo SIDE >&2) /bin/echo RAN`,
		`f() { :; }; a=$(( } )) b=$(echo SIDE >&2) f`,
	} {
		out, _ := prefixFailRun(t, src+"\n", PrefixRefusalAlwaysFatal, No)
		if strings.Contains(out, "SIDE") {
			t.Errorf("%s: said %q, want the entry behind the failure left unexpanded", src, out)
		}
	}
	// The control the row above needs: with nothing in front of it the same
	// entry does run, so the absence above is the give-up and not the probe.
	out, _ := prefixFailRun(t, "b=$(echo SIDE >&2) echo RAN\n", PrefixRefusalAlwaysFatal, No)
	if !strings.Contains(out, "SIDE") || !strings.Contains(out, "RAN") {
		t.Errorf("said %q, want the entry expanded and the command run", out)
	}
}

// The pair that holds the noun fixed. A prefix to a **frozen name** is an
// assignment that failed where the rows above are an *expansion* that failed,
// and the panel answers them differently: where a refusal costs nothing the
// command still runs, and a failed expansion costs the command there anyway.
// A rule keyed on "the prefix did not take" answers this row wrong.
func TestAFrozenPrefixIsADifferentFailureFromOneThatWouldNotExpand(t *testing.T) {
	t.Parallel()
	sem := permissive()
	sem.PrefixRefusalFatality = PrefixRefusalNeverFatal
	sem.PrefixRefusalCostsTheCommand = No
	sem.FailedExpansionAbandonsTheLine = Yes
	sem.FatalErrorStatusIsOne = Yes
	run := func(src string) string {
		t.Helper()
		var out bytes.Buffer
		dir := t.TempDir()
		r := newTestRunner(t, &Runner{
			Stdout: &out, Stderr: &out, Semantics: &sem,
			Diagnostics: &Diagnostics{
				ArithOperandExpected: "bad math",
				ReadonlyVariable:     "%s: readonly variable",
			},
			Dir: dir, Name: "sh", Vars: map[string]string{"PATH": dir},
		})
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
		return out.String()
	}
	if got := run("readonly r=1\nr=2 echo RAN\n"); !strings.Contains(got, "RAN") {
		t.Errorf("said %q, want a refusal that costs nothing to still run the command", got)
	}
	if got := run("r=$(( } )) echo RAN\n"); strings.Contains(got, "RAN") {
		t.Errorf("said %q, want a failed expansion to cost the command anyway", got)
	}
}
