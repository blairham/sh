// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// `=~` reads a written `\d` as a **digit class** here, and `\D` as its
// complement, where bash, zsh and BusyBox ash all read the letter.
//
// This shell's `=~` reaches its own regular expression library rather than
// the C library's, which is what the escape shows. Measured 2026-09-27
// against /bin/ksh `Version AJM 93u+ 2012-08-01`, `-c` under `env -i` with a
// scratch `HOME`; see interp.Semantics.RegexDigitClassEscape for the other
// three columns.
//
// **Every row is a pair**, because a class reading and a literal reading
// agree on half of all subjects: a row asserting only that `za\db` matches
// `za1b` would pass for a shell reading `\d` as the letter and handed a
// subject with a `d` in it.
func TestARegexDigitEscapeIsAClass(t *testing.T) {
	for _, c := range []struct {
		src    string
		status int
	}{
		// The control: the operator reaches an engine at all, so the rows
		// below it are the one escape and not the operator.
		{`[[ za1b =~ za[0-9]b ]]`, 0},
		{`[[ zadb =~ za[0-9]b ]]`, 1},

		// Written, which is the spelling #4932 reports.
		{`[[ za1b =~ za\db ]]`, 0},
		{`[[ zadb =~ za\db ]]`, 1},
		{`[[ zadb =~ za\Db ]]`, 0},
		{`[[ za1b =~ za\Db ]]`, 1},

		// And through a variable, which is **not** the same row: there the
		// shell's quote removal never sees the backslash, so this pair is
		// about the engine's reading alone.
		{`r='za\db'; [[ za1b =~ $r ]]`, 0},
		{`r='za\db'; [[ zadb =~ $r ]]`, 1},
		{`r='za\Db'; [[ zadb =~ $r ]]`, 0},
		{`r='za\Db'; [[ za1b =~ $r ]]`, 1},

		// A **double-quoted** operand is the two characters it was written
		// with and keeps the reading it had, which is measured rather than
		// tidy: `[[ za1b =~ "za\db" ]]` and `[[ zadb =~ "za\db" ]]` are both
		// 1 in ksh93u+, so neither subject is a match there. The first of
		// those agrees; the second is a divergence of its own about what a
		// double-quoted backslash means to this operator, and it answers the
		// same before and after.
		{`[[ za1b =~ "za\db" ]]`, 1},

		// The letter is not a class where no escape was written, which is
		// the row that says the reading is the backslash's.
		{`[[ zadb =~ zadb ]]`, 0},
		{`[[ za1b =~ zadb ]]`, 1},
	} {
		out, st := kshOut(t, c.src)
		if out != "" || st != c.status {
			t.Errorf("%s\n got %q at %d\nwant %q at %d", c.src, out, st, "", c.status)
		}
	}
}
