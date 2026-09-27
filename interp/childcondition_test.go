// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Which children raise the `CHLD` condition — see
// Semantics.ChildConditionCountsEveryReapedChild, which carries the panel.
//
// The other half of #4756's table and a different noun: that one is about a
// signal a *script* sent, this one about a real child. What moves is whether
// the child was a **job**, and the background row is the control that says so
// — the same child, the same handler and the same `wait`, running the handler
// in every column.
func childConditionRun(t *testing.T, a Answer, body string) string {
	t.Helper()
	src := "n=0\ntrap 'n=$((n+1))' CHLD\n" + body + "\n:\n:\nprintf '%s' \"$n\"\n"
	out, _ := run(t, src, func(r *Runner) {
		s := *r.Semantics
		s.ChildConditionCountsEveryReapedChild = a
		r.Semantics = &s
	})
	return strings.TrimSpace(out)
}

// The four foreground shapes are the breadth that says the rule is keyed on
// the job and not on the external command: a pipeline element, a substitution
// and a subshell each reap a child that is not one, and a column that counts a
// job alone counts none of them.
func TestWhichReapedChildrenRaiseTheConditionIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name       string
		body       string
		every, job string
	}{
		{"a foreground external command", "/bin/echo x >/dev/null", "1", "0"},
		{"a pipeline element", "/bin/echo x | /bin/cat >/dev/null", "2", "0"},
		{"a substitution", "v=$(/bin/echo x)", "1", "0"},
		{"a subshell", "( /bin/echo x >/dev/null )", "1", "0"},
		{"a background job", "/bin/echo x >/dev/null &\nwait", "1", "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := childConditionRun(t, Yes, tc.body); got != tc.every {
				t.Errorf("every reaped child: counted %q, want %q", got, tc.every)
			}
			if got := childConditionRun(t, No, tc.body); got != tc.job {
				t.Errorf("a job alone: counted %q, want %q", got, tc.job)
			}
		})
	}
}

// The row the axis rests on, stated on its own because it is what a mutant
// that simply stops raising the condition would fail: under either answer a
// background job runs the handler, so "a job alone" is a narrowing of which
// children count and never a silencing of the trap.
func TestABackgroundJobRaisesTheConditionUnderEitherAnswer(t *testing.T) {
	for _, a := range []Answer{Yes, No} {
		if got := childConditionRun(t, a, "/bin/echo x >/dev/null &\nwait"); got != "1" {
			t.Errorf("%v: counted %q, want 1", a, got)
		}
	}
}

// And the mutant on the other side: a reading that counted a job alone in
// every dialect would pass every row above except this one, which is the
// foreground shape read under the answer three columns hold.
func TestEveryReapedChildCountsWhereTheAxisSaysSo(t *testing.T) {
	if got := childConditionRun(t, Yes, "/bin/echo x >/dev/null\n/bin/echo y >/dev/null"); got != "2" {
		t.Errorf("counted %q, want 2 — one per child the shell reaped", got)
	}
	if got := childConditionRun(t, No, "/bin/echo x >/dev/null\n/bin/echo y >/dev/null"); got != "0" {
		t.Errorf("counted %q, want 0 — neither child is a job", got)
	}
}

// An axis unanswered is a refusal by name rather than a quiet reading of
// either side.
func TestWhichReapedChildrenCountUnansweredIsRefusedByName(t *testing.T) {
	src := "trap ':' CHLD\n/bin/echo x >/dev/null\n"
	out, _ := run(t, src, func(r *Runner) {
		s := *r.Semantics
		s.ChildConditionCountsEveryReapedChild = Unspecified
		r.Semantics = &s
	})
	if !strings.Contains(out, "raise the `CHLD` condition") {
		t.Errorf("got %q, want the axis named", out)
	}
}

// And the control that keeps the axis off the path of every external command:
// a script that never trapped the condition is not refused for an axis it did
// not ask about.
func TestAnUntrappedConditionNeverAsksTheAxis(t *testing.T) {
	src := "/bin/echo x >/dev/null\necho a\n"
	out, status := run(t, src, func(r *Runner) {
		s := *r.Semantics
		s.ChildConditionCountsEveryReapedChild = Unspecified
		r.Semantics = &s
	})
	if got := strings.TrimSpace(out); got != "a" {
		t.Errorf("got %q, want %q", got, "a")
	}
	if status != 0 {
		t.Errorf("status = %d, want 0", status)
	}
}
