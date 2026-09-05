// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// The job long tail, measured against dash (2026-09-04): only numbers and
// the current-job specs resolve, `wait -n` is an illegal option, and there
// is no disown at all.

func TestWaitSpecs(t *testing.T) {
	out, _ := runDash(t, t.TempDir(), `{ exit 3; } &
wait %1; echo st=$?
wait %sleep; echo name=$?
wait -n; echo n=$?`)
	if !strings.Contains(out, "st=3") {
		t.Errorf("got %q, want the job's status through %%1", out)
	}
	if !strings.Contains(out, "wait: No such job: %sleep") || !strings.Contains(out, "name=2") {
		t.Errorf("got %q, want a %%name refused as a missing job at 2", out)
	}
	if !strings.Contains(out, "wait: Illegal option -n") || !strings.Contains(out, "n=2") {
		t.Errorf("got %q, want -n refused as an option at 2", out)
	}
}

func TestKillMissingJob(t *testing.T) {
	out, _ := runDash(t, t.TempDir(), `kill %9; echo st=$?`)
	if !strings.Contains(out, "kill: No such job: %9") || !strings.Contains(out, "st=2") {
		t.Errorf("got %q, want the missing job worded at 2", out)
	}
}

func TestDisownIsNotABuiltin(t *testing.T) {
	out, _ := runDash(t, t.TempDir(), `disown; echo st=$?`)
	if !strings.Contains(out, "disown: not found") || !strings.Contains(out, "st=127") {
		t.Errorf("got %q, want the name resolved like any missing command", out)
	}
}
