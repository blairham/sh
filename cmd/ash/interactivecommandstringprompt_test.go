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

// An interactive BusyBox ash whose `-c` string hits an error a shell that is
// not interactive would end at goes on to a prompt on standard input, with the
// error's status as `$?`; an `exit` and `set -e` still end it (#6693).
//
// Measured 2026-10-16 in
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
// (BusyBox v1.37.0), `PS1='P> ' sh -i -c …` with standard input on a pipe;
// every expected string is that shell's after the terminal remark, with its
// name in place of `ash`. This shell ended at the error.
func TestAnInteractiveAshCommandStringGoesOnToAPromptAfterAnError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	t.Setenv("PS1", "P> ")
	t.Chdir(home)
	for _, c := range []struct {
		name, src, stdin string
		params           []string
		want             string
		wantCode         int
	}{
		{"a line that will not parse", "echo )", "", nil, "ash: syntax error: unexpected \")\"\nP> \n", 2},
		{
			"and the session reads standard input", "echo )", "echo from-stdin\n", nil,
			"ash: syntax error: unexpected \")\"\nP> from-stdin\nP> \n", 0,
		},
		{"a fatal expansion", ": ${zz?boom}; echo after", "echo st=$?\n", nil, "ash: zz: boom\nP> st=2\nP> \n", 0},
		{
			"a readonly assignment", "readonly r=1; r=2; echo after", "echo st=$?\n", nil,
			"ash: r: is read only\nP> st=2\nP> \n", 0,
		},
		{
			"inside an eval, a later line", "echo x\neval \": \\${zz?boom}\"\necho after", "", nil,
			"x\nash: eval: zz: boom\nP> \n", 2,
		},
		{
			"$0 and the parameters carry over", "echo )", "echo $0 $1\n",
			[]string{"zero", "one"},
			"zero: syntax error: unexpected \")\"\nP> zero one\nP> \n", 0,
		},
		{"an exit does not", "echo x; exit 4", "echo no\n", nil, "x\n", 4},
		{"nor does set -e", "set -e; false; echo n", "echo no\n", nil, "", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			go func() {
				_, _ = w.WriteString(c.stdin)
				_ = w.Close()
			}()
			t.Cleanup(func() { _ = r.Close() })
			var o bytes.Buffer
			sh := shell()
			sh.SystemStartupDirectory = t.TempDir()
			sh.Stdout, sh.Stderr, sh.Stdin = &o, &o, r
			code := driver.MainArgs(sh, append([]string{"ash", "-i", "-c", c.src}, c.params...))
			got := o.String()
			if i := strings.Index(got, "job control turned off\n"); i >= 0 {
				got = got[i+len("job control turned off\n"):]
			}
			if got != c.want || code != c.wantCode {
				t.Errorf("got (%q, %d), want (%q, %d)", got, code, c.want, c.wantCode)
			}
		})
	}
}
