// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAnAutoloadFileThatWillNotParseSaysWhere pins the sentence a function
// file that does not parse earns: the parser's own, at the function's name
// and the file's line. Measured 2026-10-02 on zsh 5.9.2 under `-f` (#5148).
func TestAnAutoloadFileThatWillNotParseSaysWhere(t *testing.T) {
	cases := []struct{ body, pre, want string }{
		{"if true; then\n", "", "ff:2: parse error near `\\n'\nst=1\n"},
		{"echo a )\n", "", "ff:1: parse error near `)'\nst=1\n"},
		{"echo ok\n}\necho b\n", "", "ff:2: parse error near `}'\nst=1\n"},
		{"{ echo OK }\nprint x\n", "", "OK\nx\nst=0\n"},
		{"{ echo OK }\nprint x\n", "setopt ignorebraces; ", "ff:3: parse error near `\\n'\nst=1\n"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "ff"), []byte(c.body), 0o644); err != nil {
			t.Fatal(err)
		}
		src := "fpath=(" + dir + "); " + c.pre + "autoload -z ff; ff; print st=$?"
		if got, _ := runZshOnPath(t, dir, src); got != c.want {
			t.Errorf("%q %s\n got %q\nwant %q", c.body, c.pre, got, c.want)
		}
	}
}
