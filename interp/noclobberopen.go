// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"io/fs"
	"os"
)

// `set -C` refuses to *overwrite a file*, and a character device is not one.
// This file is the whole of that distinction.
//
// The refusal is an exclusive create — `O_CREAT|O_EXCL`, which fails with
// EEXIST for anything already at the name — and that is one rule wider than
// the one being asked for. `2>/dev/null` names something that exists and
// cannot be overwritten by anybody, and a shell that answers it "file
// exists" has broken the most written line in shell.
//
// Measured 2026-09-10, `set -C; echo probe > TARGET`, against bash 5.3.15,
// bash 3.2.57, the same binary as `sh`, ksh93u+, dash and zsh 5.9.2, in a
// scratch directory holding an empty regular file, a directory, a unix
// socket, a fifo and a dangling symlink:
//
//	                        bash  bash32  sh  ksh93  dash  zsh
//	/dev/null                 ok      ok  ok     ok    ok   ok
//	/dev/zero, /dev/random    ok      ok  ok     ok    ok   ok
//	/dev/stdout, /dev/fd/1    ok      ok  ok     ok    ok   ok
//	a fifo with a reader      ok      ok  ok     ok    ok   ok
//	an empty regular file      –       –   –      –     –    –
//	a symlink to one           –       –   –      –     –    –
//	a symlink to /dev/null    ok      ok  ok     ok    ok   ok
//	a dangling symlink         –       –   –      –     –    –
//	a directory                –       –   –      –     –    –
//	a unix socket              –       –   –      –     –    –
//
// So the discriminator is not the path and not the `/dev` prefix — measured,
// `>/dev/stdout` is refused when standard output is a regular file, and a
// symlink is refused or allowed by what it points *at*. It is the type of
// the file already there: a **regular** file is what noclobber protects, and
// anything else is opened and whatever the open says stands.
//
// The last three rows are refusals that come from the open rather than from
// the option, and they are the reason this retries rather than testing the
// type up front: a directory, a socket and a dangling symlink cannot be
// opened for writing at all. Two wordings follow, and they are a dialect's:
// bash, ksh93 and dash report what the open said — `Is a directory`,
// `Operation not supported on socket` — where zsh reports its own refusal
// for all three. Diagnostics.NoclobberRefusalCoversAFailedOpen is that
// choice, and it is what keeps `>/dev/tty` in a session with no controlling
// terminal answering `file exists` the way zsh does: the device passes the
// type test, the open fails with ENXIO, and the refusal is reported instead.
//
// The second open creates nothing — the file is demonstrably there — and one
// dialect words its failure accordingly: dash says `cannot open d` here where
// it says `cannot create d` for the identical redirection with the option
// off, and ksh93 says `cannot create` either way. That is
// Diagnostics.NoclobberFallbackIsAnOpen, and it is a wording rather than an
// axis: the two shells do the same thing and describe it differently.
//
// Retrying rather than stat-and-open also keeps the exclusion doing the work
// it is there for. The create has to be the *exclusive* one or two shells
// racing for a new name would both believe they made it; only once EEXIST
// has come back is there a file to ask about, and only then is the type read.
func (r *Runner) openThroughNoclobber(ctx context.Context, a *Action, path string, flags int) (f *os.File, fellBack bool, err error) {
	f, err = r.openGated(ctx, a, path, flags)
	if flags&os.O_EXCL == 0 || !errors.Is(err, fs.ErrExist) {
		return f, false, err
	}
	// The name is taken, so there is something to ask about. Stat rather
	// than lstat, because a symlink is refused or allowed by what it reaches
	// — unanimous, and the same resolution every other open here uses. Through
	// the gate, like every other question this package asks about a path a
	// script named: a policy that hides the name answers as it does for one
	// that is not there, and the refusal the exclusive create already made
	// stands.
	st, serr := r.stat(path)
	if serr != nil || st.Mode().IsRegular() {
		return f, false, err
	}
	exists := err
	// Without the exclusion, and without the truncation either. A file that
	// is not regular has nothing to truncate, and dropping the flag means a
	// name that *became* a regular one between the two opens is not emptied
	// by a shell that had already decided it was a device.
	f, err = r.openGated(ctx, a, path, flags&^(os.O_EXCL|os.O_TRUNC|os.O_CREATE))
	if err != nil && !errors.Is(err, errRefused) && r.diag().NoclobberRefusalCoversAFailedOpen {
		// One dialect words every one of these as the option's own refusal,
		// so the second open's reason is dropped and the first one's stands.
		// A gate's refusal is never reworded: it is not the open's answer
		// and the script is not being told about a file.
		return f, true, exists
	}
	return f, true, err
}
