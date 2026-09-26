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

// heredocFailSemantics is the vector these rows run under: the new axis at the
// caller's answer, and the two axes that grade each of its two readings.
//
// `bad math` is the arithmetic complaint, so a row can tell the failure's own
// sentence from anything the command would have printed.
type heredocFailSemantics struct {
	// isRedirs answers whose failure a body that will not expand is.
	isRedirs Answer
	// specialFatal is what grades the first answer — a failed redirection on
	// a special builtin ending the shell.
	specialFatal Answer
	// abandons is what grades the second — a failed expansion giving up the
	// line rather than the shell.
	abandons Answer
	// redirStatus is the number a failed redirection reports here. Zero
	// leaves the substrate's, which is the fatal one; the column that
	// separates the two is BusyBox ash.
	redirStatus int
}

// tweak is for the row that has to move an axis outside the three above —
// today only the status a fatal error carries, which is what separates
// "the redirection's number" from "the number the failure already had".
func heredocFailRun(t *testing.T, src string, c heredocFailSemantics, tweak ...func(*Semantics)) (string, int) {
	t.Helper()
	sem := permissive()
	sem.HeredocBodyFailureIsTheRedirections = c.isRedirs
	sem.RedirectErrorOnSpecialBuiltinFatal = c.specialFatal
	sem.FailedExpansionAbandonsTheLine = c.abandons
	sem.FatalErrorStatusIsOne = Yes
	for _, f := range tweak {
		f(&sem)
	}
	dg := Diagnostics{ArithOperandExpected: "bad math", RedirectFailureStatus: c.redirStatus}
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

// heredocFailAnswers is every combination of the three axes a row is graded
// under, for the assertions that must hold whatever any of them says.
func heredocFailAnswers() []heredocFailSemantics {
	var all []heredocFailSemantics
	for _, isRedirs := range []Answer{Yes, No} {
		for _, specialFatal := range []Answer{Yes, No} {
			for _, abandons := range []Answer{Yes, No} {
				all = append(all, heredocFailSemantics{isRedirs, specialFatal, abandons, 0})
			}
		}
	}
	return all
}

// The commands a here-document can be written on that this shell runs itself,
// which is the whole population the axis is about. The external is the control
// and is deliberately not here — see TestTheAxisIsNotAskedForACommandOfItsOwn.
var heredocFailCommands = []struct{ name, src string }{
	{"a special builtin", ": <<END"},
	{"a regular builtin", "echo RAN <<END"},
	{"a function", "f() { echo RAN; }; f <<END"},
	{"a group", "{ echo RAN; } <<END"},
}

// A command whose here-document body could not be expanded does not run.
//
// It ran, or rather it ran and reported success: `: <<END` with `$(( 1/0 ))`
// in the body wrote the arithmetic complaint, left **0** behind and carried
// on, so `: <<END … || handle` never fired and `set -e` never tripped. Three
// of the five panel columns end the shell over it and the fourth leaves the
// failure's status (#4684).
//
// Asserted under every combination of the three axes, because none of them is
// a reason to run the command: what they decide is how far the give-up
// reaches.
func TestACommandWhoseHeredocBodyWouldNotExpandDoesNotRun(t *testing.T) {
	t.Parallel()
	for _, cmd := range heredocFailCommands {
		t.Run(cmd.name, func(t *testing.T) {
			for _, c := range heredocFailAnswers() {
				out, _ := heredocFailRun(t, cmd.src+"\n$(( } ))\nEND\n", c)
				if !strings.Contains(out, "bad math") {
					t.Errorf("%+v: said %q, want the failure reported", c, out)
				}
				if strings.Contains(out, "RAN") {
					t.Errorf("%+v: said %q, want the command left unrun", c, out)
				}
			}
		})
	}
}

// And the status it leaves is never 0, which is the half a script reads.
func TestAFailedHeredocBodyLeavesAFailingStatus(t *testing.T) {
	t.Parallel()
	for _, cmd := range heredocFailCommands {
		t.Run(cmd.name, func(t *testing.T) {
			for _, c := range heredocFailAnswers() {
				out, st := heredocFailRun(t, cmd.src+"\n$(( } ))\nEND\necho \"after st=$?\"\n", c)
				if strings.Contains(out, "after st=0") {
					t.Errorf("%+v: said %q, want the failure's status and not 0", c, out)
				}
				if !strings.Contains(out, "after") && st == 0 {
					t.Errorf("%+v: the shell ended at 0", c)
				}
			}
		})
	}
}

// Whose failure it is, which is the axis and the whole of what it decides.
//
// Where it is the redirection's the command is given up and nothing else is:
// the rest of the line runs, `||` catches it, and the shell stops only where
// a failed redirection on a special builtin stops it. Where it is this
// shell's own failed expansion it costs what any other failed expansion costs
// — the line, or the shell.
//
// The pairing is a snippet on **two lines**, because on one line giving up
// the line and ending the shell print the same nothing.
func TestWhoseFailureAFailedHeredocBodyIs(t *testing.T) {
	t.Parallel()
	const src = ": <<END; echo SAME\n$(( } ))\nEND\necho \"after st=$?\"\n"
	for _, c := range []struct {
		name   string
		sem    heredocFailSemantics
		same   bool
		after  string
		status int
	}{
		{
			// The redirection's, on a special builtin, in a shell that
			// keeps the POSIX rule: the shell stops.
			"the redirection's, and a special builtin's is fatal",
			heredocFailSemantics{Yes, Yes, Yes, 0},
			false, "", 1,
		},
		{
			// The redirection's, where it is not fatal: the command alone
			// goes and the rest of the line runs.
			"the redirection's, and not fatal",
			heredocFailSemantics{Yes, No, Yes, 0},
			true, "after st=0", 0,
		},
		{
			// This shell's own failed expansion, in the reading that gives
			// up the line: the rest of the line goes with it and the next
			// line reads the failure's status.
			"this shell's own, giving up the line",
			heredocFailSemantics{No, No, Yes, 0},
			false, "after st=1", 0,
		},
		{
			// And in the reading that is fatal, whatever the command word
			// was and whatever a failed redirection on it would have cost.
			"this shell's own, and fatal",
			heredocFailSemantics{No, No, No, 0},
			false, "", 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := heredocFailRun(t, src, c.sem)
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

// The command word decides only under the first answer, and that is the pair
// that says the axis is read before the command kind is.
//
// A special builtin and a regular one are graded apart where the failure is
// the redirection's, and identically where it is this shell's own — which is
// the difference between the ksh93/dash/ash column and the bash/zsh one.
func TestTheCommandWordDecidesOnlyWhereTheFailureIsTheRedirections(t *testing.T) {
	t.Parallel()
	ends := func(c heredocFailSemantics, src string) bool {
		t.Helper()
		out, _ := heredocFailRun(t, src+"\n$(( } ))\nEND\necho AFTER\n", c)
		return !strings.Contains(out, "AFTER")
	}
	redirs := heredocFailSemantics{Yes, Yes, Yes, 0}
	if !ends(redirs, ": <<END") {
		t.Error("the redirection's, special builtin: want the shell ended")
	}
	if ends(redirs, "echo RAN <<END") {
		t.Error("the redirection's, regular builtin: want the shell carrying on")
	}
	// The same two words under the other answer, where the axis that split
	// them is never asked: both stop, because a failed expansion is fatal
	// here whatever it was written on.
	own := heredocFailSemantics{No, Yes, No, 0}
	if !ends(own, ": <<END") {
		t.Error("this shell's own, special builtin: want the shell ended")
	}
	if !ends(own, "echo RAN <<END") {
		t.Error("this shell's own, regular builtin: want the shell ended too")
	}
}

// A give-up that is the redirection's is **catchable**, which is what says the
// unwinding was taken back rather than merely renumbered. The other answer's
// is not, and the pair is one snippet asked twice.
func TestOnlyARedirectionsHeredocFailureIsCatchable(t *testing.T) {
	t.Parallel()
	const src = "echo RAN <<END || echo CAUGHT\n$(( } ))\nEND\necho AFTER\n"
	if out, _ := heredocFailRun(t, src, heredocFailSemantics{Yes, Yes, No, 0}); !strings.Contains(out, "CAUGHT") {
		t.Errorf("said %q, want the give-up caught by ||", out)
	}
	if out, _ := heredocFailRun(t, src, heredocFailSemantics{No, Yes, No, 0}); strings.Contains(out, "CAUGHT") {
		t.Errorf("said %q, want a fatal give-up to escape ||", out)
	}
}

// The status a contained give-up leaves is the **redirection's** and not the
// fatal one, where the dialect measured a different number for the two.
//
// One column has: BusyBox ash exits 2 for a fatal error and reports 1 for a
// failed redirection, and a here-document body it could not expand reports 1.
// Every other column numbers them alike, which is why this is asserted here
// with a number no shell has — a row that cannot tell the two apart is not a
// row about which of them was taken.
func TestAContainedHeredocFailureTakesTheRedirectionsStatus(t *testing.T) {
	t.Parallel()
	const src = "echo RAN <<END\n$(( } ))\nEND\necho \"after st=$?\"\n"
	if out, _ := heredocFailRun(t, src, heredocFailSemantics{Yes, Yes, No, 7}); !strings.Contains(out, "after st=7") {
		t.Errorf("said %q, want the redirection's own status", out)
	}
	// The control: with no number of its own the dialect keeps the fatal
	// one, so the row above is the field being read and not a constant.
	if out, _ := heredocFailRun(t, src, heredocFailSemantics{Yes, Yes, No, 0}); !strings.Contains(out, "after st=1") {
		t.Errorf("said %q, want the fatal status where the dialect measured none", out)
	}
	// And the shell that stops over a special builtin exits with it too.
	if _, st := heredocFailRun(t, ": <<END\n$(( } ))\nEND\n", heredocFailSemantics{Yes, Yes, No, 7}); st != 7 {
		t.Errorf("status = %d, want the redirection's own 7", st)
	}
	// The sharper half of the control, and the reason the field is read with
	// a guard rather than through the accessor that fills zero in with 1:
	// where the dialect measured no redirection status, what stands is the
	// **failure's own**, whatever number that is. A dialect whose fatal
	// errors exit 2 and that never measured a redirection status must read 2
	// here — the accessor would say 1 and no row with a fatal status of 1
	// could tell them apart.
	notOne := func(s *Semantics) { s.FatalErrorStatusIsOne = No }
	if out, _ := heredocFailRun(t, src, heredocFailSemantics{Yes, Yes, No, 0}, notOne); !strings.Contains(out, "after st=2") {
		t.Errorf("said %q, want the failure's own status where the dialect measured none", out)
	}
}

// The axis is not asked at all for a command the shell runs as a process of
// its own: the body is expanded in that process, the failure is the child's,
// and every column carries on. Flipping the axis must not move that row.
//
// The control the whole file needs, and the one the panel agrees on.
func TestTheAxisIsNotAskedForACommandOfItsOwn(t *testing.T) {
	t.Parallel()
	const src = "/bin/echo RAN <<END\n$(( } ))\nEND\necho \"after st=$?\"\n"
	redirs, _ := heredocFailRun(t, src, heredocFailSemantics{Yes, Yes, No, 0})
	own, _ := heredocFailRun(t, src, heredocFailSemantics{No, Yes, No, 0})
	if redirs != own {
		t.Errorf("%q under one answer and %q under the other: the axis must not "+
			"reach a command the shell runs as a process of its own", redirs, own)
	}
	// The control the comparison needs: the body really did fail, the
	// command really did not run, and the script really did carry on — two
	// runs that both ended the shell would agree just as readily.
	if !strings.Contains(redirs, "bad math") || strings.Contains(redirs, "RAN") ||
		!strings.Contains(redirs, "after st=1") {
		t.Errorf("= %q, want the failure reported, the command unrun and the script alive", redirs)
	}
}

// A body that expands still runs its command, and a **quoted** delimiter
// leaves a body that would not expand alone entirely.
//
// The two controls that say the door above is opened by a failure and not by
// having a here-document: every column runs both of these at 0.
func TestACleanOrUnexpandedHeredocBodyStillRunsItsCommand(t *testing.T) {
	t.Parallel()
	for _, cmd := range []string{": <<END", "echo RAN <<END", "f() { echo RAN; }; f <<END", "{ echo RAN; } <<END"} {
		for _, body := range []struct{ name, src string }{
			{"a body that expands", cmd + "\nok\nEND\necho \"after st=$?\"\n"},
			{"a quoted delimiter", strings.Replace(cmd, "<<END", "<<'END'", 1) +
				"\n$(( } ))\nEND\necho \"after st=$?\"\n"},
		} {
			for _, c := range heredocFailAnswers() {
				out, st := heredocFailRun(t, body.src, c)
				if strings.Contains(out, "bad math") {
					t.Errorf("%s / %s [%+v]: said %q, want nothing to have failed", cmd, body.name, c, out)
				}
				if !strings.Contains(out, "after st=0") || st != 0 {
					t.Errorf("%s / %s [%+v]: said %q (status %d), want the command run at 0",
						cmd, body.name, c, out, st)
				}
			}
		}
	}
}

// A `<<-` body is a `<<` body with its indentation stripped, and it fails the
// same way — measured in all five columns, and asserted here because the
// stripping happens in a different place from the expansion.
func TestAnIndentedHeredocBodyFailsTheSameWay(t *testing.T) {
	t.Parallel()
	for _, c := range heredocFailAnswers() {
		plain, plainSt := heredocFailRun(t, ": <<END\n$(( } ))\nEND\necho \"after st=$?\"\n", c)
		dashed, dashedSt := heredocFailRun(t, ": <<-END\n\t$(( } ))\n\tEND\necho \"after st=$?\"\n", c)
		if plain != dashed || plainSt != dashedSt {
			t.Errorf("[%+v] %q at %d for `<<` and %q at %d for `<<-`", c, plain, plainSt, dashed, dashedSt)
		}
	}
}

// A clean assignment prefix in front of the command must not move any of it,
// and neither must the body's failure move what the *prefix* costs.
//
// The two constructs set the same flag and are given up at different doors —
// this one and interp/prefixexpansionfailed.go's — so the pair is kept here
// as well as there. #4675's first draft had the defect in the other
// direction: a clean prefix in front of `: <<END` with a failing body changed
// that command's answer, which no shell in the panel does.
func TestAHeredocBodysFailureIsNotThePrefixs(t *testing.T) {
	t.Parallel()
	const body = " : <<END\n$(( } ))\nEND\necho \"after st=$?\"\n"
	moved := false
	for _, c := range heredocFailAnswers() {
		bare, bareSt := heredocFailRun(t, body, c)
		with, withSt := heredocFailRun(t, "a=ok"+body, c)
		if bare != with || bareSt != withSt {
			t.Errorf("[%+v] %q at %d with no prefix, %q at %d with one: a clean prefix "+
				"must not move a failure that is not its own", c, bare, bareSt, with, withSt)
		}
		if strings.Contains(bare, "bad math") && strings.Contains(bare, "after") {
			moved = true
		}
	}
	// The control the rows above need: some row reported the failure and
	// still ran the next command, so the agreement is two shells doing
	// something rather than two shells doing nothing.
	if !moved {
		t.Error("no row reported the failure and still ran the next command: " +
			"the probe never reached the case it is about")
	}
}

// A body that will not **parse** is not a body that will not expand, and the
// two are told apart by the **number** they leave.
//
// #4684 kept them apart by excluding the parse failure from this axis
// altogether, and that was too much: the exclusion also left the refusal's
// abandonment standing on every command this shell runs itself, which ended
// the shell in ten rows where the reference carries on (#4687). The two now
// read the same axis through two doors, and what this guards is the reason
// there are two: a refusal carries a status of its own and keeps it, where a
// failed expansion takes the redirection's.
//
// 7 is a number no shell has, so a row that takes it took the redirection's
// and a row that does not took the refusal's.
func TestABodyThatWillNotParseKeepsItsOwnStatusAndAFailedExpansionDoesNot(t *testing.T) {
	t.Parallel()
	sem := heredocFailSemantics{Yes, Yes, No, 7}
	// The column that carries on over a refusal, which is the only one that
	// can show what number it left: where the shell ends there is nobody to
	// read it.
	carries := func(s *Semantics) {
		s.SubstitutionParseFailureInAHeredocBodyEndsTheShell = No
		s.SubstitutionParseFailureCarriesTheFatalStatus = No
	}
	const parses = "echo RAN <<END\n$(echo hi; for)\nEND\necho \"after st=$?\"\n"
	const expands = "echo RAN <<END\n$(( } ))\nEND\necho \"after st=$?\"\n"
	// The failed expansion takes the redirection's 7, which is what this
	// file's axis is for and what says the probe can produce a positive.
	if out, _ := heredocFailRun(t, expands, sem); !strings.Contains(out, "after st=7") {
		t.Errorf("a failed expansion said %q, want the redirection's 7", out)
	}
	// And the refusal does not: it keeps the syntax status it already had.
	out, st := heredocFailRun(t, parses, sem, carries)
	if strings.Contains(out, "st=7") || st == 7 {
		t.Errorf("a refusal said %q at %d, want its own status and not the redirection's", out, st)
	}
	if !strings.Contains(out, "after st=2") {
		t.Errorf("a refusal said %q, want the syntax status it already carried", out)
	}
	// The control both halves need: each really did fail, and neither ran
	// its command — two runs that did nothing would agree about 7 as
	// readily.
	if strings.Contains(out, "RAN") {
		t.Errorf("= %q, want the command left unrun", out)
	}
	if !strings.Contains(out, "syntax") && !strings.Contains(out, "unexpected") {
		t.Errorf("= %q, want the refusal reported", out)
	}
}

// And where no dialect answered the axis, the shell says so and runs nothing:
// acting on either reading after reporting that the shells disagree would
// answer the question anyway.
func TestAnUnansweredHeredocBodyFailureRunsNothing(t *testing.T) {
	t.Parallel()
	sem := permissive()
	sem.FatalErrorStatusIsOne = Yes
	dg := Diagnostics{ArithOperandExpected: "bad math"}
	var out bytes.Buffer
	dir := t.TempDir()
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "sh", Vars: map[string]string{"PATH": dir},
	})
	f, err := syntax.Parse("echo RAN <<END\n$(( } ))\nEND\necho \"after st=$?\"\n", syntax.Core())
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
	if !strings.Contains(got, "after st=2") {
		t.Errorf("= %q, want the refusal's own status", got)
	}
}
