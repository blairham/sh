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

// `execfail` makes an `exec` that could not happen an ordinary failed command
// rather than the end of the script. It is the one thing about `exec` this
// shell lets a script move, and it is off with nothing said.
//
// Measured 2026-09-23 on bash 5.3.15, `bash --norc -c`:
//
//	exec nosuchcmd42; echo "st=$?"                   nothing after the complaint
//	shopt -s execfail; exec nosuchcmd42; echo …      the complaint, then st=127
//	shopt -s execfail; exec ./notexec;   echo …      the complaint, then st=126
//	shopt -s execfail; exec ./adir;      echo …      the complaint, then st=126
//
// It sat in shoptStates refusing the write, and that refusal was the
// misleading kind: a script sets the option precisely so a missing command
// does not take the shell down with it, and the quiet refusal left it taken
// down anyway with the line after the `exec` never reached (#4149).

// execfailRun runs src in a scratch directory holding a file that cannot be
// executed and a directory, so the three shapes of failure are all reachable.
func execfailRun(t *testing.T, src string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notexec"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "adir"), 0o755); err != nil {
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

// TestAFailedExecEndsTheScriptUnlessAskedOtherwise is the pair, and the "off"
// half is what makes it a pair: a test that only set the option would pass
// against a shell that never ended the script for a failed `exec` at all.
func TestAFailedExecEndsTheScriptUnlessAskedOtherwise(t *testing.T) {
	out, _ := execfailRun(t, `exec nosuchcmd42; echo REACHED`)
	if strings.Contains(out, "REACHED") {
		t.Errorf("with the option off the script should have ended: %q", out)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("no complaint about the missing command: %q", out)
	}

	out, _ = execfailRun(t, `shopt -s execfail; exec nosuchcmd42; echo REACHED`)
	if !strings.Contains(out, "REACHED") {
		t.Errorf("with the option on the script should have gone on: %q", out)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("the complaint is not the option's business and should stand: %q", out)
	}

	// And the option can be taken back, which is what says it is read at the
	// failure rather than recorded once.
	out, _ = execfailRun(t, `shopt -s execfail; shopt -u execfail; exec nosuchcmd42; echo REACHED`)
	if strings.Contains(out, "REACHED") {
		t.Errorf("after `shopt -u` the script should end again: %q", out)
	}
}

// TestASurvivedExecCarriesTheStatusTheExitWouldHave: the status is the one
// the shell would have exited with, and the three shapes of failure carry
// their own. Asserting only that the script continued would pass against an
// implementation that left `$?` at 0, which is the reading a script's `||`
// depends on.
func TestASurvivedExecCarriesTheStatusTheExitWouldHave(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`shopt -s execfail; exec nosuchcmd42; echo "st=$?"`, "st=127"},
		{`shopt -s execfail; exec ./notexec; echo "st=$?"`, "st=126"},
		{`shopt -s execfail; exec ./adir; echo "st=$?"`, "st=126"},
	} {
		out, _ := execfailRun(t, tc.src)
		if !strings.Contains(out, "\n"+tc.want+"\n") && !strings.HasSuffix(out, tc.want+"\n") {
			t.Errorf("%s: want a line %q in %q", tc.src, tc.want, out)
		}
	}

	// And the status is the *command's* rather than something written beside
	// it, which is what `||` and `set -e` read. Both measured on bash 5.3.15:
	// the `||` fires, and `set -e` ends the script, because a survived `exec`
	// is an ordinary failed command.
	//
	// These two are here because the rows above could not tell a status that
	// reaches the script from one that merely exists. #4316 wrote the status
	// twice — once as the return value and once onto the runner — and a
	// mutation that zeroed the second copy broke nothing, which is what said
	// the copy had no consequence and that no row was reading it.
	if out, _ := execfailRun(t, `shopt -s execfail; exec nosuchcmd42 || echo or-fired`); !strings.Contains(out, "or-fired") {
		t.Errorf("the `||` did not fire, so the failure never reached the script: %q", out)
	}
	if out, _ := execfailRun(t, `shopt -s execfail; set -e; exec nosuchcmd42; echo REACHED`); strings.Contains(out, "REACHED") {
		t.Errorf("`set -e` should end the script for a survived `exec`: %q", out)
	}
}

// TestASurvivedExecLeavesTheExitTrapAlone: two axes decide whether a failed
// `exec` runs the EXIT trap on its way out, and with this option on there is
// no way out — so the trap is neither run nor dropped and fires later at the
// shell's own end.
//
// Measured the same day: the complaint, `after=127`, then `TRAP` — in that
// order. An implementation that reused the exit path would print TRAP in the
// middle, and one that dropped the trap would not print it at all.
func TestASurvivedExecLeavesTheExitTrapAlone(t *testing.T) {
	out, _ := execfailRun(t, `shopt -s execfail; trap 'echo TRAP' EXIT; exec nosuchcmd42; echo "after=$?"`)
	after, trap := strings.Index(out, "after=127"), strings.Index(out, "TRAP")
	if after < 0 || trap < 0 {
		t.Fatalf("want both `after=127` and `TRAP`: %q", out)
	}
	if trap < after {
		t.Errorf("the EXIT trap ran on the exec's way out rather than at the shell's end: %q", out)
	}
}

// TestTheRedirectionFormIsNotAnExec: `exec 3>f` moves a descriptor and runs
// nothing, so the option has nothing to say about it in either state.
func TestTheRedirectionFormIsNotAnExec(t *testing.T) {
	for _, src := range []string{`exec 3>/dev/null; echo "st=$?"`, `shopt -s execfail; exec 3>/dev/null; echo "st=$?"`} {
		out, st := execfailRun(t, src)
		if strings.TrimSpace(out) != "st=0" || st != 0 {
			t.Errorf("%s: %q at %d, want \"st=0\" at 0", src, out, st)
		}
	}
}

// TestTheExecfailNameIsListedAndMoves: the listing, and the statuses of the
// two writes. A refusal at 1 is what `shopt -s execfail` used to be, so the
// `s=0` and `u=0` lines are the assertion rather than decoration.
//
// The call's own status is 1 and not 0, because the last line asks about a
// name that is off and `shopt name` answers 1 for an option that is unset —
// bash's rule, and not this option's business.
func TestTheExecfailNameIsListedAndMoves(t *testing.T) {
	out, st := answersRun(t, `shopt execfail
shopt -s execfail; echo "s=$?"
shopt execfail
shopt -u execfail; echo "u=$?"
shopt execfail`)
	want := "execfail            \toff\ns=0\nexecfail            \ton\nu=0\nexecfail            \toff\n"
	if out != want || st != 1 {
		t.Errorf("status %d, output %q; want status 1 and %q", st, out, want)
	}
}
