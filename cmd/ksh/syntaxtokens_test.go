// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// Where and at what this shell's parse refusals stand. Measured 2026-10-04
// on ksh93u+ 2012-08-01 under `-c` (#5722).
func TestParseRefusalTokensAndLinesAreKsh93s(t *testing.T) {
	for _, c := range []struct{ src, errs string }{
		// A comment the input runs out in ends a line of its own, for the
		// refusal worded as the end of the input.
		{"a=( #c", "ksh: syntax error at line 2: `end of file' unexpected\n"},
		{"a=( (#i)zz ); echo after", "ksh: syntax error at line 2: `end of file' unexpected\n"},
		{"a=( x", "ksh: syntax error at line 1: `end of file' unexpected\n"},
		{"if true #c", "ksh: syntax error at line 1: `if' unmatched\n"},
		// And it is where the token after a `(` behind a name then stands,
		// in a loop's list as well as a command's.
		{"echo (#i)", "ksh: syntax error at line 2: `(' unexpected\n"},
		{"for x in a (#i)zz; do :; done", "ksh: syntax error at line 2: `(' unexpected\n"},
		{"for x in a (\nb", "ksh: syntax error at line 2: `(' unexpected\n"},
		{"for x in a b (\n\nb", "ksh: syntax error at line 1: `(' unexpected\n"},
		{"for x in (\nb", "ksh: syntax error at line 1: `(' unexpected\n"},
		// The first refusal in a word keeps its own token.
		{`echo "${:-a}${%x}"`, "ksh: syntax error at line 1: `:' unexpected\n"},
		{`echo "${%x}${:-a}"`, "ksh: syntax error at line 1: `%' unexpected\n"},
		{`echo ${x,}${y;}`, "ksh: syntax error at line 1: `,' unexpected\n"},
	} {
		out, errs, _ := runKshArgs(t, "-c", c.src)
		if out != "" || errs != c.errs {
			t.Errorf("%q:\n got %q, %q\nwant nothing, %q", c.src, out, errs, c.errs)
		}
	}
}
