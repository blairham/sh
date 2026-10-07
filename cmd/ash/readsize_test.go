// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// BusyBox ash takes 1024 bytes at a time at a prompt reading a pipe, and
// 2047 of a program on standard input. Measured 2026-10-07 in the pinned
// image: `DATA` placed at byte P after `read x` is what the `read` finds only
// where P is the block, and strace shows `read(0, …, 1024)` and
// `read(0, …, 2047)` (#6328, #6329).
func TestAshReadsItsOwnBlockOfAPipe(t *testing.T) {
	data := func(at int) string {
		return "read x\n" + strings.Repeat("\n", at-7) + "DATA\necho \"[$x]\"\n"
	}
	for _, c := range []struct {
		argv []string
		at   int
		want string
	}{
		{[]string{"ash", "-i"}, 1024, "[DATA]\n"},
		{[]string{"ash", "-i"}, 1025, "[]\n"},
		{[]string{"ash"}, 2047, "[DATA]\n"},
		{[]string{"ash"}, 2046, ""},
		{[]string{"ash"}, 2048, "[]\n"},
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
