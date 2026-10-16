// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A redirection on a command with no command word is opened by the shell
// itself, so ksh93 locates a failure among them the way it locates a
// builtin's — `t.sh[2]:`, and `.[2]:` in a sourced file — where a program's
// redirection is `t.sh: line 2:` (#6655).
//
// The noun is the command having no word. The deciding pairs hold the
// redirection and its line fixed and move only the word: none, a builtin
// (`echo`), a program (`/bin/echo`). Measured 2026-10-15 on ksh93u+
// 2012-08-01, `ENV=/dev/null`; every row is that shell's. This shell wrote
// the line-word form for a bare redirection on every route.
func TestABareRedirectionIsLocatedAsTheShellsOwn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Chdir(home)
	const cannot = "/nonexist_br/x: cannot create [No such file or directory]\n"
	for _, c := range []struct {
		name, script, want string
	}{
		{"bare", ":\n>/nonexist_br/x\n", "t.sh[2]: " + cannot},
		{"bare, with an assignment", ":\nx=1 >/nonexist_br/x\n", "t.sh[2]: " + cannot},
		{"bare, inside an if", ":\nif true; then >/nonexist_br/x; fi\n", "t.sh[2]: " + cannot},
		{"bare, a duplicated descriptor", ":\n>&7\n", "t.sh[2]: 7: cannot open [Bad file descriptor]\n"},
		{"bare, a target that will not expand", ":\n>$((1/0))\n", "t.sh[2]: 1/0: divide by zero\n"},
		{"bare, a here-document body", ":\n<<E\n$((1/0))\nE\n", "t.sh[2]: 1/0: divide by zero\n"},
		{"bare, in a sourced file", ". ./src.inc\n", "t.sh[1]: .: line 1: nosuch_br: not found\nt.sh[1]: .[2]: " + cannot},
		// The other halves: a builtin is located the same way, a program
		// and a function are not, and a prefix value that will not expand
		// is the assignment's and not the redirection's.
		{"a builtin", ":\necho a >/nonexist_br/x\n", "t.sh[2]: " + cannot},
		{"a program", ":\n/bin/echo a >/nonexist_br/x\n", "t.sh: line 2: " + cannot},
		{"a function", "f(){ :; }\nf >/nonexist_br/x\n", "t.sh: line 2: " + cannot},
		{"a prefix value", ":\nx=$((1/0)) >/dev/null\n", "t.sh: line 2: 1/0: divide by zero\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(home, "src.inc"), []byte("nosuch_br\n>/nonexist_br/x\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, "t.sh"), []byte(c.script), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, errs, _ := prompt(t, "", "ksh", "t.sh"); errs != c.want {
				t.Errorf("standard error %q, want %q", errs, c.want)
			}
		})
	}
	// And under -c, where line 1 names nothing and line 2 is bracketed.
	if _, errs, _ := prompt(t, "", "ksh", "-c", ":\n>/nonexist_br/x"); errs != "ksh[2]: "+cannot {
		t.Errorf("-c: standard error %q, want %q", errs, "ksh[2]: "+cannot)
	}
}
