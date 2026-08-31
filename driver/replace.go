// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import "syscall"

// replaceProcess is `exec cmd`: this process stops being a shell and becomes
// the command.
//
// It lives in driver rather than in interp on purpose. interp is a library, and
// a Runner embedded in another program must not be able to replace that
// program with whatever a script named — so interp exposes the decision as a
// hook and provides no implementation. A binary that *is* a shell is the one
// place the call is correct, and this is that place.
//
// It returns only on failure. On success the image is gone and there is nothing
// left to return to, which is also why the caller reports an error from here as
// the exec having failed rather than as the command having exited badly.
//
// argv must include argv[0]; execve takes the name the command sees as part of
// its arguments, which is what makes `exec -a name cmd` expressible at all.
func replaceProcess(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env)
}
