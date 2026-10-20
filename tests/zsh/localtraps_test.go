// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/internal/dialecttest"
)

// LOCAL_TRAPS, measured row by row against zsh 5.9.2. See
// dialect/zsh/localtraps.go for the rule the rows add up to and
// interp/localtraps.go for the mechanism under them; what is asserted here is
// this shell's option — that the name reaches the axis, and that every corner
// of the rule is the one the binary has.
//
// USR1 throughout, with the caller's handler and the function's printing
// different words. A probe whose two handlers say the same thing cannot tell
// "put back" from "left in place"; a probe with no outer handler at all
// cannot tell "put the caller's back" from "cleared everything". The two rows
// that have no outer handler are the ones about exactly that, and each says
// which.

// The row the issue opens with, and the control beside it: without the option
// the function's trap survives in the shell being measured too, so the rows
// below are the option working rather than traps never being local.
func TestLocalTrapsRestoresOnReturn(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`trap 'echo outer' USR1; setopt localtraps; g() { trap 'echo inner' USR1; }; g; trap`)
	if st != 0 || out != "trap -- 'echo outer' USR1\n" {
		t.Errorf("out %q status %d, want the caller's trap back", out, st)
	}
}

func TestTrapsLeakWithoutLocalTraps(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`trap 'echo outer' USR1; g() { trap 'echo inner' USR1; }; g; trap`)
	if st != 0 || out != "trap -- 'echo inner' USR1\n" {
		t.Errorf("out %q status %d, want the function's trap standing", out, st)
	}
}

// And the other control: `localoptions` is not this. The two are separate
// switches with separate mechanisms, and the corpus pins the same row from
// the option table's side.
func TestLocalOptionsDoesNotScopeTraps(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`trap 'echo outer' USR1; g() { setopt localoptions; trap 'echo inner' USR1; }; g; trap`)
	if st != 0 || out != "trap -- 'echo inner' USR1\n" {
		t.Errorf("out %q status %d, want the trap left installed by the *option* switch", out, st)
	}
}

// The save is taken when the trap moves, not when the body starts: a trap set
// *before* the `setopt localtraps` line is not restored. The one row that is
// `localoptions` inverted, and the one that rules out reusing its snapshot.
func TestLocalTrapsDoesNotReachBackBeforeItsLine(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`trap 'echo outer' USR1; g() { trap 'echo inner' USR1; setopt localtraps; }; g; trap`)
	if st != 0 || out != "trap -- 'echo inner' USR1\n" {
		t.Errorf("out %q status %d, want the earlier change left standing", out, st)
	}
}

// And the restore is not asked at the return: a body that turns the option
// back off after moving a trap still has that trap put back. `localoptions`
// answers this one the other way, which is why the two cannot share a rule.
func TestLocalTrapsIsAskedAtTheModificationAndNotAtTheReturn(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`trap 'echo outer' USR1; g() { setopt localtraps; trap 'echo inner' USR1; `+
			`unsetopt localtraps; trap; echo '(inside)'; }; g; trap`)
	want := "trap -- 'echo inner' USR1\n(inside)\ntrap -- 'echo outer' USR1\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q: the body's own trap inside and the caller's after", out, st, want)
	}
}

// The option is per condition, and one body under both answers says so: USR1
// moved while it was on goes back, USR2 moved after it went off does not.
func TestLocalTrapsSavesPerCondition(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`trap 'echo o1' USR1; trap 'echo o2' USR2; `+
			`g() { setopt localtraps; trap 'echo i1' USR1; unsetopt localtraps; trap 'echo i2' USR2; }; g; trap`)
	want := "trap -- 'echo o1' USR1\ntrap -- 'echo i2' USR2\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// `trap COND` is this shell's one-word spelling of a reset, and it is
// localized like the two-word form. A second entry point into the same save,
// which a fix written at one of them alone would leave uncovered.
func TestLocalTrapsUndoesAOneWordReset(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`trap 'echo outer' USR1; setopt localtraps; g() { trap USR1; trap; echo '(inside)'; }; g; trap`)
	want := "(inside)\ntrap -- 'echo outer' USR1\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// `localtraps` is not itself restored — there is no equivalent of
// `localoptions` rule 3 here. A function that turns it on leaves it on.
func TestLocalTrapsItselfIsNotRestored(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`g() { setopt localtraps; trap 'echo inner' USR1; }; g; [[ -o localtraps ]] && echo still_on`)
	if st != 0 || out != "still_on\n" {
		t.Errorf("out %q status %d, want the option left on", out, st)
	}
}

// Which has a consequence worth pinning, because it is what a real rc file
// runs into: once any function has turned the option on, every later call
// scopes its traps too — the option is global, and only the *save* is local.
func TestLocalTrapsLeftOnScopesALaterCall(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`g() { setopt localtraps; trap 'echo g' USR1; }; h() { trap 'echo h' USR1; }; g; h; trap; echo '(end)'`)
	if st != 0 || out != "(end)\n" {
		t.Errorf("out %q status %d, want nothing trapped: h was scoped by the option g left on", out, st)
	}
}

// An anonymous function is a function.
func TestLocalTrapsScopesAnAnonymousFunction(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`trap 'echo outer' USR1; setopt localtraps; () { trap 'echo inner' USR1; }; trap`)
	if st != 0 || out != "trap -- 'echo outer' USR1\n" {
		t.Errorf("out %q status %d, want the caller's trap back", out, st)
	}
}

// `emulate -L` is this option too, and not only `localoptions`. Measured on
// zsh 5.9.2: `emulate -L zsh` leaves `localoptions`, `localtraps` and
// `localpatterns` on and `localloops` off, so the letter is several names and
// the trap it scopes is this switch working rather than the option table's
// rule reaching further than it does.
func TestEmulateLScopesTrapsToo(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`trap 'echo outer' USR1; g() { emulate -L zsh; trap 'echo inner' USR1; }; g; trap`)
	if st != 0 || out != "trap -- 'echo outer' USR1\n" {
		t.Errorf("out %q status %d, want the caller's trap back", out, st)
	}
}

// And the letter reads on where it was written, at the top level as much as
// in a body — which is what makes the *next* call scope its traps.
func TestEmulateLLeavesTheOptionOnAtTheTopLevel(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`emulate -L zsh; [[ -o localtraps ]] && echo on; trap 'echo outer' USR1; `+
			`g() { trap 'echo inner' USR1; }; g; trap; echo '(end)'`)
	want := "on\ntrap -- 'echo outer' USR1\n(end)\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// An ignore this shell *inherited* across a subshell boundary is working and
// unlisted here, and one the child sets itself is listed — so putting the
// disposition back is not the whole of the restore: the record of where it
// came from goes back with it, or the caller's ignore reappears in the
// listing as though the child had set it.
//
// Measured: with the option on the listing is empty, and without it the
// function's own trap is listed, which is what says this row is the option
// working rather than the ignore never having been visible.
func TestLocalTrapsRestoreAnInheritedIgnoreAsInherited(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`trap '' INT; ( setopt localtraps; g() { trap 'echo inner' INT; }; g; trap; echo '(end)' )`)
	if st != 0 || out != "(end)\n" {
		t.Errorf("out %q status %d, want nothing listed: the caller's ignore is inherited and hidden", out, st)
	}
	out, st = runZsh(t, t.TempDir(),
		`trap '' INT; ( g() { trap 'echo inner' INT; }; g; trap; echo '(end)' )`)
	if st != 0 || out != "trap -- 'echo inner' INT\n(end)\n" {
		t.Errorf("out %q status %d, want the function's own trap listed", out, st)
	}
}

// A signal named by number is the same condition as one named by name, and
// the listing says so in the name.
//
// The number is asked of the platform rather than written down. Only the
// first fifteen are fixed by the standard, and USR1 is not one of them: it is
// 30 on this machine and 10 on Linux, so a literal here is a test that passes
// where it was written and fails in CI — which is what it did.
func TestLocalTrapsRestoresASignalNamedByNumber(t *testing.T) {
	n := strconv.Itoa(int(syscall.SIGUSR1))
	out, st := runZsh(t, t.TempDir(),
		`trap 'echo outer' `+n+`; setopt localtraps; g() { trap 'echo inner' `+n+`; }; g; trap`)
	if st != 0 || out != "trap -- 'echo outer' USR1\n" {
		t.Errorf("out %q status %d, want the caller's trap back", out, st)
	}
}

// The disposition and not the listing, proved by sending the signal: the
// handler that runs after the return is the caller's.
//
// Deadlined because a trap that never fires hangs rather than fails, and a
// hang costs the package's whole ten-minute budget instead of one case. The
// bound is orders of magnitude above the milliseconds this takes when it
// works — see the same rule in interp/deadline_test.go.
func TestLocalTrapsRestoreTheDispositionAndNotTheListing(t *testing.T) {
	out, st := zshWithin(t, 20*time.Second,
		`trap 'echo outer' USR1; setopt localtraps; `+
			`g() { trap 'echo inner' USR1; kill -USR1 $$; }; g; kill -USR1 $$; echo done`)
	want := "inner\nouter\ndone\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q: the body's handler inside and the caller's after", out, st, want)
	}
}

// And with nothing displaced, what comes back is the *default action*, which
// for USR1 is death. This is the row with no outer handler, and it is the one
// that says "restored" is not "forgot the listing": a shell that only dropped
// the listing would still have the handler arranged and would print
// `handled`.
//
// The status is 128 plus the signal's number, and the number comes from the
// platform for the reason the case above gives: USR1 is 30 here and 10 on
// Linux, so 158 is a fact about this machine and not about the shell.
func TestLocalTrapsRestoreTheDefaultAction(t *testing.T) {
	want := 128 + int(syscall.SIGUSR1)
	out, st := zshWithin(t, 20*time.Second,
		`setopt localtraps; g() { trap 'echo handled' USR1; }; g; echo before; kill -USR1 $$; echo unreached`)
	if st != want || !strings.HasPrefix(out, "before\n") ||
		strings.Contains(out, "handled") || strings.Contains(out, "unreached") {
		t.Errorf("out %q status %d, want the shell killed by USR1 at %d with no handler run", out, st, want)
	}
}

// zshWithin is runZsh with a deadline, for the rows whose subject is a signal
// arriving: those can hang where the rest can only be wrong.
func zshWithin(t *testing.T, d time.Duration, src string) (string, int) {
	t.Helper()
	type outcome struct {
		out string
		st  int
		err error
	}
	dir := t.TempDir()
	done := make(chan outcome, 1)
	go func() {
		out, st, err := preset.Combined(t, dialecttest.Base{
			Dir: dir, Vars: map[string]string{"PATH": dir},
		}, src)
		done <- outcome{out, st, err}
	}()
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("run %q: %v", src, got.err)
		}
		return got.out, got.st
	case <-time.After(d):
		t.Fatalf("the shell neither ran a handler nor died within %s: %q", d, src)
		return "", 0
	}
}
