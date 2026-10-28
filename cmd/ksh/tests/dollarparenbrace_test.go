// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package tests_test

import "testing"

// A `}` inside a `$((` ends the construct there, with nothing in it, and the
// rest of the line is read from behind the brace: `$(( exec {v}</dev/null;
// echo $v ) | cat)` is `)' unexpected because a `)` is what comes next
// (#6918). Measured 2026-10-27 on ksh93u+ 2012-08-01 at /bin/ksh with `-c`,
// every row byte for byte with its status. See
// syntax.Dialect.CloseBraceEndsADollarDoubleParen.
func TestACloseBraceEndsADollarDoubleParen(t *testing.T) {
	const closer = "ksh: syntax error at line 1: `)' unexpected\n"
	for _, c := range []struct {
		src, out, errs string
		code           int
	}{
		{"echo $(( exec {v}</dev/null; echo $v ) | cat)", "", closer, 3},
		{"echo $(( echo }; fi ) | cat)", "", "ksh: syntax error at line 1: `fi' unexpected\n", 3},
		{"echo before; echo $(( } ))", "", closer, 3},
		{"echo $(( echo ${v:-{a}} ) | cat)", "", closer, 3},
		// Line 1 is whole once the brace has ended the construct, so it
		// runs before the closer on line 3 is refused.
		{"echo $(( echo }\n\n) | cat)", "\n", "ksh: syntax error at line 3: `)' unexpected\n", 3},
		{"echo $(( echo }", "\n", "", 0},
		{"echo $(( } fi", "fi\n", "", 0},
		{`x="$(( } ))"; echo "[$x]"`, "[ ))]\n", "", 0},
		// The quote before the brace goes with the construct, so the one
		// behind it opens a string that the end of the input closes.
		{`echo $(( echo "}" ) | cat); echo after`, " ) | cat); echo after\n", "", 0},
		{`x=$(( echo "}" ) | cat); echo "[$x]"`, "", "", 0},
		{"cat <<EOF\n$(( 1 } ))\nEOF", " ))\n", "", 0},
		// The controls: a shielded brace, one closing a `${`, one outside.
		{`echo $(( echo \} ) | cat)`, "}\n", "", 0},
		{`echo $(( echo '}' ) | cat)`, "}\n", "", 0},
		{"echo $(( echo `echo }` ) | cat)", "}\n", "", 0},
		{"echo $(( echo $(echo a}) ) | cat)", "a}\n", "", 0},
		{"echo $(( echo ${v}x ) | cat)", "x\n", "", 0},
		{"echo $(( echo ${ echo a;} ) | cat)", "a\n", "", 0},
		{`echo $(( echo "${v:-{a}}" ) | cat)`, "{a}\n", "", 0},
		{`echo $(( echo ${v:-"}"} ) | cat)`, "}\n", "", 0},
		{"echo $(( 1 )) }", "1 }\n", "", 0},
		{"echo $(( echo {a ) | cat)", "{a\n", "", 0},
		{"echo ${v:-{a}}", "{a}\n", "", 0},
	} {
		out, errs, code := runKshArgs(t, "-c", c.src)
		if out != c.out || errs != c.errs || code != c.code {
			t.Errorf("%q:\n got %q, %q at %d\nwant %q, %q at %d", c.src, out, errs, code, c.out, c.errs, c.code)
		}
	}
}
