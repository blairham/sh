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

// A line that will not parse leaves the failing status the last command left,
// on every route that reads a program a line at a time, and 1 only over a
// success (#5989). See interp.FailingStatusKeptOverAnyFailure.
//
// Measured 2026-10-05 on zsh 5.9.2 (/opt/homebrew/bin/zsh) under `-f`. The
// failure in front is `sh -c "exit 7"` because zsh's syntax status is 1, so a
// `false` there could not tell the two readings apart. This shell ended at 1
// on every failing row.
func TestAParseFailureKeepsAFailingStatus(t *testing.T) {
	for _, c := range []struct {
		pre, line string
		want      int
	}{
		{`sh -c "exit 7"`, `echo "abc`, 7},
		{`sh -c "exit 7"`, `echo ${x`, 7},
		{`sh -c "exit 7"`, `echo $(`, 7},
		{`sh -c "exit 7"`, `echo )`, 7},
		{`sh -c "exit 7"`, `if true; then`, 7},
		{`f() { return 5 }; f`, `echo )`, 5},
		{`true`, `echo )`, 1},
	} {
		src := c.pre + "\n" + c.line + "\n"
		home := scratchHome(t)
		writeHomeFile(t, home, "s.zsh", src)
		script := filepath.Join(home, "s.zsh")
		for _, route := range []struct {
			name  string
			argv  []string
			stdin string
			want  int
		}{
			{"script", []string{"zsh", "-f", script}, os.DevNull, c.want},
			{"stdin", []string{"zsh", "-f"}, script, c.want},
			// The command string is read whole before any of it runs, so
			// nothing has run and nothing is kept: 1 on every row.
			{"-c", []string{"zsh", "-f", "-c", src}, os.DevNull, 1},
		} {
			f, err := os.Open(route.stdin)
			if err != nil {
				t.Fatal(err)
			}
			var o, e bytes.Buffer
			sh := scratchShell(t)
			sh.Stdout, sh.Stderr, sh.Stdin = &o, &e, f
			got := driver.MainArgs(sh, route.argv)
			_ = f.Close()
			if got != route.want {
				t.Errorf("%s, %q then %q: status %d, want %d (stderr %q)", route.name, c.pre, c.line, got, route.want, e.String())
			}
		}
	}
}

// And at a prompt, where `$?` after the refused line is the one before it.
func TestAParseFailureAtAPromptKeepsAFailingStatus(t *testing.T) {
	scratchHome(t)
	out, errs, _ := prompt(t, "sh -c 'exit 7'\nfi\nprint -r -- st=$?\ntrue\n)\nprint -r -- st=$?\n", "zsh", "-f", "-i")
	if !strings.Contains(out, "st=7\n") || !strings.Contains(out, "st=1\n") {
		t.Errorf("stdout %q, want st=7 and then st=1 (stderr %q)", out, errs)
	}
}
