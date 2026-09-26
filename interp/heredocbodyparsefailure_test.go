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

// A here-document body holding a substitution that will not **parse**, on a
// command this shell runs itself. See interp/heredocbodyparsefailure.go for
// the panel these rows are graded against.

// parseFailSemantics is the vector a row runs under: the axis that says whose
// failure it is, and the three that grade its two readings.
type parseFailSemantics struct {
	// isRedirs answers whose failure a body that will not parse is.
	isRedirs Answer
	// endsTheShell is asked first and outranks both readings: where it is
	// Yes the refusal's stop stands and there is nothing left to decide.
	endsTheShell Answer
	// specialFatal grades the first reading — a failed redirection on a
	// special builtin ending the shell.
	specialFatal Answer
	// abandons grades the second — a failed expansion giving up the line
	// rather than the shell.
	abandons Answer
}

// parseFailRun runs one snippet. The numbers are deliberately all different:
// 7 for a failed redirection, 5 for the syntax refusal and 1 for a fatal
// error, so a row can say *which* of the three a status came from rather than
// merely that it is not zero.
func parseFailRun(t *testing.T, src string, c parseFailSemantics, tweak ...func(*Semantics)) (string, int) {
	t.Helper()
	sem := permissive()
	sem.HeredocBodyFailureIsTheRedirections = c.isRedirs
	sem.SubstitutionParseFailureInAHeredocBodyEndsTheShell = c.endsTheShell
	sem.SubstitutionParseFailureCarriesTheFatalStatus = No
	sem.RedirectErrorOnSpecialBuiltinFatal = c.specialFatal
	sem.FailedExpansionAbandonsTheLine = c.abandons
	sem.FatalErrorStatusIsOne = Yes
	for _, f := range tweak {
		f(&sem)
	}
	dg := Diagnostics{RedirectFailureStatus: 7, SyntaxErrorStatus: 5}
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

// parseFailAnswers is every combination of the four axes, for the assertions
// that must hold whatever any of them says.
func parseFailAnswers() []parseFailSemantics {
	var all []parseFailSemantics
	for _, isRedirs := range []Answer{Yes, No} {
		for _, ends := range []Answer{Yes, No} {
			for _, special := range []Answer{Yes, No} {
				for _, abandons := range []Answer{Yes, No} {
					all = append(all, parseFailSemantics{isRedirs, ends, special, abandons})
				}
			}
		}
	}
	return all
}

// The commands a here-document can be written on that this shell runs itself.
// The external is the control and is deliberately not here — see
// TestTheParseAnswerDoesNotReachACommandOfItsOwn.
var parseFailCommands = []struct{ name, src string }{
	{"a special builtin", ": <<END"},
	{"a regular builtin", "echo RAN <<END"},
	{"a function", "f() { echo RAN; }\nf <<END"},
	{"a group", "{ echo RAN; } <<END"},
}

// A command whose here-document body will not parse does not run, and the
// refusal is reported.
//
// Asserted under every combination of the four axes, because none of them is
// a reason to run the command: what they decide is how far the give-up
// reaches and what number it leaves.
func TestACommandWhoseHeredocBodyWillNotParseDoesNotRun(t *testing.T) {
	t.Parallel()
	for _, cmd := range parseFailCommands {
		t.Run(cmd.name, func(t *testing.T) {
			for _, c := range parseFailAnswers() {
				out, _ := parseFailRun(t, cmd.src+"\n$(echo hi; for)\nEND\n", c)
				if strings.Contains(out, "RAN") {
					t.Errorf("%+v: said %q, want the command left unrun", c, out)
				}
				if !strings.Contains(out, "syntax") && !strings.Contains(out, "unexpected") {
					t.Errorf("%+v: said %q, want the refusal reported", c, out)
				}
			}
		})
	}
}

// Whose failure it is, which is what the axis decides and the whole of it.
//
// The pairing is a snippet on **two commands on one line**, because with the
// redirection alone on its line giving up the command, giving up the line and
// ending the shell all print the same nothing — which is how an earlier
// version of this question was asked and why it could not see its own answer.
func TestWhoseFailureAHeredocBodyThatWillNotParseIs(t *testing.T) {
	t.Parallel()
	const src = ": <<END; echo SAME\n$(echo hi; for)\nEND\necho \"after st=$?\"\n"
	for _, c := range []struct {
		name   string
		sem    parseFailSemantics
		tweak  func(*Semantics)
		same   bool
		after  string
		status int
	}{
		{
			// The refusal ends the shell outright, which outranks both
			// readings and is three of the five columns.
			"the shell ends over the refusal",
			parseFailSemantics{No, Yes, No, Yes},
			nil,
			false, "", 5,
		},
		{
			// The redirection's, on a special builtin, in a shell that
			// keeps the POSIX rule: the shell stops — and with the
			// refusal's own number, not the redirection's 7.
			"the redirection's, and a special builtin's is fatal",
			parseFailSemantics{Yes, No, Yes, Yes},
			nil,
			false, "", 5,
		},
		{
			// The redirection's, where it is not fatal: the command alone
			// goes and the rest of the line runs.
			"the redirection's, and not fatal",
			parseFailSemantics{Yes, No, No, Yes},
			nil,
			true, "after st=0", 0,
		},
		{
			// This shell's own failed expansion, in the reading that gives
			// up the line: the rest of the line goes with it and the next
			// line reads a failing status.
			//
			// The one column with this reading answers Yes to
			// SubstitutionParseFailureCarriesTheFatalStatus, so the
			// refusal's number and the give-up's are the same 1 there. The
			// tweak says so rather than letting the row pick one of the
			// two: nothing in the panel separates them here, and a row
			// asserting 5 or 1 would be asserting an invention.
			"this shell's own, giving up the line",
			parseFailSemantics{No, No, No, Yes},
			func(s *Semantics) { s.SubstitutionParseFailureCarriesTheFatalStatus = Yes },
			false, "after st=1", 0,
		},
		{
			// And in the reading that is fatal, whatever the command word
			// was and whatever a failed redirection on it would have cost.
			"this shell's own, and fatal",
			parseFailSemantics{No, No, No, No},
			func(s *Semantics) { s.SubstitutionParseFailureCarriesTheFatalStatus = Yes },
			false, "", 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var tweaks []func(*Semantics)
			if c.tweak != nil {
				tweaks = append(tweaks, c.tweak)
			}
			out, st := parseFailRun(t, src, c.sem, tweaks...)
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

// The command word decides only under the first reading, which is the pair
// that says the axis is read before the command kind is.
//
// A special builtin and a regular one are graded apart where the failure is
// the redirection's, and identically where it is this shell's own.
func TestTheCommandWordDecidesOnlyWhereAParseFailureIsTheRedirections(t *testing.T) {
	t.Parallel()
	ends := func(c parseFailSemantics, src string) bool {
		t.Helper()
		out, _ := parseFailRun(t, src+"\n$(echo hi; for)\nEND\necho AFTER\n", c)
		return !strings.Contains(out, "AFTER")
	}
	redirs := parseFailSemantics{Yes, No, Yes, Yes}
	if !ends(redirs, ": <<END") {
		t.Error("the redirection's, special builtin: want the shell ended")
	}
	if ends(redirs, "echo RAN <<END") {
		t.Error("the redirection's, regular builtin: want the shell carrying on")
	}
	// The same two words under the other reading, with the very axis that
	// split them still answering Yes: both stop, because a failed expansion
	// is fatal here whatever it was written on, and the special builtin's
	// rule is never what decided it.
	own := parseFailSemantics{No, No, Yes, No}
	if !ends(own, ": <<END") {
		t.Error("this shell's own, special builtin: want the shell ended")
	}
	if !ends(own, "echo RAN <<END") {
		t.Error("this shell's own, regular builtin: want the shell ended too")
	}
}

// The number a refusal leaves is the **refusal's own**, and it survives a
// rule that puts the stop back.
//
// This is the whole reason a body that will not parse does not go through
// interp/heredocbodyfailure.go's door: that door hands the give-up the
// *redirection's* status, and one column exits with the refusal's 3 where its
// own failed open exits 1. Asserted with three distinct numbers — 7 for a
// failed redirection, 5 for the refusal, 1 for a fatal error — so a row says
// which of the three it took.
func TestAHeredocBodyThatWillNotParseKeepsItsOwnStatus(t *testing.T) {
	t.Parallel()
	const src = ": <<END\n$(echo hi; for)\nEND\n"
	// A special builtin, the redirection's failure, and the POSIX rule kept:
	// the shell stops, and with 5 rather than with 7 or 1.
	if out, st := parseFailRun(t, src, parseFailSemantics{Yes, No, Yes, Yes}); st != 5 {
		t.Errorf("status = %d, said %q, want the refusal's own 5 and not the redirection's 7", st, out)
	}
	// Carried on rather than stopped, where the next line reads it.
	const carried = "echo RAN <<END\n$(echo hi; for)\nEND\necho \"after st=$?\"\n"
	if out, _ := parseFailRun(t, carried, parseFailSemantics{Yes, No, Yes, Yes}); !strings.Contains(out, "after st=5") {
		t.Errorf("said %q, want the refusal's own status left behind", out)
	}
	// The control that says 7 is reachable at all, so the rows above are a
	// number being *kept* rather than a field nothing ever reads: the same
	// command with a body that fails to **expand** takes the redirection's
	// status through the same boundary.
	const expands = "echo RAN <<END\n$(( } ))\nEND\necho \"after st=$?\"\n"
	if out, _ := parseFailRun(t, expands, parseFailSemantics{Yes, No, Yes, Yes}); !strings.Contains(out, "after st=7") {
		t.Errorf("said %q, want a failed *expansion* to take the redirection's 7", out)
	}
}

// The number a refusal leaves is **that command's** and does not outlive it.
//
// A refusal on one command and an ordinary failed open on the next: the
// second must exit with the redirection's 7 and not with the 5 the first one
// left, which is what says the number is cleared with the rest of a
// command's redirection state. Measured 2026-09-26 on ksh93u+ — `echo RAN
// <<END` with `$(echo hi; for)` leaves 3 and carries on, and `: <
// /nonexistent/f` on the next line then ends the shell at **1**.
func TestARefusalsStatusDoesNotOutliveItsCommand(t *testing.T) {
	t.Parallel()
	const src = "echo RAN <<END\n$(echo hi; for)\nEND\necho \"mid st=$?\"\n: < /nonexistent/f\necho NEVER\n"
	out, st := parseFailRun(t, src, parseFailSemantics{Yes, No, Yes, Yes})
	// The control: the first command really did refuse and really did leave
	// its own number, so the row below is a number being cleared rather than
	// one that was never written.
	if !strings.Contains(out, "mid st=5") {
		t.Errorf("= %q, want the refusal's own 5 left on the first command", out)
	}
	if strings.Contains(out, "NEVER") {
		t.Errorf("= %q, want the second command's failed open to end the shell", out)
	}
	if st != 7 {
		t.Errorf("status = %d, want the redirection's 7 and not the refusal's 5", st)
	}
}

// Flipping the axis must not move a command the shell runs as a process of
// its own: the external row is the one the five columns settled one construct
// over, and it is not this question's.
func TestTheParseAnswerDoesNotReachACommandOfItsOwn(t *testing.T) {
	t.Parallel()
	const src = "/bin/echo RAN <<END\n$(echo hi; for)\nEND\necho \"after st=$?\"\n"
	redirs, redirsSt := parseFailRun(t, src, parseFailSemantics{Yes, No, Yes, Yes})
	own, ownSt := parseFailRun(t, src, parseFailSemantics{No, No, Yes, Yes})
	if redirs != own || redirsSt != ownSt {
		t.Errorf("%q at %d under one answer and %q at %d under the other: the axis must "+
			"not reach a command the shell runs as a process of its own", redirs, redirsSt, own, ownSt)
	}
	// The control the comparison needs: the refusal really happened, the
	// command really did not run, and the script really carried on — two
	// runs that both ended the shell would agree just as readily.
	if strings.Contains(redirs, "RAN") || !strings.Contains(redirs, "after st=5") {
		t.Errorf("= %q, want the command unrun and the refusal's status left behind", redirs)
	}
}

// A **quoted** delimiter leaves the body literal, so there is no substitution
// to refuse and the command runs at 0 whatever any axis says.
//
// The control that says the door above is opened by a refusal and not by
// having a here-document with those bytes in it.
func TestAQuotedDelimiterLeavesABodyThatWillNotParseAlone(t *testing.T) {
	t.Parallel()
	for _, cmd := range parseFailCommands {
		src := strings.Replace(cmd.src, "<<END", "<<'END'", 1) +
			"\n$(echo hi; for)\nEND\necho \"after st=$?\"\n"
		for _, c := range parseFailAnswers() {
			out, st := parseFailRun(t, src, c)
			if strings.Contains(out, "syntax") || strings.Contains(out, "unexpected") {
				t.Errorf("%s [%+v]: said %q, want nothing refused", cmd.name, c, out)
			}
			if !strings.Contains(out, "after st=0") || st != 0 {
				t.Errorf("%s [%+v]: said %q (status %d), want the command run at 0", cmd.name, c, out, st)
			}
		}
	}
}

// An indented `<<-` body is a `<<` body with its indentation stripped and it
// is refused the same way — asserted because the stripping happens in a
// different place from the reading.
func TestAnIndentedHeredocBodyThatWillNotParseFailsTheSameWay(t *testing.T) {
	t.Parallel()
	for _, c := range parseFailAnswers() {
		plain, plainSt := parseFailRun(t, ": <<END; echo SAME\n$(echo hi; for)\nEND\necho \"after st=$?\"\n", c)
		dashed, dashedSt := parseFailRun(t, ": <<-END; echo SAME\n\t$(echo hi; for)\n\tEND\necho \"after st=$?\"\n", c)
		if plain != dashed || plainSt != dashedSt {
			t.Errorf("[%+v] %q at %d for `<<` and %q at %d for `<<-`", c, plain, plainSt, dashed, dashedSt)
		}
	}
}

// And where no dialect answered whose failure it is, the shell says so and
// runs nothing: acting on either reading after reporting that the shells
// disagree would answer the question anyway.
func TestAnUnansweredParseFailureReadingRunsNothing(t *testing.T) {
	t.Parallel()
	sem := permissive()
	sem.SubstitutionParseFailureInAHeredocBodyEndsTheShell = No
	sem.RedirectErrorOnSpecialBuiltinFatal = No
	sem.FatalErrorStatusIsOne = Yes
	var out bytes.Buffer
	dir := t.TempDir()
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &out, Semantics: &sem,
		Diagnostics: &Diagnostics{SyntaxErrorStatus: 5},
		Dir:         dir, Name: "sh", Vars: map[string]string{"PATH": dir},
	})
	f, err := syntax.Parse("echo RAN <<END\n$(echo hi; for)\nEND\necho \"after st=$?\"\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Contains(got, "RAN") {
		t.Errorf("= %q, want the command left unrun", got)
	}
	if !strings.Contains(got, "no dialect was chosen") {
		t.Errorf("= %q, want the unanswered axis reported", got)
	}
}
