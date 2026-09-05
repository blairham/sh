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

// `jobs` takes POSIX's two letters here and nothing else: `-p` is the
// process ids alone, and the letters bash added are illegal options.
func TestJobsOptionLetters(t *testing.T) {
	out, _ := runDash(t, t.TempDir(), `/bin/sleep 0.3 & echo "bang=$!"
jobs -p
wait`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || lines[1] != strings.TrimPrefix(lines[0], "bang=") {
		t.Errorf("got %q, want the job's process id and nothing else", out)
	}

	out, _ = runDash(t, t.TempDir(), `jobs -r; echo r=$?
jobs -n; echo n=$?`)
	if !strings.Contains(out, "jobs: Illegal option -r") || !strings.Contains(out, "r=2") {
		t.Errorf("got %q, want -r refused as illegal at 2", out)
	}
	if !strings.Contains(out, "jobs: Illegal option -n") || !strings.Contains(out, "n=2") {
		t.Errorf("got %q, want -n refused as illegal at 2", out)
	}
}
