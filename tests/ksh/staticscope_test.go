// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// ksh93 scopes a `function`-word body statically: the name it declared is
// visible in it and nowhere else, so a function it calls reads the shell's own
// name. bash, zsh, dash and BusyBox ash all hand the caller's local down
// (#2865).
func TestAFunctionFormLocalIsNotHandedToTheCallee(t *testing.T) {
	if got := ksh.Semantics().CallerLocalsReachTheCallee; got != interp.No {
		t.Errorf("CallerLocalsReachTheCallee = %v, want No", got)
	}

	dir := t.TempDir()
	out, st := runKsh(t, dir, `function callee { printf '[%s]\n' "${v-UNSET}"; }
function caller { typeset v=local; callee; }
v=global
caller`)
	if st != 0 || out != "[global]\n" {
		t.Errorf("out %q status %d, want the shell's own name", out, st)
	}

	// The POSIX form is not this question and must not move with it: a
	// `typeset` there was never local, so the callee has nothing to be kept
	// from and the caller keeps the assignment after the call.
	out, st = runKsh(t, dir, `function callee { printf '[%s]\n' "${v-UNSET}"; }
caller() { typeset v=local; callee; }
v=global
caller
printf 'after [%s]\n' "$v"`)
	if st != 0 || out != "[local]\nafter [local]\n" {
		t.Errorf("out %q status %d, want the POSIX form to write the shell's own name", out, st)
	}

	// And the seal is a swap: a write in the callee is a write to the shell's
	// own name and outlives the call, while the caller's local is untouched.
	out, st = runKsh(t, dir, `function writes { v=written; }
function around { typeset v=local; writes; printf 'kept [%s]\n' "$v"; }
v=global
around
printf 'global [%s]\n' "$v"`)
	if st != 0 || out != "kept [local]\nglobal [written]\n" {
		t.Errorf("out %q status %d, want the write to land on the shell's own name", out, st)
	}
}
