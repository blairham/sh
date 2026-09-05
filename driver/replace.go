// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"runtime"
	"syscall"
)

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
//
// files is the descriptor table the replacement is to be given beyond the
// three named streams, laid out as interp lays every other half of this
// boundary out: entry i is descriptor 3+i, and a nil entry is a number that
// must not be open there. Placing it is process work of exactly the kind the
// exec itself is — the process's own descriptor table is being rewritten —
// which is why it happens here and not in the package that decided *which*
// descriptors those are.
// A descriptor that could not be placed does not stop the exec. The command
// runs with that number missing, which is what a shell does with every other
// descriptor it could not give it, and refusing to run at all would be a
// larger failure than the one being reported.
func replaceProcess(path string, argv, env []string, files []*os.File) error {
	placeFiles(files)
	err := syscall.Exec(path, argv, env)
	// Reached only when the exec failed, and here for the sake of the files
	// rather than the error: nothing refers to the slice after placeFiles, so
	// without this the collector may close a descriptor's original in the
	// window between placing it and the exec that was to inherit it.
	runtime.KeepAlive(files)
	return err
}
