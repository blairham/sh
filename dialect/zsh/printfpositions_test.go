// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestPrintfReadsArgumentPositionsAPassAtATime is interp.PrintfPositionsPerPass.
// Every row measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`);
// they are the shapes zsh's own B03print asks about (#5143).
func TestPrintfReadsArgumentPositionsAPassAtATime(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`printf '%2$s %1$s\n' a b`, "b a\n"},
		{`printf '%2$d%1$d\n' 1 2 3 4`, "21\n43\n"},
		{`printf '%2$s %s %3$s\n' Morning Good World`, "Good Morning World\n"},
		{`printf '%1$s %s\n' a b`, "a a\nb b\n"},
		{`printf '%1$*2$d' 1 2 3 4 5 6 7 8 9 10; echo .`, " 1   3     5       7         9.\n"},
		{`printf '%3$.*1$d\n' 4 0 3`, "0003\n"},
		{`print -f '%*.*1$d\n' 1 2 3`, "2\n000\n"},
		{`printf '%1$0'"'+- #-08.5dx\n" 123`, "+00123  x\n"},
		{`printf '%s %*1$d|%s|\n' 5 7 8 9`, "5     7|8|\n9         0||\n"},
		{`printf '%.*2$s|%s|\n' abc 2 x`, "x||\n"},
		{`printf '%12$s' 1 2 3; echo " $?"`, "zsh:printf:1: 12: argument specifier out of range\n 1\n"},
		// The complaint reaches the reader first: what was written is still
		// in the builtin's buffer when it goes out.
		{`printf '%2$s\n' 1 2 3; echo " $?"`, "zsh:printf:1: 2: argument specifier out of range\n2\n 1\n"},
		{`printf '%1$s %2$s %1$s\n' a b c d e; echo " $?"`, "zsh:printf:1: 2: argument specifier out of range\na b a\nc d c\ne  1\n"},
		{`printf '%*0$d'; echo " $?"`, "zsh:printf:1: 0: argument specifier out of range\n 1\n"},
		{`printf '%0$s' a; echo " $?"`, "zsh:printf:1: %0$: invalid directive\n 1\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}

// TestPrintfIntoAnArrayTakesAnElementPerPass is
// interp.Semantics.PrintfVTakesAnElementPerPass, and `print -v` beside it.
// Same shell, same day.
func TestPrintfIntoAnArrayTakesAnElementPerPass(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -a foo; printf -v foo '%s' a b c; typeset -p foo`, "typeset -a foo=( a b c )\n"},
		{`typeset -a foo; printf -v foo '%s\n' a b; typeset -p foo`, "typeset -a foo=( $'a\\n' $'b\\n' )\n"},
		{`typeset -a foo; print -f '%2$d %4s' -v foo one 1 two 2 three 3; typeset -p foo`, "typeset -a foo=( '1  one' '2  two' '3 three' )\n"},
		// One use, a table and a scalar take the whole text.
		{`typeset -a foo; printf -v foo x; typeset -p foo`, "typeset foo=x\n"},
		{`typeset -A h; printf -v h '%s' a b; typeset -p h`, "typeset h=ab\n"},
		{`typeset -a foo; printf -v 'foo[2]' '%s' a b; typeset -p foo`, "typeset -a foo=( '' ab )\n"},
		// print -v.
		{`print -v x hello; print -r -- "[$x]"`, "[hello]\n"},
		{`print -v x -l a b; print -rn -- "[$x]"`, "[a\nb\n]"},
		{`print -v x -N a b; print -rn -- "[$x]"`, "[a\x00b]"},
		{`print -v x; print -rn -- "[$x]"`, "[]"},
		{`print -f '%s-%s' -v x a b c; print -r -- "[$x]"`, "[a-bc-]\n"},
		{`a=(1 2 3); print -v 'a[2]' q; print $a`, "1 q 3\n"},
		{`print -v x -u2 hi; print -r -- "[$x]"`, "zsh:print:1: -p or -u not allowed with -s, -S, -v, or -z\n[]\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}

// TestPrintExpandsTabs is `print -x n` and `print -X n`, and the two other
// B03print fronts beside them. Same shell, same day.
func TestPrintExpandsTabs(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`print -x4 $'\tone\ttwo\n\t\tx\ty'`, "    one\ttwo\n        x\ty\n"},
		{`print -X4 $'\tone\ttwo\n\t\tx\ty'`, "    one two\n        x   y\n"},
		{`print -X4 $'ab\tc\tdefg\th'`, "ab  c   defg    h\n"},
		{`print -x 4 $'\ta'`, "    a\n"},
		{`print -x4 a $'\tb'`, "a   b\n"},
		{`print -x4 $'a\tb'`, "a\tb\n"},
		{`print -x4 a $' \tc'`, "a  \tc\n"},
		{`print -x4 -l $'\ta' $'b\tc'`, "    a\nb\tc\n"},
		{`print -x4 -f '%s\n' $'\ta'`, "\ta\n"},
		{`print -x0 a; echo $?`, "zsh:print:1: positive integer expected after -x: 0\n1\n"},
		// `-m` with a format and nothing left writes nothing.
		{`print -m -f 'fmt\n' z a; echo $?`, "0\n"},
		{`print -m z a`, "\n"},
		// A NUL ends the directive the complaint names.
		{`printf $'%5\0d'; echo $?`, "zsh:printf:1: %5: invalid directive\n1\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}
