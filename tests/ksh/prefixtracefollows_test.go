// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A prefix is traced behind the command's line where it is the command's own
// scope, and ahead of it where it is a store this shell keeps. A
// `function`-form call scopes its prefix to the call, and `command` in front
// of a special builtin keeps the prefix that builtin keeps. Measured
// 2026-10-30 against ksh93u+ 2012-08-01 (`/bin/ksh`), script files under `env
// -i PATH=/usr/bin:/bin`. This shell read the word alone: every function put
// its prefix first (#6983), and `command` put it after (#6984).
//
// The redirection rows are the same rule read off the order of the work: the
// scoped prefix is expanded once the redirections are open, as a regular
// builtin's is, so a redirection that will not open leaves it unexpanded and
// one that does takes its line. A function marked `typeset -ft` is the
// exception, and the last row says so: its prefix is worked through ahead of
// the call as well (#7092). See Runner.tracePrefixFollowsTheCommand.
func TestAPrefixIsTracedWhereItIsStored(t *testing.T) {
	const g = `function g { :; }; `
	for _, c := range []struct{ name, src, want string }{
		{"a function-form call", g + `set -x; x=1 g`, "+ g\n+ x=1\n"},
		{"two entries", g + `set -x; x=1 y=2 g`, "+ g\n+ x=1\n+ y=2\n"},
		{"an element", `b=(x y z); i=1; ` + g + `set -x; b[i++]=Q g`, "+ g\n+ b[1]=Q\n"},
		{"an append", g + `set -x; v+=x g`, "+ g\n+ v+=x\n"},
		{
			"a substitution in the value",
			g + `set -x; x=1 y=$(echo s >&2) g`,
			"+ g\n+ x=1\n+ echo s\n+ 1>& 2\ns\n+ y=''\n",
		},
		{"a division in the subscript", `b=(x y z); ` + g + `set -x; b[1/0]=v g`, "+ g\nksh: 1/0: divide by zero\n"},
		{"a value that will not expand", g + `set -x; x=1 y=${q?boom} g; echo after`, "+ g\n+ x=1\nksh: q: boom\n"},
		{
			"a redirection that will not open",
			g + `set -x; x=$(echo s >&2) g >/nope/f; echo st=$?`,
			"+ g\n+ ksh: /nope/f: cannot create [No such file or directory]\n+ echo st=1\nst=1\n",
		},
		{"a redirection that takes the line", g + `set -x; x=1 g 2>/dev/null`, "+ g\n+ 2> /dev/null\n"},
		{"command eval", `set -x; x=1 command eval :`, "+ x=1\n+ command eval :\n+ :\n"},
		{"command :", `set -x; x=2 command :`, "+ x=2\n+ command :\n"},
		{"command -p eval", `set -x; x=1 command -p eval :`, "+ x=1\n+ command -p eval :\n+ :\n"},
		{"command command :", `set -x; x=1 command command :`, "+ x=1\n+ command command :\n"},
		{
			"an element in front of command eval",
			`typeset -A a; set -x; a[k]=v command eval 'typeset -p a'`,
			"+ a[k]=v\n+ command eval 'typeset -p a'\n+ typeset -p a\ntypeset -A a=([k]=v)\n",
		},
		{"command typeset", `set -x; x=1 command typeset y=2`, "+ x=1\n+ y=2\n+ command typeset y\n"},
		// Controls.
		{"a POSIX-form function", `f() { :; }; set -x; x=1 f`, "+ x=1\n+ f\n+ :\n"},
		{"command with a regular builtin", `set -x; x=1 command echo hi`, "+ command echo hi\n+ x=1\nhi\n"},
		{"a special builtin", `set -x; x=1 eval :`, "+ x=1\n+ eval :\n+ :\n"},
		{"a regular builtin", `set -x; x=1 true`, "+ true\n+ x=1\n"},
		{
			"a traced function-form call",
			`function g { echo in; }; typeset -ft g; set -x; x=1 g 2>/dev/null`,
			"+ x=1\n+ g\n+ 2> /dev/null\nin\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
