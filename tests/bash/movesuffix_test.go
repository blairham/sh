// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A duplication's `-` is a move only where it is written: one that arrives by
// expansion leaves a word that is not a descriptor (#6746). And with a
// `{name}` in front, or an empty word after a descriptor that is not the
// operator's own, the refusal names something other than the word (#6868).
// Measured 2026-10-16 on bash 5.3.20 under `-c`. See
// interp.Semantics.FdMoveSuffixMustBeWritten and
// interp.Diagnostics.FdVariableDuplicationTargetIsNotADescriptor.
func TestAMovesSuffixIsReadFromTheWrittenWord(t *testing.T) {
	for _, c := range []struct{ src, out, errs string }{
		{
			`x=9-; exec 9</dev/null; exec 4<&$x; echo st=$?; true <&9; echo "9:$?"`,
			"st=1\n9:0\n", "bash: line 1: 9-: ambiguous redirect\n",
		},
		{
			`exec 9</dev/null; true 4<&"9-"; echo st=$?`,
			"st=1\n", "bash: line 1: 9-: ambiguous redirect\n",
		},
		// The `-` written, however it is quoted, is a move: 9 is gone after.
		{
			`x=9; exec 9</dev/null; true 4<&$x-; true <&9; echo "9:$?"`,
			"9:1\n", "bash: line 1: 9: Bad file descriptor\n",
		},
		{
			`exec 9</dev/null; true 4<&9\-; true <&9; echo "9:$?"`,
			"9:1\n", "bash: line 1: 9: Bad file descriptor\n",
		},
		{
			`true {v}<&foo; echo st=$?`,
			"st=1\n", "bash: line 1: v: ambiguous redirect\n",
		},
		{
			`true {v}<&""; echo st=$?`,
			"st=1\n", "bash: redirection error: cannot duplicate fd: Bad file descriptor\nbash: line 1: : Bad file descriptor\n",
		},
		{
			`true 4<&""; true 0<&""; echo st=$?`,
			"st=1\n", "bash: line 1: 4: Bad file descriptor\nbash: line 1: \"\": Bad file descriptor\n",
		},
		{
			`true 4<&foo; echo st=$?`,
			"st=1\n", "bash: line 1: foo: ambiguous redirect\n",
		},
	} {
		out, errs, _ := runAlias(t, c.src)
		if out != c.out || errs != c.errs {
			t.Errorf("%s:\ngot  %q %q\nwant %q %q", c.src, out, errs, c.out, c.errs)
		}
	}
}
