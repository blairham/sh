// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A **backslash-quoted** character in a `=~` operand keeps its backslash into
// the engine here, where the other three columns take the backslash off.
//
// This is the wider fact that `\d` was the narrow case of: the operator
// reaches this shell's own regular expression library, and a written
// backslash belongs to that library rather than to quote removal. So a
// metacharacter behind one is text — `a\.b` is a literal dot — and a letter
// behind one can be a class — `za\wb` matches a digit.
//
// Measured 2026-09-28 against `/bin/ksh` `Version AJM 93u+ 2012-08-01`, `-c`
// under `env -i PATH=/usr/bin:/bin` with a scratch `HOME`. See
// interp.Semantics.RegexKeepsAWrittenBackslash for the other three columns,
// including BusyBox v1.37.0 at the digest internal/oracle pins.
//
// **Every row is a pair**, because keeping the backslash and dropping it
// agree on half of all subjects: a row asserting only that `a\.b` fails to
// match `axb` would pass for a shell that made the whole operand literal.
func TestAWrittenBackslashInARegexBelongsToTheEngine(t *testing.T) {
	for _, c := range []struct {
		src    string
		status int
	}{
		// The control: the operator reaches an engine at all, so a `.` with
		// no backslash on it is still a `.`.
		{`[[ axb =~ a.b ]]`, 0},

		// The backslash was protecting a metacharacter, so the pair the
		// engine gets is that character as text.
		{`[[ axb =~ a\.b ]]`, 1},
		{`[[ "a.b" =~ a\.b ]]`, 0},
		{`[[ ab =~ a\+b ]]`, 1},
		{`[[ "a+b" =~ a\+b ]]`, 0},
		{`[[ aab =~ a\{2\}b ]]`, 1},
		{`[[ "a{2}b" =~ a\{2\}b ]]`, 0},

		// The backslash was naming a class, so the pair the engine gets is
		// the class. The control one line down is what says the *letter* is
		// read when it stands bare, so these are the backslash's rows and
		// not `w`'s.
		{`[[ zawb =~ za\wb ]]`, 0},
		{`[[ zawb =~ zawb ]]`, 0},
		{`[[ za1b =~ za\wb ]]`, 0},
		{`[[ za1b =~ zawb ]]`, 1},
		{`[[ "za b" =~ za\sb ]]`, 0},
		{`[[ "za.b" =~ za\Wb ]]`, 0},

		// `\d` and `\D` keep RegexDigitClassEscape's reading, which is the
		// same reading this axis gives them. Both spellings, because that
		// axis is about the engine and is true of the variable one too.
		{`[[ za1b =~ za\db ]]`, 0},
		{`[[ zadb =~ za\db ]]`, 1},
		{`[[ zaXb =~ za\Db ]]`, 0},
		{`[[ za1b =~ za\Db ]]`, 1},
		{`r='za\db'; [[ za1b =~ $r ]]`, 0},
		{`r='za\db'; [[ zadb =~ $r ]]`, 1},

		// And a **double-quoted** operand is not this question. Both of
		// these answer the same before this axis existed and after it; the
		// second is a divergence of its own (#4977).
		{`[[ za1b =~ "za\db" ]]`, 1},
		{`[[ axb =~ "a\.b" ]]`, 1},
	} {
		out, st := kshOut(t, c.src)
		if out != "" || st != c.status {
			t.Errorf("%s\n got %q at %d\nwant %q at %d", c.src, out, st, "", c.status)
		}
	}
}
