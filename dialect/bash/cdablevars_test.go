// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `cdable_vars` makes a `cd` operand that named no directory a *variable*
// holding one. It is the last resort: the ordinary relative lookup and CDPATH
// both win.
//
// Measured 2026-09-23 on bash 5.3.15, and every row in its **own fresh
// directory** — on the first pass they shared one, a `mkdir` from an earlier
// row answered three later ones, and those three read as the option working
// when they had found a real directory instead.
//
// It sat in shoptStates refusing the write, and the refusal was the quiet
// kind: a script that sets this has written `cd d` meaning the variable, and
// the refused shell read the word as a directory name and said it was not
// there (#4149).

// cdvRun runs src in a fresh directory holding `target/` and a file, and
// gives back everything it wrote.
func cdvRun(t *testing.T, src string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "afile"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	out, st, err := preset.Combined(t, dialecttest.Base{
		Name: "sh", Dir: dir, Env: []string{"PATH=/usr/bin:/bin"},
		Stdout: &buf, Stderr: &buf,
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// TestABareCdOperandCanNameAVariableHere is the pair that makes it a feature
// rather than a coincidence: the same script with the option off must still
// fail, or the test would pass against a shell that resolved every operand
// through the variables.
func TestABareCdOperandCanNameAVariableHere(t *testing.T) {
	out, st := cdvRun(t, `shopt -s cdable_vars; d=target; cd d; echo "pwd=${PWD##*/}"`)
	if st != 0 || !strings.Contains(out, "pwd=target") {
		t.Errorf("with the option on: %q at %d, want a move into target", out, st)
	}
	// The announcement is the value **as written**, which is where this
	// differs from CDPATH's — that one prints the directory it arrived at.
	if !strings.HasPrefix(out, "target\n") {
		t.Errorf("want the value announced as written, got %q", out)
	}

	// The script's own status is the `echo`'s, so the `cd`'s is read from
	// the line it printed rather than from the run.
	out, _ = cdvRun(t, `d=target; cd d; echo "st=$?"`)
	if !strings.Contains(out, "st=1") || !strings.Contains(out, "No such file or directory") {
		t.Errorf("with the option off `cd d` should fail: %q", out)
	}
}

// TestTheVariableIsTheLastResortHere: a real directory of that name wins, and
// so does CDPATH. Both rows are what say this is a fallback rather than a
// lookup that runs first — an implementation that substituted before trying
// would move somewhere else in each.
func TestTheVariableIsTheLastResortHere(t *testing.T) {
	out, st := cdvRun(t, `shopt -s cdable_vars; mkdir -p d; d=target; cd d; echo "pwd=${PWD##*/}"`)
	if st != 0 || !strings.Contains(out, "pwd=d") {
		t.Errorf("a real directory should win: %q at %d", out, st)
	}
	if strings.Contains(out, "target\n") {
		t.Errorf("nothing should be announced when the directory won: %q", out)
	}

	out, st = cdvRun(t, `shopt -s cdable_vars; mkdir -p pool/d; CDPATH=$PWD/pool; d=target; cd d; echo "pwd=${PWD##*/}"`)
	if st != 0 || !strings.Contains(out, "pwd=d") {
		t.Errorf("CDPATH should win: %q at %d", out, st)
	}
}

// TestAFailedVariableCdNamesTheOperandHere: the value is used and the
// *operand* is reported, so a value that is not a directory is `cd: d: …` and
// never `cd: /some/path: …`. And nothing is announced, because the
// announcement comes after the move — printing it at the substitution wrote a
// line in front of the failure that bash does not write.
func TestAFailedVariableCdNamesTheOperandHere(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`shopt -s cdable_vars; d=$PWD/afile; cd d`, "cd: d: Not a directory"},
		{`shopt -s cdable_vars; d=/nosuch; cd d`, "cd: d: No such file or directory"},
		{`shopt -s cdable_vars; unset d; cd d`, "cd: d: No such file or directory"},
	} {
		out, st := cdvRun(t, tc.src)
		if st == 0 {
			t.Errorf("%s: status 0, want a failure: %q", tc.src, out)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: want %q in %q", tc.src, tc.want, out)
		}
		if strings.Contains(out, "/afile\n") || strings.Contains(out, "/nosuch\n") {
			t.Errorf("%s: the value was announced before the move failed: %q", tc.src, out)
		}
	}
}

// TestAnOperandWithASlashIsAPlaceHere: a dot or a slash names a place, so the
// variable is never consulted — the same rule CDPATH follows one step up.
func TestAnOperandWithASlashIsAPlaceHere(t *testing.T) {
	for _, src := range []string{
		`shopt -s cdable_vars; d=target; cd ./d`,
		`shopt -s cdable_vars; d=target; cd d/.`,
	} {
		out, st := cdvRun(t, src)
		if st == 0 {
			t.Errorf("%s: status 0, want a failure: %q", src, out)
		}
	}
}

// TestAnEmptyValueIsStillAValueHere: an empty value is taken — it announces a
// blank line, moves nowhere and answers 0 — where an *unset* variable is an
// ordinary failure. The pair is what says the test is "is the name set" and
// not "does it hold something".
func TestAnEmptyValueIsStillAValueHere(t *testing.T) {
	out, st := cdvRun(t, `shopt -s cdable_vars; d=; cd d; echo "st=$?"`)
	if st != 0 || !strings.Contains(out, "st=0") {
		t.Errorf("an empty value should be a no-op at 0: %q at %d", out, st)
	}
	if !strings.HasPrefix(out, "\n") {
		t.Errorf("want a blank line announced, got %q", out)
	}
}

// TestTheCdableVarsNameIsListedAndMoves: the listing and the two statuses. A
// refusal at 1 is what `shopt -s cdable_vars` used to be.
func TestTheCdableVarsNameIsListedAndMoves(t *testing.T) {
	out, st := answersRun(t, `shopt cdable_vars
shopt -s cdable_vars; echo "s=$?"
shopt cdable_vars
shopt -u cdable_vars; echo "u=$?"
shopt cdable_vars`)
	want := "cdable_vars         \toff\ns=0\ncdable_vars         \ton\nu=0\ncdable_vars         \toff\n"
	if out != want || st != 1 {
		t.Errorf("status %d, output %q; want status 1 and %q", st, out, want)
	}
}
