// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// This shell's `=~` engine reads `\d` as a **digit class**, and that is
// visible only through a variable: a written backslash is taken off by quote
// removal before the engine sees it.
//
// The two spellings therefore part here, where in every other column they
// agree. `Semantics.RegexDigitClassEscape` is the engine's question and is
// Yes; `Semantics.RegexKeepsAWrittenBackslash` is quote removal's and is No.
//
// Measured 2026-09-28 against BusyBox **v1.37.0**, the `/bin/ash` of
// `alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b`
// on `linux/arm64` — the digest internal/oracle pins — each probe under
// `env -i PATH=/usr/bin:/bin`.
//
// **Every row is a pair**, because a class reading and a letter reading agree
// on half of all subjects.
func TestARegexDigitEscapeIsAClassOnlyThroughAVariable(t *testing.T) {
	for _, c := range []struct {
		src    string
		status int
	}{
		// The controls: the operator reaches an engine at all.
		{`[[ za1b =~ za[0-9]b ]]`, 0},
		{`[[ zadb =~ za[0-9]b ]]`, 1},

		// **Written** — the backslash is gone before the engine, so the
		// letter is what it reads.
		{`[[ za1b =~ za\db ]]`, 1},
		{`[[ zadb =~ za\db ]]`, 0},

		// **Through a variable** — nothing quoted the backslash, so the
		// engine gets `\d` and reads the class. This is the pair the two
		// spellings part on.
		{`r='za\db'; [[ za1b =~ $r ]]`, 0},
		{`r='za\db'; [[ zadb =~ $r ]]`, 1},
		{`r='za\Db'; [[ zaXb =~ $r ]]`, 0},
		{`r='za\Db'; [[ za1b =~ $r ]]`, 1},

		// The second control, and the sharp one: `\w` already reaches the
		// engine for every dialect, and this column already agreed on it. So
		// the engine here is not short of classes, and `d` and `D` were the
		// two being withheld from it.
		{`r='za\wb'; [[ za1b =~ $r ]]`, 0},

		// A **double-quoted** backslash survives quoting here too, so the
		// engine reads that class as well — the rows #4977's panel left
		// open for this column.
		{`[[ za1b =~ "za\db" ]]`, 0},
		{`[[ zadb =~ "za\db" ]]`, 1},
	} {
		out, st, err := preset.Combined(t, dialecttest.Base{
			Name: "ash", Dir: t.TempDir(),
		}, c.src+"\n")
		if err != nil {
			t.Fatalf("unsupported: %v", err)
		}
		if out != "" || st != c.status {
			t.Errorf("%s\n got %q at %d\nwant %q at %d", c.src, out, st, "", c.status)
		}
	}
}
