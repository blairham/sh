// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `( … )` is a process of its own, so a here-document body fed to it is one
// more place heredocprocess.go's question can be put — and the panel does not
// answer it the way it answers the same question of a command.
// Semantics.HeredocBodyOnASubshellExpandsInTheSubshell holds the measured
// table; this file is the axis's own behavior.

// The write side: yes puts back what the body's expansion wrote, no leaves it.
func TestASubshellsHeredocBodyExpandingInTheSubshellConfinesTheWrite(t *testing.T) {
	for _, tc := range []struct {
		answer Answer
		want   string
	}{
		{Yes, "5\nn=0"},
		{No, "5\nn=5"},
	} {
		t.Run(tc.answer.String(), func(t *testing.T) {
			sem := testSemantics()
			sem.HeredocBodyOnASubshellExpandsInTheSubshell = tc.answer
			out, _ := run(t, "n=0\n( cat ) <<END\n$(( n+=5 ))\nEND\nprintf 'n=%s' \"$n\"", withSem(sem))
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// **The failure side is unanimous, and this axis is deliberately not asked
// there.** A body that will not expand, on a `( … )`, costs the parentheses
// and nothing more in all five columns — measured 2026-09-26 with `$(( 1/0 ))`
// and `; printf SAME` on the redirection's own line: `SAME` is written and
// `|| echo CAUGHT` fires in bash, zsh, ksh93, dash and BusyBox ash, whichever
// side of this axis the column is on. So the answer does not reach the
// give-up, and the test says so by running both.
//
// The markers are on the redirection's **own line** because a marker on the
// line after the delimiter is printed by every reading there is — it cannot
// tell a give-up of the command from a give-up of the line, which is the
// degradation #4687 found in a committed test.
func TestASubshellsFailedHeredocBodyCostsTheParenthesesUnderEitherAnswer(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		t.Run(answer.String(), func(t *testing.T) {
			sem := testSemantics()
			sem.HeredocBodyOnASubshellExpandsInTheSubshell = answer
			// Both pinned at the reading that would give up the *line* if
			// this boundary let it, so a pass here is the boundary holding
			// rather than a permissive vector agreeing by accident.
			sem.HeredocBodyFailureIsTheRedirections = No
			sem.FailedExpansionAbandonsTheLine = Yes
			out, _ := run(t, "( echo RAN ) <<END ; printf SAME\n$(( 1/0 ))\nEND\nprintf NEXT", withSem(sem))
			if strings.Contains(out, "RAN") {
				t.Errorf("out = %q, want the subshell left unrun", out)
			}
			if !strings.Contains(out, "SAME") || !strings.Contains(out, "NEXT") {
				t.Errorf("out = %q, want the rest of the line and the next line both run", out)
			}
			caught, _ := run(t, "( echo RAN ) <<END || printf CAUGHT\n$(( 1/0 ))\nEND", withSem(sem))
			if !strings.Contains(caught, "CAUGHT") {
				t.Errorf("out = %q, want the failure caught", caught)
			}
		})
	}
}

// The control that keeps the test above from reading as "this boundary lets
// everything through": the same body on a **group** is the other file's
// question and does move, because there the shell ran the command itself and
// HeredocBodyFailureIsTheRedirections decides. If this row stopped moving,
// the row above would be measuring a boundary that swallows every failure.
func TestTheSameFailingBodyOnAGroupStillFollowsTheOtherAxis(t *testing.T) {
	const src = "{ echo RAN; } <<END ; printf SAME\n$(( 1/0 ))\nEND\nprintf NEXT"
	for _, tc := range []struct {
		redirs       Answer
		wantSameLine bool
	}{
		{Yes, true},
		{No, false},
	} {
		t.Run(tc.redirs.String(), func(t *testing.T) {
			sem := testSemantics()
			sem.HeredocBodyFailureIsTheRedirections = tc.redirs
			sem.FailedExpansionAbandonsTheLine = Yes
			out, _ := run(t, src, withSem(sem))
			if same := strings.Contains(out, "SAME"); same != tc.wantSameLine {
				t.Errorf("out = %q, want the rest of the line run = %v", out, tc.wantSameLine)
			}
		})
	}
}

// **The noun is the owner of the redirection, not the command kind.** The two
// axes are held apart here on purpose: each half below sets one to yes and
// the other to no, and the write follows the axis whose *owner* opened the
// redirection. A single field cannot produce this table, which is the whole
// reason there are two — and it is the panel's own shape, since ksh93 confines
// a command's body and keeps a subshell's while dash and BusyBox ash do the
// reverse.
func TestTheSubshellsHeredocBodyAndACommandsAreSeparateAxes(t *testing.T) {
	const src = "n=0\n%s <<END\n$(( n+=5 ))\nEND\nprintf 'n=%%s' \"$n\""
	for _, tc := range []struct {
		name            string
		subshell, cmd   Answer
		wantSub, wantEx string
	}{
		{"the subshell confines and the command does not", Yes, No, "5\nn=0", "5\nn=5"},
		{"the command confines and the subshell does not", No, Yes, "5\nn=5", "5\nn=0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.HeredocBodyOnASubshellExpandsInTheSubshell = tc.subshell
			sem.HeredocExpandsInTheCommandsProcess = tc.cmd
			if out, _ := run(t, fmt.Sprintf(src, "( cat )"), withSem(sem)); out != tc.wantSub {
				t.Errorf("subshell: out = %q, want %q", out, tc.wantSub)
			}
			if out, _ := run(t, fmt.Sprintf(src, "cat"), withSem(sem)); out != tc.wantEx {
				t.Errorf("a command of its own: out = %q, want %q", out, tc.wantEx)
			}
		})
	}
}

// A group is the control, and it is what says the answer is keyed on the
// parentheses rather than on "a compound command": the subshell axis is yes
// throughout and every one of these keeps its write, because this shell runs
// them itself and there is nowhere else for the write to have gone.
func TestTheSubshellHeredocAxisLeavesAGroupAlone(t *testing.T) {
	sem := testSemantics()
	sem.HeredocBodyOnASubshellExpandsInTheSubshell = Yes
	for _, tc := range []struct{ name, cmd string }{
		{"a group", "{ cat; }"},
		{"a loop", "while read -r x; do printf '%s\\n' \"$x\"; done"},
		{"a builtin", "read -r x"},
		{"a function", "f"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "f() { cat; }\nn=0\n" + tc.cmd + " <<END\n$(( n+=5 ))\nEND\nprintf 'n=%s' \"$n\""
			out, _ := run(t, src, withSem(sem))
			if !strings.HasSuffix(out, "n=5") {
				t.Errorf("out = %q, want the write kept", out)
			}
		})
	}
}

// What is *inside* the parentheses is not the noun either. Held fixed against
// five bodies on the same day, every column answers alike — so the axis has to
// reach all five here too.
func TestTheSubshellHeredocAxisDoesNotCareWhatTheParenthesesHold(t *testing.T) {
	sem := testSemantics()
	sem.HeredocBodyOnASubshellExpandsInTheSubshell = Yes
	for _, cmd := range []string{"( cat )", "( : ; cat )", "( v=1; cat )", "( cat; cat )", "( ( cat ) )"} {
		t.Run(cmd, func(t *testing.T) {
			src := "n=0\n" + cmd + " <<END\n$(( n+=5 ))\nEND\nprintf 'n=%s' \"$n\""
			if out, _ := run(t, src, withSem(sem)); !strings.HasSuffix(out, "n=0") {
				t.Errorf("out = %q, want the write confined", out)
			}
		})
	}
}

// The parentheses reach wherever they are written, which is what a rule keyed
// on the *statement* rather than on the construct would lose.
func TestTheSubshellHeredocAxisReachesANestedAndACalledSubshell(t *testing.T) {
	sem := testSemantics()
	sem.HeredocBodyOnASubshellExpandsInTheSubshell = Yes
	for _, tc := range []struct{ name, src string }{
		{"inside a function", "n=0\ng() { ( cat ) <<END\n$(( n+=5 ))\nEND\n}\ng\nprintf 'n=%s' \"$n\""},
		{"inside another subshell", "n=0\n( ( cat ) <<END\n$(( n+=5 ))\nEND\nprintf 'inner=%s ' \"$n\" )\nprintf 'n=%s' \"$n\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := run(t, tc.src, withSem(sem)); !strings.HasSuffix(out, "n=0") {
				t.Errorf("out = %q, want the write confined", out)
			}
		})
	}
}

// The body is expanded either way — exactly once, and even where the
// parentheses hold something that never reads a byte of it. Only *where the
// write lands* is the axis's, which is the half a probe reading the body's
// own text cannot see. The tick is written to **stderr** from inside the
// substitution and counted as a whole line, so the body's own text reaching
// the command cannot be mistaken for it.
func TestASubshellsHeredocBodyIsExpandedUnderEitherAnswer(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		for _, cmd := range []string{"( cat >/dev/null )", "( : )"} {
			t.Run(answer.String()+" "+cmd, func(t *testing.T) {
				sem := testSemantics()
				sem.HeredocBodyOnASubshellExpandsInTheSubshell = answer
				src := cmd + " <<END\n$(echo TICK >&2; echo z)\nEND\n"
				out, _ := run(t, src, withSem(sem))
				if n := strings.Count(out, "TICK\n"); n != 1 {
					t.Errorf("out = %q, want exactly one TICK line, got %d", out, n)
				}
			})
		}
	}
}

// And a redirection the shell never reaches expands nothing, under either
// answer — the control that says the count above is about the expansion
// happening rather than about the axis.
func TestASubshellHeredocTheShellNeverReachesExpandsNothing(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		t.Run(answer.String(), func(t *testing.T) {
			sem := testSemantics()
			sem.HeredocBodyOnASubshellExpandsInTheSubshell = answer
			out, _ := run(t, "false && ( cat ) <<END\n$(echo TICK >&2; echo z)\nEND\nprintf done", withSem(sem))
			if strings.Contains(out, "TICK") {
				t.Errorf("out = %q, want nothing expanded", out)
			}
		})
	}
}

// A quoted delimiter is not expanded at all, so there is no write to place and
// the axis is never reached — the control that must not move.
func TestAQuotedDelimiterOnASubshellAsksNothing(t *testing.T) {
	sem := testSemantics()
	sem.HeredocBodyOnASubshellExpandsInTheSubshell = Unspecified
	out, st := run(t, "n=0\n( cat ) <<'END'\n$(( n+=5 ))\nEND\nprintf 'n=%s' \"$n\"", withSem(sem))
	if out != "$(( n+=5 ))\nn=0" || st != 0 {
		t.Errorf("out = %q status = %d, want the body literal and no write", out, st)
	}
}

// A subshell fed an ordinary here-document asks nothing either, so an
// unanswered dialect can still run one — the rule every axis in this seam is
// asked by.
func TestAQuietSubshellHeredocAsksNothing(t *testing.T) {
	sem := testSemantics()
	sem.HeredocBodyOnASubshellExpandsInTheSubshell = Unspecified
	out, st := run(t, "v=hi\n( cat ) <<END\n[$v]\nEND", withSem(sem))
	if out != "[hi]\n" || st != 0 {
		t.Errorf("out = %q status = %d, want the body through and 0", out, st)
	}
}

// And one that writes refuses rather than guessing.
func TestAnUnansweredSubshellHeredocThatWritesIsRefused(t *testing.T) {
	sem := testSemantics()
	sem.HeredocBodyOnASubshellExpandsInTheSubshell = Unspecified
	out, st := run(t, "n=0\n( printf CAME-HERE ) <<END\n$(( n+=5 ))\nEND", withSem(sem))
	if st != 2 {
		t.Errorf("status = %d, want 2 — an unanswered axis is a refusal", st)
	}
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("out = %q, want the refusal named", out)
	}
	if strings.Contains(out, "CAME-HERE") {
		t.Errorf("out = %q, want the subshell never to have run", out)
	}
}
