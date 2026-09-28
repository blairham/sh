// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// Which module builtins a fresh shell has, and which wait for a `zmodload`.
//
// Every `want` is the reference's own answer, measured 2026-09-28 on
// `/opt/homebrew/bin/zsh` — zsh 5.9.2 (aarch64-apple-darwin25.4.0) — run `-f`
// from a script file under `env -i PATH=/usr/bin:/bin TERM=dumb` with a
// scratch `HOME`, one `whence -w` per shell.

// TestAModuleBuiltinWaitsForItsModuleOrDoesNot is the row #4997 filed, with
// the roster corrected.
//
// **The issue reported it as "every other builtin a module names" and named
// `zparseopts` and `zstyle` among them.** Those two are there in a fresh
// shell, and so are twenty-one more; the absent ones are twenty. The present
// column is the control, and without it this row is a rule about modules
// rather than about builtins — `zmodload -e` says **none** of the fourteen
// modules is loaded, so "present" cannot mean "the module is already in".
func TestAModuleBuiltinWaitsForItsModuleOrDoesNot(t *testing.T) {
	for _, tc := range []struct {
		name    string
		present bool
	}{
		// Absent until a `zmodload`.
		{"strftime", false},
		{"syserror", false},
		{"sysopen", false},
		{"sysread", false},
		{"sysseek", false},
		{"syswrite", false},
		{"zsystem", false},
		{"zselect", false},
		{"zstat", false},
		{"zpty", false},
		{"zf_chgrp", false},
		{"zf_chmod", false},
		{"zf_chown", false},
		{"zf_ln", false},
		{"zf_mkdir", false},
		{"zf_mv", false},
		{"zf_rm", false},
		{"zf_rmdir", false},
		{"zf_sync", false},
		// The control: present in a fresh shell, one from each module that
		// declares its builtins autoloadable.
		{"zparseopts", true},
		{"zstyle", true},
		{"zformat", true},
		{"bindkey", true},
		{"vared", true},
		{"zle", true},
		{"sched", true},
		{"compadd", true},
		{"comptags", true},
		{"echoti", true},
		{"ulimit", true},
		{"limit", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The **status** is half the answer and is why it is asserted:
			// `whence -w` is 1 for a name it found nothing for and 0 for one
			// it did, so a shell that wrote the right word and the wrong
			// number would pass a row that only read the word.
			want, code := tc.name+": none\n", 1
			if tc.present {
				want, code = tc.name+": builtin\n", 0
			}
			out, st := runZsh(t, t.TempDir(), "whence -w "+tc.name)
			if out != want || st != code {
				t.Errorf("= %q (status %d), want %q at %d", out, st, want, code)
			}
		})
	}
}

// And the load brings them, which is the other half: the gate opens by the
// road the feature selection already used, so nothing new puts them back.
func TestLoadingAModuleBringsItsGatedBuiltins(t *testing.T) {
	for _, tc := range []struct{ module, builtin string }{
		{"zsh/datetime", "strftime"},
		{"zsh/system", "zsystem"},
		{"zsh/system", "sysopen"},
		{"zsh/zselect", "zselect"},
		{"zsh/stat", "zstat"},
		{"zsh/zpty", "zpty"},
		{"zsh/files", "zf_rm"},
	} {
		t.Run(tc.module+" "+tc.builtin, func(t *testing.T) {
			want := tc.builtin + ": builtin\n"
			out, st := runZsh(t, t.TempDir(),
				"zmodload "+tc.module+"\nwhence -w "+tc.builtin)
			if out != want || st != 0 {
				t.Errorf("= %q (status %d), want %q", out, st, want)
			}
		})
	}
}

// The sentence a script gets for one of them before the load, which is what
// #4997 says the gate is worth: told by name at its own call site.
func TestCallingAGatedBuiltinBeforeItsLoadIsCommandNotFound(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "strftime \"%Y\" 0\nprint -r -- \"st=$?\"")
	const want = "zsh:1: command not found: strftime\nst=127\n"
	if out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
	// And it runs after the load, so the gate is a gate and not a removal.
	// The year is not asserted — the epoch reads as 1969 or 1970 depending on
	// the zone this runs in, and what this row is about is that a number came
	// back at all.
	out, st = runZsh(t, t.TempDir(), "zmodload zsh/datetime\nstrftime \"%Y\" 0")
	if st != 0 || (out != "1970\n" && out != "1969\n") {
		t.Errorf("after the load = %q (status %d), want the epoch's year", out, st)
	}
}
