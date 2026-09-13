// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// What a backslash the input ends immediately after leaves in the field, per
// dialect (#2680).
//
// The lexer's own tests pin the three readings against a Dialect built by
// hand; this is what says each preset asks for the right one, and it asks
// through a *field* rather than through a span, so a reading that parses one
// way and expands another cannot pass.
//
// Measured 2026-09-13 over `-c` and over a script file with no trailing
// newline, which answer alike. A file that ends in a newline asks something
// else — the backslash is an ordinary line continuation then, and every
// column drops it.
//
//	written                bash 5.3  dash    ksh93   zsh     ash
//	printf "[%s]" x\       [x\]      [x\]    [x]     [x]     [x\]
//	printf "[%s]" \        [\]       [\]     [\]     []      [\]
//	printf "[%s][%s]" a \  [a][\]    [a][\]  [a][\]  [a][]   [a][\]
//
// bash 3.2 is the column with no dialect of its own: it drops the *word*
// along with the backslash — `[a]` on the third row, where zsh keeps the
// empty field — so a fourth reading nothing could ask for stays out of the
// vector. See docs/spec/semantics.md.
func TestEachDialectEndsAWordOnATrailingBackslash(t *testing.T) {
	for _, c := range []struct {
		name                    string
		src                     string
		bash, zsh, ksh, dashOut string
	}{
		{
			"inside a word", `printf "[%s]" x\`,
			`[x\]`, `[x]`, `[x]`, `[x\]`,
		},
		{
			// The row ksh93 parts from zsh on, and the one the issue's
			// nested backquote reduces to.
			"the whole word", `printf "[%s]" \`,
			`[\]`, `[]`, `[\]`, `[\]`,
		},
		{
			// Two conversions, so the empty field is visible as a field:
			// zsh prints `[a][]` where losing the word would print `[a]`.
			"the whole word, with the field made visible", `printf "[%s][%s]" a \`,
			`[a][\]`, `[a][]`, `[a][\]`, `[a][\]`,
		},
		{
			// The construct #2680 was filed for. The older substitution
			// unescapes `\\` to one backslash, so the inner parse meets the
			// same end of input — which is how a word rule became a refusal
			// of the whole line, at status 2, with the rest of the script
			// discarded.
			"a nested backquote substitution", "echo \"`echo \\\\`echo n\\\\``\"",
			"\\echo n\\\n", "echo n\\\n", "\\echo n\\\n", "\\echo n\\\n",
		},
		{
			// The control on the row above: an escaped backquote nests
			// without ever reaching the end of the input, so this row moves
			// with nothing and every dialect prints the inner result.
			"a nested backquote that ends properly", "echo \"`echo \\`echo n\\``\"",
			"n\n", "n\n", "n\n", "n\n",
		},
		{
			// The other control: `$( )` needs no escaping to nest and hands
			// its body on unchanged, so a change to the older form's
			// unescaping must leave both backslashes where they are.
			"the newer spelling is untouched", "echo \"$(echo \\`echo n\\`)\"",
			"`echo n`\n", "`echo n`\n", "`echo n`\n", "`echo n`\n",
		},
	} {
		for _, d := range []struct{ name, want string }{
			{"bash", c.bash}, {"zsh", c.zsh}, {"ksh", c.ksh}, {"dash", c.dashOut},
		} {
			p := presets[d.name]
			out, st, err := p.Combined(t, dialecttest.Base{}, c.src)
			if err != nil {
				t.Errorf("%s/%s: %v", c.name, d.name, err)
				continue
			}
			if st != 0 {
				t.Errorf("%s/%s: status %d, want 0 — no shell in the panel refuses", c.name, d.name, st)
			}
			if out != d.want {
				t.Errorf("%s/%s: got %q, want %q", c.name, d.name, out, d.want)
			}
		}
	}
}
