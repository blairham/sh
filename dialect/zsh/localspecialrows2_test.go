// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// Two more of the shell's own names in a function and out of one. A local of
// PPID is the outer binding, freeze and all, and the right prompt's indent is
// the shell's own integer once something sets it and nothing until then.
// Measured 2026-10-03 on zsh 5.9.2 under `env -i PATH=/usr/bin:/bin`, `-f`
// (#5619).
func TestPPIDAndTheRightPromptIndentInAndOutOfAFunction(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`f(){ local PPID; print ${(t)PPID} $(( PPID == $PPID )); }; f`, "integer-local-readonly-special 1\n"},
		{`f(){ local PPID=5; print in; }; f; print after`, "f: read-only variable: PPID\n"},
		{`print "[${(t)ZLE_RPROMPT_INDENT}]"; typeset -p ZLE_RPROMPT_INDENT; print st=$?; typeset + | /usr/bin/grep -c ZLE_RPROMPT_INDENT`, "[]\nst=0\n0\n"},
		{`ZLE_RPROMPT_INDENT=1+1; print ${(t)ZLE_RPROMPT_INDENT} $ZLE_RPROMPT_INDENT`, "integer-special 2\n"},
		{`f(){ local ZLE_RPROMPT_INDENT=3; print ${(t)ZLE_RPROMPT_INDENT}; }; f`, "integer-local-special\n"},
		{`typeset -p TERM; print st=$?`, "st=0\n"},
	} {
		if out, _ := runZsh(t, t.TempDir(), c.src); out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}
