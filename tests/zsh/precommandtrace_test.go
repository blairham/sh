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
		// With no PATH to look in: macOS has a /usr/bin/command and Linux
		// does not, so the outcome after the line would be the platform's.
		{
			"command command", `PATH=/nonexistent; command command true`,
			"+zsh:1> PATH=/nonexistent \n+zsh:1> command true\nzsh:1: command not found: command\n",
		},
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

// A modifier that leaves nothing to run writes no trace line: `exec` with only
// redirections has no command behind it, and a modifier that refuses what is
// behind it — `builtin` naming no builtin, `exec` with options it cannot read
// — runs nothing. With a prefix in front, the prefix is written with no
// newline and the next line finishes it, and a refused one writes no prefix
// either. Measured 2026-11-02 against zsh 5.9.2 (`/opt/homebrew/bin/zsh`)
// under `-f -c` and `env -i PATH=/usr/bin:/bin`, from an empty directory.
// This shell wrote a line for each: `+zsh:1> ` with nothing after it, or the
// words behind the modifier (#7097).
func TestAModifierThatRunsNothingWritesNoTraceLine(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"exec's redirection form", `exec 3>/dev/null; echo ok`, "+zsh:1> echo ok\nok\n"},
		{"two redirections", `exec 3>/dev/null 4>/dev/null; echo ok`, "+zsh:1> echo ok\nok\n"},
		{"behind builtin", `builtin exec 3>/dev/null; echo ok`, "+zsh:1> echo ok\nok\n"},
		{"a prefix, and the next line", `x=1 exec 3>/dev/null; echo ok`, "+zsh:1> x=1 +zsh:1> echo ok\nok\n"},
		{"a prefix, and nothing after", `x=1 exec 3>/dev/null`, "+zsh:1> x=1 "},
		{
			"no such builtin", `builtin nosuch_zz; echo st=$?`,
			"zsh:1: no such builtin: nosuch_zz\n+zsh:1> echo 'st=1'\nst=1\n",
		},
		{"a function is no builtin", `f() { :; }; builtin f`, "zsh:1: no such builtin: f\n"},
		{"builtin -- true", `builtin -- true`, "zsh:1: no such builtin: --\n"},
		{"builtin --", `builtin --`, "zsh:1: no such builtin: --\n"},
		{"builtin -x", `builtin -x`, "zsh:1: no such builtin: -x\n"},
		{
			"a prefix to a refused builtin", `x=1 builtin nosuch; echo ok`,
			"zsh:1: no such builtin: nosuch\n+zsh:1> echo ok\nok\n",
		},
		{
			"a refused builtin in a substitution", `x=$( a=1 builtin nosuch ); echo ok`,
			"+zsh:1> x=zsh:1: no such builtin: nosuch\n+zsh:1> x='' \n+zsh:1> echo ok\nok\n",
		},
		{"exec -a", `exec -a`, "zsh:1: exec requires a command to execute\n"},
		{"exec -a zz", `exec -a zz`, "zsh:1: exec flag -a requires a parameter\n"},
		{"exec -x", `exec -x /bin/echo hi`, "zsh:1: unknown exec flag -x\n"},
		{"exec -c", `exec -c; echo ok`, "zsh:1: exec requires a command to execute\n"},
		// Controls: a modifier with something behind it still writes it,
		// and a bare `exec` is the empty command's bare line.
		{"exec to a program", `exec /usr/bin/true`, "+zsh:1> /usr/bin/true\n"},
		{"a prefix through exec to a builtin", `x=1 exec true`, "+zsh:1> x=1 +zsh:1> true\n"},
		{"a bare exec", `exec; echo ok`, "\n+zsh:1> echo ok\nok\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runZshOnPath(t, t.TempDir(), "set -x; "+c.src)
			if out != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}
