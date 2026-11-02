// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package bash_test

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strings"
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
//
// The notice on the end is #4649's: a foreground subshell a signal ended is a
// child this shell has to say something about, and the bare SIGTERM form is
// what this dialect says. The helper hands back standard output with standard
// error appended, so the order here is the concatenation's and not the
// shell's — at a terminal the notice comes first, as it does in the reference.
//
// The command is written back through the arrangement this shell writes a
// whole program back in, which is what the reference's own deparser produces:
// measured 2026-09-26 on bash 5.3.20, this body comes back as
// `( kill $BASHPID; echo sender-ran-on )` on one line, and a body holding a
// `for` or an `if` keeps newlines in places of its own. Both shapes are
// byte-identical here (#4725); the arrangement itself is pinned in
// interp/killedcommandlayout_test.go.
func TestAnUntrappedSignalAtBashPidEndsOnlyTheBody(t *testing.T) {
	got := runBashAnchored(t, `( kill $BASHPID
  echo sender-ran-on )
echo "sub=$?"
echo still-here`)
	const want = "sub=143\nstill-here\n"
	if !strings.HasPrefix(got, want) {
		t.Errorf("output = %q, want it to start %q", got, want)
	}
	// The notice's own words are the host's — `Terminated: 15` on a BSD and
	// `Terminated` on Linux — so what is asserted is that it named this
	// command and nothing else ran.
	if !strings.HasSuffix(got, "( kill $BASHPID; echo sender-ran-on )\n") {
		t.Errorf("output = %q, want the subshell's own text written back", got)
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

// A copy of the subshell that kills the subshell's `$BASHPID` reaches the
// subshell, and its next external command still starts (#7233).
//
// The copy has a group of its own, so the number was once somebody else's
// process to it and went to kill(2) — at the subshell's placeholder, which
// died of it. No handler ran, and the pipeline's `/bin/cat`, put into the
// placeholder's group as it started, failed with `fork/exec /bin/cat:
// operation not permitted`. Measured 2026-11-01 under `-c` against bash
// 5.3.20 and zsh 5.9.2, which agree on every row: the handler runs once the
// copy is done, before the next command.
func TestACopyThatKillsTheSubshellsBashPidRunsTheSubshellsTrap(t *testing.T) {
	for _, tc := range []struct{ name, copy string }{
		{"pipeline element", `{ kill -USR1 $p; echo l; } | /bin/cat`},
		{"nested subshell", `(kill -USR1 $p; echo l)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runBashAnchored(t, `( trap 'echo U' USR1; p=$BASHPID; `+tc.copy+`; echo after )
echo "outer $?"`)
			if want := "l\nU\nafter\nouter 0\n"; got != want {
				t.Errorf("output = %q, want %q", got, want)
			}
		})
	}
}

// The rest of the body's own `kill $BASHPID`, sent from the copy: an ignored
// signal does nothing, and an untrapped fatal one ends the subshell — not the
// copy, which has finished its line by then — at 128+N. Measured as above.
func TestACopyThatKillsTheSubshellsBashPidIsAnsweredByTheSubshell(t *testing.T) {
	got := runBashAnchored(t, `( trap '' USR1; p=$BASHPID; { kill -USR1 $p; } | /bin/cat; echo after )
echo "outer $?"`)
	if want := "after\nouter 0\n"; got != want {
		t.Errorf("ignored: output = %q, want %q", got, want)
	}
	got = runBashAnchored(t, `( p=$BASHPID; (kill $p; echo inner); echo after )
echo "outer $?"`)
	if want := "inner\nouter 143\n"; !strings.HasPrefix(got, want) {
		t.Errorf("untrapped: output = %q, want it to start %q", got, want)
	}
}

// A body's own signal that arrives again while it waits runs its handler once,
// as the shell's does: interp.Semantics' WaitingSignalArrivingAgainRunsOnce
// (#7166). Measured 2026-11-01 on bash 5.3.20 under `-c`, `T after`.
func TestABodysOwnSignalArrivingAgainRunsItsHandlerOnce(t *testing.T) {
	got := runBashAnchored(t, `( trap 'echo T' USR1; kill -USR1 $BASHPID $BASHPID; echo after )`)
	if want := "T\nafter\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// A body's own signal is handled before its `kill` returns, as the shell's is
// (#5359) — so a body whose last command is that `kill` still runs the
// handler, and an and-or list after it sees the handler first (#7345).
//
// The body here is a pipeline element or a `&` job that is one simple
// command, which has no moment between commands after the `kill` for the
// handler to wait for: it ran nothing at all. Measured 2026-11-02 under `-c`,
// each shell in a process group of its own, against bash 5.3.20 and 3.2.57
// (3.2 reading the pid back from a child's `$PPID`); zsh 5.9.2, ksh93u+ and
// dash agree on every row they can reach. Each row's want is what they wrote.
func TestABodyRunsTheHandlerForItsOwnSignalAsKillReturns(t *testing.T) {
	const own = `trap "echo U" USR1; kill -USR1 $BASHPID`
	for _, c := range []struct{ name, src, want string }{
		{"a simple-command element", `eval '` + own + `' | /bin/cat; echo out=$?`, "U\nout=0\n"},
		{"a simple-command job", `eval '` + own + `' & wait $!; echo out=$?`, "U\nout=0\n"},
		{"before the and-or list goes on", `( trap 'echo U' USR1; kill -USR1 $BASHPID && echo a )`, "U\na\n"},
		{
			"a handler that exits ends the element with its status",
			`eval 'trap "echo U; exit 7" USR1; kill -USR1 $BASHPID' | /bin/cat; echo st=${PIPESTATUS[0]}`,
			"U\nst=7\n",
		},
		{
			"before the element's EXIT trap",
			`eval 'trap "echo X" EXIT; ` + own + `' | /bin/cat`,
			"U\nX\n",
		},
		// Inside the `eval`'s own redirection, which is what says the
		// moment is the `kill` returning and not the element ending.
		{
			"inside the command's redirections",
			`eval 'trap "echo U >&2" USR1; kill -USR1 $BASHPID' 2>/dev/null | /bin/cat; echo out=$?`,
			"out=0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := runBashAnchored(t, c.src); got != c.want {
				t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
			}
		})
	}
}

// A **group** kill at `$BASHPID` is refused as no such process, from wherever
// in the subshell it is sent, and nothing is signaled (#7238).
//
// The number is a group here because a placeholder leads one. In bash the
// subshell is a fork that leads no group unless the monitor was on when it was
// forked, so there is nothing of that number to signal. Measured 2026-11-01
// under `-c` on bash 5.3.20: every row is `kill: (-N) - No such process` and 1,
// and zsh 5.9.2 answers the same with `$sysparams[pid]`. The pipeline row is
// the issue's own: the kill reached the placeholder, and the `/bin/cat` beside
// it, checked into the group a moment before it went, failed with `fork/exec
// /bin/cat: operation not permitted` and took `k=0` with it.
func TestAGroupKillAtBashPidIsNoSuchProcess(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"from a pipeline element",
			`( p=$BASHPID; { kill -- -$p; echo k=$?; } | /bin/cat; /bin/echo after )`, "k=1\nafter\n",
		},
		{"from the body itself", `( p=$BASHPID; kill -- -$p; echo k=$?; /bin/echo after )`, "k=1\nafter\n"},
		{"from a nested subshell", `( p=$BASHPID; (kill -- -$p; echo k=$?); /bin/echo after )`, "k=1\nafter\n"},
		{"with a named signal", `( p=$BASHPID; kill -TERM -$p; echo k=$? )`, "k=1\n"},
		{"as the existence probe", `( p=$BASHPID; kill -0 -$p; echo k=$? )`, "k=1\n"},
		// The monitor that decides is the one the subshell was forked
		// under: turned on inside it, it makes no group. The pair to this
		// row is the test below.
		{"with the monitor turned on inside", `( set -m; p=$BASHPID; kill -0 -- -$p; echo k=$? )`, "k=1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runBashAnchored(t, tc.src+"\necho \"outer $?\"")
			got = groupOperand.ReplaceAllString(got, "(-N)")
			if want := tc.want + "outer 0\nbash: line 1: kill: (-N) - No such process\n"; got != want {
				t.Errorf("output = %q, want %q", got, want)
			}
		})
	}
}

// And the half that stays: a subshell forked while the monitor was on is a
// group of its own in bash, so the same probe finds it — even once the
// subshell has turned the monitor off. Measured 2026-11-01 on bash 5.3.20,
// `k=0`. With the row above, this is what says the rule is keyed on the
// monitor at the fork and not on the monitor at the `kill`.
func TestAGroupKillAtBashPidReachesASubshellForkedUnderTheMonitor(t *testing.T) {
	got := runBashAnchored(t, `set -m; ( set +m; p=$BASHPID; kill -0 -- -$p; echo k=$? )`)
	if want := "k=0\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// groupOperand is the negative pid bash quotes back in a refusal, which is a
// different number every run.
var groupOperand = regexp.MustCompile(`\(-[0-9]+\)`)

// runBashAnchored runs a script with a placeholder program supplied, which is
// what a shell binary does for itself — see driver/procanchor.go. Without one
// `$BASHPID` in a forked body is empty and none of the cases above has a
// number to aim at.
// A pipeline element that sends its own `$BASHPID` a signal the parent trapped
// dies of it: the element keeps a listing of the parent's traps and not the
// traps, so no handler runs and the shell carries on past the pipeline —
// interp.Semantics.PipelineElementKeepsTheTraps, which zsh answers the other
// way (#7327). Measured 2026-11-02 under `-c` against bash 5.3.20 and 3.2.57:
// `out=0` alone, and `in` too with the signal ignored in place of the handler.
func TestAPipelineElementThatSignalsItselfDiesOfTheDefault(t *testing.T) {
	got := runBashAnchored(t, `trap 'echo T' USR1; { kill -USR1 $BASHPID; echo in; } | /bin/cat; echo out=$?`)
	if want := "out=0\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	got = runBashAnchored(t, `trap '' USR1; { kill -USR1 $BASHPID; echo in; } | /bin/cat; echo out=$?`)
	if want := "in\nout=0\n"; got != want {
		t.Errorf("ignored: output = %q, want %q", got, want)
	}
}

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
	r := (&interp.Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "bash",
		Dialect: presetDialect(), ProcessAnchor: []string{anchor},
	}).SeedVars(map[string]string{"PATH": dir})
	bash.Apply(r)
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String() + errs.String()
}
