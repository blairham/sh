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

// A run-time diagnostic from `$ENV` is named by the file's path where the
// shell's name goes, while the file runs (#5884).
//
// Measured 2026-10-05 on ksh93u+ 2012-08-01 (/bin/ksh) under `env -i
// PATH=/usr/bin:/bin HOME=<scratch>`, every line below that shell's with the
// file's path standing for @. A function the file defined, called by the
// program after it, is the shell's again. This shell named itself throughout.
func TestAKshEnvDiagnosticIsNamedByTheFile(t *testing.T) {
	const env = "echo before\nnosuchcmd_q\ncd /nonexistent\nf() { nosuchF; }\nf\n"
	for _, c := range []struct {
		name  string
		argv  []string
		stdin string
		want  []string
	}{
		{"under -E", []string{"ksh", "-E", "-c", "f"}, "", []string{
			"@: line 2: nosuchcmd_q: not found",
			"@[3]: cd: /nonexistent: [No such file or directory]",
			"@: line 4: nosuchF: not found",
			"ksh: line 4: nosuchF: not found",
		}},
		{"under -i", []string{"ksh", "-i"}, "echo X\n", []string{
			"@: line 2: nosuchcmd_q: not found",
			"@[3]: cd: /nonexistent: [No such file or directory]",
			"@: line 4: nosuchF: not found",
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			path := filepath.Join(home, "env.sh")
			if err := os.WriteFile(path, []byte(env), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("ENV", path)
			in := filepath.Join(home, "typed")
			if err := os.WriteFile(in, []byte(c.stdin), 0o600); err != nil {
				t.Fatal(err)
			}
			stdin, err := os.Open(in)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = stdin.Close() })
			var o, e bytes.Buffer
			sh := shell()
			sh.SystemStartupDirectory = t.TempDir()
			sh.Stdout, sh.Stderr, sh.Stdin = &o, &e, stdin
			driver.MainArgs(sh, c.argv)
			got := e.String()
			for _, line := range c.want {
				if want := strings.ReplaceAll(line, "@", path) + "\n"; !strings.Contains(got, want) {
					t.Errorf("stderr %q, want a line %q", got, want)
				}
			}
		})
	}
}
