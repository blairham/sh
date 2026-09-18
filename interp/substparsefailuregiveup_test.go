// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// How far a substitution body that will not parse reaches, and what number it
// leaves behind, where the failure is not the script's own line being read.
//
// Two axes and two shapes. A **here-document body** is the one redirection
// half the panel splits on — see
// Semantics.SubstitutionParseFailureInAHeredocBodyEndsTheShell, where the
// measurement is — and the redirection *target* is the control that says the
// question is the body's rather than redirections' as a class. The number is
// Semantics.SubstitutionParseFailureCarriesTheFatalStatus, read at that
// boundary and again where the shell ends from text it borrowed.
//
// The body has to feed a command this shell runs as a process of its own,
// which is what puts the expansion behind the boundary at all — see
// heredocprocess.go. `cat` is that command here, as it is in every other
// suite in this file's neighborhood.

// runSubstGiveUp runs src with the two axes set and returns the lines the
// script itself printed — the refusal's own wording is a different suite's
// subject and is dropped — along with what the shell exited with.
func runSubstGiveUp(t *testing.T, src string, endsTheShell, fatalStatus Answer) (string, int) {
	t.Helper()
	out, st := runSubstGiveUpRaw(t, src, endsTheShell, fatalStatus)
	var kept []string
	for _, line := range strings.Split(out, "\n") {
		if line != "" && !strings.Contains(line, "unexpected") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n"), st
}

func runSubstGiveUpRaw(t *testing.T, src string, endsTheShell, fatalStatus Answer) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		s := *r.Semantics
		s.SubstitutionParseFailureInAHeredocBodyEndsTheShell = endsTheShell
		s.SubstitutionParseFailureCarriesTheFatalStatus = fatalStatus
		// The failure is not caught at a `.` or an `eval` in either reading
		// here, so the borrowed rows are about the status alone and not about
		// whether the script ends.
		s.FatalErrorEndsBorrowedTextOnly = No
		s.FatalErrorStatusIsOne = Yes
		r.Semantics = &s
	})
}

func TestASubstitutionRefusedInAHereDocumentBodyEndsTheShellWhereTheAxisSaysSo(t *testing.T) {
	// `cat` never runs in either reading — that half is settled at every
	// boundary giveUpTheCommand is — so what each row asks is whether `after`
	// is reached.
	const src = "printf 'start\\n'\ncat <<END\nbefore $(echo hi; for) after\nEND\nprintf 'after\\n'\n"
	t.Run("the stop stands", func(t *testing.T) {
		out, _ := runSubstGiveUp(t, src, Yes, No)
		if out != "start" {
			t.Errorf("out = %q, want just the first line — the script ends at the refusal", out)
		}
	})
	t.Run("the stop is taken away", func(t *testing.T) {
		out, _ := runSubstGiveUp(t, src, No, No)
		if out != "start\nafter" {
			t.Errorf("out = %q, want both lines and no `cat` output — the command is given up and the script runs on", out)
		}
	})
}

// And the control: a redirection *target* holding the same substitution ends
// the script whichever way the body's axis is set, because the axis is asked
// at the body's boundary and nowhere else.
func TestASubstitutionRefusedInARedirectionTargetEndsTheShellWhateverTheBodySays(t *testing.T) {
	const src = "printf 'start\\n'\ncat < \"$(echo hi; for)\"\nprintf 'after\\n'\n"
	for _, a := range []Answer{Yes, No} {
		out, _ := runSubstGiveUp(t, src, a, No)
		if out != "start" {
			t.Errorf("with the body axis %v: out = %q, want just the first line", a, out)
		}
	}
}

// The number the carried-on shell leaves behind, which is the second axis and
// is read at the same boundary.
func TestTheStatusASubstitutionRefusalLeavesInAHereDocumentBody(t *testing.T) {
	const src = "cat <<END\n$(echo hi; for)\nEND\nprintf 'st=%s' \"$?\"\n"
	t.Run("a fatal error's number", func(t *testing.T) {
		out, _ := runSubstGiveUp(t, src, No, Yes)
		if !strings.HasSuffix(out, "st=1") {
			t.Errorf("out = %q, want it to end in st=1", out)
		}
	})
	t.Run("the refusal's own number", func(t *testing.T) {
		out, _ := runSubstGiveUp(t, src, No, No)
		if !strings.HasSuffix(out, "st=2") {
			t.Errorf("out = %q, want it to end in st=2 — the syntax status the refusal already set", out)
		}
	})
}

// The same axis at the other site: text the script borrowed, where the shell
// ends rather than carrying on and the number is its exit status.
func TestTheStatusASubstitutionRefusalEndsBorrowedTextWith(t *testing.T) {
	const src = "printf 'start\\n'\neval 'v=$(echo hi; for)'\nprintf 'never\\n'\n"
	if out, st := runSubstGiveUp(t, src, Yes, Yes); st != 1 || strings.Contains(out, "never") {
		t.Errorf("out = %q, status = %d, want the script to end at 1 — a fatal error's number", out, st)
	}
	if out, st := runSubstGiveUp(t, src, Yes, No); st != 2 || strings.Contains(out, "never") {
		t.Errorf("out = %q, status = %d, want the script to end at 2 — the syntax status the refusal already set", out, st)
	}
}

// And the row that must not move: the script's own line, which every column
// answers with the syntax status and where the axis is never asked.
func TestASubstitutionRefusalOnTheScriptsOwnLineKeepsItsSyntaxStatus(t *testing.T) {
	const src = "printf 'start\\n'\nv=$(echo hi; for)\nprintf 'never\\n'\n"
	for _, a := range []Answer{Yes, No} {
		out, st := runSubstGiveUp(t, src, Yes, a)
		if st != 2 {
			t.Errorf("with the status axis %v: status = %d, want 2", a, st)
		}
		if strings.Contains(out, "never") {
			t.Errorf("with the status axis %v: out = %q, want the script to end at the refusal", a, out)
		}
	}
}

// The parser is asked for the failing body directly, so that a change to what
// `for` on its own reports cannot quietly turn these rows into a suite about
// something else.
func TestTheBodyTheseRowsUseDoesNotParse(t *testing.T) {
	if _, err := syntax.Parse("echo hi; for", syntax.Core()); err == nil {
		t.Fatal("the body parsed, so every row above is measuring a shell that never refused anything")
	}
}
