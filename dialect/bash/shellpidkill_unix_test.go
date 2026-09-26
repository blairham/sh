// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package bash_test

import (
	"bytes"
	"context"
	"os/exec"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A subshell that kills `$BASHPID` runs its own traps, and this is the row
// that says the rule is the **core's** rather than a dialect's.
//
// #4596 reported the same shape against zsh and recorded that real bash does
// not fire the trap on it. Measured 2026-09-26 against bash 5.3.20
// (`/opt/homebrew/bin/bash --norc --noprofile`, `go version -m` → not a Go
// executable) it does — as a script file and as the last command of a `-c`,
// with the same 19 zsh reports and the same 143 for an untrapped one. So the
// two shells that can reach this shape at all agree about it, nothing is asked
// of a Semantics axis, and the fix lives in interp's sendSignal.
//
// `$BASHPID` and `$sysparams[pid]` are the only two ways a script can learn
// the number, and both answer through [interp.Runner.SubshellProcessGroup] —
// so a dialect without such a parameter cannot reach this code at all.
func TestASubshellThatKillsBashPidRunsItsOwnTrap(t *testing.T) {
	got := runBashAnchored(t, `( trap 'echo trapped; exit 19' TERM
  kill $BASHPID
  echo "kill returned $?" )
echo "sub=$?"`)
	if want := "trapped\nsub=19\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// And an untrapped one ends the body at 128+15, leaving the shell around it to
// carry on — the same answer zsh gives, measured in both.
func TestAnUntrappedSignalAtBashPidEndsOnlyTheBody(t *testing.T) {
	got := runBashAnchored(t, `( kill $BASHPID
  echo sender-ran-on )
echo "sub=$?"
echo still-here`)
	if want := "sub=143\nstill-here\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// `$$` is the other half of the pair that holds the noun fixed. Same
// construct, same signal, same trap — the parent's number, so the parent dies
// and the body's own trap never runs.
func TestASubshellThatKillsTheShellsPidDoesNotRunTheSubshellsTrap(t *testing.T) {
	got := runBashAnchored(t, `( trap 'echo trapped' TERM
  kill $$
  echo sender-ran-on )
echo not-reached`)
	if want := "sender-ran-on\n"; got != want {
		t.Errorf("output = %q, want %q: only the sender runs on, and it runs no trap", got, want)
	}
}

// runBashAnchored runs a script with a placeholder program supplied, which is
// what a shell binary does for itself — see driver/procanchor.go. Without one
// `$BASHPID` in a forked body is empty and none of the cases above has a
// number to aim at.
func runBashAnchored(t *testing.T, src string) string {
	t.Helper()
	anchor, err := exec.LookPath("cat")
	if err != nil {
		t.Fatalf("no placeholder program on this machine: %v", err)
	}
	f, perr := syntax.Parse(src, bash.Dialect())
	if perr != nil {
		t.Fatalf("parse %q: %v", src, perr)
	}
	dir := t.TempDir()
	var out, errs bytes.Buffer
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "bash", Vars: map[string]string{"PATH": dir},
		Dialect: presetDialect(), ProcessAnchor: []string{anchor},
	}
	bash.Apply(r)
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String() + errs.String()
}
