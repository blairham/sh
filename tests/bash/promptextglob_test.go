// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// **`shopt -s extglob` typed at the prompt reaches the next line** (#5241).
//
// The prompt read every line with the grammar it was handed at startup, so an
// extended pattern after `shopt -s extglob` was still a syntax error there.
// Measured 2026-09-30 against bash 5.3.20 under `--norc --noprofile -i`.
//
// **The pair is the point.** Without the `shopt` the same pattern is a syntax
// error in both shells; with it, bash matches. A shell that took the pattern
// either way would pass the second row alone.
func TestExtglobSetAtThePromptReachesTheNextLine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const prep = "PS1=''\nPS2=''\n"
	const line = "case ab in @(ab|cd)) echo hit;; esac\necho DONE\n"
	for _, c := range []struct {
		name, src, out string
		refused        bool
	}{
		{"without extglob", line, "DONE\n", true},
		{"after shopt -s extglob", "shopt -s extglob\n" + line, "hit\nDONE\n", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "in.sh")
			if err := os.WriteFile(path, []byte(prep+c.src), 0o600); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			var out, errs bytes.Buffer
			sh := bashShell(&out, &errs)
			sh.Stdin = f
			driver.MainArgs(sh, []string{"bash", "--norc", "--noprofile", "-i"})
			if out.String() != c.out {
				t.Errorf("out %q, want %q", out.String(), c.out)
			}
			if got := strings.Contains(errs.String(), "syntax error near unexpected token"); got != c.refused {
				t.Errorf("refused %v in %q, want %v", got, errs.String(), c.refused)
			}
		})
	}
}
