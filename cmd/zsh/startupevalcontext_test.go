// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A startup file is read before the program, so `$zsh_eval_context` holds
// the file and not the route's own word under it (#5885).
//
// Measured 2026-10-05 on zsh 5.9.2 (/opt/homebrew/bin/zsh) under `env -u
// FPATH` with a scratch `$ZDOTDIR`, each startup file printing the parameter
// at its top, in a function it calls, in a command substitution and in an
// `eval`, and the `.zshrc` sourcing one more file; every line below is that
// shell's. This shell wrote `cmdarg` (or `toplevel`) where each `file` is.
func TestAStartupFileIsTheBottomOfTheEvalContext(t *testing.T) {
	const body = `print -r -- @ ctx=$zsh_eval_context
f() { print -r -- infn=$zsh_eval_context }; f
print -r -- $(print sub=$zsh_eval_context)
eval 'print -r -- ev=$zsh_eval_context'
`
	const each = "ctx=file\ninfn=file shfunc\nsub=file cmdsubst\nev=file eval\n"
	home := scratchHome(t)
	dir := filepath.Join(home, "zd")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".zshenv", ".zprofile", ".zshrc", ".zlogin"} {
		writeHomeFile(t, dir, name, strings.Replace(body, "@", name, 1))
	}
	writeHomeFile(t, dir, "inner", "print -r -- inner=$zsh_eval_context\n")
	f, err := os.OpenFile(filepath.Join(dir, ".zshrc"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(". " + filepath.Join(dir, "inner") + "\n")
	_ = f.Close()
	t.Setenv("ZDOTDIR", dir)

	var o, e bytes.Buffer
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &o, &e
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	sh.Stdin = null
	driver.MainArgs(sh, []string{"zsh", "-l", "-i", "-c", "print main=$zsh_eval_context"})
	want := ".zshenv " + each +
		".zprofile " + each +
		".zshrc " + each + "inner=file file\n" +
		".zlogin " + each +
		"main=cmdarg\n"
	if o.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s\nstderr %q", o.String(), want, e.String())
	}
}

// And ahead of a script, where the program's own word is `toplevel`.
func TestAZshenvAheadOfAScriptIsTheBottomOfTheEvalContext(t *testing.T) {
	home := scratchHome(t)
	writeHomeFile(t, home, ".zshenv", "print -r -- env=$zsh_eval_context\n")
	writeHomeFile(t, home, "s.zsh", "print -r -- main=$zsh_eval_context\n")
	var o, e bytes.Buffer
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &o, &e
	driver.MainArgs(sh, []string{"zsh", filepath.Join(home, "s.zsh")})
	if want := "env=file\nmain=toplevel\n"; o.String() != want {
		t.Errorf("stdout %q, want %q (stderr %q)", o.String(), want, e.String())
	}
}
