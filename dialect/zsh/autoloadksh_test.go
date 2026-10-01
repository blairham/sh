// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// `autoload -k`, the ksh style: the file is run and is expected to define the
// function itself (#5140). Measured 2026-10-01 on zsh 5.9.2 (`-f`, a script
// file under `env -i PATH=/usr/bin:/bin LC_ALL=C`) with the same three files;
// each row is what that shell wrote.
func TestTheKshStyleRunsTheFileAndThenTheFunctionItDefined(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"kf": "print \"loading $*\"\nkf() { print \"kf called $*\"; }\nprint \"loaded\"\n",
		"kb": "print \"body only $*\"\n",
		"kr": "kr() { print Autoloaded ksh style; } > kr.log\n",
		"kg": "print \"loading2 $*\"\nkg() { print \"kg called $*\"; }\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const setup = "fpath=(. $fpath)\n"
	for _, c := range []struct{ name, src, want string }{
		{"the stub records the letter", "autoload -Uk kf\nfunctions kf", "kf () {\n\t# undefined\n\tbuiltin autoload -XUk\n}\n"},
		{"the file runs, then the function", "autoload -Uk kf\nkf a b\nprint st=$?\nkf c", "loading a b\nloaded\nkf called a b\nst=0\nkf called c\n"},
		{"a file defining nothing", "autoload -Uk kb\nkb x 2>&1\nprint st=$?\nfunctions kb", "body only x\nzsh:3: kb: function not defined by file\nst=1\nkb () {\n\t# undefined\n\tbuiltin autoload -XUk\n}\n"},
		{"a definition with a redirection", "autoload -Uk kr\nkr\nprint No output yet\nprint -r -- \"$(<kr.log)\"\nfunctions kr", "No output yet\nAutoloaded ksh style\nkr () {\n\tprint Autoloaded ksh style\n} > kr.log\n"},
		{"+X defines the wrapper and runs nothing", "autoload -Uk +X kg\nprint st=$?\nfunctions kg\nkg z", "st=0\nkg () {\n\tprint \"loading2 $*\"\n\tkg () {\n\t\tprint \"kg called $*\"\n\t}\n\tkg \"$@\"\n}\nloading2 z\nkg called z\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_ = os.Remove(filepath.Join(dir, "kr.log"))
			if out, _ := runZsh(t, dir, setup+c.src+"\n"); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
