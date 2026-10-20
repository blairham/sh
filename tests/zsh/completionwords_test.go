// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A completion's `$words` is the command the cursor is in (#6174), and an
// alias in command position is expanded into it unless COMPLETE_ALIASES is
// set. Every row measured 2026-10-05 against zsh 5.9.2 through a
// pseudo-terminal, a `zle -C` widget writing `$words` and `$CURRENT`.
func TestTheWordsAreTheCommandBeingCompleted(t *testing.T) {
	for _, c := range []struct{ name, setup, line, want string }{
		{"a pipe", "", "echo a | grep -", "grep,-|2"},
		{"an and-list", "", "true && grep -", "grep,-|2"},
		{"a semicolon", "", "x; grep -", "grep,-|2"},
		{"a subshell", "", "(grep -", "grep,-|2"},
		{"a command substitution", "", "echo $(grep -", "grep,-|2"},
		{"a backquote", "", "echo `grep -", "grep,-|2"},
		{"a loop body", "", "for i in a b; do grep -", "grep,-|2"},
		{"a reserved word", "", "if grep -", "grep,-|2"},
		{"a brace", "", "{ grep -", "grep,-|2"},
		{"a bang", "", "! grep -", "grep,-|2"},
		{"an assignment", "", "x=1 grep -", "grep,-|2"},
		{"a redirection apart", "", "grep a > out -", "grep,a,-|3"},
		{"a redirection joined", "", "grep a 2>/dev/null -", "grep,a,-|3"},
		{"the whole line", "", "git ", "git,|2"},
		{"an alias", "alias ll='gls -h --x'\n", "ll -", "gls,-h,--x,-|4"},
		{"an alias after a separator", "alias ll='gls -h --x'\n", "x; ll -", "gls,-h,--x,-|4"},
		{"complete_aliases keeps it", "alias ll='gls -h --x'\nsetopt completealiases\n", "ll -", "ll,-|2"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := completionFor(t, c.setup+widgetOf(`compadd -QU -- "${(j:,:)words}|$CURRENT"`), c.line)
			if len(got) != 1 || got[0] != c.want {
				t.Errorf("%q: words %q, want %q", c.line, got, c.want)
			}
		})
	}
}
