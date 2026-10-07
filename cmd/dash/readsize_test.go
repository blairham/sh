// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// dash takes the C library's buffer at a time, at a prompt reading a pipe and
// of a program on standard input alike: 1024 bytes on macOS and 8192 in
// Debian. Measured 2026-10-07 with dash 0.5.12: `DATA` placed at byte P after
// `read x` is what the `read` finds only where P is the block, and a byte
// earlier the `read` gets the rest of the word (#6328, #6329).
func TestDashReadsTheCBufferOfAPipe(t *testing.T) {
	block := interp.ReadSizeCBuffer.Bytes()
	data := func(at int) string {
		return "read x\n" + strings.Repeat("\n", at-7) + "DATA\necho \"[$x]\"\n"
	}
	for _, c := range []struct {
		argv []string
		at   int
		want string
	}{
		{[]string{"dash", "-i"}, block, "[DATA]\n"},
		{[]string{"dash", "-i"}, block + 1, "[]\n"},
		{[]string{"dash"}, block, "[DATA]\n"},
		// The block ends one byte into `DATA`: the lines before it run
		// first, so the `read` gets `ATA` and the `D` joins the next line.
		{[]string{"dash"}, block - 1, ""},
		{[]string{"dash"}, block + 1, "[]\n"},
	} {
		if out := onAPipe(t, data(c.at), c.argv...); out != c.want {
			t.Errorf("%v, DATA at %d: stdout %q, want %q", c.argv, c.at, out, c.want)
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
