// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
)

// The stop a continuation puts on a `$` is taken away by an unquoted pattern
// character earlier in the same word, which is this shell's own exception to
// the axis one file over (#3523).
//
// Measured 2026-09-18 on ksh93u+ 2012-08-01, `x=5`, each probe a script file
// of its own under `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin on
// /dev/null. The word is `<prefix>$\⏎x`, written as a `printf` operand.
//
// The three quoted spellings are the discrimination: `'*'` and `"*"` and a
// backslash-escaped `*` all keep the stop, so the character has to be
// unquoted, and a fix that looked for the byte alone would pass every other
// row here and fail these. `}` and `]` keep it too, which is what says the
// set is five characters rather than "the punctuation".
func TestAPatternCharacterUndoesTheStop(t *testing.T) {
	if !ksh.Dialect().PatternCharacterUndoesTheContinuationStop {
		t.Fatal("this dialect does not take the stop away for a pattern character")
	}
	for _, tc := range []struct{ prefix, want string }{
		// The stop holds.
		{`a`, `<a$x>`},
		{`!`, `<!$x>`},
		{`}`, `<}$x>`},
		{`]`, `<]$x>`},
		{`'*'`, `<*$x>`},
		{`"*"`, `<*$x>`},
		{`\*`, `<*$x>`},
		// And it is taken away.
		{`[`, `<[5>`},
		{`{`, `<{5>`},
		{`*`, `<*5>`},
		{`?`, `<?5>`},
		{`~`, `<~5>`},
		{`a[b]`, `<a[b]5>`},
	} {
		src := "x=5\nprintf \"<%s>\\n\" " + tc.prefix + "$\\\nx\n"
		out, st, err := preset.Combined(t, dialecttest.Base{}, src)
		if err != nil {
			t.Fatalf("%q: %v", tc.prefix, err)
		}
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("prefix %s wrote %q at %d, want %q at 0", tc.prefix, out, st, tc.want)
		}
	}
}
