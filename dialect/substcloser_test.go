// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"
)

// The token a `$( … )` body's refusal names, across every preset a shipped
// binary runs under (#3296).
//
// A substitution's body is read on its own here, deliberately — the long note
// at the head of Runner.subst records that parsing it with the script's line
// was measured, costed and declined. The cost this pays is that the body's
// parse runs out of *input* where the script's own reader meets the `)` that
// closed the substitution, and each dialect has its own measured rule for
// what running out of input means. So the right rule answered the wrong
// question and the token named was one no reference writes.
//
// Measured 2026-09-16 from a script file, `env -i PATH=/usr/bin:/bin LC_ALL=C
// <shell> case.sh` with stdin from /dev/null, in a fresh directory, over ten
// bodies of the form `v=$(echo hi; X)`. BusyBox v1.37.0 in the digest-pinned
// Alpine image internal/oracle reaches, under `--init`.
//
//	bash 5.3.20   names `)` in all ten
//	bash 3.2.57   a third answer: `newline`, tagged `command substitution:`
//	zsh 5.9.2     names `)` in six of the eight it refuses
//	ksh93u+       names `)` in eight; `case` and `(` consume the closer
//	dash, ash     name `)` in **nine** — `for` alone is their own sentence
//
// So this is not a dialect disagreement and it is not asked as one. It is a
// fact about where the body ends, and every column's own rule produces the
// right token once the parser is shown the closer — dash's included, which
// judges whatever stands in a loop variable's place as a name and so keeps
// `Bad for loop variable` for `for` while gaining the closer for the rest.
//
// **The `for` row is why the issue's premise was narrower than the bug.** It
// recorded dash and BusyBox ash as already byte-perfect, measuring the single
// construct the whole panel answers specially; on the other nine they were
// wrong, and `dash/` and `ash/substitution-closer.tests` are what now hold
// them.
//
// What this does *not* fix is the echoed second line: bash and zsh quote the
// offending text after the sentence, and the text they quote is the
// **script's** line, not the body — `q=1; v=$(echo hi; for); z=2` entire,
// measured. That text lives in the driver and never reaches this package, so
// it is the tree-keeping change Runner.subst declines rather than a wording,
// and it is filed with the panel.
func TestASubstitutionRefusalNamesTheCloser(t *testing.T) {
	for _, c := range []struct {
		body string
		want map[string]string
	}{
		{
			// The construct the issue was filed from, and the one where the
			// panel splits: dash and BusyBox judge the closer as a loop
			// variable's name rather than as a token, so they keep their own
			// sentence while the other three name the `)`.
			body: "for",
			want: map[string]string{
				"bash":  "bash: line 2: syntax error near unexpected token `)'\n",
				"zsh":   "zsh:2: parse error near `)'\n",
				"ksh":   "ksh: line 2: syntax error at line 2: `)' unexpected\n",
				"dash":  "dash: 2: Syntax error: Bad for loop variable\n",
				"ash":   "ash: syntax error: bad for loop variable\n",
				"posix": "sh: \")\" unexpected\n",
			},
		},
		{
			// Every other construct, where all five name the closer. `if` is
			// the one the three suite files grade, because it is the shape
			// that moves in dash and BusyBox as well as in ksh93.
			body: "if",
			want: map[string]string{
				"bash":  "bash: line 2: syntax error near unexpected token `)'\n",
				"zsh":   "zsh:2: parse error near `)'\n",
				"ksh":   "ksh: line 2: syntax error at line 2: `)' unexpected\n",
				"dash":  "dash: 2: Syntax error: \")\" unexpected\n",
				"ash":   "ash: syntax error: unexpected \")\"\n",
				"posix": "sh: \")\" unexpected\n",
			},
		},
		{
			body: "{",
			want: map[string]string{
				"bash":  "bash: line 2: syntax error near unexpected token `)'\n",
				"zsh":   "zsh:2: parse error near `)'\n",
				"ksh":   "ksh: line 2: syntax error at line 2: `)' unexpected\n",
				"dash":  "dash: 2: Syntax error: \")\" unexpected\n",
				"ash":   "ash: syntax error: unexpected \")\"\n",
				"posix": "sh: \")\" unexpected\n",
			},
		},
		{
			body: "f()",
			want: map[string]string{
				"bash":  "bash: line 2: syntax error near unexpected token `)'\n",
				"zsh":   "zsh:2: parse error near `)'\n",
				"ksh":   "ksh: line 2: syntax error at line 2: `)' unexpected\n",
				"dash":  "dash: 2: Syntax error: \")\" unexpected\n",
				"ash":   "ash: syntax error: unexpected \")\"\n",
				"posix": "sh: \")\" unexpected\n",
			},
		},
		{
			// An operator left wanting rather than a compound left open,
			// which is a different road to the same place: the body ends
			// where a command was due.
			body: "echo x |",
			want: map[string]string{
				"bash":  "bash: line 2: syntax error near unexpected token `)'\n",
				"zsh":   "zsh:2: parse error near `)'\n",
				"ksh":   "ksh: line 2: syntax error at line 2: `)' unexpected\n",
				"dash":  "dash: 2: Syntax error: \")\" unexpected\n",
				"ash":   "ash: syntax error: unexpected \")\"\n",
				"posix": "sh: \")\" unexpected\n",
			},
		},
		{
			body: "echo x &&",
			want: map[string]string{
				"bash":  "bash: line 2: syntax error near unexpected token `)'\n",
				"zsh":   "zsh:2: parse error near `)'\n",
				"ksh":   "ksh: line 2: syntax error at line 2: `)' unexpected\n",
				"dash":  "dash: 2: Syntax error: \")\" unexpected\n",
				"ash":   "ash: syntax error: unexpected \")\"\n",
				"posix": "sh: \")\" unexpected\n",
			},
		},
		{
			// **The control.** A token the body's own grammar refuses before
			// the closer is ever reached: the first refusal stands and the
			// closer changes nothing. Without this row, a change that named
			// the closer for *every* refused body would pass everything
			// above.
			//
			// zsh and ksh are empty here because this engine accepts `echo
			// hi; ;` in both — real zsh accepts it too and ksh93 refuses it
			// at 3, which is a gap of its own and filed. What the row pins is
			// that this change did not move any of the six.
			body: ";",
			want: map[string]string{
				"bash":  "bash: line 2: syntax error near unexpected token `;'\n",
				"zsh":   "",
				"ksh":   "",
				"dash":  "dash: 2: Syntax error: \";\" unexpected\n",
				"ash":   "ash: syntax error: unexpected \";\"\n",
				"posix": "sh: \";\" unexpected\n",
			},
		},
	} {
		t.Run(c.body, func(t *testing.T) {
			src := "printf 'start\\n'\nv=$(echo hi; " + c.body + ")\n"
			for preset, want := range c.want {
				t.Run(preset, func(t *testing.T) {
					out, errs, _ := splitRun(t, presets[preset], src)
					if errs != want {
						t.Errorf("wrote %q, want %q", errs, want)
					}
					if !strings.HasPrefix(out, "start\n") {
						t.Errorf("standard output was %q; the line before the substitution has to have run", out)
					}
				})
			}
		})
	}
}

// The older spelling is left where it was, and this is what says so.
//
// “ v=`echo hi; for` “ ends at its backquote rather than where its contents
// end — see syntax.Span.Backquoted — and the columns read it with the script,
// which is why Diagnostics.BackquotedSubstitutionRestartsLines exists one
// message over. Measured 2026-09-16 the same way: bash 5.3.20 writes
// `command substitution: line 2: syntax error near unexpected token
// `newline'`, bash 3.2.57 the same, zsh `parse error near `for'` and ksh93
// “ `for' unmatched “. All four are what this shell already wrote, so the
// closer is **not** offered to a backquoted body.
//
// The row is not decoration: appending a backquote to the body instead would
// have changed every one of these, and appending the parenthesis to a body
// that ends at a backquote would have named a token the text does not hold.
func TestABackquotedRefusalIsLeftAtItsOwnToken(t *testing.T) {
	src := "printf 'start\\n'\nv=`echo hi; for`\n"
	for preset, want := range map[string]string{
		"bash":  "bash: command substitution: line 2: syntax error near unexpected token `newline'\n",
		"zsh":   "zsh:2: parse error near `for'\n",
		"ksh":   "ksh: line 2: syntax error at line 2: `for' unmatched\n",
		"dash":  "dash: 2: Syntax error: Bad for loop variable\n",
		"ash":   "ash: syntax error: bad for loop variable\n",
		"posix": "sh: syntax error: unterminated for\n",
	} {
		t.Run(preset, func(t *testing.T) {
			_, errs, _ := splitRun(t, presets[preset], src)
			if errs != want {
				t.Errorf("wrote %q, want %q", errs, want)
			}
		})
	}
}
