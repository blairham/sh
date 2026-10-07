// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/blairham/sh/driver"
)

// bash 5.3.20 `-i` on a pipe takes the line and nothing past it, so a `read`
// typed at the prompt finds the next one, and an arrow that arrived in one
// burst is still one key. Measured 2026-10-07 (#6334).
func TestABashPromptOnAPipeLeavesTheNextLineForRead(t *testing.T) {
	for _, c := range []struct{ typed, want string }{
		{"read x\nDATA\necho \"[$x]\"\n", "[DATA]\n"},
		{"echo abc\x1b[DX\n", "abXc\n"},
	} {
		if out := onAPipe(t, c.typed, "bash", "-i"); out != c.want {
			t.Errorf("%q: stdout %q, want %q", c.typed, out, c.want)
		}
	}
}

// onAPipe runs the binary with typed written to its standard input in one
// write, and returns what it printed.
func onAPipe(t *testing.T, typed string, argv ...string) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	var o, e bytes.Buffer
	sh := shell()
	sh.SystemStartupDirectory = t.TempDir()
	sh.Stdout, sh.Stderr = &o, &e
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
	driver.MainArgs(sh, argv)
	return o.String()
}
