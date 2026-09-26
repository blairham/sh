// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `( … )` is a process of its own, so the word its redirection is aimed at
// is one more place the two halves of redirtarget.go's question can be put —
// and the panel does not answer it the way it answers the same question of a
// command. Semantics.RedirectTargetOnASubshellExpandsInTheSubshell holds the
// measured table; this file is the axis's own behavior.

// The write side: yes puts back what the target's expansion wrote, no leaves
// it.
func TestASubshellsTargetExpandingInTheSubshellConfinesTheWrite(t *testing.T) {
	for _, tc := range []struct {
		answer Answer
		want   string
	}{
		{Yes, "u=UNSET"},
		{No, "u=made"},
	} {
		t.Run(tc.answer.String(), func(t *testing.T) {
			sem := testSemantics()
			sem.RedirectTargetOnASubshellExpandsInTheSubshell = tc.answer
			out, _ := run(t, "unset u\n( : ) > \"${u:=made}\"\nprintf 'u=%s' \"${u-UNSET}\"", withSem(sem))
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// The failure side, which is the same answer's other consequence: an
// expansion that happened inside the parentheses costs the parentheses, and
// one that happened here costs the shell.
func TestASubshellsTargetExpandingInTheSubshellDecidesWhatAFailureCosts(t *testing.T) {
	for _, tc := range []struct {
		answer    Answer
		wantAlive bool
	}{
		{Yes, true},
		{No, false},
	} {
		t.Run(tc.answer.String(), func(t *testing.T) {
			sem := testSemantics()
			sem.RedirectTargetOnASubshellExpandsInTheSubshell = tc.answer
			// Pinned, not left open: under no the word was expanded here and
			// whose failure that is becomes the other axis's question, so a
			// test that left it unanswered would be measuring the refusal.
			sem.RedirectTargetFailureIsTheRedirections = No
			out, _ := run(t, "set -u\n( echo RAN ) > \"$NOPE_R\"\nprintf 'st=%s alive' \"$?\"", withSem(sem))
			if strings.Contains(out, "RAN") {
				t.Errorf("out = %q, want the subshell left unrun", out)
			}
			if alive := strings.Contains(out, "alive"); alive != tc.wantAlive {
				t.Errorf("out = %q, want the script alive afterwards = %v", out, tc.wantAlive)
			}
		})
	}
}

// And under yes the failure reaches no further than the parentheses: `||`
// catches it and the rest of the *line* still runs. The marker is on the
// redirection's own line deliberately — one on a line of its own cannot tell
// a failure that gave up the line from one that gave up the command.
func TestASubshellsFailedTargetReachesNoFurtherThanTheParentheses(t *testing.T) {
	sem := testSemantics()
	sem.RedirectTargetOnASubshellExpandsInTheSubshell = Yes
	t.Run("the rest of the line runs", func(t *testing.T) {
		out, _ := run(t, "set -u\n( echo RAN ) > \"$NOPE_R\" ; printf SAME\nprintf NEXT", withSem(sem))
		if !strings.Contains(out, "SAME") || !strings.Contains(out, "NEXT") {
			t.Errorf("out = %q, want the rest of the line and the next line both run", out)
		}
	})
	t.Run("|| catches it", func(t *testing.T) {
		out, _ := run(t, "set -u\n( echo RAN ) > \"$NOPE_R\" || printf CAUGHT", withSem(sem))
		if !strings.Contains(out, "CAUGHT") {
			t.Errorf("out = %q, want the failure caught", out)
		}
	})
}

// **The noun is the owner of the redirection, not the command kind.** The two
// axes are held apart here on purpose: each half below sets one to yes and
// the other to no, and the write follows the axis whose *owner* opened the
// redirection. A single field cannot produce this table, which is the whole
// reason there are two.
func TestTheSubshellsTargetAndACommandsAreSeparateAxes(t *testing.T) {
	const src = "unset u\n%s > \"${u:=made}\"\nprintf 'u=%%s' \"${u-UNSET}\""
	for _, tc := range []struct {
		name            string
		subshell, cmd   Answer
		wantSub, wantEx string
	}{
		{"the subshell confines and the command does not", Yes, No, "u=UNSET", "u=made"},
		{"the command confines and the subshell does not", No, Yes, "u=made", "u=UNSET"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := testSemantics()
			sem.RedirectTargetOnASubshellExpandsInTheSubshell = tc.subshell
			sem.RedirectTargetExpandsInTheCommandsProcess = tc.cmd
			if out, _ := run(t, fmt.Sprintf(src, "( : )"), withSem(sem)); out != tc.wantSub {
				t.Errorf("subshell: out = %q, want %q", out, tc.wantSub)
			}
			if out, _ := run(t, fmt.Sprintf(src, "cat /dev/null"), withSem(sem)); out != tc.wantEx {
				t.Errorf("a command of its own: out = %q, want %q", out, tc.wantEx)
			}
		})
	}
}

// A group is the control, and it is what says the answer is keyed on the
// parentheses rather than on "a compound command": the subshell axis is yes
// throughout and the group keeps its write, because this shell runs a group
// itself and there is nowhere else for the write to have gone.
func TestTheSubshellAxisLeavesAGroupAlone(t *testing.T) {
	sem := testSemantics()
	sem.RedirectTargetOnASubshellExpandsInTheSubshell = Yes
	for _, tc := range []struct{ name, cmd string }{
		{"a group", "{ : ; }"},
		{"a loop", "for i in 1; do : ; done"},
		{"a builtin", ":"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "unset u\n" + tc.cmd + " > \"${u:=made}\"\nprintf 'u=%s' \"${u-UNSET}\""
			if out, _ := run(t, src, withSem(sem)); out != "u=made" {
				t.Errorf("out = %q, want the write kept", out)
			}
		})
	}
}

// The parentheses reach wherever they are written, which is what a rule keyed
// on the *statement* rather than on the construct would lose.
func TestTheSubshellAxisReachesANestedAndACalledSubshell(t *testing.T) {
	sem := testSemantics()
	sem.RedirectTargetOnASubshellExpandsInTheSubshell = Yes
	for _, tc := range []struct{ name, src string }{
		{"inside a function", "set -u\ng() { ( echo RAN ) > \"$NOPE_R\"; printf INFUNC; }\ng\nprintf ' alive'"},
		{"inside another subshell", "set -u\n( ( echo RAN ) > \"$NOPE_R\" )\nprintf 'alive'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, withSem(sem))
			if strings.Contains(out, "RAN") {
				t.Errorf("out = %q, want the subshell left unrun", out)
			}
			if !strings.Contains(out, "alive") {
				t.Errorf("out = %q, want the script alive afterwards", out)
			}
		})
	}
}

// A subshell aimed at a plain name asks nothing, so an unanswered dialect can
// still run one — the rule every axis in this seam is asked by.
func TestAQuietSubshellTargetAsksNothing(t *testing.T) {
	sem := testSemantics()
	sem.RedirectTargetOnASubshellExpandsInTheSubshell = Unspecified
	out, st := run(t, `v=f; ( echo hi ) > "$v"; cat f`, withSem(sem))
	if out != "hi\n" || st != 0 {
		t.Errorf("out = %q status = %d, want the file written and read back", out, st)
	}
}

// And one that writes refuses rather than guessing.
func TestAnUnansweredSubshellTargetThatWritesIsRefused(t *testing.T) {
	sem := testSemantics()
	sem.RedirectTargetOnASubshellExpandsInTheSubshell = Unspecified
	out, _ := run(t, "unset u\n( printf CAME-HERE ) > \"${u:=made}\"", withSem(sem))
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("out = %q, want the refusal named", out)
	}
	if strings.Contains(out, "CAME-HERE") {
		t.Errorf("out = %q, want the subshell never to have run", out)
	}
}
