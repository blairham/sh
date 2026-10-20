// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// Once **per operand**, the same answer the bash columns hold and the
// opposite of zsh's (#4556).
//
// Measured 2026-09-25 against `/bin/ksh` — Version AJM 93u+ 2012-08-01, `go
// version -m` on it says *not a Go executable* — with the snippet in a file
// of its own. The same file under bash 5.3.20 writes the same bytes, which is
// what says this is one answer held by two columns rather than two readings
// that happen to agree on the reduction.
func TestASublistFiresTheDebugTrapOncePerOperand(t *testing.T) {
	src := "trap 'echo \"T@$LINENO\"' DEBUG\n" +
		"echo x && echo y\n" +
		"false || echo z\n" +
		"echo w && echo v && echo u\n" +
		"echo q && { echo r; echo s; }\n" +
		"echo t\n"
	const want = "T@2\nx\nT@2\ny\nT@3\nT@3\nz\nT@4\nw\nT@4\nv\nT@4\nu\n" +
		"T@5\nq\nT@5\nr\nT@5\ns\nT@6\nt\n"
	out, st := runKsh(t, t.TempDir(), src)
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
}

func TestSublistDebugAxis(t *testing.T) {
	if got, want := ksh.Semantics().DebugTrapSublists, interp.DebugTrapSublistPerOperand; got != want {
		t.Errorf("DebugTrapSublists = %v, want %v", got, want)
	}
}
