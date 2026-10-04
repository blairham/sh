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

// A startup file that will not parse runs up to the line that will not, and
// costs that file alone: the files after it are read and the command the shell
// was started for runs (#5869).
//
// Measured 2026-10-04 on zsh 5.9.2 (/opt/homebrew/bin/zsh) under `env -u
// FPATH`, with `$ZDOTDIR` a scratch directory; every line below is that
// shell's, with the directory standing for the path. This shell used to parse
// each file whole first, so it ran none of the file and stopped before the
// command.
func TestAZshStartupFileThatWillNotParseRunsUpToTheFailure(t *testing.T) {
	for _, c := range []struct {
		name  string
		files map[string]string
		argv  []string
		out   string
		errs  string
	}{
		{
			// The status the command sees is what the file's last command
			// left, as if the failure had not been there.
			name:  "the issue's own .zshenv under -c",
			files: map[string]string{".zshenv": "echo before\nnosuchcmd_q\necho ${x\necho after\n"},
			argv:  []string{"zsh", "-c", "echo main $?"},
			out:   "before\nmain 127\n",
			errs: "@/.zshenv:2: command not found: nosuchcmd_q\n" +
				"@/.zshenv:5: closing brace expected\n",
		},
		{
			// And every file after a broken one is still read, a broken one
			// among them.
			name: "a login shell reads past two broken files",
			files: map[string]string{
				".zshenv":   "echo in-zshenv\necho )\necho after-env\n",
				".zprofile": "echo in-zprofile\nfi\n",
				".zlogin":   "echo in-zlogin\n",
			},
			argv: []string{"zsh", "-l", "-c", "echo main $?"},
			out:  "in-zshenv\nin-zprofile\nin-zlogin\nmain 0\n",
			errs: "@/.zshenv:2: parse error near `)'\n" +
				"@/.zprofile:2: parse error near `fi'\n",
		},
		{
			// Nothing in the file ran, so nothing is left to keep: 0, and not
			// the 1 the file before it left.
			name: "a failure on the first line leaves 0",
			files: map[string]string{
				".zshenv":   "false\n",
				".zprofile": "if\n",
			},
			argv: []string{"zsh", "-l", "-c", "echo main $?"},
			out:  "main 0\n",
			errs: "@/.zprofile:2: parse error near `\\n'\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := scratchHome(t)
			dir := filepath.Join(home, "zd")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			for name, body := range c.files {
				writeHomeFile(t, dir, name, body)
			}
			t.Setenv("ZDOTDIR", dir)
			var o, e bytes.Buffer
			sh := scratchShell(t)
			sh.Stdout, sh.Stderr = &o, &e
			code := driver.MainArgs(sh, c.argv)
			if code != 0 {
				t.Errorf("status %d, want the command's own 0", code)
			}
			if o.String() != c.out {
				t.Errorf("stdout %q, want %q", o.String(), c.out)
			}
			if want := strings.ReplaceAll(c.errs, "@", dir); e.String() != want {
				t.Errorf("stderr %q, want %q", e.String(), want)
			}
		})
	}
}

// At a prompt the same failure on the first line leaves the syntax-error
// status rather than 0, which is the one place the interactive shell parts
// from the one running a command: measured, `.zshrc` holding `if` alone
// answers `echo st=$?` typed at the first prompt with `st=1`, and `x=1` in
// front of the `if` makes it `st=0`.
func TestAZshrcThatFailsOnItsFirstLeavesOneAtThePrompt(t *testing.T) {
	for _, c := range []struct{ rc, want string }{
		{"if\n", "st=1"},
		{"x=1\nif\n", "st=0"},
	} {
		home := scratchHome(t)
		writeHomeFile(t, home, ".zshrc", c.rc)
		out, errs, _ := prompt(t, "echo st=$?\n", "zsh", "-i")
		if !strings.Contains(out, c.want) {
			t.Errorf("%q: said %q, want %s", c.rc, out, c.want)
		}
		if !strings.Contains(errs, filepath.Join(home, ".zshrc")+":") {
			t.Errorf("%q: stderr %q, want the file named", c.rc, errs)
		}
	}
}

// And it is what lets an alias the file defines reach the file's own later
// lines, which a whole-file parse had already read past. Measured on the same
// zsh: a `.zshenv` of `alias a='echo hit'` and then `a` prints `hit`.
func TestAnAliasAZshStartupFileDefinesReachesItsNextLine(t *testing.T) {
	home := scratchHome(t)
	writeHomeFile(t, home, ".zshenv", "alias a='echo hit'\na\n")
	var o, e bytes.Buffer
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &o, &e
	if code := driver.MainArgs(sh, []string{"zsh", "-c", "echo main"}); code != 0 {
		t.Errorf("status %d, want 0", code)
	}
	if want := "hit\nmain\n"; o.String() != want || e.Len() != 0 {
		t.Errorf("stdout %q and stderr %q, want %q and nothing", o.String(), e.String(), want)
	}
}
