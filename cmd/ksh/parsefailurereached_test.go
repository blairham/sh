// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// A failure to read the rest of a script is located at the line of the last
// command the script ran, and the sentence names the line the reading gave
// out on. Measured 2026-10-04 on ksh93u+ 2012-08-01 over script files whose
// last line is an unclosed `(` (#5722). See
// interp.Diagnostics.ParseFailureIsLocatedWhereTheProgramGotTo.
func TestAParseFailureIsLocatedWhereTheScriptGotTo(t *testing.T) {
	for _, c := range []struct{ name, src, errs string }{
		{"the last command", "echo one\necho two\n(\n", "s.sh: line 2: syntax error at line 4: `(' unmatched\n"},
		{"an assignment counts", "echo one\nv=SET\necho \"${v-'$('}\"\n", "s.sh: line 2: syntax error at line 3: `'' unmatched\n"},
		{"a comment does not", "echo a\n# c\n(\n", "s.sh: syntax error at line 4: `(' unmatched\n"},
		{"line 1 is left out", "echo one\n(\n", "s.sh: syntax error at line 3: `(' unmatched\n"},
		{"inside an if", "if true; then\n:\nfi\n(\n", "s.sh: line 2: syntax error at line 5: `(' unmatched\n"},
		{"a call names its own line", "f() {\n:\n:\n}\n\nf\n\n(\n", "s.sh: line 6: syntax error at line 9: `(' unmatched\n"},
		{"a loop puts it back", "echo a\n:\nfor i in 1; do\n:\ndone\n(\n", "s.sh: line 2: syntax error at line 7: `(' unmatched\n"},
		{"a while loop too", "echo a\ni=0; while [ $i -lt 1 ]; do\n i=1\ndone\n(\n", "s.sh: line 2: syntax error at line 6: `(' unmatched\n"},
		{"the arithmetic loop's header counts", "echo a\n:\nfor ((i=0;i<1;i++)); do\n:\ndone\n(\n", "s.sh: line 3: syntax error at line 7: `(' unmatched\n"},
		{"a subshell never moved it", "echo a\n(\n:\n)\n(\n", "s.sh: syntax error at line 6: `(' unmatched\n"},
		{"a group did", "echo a\n{\n:\n}\n(\n", "s.sh: line 3: syntax error at line 6: `(' unmatched\n"},
	} {
		_, errs, code := runKshScript(t, c.src)
		if errs != c.errs || code != 3 {
			t.Errorf("%s: %q\n got %q at %d\nwant %q at 3", c.name, c.src, errs, code, c.errs)
		}
	}
}

// A `(` after a command's only word is blamed at the line of the token after
// it, where the word could have been a name. Measured 2026-10-04 on ksh93u+
// 2012-08-01 (#5722). See
// syntax.Dialect.ParenAfterANameIsRefusedWhereTheNextTokenStands.
func TestAParenAfterANameIsRefusedWhereTheNextTokenStands(t *testing.T) {
	for _, c := range []struct{ src, at string }{
		{"echo (\n\nx\n", "line 3"},
		{"\"echo\" (\n\nx\n", "line 3"},
		{"a-b (\n\nx\n", "line 3"},
		{"echo (#i)ab*\n", "line 2"},
		// The controls: a word holding an expansion, a second word, an
		// assignment, and a token on the same line.
		{"$x (\n\nx\n", "line 1"},
		{"echo a (\n\nx\n", "line 1"},
		{"a=b c (\n\nx\n", "line 1"},
		{"echo ( x\n\nx\n", "line 1"},
	} {
		_, errs, _ := runKshScript(t, c.src)
		want := "s.sh: syntax error at " + c.at + ": `(' unexpected\n"
		if errs != want {
			t.Errorf("%q: got %q, want %q", c.src, errs, want)
		}
		if strings.Contains(errs, "s.sh: line ") {
			t.Errorf("%q: located at a line although nothing ran: %q", c.src, errs)
		}
	}
}
