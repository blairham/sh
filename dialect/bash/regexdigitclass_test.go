// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `=~` reads a written `\d` as the **letter** here, not as a digit class.
//
// POSIX EREs have no such escape and this operator reaches the C library's
// engine, so `\d` is `d`. ksh93 is the one column on the other side of it, and
// this test is the control that says so — see
// interp.Semantics.RegexDigitClassEscape, which is an axis rather than a rule
// precisely because the two columns part here.
//
// Measured 2026-09-27 against `/opt/homebrew/bin/bash` `GNU bash, version
// 5.3.20(1)-release`, `-c` under `env -i` with a scratch `HOME`. Each row is a
// pair: a class reading and a literal reading agree on half of all subjects.
func TestARegexDigitEscapeIsTheLetter(t *testing.T) {
	for _, c := range []struct {
		src    string
		status int
	}{
		// The control: the operator reaches an engine at all.
		{`[[ za1b =~ za[0-9]b ]]`, 0},
		{`[[ zadb =~ za[0-9]b ]]`, 1},

		// Written, and through a variable, which is the spelling that keeps
		// the shell's quote removal out of the question entirely.
		{`[[ zadb =~ za\db ]]`, 0},
		{`[[ za1b =~ za\db ]]`, 1},
		{`r='za\db'; [[ zadb =~ $r ]]`, 0},
		{`r='za\db'; [[ za1b =~ $r ]]`, 1},
		{`r='za\Db'; [[ za1b =~ $r ]]`, 1},
		{`r='za\Db'; [[ zadb =~ $r ]]`, 1},
	} {
		out, st := runBash(t, t.TempDir(), c.src)
		if out != "" || st != c.status {
			t.Errorf("%s\n got %q at %d\nwant %q at %d", c.src, out, st, "", c.status)
		}
	}
}
