// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package zsh_test

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A subshell that kills **the pid it was told it is** runs its own traps.
//
// `$sysparams[pid]` is the only way a zsh script can learn that number, and
// the number this shell answers with is the group the body leads rather than a
// process of its own — see subshellPid. The signal was therefore aimed at the
// group's placeholder leader, a process that is not a shell and has no traps,
// so the kill succeeded, nothing ran, and the subshell left with 0 (#4596).
//
// Measured 2026-09-26 against zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`, `go
// version -m` → not a Go executable). The same file run there prints `trapped`
// and reports 19.
func TestASubshellThatKillsItsOwnPidRunsItsOwnTrap(t *testing.T) {
	got := runZshAnchored(t, `( trap 'print -r -- trapped; exit 19' TERM
  kill $sysparams[pid]
  print -r -- "kill returned $?" )
print -r -- "sub=$?"`)
	if want := "trapped\nsub=19\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// And the same with the signal spelled out, which is the second chunk of
// zsh's own `B11kill.ztst` and the control the issue reported: naming the
// signal changes nothing, so the no-sigspec default was never what was
// missing.
func TestASubshellThatKillsItsOwnPidWithANamedSignalRunsThatTrap(t *testing.T) {
	got := runZshAnchored(t, `( trap 'exit 11' USR1
  kill -USR1 $sysparams[pid] )
print -r -- "sub=$?"`)
	if want := "sub=11\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// **The pid is the noun, and this is the pair that holds it fixed.**
//
// Same construct, same context, same signal and the same trap; only the number
// differs. `$sysparams[pid]` is the body and runs the body's trap; `$$` is
// still the shell at the top — in this shell and in real zsh alike — and a
// trap the subshell set for itself catches nothing aimed at a pid it does not
// have. Measured in zsh 5.9.2: `( trap 'print T' TERM; kill $$; print s )`
// prints `s`, never `T`, and the shell stops after the subshell.
//
// A rule keyed on "is this runner a subshell" cannot tell these two rows
// apart, and that is exactly the wrong noun this case exists to refuse.
func TestASubshellThatKillsTheShellsOwnPidDoesNotRunTheSubshellsTrap(t *testing.T) {
	got := runZshAnchored(t, `( trap 'print -r -- trapped' TERM
  kill $$
  print -r -- sender-ran-on )
print -r -- not-reached`)
	if strings.Contains(got, "trapped") {
		t.Errorf("output = %q: a signal aimed at the shell ran the subshell's trap", got)
	}
	if !strings.Contains(got, "sender-ran-on") {
		t.Errorf("output = %q: the subshell that signaled the shell must run on", got)
	}
	if strings.Contains(got, "not-reached") {
		t.Errorf("output = %q: the shell was killed and must not run the next command", got)
	}
}

// An untrapped fatal signal ends the body that sent it and nothing else, and
// the status the parentheses report is the one a killed process reports.
//
// Measured in zsh 5.9.2 and in bash 5.3.20 alike: 128+15, the sender does not
// run on, and the shell around it carries on to the next command.
func TestAnUntrappedFatalSignalEndsOnlyTheBodyThatSentIt(t *testing.T) {
	got := runZshAnchored(t, `( kill $sysparams[pid]
  print -r -- sender-ran-on )
print -r -- "sub=$?"
print -r -- still-here`)
	if want := "sub=143\nstill-here\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// And the EXIT trap does not run for it, which is the same answer zsh gives a
// shell killed by a signal at the top level — `ExitTrapRunsOnSignalDeath` is
// `No` in this dialect. Measured: `( trap 'print E' EXIT; kill <own pid> )` is
// 143 and prints nothing in zsh 5.9.2.
//
// The row before the fix printed `E` and reported 0, which is a *second* thing
// the missing death cost: the body left the way a body that ran to the end
// leaves.
func TestABodyKilledByItsOwnSignalRunsNoExitTrap(t *testing.T) {
	got := runZshAnchored(t, `( trap 'print -r -- exit-trap' EXIT
  kill $sysparams[pid] )
print -r -- "sub=$?"`)
	if want := "sub=143\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// KILL cannot be caught here either, exactly as it cannot at the top level: a
// `trap` for it is a listing rather than a handler, so the body dies at
// 128+9 with the handler unrun. This shell would otherwise be the one place a
// script could survive `kill -9`, since a self-aimed signal never reaches the
// kernel. See interp's sendSignal for the same rule one boundary out.
func TestABodyCannotCatchAKillAimedAtItself(t *testing.T) {
	got := runZshAnchored(t, `( trap 'print -r -- caught' KILL
  kill -KILL $sysparams[pid] )
print -r -- "sub=$?"`)
	if want := "sub=137\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// Nothing leaves the process, and the placeholder that leads the body's group
// is still there afterwards.
//
// This is the gate's view of what `ps` and `kill -0` say from outside: before
// the fix the anchor really was signaled and really did die — measured, `ps
// -o stat=` on it went from `SN` to nothing and `kill -0` reported no such
// process while the shell carried on — and that is the whole defect seen from
// the other side. A body that has just been handed its group id may be about
// to start something in it, so taking the leader away is not a harmless
// approximation of a self-kill.
//
// The deny is the positive control's other half: a gate that refuses
// everything would make a shell *without* the fix report EPERM rather than
// signal anything, so the assertion is on what reached the gate rather than on
// what the kill returned.
func TestASelfAimedSignalNeverReachesTheKernel(t *testing.T) {
	aimed := gatedKills(t, `( trap 'exit 19' TERM
  kill $sysparams[pid] )
print -r -- "sub=$?"`)
	if len(aimed) != 0 {
		t.Errorf("signals left the shell aimed at %v, want none: the body's own pid is the body", aimed)
	}
}

// And the instrument can produce a positive, asked of the same gate in the
// same run: a pid that is nobody's shell is still a real signal to a real
// process, so a case reporting "none reached the gate" is evidence rather than
// a gate nothing was ever wired to.
//
// 1 is `init`, chosen because the gate denies the send: nothing is delivered,
// and what the case reads is the aim.
func TestTheGateSeesASignalAimedAnywhereElse(t *testing.T) {
	aimed := gatedKills(t, `( kill -TERM 1 )
print -r -- "sub=$?"`)
	if len(aimed) != 1 || aimed[0] != 1 {
		t.Errorf("the gate saw %v, want exactly [1]: a signal aimed elsewhere must still reach it", aimed)
	}
}

// gatedKills runs a script with an anchor and a gate that refuses every
// signal, and reports the pids the shell asked to signal.
func gatedKills(t *testing.T, src string) []int {
	t.Helper()
	anchor, err := exec.LookPath("cat")
	if err != nil {
		t.Fatalf("no placeholder program on this machine: %v", err)
	}
	var mu sync.Mutex
	var aimed []int
	gate := interp.GateFunc(func(_ context.Context, a interp.Action) interp.Decision {
		if a.Kind == interp.ActionSignal {
			mu.Lock()
			aimed = append(aimed, a.PID)
			mu.Unlock()
			return interp.Deny
		}
		return interp.Allow
	})
	f, perr := syntax.Parse(src, zsh.Dialect())
	if perr != nil {
		t.Fatalf("parse %q: %v", src, perr)
	}
	dir := t.TempDir()
	var out, errs bytes.Buffer
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "zsh", Vars: map[string]string{"PATH": dir},
		Dialect: presetDialect(), Gate: gate, ProcessAnchor: []string{anchor},
	}
	zsh.Apply(r)
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	mu.Lock()
	defer mu.Unlock()
	return append([]int(nil), aimed...)
}
