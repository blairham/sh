// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// A real interactive session has the dialect's default prompt in PS1, and the
// run-commands file can read it.
//
// Through a pseudo-terminal, with **no `-i` and no `-c`**, because neither of
// those is this question. `-i -c cmd` runs one command and leaves without
// entering the prompt loop, and a session assembled any other way is not one a
// person could have. The probe records `$-` in the same run so the assertion
// that this was interactive is evidence rather than a claim about the
// invocation.
//
// It is also why "a prompt appeared" cannot stand in for this. The drawer has
// always fallen back to [repl.PromptStyle.Default] when nothing was assigned,
// so the screen looked right — `bash-5.3$ ` — while `$PS1` was empty and
// `[ -z "$PS1" ] && return` at the top of a person's `~/.bashrc` fired at a
// prompt and skipped the whole file (#1421). Every probe that watches the
// screen passes on that bug.
func TestAnInteractiveSessionHasTheDefaultPromptInPS1(t *testing.T) {
	home := t.TempDir()
	// Never the person's own home: this session reads startup files and
	// writes history, and neither belongs anywhere near theirs.
	t.Setenv("HOME", home)
	// Written to a file and read through $ENV, so the value is observed at
	// the moment a real `~/.bashrc` observes it rather than after the session
	// has settled.
	rc := filepath.Join(home, "rc.sh")
	if err := os.WriteFile(rc, []byte(
		`case $- in *i*) echo "SAW-INTERACTIVE";; *) echo "SAW-NONINTERACTIVE";; esac`+"\n"+
			`echo "SAW-PS1[${PS1-unset}]"`+"\n"+
			`echo "SAW-PS2[${PS2-unset}]"`+"\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}

	control, tty := terminal(t)
	sh := shell()
	// A table unlike any real shell's, because nothing outside dialect/ names
	// one — and because a driver that had hard-coded a real default would
	// pass here by accident. What the real tables hold is asserted in
	// dialect/bash/prompt_test.go and graded by the corpus.
	// It ends in `$ ` because that is the anchor endSession waits on, and
	// nothing else about it resembles a shell.
	sh.PromptStyle = repl.PromptStyle{Default: "ptyPS1$ ", DefaultContinued: "ptyPS2> "}
	sh.Stdin, sh.Stdout, sh.Stderr = tty, tty, tty

	t.Setenv("ENV", rc)

	drawn := watch(t, control)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"testsh"}) }()

	// endSession does the waiting: a caller that waits for the prompt itself
	// and then calls it hangs on a prompt that is never drawn again, because
	// the screen's cursor has already passed the only one there is.
	drawn.endSession(t, control)
	select {
	case <-done:
	case <-time.After(sessionBudget):
		_ = control.Close()
		t.Fatalf("the session did not end; drawn so far: %q", drawn.text())
	}

	got := drawn.text()
	// The session has to have been interactive for the rest to mean
	// anything, and this is the shell's own answer rather than the test's.
	if !strings.Contains(got, "SAW-INTERACTIVE") {
		t.Fatalf("the session was not interactive, so it cannot answer this; drawn: %q", got)
	}
	// The whole value in each case. A length would be satisfied by any
	// eight characters, and a Contains on the payload alone cannot see a
	// prefix somebody added — the brackets are there to make the assertion
	// an equality on the parameter.
	for _, want := range []string{"SAW-PS1[ptyPS1$ ]", "SAW-PS2[ptyPS2> ]"} {
		if !strings.Contains(got, want) {
			t.Errorf("the rc file did not see %s; drawn: %q", want, got)
		}
	}
}

// The two defects together, in the shape they actually occur in: the first
// line of a real `~/.bashrc`.
//
//	[ -z "$PS1" ] && return
//
// They canceled out. `$PS1` was empty in an interactive session (#1421) so
// the guard fired, and the only reason the whole file was not abandoned is
// that `return` in a startup file was refused (#1422). Fixing the second
// alone would have turned a cosmetic wrongness into the rc being skipped
// entirely, at status 0 with nothing said — so the pair has to be checked
// together and not only one at a time.
//
// Through a pseudo-terminal, no `-c` and no `-i`, with `$-` recorded in the
// same run.
func TestTheStandardRcGuardDoesNotFireAtAPrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rc := filepath.Join(home, "rc.sh")
	if err := os.WriteFile(rc, []byte(
		`case $- in *i*) echo "SAW-INTERACTIVE";; *) echo "SAW-NONINTERACTIVE";; esac`+"\n"+
			`[ -z "$PS1" ] && return`+"\n"+
			`echo "RC-RAN-ON"`+"\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV", rc)

	control, tty := terminal(t)
	sh := shell()
	sh.PromptStyle = repl.PromptStyle{Default: "ptyPS1$ ", DefaultContinued: "ptyPS2> "}
	// Asked because the file returns on the other route, so the axis has to
	// have an answer for this shell to be one at all.
	sh.Semantics.StartupFileReturnCarriesItsArgument = interp.Yes
	sh.Stdin, sh.Stdout, sh.Stderr = tty, tty, tty

	drawn := watch(t, control)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"testsh"}) }()
	drawn.endSession(t, control)
	select {
	case <-done:
	case <-time.After(sessionBudget):
		_ = control.Close()
		t.Fatalf("the session did not end; drawn so far: %q", drawn.text())
	}

	got := drawn.text()
	if !strings.Contains(got, "SAW-INTERACTIVE") {
		t.Fatalf("the session was not interactive, so it cannot answer this; drawn: %q", got)
	}
	if !strings.Contains(got, "RC-RAN-ON") {
		t.Errorf("the guard fired at a prompt and the rest of the rc file was skipped; drawn: %q", got)
	}
	if strings.Contains(got, "can only") {
		t.Errorf("a `return` in a startup file was refused; drawn: %q", got)
	}
}

// And the guard still fires where it is meant to. A shell with nobody to
// prompt leaves PS1 unset, so the guard fires and the `return` is obeyed: the
// rc stops, silently, which is the whole reason the line is written.
//
// The route is the *non-interactive* startup file — `$BASH_ENV`'s shape,
// Semantics.NonInteractiveStartupVariable — because that is where a person's
// file is read with nobody to prompt. `$ENV` is not it and neither is
// `-i -c`: measured, bash on `-i -c` has PS1 set to its default, so the guard
// does not fire there in real bash either.
//
// The pair is the point. A shell that got only #1421 right would keep going
// here, and one that got only #1422 right would stop at a prompt.
func TestTheStandardRcGuardStillFiresForAScript(t *testing.T) {
	home := t.TempDir()
	rc := filepath.Join(home, "rc.sh")
	if err := os.WriteFile(rc, []byte(
		`echo "RC-STARTED"`+"\n"+
			`[ -z "$PS1" ] && return`+"\n"+
			`echo "RC-RAN-ON"`+"\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("SHELL_ENV", rc)

	sh := shell()
	sh.PromptStyle = repl.PromptStyle{Default: "ptyPS1$ ", DefaultContinued: "ptyPS2> "}
	sh.Semantics.StartupFileReturnCarriesItsArgument = interp.Yes
	sh.Semantics.NonInteractiveStartupVariable = "SHELL_ENV"
	out, errs, code := runArgs(t, sh, "testsh", "-c", "echo main")
	if code != 0 {
		t.Fatalf("status = %d, stderr %q", code, errs)
	}
	if !strings.Contains(out, "RC-STARTED") {
		t.Fatalf("the rc file did not run at all: %q / %q", out, errs)
	}
	if strings.Contains(out, "RC-RAN-ON") {
		t.Errorf("the guard did not fire and the rc kept going: %q", out)
	}
	if strings.Contains(errs, "can only") {
		t.Errorf("the guard fired and the `return` was refused: %q", errs)
	}
	if !strings.Contains(out, "main") {
		t.Errorf("the shell did not go on to its own work: %q", out)
	}
}
