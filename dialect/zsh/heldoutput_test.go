// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **A builtin's output comes after its complaints on one stream**, its
// standard output being held until it returns (#5228). Measured 2026-10-01 on
// zsh 5.9.2 under `-c` with both streams on one pipe, which writes these lines
// byte for byte; the first `eval` row is the control, three builtins each
// writing theirs as they return, and the second hands a child the stream
// with what was held already written. See interp.Semantics.BuiltinOutputHeldUntilItReturns.
func TestABuiltinsOutputComesAfterItsComplaints(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `aliases[aa]=1 aliases[xx]=3; alias -m "a*" "[" "x*"
eval "print a; print -u2 b; print c"
eval 'print d; /bin/sh -c "echo e >&2"; print f'`)
	if want := "zsh:alias:1: bad pattern : [\naa=1\nxx=3\na\nb\nc\nd\ne\nf\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// **A child started inside a held builtin is handed the descriptor itself**,
// not the hold: `> f` on an `eval` is a file to the command inside it, as it
// is in zsh 5.9.2 (measured 2026-10-01, `notpipe`). Handed the hold, the
// child would be given a pipe this shell copies from.
func TestAChildInsideAHeldBuiltinWritesTheDescriptor(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `eval '/bin/sh -c "test -p /dev/stdout && echo pipe || echo notpipe"' > f; /bin/cat f`)
	if want := "notpipe\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
