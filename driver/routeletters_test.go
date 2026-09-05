// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// routeMembership asks `$-` for the two route letters the way a script would.
// Membership rather than the string, because no two shells in the panel order
// `$-` alike.
const routeMembership = `case $- in *c*) echo has-c ;; *) echo no-c ;; esac
case $- in *s*) echo has-s ;; *) echo no-s ;; esac`

// routeShell answers both route axes, so a test can say which answer it is
// exercising without naming a shell.
func routeShell(showsC, showsS interp.Answer) driver.Shell {
	sh := shell()
	sh.Semantics.CommandStringShowsCInDollarDash = showsC
	sh.Semantics.CommandStringShowsSInDollarDash = showsS
	return sh
}

// The front end reads the route off the invocation and hands it to the
// runner, which is the whole of what #551 needed: `interp` has no way to know
// whether `-c` was written, and every letter below follows from it.
func TestTheInvocationRouteReachesDollarDash(t *testing.T) {
	script := writeScript(t, routeMembership+"\n")
	for _, tc := range []struct {
		name   string
		showsC interp.Answer
		showsS interp.Answer
		argv   []string
		want   string
	}{
		{
			"a script operand is neither route, whatever the axes say",
			interp.Yes, interp.Yes,
			[]string{"testsh", script},
			"no-c\nno-s\n",
		},
		{
			"a command string, in a shell that shows the letter",
			interp.Yes, interp.No,
			[]string{"testsh", "-c", routeMembership},
			"has-c\nno-s\n",
		},
		{
			"a command string, in a shell that shows neither",
			interp.No, interp.No,
			[]string{"testsh", "-c", routeMembership},
			"no-c\nno-s\n",
		},
		{
			"a command string, in the shell that shows both",
			interp.Yes, interp.Yes,
			[]string{"testsh", "-c", routeMembership},
			"has-c\nhas-s\n",
		},
		{
			"a bundled command string is the same route",
			interp.Yes, interp.No,
			[]string{"testsh", "-ec", routeMembership},
			"has-c\nno-s\n",
		},
		{
			// `-c` wins about where the program comes from and does not
			// take the letter away: measured, all six run the command
			// string here and all six still show `s`.
			"`-s` written and `-c` supplying the program anyway",
			interp.No, interp.No,
			[]string{"testsh", "-s", "-c", routeMembership},
			"no-c\nhas-s\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, code := runArgs(t, routeShell(tc.showsC, tc.showsS), tc.argv...)
			if code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs)
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// A program arriving on standard input shows `s` with no axis asked, which is
// the unanimous half of the letter — with `-s` written or without it.
func TestAProgramOnStandardInputShowsTheSLetter(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
	}{
		{"no operands at all", []string{"testsh"}},
		{"after an end of options", []string{"testsh", "--"}},
		{"with -s written out", []string{"testsh", "-s"}},
		{"with -s and parameters", []string{"testsh", "-s", "a", "b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Both axes left unanswered, since neither is this route's
			// question: the letter must arrive without one being asked.
			sh := routeShell(interp.Unspecified, interp.Unspecified)
			sh.Stdin = pipeWith(t, routeMembership+"\n")
			var o, e bytes.Buffer
			sh.Stdout, sh.Stderr = &o, &e
			if code := driver.MainArgs(sh, tc.argv); code != 0 {
				t.Fatalf("status %d, stderr %q", code, e.String())
			}
			if got, want := o.String(), "no-c\nhas-s\n"; got != want {
				t.Errorf("got %q, want %q", got, want)
			}
			if s := e.String(); strings.Contains(s, "disagree") {
				t.Errorf("said %q, want the letter decided without asking an axis", s)
			}
		})
	}
}

// A prompt is the standard-input route with a person on the other end, and
// `$-` says so: all four shells show `s` at a terminal with nothing to run.
//
// Through a real pseudo-terminal, because that is the only way to reach the
// branch — the prompt route hands off before the one that reads a route off
// the operands, and session names the route itself. A seam test would prove
// the field and not that anything sets it.
func TestAPromptIsTheStandardInputRoute(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // history and startup files, never the user's

	control, tty := terminal(t)
	// Both axes unanswered: the letter is the route's and no axis is asked
	// for it.
	sh := routeShell(interp.Unspecified, interp.Unspecified)
	sh.Stdin, sh.Stdout, sh.Stderr = tty, tty, tty

	drawn := watch(t, control)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"testsh"}) }()

	drawn.awaitReadyForInput(t)
	// The marker is split in the typed line and whole in the answer, so
	// waiting on it cannot be satisfied by the terminal echoing what was
	// typed back.
	write(t, control, `case $- in *s*) echo "route""=stdin" ;; *) echo "route""=other" ;; esac`+"\r")
	drawn.await(t, "route=stdin")
	// The answer says the command ran; the next prompt says the shell is
	// reading again, which is the only moment ^D survives.
	drawn.endSession(t, control)

	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("status = %d, want 0", code)
		}
	case <-time.After(sessionBudget):
		_ = control.Close() // unblock the read the shell is sitting in
		t.Fatalf("the session did not end; drawn so far: %q", drawn.text())
	}
}
