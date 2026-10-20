// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A minus option word after a plus one is another plus word, so every letter
// on the line comes off. Measured 2026-10-03 on ksh93u+ 2012-08-01 under
// `-c` (#5667). See interp.Semantics.EarlierPlusMakesALaterLetterARemoval.
func TestAnEarlierPlusMakesALaterLetterARemoval(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset +x -t s=Bc; typeset -p s`, "s=Bc\n"},
		{`typeset +i -t s=1+2; typeset -p s`, "s=1+2\n"},
		{`typeset -x s=Ab; typeset +t -x s=Bc; typeset -p s; /usr/bin/printenv s; echo $?`, "s=Bc\n1\n"},
		// Through the standing case letter before it comes off.
		{`typeset -l s=Ab; typeset +x -t s=Bc; typeset -p s`, "typeset -l s=bc\n"},
		{`typeset -l s=Ab; typeset +l -t s=Bc; typeset -p s`, "s=bc\n"},
		{`typeset -l s=A; typeset +l -x s=Bc; typeset -p s`, "s=bc\n"},
		// Valueless, over a name that is there.
		{`typeset -i s=1; typeset +x -t s; typeset -p s`, "typeset -i s=1\n"},
		// A plus letter takes its number like a minus one.
		{`typeset -x s=1; typeset +x -i 16 s=255; typeset -p s`, "s=255\n"},
		{`typeset +F 3 v=1.5; echo $?; typeset -p v`, "0\nv=1.5\n"},
		// The scan goes past the number to a minus word behind it.
		{`typeset -x s=1; typeset +i 16 -x s=255; typeset -p s`, "s=255\n"},
		// The other order adds both, as it did.
		{`typeset -t +x s=Bc; typeset -p s`, "typeset -x -t s=Bc\n"},
		{`typeset -t +i s=1+2; typeset -p s`, "typeset -t -i s=3\n"},
	} {
		if out, st := runKshEmptyArray(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}

// A line carrying a plus word declares nothing for a valueless operand over a
// name with no binding, in either order. Measured 2026-10-03 on ksh93u+
// 2012-08-01 under `-c` (#5667). See
// interp.Semantics.EarlierDeclarationLetterBlocksALaterPlus.
func TestAPlusWordLineDeclaresNoNewValuelessName(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -t +x s; typeset -p s; echo "[${s-unset}]"`, "[unset]\n"},
		{`typeset -i +x s; s=1+2; echo "[$s]"`, "[1+2]\n"},
		{`typeset -a +x s; typeset -p s; echo end`, "end\n"},
		{`function f { typeset -i +x s; s=1+1; echo "[$s]"; }; f`, "[1+1]\n"},
		// The controls: a value, and a name that is there.
		{`typeset -t +x s=1; typeset -p s`, "typeset -x -t s=1\n"},
		{`typeset -i s; typeset -t +x s; typeset -p s`, "typeset -x -t -i s\n"},
	} {
		if out, st := runKshEmptyArray(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}

// Both case letters on one line: the later one folds and the listing writes
// neither. Measured 2026-10-03 on ksh93u+ 2012-08-01 under `-c` (#5667). See
// interp.Semantics.TwoCaseLettersOnOneDeclarationCancel.
func TestTwoCaseLettersOnOneLineFoldByTheLaterAndListNeither(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -ul s=Bc; typeset -p s; s=Qz; echo "[$s]"`, "s=bc\n[qz]\n"},
		{`typeset -lu s=Bc; typeset -p s; s=Qz; echo "[$s]"`, "s=BC\n[QZ]\n"},
		{`typeset -u -l s=Bc; typeset -p s`, "s=bc\n"},
		{`typeset -l +u s=Bc; typeset -p s`, "s=BC\n"},
		{`typeset -ul s; typeset -p s`, "s\n"},
		// Across a function and a subshell, the record travels.
		{`typeset -ul s=Bc; function f { typeset -u s=x; }; f; typeset -p s`, "s=bc\n"},
		{`typeset -ul s=Bc; (typeset -p s)`, "s=bc\n"},
		// A single letter on a later line lists again, and two lines are
		// the later line's.
		{`typeset -ul s=Bc; typeset -l s; typeset -p s`, "typeset -l s=bc\n"},
		{`typeset -u s=a; typeset -l s=Bc; typeset -p s`, "typeset -l s=bc\n"},
	} {
		if out, st := runKshEmptyArray(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}
