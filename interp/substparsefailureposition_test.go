// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// **Where** a substitution that will not parse was written decides which axis
// says how far its failure reaches, and a here-document body is graded by the
// body's own axis in every position.
//
// See interp/substitutionstop.go, where the box is, and
// Runner.substParseErrorEscapesASubshell, which is the one line these rows
// are about. The neighbor suite in substparsefailuregiveup_test.go asserts
// the body's axis on a command of its own at the top level; what is here is
// every position where the command is *also* a subshell, which is where the
// two axes used to be asked of one failure.
//
// Measured 2026-09-26 over a script file under `env -i PATH=/usr/bin:/bin
// HOME=<scratch> LC_ALL=C` with stdin from /dev/null, against
// /opt/homebrew/bin/bash 5.3.20, /bin/bash 3.2.57, /opt/homebrew/bin/zsh
// 5.9.2 under `-f`, /bin/ksh ksh93u+ 2012-08-01 (AT&T's build), /bin/dash
// 0.5.12 and BusyBox v1.37.0 in the digest-pinned alpine image. bash is the
// column that decides: it answers Yes to the word's axis and No to the
// body's, so a `$( … )` in a word on a pipeline element ends its script at 2
// and the same `$( … )` in a here-document body on the same pipeline element
// runs the rest of the line and leaves the script at 0.

// substPositionSem is the vector these rows run under: bash's answers for
// everything the positions below reach, with the two axes under test named.
func substPositionSem(bodyEndsTheShell, wordEscapes Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.SubstitutionParseFailureInAHeredocBodyEndsTheShell = bodyEndsTheShell
		s.SubstitutionParseErrorEscapesASubshell = wordEscapes
		// The older spelling reaches the same box, and only once it is fatal
		// — which is the row the backquote control below needs.
		s.SubstitutionParseErrorIsFatal = Yes
		s.SubstitutionParseFailureCarriesTheFatalStatus = No
		s.FatalErrorEndsBorrowedTextOnly = No
		s.FatalErrorStatusIsOne = Yes
		// Whose failure a body that will not parse is, and how far a failed
		// expansion reaches — the two that grade the readings underneath,
		// held at one answer each so that nothing here moves with them.
		s.HeredocBodyFailureIsTheRedirections = No
		s.FailedExpansionAbandonsTheLine = Yes
		// A pipeline's last element is a subshell of its own, which is what
		// puts a here-document on it in the same place as the others.
		s.LastPipelineElementInCurrentShell = No
		s.HeredocBodyOnASubshellExpandsInTheSubshell = Yes
		r.Semantics = &s
	}
}

// runSubstPosition runs src and answers whether the script reached its last
// line. The refusal's own wording is a different suite's subject.
func runSubstPosition(t *testing.T, src string, bodyEndsTheShell, wordEscapes Answer) (carriedOn bool, out string) {
	t.Helper()
	out, _ = runGrammar(t, src, nil, substPositionSem(bodyEndsTheShell, wordEscapes))
	return strings.Contains(out, "AFTER"), out
}

// The positions a here-document can be written in where the command is a
// subshell of this shell — the places the word's axis used to be asked. The
// first is the control at the top level, where there is no subshell at all.
var substBodyPositions = []struct{ name, src string }{
	{"a command of its own", "cat <<END"},
	{"the first element of a pipeline", "cat <<END | cat"},
	{"a middle element of a pipeline", "cat | cat <<END | cat"},
	{"the last element of a pipeline", "cat | cat <<END"},
	{"a background command", "cat <<END &\nwait"},
	{"a command inside a subshell", "( cat <<END )"},
	{"a pipeline inside a subshell", "( cat <<END | cat )"},
	{"a group on a pipeline element", "{ cat; } <<END | cat"},
	{"a builtin on a pipeline element", "read x <<END | cat"},
	{"a function on a pipeline element", "f() { cat; }\nf <<END | cat"},
}

// The body's axis decides in every position, and the word's decides in none
// of them.
//
// Both answers are asserted in each position, because "the script carried on"
// is also what a row that never reached the failure says. The word's axis is
// held at Yes throughout — the answer bash, zsh, dash and BusyBox ash all
// give — so a position that read it would end the script on the No row.
func TestAHeredocBodyThatWillNotParseIsGradedByItsOwnAxisInEveryPosition(t *testing.T) {
	t.Parallel()
	for _, p := range substBodyPositions {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			src := p.src + "\n$(echo hi; for)\nEND\necho AFTER\n"
			if carried, out := runSubstPosition(t, src, No, Yes); !carried {
				t.Errorf("with the body's axis No: out = %q, want the script to reach AFTER — "+
					"the give-up costs the command and the word's axis must not be asked here", out)
			}
			if carried, out := runSubstPosition(t, src, Yes, Yes); carried {
				t.Errorf("with the body's axis Yes: out = %q, want the script ended", out)
			}
		})
	}
}

// The pair that holds the **position** fixed and moves the substitution,
// which is what says the noun is the body.
//
// The same pipeline element, the same failing substitution, one place apart:
// in an ordinary word it is the word's axis and the script ends; in a
// here-document body it is the body's and the script carries on. Without this
// the suite above would pass just as well for a rule that had stopped
// recording the stop altogether.
func TestTheSameFailureInAWordOnAPipelineElementStillEndsTheScript(t *testing.T) {
	t.Parallel()
	for _, p := range []struct{ name, src string }{
		{"an ordinary word", "v=$(echo hi; for) | cat\necho AFTER\n"},
		{"a word on a background command", "v=$(echo hi; for) &\nwait\necho AFTER\n"},
		{"a word inside a subshell", "( v=$(echo hi; for) )\necho AFTER\n"},
		{"a redirection target", "cat < \"$(echo hi; for)\" | cat\necho AFTER\n"},
		{"a here-string", "cat <<<\"$(echo hi; for)\" | cat\necho AFTER\n"},
		{"a backquoted body", "v=`echo hi; for` | cat\necho AFTER\n"},
	} {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			// Whichever way the *body's* axis is set, because none of these
			// is a here-document body.
			for _, body := range []Answer{Yes, No} {
				if carried, out := runSubstPosition(t, p.src, body, Yes); carried {
					t.Errorf("with the body's axis %v: out = %q, want the script ended by the word's axis", body, out)
				}
			}
			// And the control that says the word's axis is what ended it,
			// rather than the failure ending everything wherever it is: with
			// that axis No the same script runs on.
			if carried, out := runSubstPosition(t, p.src, No, No); !carried {
				t.Errorf("with the word's axis No: out = %q, want the script to reach AFTER", out)
			}
		})
	}
}

// A **quoted** delimiter is the control the positions need: there is no
// substitution to refuse, so every position runs its command and reaches the
// last line whatever either axis says.
func TestAQuotedDelimiterCarriesOnInEveryPosition(t *testing.T) {
	t.Parallel()
	for _, p := range substBodyPositions {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			src := strings.Replace(p.src, "<<END", "<<'END'", 1) + "\n$(echo hi; for)\nEND\necho AFTER\n"
			for _, body := range []Answer{Yes, No} {
				carried, out := runSubstPosition(t, src, body, Yes)
				if !carried {
					t.Errorf("with the body's axis %v: out = %q, want the script to reach AFTER", body, out)
				}
				if strings.Contains(out, "unexpected") || strings.Contains(out, "syntax") {
					t.Errorf("with the body's axis %v: out = %q, want nothing refused", body, out)
				}
			}
		})
	}
}
