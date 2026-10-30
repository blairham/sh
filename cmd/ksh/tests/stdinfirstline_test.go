// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package tests_test

import (
	"path/filepath"
	"testing"
)

// A program read from standard input names its first line the way a script
// file does: `ksh: line 1: …`, where `-c` writes no line there. This shell
// left `line 1:` out on that route, because the route took the `-c` style for
// everything but a builtin's own complaint, which already had the file's
// bracket (#6871).
//
// The noun is the route, and the deciding pairs hold the failing text fixed
// and move it between `-c` and standard input. Measured 2026-10-30 on ksh93u+
// 2012-08-01 under `env -i PATH=/usr/bin:/bin`, the program on a pipe and as
// a regular file alike; every row is that shell's. A parse failure met while
// reading keeps its own answer on both routes, since its sentence carries the
// line: those are the controls.
func TestAProgramOnStandardInputNamesItsFirstLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	t.Setenv("HISTFILE", filepath.Join(home, ".sh_history"))
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Chdir(home)
	const cd = "cd: /nonexist_sf: [No such file or directory]\n"
	for _, c := range []struct {
		name, program, want string
	}{
		{"not found", "nosuch_sf\n", "ksh: line 1: nosuch_sf: not found\n"},
		{"an unset parameter", "echo b; echo ${x_sf?boom}\n", "ksh: line 1: x_sf: boom\n"},
		{"an arithmetic failure", "x=$(( 1 + ))\n", "ksh: line 1:  1 + : more tokens expected\n"},
		{"a body refused as it is expanded", "echo ${ echo {v}}\n", "ksh: line 1: syntax error at line 1: `end of file' unexpected\n"},
		{"through eval", "eval nosuch_sf\n", "ksh[1]: eval: line 1: nosuch_sf: not found\n"},
		// The controls, which this shell already wrote.
		{"a builtin", "cd /nonexist_sf\n", "ksh[1]: " + cd},
		{"the second line", ":\nnosuch_sf\n", "ksh: line 2: nosuch_sf: not found\n"},
		{"a parse failure on the first line", "echo )\n", "ksh: syntax error at line 1: `)' unexpected\n"},
		{"a parse failure after the first line ran", ":\necho )\n", "ksh: syntax error at line 2: `)' unexpected\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, errs, _ := prompt(t, c.program, "ksh"); errs != c.want {
				t.Errorf("standard error %q, want %q", errs, c.want)
			}
		})
	}
	// And `-c` still names nothing on its first line, which is the other half
	// of every pair above.
	for _, c := range []struct {
		name, program, want string
	}{
		{"-c, not found", "nosuch_sf", "ksh: nosuch_sf: not found\n"},
		{"-c, an unset parameter", "echo ${x_sf?boom}", "ksh: x_sf: boom\n"},
		{"-c, through eval", "eval nosuch_sf", "ksh: eval: line 1: nosuch_sf: not found\n"},
		{"-c, a builtin", "cd /nonexist_sf", "ksh: " + cd},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, errs, _ := prompt(t, "", "ksh", "-c", c.program); errs != c.want {
				t.Errorf("standard error %q, want %q", errs, c.want)
			}
		})
	}
}
