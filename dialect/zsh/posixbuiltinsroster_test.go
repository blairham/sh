// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **`posixbuiltins` replaces the roster of builtins that keep an assignment
// prefix with the standard's rule** (#5136). Measured 2026-10-01 on zsh 5.9.2
// under `-f -c` as `( x=43; x=v CMD; print "x=$x" > result ); <result`, read
// back outside the subshell because two of the rows redirect and one is
// `exec`. Each row is the default and then the option on; the bare `builtin`
// is the row that moved when this was first tried, before #5137 gave a line
// of modifiers its own road.
func TestPosixBuiltinsClearsThePrefixRoster(t *testing.T) {
	for _, c := range []struct{ cmd, off, on string }{
		{"alias foo=bar", "x=v", "x=43"},
		{"hash -r", "x=v", "x=43"},
		{"hash 2>out", "x=v", "x=43"},
		{"builtin alias foo=bar", "x=v", "x=43"},
		{"builtin :", "x=43", "x=v"},
		{"exec 2>out", "x=43", "x=v"},
		{"builtin", "x=v", "x=v"},
		{"exec", "x=v", "x=v"},
	} {
		for _, o := range []struct{ set, want string }{
			{"", c.off},
			{"setopt posixbuiltins; ", c.on},
			{"setopt posixbuiltins; unsetopt posixbuiltins; ", c.off},
		} {
			src := o.set + `( x=43; x=v ` + c.cmd + `; print "x=$x" > result ); /bin/cat result`
			t.Run(o.set+c.cmd, func(t *testing.T) {
				out, _ := runZsh(t, t.TempDir(), src)
				if out != o.want+"\n" {
					t.Errorf("%s\ngot  %q\nwant %q", src, out, o.want+"\n")
				}
			})
		}
	}
}
