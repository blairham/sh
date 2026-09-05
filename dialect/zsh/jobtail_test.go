// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// The job and lookup long tail, measured against zsh 5.9.2 (2026-09-04).

// A second `%name` match is taken — the most recent — rather than refused,
// and a missing spec is worded at 127. `wait -n` is a job named -n here.
func TestWaitSpecs(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `{ exit 3; } &
{ exit 4; } &
wait %{; echo st=$?
wait %9; echo miss=$?
wait -n; echo n=$?`)
	if !strings.Contains(out, "st=4") {
		t.Errorf("got %q, want the most recent match taken", out)
	}
	if !strings.Contains(out, ": %9: no such job") || !strings.Contains(out, "miss=127") {
		t.Errorf("got %q, want the missing spec worded at 127", out)
	}
	if !strings.Contains(out, ": job not found: -n") || !strings.Contains(out, "n=127") {
		t.Errorf("got %q, want -n read as a job", out)
	}
}

// disown takes the job out of the table; bare with nothing held it says so.
func TestDisown(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `{ exit 0; } &
disown
jobs; echo st=$?
disown; echo none=$?`)
	if strings.Contains(out, "[1]") {
		t.Errorf("got %q, want the job gone from the listing", out)
	}
	if !strings.Contains(out, ": no current job") || !strings.Contains(out, "none=1") {
		t.Errorf("got %q, want the bare disown worded at 1", out)
	}
}

// type -p answers in sentences here — the hit and the miss alike.
func TestTypePIsASentence(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `type -p nosuchzz; echo st=$?`)
	if !strings.Contains(out, "nosuchzz not found") || !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want the sentence for a miss", out)
	}
}

// The directory stack moves in silence here: only dirs prints.
func TestDirectoryStackIsSilent(t *testing.T) {
	home := t.TempDir()
	out, _ := runZsh(t, home, zsh.Prelude()+`
HOME=`+home+`
cd
pushd /tmp; echo st=$?
dirs
popd; echo p=$?
popd; echo p2=$?`)
	if !strings.Contains(out, "st=0\n/tmp ~\n") {
		t.Errorf("got %q, want a silent pushd and the stack from dirs", out)
	}
	if !strings.Contains(out, "p=0\n") || !strings.Contains(out, "popd: directory stack empty\np2=1\n") {
		t.Errorf("got %q, want a silent popd and the empty-stack refusal", out)
	}
}

// `jobs -p` is the one place the letter splits: zsh reads it as the job's
// process *group* and prints its ordinary rows, where the other three print
// the ids and nothing else.
func TestJobsDashPIsStillAListing(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `/bin/sleep 0.3 & echo "bang=$!"
jobs -p
wait`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %q, want two lines", out)
	}
	bang := strings.TrimPrefix(lines[0], "bang=")
	if !strings.HasPrefix(lines[1], "[1]") || !strings.Contains(lines[1], bang) {
		t.Errorf("got %q, want a numbered row carrying the process id", out)
	}

	out, _ = runZsh(t, t.TempDir(), `jobs -n; echo n=$?
jobs -d; echo d=$?`)
	if !strings.Contains(out, "bad option: -n") || !strings.Contains(out, "n=1") {
		t.Errorf("got %q, want the letter zsh does not have refused at 1", out)
	}
	if !strings.Contains(out, "-d is not implemented yet") {
		t.Errorf("got %q, want the letter zsh does have named as missing", out)
	}
}

// Both state filters at once list a job in either state here, where bash
// lets the last letter given decide.
func TestJobsStateFiltersAddUp(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `/bin/sleep 0.3 & jobs -rs; wait`)
	if !strings.Contains(out, "[1]") {
		t.Errorf("got %q, want the running job listed through -rs", out)
	}
}
