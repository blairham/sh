// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A declaration whose own numeric letter evaluates a value that will not
// evaluate ends the script at 0, where a plain assignment to a typed name ends
// it at 1; an `eval` that catches either reports 1. Measured 2026-10-03 on zsh
// 5.9.2 under `-fc` (#5677). See
// interp.Semantics.DeclaredTypedValueFailureLeavesZero.
func TestATypedDeclarationThatWillNotEvaluateEndsAtZero(t *testing.T) {
	for _, c := range []struct {
		src, want string
		status    int
	}{
		{`integer x=1/0; print after`, "zsh:1: division by zero\n", 0},
		{`float x=1/0; print after`, "zsh:1: division by zero\n", 0},
		{`false; integer x=1/0; print after`, "zsh:1: division by zero\n", 0},
		{`typeset -i x=1/0; print after`, "zsh:1: division by zero\n", 0},
		{`integer x=a+; print after`, "zsh:1: bad math expression: operand expected at end of string\n", 0},
		{`integer x; x=1/0; print after`, "zsh:1: division by zero\n", 1},
		{`typeset -F x=1/0; print after`, "zsh:1: division by zero\n", 0},
		{`local -i x=1/0; print after`, "zsh:1: division by zero\n", 0},
		{`f() { integer x=1/0; print in; }; f; print after $?`, "f: division by zero\n", 0},
		{`f() { integer x=1/0; }; f; print after $?`, "f: division by zero\n", 0},
		{`integer x=1/0 y=2; print after`, "zsh:1: division by zero\n", 0},
		{`{ integer x=1/0; } always { print al $?; }; print after`, "zsh:1: division by zero\nal 0\n", 1},
		{`integer x=1/0 || print or; print after`, "zsh:1: division by zero\n", 0},
		{`(integer x=1/0); print sub $?`, "zsh:1: division by zero\nsub 0\n", 0},
		{`eval 'integer x=1/0'; print after $?`, "(eval):1: division by zero\nafter 1\n", 0},
		{`integer x=1/0`, "zsh:1: division by zero\n", 0},
		{`false; typeset -i x=1/0`, "zsh:1: division by zero\n", 0},
		{`integer -x x=1/0; print after`, "zsh:1: division by zero\n", 0},
		{`typeset -i x; typeset x=1/0; print after`, "zsh:1: division by zero\n", 1},
		{`integer x=1; x+=1/0; print after`, "zsh:1: division by zero\n", 1},
		{`integer x=1/0 2>/dev/null; print after`, "", 0},
		{`eval 'case $((1/0)) in esac'; print after $?`, "(eval):1: division by zero\nafter 1\n", 0},
		{`eval 'local x=$((1/0))'; print after $?`, "(eval):1: division by zero\nafter 1\n", 0},
		{`false; eval 'local x=$((1/0))'; print after $?`, "(eval):1: division by zero\nafter 1\n", 0},
		{`eval 'integer x=1/0; print in'; print after $?`, "(eval):1: division by zero\nafter 1\n", 0},
		{`eval 'x=1/0'; print after $?`, "after 0\n", 0},
		{`eval '(exit 3); local x=$((1/0))'; print after $?`, "(eval):1: division by zero\nafter 3\n", 0},
		{`setopt posixbuiltins; eval 'case $((1/0)) in esac'; print after $?`, "(eval):1: division by zero\nafter 1\n", 0},
		{`eval 'print ${x:?boom}'; print after $?`, "(eval):1: x: boom\n", 1},
		{`false; eval 'print ${x:?boom}'; print after $?`, "(eval):1: x: boom\n", 1},
	} {
		if out, st := runZsh(t, t.TempDir(), c.src); out != c.want || st != c.status {
			t.Errorf("%s\n got %q at %d, want %q at %d", c.src, out, st, c.want, c.status)
		}
	}
}
