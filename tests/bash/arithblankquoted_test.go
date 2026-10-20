// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A value that will not evaluate is quoted back with its own blanks — the
// newline `mapfile` keeps on a line, a space around an assigned value, the
// value a name holds. Measured 2026-10-16 on bash 5.3.20 under `-c`
// (#6709).
func TestAFailedValueIsQuotedWithItsBlanks(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			"typeset -i n=5; mapfile n <<< 1+",
			"bash: line 1: mapfile: 1+\n: arithmetic syntax error: operand expected (error token is \"+\n\")\n",
		},
		{
			"typeset -ia n; mapfile n <<< 2+",
			"bash: line 1: mapfile: 2+\n: arithmetic syntax error: operand expected (error token is \"+\n\")\n",
		},
		{
			"typeset -i n; n=' 1+ '",
			"bash: line 1: 1+ : arithmetic syntax error: operand expected (error token is \"+ \")\n",
		},
		{
			"typeset -i n; n=$'1+\\t'",
			"bash: line 1: 1+\t: arithmetic syntax error: operand expected (error token is \"+\t\")\n",
		},
		{
			"x=' 1/0 '; echo $((x))",
			"bash: line 1: 1/0 : division by 0 (error token is \"0 \")\n",
		},
		// The controls: `-t` takes the newline off before the store, and a
		// value written without blanks is quoted as it is.
		{
			"typeset -i n=5; mapfile -t n <<< 1+",
			"bash: line 1: mapfile: 1+: arithmetic syntax error: operand expected (error token is \"+\")\n",
		},
		{
			"x='3 4'; echo $((x))",
			"bash: line 1: 3 4: arithmetic syntax error in expression (error token is \"4\")\n",
		},
	} {
		out, _ := runBash(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
