// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// fdMove is the vector these tests vary: the one axis under test, with
// everything the snippets touch answered so that nothing else is being asked.
func fdMove(dir string, form FdMoveForm, dg Diagnostics) func(*Runner) {
	return func(r *Runner) {
		sem := CoreSemantics()
		sem.GreatAmpTarget = GreatAmpTargetIsADescriptor
		sem.MultiDigitDuplicationTargetIsAnError = No
		sem.RedirectErrorOnSpecialBuiltinFatal = No
		sem.DuplicationTargetErrorOnABuiltinIsFatal = No
		sem.FdVariableOutlivesTheCommand = No
		sem.FdVariableBadCloseIsAnError = No
		// Not this axis: the snippets run `cat` with a descriptor `exec`
		// opened, which asks it on the way to asking this one.
		sem.ExecOpenedFdReachesACommand = Yes
		sem.FdMove = form
		r.Semantics, r.Diagnostics, r.Dir = &sem, &dg, dir
	}
}

// braces turns on the `{name}<&` spelling, which the core grammar does not
// have — the tests that need it say so rather than borrowing a shell.
func braces(d *syntax.Dialect) { d.FdVariableRedirections = true }

// The operator itself, in both directions and under both readings of it: 6
// becomes a copy of 5 and 5 stops being a name for anything. `exec` is where
// the two forms agree, because nothing is taken back afterwards.
func TestAMoveCopiesTheDescriptorAndClosesTheSource(t *testing.T) {
	for _, form := range []FdMoveForm{FdMoveDuplicatesThenCloses, FdMoveRelocates} {
		t.Run(form.String(), func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "f", "body\n")
			dg := Diagnostics{DuplicationSourceNotOpen: "%[1]s: %[2]s"}
			src := `exec 5< f; exec 6<&5-; cat <&6; cat <&5; printf "[%s]" "$?"`
			out, _ := run(t, src, fdMove(dir, form, dg))
			if !strings.Contains(out, "body\n") {
				t.Errorf("out = %q, want the file read through the descriptor it moved to", out)
			}
			if !strings.Contains(out, "[1]") {
				t.Errorf("out = %q, want the source closed and the read of it refused", out)
			}
		})
	}
}

// The writing side is the same operator. Worth its own test because the two
// directions reach the descriptor table by different arms.
func TestAMoveOnTheWritingSideCarriesTheFileWithIt(t *testing.T) {
	for _, form := range []FdMoveForm{FdMoveDuplicatesThenCloses, FdMoveRelocates} {
		t.Run(form.String(), func(t *testing.T) {
			dir := t.TempDir()
			src := `exec 5> f; exec 6>&5-; echo moved >&6; exec 6>&-; echo x >&5; printf "[%s]" "$?"`
			out, _ := run(t, src, fdMove(dir, form, Diagnostics{DuplicationSourceNotOpen: "%[1]s: %[2]s"}))
			if got := readFile(t, dir, "f"); got != "moved\n" {
				t.Errorf("f = %q, want the line written through the number it moved to", got)
			}
			if !strings.Contains(out, "[1]") {
				t.Errorf("out = %q, want the write through the closed source refused", out)
			}
		})
	}
}

// The property the two forms are an axis *for*: a redirection on an ordinary
// command is undone when the command ends, and the forms disagree about
// whether the move's close is part of what is undone.
func TestWhetherAMovedSourceComesBackWhenTheCommandEnds(t *testing.T) {
	for _, tc := range []struct {
		form FdMoveForm
		want string
	}{
		{FdMoveDuplicatesThenCloses, "[1]"},
		{FdMoveRelocates, "[0]"},
	} {
		t.Run(tc.form.String(), func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "f", "body\n")
			dg := Diagnostics{DuplicationSourceNotOpen: "%[1]s: %[2]s"}
			src := `exec 5< f; true 6<&5-; cat <&5 >/dev/null; printf "[%s]" "$?"`
			out, _ := run(t, src, fdMove(dir, tc.form, dg))
			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %q, want %s — the source's close is %s the command's to take back",
					out, tc.want, map[string]string{"[1]": "not", "[0]": ""}[tc.want])
			}
		})
	}
}

// And the destination is the command's under both, which is what says the
// axis is about the *close* and not about redirections in general.
func TestAMovesDestinationIsAlwaysTheCommandsToTakeBack(t *testing.T) {
	for _, form := range []FdMoveForm{FdMoveDuplicatesThenCloses, FdMoveRelocates} {
		t.Run(form.String(), func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "f", "body\n")
			dg := Diagnostics{DuplicationSourceNotOpen: "%[1]s: %[2]s"}
			src := `exec 5< f; true 6<&5-; cat <&6 >/dev/null; printf "[%s]" "$?"`
			out, _ := run(t, src, fdMove(dir, form, dg))
			if !strings.Contains(out, "[1]") {
				t.Errorf("out = %q, want the destination gone once the command it was made for has ended", out)
			}
		})
	}
}

// The control the axis is measured against: a plain close is not a move, and
// is undone when the command ends under every form. The `-` on its own never
// reaches the axis at all — it is the close every shell in the panel has.
func TestAPlainCloseIsNotAMoveAndComesBack(t *testing.T) {
	for _, form := range []FdMoveForm{
		FdMoveIsNotAnOperator, FdMoveDuplicatesThenCloses, FdMoveRelocates,
	} {
		t.Run(form.String(), func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "f", "body\n")
			dg := Diagnostics{DuplicationSourceNotOpen: "%[1]s: %[2]s"}
			src := `exec 5< f; true 5<&-; cat <&5; printf "[%s]" "$?"`
			out, _ := run(t, src, fdMove(dir, form, dg))
			if out != "body\n[0]" {
				t.Errorf("out = %q, want the close undone with the command that wrote it", out)
			}
		})
	}
}

// Moving a descriptor onto its own number is a descriptor that is still open.
// The close half has to see that the duplication already put the source where
// the close would have taken it from.
func TestAMoveOntoItsOwnNumberDoesNotCloseIt(t *testing.T) {
	for _, form := range []FdMoveForm{FdMoveDuplicatesThenCloses, FdMoveRelocates} {
		t.Run(form.String(), func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "f", "body\n")
			dg := Diagnostics{DuplicationSourceNotOpen: "%[1]s: %[2]s"}
			out, _ := run(t, `exec 5< f; exec 5<&5-; cat <&5; printf "[%s]" "$?"`, fdMove(dir, form, dg))
			if out != "body\n[0]" {
				t.Errorf("out = %q, want the descriptor still open after moving it to itself", out)
			}
		})
	}
}

// The second thing the two forms disagree about, and the one a script can
// read back: which number `{name}<&$w-` hands the name. The relocating form
// has given the source's number up by the time it chooses, so the name
// receives that number; the duplicating form chooses first.
func TestWhichNumberANamedMoveReceives(t *testing.T) {
	for _, tc := range []struct {
		form FdMoveForm
		want string
	}{
		{FdMoveDuplicatesThenCloses, "[10][11]"},
		{FdMoveRelocates, "[10][10]"},
	} {
		t.Run(tc.form.String(), func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "f", "body\n")
			src := `exec {w}< f; exec {v}<&$w-; printf "[%s][%s]" "$w" "$v"`
			out, st := runGrammar(t, src, braces, fdMove(dir, tc.form, Diagnostics{}))
			if st != 0 {
				t.Fatalf("status %d, out %q", st, out)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// Where the operator does not exist the `-` stays in the word, and the word
// is refused by whatever refuses a word that is not a descriptor. Routed back
// through that path rather than given a refusal of its own, because the three
// shells without the operator do not agree on what to say.
func TestWithoutTheOperatorTheDashStaysInTheWord(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "body\n")
	dg := Diagnostics{DuplicationTargetIsNotADescriptor: "%[2]s: not a descriptor"}
	out, _ := run(t, `exec 5< f; exec 6<&5-; printf "[%s]" "$?"`,
		fdMove(dir, FdMoveIsNotAnOperator, dg))
	if out != "sh: 5-: not a descriptor\n[1]" {
		t.Errorf("out = %q, want the whole word refused, dash and all", out)
	}
}

// An axis with no answer refuses, and says which question was not answered
// rather than inventing a reading. Asked only where a move was written — see
// the control below.
func TestAnUnansweredMoveAxisRefuses(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "body\n")
	out, st := run(t, `exec 5< f; exec 6<&5-`, fdMove(dir, FdMoveUnspecified, Diagnostics{}))
	if st != 2 {
		t.Errorf("status %d, want 2", st)
	}
	if !strings.Contains(out, "a trailing `-` on a duplication target") {
		t.Errorf("out = %q, want the unanswered axis named", out)
	}
}

// And an ordinary duplication still runs under the same unanswered axis,
// which is what "asked only where it matters" has to mean here: `6<&5` is
// nobody's question and must not be refused for a question it never raises.
func TestAnUnansweredMoveAxisStillDuplicates(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "body\n")
	out, st := run(t, `exec 5< f; exec 6<&5; cat <&6`, fdMove(dir, FdMoveUnspecified, Diagnostics{}))
	if st != 0 || out != "body\n" {
		t.Errorf("out = %q status %d, want the plain duplication untouched", out, st)
	}
}

// A move from a number nothing is open at fails the redirection, and the two
// shells that have the operator quote different amounts of the word back.
// NamesTheMoveSuffixInTheTarget is that choice, and it is the opposite way
// round from NamesTheDuplicationTargetAsWritten — which is why it is a field
// of its own rather than a second reading of that one.
func TestWhatAFailedMoveNamesAsItsSource(t *testing.T) {
	for _, tc := range []struct {
		name string
		dg   Diagnostics
		want string
	}{
		{
			"the number alone",
			Diagnostics{
				DuplicationSourceNotOpen:           "%[1]s: %[2]s",
				NamesTheDuplicationTargetAsWritten: true,
			},
			"sh: 5: ",
		},
		{
			"the word with its suffix",
			Diagnostics{
				DuplicationSourceNotOpen:      "%[1]s: %[2]s",
				NamesTheMoveSuffixInTheTarget: true,
			},
			"sh: 5-: ",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			out, st := run(t, `exec 6<&5-`, fdMove(dir, FdMoveDuplicatesThenCloses, tc.dg))
			if st == 0 {
				t.Fatalf("status 0, want the redirection to have failed; out %q", out)
			}
			if !strings.HasPrefix(out, tc.want) {
				t.Errorf("out = %q, want it to start %q", out, tc.want)
			}
		})
	}
}
