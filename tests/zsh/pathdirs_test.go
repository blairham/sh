// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPathDirsSearchesASlashedNameDownThePath pins `pathdirs`. Measured
// 2026-10-02 on zsh 5.9.2 under `-f` (#5155), with `sub/x` and a
// non-executable `sub/n` in a path directory `top`, and the working
// directory holding `sub/here` and a non-executable `sub/n` of its own.
func TestPathDirsSearchesASlashedNameDownThePath(t *testing.T) {
	dir := t.TempDir()
	top := filepath.Join(dir, "top")
	write := func(path, body string, mode os.FileMode) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(top, "sub/x"), "#!/bin/sh\necho lower\n", 0o755)
	write(filepath.Join(top, "sub/n"), "echo no\n", 0o644)
	write(filepath.Join(top, "sub/q"), "#!/bin/sh\necho pathq\n", 0o755)
	write(filepath.Join(dir, "sub/here"), "#!/bin/sh\necho cwd\n", 0o755)
	write(filepath.Join(dir, "sub/q"), "echo cwdq\n", 0o644)
	found := filepath.Join(top, "sub/x")
	pre := "setopt pathdirs; path=(" + top + " /bin); "
	cases := []struct{ src, want string }{
		{pre + "sub/x", "lower\n"},
		{pre + "sub/here", "cwd\n"},
		{pre + "sub/q", "pathq\n"},
		{pre + "whence sub/x; type sub/x; command -v sub/x", found + "\nsub/x is " + found + "\n" + found + "\n"},
		{pre + "whence sub/here", "sub/here\n"},
		{pre + "sub/nope", "zsh:1: command not found: sub/nope\n"},
		{pre + "sub/n", "zsh:1: permission denied: sub/n\n"},
		{pre + "./sub/x", "zsh:1: no such file or directory: ./sub/x\n"},
		{pre + "/sub/x", "zsh:1: no such file or directory: /sub/x\n"},
		{pre + "unsetopt pathdirs; sub/x", "zsh:1: no such file or directory: sub/x\n"},
		{pre + "hash -r; sub/x; print ${commands[sub/x]-none}", "lower\nnone\n"},
	}
	for _, c := range cases {
		got, _ := runZsh(t, dir, c.src)
		if got != c.want {
			t.Errorf("%s\n got %q\nwant %q", strings.TrimPrefix(c.src, pre), got, c.want)
		}
	}
}
