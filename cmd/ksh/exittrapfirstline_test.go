// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The shell's own EXIT trap names line 1 of its body where nothing else under
// `-c` does: ksh93 writes `ksh: line 1:` and `ksh[1]:` there, as for a text
// entered after the one it was given (#6656).
//
// The noun is whose EXIT trap runs. The deciding pairs hold the failing text
// and its line fixed: the same `nosuch` as the whole string (no line), as the
// shell's EXIT trap (line 1), as a subshell's EXIT trap and as a signal's trap
// (no line). Measured 2026-10-15 on ksh93u+ 2012-08-01, `ENV=/dev/null`; every
// row is that shell's. This shell named no line for the shell's trap.
func TestTheShellsExitTrapNamesItsFirstLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	t.Setenv("HISTFILE", filepath.Join(home, ".sh_history"))
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Chdir(home)
	const cd = "cd: /nonexist_et: [No such file or directory]\n"
	for _, c := range []struct {
		name string
		argv []string
		want string
	}{
		{"-c, not found", []string{"ksh", "-c", ":\ntrap nosuch_et EXIT\n:"}, "ksh: line 1: nosuch_et: not found\n"},
		{"-c, a builtin", []string{"ksh", "-c", "trap 'cd /nonexist_et' EXIT"}, "ksh[1]: " + cd},
		{"-c, through eval", []string{"ksh", "-c", "trap 'eval nosuch_et' EXIT"}, "ksh[1]: eval: line 1: nosuch_et: not found\n"},
		{"-c, after exit", []string{"ksh", "-c", "trap nosuch_et EXIT; exit 0"}, "ksh: line 1: nosuch_et: not found\n"},
		{"-c, an arithmetic failure", []string{"ksh", "-c", "trap ': $((1/0))' EXIT"}, "ksh: line 1: 1/0: divide by zero\n"},
		{"-c, the body's second line", []string{"ksh", "-c", "trap ':\nnosuch_et' EXIT"}, "ksh: line 2: nosuch_et: not found\n"},
		// The other halves.
		{"-c, the string itself", []string{"ksh", "-c", "nosuch_et"}, "ksh: nosuch_et: not found\n"},
		{"-c, a subshell's EXIT trap", []string{"ksh", "-c", "(trap nosuch_et EXIT; :)"}, "ksh: nosuch_et: not found\n"},
		{"-c, a signal's trap", []string{"ksh", "-c", "trap nosuch_et USR1; kill -USR1 $$"}, "ksh: nosuch_et: not found\n"},
		{"-c, a subshell's builtin", []string{"ksh", "-c", "(trap 'cd /nonexist_et' EXIT; :)"}, "ksh: " + cd},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, errs, _ := prompt(t, "", c.argv...); errs != c.want {
				t.Errorf("standard error %q, want %q", errs, c.want)
			}
		})
	}
	// At a prompt, on a pipe, where the prompt's own line 1 names nothing.
	t.Run("at a prompt", func(t *testing.T) {
		if _, errs, _ := prompt(t, "trap nosuch_et EXIT\n", "ksh", "-i"); errs != "$ $ \nksh: line 1: nosuch_et: not found\n" {
			t.Errorf("standard error %q", errs)
		}
	})
	// And `-i script`, whose file is located as a prompt is.
	t.Run("-i script", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(home, "x.sh"), []byte("trap nosuch_et EXIT\n:\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, errs, _ := prompt(t, "", "ksh", "-i", "x.sh"); errs != "\nx.sh: line 1: nosuch_et: not found\n" {
			t.Errorf("standard error %q", errs)
		}
	})
}
