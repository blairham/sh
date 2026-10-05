// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cdr and chpwd_recent_dirs, in an interactive shell reading its lines from
// a pipe: the hook keeps a change only in an interactive shell, so -c cannot
// reach it. Each row of testdata/cdr.tsv is a line typed after the hook is
// installed and what standard output held, each newline written ⏎, measured
// 2026-10-05 against /opt/homebrew/bin/zsh -f -i (zsh 5.9.2) with its own
// copies, under env -i with a HOME of its own. @D is a directory holding a,
// b, c, `sp ace` and x/y, and @H is the home.
func TestCdrAnswersWhatZshAnswers(t *testing.T) {
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"a", "b", "c", "sp ace", "x/y"} {
		if err := os.MkdirAll(filepath.Join(d, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(filepath.Join("testdata", "cdr.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		row := strings.SplitN(line, "\t", 2)
		t.Run(row[0], func(t *testing.T) {
			home, err := filepath.EvalSymlinks(scratchHome(t))
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("HOME", home)
			t.Setenv("FPATH", shippedFunctionDir(t))
			fill := strings.NewReplacer("@D", d, "@H", home)
			out, _, _ := prompt(t, "PS1=''; PS2=''\n"+
				"autoload -Uz chpwd_recent_dirs cdr add-zsh-hook; add-zsh-hook chpwd chpwd_recent_dirs\n"+
				fill.Replace(row[0])+"\nexit\n", "zsh", "-f", "-i")
			want := fill.Replace(strings.ReplaceAll(row[1], "⏎", "\n"))
			if row[1] == "(nothing)" {
				want = ""
			}
			if out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
}
