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
