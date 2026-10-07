// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// At a terminal, ^D in the first of two here-documents ends that document
// only: its warning is written, a continuation prompt is drawn for the second
// document's body, and a second ^D ends that one. The session then goes on.
//
// Measured 2026-10-07 through a pseudo-terminal against bash 5.3.20, `PS1='P>
// '`, no startup files: `cat <<E1 <<E2`, `a`, ^D, ^D, `echo still` draws
//
//	P> cat <<E1 <<E2
//	> a
//	> bash: warning: here-document at line 1 delimited by end-of-file (wanted `E1')
//	> bash: warning: here-document at line 2 delimited by end-of-file (wanted `E2')
//	P> echo still
//	still
//
// Here the first ^D wrote both warnings at once and ran the command (#6287).
// See interp.Semantics.EndOfInputEndsOneHeredocBody.
func TestControlDEndsOneHeredocBodyAtATime(t *testing.T) {
	control, screen := interruptSession(t, "")
	typeAndAwait := func(keys, mark string) {
		t.Helper()
		if _, err := control.WriteString(keys); err != nil {
			t.Fatalf("typing %q: %v", keys, err)
		}
		if err := screen.Await(mark, interruptBudget); err != nil {
			t.Fatalf("after %q: %v", keys, err)
		}
	}
	const (
		first  = "here-document at line 1 delimited by end-of-file (wanted `E1')"
		second = "here-document at line 2 delimited by end-of-file (wanted `E2')"
	)
	typeAndAwait("cat <<E1 <<E2\n", interruptPS2)
	typeAndAwait("a\n", interruptPS2)
	from := len(screen.Text())
	typeAndAwait("\x04", first)
	// And then a prompt for the second document, with nothing said about it
	// yet: the ^D that ends it has not been pressed.
	if err := screen.Await(interruptPS2, interruptBudget); err != nil {
		t.Fatalf("no continuation prompt for the second document: %v", err)
	}
	if drawn := screen.Text()[from:]; strings.Contains(drawn, "E2") || strings.Contains(drawn, interruptMark) {
		t.Fatalf("one ^D ended both documents:\n%s", smoke.Readable(drawn))
	}
	typeAndAwait("\x04", second)
	awaitInterruptPrompt(t, screen, "the second ^D")
	// The session goes on. The marker is put together by the shell, so the
	// echo of the line typed cannot answer for it.
	typeAndAwait("echo st''ill\n", "still")
}

// The same inside a command substitution that is still open: ^D ends the
// document, and what is typed after it is the rest of the substitution's
// program, which runs with it. Measured 2026-10-07 through a pseudo-terminal
// against bash 5.3.20: `v=$(cat <<E1`, `a`, ^D, `echo still`, `)` draws E1's
// warning, `> ` for each line after it, and leaves `a` and `still` in v.
// Here the ^D refused the substitution and the lines after it ran on their
// own (#6309).
func TestControlDEndsAHeredocInsideAnOpenSubstitution(t *testing.T) {
	control, screen := interruptSession(t, "")
	typeAndAwait := func(keys, mark string) {
		t.Helper()
		if _, err := control.WriteString(keys); err != nil {
			t.Fatalf("typing %q: %v", keys, err)
		}
		if err := screen.Await(mark, interruptBudget); err != nil {
			t.Fatalf("after %q: %v", keys, err)
		}
	}
	typeAndAwait("v=$(cat <<E1\n", interruptPS2)
	typeAndAwait("a\n", interruptPS2)
	typeAndAwait("\x04", "here-document at line 1 delimited by end-of-file (wanted `E1')")
	from := len(screen.Text())
	if err := screen.Await(interruptPS2, interruptBudget); err != nil {
		t.Fatalf("no continuation prompt for the rest of the substitution: %v", err)
	}
	typeAndAwait("echo still\n", interruptPS2)
	typeAndAwait(")\n", interruptMark)
	if drawn := screen.Text()[from:]; strings.Contains(drawn, "unexpected") {
		t.Fatalf("the substitution was refused:\n%s", smoke.Readable(drawn))
	}
	typeAndAwait(`printf 'r-%s.' $v`+"\n", "r-a.r-still.")
}
