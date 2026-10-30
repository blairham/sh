// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A command behind a precommand modifier is traced as the command the scan
// left, with the modifier words and their options out of the line. Measured
// 2026-10-30 against zsh 5.9.2 (`/opt/homebrew/bin/zsh`) under `-f -c` and
// `env -i PATH=/usr/bin:/bin`, from an empty directory. This shell wrote the
// words as they were written (#6967).
//
// `builtin` and `exec` are read through and `command` stops the scan at the
// next word, so `command command true` keeps one. The name a program is
// started under is written as it reaches the program, `exec -a` and a dash
// included. A prefixed line's second prompt is asked of the command the scan
// left: `x=1 exec /usr/bin/true` is one prompt and `x=1 exec true` two.
// `command -v` is a lookup and not a modifier, so it stays; and `noglob` was
// already left out.
func TestAPrecommandModifierIsLeftOutOfTheTrace(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"command", `command print in`, "+zsh:1> print in\nzsh:1: command not found: print\n"},
		{"command -p", `command -p true`, "+zsh:1> true\n"},
		{"command --", `command -- true`, "+zsh:1> true\n"},
		{"builtin", `builtin true`, "+zsh:1> true\n"},
		{"exec", `exec /usr/bin/true`, "+zsh:1> /usr/bin/true\n"},
		{"builtin builtin", `builtin builtin true`, "+zsh:1> true\n"},
		{"command command", `command command true`, "+zsh:1> command true\n"},
		{"builtin command", `builtin command true`, "+zsh:1> true\n"},
		{"command builtin", `command builtin true`, "+zsh:1> builtin true\nzsh:1: command not found: builtin\n"},
		{"command exec", `command exec /bin/echo hi`, "+zsh:1> exec /bin/echo hi\nzsh:1: command not found: exec\n"},
		{"noglob builtin", `noglob builtin print a*`, "+zsh:1> print 'a*'\na*\n"},
		{"a modifier from an expansion", `c=command; $c true`, "+zsh:1> c=command \n+zsh:1> true\n"},
		{"exec -a", `exec -a zz /bin/echo hi`, "+zsh:1> zz hi\nhi\n"},
		{"exec -a an empty name", `exec -a '' /bin/echo hi`, "+zsh:1> '' hi\nhi\n"},
		{"exec -l", `exec -l /bin/echo hi`, "+zsh:1> -/bin/echo hi\nhi\n"},
		{"exec -l and -a", `exec -la zz /bin/echo hi`, "+zsh:1> zz hi\nhi\n"},
		{"exec -c", `exec -c /usr/bin/true`, "+zsh:1> /usr/bin/true\n"},
		{"exec -a to a builtin", `exec -a zz echo hi`, "+zsh:1> echo hi\nhi\n"},
		{"a dash", `- /bin/echo hi`, "+zsh:1> -/bin/echo hi\nhi\n"},
		{"a dash through command", `- command /bin/echo hi`, "+zsh:1> -/bin/echo hi\nhi\n"},
		// A prefixed line: the second prompt is the command's question.
		{"a prefix through command", `x=1 command true`, "+zsh:1> x=1 true\n"},
		{"a prefix through builtin", `x=1 builtin true`, "+zsh:1> x=1 +zsh:1> true\n"},
		{"a prefix through exec to a builtin", `x=1 exec true`, "+zsh:1> x=1 +zsh:1> true\n"},
		{"a prefix through exec to a program", `x=1 exec /usr/bin/true`, "+zsh:1> x=1 /usr/bin/true\n"},
		{"a prefix through builtin and command", `x=1 builtin command true`, "+zsh:1> x=1 true\n"},
		{"a prefix through exec -a", `x=1 exec -a zz /bin/echo hi`, "+zsh:1> x=1 zz hi\nhi\n"},
		// Controls.
		{"command -v", `command -v true`, "+zsh:1> command -v true\ntrue\n"},
		{"a word command does not take", `command -x true`, "+zsh:1> -x true\nzsh:1: command not found: -x\n"},
		{"noglob", `noglob print a*`, "+zsh:1> print 'a*'\na*\n"},
		{"no modifier", `print in`, "+zsh:1> print in\nin\n"},
		{"a prefix to a program", `x=1 /usr/bin/true`, "+zsh:1> x=1 /usr/bin/true\n"},
		{"a prefix to a builtin", `x=1 true`, "+zsh:1> x=1 +zsh:1> true\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZshOnPath(t, t.TempDir(), "set -x; "+c.src)
			if out != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}
