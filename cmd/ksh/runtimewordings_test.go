// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// A trap body that fires inside `kill` is not `kill` speaking: its not-found
// is located the way a script's is. Measured 2026-10-04 on ksh93u+
// 2012-08-01 (#5722).
func TestATrapBodyFiredInsideABuiltinIsNotThatBuiltinsSpeech(t *testing.T) {
	out, errs, _ := runKshScript(t, "trap 'echo a\nnosuchcmd-xyz\ncd /nonexist' INT\necho two\nkill -INT $$\necho three\n")
	if want := "two\na\nthree\n"; out != want {
		t.Errorf("out %q, want %q", out, want)
	}
	if want := "s.sh: line 6: nosuchcmd-xyz: not found\ns.sh[7]: cd: /nonexist: [No such file or directory]\n"; errs != want {
		t.Errorf("errs %q, want %q", errs, want)
	}
}

// Two runtime wordings of this shell. Measured 2026-10-04 on ksh93u+
// 2012-08-01 under `-c` (#5722).
func TestPrintfCharConstantsAndDuplicationNumbers(t *testing.T) {
	for _, c := range []struct{ src, out, errs string }{
		{`printf '%d\n' "'AB"; echo st=$?`, "65\nst=1\n", "ksh: printf: warning: 'AB: invalid character constant\n"},
		{`printf '%d\n' "'"; echo st=$?`, "0\nst=1\n", "ksh: printf: warning: ': invalid character constant\n"},
		{`printf '%d\n' "'A"; echo st=$?`, "65\nst=0\n", ""},
		{`echo hi >&08; echo st=$?`, "st=1\n", "ksh: 08: cannot open [Bad file descriptor]\n"},
		{`echo hi >&9; echo st=$?`, "st=1\n", "ksh: 9: cannot open [Bad file descriptor]\n"},
	} {
		out, errs, _ := runKshArgs(t, "-c", c.src)
		if out != c.out || errs != c.errs {
			t.Errorf("%s:\n got %q, %q\nwant %q, %q", c.src, out, errs, c.out, c.errs)
		}
	}
}
