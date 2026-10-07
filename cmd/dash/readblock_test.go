// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// dash -i reads a pipe a block at a time, and an error throws away whatever
// is left of the block. Measured 2026-10-07 on dash 0.5.12, macOS and Debian,
// with the whole input written in one write (#6322):
//
//	fi / echo A                nothing after the refusal
//	echo ${x?boom} / echo A    the same
//	false / echo A             A
//
// and the same lines written one at a time, each read holding one line, run
// every line after the refusal.
func TestADashPromptLosesTheRestOfTheBlockAfterAnError(t *testing.T) {
	for _, c := range []struct{ typed, want string }{
		{"fi\necho A\n", ""},
		{"echo ${x?boom}\necho A\n", ""},
		{"false\necho A\n", "A\n"},
	} {
		if out, errs, _ := prompt(t, c.typed, "dash", "-i"); out != c.want {
			t.Errorf("%q: stdout %q, want %q (stderr %q)", c.typed, out, c.want, errs)
		}
	}
	if out, errs, _ := promptALineAtATime(t, "fi\necho A\n", "dash", "-i"); out != "A\n" {
		t.Errorf("a line at a time: stdout %q, want A (stderr %q)", out, errs)
	}
}

// aLineAtATime hands over one line per read, as a terminal does and as a
// writer that writes a line and waits does.
type aLineAtATime struct{ rest string }

func (l *aLineAtATime) Read(p []byte) (int, error) {
	if l.rest == "" {
		return 0, io.EOF
	}
	n := strings.IndexByte(l.rest, '\n') + 1
	if n == 0 {
		n = len(l.rest)
	}
	n = copy(p, l.rest[:n])
	l.rest = l.rest[n:]
	return n, nil
}

// promptALineAtATime is prompt with the input read a line per read, for a
// test about what happens to the lines after a refused one rather than about
// how many of them one read took.
func promptALineAtATime(t *testing.T, typed string, argv ...string) (out, errs string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	sh := shell()
	sh.SystemStartupDirectory = t.TempDir()
	sh.Stdout, sh.Stderr = &o, &e
	sh.Stdin = &aLineAtATime{typed}
	code = driver.MainArgs(sh, argv)
	return o.String(), e.String(), code
}
