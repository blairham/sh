// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// `C-o` accepts the line and leaves the entry after it on the next prompt.
//
// It is how a run of history is replayed a line at a time — hold it down and
// each entry runs in turn — and it is the whole of eighteen of
// `history.tests`'s differing lines, because the suite's inner interactive
// shells replay recalled commands with nothing else (#4177).
//
// Two reads rather than one, because the key's meaning is split across them:
// the first is where it accepts, the second is where what it fetched shows up.
// A test that asked only about the first would pass against a `C-o` that is a
// plain Return.
func TestOperateAndGetNextAcceptsAndFetchesTheOneAfter(t *testing.T) {
	e := &editor{
		in: typing("\x12one\x0f"), out: &strings.Builder{},
		history: fourLines,
		width:   func() int { return 80 },
	}
	// `C-r one` lands on `echo one`, and `C-o` accepts it.
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if line != "echo one" {
		t.Fatalf("accepted %q, want the entry the search found", line)
	}
	// The accepted line is remembered, exactly as the caller's loop does it,
	// so the indexes the next read walks are the ones a session really has.
	e.remember(line)
	e.in = typing("\r")
	line, err = e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if want := "echo two"; line != want {
		t.Errorf("next prompt offered %q, want %q — the entry after the one C-o ran", line, want)
	}
	// And the prompt after *that* is a fresh one. The fetch belongs to the one
	// key that asked for it: a second entry appearing unasked would be a shell
	// that had started replaying the history on its own.
	e.remember(line)
	e.in = typing("\r")
	line, err = e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("third read: %v", err)
	}
	if line != "" {
		t.Errorf("the prompt after that offered %q, want an empty line", line)
	}
}

// And the index is into the list as it stands at each prompt, which is what
// makes a second `C-o` land where it does.
//
// Measured 2026-09-23 against bash 5.3.20 with `echo 0`, `echo 1`, `echo 2` and
// `echo 3` in the history and no terminal on any stream: `Up Up C-o C-o` then
// a newline runs `echo 2`, `echo 3` and `echo 2` again — the last because
// running the first appended it, so the entry now sitting one past `echo 3` is
// the copy of `echo 2` this walk made.
func TestOperateAndGetNextWalksTheListAsItGrows(t *testing.T) {
	e := &editor{
		in: typing("\x1b[A\x1b[A\x0f"), out: &strings.Builder{},
		history: []string{"echo 0", "echo 1", "echo 2", "echo 3"},
		width:   func() int { return 80 },
	}
	var ran []string
	for _, keys := range []string{"", "\x0f", "\r"} {
		if keys != "" {
			e.in = typing(keys)
		}
		line, err := e.readLine(drawPrompt("$ "))
		if err != nil {
			t.Fatalf("read %d: %v", len(ran), err)
		}
		ran = append(ran, line)
		e.remember(line)
	}
	want := []string{"echo 2", "echo 3", "echo 2"}
	for i := range want {
		if ran[i] != want[i] {
			t.Fatalf("ran %v, want %v", ran, want)
		}
	}
}

// At a fresh prompt there is no current line for a next one to be relative to,
// so `C-o` is an accept and nothing more.
//
// The row that pins the field's polarity as much as the behavior: an editor
// built as a bare literal — which is how every test in this package builds one
// — must start with nothing fetched, or a history entry would appear on every
// first prompt in the package.
func TestOperateAndGetNextAtAFreshPromptFetchesNothing(t *testing.T) {
	e := &editor{
		in: typing("ls\x0f"), out: &strings.Builder{},
		history: fourLines,
		width:   func() int { return 80 },
	}
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil || line != "ls" {
		t.Fatalf("first read gave %q (%v), want the typed line", line, err)
	}
	e.remember(line)
	e.in = typing("\r")
	line, err = e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if line != "" {
		t.Errorf("next prompt offered %q, want an empty line", line)
	}
}
