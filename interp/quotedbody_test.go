// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// What a `${ }` operand written inside double quotes comes to.
//
// The body is double-quoted *content* rather than a fresh unquoted word, so a
// single quote in it is an ordinary character: it quotes nothing, it is not
// removed, and what stands between two of them is still substituted. Measured
// 2026-09-07 and unanimous across bash 5.3.15, bash 3.2.57, that build invoked
// as `sh`, dash, ksh93u+ and zsh 5.9.2 — core rather than an axis, and there
// is nothing here for a dialect to answer.
//
// The syntax package pins the parse; these rows are the *values*, which is
// where the two readings that could not be told apart on a refusal separate:
// a shell that only scanned past the quotes to find the delimiter would give
// the text back, and all six give the substitution's result.
func TestASingleQuoteInAQuotedOperandIsAnOrdinaryCharacter(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a parameter between the quotes", `v=VAL; printf "[%s]" "${u:-'$v'}"`, `['VAL']`},
		{"and a command substitution", `printf "[%s]" "${u:-'$(echo hi)'}"`, `['hi']`},
		{"and the older spelling of one", "printf \"[%s]\" \"${u:-'`echo bq`'}\"", `['bq']`},
		{"an empty pair is two characters", `printf "[%s]" "${u:-''}"`, `['']`},
		{"a backslash still escapes", `v=VAL; printf "[%s]" "${u:-'\$v'}"`, `['$v']`},
		{"and the quotes do not split", `printf "[%s]" "${u:-'a b'}"`, `['a b']`},
		// The assigning operator takes the same word, and it is the value
		// that was assigned as well as the one substituted.
		{"the same word an assignment stores", `v=VAL; printf "[%s]" "${u='$v'}" "[$u]"`, `['VAL'][['VAL']]`},

		// Unquoted, the same characters are an ordinary single-quoted run.
		// This is the row that says the rule is the enclosing context's: a
		// reading that took the quotes literally everywhere passes every row
		// above and fails here.
		{"unquoted the quote is a quote again", `v=VAL; printf "[%s]" ${u:-'$v'}`, `[$v]`},

		// A double quote in the same position does not follow the rule: it
		// opens a run of its own and is removed.
		{"a double quote still quotes", `v=VAL; printf "[%s]" "${u:-"$v"}"`, `[VAL]`},
		{"and in the middle of the operand", `printf "[%s]" "${u:-x"y"z}"`, `[xyz]`},

		// Nor does a pattern operand, whose quotes quote and are removed.
		{"a pattern operand's quotes quote", `s=xay; printf "[%s]" "${s#'x'}"`, `[ay]`},
		{"so there is no substitution in one to leave open", `s=xay; printf "[%s]" "${s#'a$(b'}"`, `[xay]`},

		// A replacement operand is the third reading, and it is the one the
		// panel divides on rather than a consequence of this rule. Measured
		// 2026-09-07 with `s=xay; v=VAL`: `"${s/a/'$v'}"` is `x$vy` in bash
		// 5.3, that build as `sh` and ksh93 — the quotes quoting and removed
		// — and `x'VAL'y` in bash 3.2 and zsh, where they are characters.
		// dash has no operator. Pinned as the majority reading this
		// implementation already gave, so the split is recorded rather than
		// answered here; it wants a semantics axis of its own.
		{"a replacement operand's quotes quote", `s=xay; v=VAL; printf "[%s]" "${s/a/'$v'}"`, `[x$vy]`},
		{"and its pattern's do, unanimously", `s=xay; printf "[%s]" "${s/'a'/Z}"`, `[xZy]`},
		{"unquoted, all five agree with that", `s=xay; v=VAL; printf "[%s]" ${s/a/'$v'}`, `[x$vy]`},

		// And a here-document body is the same context by the other road.
		{"a heredoc body reads the operand alike", "v=VAL; cat <<EOF\n[${u:-'$v'}]\nEOF", "['VAL']\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
