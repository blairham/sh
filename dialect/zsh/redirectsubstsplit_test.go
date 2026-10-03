// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRedirectTargetSplitsACommandSubstitution pins how a redirect target
// is split. Measured 2026-10-02 on zsh 5.9.2 under `-f` (#5155): with
// multios set, an unquoted command substitution is split and every word is
// a target, while a parameter holding the same text is one name, and so is
// a quoted substitution.
func TestRedirectTargetSplitsACommandSubstitution(t *testing.T) {
	cases := []struct{ src, want string }{
		{"/bin/cat <$(print f1 f2)", "A\nB\n"},
		{"IFS=:; /bin/cat <$(print f1:f2)", "A\nB\n"},
		{`x="f1 f2"; /bin/cat <$x`, "zsh:1: no such file or directory: f1 f2\n"},
		{`/bin/cat <"$(print f1 f2)"`, "zsh:1: no such file or directory: f1 f2\n"},
		{"unsetopt multios; /bin/cat <$(print f1 f2)", "zsh:1: no such file or directory: f1 f2\n"},
		{"print hi >$(print o1 o2); /bin/cat o1 o2", "hi\nhi\n"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		for name, body := range map[string]string{"f1": "A\n", "f2": "B\n"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if got, _ := runZsh(t, dir, c.src); got != c.want {
			t.Errorf("%s: got %q, want %q", c.src, got, c.want)
		}
	}
}
