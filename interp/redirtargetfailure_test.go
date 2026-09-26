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

// targetFailSemantics is the vector these rows run under: the new axis at the
// caller's answer, and the two axes that grade each of its two readings.
//
// `bad math` is the arithmetic complaint, so a row can tell the failure's own
// sentence from anything the command would have printed.
type targetFailSemantics struct {
	// isRedirs answers whose failure a target that will not expand is.
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

func targetFailRun(t *testing.T, src string, c targetFailSemantics) (string, int) {
	t.Helper()
	sem := permissive()
	sem.RedirectTargetFailureIsTheRedirections = c.isRedirs
	sem.RedirectErrorOnSpecialBuiltinFatal = c.specialFatal
	sem.FailedExpansionAbandonsTheLine = c.abandons
	sem.FatalErrorStatusIsOne = Yes
	// The wider question the target's expansion reaches first, answered so
	// that the rows below are about this axis alone: a refusal there sets
	// r.unspecified and every later reading is deliberately left alone.
	sem.RedirectTargetIsAnOrdinaryWord = No
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

// targetFailAnswers is every combination of the three axes a row is graded
// under, for the assertions that must hold whatever any of them says.
func targetFailAnswers() []targetFailSemantics {
	var all []targetFailSemantics
	for _, isRedirs := range []Answer{Yes, No} {
		for _, specialFatal := range []Answer{Yes, No} {
			for _, abandons := range []Answer{Yes, No} {
				all = append(all, targetFailSemantics{isRedirs, specialFatal, abandons, 0})
			}
		}
	}
	return all
}

// The commands a redirection can be written on that this shell runs itself,
// which is the whole population the axis is about. The external is the
// control and is deliberately not here — see
// TestTheTargetAxisIsNotAskedForACommandOfItsOwn.
var targetFailCommands = []struct{ name, src string }{
	{"a special builtin", `: < $(( } ))`},
	{"a regular builtin", `echo RAN < $(( } ))`},
	{"a function", `f() { echo RAN; }; f < $(( } ))`},
	{"a group", `{ echo RAN; } < $(( } ))`},
}

// A command whose redirection target could not be expanded does not run, and
// the failure is reported — whatever any of the three axes says, because none
// of them is a reason to run it. What they decide is how far the give-up
// reaches.
func TestACommandWhoseTargetWouldNotExpandDoesNotRun(t *testing.T) {
	t.Parallel()
	for _, cmd := range targetFailCommands {
		t.Run(cmd.name, func(t *testing.T) {
			for _, c := range targetFailAnswers() {
				out, _ := targetFailRun(t, cmd.src+"\n", c)
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
func TestAFailedTargetLeavesAFailingStatus(t *testing.T) {
	t.Parallel()
	for _, cmd := range targetFailCommands {
		t.Run(cmd.name, func(t *testing.T) {
			for _, c := range targetFailAnswers() {
				out, st := targetFailRun(t, cmd.src+"\necho \"after st=$?\"\n", c)
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
// Yes is the redirection's, so it reaches exactly as far as a failed
// redirection does here — the shell over a special builtin and the command
// over everything else. No is this shell's own failed expansion, which is
// fatal wherever it is written unless the line is what this shell gives up.
func TestWhoseFailureAFailedTargetIs(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		sem  targetFailSemantics
		src  string
		ends bool
	}{
		{"the redirections, on a special builtin", targetFailSemantics{Yes, Yes, No, 0}, `: < $(( } ))`, true},
		{"the redirections, on a regular builtin", targetFailSemantics{Yes, Yes, No, 0}, `echo RAN < $(( } ))`, false},
		{"the redirections, on a function", targetFailSemantics{Yes, Yes, No, 0}, `f() { echo RAN; }; f < $(( } ))`, false},
		{"the redirections, on a group", targetFailSemantics{Yes, Yes, No, 0}, `{ echo RAN; } < $(( } ))`, false},
		{"the redirections, none of them fatal", targetFailSemantics{Yes, No, No, 0}, `: < $(( } ))`, false},
		{"this shell's expansion, on a special builtin", targetFailSemantics{No, Yes, No, 0}, `: < $(( } ))`, true},
		{"this shell's expansion, on a regular builtin", targetFailSemantics{No, Yes, No, 0}, `echo RAN < $(( } ))`, true},
		{"this shell's expansion, on a function", targetFailSemantics{No, Yes, No, 0}, `f() { echo RAN; }; f < $(( } ))`, true},
		{"this shell's expansion, on a group", targetFailSemantics{No, Yes, No, 0}, `{ echo RAN; } < $(( } ))`, true},
		{"this shell's expansion, giving up the line", targetFailSemantics{No, Yes, Yes, 0}, `echo RAN < $(( } ))`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := targetFailRun(t, c.src+"\necho after\n", c.sem)
			if ends := !strings.Contains(out, "after"); ends != c.ends {
				t.Errorf("said %q, want the shell ending=%v", out, c.ends)
			}
		})
	}
}

// The pair that holds the noun fixed. Under the redirection's reading a
// **function** and a **group** carry on where a special builtin does not,
// which is what says the noun is "a special builtin" and not "a command this
// shell runs itself" — the two readings agree on every other row.
func TestTheFailedTargetNounIsASpecialBuiltinNotACommandThisShellRuns(t *testing.T) {
	t.Parallel()
	sem := targetFailSemantics{Yes, Yes, No, 0}
	special, _ := targetFailRun(t, ": < $(( } ))\necho after\n", sem)
	if strings.Contains(special, "after") {
		t.Errorf("a special builtin said %q, want the shell ended", special)
	}
	for _, c := range []struct{ name, src string }{
		{"a function", "f() { echo RAN; }; f < $(( } ))"},
		{"a group", "{ echo RAN; } < $(( } ))"},
	} {
		out, _ := targetFailRun(t, c.src+"\necho after\n", sem)
		if !strings.Contains(out, "after") {
			t.Errorf("%s said %q, want this shell carrying on", c.name, out)
		}
	}
}

// Under the redirection's reading the contained give-up is catchable by `||`
// and the fatal one is not, which is the third column a script can see.
func TestOnlyTheContainedTargetFailureIsCatchable(t *testing.T) {
	t.Parallel()
	sem := targetFailSemantics{Yes, Yes, No, 0}
	if out, _ := targetFailRun(t, "echo RAN < $(( } )) || echo CAUGHT\n", sem); !strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want a regular builtin's give-up caught by ||", out)
	}
	if out, _ := targetFailRun(t, ": < $(( } )) || echo CAUGHT\n", sem); strings.Contains(out, "CAUGHT") {
		t.Errorf("= %q, want a special builtin's give-up to escape ||", out)
	}
}

// And the number the contained one leaves is a failed redirection's, which is
// the column that separates the two readings where a shell numbers a failed
// redirection and a fatal error differently.
func TestAContainedTargetFailureLeavesTheRedirectionsStatus(t *testing.T) {
	t.Parallel()
	out, _ := targetFailRun(t, "echo RAN < $(( } ))\necho \"after st=$?\"\n",
		targetFailSemantics{Yes, Yes, No, 7})
	if !strings.Contains(out, "after st=7") {
		t.Errorf("= %q, want the redirection's own status", out)
	}
	// The control the comparison needs: the other reading does not take it,
	// so the 7 above is this axis and not something every failure picks up.
	out, _ = targetFailRun(t, "echo RAN < $(( } ))\necho \"after st=$?\"\n",
		targetFailSemantics{No, Yes, Yes, 7})
	if strings.Contains(out, "after st=7") {
		t.Errorf("= %q, want the failed expansion's own status", out)
	}
}

// The axis is not asked for a command this shell runs as a process of its
// own, under **either** answer to the sibling question.
//
// Where that word was expanded is
// Semantics.RedirectTargetExpandsInTheCommandsProcess's, and the answer to it
// already says whose the failure is: Yes puts it in the child, and No says
// the shell expanded it for a command that is still a process of its own,
// which is what makes dash and BusyBox ash end over an external's target.
// Neither leaves this axis anything to decide, and a dialect that never
// answered this one must not be made to refuse over a command it never
// covers — which is exactly what the `unspecified` row below would catch.
func TestTheTargetAxisIsNotAskedForACommandOfItsOwn(t *testing.T) {
	t.Parallel()
	for _, inTheCommand := range []Answer{Yes, No} {
		for _, isRedirs := range []Answer{Yes, No, Unspecified} {
			sem := permissive()
			sem.RedirectTargetFailureIsTheRedirections = isRedirs
			sem.RedirectTargetExpandsInTheCommandsProcess = inTheCommand
			sem.RedirectErrorOnSpecialBuiltinFatal = Yes
			sem.FatalErrorStatusIsOne = Yes
			sem.RedirectTargetIsAnOrdinaryWord = No
			dg := Diagnostics{ArithOperandExpected: "bad math"}
			var out bytes.Buffer
			dir := t.TempDir()
			r := newTestRunner(t, &Runner{
				Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &dg,
				Dir: dir, Name: "sh", Vars: map[string]string{"PATH": dir},
			})
			f, err := syntax.Parse("/bin/echo RAN < $(( } ))\necho after\n", syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatalf("run: %v", err)
			}
			got := out.String()
			if strings.Contains(got, "whose failure a redirection target") {
				t.Errorf("inTheCommand=%v isRedirs=%v: said %q, want this axis never asked",
					inTheCommand, isRedirs, got)
			}
			// And the sibling's answer is what decides the reach, as it did
			// before this axis existed: the child's failure costs the
			// command, and the shell's own costs the shell.
			if carriedOn := strings.Contains(got, "after"); carriedOn != (inTheCommand == Yes) {
				t.Errorf("inTheCommand=%v isRedirs=%v: said %q, want the sibling axis to decide the reach",
					inTheCommand, isRedirs, got)
			}
		}
	}
}

// A dialect that never answered has to say so rather than pick a reading.
//
// The command does not run — that much is true of both readings — and
// **neither reading is acted on**: the give-up is not taken and the failed
// expansion is not either, so the script is left standing exactly where the
// refusal found it. Acting on one of them after saying the shells disagree
// would answer the question anyway, which is the guard the same boundary one
// construct over already carries.
func TestAFailedTargetWithNobodyAnswering(t *testing.T) {
	t.Parallel()
	out, _ := targetFailRun(t, "echo RAN < $(( } ))\necho after\n",
		targetFailSemantics{Unspecified, Yes, No, 0})
	if !strings.Contains(out, "whose failure a redirection target that will not expand is") {
		t.Errorf("= %q, want the refusal to name the axis", out)
	}
	if strings.Contains(out, "RAN") {
		t.Errorf("= %q, want the command left unrun", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("= %q, want neither reading acted on — the script left standing", out)
	}
	// A **special builtin** is not part of that claim and is the row to read
	// carefully: it ends the shell here too, and not because a reading was
	// taken. The command not running is what both readings agree on, and a
	// failed redirection on a special builtin is fatal wherever it comes
	// from — see RedirectErrorOnSpecialBuiltinFatal, which is answered Yes
	// in this vector. So the refusal narrows the shell's fate on every
	// command but that one.
	special, _ := targetFailRun(t, ": < $(( } ))\necho after\n",
		targetFailSemantics{Unspecified, Yes, No, 0})
	if strings.Contains(special, "after") {
		t.Errorf("a special builtin = %q, want the redirection failure still fatal there", special)
	}
}

// The control the whole file rests on: a target that expands is opened and its
// command runs, whatever the axis says. A here-document body is the pair one
// construct over, and it is a *different* field — see
// Semantics.HeredocBodyFailureIsTheRedirections and the dash and ash rows it
// parts from this one on.
func TestATargetThatExpandsStillOpens(t *testing.T) {
	t.Parallel()
	for _, c := range targetFailAnswers() {
		out, st := targetFailRun(t, "echo RAN < /dev/null\necho \"after st=$?\"\n", c)
		if !strings.Contains(out, "RAN") || !strings.Contains(out, "after st=0") || st != 0 {
			t.Errorf("%+v: said %q (status %d), want the command run at 0", c, out, st)
		}
	}
}
