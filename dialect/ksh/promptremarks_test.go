// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// **This dialect's two remarks stay unsaid at the prompt** (#5239).
//
// The prompt now writes the remarks a finished line draws. ksh93's two — an
// obsolete backquote, and operators written with no blank between them — are
// ones it writes under `-n` alone, and a prompt is always running, so neither
// may appear there. Measured 2026-09-30 against ksh93u+ under `-i`: nothing.
//
// **The `-n` rows are the positive control.** They are the same two remarks,
// written by this shell, which is what makes the prompt rows' silence mean
// "kept back" rather than "never produced".
func TestKshRemarksStayOffThePrompt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// A control row names the file on the command line; a prompt row feeds it
	// as standard input. `-n` has to be given a file: it reads no program
	// from standard input the way a prompt does.
	run := func(t *testing.T, src string, args ...string) (string, string) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "in.sh")
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		var out, errs bytes.Buffer
		sh := kshShell(&out, &errs)
		argv := append([]string{"ksh"}, args...)
		if len(args) > 0 && args[0] == "-n" {
			argv = append(argv, path)
		} else {
			sh.Stdin = f
		}
		driver.MainArgs(sh, argv)
		return out.String(), errs.String()
	}
	for _, c := range []struct {
		name, src string
		args      []string
		warns     bool
	}{
		{"backquote, -n on a file (control)", "x=`echo a`\necho $x\n", []string{"-n"}, true},
		{"operators, -n on a file (control)", "true&;true\n", []string{"-n"}, true},
		{"backquote at the prompt", "PS1=''\nx=`echo a`; echo $x\n", []string{"-i"}, false},
		{"operators at the prompt", "PS1=''\n(:);(:)\necho done\n", []string{"-i"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, errs := run(t, c.src, c.args...)
			if got := strings.Contains(errs, "warning:"); got != c.warns {
				t.Errorf("warned %v in %q, want %v", got, errs, c.warns)
			}
		})
	}
}
