// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A builtin's failed write, and the one shell that words it on some routes
// and not others.
//
// Three of the panel report 1 and two of those say so; ksh93 reports 1 in
// silence. zsh keeps the builtin's status and — this is the part that was
// missing — *still says something*, unless the command that wrote is the one
// that closed the stream.
//
// Measured 2026-09-12 through `-c`, zsh 5.9.2, all of them status 0:
//
//	echo hi >&-                 silent
//	exec 1>&-; echo hi          zsh:1: write error: bad file descriptor
//	exec 1>&-; echo hi >&-      silent
//	{ echo a; echo b; } >&-     the sentence, twice
//	f(){ echo hi; }; f >&-      f: write error: bad file descriptor
//	exec 1>&-; echo hi 3>&-     the sentence
//
// So the line #1363 drew — `exec` against a per-command redirection — is not
// where the shell draws it. A close the writing command restates silences the
// sentence even after `exec` parked one, and a close written on a group or on
// a function call does not silence the commands inside it. What decides is
// whether *this command's own redirection list* closed the stream it writes
// to. See Runner.outputClosedByThisCommand.
//
// The wording is emitted before BuiltinWriteErrorFailsTheCommand is asked,
// which is why it is a second field rather than the existing one moved:
// BuiltinWriteError is reached only after that axis answers Yes, and moving
// it in front would make this shell speak on `echo hi >&-` too — the row #770
// recorded and which is right today.
func closedWriteSetup(sentence string) func(*Runner) {
	sem := permissive()
	// The answer that makes the routes tell each other apart: a failed write
	// that does not fail the command, which is the shell with the sentence.
	sem.BuiltinWriteErrorFailsTheCommand = No
	return func(r *Runner) {
		r.Semantics = &sem
		r.Diagnostics = &Diagnostics{InheritedClosedStreamWriteError: sentence}
	}
}

func TestAFailedWriteIsWordedOnlyWhereTheCommandDidNotCloseTheStream(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		says      int
	}{
		{"the command closed it itself", `echo hi >&-`, 0},
		{"the command restated a close `exec` had parked", `exec 1>&-; echo hi >&-`, 0},
		{"`exec` parked the close", `exec 1>&-; echo hi`, 1},
		{"a close on some other descriptor", `exec 1>&-; echo hi 3>&-`, 1},
		{"a group carried the close", `{ echo a; echo b; } >&-`, 2},
		{"the call carried the close", `f(){ echo hi; }; f >&-`, 1},
		{"once per write and not once per close", `exec 1>&-; echo a; echo b`, 2},
		{"a builtin that is not echo", `exec 1>&-; printf hi`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, closedWriteSetup("SAID\n"))
			if got := strings.Count(out, "SAID"); got != tc.says {
				t.Errorf("said it %d times, want %d — output %q", got, tc.says, out)
			}
			if st != 0 {
				t.Errorf("status %d, want 0: the wording must not move the status", st)
			}
		})
	}
}

// The controls, which are the rows a change that spoke for a stream merely
// *being* closed would get wrong. Neither of these has a failed write in it.
func TestAStreamMerelyBeingClosedSaysNothing(t *testing.T) {
	for _, src := range []string{`exec 1>&-; true`, `exec 1>&-; echo -n ""`} {
		out, st := run(t, src, closedWriteSetup("SAID\n"))
		if strings.Contains(out, "SAID") || st != 0 {
			t.Errorf("%s: got %q (status %d), want nothing at 0", src, out, st)
		}
	}
}

// Empty is the other four dialects, and it has to leave them exactly as they
// were: they word both routes the same way through BuiltinWriteError, so a
// second sentence here would be a second copy of it.
func TestTheOtherWordingIsUnmovedByThisOne(t *testing.T) {
	sem := permissive()
	sem.BuiltinWriteErrorFailsTheCommand = Yes
	for _, src := range []string{`echo hi >&-`, `exec 1>&-; echo hi`} {
		out, _ := run(t, src, func(r *Runner) {
			r.Semantics = &sem
			r.Diagnostics = &Diagnostics{BuiltinWriteError: "%[1]s: write failed"}
		})
		if got := strings.Count(out, "write failed"); got != 1 {
			t.Errorf("%s: said it %d times, want exactly 1 — output %q", src, got, out)
		}
	}
}
