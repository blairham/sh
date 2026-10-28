// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// A subshell's EXIT trap is located at the line that opened the subshell when
// its body ran off its end, and at the `exit`'s or the failing command's line
// when one ended it; the shell's own is located at an `exit`'s line too, and
// past the end only where the script ran out (#6899).
//
// Measured 2026-10-27 on zsh 5.9.2 under `-f` from script files, with a
// three-line body whose second line fails and whose third writes `$LINENO`.
// Every row's body is on the same lines, so a row that pinned the body's own
// line would read the same as the control, which is the shell's own trap at
// the line past the end. The sources are run without their final newline,
// because the harness parses the text whole and a final newline moves the end
// a line past where the file route puts it. A `<( … )` opening on the command's
// line was measured through the binary, with `cat` on the path, and is one
// more copy of the same clone. A fatal error — a division by zero, a readonly
// reassignment, an unset name under `set -u` — leaves zsh's trap at the line
// after the failing statement, which the binary's statement-at-a-time route
// reproduces and a whole-text parse cannot, so it is not a row here.
func TestASubshellsExitTrapIsLocatedWhereTheSubshellOpened(t *testing.T) {
	const body = "trap \":\nnosuch_t\necho L=\\$LINENO\" EXIT\n"
	for _, c := range []struct{ name, src, out, errs string }{
		{"the opening parenthesis", ":\n(" + body + ":\n:\n)\n:\n", "L=2\n", "zsh:2: command not found: nosuch_t\n"},
		{"the parenthesis and not the trap", ":\n(:\n" + body + ":\n:\n)\n:\n", "L=2\n", "zsh:2: command not found: nosuch_t\n"},
		{"a parenthesis on a line of its own", ":\n(\n" + body + ":\n)\n:\n", "L=2\n", "zsh:2: command not found: nosuch_t\n"},
		{"a pipeline element's own line", ":\n: |\n(\n" + body + ":\n)\n:\n", "L=3\n", "zsh:3: command not found: nosuch_t\n"},
		{"a group piped onward", ":\n{\n" + body + ":\n} | :\n:\n", "", "zsh:2: command not found: nosuch_t\n"},
		{"a background job", ":\n(\n" + body + ":\n) &\nwait\n:\n", "L=2\n", "zsh:2: command not found: nosuch_t\n"},
		{"nested, the inner parenthesis", ":\n(\n:\n(\n" + body + ":\n)\n:\n)\n:\n", "L=4\n", "zsh:4: command not found: nosuch_t\n"},
		{"a command substitution's command", ":\nx=$(\n" + body + ":\n)\necho \"$x\"\n", "L=2\n", "zsh:2: command not found: nosuch_t\n"},
		{"a continued command's first line", ":\necho a \\\n$(\n" + body + ":\n) >/dev/null\n:\n", "", "zsh:2: command not found: nosuch_t\n"},
		{"an exit's line", ":\n(\n" + body + ":\n:\nexit 0\n)\n:\n", "L=8\n", "zsh:8: command not found: nosuch_t\n"},
		{"an exit inside an if", ":\n(\n" + body + ":\nif true; then\n:\nexit 0\nfi\n)\n:\n", "L=9\n", "zsh:9: command not found: nosuch_t\n"},
		{"an exit in a function, at the call", ":\nf() {\n:\nexit 0\n}\n(\n" + body + ":\nf\n)\n:\n", "L=11\n", "zsh:11: command not found: nosuch_t\n"},
		{"errexit's failing command", ":\n(\n" + body + ":\nset -e\n:\nfalse\n:\n)\n:\n", "", "zsh:9: command not found: nosuch_t\n"},
		{"a group as the last element is the shell", ":\n: |\n{\n" + body + ":\n}\n:\n", "L=10\n", "zsh:10: command not found: nosuch_t\n"},
		{"inside a function, counted from it", ":\nf() {\n:\n(\n" + body + ":\n)\n}\n:\nf\n:\n", "L=2\n", "f:2: command not found: nosuch_t\n"},
		{"the shell's own exit", ":\n" + body + ":\nexit 0\n:\n", "L=6\n", "zsh:6: command not found: nosuch_t\n"},
		{"the shell's own exit inside a function", ":\nf() {\n:\nexit 0\n}\n" + body + ":\nf\n:\n", "L=10\n", "zsh:10: command not found: nosuch_t\n"},
		{"the shell's own errexit", ":\n" + body + ":\nset -e\nfalse\n:\n", "", "zsh:7: command not found: nosuch_t\n"},
		{"the shell's own word error", ":\n" + body + ":\n: ${x?word}\n:\n", "L=6\n", "zsh:6: x: word\nzsh:6: command not found: nosuch_t\n"},
		{"the shell running out, the control", ":\n" + body + ":\n:\n:\n", "L=8\n", "zsh:8: command not found: nosuch_t\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, errs := runZshSplitOnRoute(t, interp.RouteScriptFile, strings.TrimSuffix(c.src, "\n"))
			if out != c.out || errs != c.errs {
				t.Errorf("wrote %q and %q, want %q and %q", out, errs, c.out, c.errs)
			}
		})
	}
}
