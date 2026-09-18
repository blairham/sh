// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import (
	"context"

	"github.com/blairham/sh/interp"
)

// `newgrp` is `exec newgrp` under a builtin's name, and that is the whole of
// it — this shell's own manual says so, and it is why the word can be a
// *special* builtin at all: a command that replaces the shell has no status to
// hand back and nothing after it to run.
//
// Measured 2026-09-18 on AT&T ksh93u+ 2012-08-01, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input from /dev/null, against
// bash 5.3.20, zsh 5.9.2 and dash, where the word is an ordinary PATH hit:
//
//	printf start; newgrp nosuchgroupzz; printf ' after st=%s' "$?"
//	    ksh93u+   start, then `newgrp: nosuchgroupzz: bad group name`
//	              — and no `after` at all
//	    the rest  the same two, then ` after st=0`
//
//	type newgrp
//	    ksh93u+   newgrp is a special shell builtin
//	    the rest  newgrp is /usr/bin/newgrp
//
// **The diagnostic is the program's own**, which is the row that says what is
// happening: `newgrp -Z` there writes `newgrp: illegal option -- Z` and
// `usage: newgrp [-l] [group]`, neither of which is a shell's wording, and
// then the script is over. So #3316 read the ending as a special builtin's
// fatality and it is not one — the shell was **replaced**, and the program it
// was replaced with exited. Nothing in this file asks
// `BadOptionToSpecialBuiltinFatal`, because no option reaches the shell.
//
// What the special roster buys is the other two consequences a name on it
// gets: the sentence `type` and `command -V` write, and an assignment prefixed
// to it persisting. The third — a failure being fatal — is unreachable here by
// construction, and that is a fact about `exec` rather than about the roster.
//
// Registered through the `exec` builtin rather than written again, so that the
// descriptor table, the argv naming, the mask and the replacement hook are the
// ones `exec` already carries. A second copy of that road is exactly the shape
// this tree has been bitten by before.
func registerNewgrp(r *interp.Runner) {
	r.Register("newgrp", func(rr *interp.Runner, ctx context.Context, args []string) int {
		exec, ok := rr.Builtin("exec")
		if !ok {
			// Nothing to replace the shell with, which cannot happen in a
			// shell built from this package and is not worth a wording of
			// its own: the name falls back to what every other column does
			// with it.
			return 127
		}
		// `exec newgrp "$@"`, spelled as the operands that builtin reads.
		// A Builtin is handed the operands *without* the name it was called
		// by, so what goes in front is the word `newgrp` and nothing else —
		// and it is the word rather than a resolved path, so the lookup is
		// `exec`'s own against this runner's `PATH` and `Dir`. See
		// interp/lookpath.go for why that matters.
		return exec(rr, ctx, append([]string{"newgrp"}, args...))
	})
}
