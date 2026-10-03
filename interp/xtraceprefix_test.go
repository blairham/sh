// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"io"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Where `set -x` writes an assignment that stands in front of a command —
// Diagnostics.TracePrefixAssignment, and #3133, where it was written nowhere.
//
// Named for the field and not for the shells: which value each preset picks is
// asserted in dialect/xtraceprefix_test.go against the panel's own bytes. What
// is asserted here is that the four shapes are four shapes, and that each of
// them can be reached.
//
// The probe is one script for every case, so a value that produced the right
// line for a function and the wrong one for a builtin cannot pass: `A=3 f zz`
// is a function, `B=4 true` a regular builtin, and `C=5 :` a special one, and
// the last two are exactly where the one column that writes the prefix *after*
// the command stops doing so.
const prefixTraceProbe = "f() { :; }\nset -x\nA=3 f zz\nB=4 true\nC=5 :\n"

func prefixTrace(t *testing.T, d Diagnostics) string {
	t.Helper()
	return traceOf(t, prefixTraceProbe, permissive(), d)
}

func wantTrace(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("trace was\n%s\nwant\n%s", got, want)
	}
}

// TestThePrefixIsWrittenAheadOfTheCommand is bash's shape and the zero value:
// a line per assignment, in front of the command's own.
func TestThePrefixIsWrittenAheadOfTheCommand(t *testing.T) {
	wantTrace(t, prefixTrace(t, Diagnostics{}),
		"+ A=3\n+ f zz\n+ :\n+ B=4\n+ true\n+ C=5\n+ :\n")
}

// TestThePrefixIsWrittenBehindTheCommand is ksh93's, including the half that
// is not "behind": a special builtin and a function keep the assignment in
// front, because there it is an ordinary assignment that stays assigned.
func TestThePrefixIsWrittenBehindTheCommand(t *testing.T) {
	wantTrace(t, prefixTrace(t, Diagnostics{TracePrefixAssignment: TracePrefixOwnLineAfter}),
		"+ A=3\n+ f zz\n+ :\n+ true\n+ B=4\n+ C=5\n+ :\n")
}

// TestThePrefixIsWrittenOnTheCommandsLine is dash's and BusyBox ash's: one
// line, the assignments where the script wrote them.
func TestThePrefixIsWrittenOnTheCommandsLine(t *testing.T) {
	wantTrace(t, prefixTrace(t, Diagnostics{TracePrefixAssignment: TracePrefixOnTheCommandLine}),
		"+ A=3 f zz\n+ :\n+ B=4 true\n+ C=5 :\n")
}

// TestThePrefixRepeatsTheTracePrefix is zsh's: the same one line, with the
// trace prefix written a second time between the assignments and the words.
//
// Every command in the probe is one this shell runs itself, so every line
// repeats; TestTheRepeatStopsAtACommandThisShellHandsOver is the other half.
func TestThePrefixRepeatsTheTracePrefix(t *testing.T) {
	got := prefixTrace(t, Diagnostics{
		TracePrefixAssignment: TracePrefixOnTheCommandLineRepeatingThePrefix,
	})
	wantTrace(t, got, "+ A=3 + f zz\n+ :\n+ B=4 + true\n+ C=5 + :\n")
}

// TestTheRepeatStopsAtACommandThisShellHandsOver is the row that says the
// repeat is a question about the command and not about the line: a name
// nothing will run reaches a process — or would have — and gets one prefix.
func TestTheRepeatStopsAtACommandThisShellHandsOver(t *testing.T) {
	got := traceOf(t, "set -x\nA=3 nosuchcommand_zz\n", permissive(), Diagnostics{
		TracePrefixAssignment: TracePrefixOnTheCommandLineRepeatingThePrefix,
	})
	if !strings.HasPrefix(got, "+ A=3 nosuchcommand_zz\n") {
		t.Errorf("trace was %q, want one prefix and no repeat", got)
	}
}

// TestAPrefixValueIsExpandedOnceUnderATrace is the trap this change had to
// step over: the line is written before the command runs and the command
// needs the same value, so an implementation that expanded twice would run a
// substitution in a prefix twice — silently, and only with `set -x` on.
func TestAPrefixValueIsExpandedOnceUnderATrace(t *testing.T) {
	for _, style := range []TracePrefixAssignment{
		TracePrefixOwnLineBefore,
		TracePrefixOwnLineAfter,
		TracePrefixOnTheCommandLine,
		TracePrefixOnTheCommandLineRepeatingThePrefix,
	} {
		// A file rather than a variable, because the substitution runs in a
		// subshell of its own and a counter it incremented would be back at
		// nought by the time anything could read it — which is a way of
		// passing this test without running the substitution at all.
		dir := t.TempDir()
		src := "set -x\nV=$(printf a >> " + dir + "/n; printf v) true\nset +x\n" +
			"printf '[%s]' \"$(cat " + dir + "/n)\"\n"
		var out strings.Builder
		sem := permissive()
		diag := Diagnostics{TracePrefixAssignment: style}
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		r := newTestRunner(t, &Runner{
			Stdout: &out, Stderr: io.Discard, Semantics: &sem, Diagnostics: &diag,
			Name: "sh", Env: testPATH(),
		})
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(out.String()); got != "[a]" {
			t.Errorf("style %d ran the substitution %s, want [a] — once", style, got)
		}
	}
}

// TestAFrozenNameInAPrefixWritesNoLine is what every column does with a
// prefix that was refused: the assignment that did not happen is not written
// as though it had. Where the prefix has a line of its own the command's line
// stands; where the prefix and the command share one line, that line is
// written once the prefix is in place and so is never written at all (#5509).
func TestAFrozenNameInAPrefixWritesNoLine(t *testing.T) {
	for _, c := range []struct {
		style       TracePrefixAssignment
		commandLine bool
	}{
		{TracePrefixOwnLineBefore, true},
		{TracePrefixOnTheCommandLine, false},
	} {
		got := traceOf(t, "readonly x=1\nset -x\nx=2 true\n", permissive(),
			Diagnostics{TracePrefixAssignment: c.style})
		if strings.Contains(got, "x=2") {
			t.Errorf("style %d wrote %q, want no line for the refused prefix", c.style, got)
		}
		if strings.Contains(got, "+ true") != c.commandLine {
			t.Errorf("style %d wrote %q, want the command's own line: %v", c.style, got, c.commandLine)
		}
	}
}

// TestAnUntracedPrefixIsUnchanged is the guard on the expansion this moved:
// the values are computed early *only* when the trace needs them, so a
// prefixed command with no `set -x` on takes the route it always did.
func TestAnUntracedPrefixIsUnchanged(t *testing.T) {
	got := traceOf(t, "f() { :; }\nA=3 f zz\nB=4 true\n", permissive(), Diagnostics{})
	if got != "" {
		t.Errorf("an untraced run wrote %q, want nothing", got)
	}
}

// TestAnAppendingPrefixIsRenderedByItsColumn is the one cell the prefix
// position spells differently from the statement: bash writes the value the
// append came to and drops the operator, where ksh93 and zsh keep both.
//
// The bare `w+=2` beside it is the control. A shell that simply spelled every
// append as a join would pass the first row and fail the second.
func TestAnAppendingPrefixIsRenderedByItsColumn(t *testing.T) {
	src := "v=14\nset -x\nv+=5 true\nw=1\nw+=2\nset +x\n"
	kept := traceOf(t, src, permissive(), Diagnostics{})
	wantTrace(t, kept, "+ v+=5\n+ true\n+ w=1\n+ w+=2\n+ set +x\n")
	joined := traceOf(t, src, permissive(), Diagnostics{TracePrefixAppendIsTheJoinedValue: true})
	wantTrace(t, joined, "+ v=145\n+ true\n+ w=1\n+ w+=2\n+ set +x\n")
}

// TestAnAppendingPrefixJoinsOnce holds the rendering still and moves the axis
// that decides *when* a command's prefix is worked through.
//
// The two are connected by one loop. Where the prefix is worked through ahead
// of the redirections, each entry's trace line is written inside the walk that
// also holds the entry's value under its name for the entries behind it — so
// by the time the line is rendered the name already carries the join, and
// deriving it a second time appends the word twice: `v=14; v+=5 true` traced
// `+ v=1455` where the column that writes an append as its joined value writes
// `+ v=145` (#4431).
//
// The unset name beside it separates the two arithmetics rather than repeating
// the first row: joining nothing to `2` is `2` either way round, so a single
// join writes `+ w=2` and a second one writes `+ w=22`. A special builtin
// carries it, because that is the command the middle answer takes the walk
// for while it opens a regular builtin's redirections first.
func TestAnAppendingPrefixJoinsOnce(t *testing.T) {
	for _, order := range []PrefixRedirectionOrder{
		PrefixExpandedBeforeRedirectionsAlways,
		PrefixExpandedBeforeRedirectionsWhereItPersists,
		PrefixExpandedBeforeRedirectionsNever,
	} {
		t.Run(order.String(), func(t *testing.T) {
			sem := permissive()
			sem.PrefixExpandedBeforeTheRedirections = order
			got := traceOf(t, "v=14\nset -x\nv+=5 true\nw+=2 :\nset +x\n", sem,
				Diagnostics{TracePrefixAppendIsTheJoinedValue: true})
			wantTrace(t, got, "+ v=145\n+ true\n+ w=2\n+ :\n+ set +x\n")
		})
	}
}
