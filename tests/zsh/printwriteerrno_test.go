// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"syscall"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// **Who wrote the redirection decides the sentence, and the errno does not.**
//
// The silence a command gets for closing its own output is not a rule about
// `EBADF`: it is a rule about whose redirection put fd 1 where it is. Measured
// 2026-09-29 on zsh 5.9 (aarch64-alpine-linux-musl, `alpine:3.20` with
// `apk add zsh`, script files run as `zsh s.zsh` with standard error kept
// apart) — Linux rather than this laptop because two of the three errnos need
// `/dev/full` and a fifo:
//
//	print foo > /dev/full        	`zsh:print:1: write error: no space left on device`
//	exec 1>/dev/full; print foo  	`zsh:print:2: …` **and** `zsh:2: write error: …`
//	print foo >&-                	nothing at all
//	exec 1>&-; print foo         	`zsh:2: write error: bad file descriptor`
//	print foo >&9, fifo unread   	`zsh:print:1: write error: broken pipe`
//	exec 1>&9; print foo, ditto  	`zsh:print:1: …` **and** `zsh:1: write error: …`
//
// Three errnos, and the second sentence — the one with no builtin in the
// location, which is this shell's InheritedClosedStreamWriteError — is absent
// on every row the command redirected for itself and present on every row it
// inherited. The errno moves the *first* sentence and the status, which is a
// different axis (BuiltinWriteErrorFailsTheCommand, still answered No here:
// the rows above where zsh names the builtin and leaves 1 are residue on
// #4436, not this).
//
// This was keyed on `EBADF` when it went in, and a grid of seven closed-stream
// shapes could not tell: every row in it had the same errno. The check is a
// pair that holds the ownership fixed and changes the errno, and a pair that
// holds the errno fixed and changes the ownership.
func TestTheSilenceIsForTheOwnerOfTheRedirectionNotTheErrno(t *testing.T) {
	const said = "zsh:1: write error: no space left on device\n"
	for _, tc := range []struct{ name, src, wantErr string }{
		// The command's own redirection, with an errno that is not EBADF:
		// silent, exactly as `>&-` is.
		{"its own redirection, out of space", "print foo 1>&1\n", ""},
		{"its own, written the other way", "print foo >&1\n", ""},
		// And the same failed write on a stream it did not redirect, which
		// is the row that keeps this from being "stop complaining".
		{"inherited, out of space", "print foo\n", said},
		{"inherited, echo for comparison", "echo foo\n", said},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs := runZshWithFullDisk(t, tc.src+"print -u2 -r -- \"st $?\"\n")
			if want := tc.wantErr + "st 0\n"; errs != want {
				t.Errorf("said %q, want %q", errs, want)
			}
		})
	}
}

// runZshWithFullDisk runs src with a standard output that refuses every write
// the way a full filesystem does, and answers with standard error alone.
//
// A writer rather than a file because `/dev/full` is Linux's and these tests
// run on both: what the grid above needs is an errno that is not EBADF on a
// stream the command redirected to itself, and `1>&1` aims the command's own
// redirection at the stream the runner already has.
func runZshWithFullDisk(t *testing.T, src string) string {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var e bytes.Buffer
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{
		Stdout: fullDisk{}, Stderr: &e, Semantics: &sem, Diagnostics: &diag,
		Dir: t.TempDir(), Name: "zsh", Dialect: presetDialect(),
	}
	zsh.Apply(r)
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return e.String()
}

// fullDisk is a standard output with no room left on it.
type fullDisk struct{}

func (fullDisk) Write(p []byte) (int, error) { return 0, syscall.ENOSPC }
