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

// An interactive BusyBox ash locates its `-c` string as a line typed at the
// prompt: a builtin's complaint names itself and no line, where `ash -c`
// names the line counted from 0. A file `.` reads keeps its line either way,
// as it does at the prompt (#6694).
//
// The noun is the shell being interactive, and the deciding pair holds the
// bytes fixed: every row below is one program run with and without `-i`.
// The prompt rows are the second rule, which the first one leans on: a
// function a file defined is located by the input being read when it fails —
// by name alone at the prompt once the file is done, with its line while
// another file is being read.
//
// Measured 2026-10-15 in
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
// (BusyBox v1.37.0), standard input /dev/null and through a pseudo-terminal
// alike; every expected line is that shell's. This shell wrote `cd: line 0:`
// under `-i -c`, and `cd: line 3:` for the function at the prompt.
func TestAnInteractiveAshLocatesItsCommandStringAsAPrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	t.Chdir(home)
	files := map[string]string{
		"s.inc": ":\ncd /nonexist_6\n",
		"g.inc": ":\ng(){\ncd /nonexist_7\n}\n",
		"d.inc": "g\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(home, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(argv []string, stdin string) string {
		t.Helper()
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			_, _ = w.WriteString(stdin)
			_ = w.Close()
		}()
		t.Cleanup(func() { _ = r.Close() })
		var o, e bytes.Buffer
		sh := shell()
		sh.SystemStartupDirectory = t.TempDir()
		sh.Stdout, sh.Stderr, sh.Stdin = &o, &e, r
		t.Setenv("PS1", "")
		t.Setenv("PS2", "")
		driver.MainArgs(sh, argv)
		// The remark about the terminal comes first under `-i`, and is not
		// what is asked here.
		return strings.TrimPrefix(e.String(), "ash: can't access tty; job control turned off\n")
	}
	const cant = "can't cd to /nonexist_%s: No such file or directory\n"
	msg := func(location, n string) string {
		return "ash: cd: " + location + strings.Replace(cant, "%s", n, 1)
	}
	for _, c := range []struct {
		name, program string
		// commandString is `ash -c`'s answer, and interactive `ash -i
		// -c`'s. Empty means not asked.
		commandString, interactive string
	}{
		{"the first line", "cd /nonexist_1", msg("line 0: ", "1"), msg("", "1")},
		{"the second line", ":\ncd /nonexist_2", msg("line 1: ", "2"), msg("", "2")},
		{"a function body", "f(){\n:\ncd /nonexist_4\n}\nf", msg("line 2: ", "4"), msg("", "4")},
		{"an EXIT trap", "trap 'cd /nonexist_5' EXIT", msg("line 0: ", "5"), msg("", "5")},
		// A file `.` reads keeps its own line under `-i` too.
		{"a file", ". ./s.inc", msg("line 2: ", "6"), msg("line 2: ", "6")},
		// And a function that file defined is the string's again once the
		// file is done. (Its `-c` line is #6696's.)
		{"a function a file defined", ". ./g.inc; g", "", msg("", "7")},
		{"the same, called from a file", ". ./g.inc; . ./d.inc", msg("line 3: ", "7"), msg("line 3: ", "7")},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.commandString != "" {
				if got := run([]string{"ash", "-c", c.program}, ""); got != c.commandString {
					t.Errorf("-c: stderr\n%s\nwant\n%s", got, c.commandString)
				}
			}
			if got := run([]string{"ash", "-i", "-c", c.program}, ""); got != c.interactive {
				t.Errorf("-i -c: stderr\n%s\nwant\n%s", got, c.interactive)
			}
		})
	}
	// The prompt ends its line at the end of the input, as BusyBox's does.
	for _, c := range []struct{ name, typed, want string }{
		{"a function a file defined, at the prompt", ". ./g.inc\ng\n", msg("", "7") + "\n"},
		{"the same, called from a file at the prompt", ". ./g.inc\n. ./d.inc\n", msg("line 3: ", "7") + "\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := run([]string{"ash", "-i"}, c.typed); got != c.want {
				t.Errorf("stderr\n%s\nwant\n%s", got, c.want)
			}
		})
	}
}
