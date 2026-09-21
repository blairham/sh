// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import "testing"

// Where a **backquoted** body's refusal says it happened, across every preset
// a shipped binary runs under (#3553).
//
// #3354 gave a refused `$( … )` body the route its location names and left
// this row out; what it left out is a question about the *line*, and the panel
// splits four ways on it. Measured 2026-09-18 from a script file, `env -i
// PATH=/usr/bin:/bin LC_ALL=C <shell> s.sh` with stdin from /dev/null, in a
// fresh directory, over forty-four shapes. Take N as the file line the failure
// is on and B as the newlines inside the backquotes:
//
//	zsh 5.9.2     N
//	ksh93u+       N inside the sentence, the command's line in the prefix
//	dash 0.5.12   the body's own line, numbered from one
//	bash 5.3.20   N + B where the substitution is in the command's first
//	              token, N otherwise
//
// The runner used to place this refusal at the line the **command** began on
// in every column, which is dash's answer written without dash's numbering and
// nobody else's at all: “ v=`echo hi⏎for` “ opening line 2 was reported at
// line 2 where zsh writes 3 and bash writes 4. Two of the four columns move
// here; ksh93 already wrote both its numbers and dash already restarted.
//
// The three rows below hold the same body and the same opening line and bash
// answers two ways, which is what says its rule is a rule rather than an
// offset — `v=1 w=` ahead of the substitution is the control. See
// interp.Diagnostics.BackquotedSubstitutionFailureAddsItsBodysNewlines.
func TestABackquotedRefusalIsPlacedAtItsFailuresLine(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want map[string]string
	}{
		{
			name: "the substitution is the command's first token",
			src:  "printf 'start\\n'\nv=`echo hi\nfor`\n",
			want: map[string]string{
				"bash": "bash: command substitution: line 4: syntax error near unexpected token `newline'\n" +
					"bash: command substitution: line 4: `for'\n",
				"zsh": "zsh:3: parse error near `for'\n" +
					"zsh:2: parse error in command substitution\n",
				"ksh":   "ksh: line 2: syntax error at line 3: `for' unmatched\n",
				"dash":  "dash: 2: Syntax error: Bad for loop variable\n",
				"ash":   "ash: syntax error: bad for loop variable\n",
				"posix": "sh: syntax error: unterminated for\n",
			},
		},
		{
			// A command word ahead of it, and bash alone moves.
			name: "a command word ahead of it",
			src:  "printf 'start\\n'\nv=1 w=`echo hi\nfor`\n",
			want: map[string]string{
				"bash": "bash: command substitution: line 3: syntax error near unexpected token `newline'\n" +
					"bash: command substitution: line 3: `for'\n",
				"zsh": "zsh:3: parse error near `for'\n" +
					"zsh:2: parse error in command substitution\n",
				"ksh":   "ksh: line 2: syntax error at line 3: `for' unmatched\n",
				"dash":  "dash: 2: Syntax error: Bad for loop variable\n",
				"ash":   "ash: syntax error: bad for loop variable\n",
				"posix": "sh: syntax error: unterminated for\n",
			},
		},
		{
			// A three-line body failing on its second line: bash adds the
			// body's *whole* newline count, so 3 + 2 rather than 3 + 1. This
			// is the row that tells the three readings #3553 offered apart.
			name: "a three-line body failing on its second",
			src:  "printf 'start\\n'\nv=`echo hi\nfor\necho t`\n",
			want: map[string]string{
				"bash": "bash: command substitution: line 5: syntax error near unexpected token `newline'\n" +
					"bash: command substitution: line 5: `for'\n",
				"zsh": "zsh:4: parse error near `\\n'\n" +
					"zsh:2: parse error in command substitution\n",
				"ksh":   "ksh: line 2: syntax error at line 4: `newline' unexpected\n",
				"dash":  "dash: 3: Syntax error: Bad for loop variable\n",
				"ash":   "ash: syntax error: bad for loop variable\n",
				"posix": "sh: \"newline\" unexpected\n",
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for preset, want := range c.want {
				t.Run(preset, func(t *testing.T) {
					_, errs, _ := splitRunWithText(t, presets[preset], c.src)
					if errs != want {
						t.Errorf("wrote %q, want %q", errs, want)
					}
				})
			}
		})
	}
}
