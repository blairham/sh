// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"io/fs"
	"os"
	"syscall"

	"github.com/blairham/sh/internal/boundary"
)

// boundary is this shell's gate and sink, for the files the *front end* opens
// rather than the ones a script does.
//
// There are two of those and both were outside the boundary until now: the
// program named on the command line, and the startup files a session sources.
// Neither is the interpreter's own plumbing — a script operand is chosen by
// whoever invoked the shell and $ENV by a variable a line of script can set —
// so both are accesses a policy is entitled to refuse and an audit trail is
// entitled to see. They were latent while nothing supplied a gate, and #460
// stopped them being latent.
func (sh Shell) boundary() boundary.Boundary {
	return boundary.Boundary{Gate: sh.Gate, Events: sh.Events, Session: sh.Session}
}

// readFile is os.ReadFile through this shell's gate.
//
// A refusal comes back as a permission error rather than as a missing file,
// which is the opposite of what a denied *stat* answers, and the difference is
// what the caller can do about it. A hidden path answers a `test -f` the way
// an absent one does because the construct only wanted a yes or a no; a script
// the shell was told to run and may not read is a failure with nowhere to go,
// and saying "no such file" for it would send someone looking for a typo.
func (sh Shell) readFile(path string) ([]byte, error) {
	if !sh.boundary().Open(context.Background(), path, false) {
		return nil, &fs.PathError{Op: "open", Path: path, Err: syscall.EACCES}
	}
	return os.ReadFile(path)
}
