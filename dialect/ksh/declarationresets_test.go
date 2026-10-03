// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A declaration that assigns resets the attributes the name carried, unless a
// case letter is already on it, and the declaration's own letters land
// afresh. Measured 2026-10-03 on ksh93u+ 2012-08-01 under `-c` (#5647). See
// interp.Semantics.DeclarationAssignmentResetsTheAttributes.
func TestADeclarationThatAssignsResetsTheAttributes(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -x s=1; typeset s=5; typeset -p s`, "s=5\n"},
		// Before the value is read, so the integer letter evaluates nothing.
		{`typeset -i s=1; typeset s=2+3; typeset -p s`, "s=2+3\n"},
		{`typeset -t s=1; typeset s=7; typeset -p s`, "s=7\n"},
		{`typeset -H s=1; typeset s=7; typeset -p s`, "s=7\n"},
		{`typeset -Z3 s=1; typeset s=7; typeset -p s`, "s=7\n"},
		{`typeset -L4 s=1; typeset s=7; typeset -p s`, "s=7\n"},
		{`typeset -F2 s=1; typeset s=7; typeset -p s`, "s=7\n"},
		{`typeset -E s=1; typeset s=7; typeset -p s`, "s=7\n"},
		{`typeset -i 16 s=1; typeset s=7; typeset -p s`, "s=7\n"},
		{`typeset -ix s=1; typeset s=7; typeset -p s`, "s=7\n"},
		// A case letter already on the name keeps every one of them.
		{`typeset -lx s=Ab; typeset s=7; typeset -p s`, "typeset -x -l s=7\n"},
		{`typeset -lt s=Ab; typeset s=7; typeset -p s`, "typeset -t -l s=7\n"},
		{`typeset -lx s=A; readonly s=B; typeset -p s`, "typeset -x -r -l s=b\n"},
		// One on this declaration does not.
		{`typeset -x s=1; typeset -l s=B; typeset -p s`, "typeset -l s=b\n"},
		{`typeset -Z3 s=1; typeset -l s=7; typeset -p s`, "typeset -l s=7\n"},
		// The declaration's own letters land, and only they.
		{`typeset -i s=1; typeset -r s=5; typeset -p s`, "typeset -r s=5\n"},
		{`typeset -Z3 s=1; typeset -i s=5; typeset -p s`, "typeset -i s=5\n"},
		{`typeset -ix s=1; typeset -x s=5; typeset -p s`, "typeset -x s=5\n"},
		{`typeset -i s=1; readonly s=2+3; typeset -p s`, "typeset -r s=2+3\n"},
		{`typeset -i s=1; export s=2+3; typeset -p s`, "typeset -x s=2+3\n"},
		{`typeset -Z3 s=1; export s=7; typeset -p s`, "typeset -x s=7\n"},
		// The kind stays.
		{`typeset -a s=(1 2); typeset s=7; typeset -p s`, "typeset -a s=(7 2)\n"},
		// A second declaration of a local resets; the one that made it has
		// nothing to reset.
		{`function f { typeset -i s=1; typeset s=2+3; typeset -p s; }; f`, "s=2+3\n"},
		{`typeset -x s=1; function f { typeset -x s=2; typeset s=3; /usr/bin/printenv s; echo $?; }; f; typeset -p s`, "1\ntypeset -x s=1\n"},
		// The controls: no value, and a plain assignment.
		{`typeset -i s=1; typeset s; typeset -p s`, "typeset -i s=1\n"},
		{`typeset -i s=1; s=2+3; typeset -p s`, "typeset -i s=5\n"},
	} {
		if out, st := runKshEmptyArray(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}

// A `-p` listing's operand carrying a value is declared with no letters and
// then listed: the scope a plain `typeset` takes, the reset a plain `typeset
// name=value` makes, and a bare assignment's store. Measured 2026-10-03 on
// ksh93u+ 2012-08-01 under `-c` (#5647). See
// interp.DeclarePrintOperandIsDeclaredWithoutLetters.
func TestAPrintOperandIsDeclaredWithoutLetters(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -x s=1; typeset -p s=5; typeset -p s`, "s=5\ns=5\n"},
		{`typeset -x s=1; typeset -p s=5 >/dev/null; /usr/bin/printenv s; echo $?`, "1\n"},
		{`typeset -i s=1; typeset -p s=2+3; typeset -p s`, "s=2+3\ns=2+3\n"},
		{`typeset -x s=1; typeset -p -i s=5; typeset -p s`, "s=5\ns=5\n"},
		{`typeset -lx s=A; typeset -p s=B`, "typeset -x -l s=b\n"},
		{`typeset -x s=1; typeset -p s=(a b); typeset -p s`, "typeset -a s=(a b)\ntypeset -a s=(a b)\n"},
		// A keyword function's local, starting empty, appended to or not.
		{`typeset -x s=1; function f { typeset -p s=5; }; f; typeset -p s`, "s=5\ntypeset -x s=1\n"},
		{`typeset -i s=1; function f { typeset -p s+=5 >/dev/null; echo "[$s]"; }; f; typeset -p s`, "[5]\ntypeset -i s=1\n"},
		// At the top level an append joins through the letters it keeps.
		{`typeset -i s=1; typeset -p s+=5; typeset -p s`, "typeset -i s=6\n"},
		{`typeset -Z3 s=1; typeset -p s+=5 >/dev/null; typeset -p s`, "typeset -Z 3 -R 3 s=015\n"},
		// A POSIX function has no scope to take.
		{`f() { typeset -p q=5 >/dev/null; }; f; typeset -p q`, "q=5\n"},
	} {
		if out, st := runKshEmptyArray(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}
