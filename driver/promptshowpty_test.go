// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
)

// `prompt show`, end to end, at a real terminal.
//
// The only place it can be asked. The function exists only in an interactive
// session — see driver/promptprelude.go for why the substrate's prelude is
// sourced there and nowhere else — so every test in this package that reads a
// pipe is structurally unable to reach it, and a unit test of the report is a
// test of a struct rather than of a word a person types.
//
// **A two-row prompt throughout.** The configuration under test names
// `newline`, so the theme draws two rows, and the report has to say so. A
// one-row configuration would exercise the same code and prove nothing about
// the thing the spec asks for: a person configures a two-row prompt and asks
// what it resolved to.

// TestPromptShowNamesEverySettingAndWhichLayerAnsweredIt drives a session,
// configures a theme at the prompt, and asks.
func TestPromptShowNamesEverySettingAndWhichLayerAnsweredIt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	control, tty := terminal(t)
	sh := shell()
	sh.Stdin, sh.Stdout, sh.Stderr = tty, tty, tty

	drawn := watch(t, control, defaultPrompt)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"testsh"}) }()
	drawn.awaitReadyForInput(t)

	// A two-row prompt, configured the way a person configures one: variables
	// in the session, which is the cheapest of the spec's four layers and the
	// one every dialect has.
	write(t, control, "SH_PROMPT_LEFT_ELEMENTS='dir newline prompt_char'\r")
	write(t, control, "SH_PROMPT_DIR_FOREGROUND=4\r")
	// And an element nothing draws, because the report's job is to say so.
	write(t, control, "SH_PROMPT_RIGHT_ELEMENTS='weather'\r")
	write(t, control, "prompt show\r")

	// The three claims the spec makes about this surface, each waited for
	// rather than read out of a buffer at a deadline.
	drawn.await(t, "DIR_FOREGROUND = 4")
	drawn.await(t, "weather")
	write(t, control, "echo done-$((6 * 7))\r")
	drawn.await(t, "done-42")

	text := drawn.text()
	for _, want := range []string{
		"drawing: yes",
		"[session]",
		"not yet",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("`prompt show` did not say %q; it drew:\n%s", want, text)
		}
	}
	// The element that does draw says what draws it, and it is not the same
	// answer as the one nothing draws — a report where every line said the
	// same thing would pass the check above and be useless.
	if !strings.Contains(text, "dir — built-in") {
		t.Errorf("`prompt show` did not name what draws `dir`; it drew:\n%s", text)
	}

	endPromptSession(t, control, done, drawn)
}

// TestPromptIsPresentedTheWayAPreludeNameIs pins the divergence rather than
// leaving it to be discovered.
//
// A prelude function is presented as a builtin — #1117's one table — so
// `prompt` answers `type` the way `pushd` does. That is the cost the
// maintainer accepted on 2026-09-21 in exchange for not adding a builtin, and
// a cost nothing asserts is a cost nobody can notice changing.
func TestPromptIsPresentedTheWayAPreludeNameIs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	control, tty := terminal(t)
	sh := shell()
	sh.Stdin, sh.Stdout, sh.Stderr = tty, tty, tty

	drawn := watch(t, control, defaultPrompt)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"testsh"}) }()
	drawn.awaitReadyForInput(t)

	write(t, control, "type prompt\r")
	drawn.await(t, "prompt is a shell builtin")
	// And the Go behind it is *not* a name this shell has. The lookup answers
	// it only while a prelude function is running, so a person asking about
	// it is asking about nothing — which is the half that keeps the surface
	// from widening twice.
	write(t, control, "type promptengine; echo asked-$((6 * 7))\r")
	drawn.await(t, "asked-42")
	if strings.Contains(drawn.text(), "promptengine is a shell builtin") {
		t.Errorf("the engine's own word is presented as a builtin; the session drew:\n%s", drawn.text())
	}

	endPromptSession(t, control, done, drawn)
}

// TestPromptRefusesAWordItDoesNotKnow checks the other side of the surface.
//
// Named `prompt` and not `promptengine`, because a complaint naming a word
// the person did not type is one they cannot search for — and located the
// dialect's way, which is the whole reason the name is a prelude function
// rather than a Go builtin writing to a stream.
func TestPromptRefusesAWordItDoesNotKnow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	control, tty := terminal(t)
	sh := shell()
	sh.Stdin, sh.Stdout, sh.Stderr = tty, tty, tty

	drawn := watch(t, control, defaultPrompt)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"testsh"}) }()
	drawn.awaitReadyForInput(t)

	write(t, control, "prompt wibble; echo status-$?\r")
	drawn.await(t, "status-2")
	if !strings.Contains(drawn.text(), "prompt: wibble: no such subcommand") {
		t.Errorf("the refusal did not name the word the person typed:\n%s", drawn.text())
	}

	endPromptSession(t, control, done, drawn)
}

// TestAScriptHasNoPromptFunction is the containment.
//
// The substrate's prelude is sourced for an interactive session only, so a
// script reaches no `prompt` and the name answers exactly what it answered
// before any of this existed. Without that, every dialect binary would report
// a name to `type` in a script that the shell it claims to be does not have.
func TestAScriptHasNoPromptFunction(t *testing.T) {
	sh := shell()
	out, _, _ := runArgs(t, sh, "testsh", "-c", "type prompt >/dev/null 2>&1; echo saw-$?")
	if strings.TrimSpace(out) == "saw-0" {
		t.Errorf("a script found a `prompt` command: %q", out)
	}
}

// endPromptSession ends a session the tests above started, and turns a hang
// into a failure rather than a stuck run.
func endPromptSession(t *testing.T, control interface{ WriteString(string) (int, error) }, done <-chan int, drawn *screen) {
	t.Helper()
	if _, err := control.WriteString("exit\r"); err != nil {
		t.Fatalf("ending the session: %v", err)
	}
	select {
	case <-done:
	case <-time.After(sessionBudget):
		t.Fatalf("the session did not end; drawn so far: %q", drawn.text())
	}
}
