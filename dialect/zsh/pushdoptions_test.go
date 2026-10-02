// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPushdOptionsMoveTheStack pins `pushdignoredups`, `pushdminus` and
// `pushdtohome`, which the prelude's `pushd` and `popd` read. Measured
// 2026-10-02 on zsh 5.9.2 under `-f` (#5155), in a directory M holding `t`
// and `n`, with `dirs` written M-relative.
func TestPushdOptionsMoveTheStack(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"t", "n", "h"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct{ src, want string }{
		// Only the first entry naming the new directory goes.
		{
			"pushd $M/t; pushd $M/t; setopt pushdignoredups; pushd $M/t; dirs; pushd $M; dirs; pushd $M/n; pushd $M/t; dirs",
			"M/t M/t M\nM M/t M/t\nM/t M/n M M/t\n",
		},
		{"pushd $M/t; pushd $M/n; pushd -0; dirs", "M M/n M/t\n"},
		{
			"pushd $M/t; pushd $M/n; setopt pushdminus; pushd -0; dirs; pushd +1; dirs; popd -1; dirs",
			"M/n M/t M\nM/t M M/n\nM/t M/n\n",
		},
		{"HOME=$M/h; pushd $M/t; setopt pushdtohome; pushd; dirs", "~ M/t M\n"},
		{"HOME=$M/h; pushd $M/t; pushd; dirs", "M M/t\n"},
	}
	for _, c := range cases {
		got, _ := runZshOnPath(t, dir, "HOME=/nonexistent; M="+dir+"; cd $M; "+c.src)
		if got = strings.ReplaceAll(got, dir, "M"); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
