// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// An interactive ksh93 that reads a `-c` string or a script file as it reads a
// prompt ends the line on standard error when that input runs out, as the
// prompt does on a pipe (#6654, Semantics.PromptEndOfInputEndsTheLine).
//
// The noun is the *end of the input*. The deciding pairs hold everything else
// fixed: `true` against `exit` (an `exit` reached first writes nothing), the
// same text with and without `-i`, and a trap's output, which comes after the
// newline because the newline is the input's and the trap is the shell's
// leaving. Measured 2026-10-15 on ksh93u+ 2012-08-01, `ENV=/dev/null`, on
// standard error alone; this shell wrote no newline on either route. The rows
// a command string's last command decides — the program the shell becomes —
// are dialect/ksh's TestTheTailIsReachedEvenWhereItCannotBeBecome, because a
// shell in a test process does not replace it.
func TestAnInteractiveKshEndsTheLineAtTheEndOfItsProgram(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	t.Setenv("HISTFILE", filepath.Join(home, ".sh_history"))
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Chdir(home)
	for _, c := range []struct {
		name, program string
		// file is the script route's diagnostic name, and cmd the string's.
		wantCmd, wantFile string
	}{
		{"true", "true", "\n", "\n"},
		{"false", "false", "\n", "\n"},
		{"a failure, then the end", "cd /nonexist_el", "ksh: cd: /nonexist_el: [No such file or directory]\n\n", "e.sh: cd: /nonexist_el: [No such file or directory]\n\n"},
		{"a command not found, then more", "nosuch_el; :", "ksh: nosuch_el: not found\n\n", "e.sh: nosuch_el: not found\n\n"},
		{"a parse failure", "echo $(fi)", "ksh: syntax error: `fi' unexpected\n\n", "e.sh: syntax error: `fi' unexpected\n\n"},
		{"the EXIT trap after it", `trap "echo T >&2" EXIT; true`, "\nT\n", "\nT\n"},
		{"descriptor 2 as the script left it", "exec 2>/dev/null; true", "", ""},
		// The other half: the input did not end, the shell left first.
		{"exit", "exit 3", "", ""},
		{"exit in a function", "f() { exit 2; }; f", "", ""},
		{"set -e", "set -e; false; echo x", "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(home, "e.sh"), []byte(c.program+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, errs, _ := prompt(t, "", "ksh", "-i", "-c", c.program); errs != c.wantCmd {
				t.Errorf("-i -c: standard error %q, want %q", errs, c.wantCmd)
			}
			if _, errs, _ := prompt(t, "", "ksh", "-i", "e.sh"); errs != c.wantFile {
				t.Errorf("-i e.sh: standard error %q, want %q", errs, c.wantFile)
			}
		})
	}
	// And not without `-i`, where the same text reads as a program.
	if _, errs, _ := prompt(t, "", "ksh", "-c", "true"); errs != "" {
		t.Errorf("-c: standard error %q, want nothing", errs)
	}
}
