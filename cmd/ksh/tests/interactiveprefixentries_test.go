// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package tests_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// An interactive ksh93 carries on past a failed prefix value in front of a
// special builtin or a function, and keeps the entries the prefix had already
// assigned ahead of it — as a script keeps them where the same failure ends
// it. In front of a regular builtin the failure is the command's alone in a
// script too, and the entries go back. Measured 2026-11-02 on ksh93u+
// 2012-08-01 under `-i` with the lines on standard input, `env -i
// PATH=/usr/bin:/bin HOME=<scratch> PS1= ENV=`. This shell put them back in
// every row (#7098). See
// interp.Semantics.PrefixEntriesStayWhenAnInteractiveShellCarriesOn.
func TestAnInteractiveKshKeepsAFailedPrefixsEntries(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ command, want string }{
		{":", "v=new\n"},
		{"f", "v=new\n"},
		// The control, which also proves the probe can read `old`.
		{"true", "v=old\n"},
	} {
		t.Run(c.command, func(t *testing.T) {
			t.Parallel()
			out, errs := typedAtAKshPrompt(t,
				"f() { :; }\nv=old\nv=new w=${q?boom} "+c.command+"\necho \"v=$v\"\n")
			// The complaint is what says the prefix failed at all, so a row
			// that reads `old` is not one where nothing happened.
			if out != c.want || !strings.Contains(errs, "q: boom") {
				t.Errorf("stdout %q, want %q after `q: boom` (stderr %q)", out, c.want, errs)
			}
		})
	}
}

// typedAtAKshPrompt runs `ksh -i` with typed on a pipe for its standard input,
// in an environment of the test's own rather than the process's, so that the
// rows can run side by side.
func typedAtAKshPrompt(t *testing.T, typed string) (out, errs string) {
	t.Helper()
	var o, e bytes.Buffer
	sh := shell()
	sh.SystemStartupDirectory = t.TempDir()
	sh.Stdout, sh.Stderr = &o, &e
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "PS1=", "ENV="}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString(typed)
		_ = w.Close()
	}()
	t.Cleanup(func() { _ = r.Close() })
	sh.Stdin = r
	driver.MainArgs(sh, []string{"ksh", "-i"})
	return o.String(), e.String()
}
