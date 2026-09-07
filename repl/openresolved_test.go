// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// An operator whose list closed without it is let go of, so the prompt stops
// naming it.
//
// This is the surface the mistake shows on and nowhere else: the parse
// succeeds either way and the tree is the same, and the only thing a stale
// entry changes is the word drawn at a continuation prompt. Measured on
// zsh 5.9.2 through a pty, PS2='[%_]':
//
//	% case x in x) : ||
//	[case cmdor]> ;;
//	[case]> esac
//	%
//
// The `cmdor` is gone once the `;;` arrives, because the arm's list closed and
// took the dangling `||` with it. A grammar that admits the dangling operator
// and forgets to let go of it draws `[case cmdor]` there, which names an
// operator the person typing has already resolved and cannot act on.
//
// The second arm is the half that says it is *let go of* rather than never
// recorded: with two arms dangling, zsh draws `case cmdor` once and not twice.
//
//	% case x in x) : ||
//	[case cmdor]> ;; y) : ||
//	[case cmdor]> esac
func TestAnOperatorItsListClosedWithoutIsLetGoOf(t *testing.T) {
	d := syntax.Core()
	d.OpenEndedAndOr = true
	for _, tc := range []struct {
		name, input, want string
	}{
		{
			"the arm closes and the operator goes with it",
			"case x in x) : ||\n;;\nesac\necho after\n",
			"<><case cmdor><case><><>",
		},
		{
			"a second dangling arm is drawn once, not twice",
			"case x in x) : ||\n;; y) : ||\n;;\nesac\n",
			"<><case cmdor><case cmdor><case><>",
		},
		// The control: an operator still waiting for its command is named,
		// which is what the rows above are the absence of.
		{
			"an operator still waiting is named",
			"case x in x) : ||\n:\n;;\nesac\n",
			"<><case cmdor><case><case><>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs strings.Builder
			r := newTestRunner(map[string]string{"PS1": "<%_>", "PS2": "<%_>"})
			r.Dialect = &d
			r.Stdout = &out
			s := Shell{
				Runner: r, In: readerFile(t, tc.input), Out: &out, Err: &errs,
				Dialect: d,
				Style: PromptStyle{
					Escape: '%',
					Codes:  map[rune]PromptField{'_': FieldOpenState},
					OpenWords: map[string]OpenWord{
						"case": {Text: "case"},
						"||":   {Text: "cmdor"},
						"&&":   {Text: "cmdand"},
					},
				},
			}
			if _, err := s.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			if got := errs.String(); got != tc.want {
				t.Errorf("prompts = %q, want %q", got, tc.want)
			}
		})
	}
}
