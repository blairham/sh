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

// An interactive ksh93 whose standard input is a descriptor it can rewind
// reads it as it reads `-i script`: no prompt, each input numbered from 1, the
// end of the input ending the line, and a syntax error ending the shell at 3
// rather than costing the line. A pipe is read at a prompt (#6659).
//
// Measured 2026-10-16 on ksh93u+ 2012-08-01, `ENV=/dev/null PS1='P> ' ksh -i`,
// the same bytes as `< in.txt` and through `cat in.txt |`; every expected
// string is that shell's. The pipe half was this shell's answer for both.
func TestAnInteractiveKshReadsARewindableStdinAsAScript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	t.Setenv("PS1", "P> ")
	t.Setenv("HISTFILE", filepath.Join(home, ".sh_history"))
	t.Chdir(home)
	const lines = "echo A=$LINENO\nif true\nthen echo B=$LINENO; fi\necho C=$LINENO\n"
	const broken = "echo a\n)\necho b\n"
	for _, c := range []struct {
		name, text      string
		file            bool
		wantOut, wantEr string
		wantCode        int
	}{
		{"a regular file", lines, true, "A=1\nB=2\nC=1\n", "\n", 0},
		{"a regular file, a syntax error", broken, true, "a\n", "ksh: syntax error: `)' unexpected\n\n", 3},
		{"a pipe", lines, false, "A=1\nB=2\nC=1\n", "P> P> > P> P> \n", 0},
		{"a pipe, a syntax error", broken, false, "a\nb\n", "P> P> ksh: syntax error: `)' unexpected\nP> P> \n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			var o, e bytes.Buffer
			sh := shell()
			sh.SystemStartupDirectory = t.TempDir()
			sh.Stdout, sh.Stderr = &o, &e
			if c.file {
				path := filepath.Join(home, "in.txt")
				if err := os.WriteFile(path, []byte(c.text), 0o600); err != nil {
					t.Fatal(err)
				}
				f, err := os.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = f.Close() })
				sh.Stdin = f
			} else {
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				go func() {
					_, _ = w.WriteString(c.text)
					_ = w.Close()
				}()
				t.Cleanup(func() { _ = r.Close() })
				sh.Stdin = r
			}
			code := driver.MainArgs(sh, []string{"ksh", "-i"})
			errs := strings.ReplaceAll(e.String(), "\r", "")
			if o.String() != c.wantOut || errs != c.wantEr || code != c.wantCode {
				t.Errorf("got (%q, %q, %d), want (%q, %q, %d)", o.String(), errs, code, c.wantOut, c.wantEr, c.wantCode)
			}
		})
	}
}
