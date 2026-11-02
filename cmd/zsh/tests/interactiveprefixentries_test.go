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

// An interactive zsh that carries on past a failed prefix value puts back the
// entries the prefix had already assigned ahead of it, in front of a special
// builtin and a function as well — where a script that the same failure ends
// leaves them. Measured 2026-11-02 on zsh 5.9.2 under `-i` with the lines on
// standard input, `env -i PATH=/usr/bin:/bin HOME=<scratch> PS1= ENV=`: `v=old`
// in every row, where an interactive ksh93 keeps `v=new` for the first two
// (#7098). See interp.Semantics.PrefixEntriesStayWhenAnInteractiveShellCarriesOn.
func TestAnInteractiveZshPutsAFailedPrefixsEntriesBack(t *testing.T) {
	t.Parallel()
	for _, command := range []string{":", "f", "true"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			out, errs := typedAtAZshPrompt(t,
				"f() { :; }\nv=old\nv=new w=${q?boom} "+command+"\necho \"v=$v\"\n")
			// The complaint is what says the prefix failed at all, so a row
			// that reads `old` is not one where nothing happened.
			if out != "v=old\n" || !strings.Contains(errs, "q: boom") {
				t.Errorf("stdout %q, want %q after `q: boom` (stderr %q)", out, "v=old\n", errs)
			}
		})
	}
}

// typedAtAZshPrompt runs `zsh -i` with typed on a pipe for its standard input,
// in an environment of the test's own rather than the process's, so that the
// rows can run side by side.
func typedAtAZshPrompt(t *testing.T, typed string) (out, errs string) {
	t.Helper()
	var o, e bytes.Buffer
	sh := scratchShell(t)
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
	driver.MainArgs(sh, []string{"zsh", "-i"})
	return o.String(), e.String()
}
