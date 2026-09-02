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
func setUmask(mask int) (int, error) { return syscall.Umask(mask), nil }
