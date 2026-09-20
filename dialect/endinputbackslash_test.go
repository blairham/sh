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
//	written            bash 5.3  dash    ksh93   zsh    ash
//	printf "[%s]" x\   [x\]      [x\]    [x]     [x]    [x\]
//	printf "[%s]" \    [\]       [\]     [\]     []     [\]
//	printf "[%s]" a \  [a][\]    [a][\]  [a][\]  [a][]  [a][\]
//
// bash 3.2 is the column with no dialect of its own: it drops the *word*
// along with the backslash — `[a]` on the third row, where zsh keeps the
// empty field — so a fourth reading nothing could ask for stays out of the
// vector. That row only says so because it reuses one conversion:
// `printf "[%s][%s]" a \` is `[a][]` in bash 3.2 too, since a second
// conversion fills a missing operand with the empty string. The corpus
// carries it, so docs/spec/measurements.md has the bash 3.2 column.
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
			// One conversion reused, so an empty field is visible as a
			// field: zsh prints `[a][]` where losing the word prints `[a]`.
			// Written `printf "[%s][%s]" a \` the row measures nothing —
			// the second conversion prints `[]` for an operand that is not
			// there, so the two readings look alike.
			"the whole word, with the field made visible", `printf "[%s]" a \`,
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
		{
			// Depth three, because a rule that holds at depth two usually
			// breaks there: each layer doubles the backslashes in front of
			// the backquotes it has to survive, so the innermost pair is
			// written `\\\`` and the middle one `\``. Unanimous `n` in all
			// eight columns, and it parsed here before the fix too — which
			// is the point of the row: the fix is to the word rule the
			// *inner* parse runs into, and depth is not what decided it.
			"nested three deep", "echo \"`echo \\`echo \\\\\\`echo n\\\\\\`\\``\"",
			"n\n", "n\n", "n\n", "n\n",
		},
		{
			// The same depth in the newer spelling, which needs no escaping
			// at any depth. If a change to the older form's unescaping ever
			// reached shared code, this is the row that would move.
			"the newer spelling three deep", "echo \"$(echo $(echo $(echo n)))\"",
			"n\n", "n\n", "n\n", "n\n",
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

// An unbalanced nested backquote, whole line and status, per dialect (#2680).
//
// The snippet writes the nesting wrong: the escaped backquote opens an inner
// body nothing closes, so the text the outer substitution hands on runs out
// mid-substitution. A refusal is a legitimate answer — most of the panel
// gives one — but status, what reaches standard output and what reaches
// standard error move independently, and the panel puts the boundary of a
// substitution's parse failure in four different places. The corpus row
// `core/an-unbalanced-nested-backquote` records all three observations:
//
//	dash, BusyBox ash   2, nothing printed — the line is read before it is run
//	zsh 5.9.2           1, `A` already out
//	bash 5.3, 3.2       0, the complaint on stderr and `A`, an empty line, `B`
//	ksh93u+             0, and it prints `n` — no objection at all
//
// Three of those four are now ours by measurement rather than by default. The
// `bash` dialect scopes the failure to the word — it names the construct,
// expands the word to the empty string and carries the script on to `B` at 0
// — which is [interp.Semantics.SubstitutionParseErrorIsFatal] answered `No`
// there and `Yes` everywhere else (#2703). The other two answers are not this
// axis: ksh93 reads the text as nesting that closes and so never has a
// failure to place, and dash and ash refuse before `echo A` has run, which is
// a question about *when* the body is read — and is
// [syntax.Dialect.SubstitutionBodyRead], taken in #2857, which is what moved
// the dash row here from the wrong side of it.
//
// The wording is the half that is #2680's: the same end of input inside the
// same re-lexed body used to come back out as `input ends after a backslash`,
// which named a rule instead of the construct. A refusal whose cause really
// is the missing backquote has to keep saying so.
func TestAnUnbalancedNestedBackquoteNamesTheBackquote(t *testing.T) {
	// `echo A` runs first in three of these four, which is itself a fact
	// about where the failure happens: the older substitution's body is
	// unescaped and re-lexed when the word is *expanded*, not when the line
	// is parsed, so `syntax.Parse` accepts this text and the refusal arrives
	// mid-run.
	//
	// **dash is the fourth**, and its row carried `A` until #2857: that
	// column reads *both* substitution spellings with the line that holds
	// them, so the refusal arrives before the `echo A` in front of it runs,
	// and the corpus row has said `nothing printed` since it was recorded.
	// BusyBox ash is the same column and has no preset in this table.
	// See syntax.Dialect.SubstitutionBodyRead.
	//
	// The `bash` row is the one that carries on: the empty line is the word
	// that failed, and `B` is the statement after it.
	const src = "echo A; echo \"`echo \\`echo n`\"; echo B"
	for _, c := range []struct {
		name, want string
		status     int
	}{
		{"bash", "A\nbash: command substitution: line 1: unexpected EOF while looking for matching ``'\n\nB\n", 0},
		{"zsh", "A\nzsh:1: unmatched `\n", 1},
		{"ksh", "A\nksh: syntax error at line 1: ``' unmatched\n", 3},
		{"dash", "dash: 1: Syntax error: EOF in backquote substitution\n", 2},
	} {
		out, st, err := presets[c.name].Combined(t, dialecttest.Base{}, src)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if st != c.status {
			t.Errorf("%s: status %d, want %d", c.name, st, c.status)
		}
		if out != c.want {
			t.Errorf("%s: got %q, want %q", c.name, out, c.want)
		}
	}
}
