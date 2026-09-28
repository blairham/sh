// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A backslash written inside **double quotes** in a `=~` operand is one of
// the two characters it was written with here, where the other columns take
// it off.
//
// It is not the same question as a backslash-quoted span: that one is
// #4976's, and this shell keeps the pair there too but for the engine to
// read rather than as a character. The row that separates them is the third
// group below.
//
// Measured 2026-09-28 against `/bin/ksh` `Version AJM 93u+ 2012-08-01`, `-c`
// under `env -i PATH=/usr/bin:/bin` with a scratch `HOME`. See
// interp.Semantics.RegexDoubleQuotedBackslashStands for the whole panel.
func TestADoubleQuotedBackslashStandsForItself(t *testing.T) {
	for _, c := range []struct {
		src    string
		status int
	}{
		// The control: a quoted run is still an expression here, so a `.`
		// inside quotes is live. That is what puts this column on a
		// different mechanism from bash, where the run would be text.
		{`[[ axb =~ "a.b" ]]`, 0},

		// The backslash is a character, so what reaches the engine is an
		// escaped one — and only a subject holding a backslash matches.
		{`[[ "a.b" =~ "a\.b" ]]`, 1},
		{`[[ 'a\.b' =~ "a\.b" ]]`, 0},
		{`[[ axb =~ "a\.b" ]]`, 1},
		{`[[ zadb =~ "za\db" ]]`, 1},
		{`[[ 'za\db' =~ "za\db" ]]`, 0},
		{`[[ za1b =~ "za\db" ]]`, 1},

		// **What stands behind it keeps its meaning.** A `+` quantifies the
		// backslash, so the pattern is one or more backslashes — it matches
		// neither `a+b` nor its own text, and it does match `a\b`. These are
		// the rows that say the pair is not two literal characters.
		{`[[ "a+b" =~ "a\+b" ]]`, 1},
		{`[[ 'a\+b' =~ "a\+b" ]]`, 1},
		{`[[ 'a\b' =~ "a\+b" ]]`, 0},
		{`[[ 'a\\b' =~ "a\+b" ]]`, 0},
		// And a `.` behind one is still any character.
		{`[[ 'a\xb' =~ "a\.b" ]]`, 0},

		// A **backslash-quoted** span is the other question, and it answers
		// the same before this change and after: the pair goes to the engine
		// and `\.` is a literal dot there.
		{`[[ axb =~ a\.b ]]`, 1},
		{`[[ "a.b" =~ a\.b ]]`, 0},
		{`[[ za1b =~ za\wb ]]`, 0},
	} {
		out, st := kshOut(t, c.src)
		if out != "" || st != c.status {
			t.Errorf("%s\n got %q at %d\nwant %q at %d", c.src, out, st, "", c.status)
		}
	}
}
