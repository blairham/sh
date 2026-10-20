// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CDPATH is the whole of how a relative operand is resolved here: a search
// that misses is a failure rather than falling back to the operand as it
// stands (#2896). Measured 2026-09-18 against ksh93u+ 2012-08-01 from a
// script file under `env -i`, with `elsewhere` and `target` side by side.
func TestCdpathMissRefusesTheOperand(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"elsewhere/hit", "target"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	out, st := runKsh(t, dir, "CDPATH="+dir+"/elsewhere\ncd target\necho \"st=$?\"")
	if st != 0 || !strings.Contains(out, "cd: target: ") || !strings.Contains(out, "st=1") {
		t.Errorf("out %q status %d, want the ordinary refusal at 1", out, st)
	}
	// The search still works, and an empty CDPATH is no search at all — so
	// an ordinary relative `cd` is untouched.
	out, st = runKsh(t, dir, "CDPATH="+dir+"/elsewhere\ncd hit\necho \"st=$?\"")
	if st != 0 || !strings.Contains(out, "st=0") {
		t.Errorf("a hit: out %q status %d, want it to arrive", out, st)
	}
	out, st = runKsh(t, dir, "CDPATH=\ncd target\necho \"st=$?\"")
	if st != 0 || !strings.Contains(out, "st=0") {
		t.Errorf("an empty CDPATH: out %q status %d, want it to arrive", out, st)
	}
}

// `disown` answers 1 for every call and says nothing here, found or not
// (#3187). Six spellings, six silent 1s, measured 2026-09-18 against ksh93u+
// 2012-08-01 from a script file under `env -i` — and the job it can list is
// still listed afterwards, which is what says the status is not the lookup.
func TestDisownAnswersOneInSilence(t *testing.T) {
	for _, src := range []string{
		"true & disown",
		"true & disown %1",
		"true & p=$!; disown $p",
		"disown",
		"disown %9",
	} {
		out, st := runKsh(t, t.TempDir(), src+"\necho \"st=$?\"")
		if st != 0 || out != "st=1\n" {
			t.Errorf("%s: out %q status %d, want a silent 1", src, out, st)
		}
	}
	// A bad option is refused before the axis is reached, which is that
	// shell's own answer: `disown: -a: unknown option` and the usage at 2.
	out, st := runKsh(t, t.TempDir(), "disown -a\necho \"st=$?\"")
	if st != 0 || !strings.Contains(out, "-a") || !strings.Contains(out, "st=2") {
		t.Errorf("a bad option: out %q status %d, want the letter named at 2", out, st)
	}
}
