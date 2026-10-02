// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAutoloadOfAnAbsolutePathDefinesItsBaseName: an absolute path as the
// name declares the file's base name, fixed to that file whatever `$fpath`
// holds, and lists the directory the way `-r` does. iTerm2's shell
// integration is `'builtin' 'autoload' '-Uz' '--' "$file"`, a call by
// `${file:t}` and an `unfunction`. Measured 2026-10-02 on zsh 5.9.2 (#5392).
func TestAutoloadOfAnAbsolutePathDefinesItsBaseName(t *testing.T) {
	dir := t.TempDir()
	fns := filepath.Join(dir, "fns")
	if err := os.Mkdir(fns, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fns, "myfn.zsh"), []byte("print ran \"$@\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ src, want string }{
		{
			`f=$PWD/fns/myfn.zsh; 'builtin' 'autoload' '-Uz' '--' "$f"; whence -w ${f:t}; "${f:t}" a; 'builtin' 'unfunction' '--' "${f:t}"; print $?`,
			"myfn.zsh: function\nran a\n0\n",
		},
		{`fpath=(); autoload -U $PWD/fns//myfn.zsh; myfn.zsh b`, "ran b\n"},
		{
			`autoload -Uz $PWD/fns/myfn.zsh; functions myfn.zsh`,
			"myfn.zsh () {\n\t# undefined\n\tbuiltin autoload -XUz DIR/fns\n}\n",
		},
		{
			`zmodload zsh/parameter; autoload -Uz $PWD/fns/myfn.zsh; print -r -- ${functions_source[myfn.zsh]}`,
			"DIR/fns/myfn.zsh\n",
		},
		{
			`autoload -Uz $PWD/fns/nosuch; whence -w nosuch; nosuch; print $?`,
			"nosuch: function\nzsh:1: nosuch: function definition file not found\n1\n",
		},
		// The relative spelling is #1999's, and names no `myfn.zsh`.
		{`autoload fns/myfn.zsh; whence -w myfn.zsh`, "myfn.zsh: none\n"},
	} {
		out, _ := runZsh(t, dir, tc.src)
		if out = strings.ReplaceAll(out, dir, "DIR"); out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}
