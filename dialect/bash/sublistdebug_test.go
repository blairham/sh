// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// An `&&`/`||` list fires the DEBUG trap once **per operand** here, which is
// the side of #4556 this column was already on and must stay on: zsh fires
// once for the whole list, and the change that gave zsh its answer had to
// leave these rows byte-identical.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/bash` — GNU bash, version
// 5.3.20(1)-release (aarch64-apple-darwin25.6.0), `go version -m` on it says
// *not a Go executable* — run `--noprofile --norc` with the snippet in a file
// of its own. The same file under ksh93 AJM 93u+ 2012-08-01 writes the same
// bytes, which is why both columns hold the same answer.
func TestASublistFiresTheDebugTrapOncePerOperand(t *testing.T) {
	src := "trap 'echo \"T@$LINENO\"' DEBUG\n" +
		"echo x && echo y\n" +
		"false || echo z\n" +
		"echo w && echo v && echo u\n" +
		"echo q && { echo r; echo s; }\n" +
		"echo t\n"
	const want = "T@2\nx\nT@2\ny\nT@3\nT@3\nz\nT@4\nw\nT@4\nv\nT@4\nu\n" +
		"T@5\nq\nT@5\nr\nT@5\ns\nT@6\nt\n"
	out, st := runBash(t, t.TempDir(), src)
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
}

// And the axis says so, so a column flipped by hand fails here as well as in
// the output above.
func TestSublistDebugAxis(t *testing.T) {
	if got, want := bash.Semantics().DebugTrapSublists, interp.DebugTrapSublistPerOperand; got != want {
		t.Errorf("DebugTrapSublists = %v, want %v", got, want)
	}
}
