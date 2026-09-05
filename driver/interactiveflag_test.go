// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// membership is the shape a script actually uses to read `$-`, and the shape
// the corpus uses for the same reason: the panel disagrees about the spelling
// — whether `c` and `s` appear at all, and in what order — so what is
// assertable is whether a letter is present.
const membership = `case $- in *i*) echo yes ;; *) echo no ;; esac`

// `-i` reaches the runner on every route, which is the half that was dropped.
//
// The flag was read in exactly one branch of the route table — the one with
// nothing to run — so `sh -i script.sh` ran the script with the flag on the
// floor, and no route told the runner anything at all. Measured across bash
// 5.3, dash, ksh93 and zsh: `-i` puts `i` in `$-` on every route, including a
// script operand and `-c`, whether or not a prompt is ever drawn. (#472.)
func TestDashIReachesTheRunnerOnEveryRoute(t *testing.T) {
	script := writeScript(t, membership+"\n")
	for _, tc := range []struct {
		name string
		argv []string
	}{
		{"a script operand", []string{"testsh", "-i", script}},
		{"a script operand after --", []string{"testsh", "-i", "--", script}},
		{"a command string", []string{"testsh", "-i", "-c", membership}},
		{"i bundled with c", []string{"testsh", "-ic", membership}},
		{"i after the command string's own options", []string{"testsh", "-c", "-i", membership}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, code := runArgs(t, shell(), tc.argv...)
			if code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs)
			}
			// Both halves. The script has to have run at all — `-i` must not
			// divert it to a prompt, since three of the four panel shells
			// exit when the script ends and none of them declines to run it
			// — and it has to have run in an interactive shell.
			if strings.TrimSpace(out) != "yes" {
				t.Errorf("got %q, want the program to run and report `i` in $-", out)
			}
		})
	}
}

// And only where it was given. Without `-i` and with something to run, no
// shell in the panel puts `i` in `$-`, so a fix that always set the flag
// would be as wrong as the bug.
func TestWithoutDashIAScriptIsNotInteractive(t *testing.T) {
	script := writeScript(t, membership+"\n")
	for _, tc := range []struct {
		name string
		argv []string
	}{
		{"a script operand", []string{"testsh", script}},
		{"a command string", []string{"testsh", "-c", membership}},
		{"a set option that is not -i", []string{"testsh", "-e", "-c", membership}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, code := runArgs(t, shell(), tc.argv...)
			if code != 0 {
				t.Fatalf("status %d, stderr %q", code, errs)
			}
			if strings.TrimSpace(out) != "no" {
				t.Errorf("got %q, want no `i` in $- for a script", out)
			}
		})
	}
}

// An interactive shell expands aliases whatever the dialect says.
//
// Measured unanimous: all four run the alias in `sh -i script.sh`, and the
// one that would not have run it in `sh script.sh` is the one whose dialect
// leaves alias expansion off. This is the second thing the dropped flag cost,
// and it is why the fact is a field on the Runner rather than a letter added
// straight to `$-`: `$-` is what it is observable *by*, not what it is *for*.
func TestAnInteractiveShellExpandsAliases(t *testing.T) {
	const src = "alias hi='echo aliased'\nhi\n"
	sh := shell()
	if sh.Dialect.ExpandAliases {
		t.Fatal("this test needs a dialect that does not expand aliases in a script")
	}
	// `alias` itself asks an axis the zero vector leaves open — whether it
	// reads options at all — and this test is not about that one. Answered
	// either way, since nothing here passes it an option.
	sem := interp.CoreSemantics()
	sem.AliasParsesOptions = interp.No
	sh.Semantics = sem
	script := writeScript(t, src)

	out, errs, code := runArgs(t, sh, "testsh", "-i", script)
	if code != 0 || strings.TrimSpace(out) != "aliased" {
		t.Errorf("with -i: got %q status %d stderr %q, want the alias expanded", out, code, errs)
	}
	if out, _, _ := runArgs(t, sh, "testsh", script); strings.Contains(out, "aliased") {
		t.Errorf("without -i: got %q, want the alias left alone", out)
	}
}

// A prompt is interactive whether or not `-i` said so, which is the other
// direction of the same fact: all four put `i` in `$-` at a terminal with
// nothing to run.
//
// A real terminal, because that is the only thing that reaches the prompt
// route at all — everything short of one is a script, which is #509's rule.
func TestAPromptIsInteractiveWithoutBeingTold(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // history and startup files, never the user's

	control, tty := terminal(t)
	sh := shell()
	sh.Stdin, sh.Stdout, sh.Stderr = tty, tty, tty

	drawn := watch(t, control)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"testsh"}) }()

	drawn.await(t, "$ ")
	// `yes` alone would be satisfied by the terminal echoing the line that
	// was typed, so the answer is spelled as something the line does not
	// contain.
	write(t, control, `case $- in *i*) echo mark-$((6 * 7)) ;; esac`+"\r")
	drawn.await(t, "mark-42")
	write(t, control, "\x04")

	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("status = %d, want 0", code)
		}
	case <-time.After(10 * time.Second):
		_ = control.Close()
		t.Fatalf("the session did not end; drawn so far: %q", drawn.text())
	}
}

// `-i` with standard input and nothing to run still prompts, and the prompt
// is interactive — the route that already worked, kept working. All four
// accept `-i` away from a terminal and say only that job control is off.
func TestDashIPromptsAwayFromATerminal(t *testing.T) {
	sh := shell()
	sh.Stdin = openFile(t, writeScript(t, membership+"\n"))
	out, errs, code := runArgs(t, sh, "testsh", "-i")
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if !strings.Contains(out, "yes") {
		t.Errorf("got %q, want the prompt to report `i` in $-", out)
	}
}
