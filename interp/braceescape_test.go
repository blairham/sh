// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// What a backslash before `}` comes to inside a `${ }` written in double
// quotes (#1966).
//
// The syntax package pins the parse; these rows are the *bytes*, whole and
// exact, because the fault this fixes was an added character rather than a
// missing one — an assertion that looked for a substring passed while the
// result carried a stray backslash in front of it.
//
// Measured 2026-09-10, `u` unset, across dash, bash 5.3.15, that build as
// `sh`, bash 3.2.57, ksh93u+ and zsh 5.9.2. Five write `A}B`; bash 3.2 keeps
// the backslash and has no dialect here to answer for it. The neighboring
// rows — an opening brace, an ordinary character, the same text outside an
// expansion — are unanimous across all six, and they are what say the rule is
// the closing brace in an operand rather than backslashes at large. Core, not
// an axis: there is nothing here for a dialect to answer.
func TestABackslashBeforeAClosingBraceIsConsumedInAQuotedOperand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the brace is freed and the backslash goes", `printf "[%s]" "${u-A\}B}"`, `[A}B]`},
		{"the colon form too", `printf "[%s]" "${u:-A\}B}"`, `[A}B]`},
		{"and with an expansion after it", `w=W; printf "[%s]" "${u-A\}${w}B}"`, `[A}WB]`},

		// The neighbors that say what the rule is not.
		{"an opening brace keeps its backslash", `printf "[%s]" "${u-A\{B}"`, `[A\{B]`},
		{"so does an ordinary character", `printf "[%s]" "${u-A\qB}"`, `[A\qB]`},
		{"and so does an ordinary double-quoted run", `printf "[%s]" "A\}B"`, `[A\}B]`},

		// The escapes double quotes already had, unmoved.
		{"a dollar is still escaped", `v=VAL; printf "[%s]" "${u-A\$vB}"`, `[A$vB]`},
		{"a backslash is still escaped", `printf "[%s]" "${u-A\\B}"`, `[A\B]`},

		// A single quote is an ordinary character in a quoted operand, so it
		// protects nothing and the brace inside it is freed like any other.
		{"single quotes do not protect the brace", `printf "[%s]" "${u-A'\}'B}"`, `[A'}'B]`},

		// Unquoted the operand is an ordinary word, where a backslash already
		// escaped anything — the reading the quoted one had been missing.
		{"unquoted it was already right", `printf "[%s]" ${u-A\}B}`, `[A}B]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The same escape in the operands of a substitution, which is the route
// powerlevel10k takes and where this was found: it builds a run of
// `${NAME-<sep>\}` entries and re-reads the result under `${(e)}`, and a kept
// backslash makes the built text refuse to parse (#1927).
func TestABackslashBeforeAClosingBraceIsConsumedInASubstitutionsOperands(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the replacement half", `v=x; printf "[%s]" "${v/x/A\}B}"`, `[A}B]`},
		{"the replacement half of the global form", `v=x; printf "[%s]" "${v//x/A\}B}"`, `[A}B]`},
		{"unquoted, which was already right", `v=x; printf "[%s]" ${v/x/A\}B}`, `[A}B]`},

		// The pattern half takes the freed brace as a literal to match, which
		// is the row that says the escape reaches both operands.
		{"the pattern half matches a real brace", `w='a}b'; printf "[%s]" "${w/a\}b/Z}"`, `[Z]`},
		{"and a trim pattern does too", `w='a}b'; printf "[%s]" "${w%\}b}"`, `[a]`},

		// What the reduction of #1927 builds, spelled without zsh's flags so
		// the core can carry it: text made of `${…-…}` entries, then read
		// again. A kept backslash and the second read refuses the line.
		{
			"text built with the escape re-reads as an expansion",
			`A=1; u=x; s="${u/x/\${A-z\}}"; eval "r=$s"; printf "[%s]" "$r"`,
			`[1]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
