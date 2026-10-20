// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A diagnostic writes the text it quotes with every character that cannot be
// printed made visible — `\t`, `\n`, `^A`, `^?`, `\M-^@` — whichever sentence
// it is and whatever the text is: a pattern, a parameter's message, a name
// (#6690). One row per sentence, because the rendering is the writer's and a
// sentence that bypassed it would show here and nowhere else. Every row was
// measured on zsh 5.9.2 under `-f -c` in a directory where nothing matches.
func TestADiagnosticWritesTheTextItQuotesVisibly(t *testing.T) {
	for _, tc := range []struct{ src, errs string }{
		{"print \"zz\tq\"*", "zsh:1: no matches found: zz\\tq*\n"},
		{`print $'zz\x01q'*`, "zsh:1: no matches found: zz^Aq*\n"},
		{"print \"zz\nq\"*", "zsh:1: no matches found: zz\\nq*\n"},
		{"print [ab](e:'true\nfalse':)", "zsh:1: no matches found: [ab](e:true\\nfalse:)\n"},
		{`print $'zz\x7fq'* `, "zsh:1: no matches found: zz^?q*\n"},
		{`print $'zz\x1bq'*`, "zsh:1: no matches found: zz^[q*\n"},
		{`print $'zz\x80q'*`, "zsh:1: no matches found: zz\\M-^@q*\n"},
		{`print "zz\\q"*`, "zsh:1: no matches found: zz\\q*\n"},
		{`/usr/bin/true $'zz\x01q'*`, "zsh:1: no matches found: zz^Aq*\n"},
		{`unset x; echo ${x?$'bo\x01om'}`, "zsh:1: x: bo^Aom\n"},
		{`print *($'\x01')`, "zsh:1: unknown file attribute: ^A\n"},
		// A newline an open group read on through, which is the shape #6727
		// was filed for: one line, the newline written `\n`.
		{"echo k(\n)\necho q", "zsh:1: unknown file attribute: \\n\n"},
		{"print -r -- x(|y\nz)\necho q", "zsh:1: no matches found: x(|y\\nz)\n"},
		{`setopt $'no\x01opt'`, "zsh:setopt:1: no such option: no^Aopt\n"},
		{`unsetopt $'no\x01opt'`, "zsh:unsetopt:1: no such option: no^Aopt\n"},
		{`typeset $'a\x01b'`, "zsh:typeset:1: not valid in this context: a^Ab\n"},
		{`unset $'a\x01b'`, "zsh:unset:1: a^Ab: invalid parameter name\n"},
		{`test $'a\x01' -eq 1`, "zsh:test:1: integer expression expected: a^A\n"},
		{`printf $'%\x01'`, "zsh:printf:1: %^A: invalid directive\n"},
		{`trap : $'SIGNO\x01'`, "zsh:trap:1: undefined signal: SIGNO^A\n"},
		{`builtin $'no\x01'`, "zsh:1: no such builtin: no^A\n"},
		{`source $'/no\x01pe'`, "zsh:source:1: no such file or directory: /no^Ape\n"},
		{`print x > /nonexistent/$'f\x01'`, "zsh:1: no such file or directory: /nonexistent/f^A\n"},
		// Already rendered before #6690, by their own sites; still once.
		{`cd $'/no\x01pe'`, "zsh:cd:1: no such file or directory: /no^Ape\n"},
		{`$'no\x01cmd'`, "zsh:1: command not found: no^Acmd\n"},
	} {
		_, _, errs := runZshSplit(t, t.TempDir(), tc.src)
		if errs != tc.errs {
			t.Errorf("%q:\n errs %q\nwant %q", tc.src, errs, tc.errs)
		}
	}
}
