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
