// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// The case letters' listing and their fold are two pieces of state. Each line
// is followed by `typeset -p s; s=Qz; echo "[$s]"`, so a row shows both.
// Measured 2026-10-03 on ksh93u+ 2012-08-01 under `env -i … -c` (#5671). See
// interp.Semantics.CaseListingAndFoldAreSeparate.
func TestTheCaseListingAndFoldAreSeparate(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`s=Ab; typeset -lx s`, "typeset -x -l s=Ab\n[qz]\n"},
		{`s=Ab; typeset -lx s=Bc`, "typeset -x -l s=Bc\n[qz]\n"},
		{`typeset -E s=1; typeset -l s`, "typeset -l s=1\n[Qz]\n"},
		{`typeset -E s=1; typeset -u s`, "typeset -u s=1\n[Qz]\n"},
		{`typeset -E s=1; typeset +E s; typeset -l s`, "typeset -l s=1\n[qz]\n"},
		{`typeset -i s=1; typeset -l s`, "typeset -l s=1\n[Qz]\n"},
		{`typeset -i s=1; typeset -u s`, "typeset -u s=1\n[Qz]\n"},
		{`typeset -i s=1; typeset +i s; typeset -l s`, "typeset -l s=1\n[qz]\n"},
		{`typeset -il s=1`, "typeset -l -i s=1\n[0]\n"},
		{`typeset -il s=1; typeset +i s`, "typeset -l s=1\n[Qz]\n"},
		{`typeset -il s=1; typeset +i s; typeset -l s`, "typeset -l s=1\n[qz]\n"},
		{`typeset -il s=1; typeset +i s; typeset -u s`, "typeset -u s=1\n[QZ]\n"},
		{`typeset -il s=1; typeset +il s`, "s=1\n[Qz]\n"},
		{`typeset -il s=1; typeset +l s`, "typeset -i s=1\n[0]\n"},
		{`typeset -ix s=1+2`, "typeset -x -i s=3\n[0]\n"},
		{`typeset -l -x s=Bc`, "typeset -x -l s=Bc\n[qz]\n"},
		{`typeset -l s; typeset -ux s=Bc`, "typeset -x -u s=BC\n[QZ]\n"},
		{`typeset -l s; typeset -x s=Bc`, "typeset -x -l s=bc\n[qz]\n"},
		{`typeset -l s=a; typeset -ux s=Bc`, "typeset -x -u s=BC\n[QZ]\n"},
		{`typeset -l s=A; typeset +l -i s`, "s=a\n[qz]\n"},
		{`typeset -l s=A; typeset +l -i s=1+2`, "s=1+2\n[qz]\n"},
		{`typeset -l s=Bc`, "typeset -l s=bc\n[qz]\n"},
		{`typeset -l s=Bc; typeset -i +l s=4`, "typeset -l -i s=4\n[0]\n"},
		{`typeset -l s=Bc; typeset -i s`, "typeset -i s=0\n[0]\n"},
		{`typeset -l s=Bc; typeset -i s=4; typeset +i s`, "s=4\n[Qz]\n"},
		{`typeset -l s=Bc; typeset -lx s=Dd`, "typeset -x -l s=dd\n[qz]\n"},
		{`typeset -l s=Bc; typeset +l -E s`, "s=bc\n[qz]\n"},
		{`typeset -l s=Bc; typeset +l s`, "s=bc\n[Qz]\n"},
		{`typeset -l s=Bc; typeset +l s; s=XY; typeset -l s`, "typeset -l s=xy\n[qz]\n"},
		{`typeset -l s=Bc; typeset +l s; typeset -l s`, "typeset -l s=bc\n[qz]\n"},
		{`typeset -l s=Bc; typeset +l s; typeset -lx s=Dd`, "typeset -x -l s=Dd\n[qz]\n"},
		{`typeset -l s=Bc; typeset +u s`, "typeset -l s=bc\n[Qz]\n"},
		{`typeset -l s=Bc; typeset +u s; s=XY; typeset -l s`, "typeset -l s=XY\n[qz]\n"},
		{`typeset -l s=Bc; typeset +u s; s=XY; typeset -u s`, "typeset -u s=XY\n[QZ]\n"},
		{`typeset -l s=Bc; typeset +u s; typeset -l s=XY`, "typeset -l s=xy\n[qz]\n"},
		{`typeset -l s=Bc; typeset +u s; typeset -u s`, "typeset -u s=BC\n[QZ]\n"},
		{`typeset -l s=Bc; typeset +u s; typeset -ux s=Dd`, "typeset -x -u s=Dd\n[QZ]\n"},
		{`typeset -l s=Bc; typeset +u s; typeset s=XY`, "s=XY\n[Qz]\n"},
		{`typeset -lH s=Bc`, "typeset -H -l s=bc\n[qz]\n"},
		{`typeset -lt s=Bc`, "typeset -t -l s=bc\n[qz]\n"},
		{`typeset -lu s=Bc`, "s=BC\n[QZ]\n"},
		{`typeset -lu s=Bc; typeset +l s`, "typeset -u s=BC\n[Qz]\n"},
		{`typeset -lx s`, "typeset -x -l s\n[qz]\n"},
		{`typeset -lx s; s=Bc`, "typeset -x -l s=bc\n[qz]\n"},
		{`typeset -lx s=Bc`, "typeset -x -l s=Bc\n[qz]\n"},
		{`typeset -lx s=Bc t=Ef; typeset -p t`, "typeset -x -l t=Ef\ntypeset -x -l s=Bc\n[qz]\n"},
		{`typeset -lx s=Bc; typeset -lx s=Dd`, "typeset -x -l s=dd\n[qz]\n"},
		{`typeset -u s; typeset -l s; s=Ab`, "typeset -l s=ab\n[qz]\n"},
		{`typeset -u s=Ab; typeset -lx s=Bc`, "typeset -x -l s=bc\n[qz]\n"},
		{`typeset -u s=Ab; typeset -lx s=Bc t=Ef; typeset -p t`, "typeset -x -l t=Ef\ntypeset -x -l s=bc\n[qz]\n"},
		{`typeset -u s=Bc; typeset -iu s=4; typeset +i s`, "typeset -u s=4\n[Qz]\n"},
		{`typeset -u s=Bc; typeset -l s`, "typeset -l s=bc\n[qz]\n"},
		{`typeset -u s=Bc; typeset -lx s`, "typeset -x -l s=bc\n[qz]\n"},
		{`typeset -u s=Bc; typeset -lx s=Dd`, "typeset -x -l s=dd\n[qz]\n"},
		{`typeset -ul s`, "s\n[qz]\n"},
		{`typeset -ul s; s=Bc`, "s=bc\n[qz]\n"},
		{`typeset -ul s=Bc`, "s=bc\n[qz]\n"},
		{`typeset -ul s=Bc; typeset +l s`, "typeset -u s=bc\n[Qz]\n"},
		{`typeset -ul s=Bc; typeset +l s; s=Xy; typeset -l s`, "typeset -l s=xy\n[qz]\n"},
		{`typeset -ul s=Bc; typeset +l s; typeset -i s=3`, "typeset -i s=3\n[0]\n"},
		{`typeset -ul s=Bc; typeset +l s; typeset -lx s=Dd`, "typeset -x -l s=Dd\n[qz]\n"},
		{`typeset -ul s=Bc; typeset +l s; typeset -t s`, "typeset -t -u s=bc\n[Qz]\n"},
		{`typeset -ul s=Bc; typeset +l s; typeset -u s`, "typeset -u s=bc\n[QZ]\n"},
		{`typeset -ul s=Bc; typeset +l s; typeset -x s`, "typeset -x -u s=bc\n[Qz]\n"},
		{`typeset -ul s=Bc; typeset +l s; typeset +u s`, "s=bc\n[Qz]\n"},
		{`typeset -ul s=Bc; typeset +l s; typeset s=XY`, "s=XY\n[Qz]\n"},
		{`typeset -ul s=Bc; typeset +l s; unset s; s=Ab`, "s=Ab\n[Qz]\n"},
		{`typeset -ul s=Bc; typeset +u s`, "typeset -l s=bc\n[Qz]\n"},
		{`typeset -ulx s=Bc`, "typeset -x s=Bc\n[qz]\n"},
		{`typeset -ux s=Bc`, "typeset -x -u s=Bc\n[QZ]\n"},
		{`typeset -x -ul s=Bc`, "typeset -x s=Bc\n[qz]\n"},
		{`typeset -x s=Ab; typeset -lx s=Bc`, "typeset -x -l s=Bc\n[qz]\n"},
		{`typeset -x s=Bc; typeset -l s`, "typeset -x -l s=bc\n[qz]\n"},
		{`typeset -x s=Bc; typeset -l s=Dd`, "typeset -l s=dd\n[qz]\n"},
		{`typeset -xF2 s=1`, "typeset -x -F 2 s=1.00\n[0.00]\n"},
		{`typeset -xl s=Bc`, "typeset -x -l s=Bc\n[qz]\n"},
		{`typeset -xL4 s=ab`, "typeset -x -L 4 s='ab  '\n[Qz  ]\n"},
		{`typeset +l -i s=4`, "s=4\n[Qz]\n"},
		{`typeset +x -l s=Bc`, "s=Bc\n[Qz]\n"},
		// The record travels through a function's scope and a subshell.
		{`function f { typeset -ul s=Bc; typeset +l s; typeset -p s; }; f; s=Ab`, "typeset -u s=bc\ns=Ab\n[Qz]\n"},
		{`typeset -ul s=Bc; function f { typeset +l s; }; f`, "s=bc\n[qz]\n"},
		{`typeset -ul s=Bc; typeset +l s; (typeset -p s)`, "typeset -u s=bc\ntypeset -u s=bc\n[Qz]\n"},
	} {
		src := c.src + `; typeset -p s; s=Qz; echo "[$s]"`
		if out, st := runKshEmptyArray(t, src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}
