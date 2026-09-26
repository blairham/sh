// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"testing"
)

// The noun, stated as a test: **the pid**, and this body's own.
//
// Four numbers are put to the same runner and exactly one of them is this
// shell. A rule keyed on "am I a subshell" answers yes to all four; one keyed
// on `os.Getpid()` answers yes to the wrong one; the group this body leads is
// the only reading that separates them. See selfkillbody.go for the
// measurements behind that sentence.
func TestOnlyTheGroupThisBodyLeadsIsThisBody(t *testing.T) {
	const mine, outer = 4242, 4243
	r := newTestRunner(t, &Runner{inSubshell: true, bodyAnchor: &procAnchor{
		tried: true, pgid: mine,
		// The body this one is nested in. A real shell forked again for the
		// inner body, so the outer body's number is somebody else's process
		// however contained this one is — `existing` walks outward and
		// identity must not.
		outer: &procAnchor{tried: true, pgid: outer},
	}})
	for _, c := range []struct {
		name string
		pid  int
		want bool
	}{
		{"the group this body leads", mine, true},
		{"the group the body around it leads", outer, false},
		{"this process, which is the shell at the top", os.Getpid(), false},
		{"somebody else's process", mine + 1, false},
		{"a process group, written negative", -mine, false},
		{"zero, which means this shell's process group", 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := r.aimedAtThisBody(c.pid); got != c.want {
				t.Errorf("aimedAtThisBody(%d) = %v, want %v", c.pid, got, c.want)
			}
		})
	}
}

// A body that has never been asked which process it is has no group, and
// asking must not start one.
//
// This is what keeps the check affordable: a script full of subshells that
// never mention `$BASHPID` or `$sysparams[pid]` would otherwise fork a
// placeholder process per `kill` in order to learn that the answer is no. It
// is also sound — the parameter is the only way the number reaches a script,
// so a body that was never given one cannot have named it.
func TestABodyWithNoGroupIsNeverTheTargetAndStartsNothing(t *testing.T) {
	unstarted := &procAnchor{argv: []string{"/nonexistent/anchor"}}
	r := newTestRunner(t, &Runner{inSubshell: true, bodyAnchor: unstarted})
	if r.aimedAtThisBody(4242) {
		t.Error("a body with no group claimed a pid as its own")
	}
	if unstarted.tried {
		t.Error("the check started the anchor; a `kill` in a subshell must not fork a process")
	}
	// And the shell at the top, which has no anchor at all.
	//
	// testrunner:bare — the unset field *is* the subject. A shell with
	// nothing set is what a top-level one looks like to this rule, and it
	// runs no command, so a directory of its own would say nothing.
	top := &Runner{}
	if top.aimedAtThisBody(os.Getpid()) {
		t.Error("the top-level shell claimed its own pid through the body rule")
	}
}

// The arrival goes on the body's own list and not the process's, which is what
// keeps the parent from running its handler for a signal the process never
// had. Same argument as brokenPipeAbsorbed, same list.
func TestABodysOwnSignalIsRecordedOnItsOwnList(t *testing.T) {
	r := newTestRunner(t, &Runner{inSubshell: true, traps: map[string]string{"TERM": "echo T"}})
	r.selfSignaledBody("TERM")
	if len(r.selfPending) != 1 || r.selfPending[0] != "TERM" {
		t.Errorf("selfPending = %v, want [TERM]", r.selfPending)
	}
	s := r.sigs()
	s.mu.Lock()
	shared := append([]string(nil), s.pending...)
	s.mu.Unlock()
	if len(shared) != 0 {
		t.Errorf("the shared list holds %v, want nothing: the process never had this signal", shared)
	}
}
