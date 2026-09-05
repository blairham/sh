// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import "syscall"

// setUmask sets this process's file-creation mask and returns the one it
// replaced.
//
// It lives in driver rather than in interp for the reason dieBySignal and
// replaceProcess do: the mask is *process* state, shared by everything the
// process writes afterwards, and a Runner embedded in some other program must
// not change its host's mask on the say-so of the text it was handed. A binary
// that is a shell is the one place this is the right answer.
//
// syscall.Umask cannot fail — it has no error to return — but the hook is
// declared with one so a caller with a different idea of process state, a test
// double or a sandbox, has somewhere to put a refusal.
//
// It is outside the gate and the event stream, and that is a decision rather
// than an oversight — the same one rlimit.go records for the limits, and worth
// stating in both places because both look like omissions.
//
// The action vocabulary is the syscall-shaped surface where a *path or a
// program is named*: an open, a stat, a directory read, an exec. `umask` names
// neither. It does not perform an access; it changes the mode the next access
// would create with, and that next access is itself gated and recorded. A
// policy is asked about the file; the mask it lands with is the process's
// business, exactly as the process's uid is.
//
// And the seam an embedder needs already exists and is better than an event.
// This function is a *hook*, nil in a library and filled in only by a binary
// that is a shell — so a program that wants to see, log or refuse a mask
// change installs its own SetUmask, which is a decision point rather than a
// notification after the fact. A new event kind would be strictly weaker: it
// could report the change and never prevent it, which is the shape of promise
// ActionInherit is documented for not making.
func setUmask(mask int) (int, error) { return syscall.Umask(mask), nil }
