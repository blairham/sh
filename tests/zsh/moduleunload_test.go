// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// What `zmodload -u` takes back, which is the load's own bookkeeping read
// backwards — and both rosters go through it, because a module half-unloaded
// is a state neither shell has.
//
// Every `want` is the reference's own answer, measured 2026-09-28 on
// `/opt/homebrew/bin/zsh` — zsh 5.9.2 (aarch64-apple-darwin25.4.0), `go
// version -m` says *not a Go executable* for it — run `-f` from a script file
// under `env -i PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME`, one
// shell per row, the module loaded and immediately unloaded (#5025).

// TestUnloadingAModuleTakesItsGatedBuiltinsBack is the builtin half.
//
// It broke when #5029 landed rather than being written wrong: the release
// restores "the way this runner was built", and #5029 *moved* how a gated
// builtin is built. A roster read from before that move restores a state that
// is no longer the startup state, which is the whole of this row.
func TestUnloadingAModuleTakesItsGatedBuiltinsBack(t *testing.T) {
	for _, tc := range []struct {
		module, builtin string
		// what `whence -w` writes after the unload
		word string
	}{
		{"zsh/datetime", "strftime", "none"},
		{"zsh/system", "zsystem", "none"},
		{"zsh/system", "sysopen", "none"},
		{"zsh/zselect", "zselect", "none"},
		{"zsh/stat", "zstat", "none"},
		{"zsh/zpty", "zpty", "none"},
		{"zsh/files", "zf_rm", "none"},
		// **The controls, and they are two kinds.** `zsh/zutil` declares its
		// builtins autoloadable, so they are there in a fresh shell and the
		// unload leaves them — copying "an unload removes the builtins"
		// would turn `zmodload -u zsh/zutil; zstyle` from 0 into `command not
		// found`. And the plain spellings go back to being the ordinary
		// command they were, which is a *third* answer and not `none`.
		{"zsh/zutil", "zparseopts", "builtin"},
		{"zsh/zutil", "zstyle", "builtin"},
		{"zsh/files", "rm", "command"},
		{"zsh/stat", "stat", "command"},
	} {
		t.Run(tc.module+" "+tc.builtin, func(t *testing.T) {
			want := tc.builtin + ": " + tc.word + "\n"
			code := 0
			if tc.word == "none" {
				code = 1
			}
			dir := t.TempDir()
			if tc.word == "command" {
				// runZsh's `PATH` is the scratch directory, so the ordinary
				// command this name goes back to being has to be *on* it —
				// otherwise the row reads `none` for the reason every other
				// row reads it, and the control stops separating anything.
				pathStub(t, dir, tc.builtin)
			}
			out, st := runZsh(t, dir,
				"zmodload "+tc.module+"\nzmodload -u "+tc.module+
					"\nwhence -w "+tc.builtin)
			if out != want || st != code {
				t.Errorf("= %q (status %d), want %q at %d", out, st, want, code)
			}
		})
	}
}

// pathStub puts an executable of that name on the scratch `PATH`, so that a
// name going back to "an ordinary command" has one to be.
func pathStub(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
}

// TestUnloadingAModuleTakesItsGatedParametersBack is the parameter half, and
// `installGatedParameters` had no opposite number at all.
//
// **`zsh/watch` is the control and it is the whole reason the rule is not
// "everything the module brought goes".** Nothing referred to any of these
// names, so "is it in use" does not separate them; what does is whose name it
// is. `WATCHFMT` and `LOGCHECK` are ordinary parameters the module assigns a
// default to — `scalar` and `integer`, neither of them special — and `watch`
// and `WATCH` are the shell's own, present at `${+watch}` of 1 in a fresh
// shell before any module is loaded. The seven that go are the seven the
// module declares as features of its own and that carry
// `hide-hideval-special` while it is loaded.
func TestUnloadingAModuleTakesItsGatedParametersBack(t *testing.T) {
	for _, tc := range []struct {
		module, param string
		// `${+param}` after the unload
		set bool
	}{
		{"zsh/langinfo", "langinfo", false},
		{"zsh/mapfile", "mapfile", false},
		{"zsh/system", "sysparams", false},
		{"zsh/system", "errnos", false},
		{"zsh/datetime", "epochtime", false},
		{"zsh/datetime", "EPOCHSECONDS", false},
		{"zsh/datetime", "EPOCHREALTIME", false},
		// The control.
		{"zsh/watch", "WATCHFMT", true},
		{"zsh/watch", "LOGCHECK", true},
		{"zsh/watch", "watch", true},
		{"zsh/watch", "WATCH", true},
	} {
		t.Run(tc.module+" "+tc.param, func(t *testing.T) {
			want := "0\n"
			if tc.set {
				want = "1\n"
			}
			out, st := runZsh(t, t.TempDir(),
				"zmodload "+tc.module+"\nzmodload -u "+tc.module+
					"\nprint ${+"+tc.param+"}")
			if out != want || st != 0 {
				t.Errorf("= %q (status %d), want %q", out, st, want)
			}
		})
	}
}

// And the sentence, which is what a script that asks by name gets: the same
// one a fresh shell gives, because the name is back where it started rather
// than merely emptied. `typeset -p langinfo` is `no such variable: langinfo`
// at 1 there before the load and after the unload alike.
func TestAReleasedParameterAnswersAsOneTheShellHasNotGot(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir,
		"zmodload zsh/langinfo\nzmodload -u zsh/langinfo\n"+
			"typeset -p langinfo\nprint -r -- \"st=$?\"")
	// runZsh merges the two streams, so the refusal is in `out` — which is
	// what this row is about: the *sentence*, not only the status.
	if want := "zsh:typeset:3: no such variable: langinfo\nst=1\n"; out != want || st != 0 {
		t.Errorf("wrote %q at %d, want %q", out, st, want)
	}
}

// TestALoadAfterAnUnloadBringsBothHalvesBack is what keeps the release from
// being a one-way door: the names are withdrawn rather than unregistered, so
// nothing has to have been remembered elsewhere to put them back.
func TestALoadAfterAnUnloadBringsBothHalvesBack(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"zmodload zsh/datetime\nzmodload -u zsh/datetime\nzmodload zsh/datetime\n"+
			"whence -w strftime\nprint ${+EPOCHSECONDS} ${+epochtime}")
	if want := "strftime: builtin\n1 1\n"; out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
	// And the value is real rather than a name standing empty: the producer
	// the withdrawal kept is the one answering again.
	out, st = runZsh(t, t.TempDir(),
		"zmodload zsh/langinfo\nzmodload -u zsh/langinfo\nzmodload zsh/langinfo\n"+
			"print -r -- ${langinfo[CODESET]}")
	if out == "" || out == "\n" || st != 0 {
		t.Errorf("= %q (status %d), want the table answering again", out, st)
	}
}
