// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package tests_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/driver"
)

// An interactive BusyBox ash running a script file — `ash -i FILE` with no
// terminal — locates and ends it three ways that `ash FILE` does not (#6695):
//
//   - the remark about the terminal is located as the route locates anything,
//     at line 0: `FILE: line 0: can't access tty; …`;
//   - a subshell names no line where the shell names its own: the noun is the
//     subshell, so `{ nosuch; }` keeps `line 5:` and `(nosuch)` loses it, and
//     a file `.` reads inside one still names its line;
//   - the end of the file ends the line on standard error, as the prompt does
//     at the end of its input — and `exit`, `return` and `set -e` end the
//     program before that and write none, where a fatal error does not.
//
// Measured 2026-10-15 in
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
// (BusyBox v1.37.0), standard input /dev/null; every expected line is that
// shell's.
func TestAnInteractiveAshScriptIsLocatedAndEndedAsBusyBoxDoes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	t.Chdir(home)
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(home, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("s.inc", ":\ncd /nonexist_6\n")
	write("F.sh", ":\nnosuch_top\nx=$(nosuch_cs)\n(nosuch_sub)\n{ nosuch_br; }\n"+
		"f(){ nosuch_fn; }; x=$(f)\n( . ./s.inc )\ny=$(cd /nonexist_g)\nnosuch_p1 | :\n")
	run := func(argv ...string) string {
		t.Helper()
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_ = w.Close()
		t.Cleanup(func() { _ = r.Close() })
		var o, e bytes.Buffer
		sh := shell()
		sh.SystemStartupDirectory = t.TempDir()
		sh.Stdout, sh.Stderr, sh.Stdin = &o, &e, r
		driver.MainArgs(sh, append([]string{"ash"}, argv...))
		return e.String()
	}
	const tty = "can't access tty; job control turned off\n"
	const cant = ": No such file or directory\n"
	t.Run("the subshells of one file", func(t *testing.T) {
		want := "F.sh: line 0: " + tty +
			"F.sh: line 2: nosuch_top: not found\n" +
			"F.sh: nosuch_cs: not found\n" +
			"F.sh: nosuch_sub: not found\n" +
			"F.sh: line 5: nosuch_br: not found\n" +
			"F.sh: nosuch_fn: not found\n" +
			"F.sh: cd: line 2: can't cd to /nonexist_6" + cant +
			"F.sh: cd: can't cd to /nonexist_g" + cant +
			"F.sh: nosuch_p1: not found\n" +
			"\n"
		if got := run("-i", "F.sh"); got != want {
			t.Errorf("-i: stderr\n%s\nwant\n%s", got, want)
		}
		// The other half of the pair: the same file, not interactive.
		want = "F.sh: line 2: nosuch_top: not found\n" +
			"F.sh: line 3: nosuch_cs: not found\n" +
			"F.sh: line 4: nosuch_sub: not found\n" +
			"F.sh: line 5: nosuch_br: not found\n" +
			"F.sh: line 6: nosuch_fn: not found\n" +
			"F.sh: cd: line 2: can't cd to /nonexist_6" + cant +
			"F.sh: cd: line 8: can't cd to /nonexist_g" + cant +
			"F.sh: line 9: nosuch_p1: not found\n"
		if got := run("F.sh"); got != want {
			t.Errorf("without -i: stderr\n%s\nwant\n%s", got, want)
		}
	})
	for i, c := range []struct{ program, ending string }{
		{"echo a", "\n"},
		{": ${zz?b}; echo after", "E.sh: line 1: zz: b\n\n"},
		{"exit 3", ""},
		{"echo a; return 4; echo b", ""},
		{"set -e; false", ""},
		{"f(){ exit 2; }; f", ""},
	} {
		t.Run(c.program, func(t *testing.T) {
			write("E.sh", c.program+"\n")
			if got, want := run("-i", "E.sh"), "E.sh: line 0: "+tty+c.ending; got != want {
				t.Errorf("-i FILE: stderr %q, want %q", got, want)
			}
			if i > 0 {
				return
			}
			// And the route that is not a file: `-i -c` names no line on the
			// remark and writes no newline at its end.
			if got, want := run("-i", "-c", c.program), "ash: "+tty; got != want {
				t.Errorf("-i -c: stderr %q, want %q", got, want)
			}
		})
	}
}
