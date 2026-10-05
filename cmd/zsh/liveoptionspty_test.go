// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// The options the line editor reads reach it when they are set at the prompt
// (#6167): AUTO_LIST and BEEP decide whether an ambiguous Tab lists and rings,
// PROMPT_SP whether output that never ended its line is marked.
//
// Measured 2026-10-05 against zsh 5.9.2 through a pseudo-terminal with no
// startup file but a prompt: `unsetopt autolist beep` typed at the prompt,
// and the next Tab over three `aa` files neither rings nor lists; `unsetopt
// promptsp`, and `printf y` is followed by the prompt with no `%`. This read
// all of them when the session started, so only the rc file moved them.
func TestEditorOptionsSetAtThePromptReachTheEditor(t *testing.T) {
	control, screen := widgetSession(t, "")
	home := os.Getenv("HOME")
	for _, name := range []string{"aa1", "aa2", "aa3"} {
		if err := os.WriteFile(filepath.Join(home, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// run types keys, then a line whose output is a mark the keys cannot
	// contain, and answers what was drawn from the keys to the next prompt.
	n := 0
	run := func(keys string) string {
		t.Helper()
		n++
		from := len(screen.Text())
		mark := "DONE" + strings.Repeat("x", n)
		if _, err := control.WriteString(keys + "\x15print -r -- DONE${(l:" + itoa(n) + "::x:)}\r"); err != nil {
			t.Fatalf("typing %q: %v", keys, err)
		}
		for _, want := range []string{mark, widgetMark} {
			if err := screen.Await(want, widgetBudget); err != nil {
				t.Fatalf("after %q, want %q\n%v\n%q", keys, want, err, smoke.LastLines(screen.Text(), 6))
			}
		}
		return screen.Text()[from:]
	}

	// The positive control: as the session starts, the Tab rings and lists,
	// and output with no newline is marked.
	if got := run("ls aa\t"); !strings.Contains(got, "\a") || !strings.Contains(got, "aa1  aa2  aa3") {
		t.Fatalf("before any unsetopt, the Tab should ring and list:\n%q", got)
	}
	if got := run("printf y\r"); !strings.Contains(got, "%") {
		t.Fatalf("before any unsetopt, unfinished output should be marked:\n%q", got)
	}

	run("unsetopt autolist beep\r")
	if got := run("ls aa\t"); strings.Contains(got, "\a") || strings.Contains(got, "aa1  aa2") {
		t.Errorf("after unsetopt autolist beep, the Tab rang or listed:\n%q", got)
	}
	// On the same line as the output, which is what says the prompt reads
	// the option rather than the next key: measured, `unsetopt promptsp;
	// printf y` already draws no mark.
	if got := run("unsetopt promptsp; printf y\r"); strings.Contains(got, "%") {
		t.Errorf("after unsetopt promptsp, unfinished output was marked:\n%q", got)
	}
	// PROMPT_CR is the outer of the pair: with it set the prompt still
	// begins with a return after the unfinished output, and unset at the
	// prompt it does not. The output is spelled so the typed line cannot
	// hold it: `printf 'Z%s' Q` draws `ZQ`.
	beforePrompt := func(got, out string) string {
		i := strings.Index(got, out)
		if i < 0 {
			t.Fatalf("%q never drawn:\n%q", out, got)
		}
		rest := got[i+len(out):]
		if j := strings.Index(rest, "HWROW"); j >= 0 {
			rest = rest[:j]
		}
		return rest
	}
	if got := beforePrompt(run("printf 'Z%s' Q\r"), "ZQ"); !strings.Contains(got, "\r") {
		t.Fatalf("with promptcr set, a return should come before the prompt: %q", got)
	}
	if got := beforePrompt(run("unsetopt promptcr; printf 'W%s' Q\r"), "WQ"); strings.Contains(got, "\r") {
		t.Errorf("after unsetopt promptcr, a return still came before the prompt: %q", got)
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// And a widget that unsets them in the middle of a line reaches the Tab after
// it on the same line. Measured the same day: `w() { unsetopt autolist beep
// }` on `^X`, then `ls aa`, `^X`, Tab — zsh neither rings nor lists.
func TestEditorOptionsSetByAWidgetReachTheNextKey(t *testing.T) {
	control, screen := widgetSession(t, "w() { unsetopt autolist beep }\nzle -N w\nbindkey '^X' w\n")
	home := os.Getenv("HOME")
	for _, name := range []string{"aa1", "aa2", "aa3"} {
		if err := os.WriteFile(filepath.Join(home, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	from := len(screen.Text())
	if _, err := control.WriteString("ls aa\x18\t\x15print -r -- END$((3+4))\r"); err != nil {
		t.Fatal(err)
	}
	if err := screen.Await("END7", widgetBudget); err != nil {
		t.Fatalf("%v\n%q", err, smoke.LastLines(screen.Text(), 6))
	}
	if got := screen.Text()[from:]; strings.Contains(got, "\a") || strings.Contains(got, "aa1  aa2") {
		t.Errorf("the Tab after the widget rang or listed:\n%q", got)
	}
}
