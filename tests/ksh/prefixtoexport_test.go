// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A prefix in front of `export` is exported as it is assigned, and stays
// exported: the operand and the entries behind it hand a child the prefix's
// value, and the name is still exported once the command is over. Measured
// 2026-10-30 against ksh93u+ 2012-08-01 (`/bin/ksh`) under `env -i
// PATH=/usr/bin:/bin`, from `-c` and from a script file alike. This shell
// took the attribute off, as ksh93 does at every other builtin (#7003). See
// interp.PrefixExportAtABuiltinOffExceptAtExport.
//
// The controls are the other declarations, which take the attribute off.
func TestAPrefixToExportIsExported(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"afterwards", `v=old; v=new export y=1; /usr/bin/printenv v; typeset -p v`, "new\ntypeset -x v=new\n"},
		{"to the operand", `v=old; v=new export y=$(/usr/bin/printenv v); echo "y=$y"`, "y=new\n"},
		{"to the entry behind", `v=old; v=new w=$(/usr/bin/printenv v) export y=1; echo "w=$w"`, "w=new\n"},
		{"with no operand", `v=old; v=new export -p >/dev/null; typeset -p v`, "typeset -x v=new\n"},
		{"through command", `v=old; v=new command export y=1; /usr/bin/printenv v`, "new\n"},
		{"in a POSIX-form function", `f() { v=new export y=1; }; v=old; f; /usr/bin/printenv v`, "new\n"},
		{"in a function-form function", `function f { v=new export y=1; }; v=old; f; /usr/bin/printenv v`, "new\n"},
		// Controls.
		{"typeset -x", `v=old; v=new typeset -x y=1; typeset -p v`, "v=new\n"},
		{"readonly", `v=old; v=new readonly y=$(/usr/bin/printenv v); echo "y=$y"; typeset -p v`, "y=\nv=new\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}

// A prefix in front of any other declaration over a name that was exported
// reaches no child while the operand expands: ksh93 takes the attribute off
// the prefix at a builtin, and the operand is expanded behind the prefix.
// Measured the same day and the same way (#7003).
func TestAPrefixToTypesetReachesNoChildThroughItsOperand(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"typeset", `export v=old; v=new typeset y=$(/usr/bin/printenv v); echo "y=$y"; typeset -p v`, "y=\nv=new\n"},
		{"readonly", `export v=old; v=new readonly y=$(/usr/bin/printenv v); echo "y=$y"`, "y=\n"},
		{"export, the control", `export v=old; v=new export y=$(/usr/bin/printenv v); echo "y=$y"`, "y=new\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}
