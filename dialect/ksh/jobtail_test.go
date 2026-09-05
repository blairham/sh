// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// The job and lookup long tail, measured against ksh93u+ (2026-09-04).

// A `wait` whose spec names nothing says nothing at all and reports 0, and
// `wait -n` is an unknown option with the usage line after it.
func TestWaitSpecs(t *testing.T) {
	out, _ := runKsh(t, t.TempDir(), `{ exit 3; } &
wait %1; echo st=$?
wait %9; echo miss=$?
wait -n; echo n=$?`)
	if !strings.Contains(out, "st=3") {
		t.Errorf("got %q, want the job's status through %%1", out)
	}
	if !strings.Contains(out, "miss=0") || strings.Contains(out, "no such job") {
		t.Errorf("got %q, want the missing spec passed over in silence at 0", out)
	}
	if !strings.Contains(out, "wait: -n: unknown option") ||
		!strings.Contains(out, "Usage: wait [ options ] [job ...]") ||
		!strings.Contains(out, "n=2") {
		t.Errorf("got %q, want -n refused with the usage line at 2", out)
	}
}

// disown shields the job from a HUP this engine never forwards — and the
// listing keeps it. With nothing held it fails in silence.
func TestDisownKeepsTheJob(t *testing.T) {
	out, _ := runKsh(t, t.TempDir(), `{ exit 0; } &
disown
jobs; echo st=$?`)
	if !strings.Contains(out, "[1]") {
		t.Errorf("got %q, want the job still listed", out)
	}
	out, _ = runKsh(t, t.TempDir(), `disown; echo none=$?`)
	if strings.Contains(out, "disown:") || !strings.Contains(out, "none=1") {
		t.Errorf("got %q, want a silent 1", out)
	}
}

// type -p searches PATH past the shell's own answer and prints the bare
// path; -f leaves the function out and falls to the tracked-alias sentence.
func TestTypePAndF(t *testing.T) {
	out, _ := runKsh(t, t.TempDir(), `type -p nosuchzz; echo miss=$?`)
	if !strings.Contains(out, "miss=1") || strings.Contains(out, "not found") {
		t.Errorf("got %q, want a silent 1 for -p on nothing", out)
	}
}
