// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package tests_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// An interactive BusyBox ash locates a failure in `eval`'s text by name alone:
// `eval:` (or the builtin that speaks) is kept and the line is not, wherever
// the eval is — typed at the prompt, inside a function, inside a file `.`
// read. A file `.` reads keeps its line there (#6611).
//
// The noun is the text being eval's in an interactive shell. The deciding
// pairs hold the bytes fixed: the same eval with and without `-i`, and the
// same line run by `eval` and by `.` at the prompt. Measured 2026-10-15 in
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
// (BusyBox v1.37.0); every expected line is that shell's. This shell wrote
// the line counted over the session at the prompt — `eval: line 3:`, `cd:
// line 5:` — and `eval: line 3: syntax error` for the parse failure.
func TestAnInteractiveAshNamesNoLineForEvalText(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	t.Chdir(home)
	files := map[string]string{
		"s3.inc": "nosuch_s3\n",
		"s2.inc": "eval \"true\nnosuch_fe\"\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(home, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const typed = "eval \"nosuch_a\"\n" +
		"eval \"true\nnosuch_b\"\n" +
		"eval \"true\ncd /nonexist_q\"\n" +
		"f(){ eval \"true\nnosuch_c\"; }\nf\n" +
		". ./s2.inc\n" +
		". ./s3.inc\n" +
		"eval \"$(cat s3.inc)\"\n" +
		"eval 'echo )'\n"
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
		return e.String()
	}
	interactive := "ash: eval: nosuch_a: not found\n" +
		"ash: eval: nosuch_b: not found\n" +
		"ash: cd: can't cd to /nonexist_q: No such file or directory\n" +
		"ash: eval: nosuch_c: not found\n" +
		"ash: eval: nosuch_fe: not found\n" +
		"ash: ./s3.inc: line 1: nosuch_s3: not found\n" +
		"ash: eval: nosuch_s3: not found\n" +
		"ash: eval: syntax error: unexpected \")\"\n"
	for _, c := range []struct {
		name, stdin string
		argv        []string
		want        string
	}{
		// The prompt ends its line at the end of the input, as BusyBox's does.
		{"at a prompt", typed, []string{"ash", "-i"}, interactive + "\n"},
		// And the string's last line is a syntax error inside `eval`, which
		// ends `-i -c` at an error — so this shell goes on to a prompt on
		// standard input, which here is empty: an empty PS1 and the closing
		// newline. Measured 2026-10-16, `sh -i -c 'eval "echo )"'` is the
		// error, `P> ` and the newline at 2 (#6693).
		{"-i -c", "", []string{"ash", "-i", "-c", typed}, interactive + "\n"},
		// The other half of the pair: the same text, not interactive, where
		// `-c` counts its lines from 0.
		{"-c", "", []string{"ash", "-c", typed}, "ash: eval: line 0: nosuch_a: not found\n" +
			"ash: eval: line 2: nosuch_b: not found\n" +
			"ash: cd: line 4: can't cd to /nonexist_q: No such file or directory\n" +
			"ash: eval: line 6: nosuch_c: not found\n" +
			"ash: eval: line 2: nosuch_fe: not found\n" +
			"ash: ./s3.inc: line 1: nosuch_s3: not found\n" +
			"ash: eval: line 10: nosuch_s3: not found\n" +
			"ash: eval: line 11: syntax error: unexpected \")\"\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := run(c.argv, c.stdin)
			// The remark about the terminal comes first under `-i`, and is
			// not what is asked here.
			got = strings.TrimPrefix(got, "ash: can't access tty; job control turned off\n")
			if got != c.want {
				t.Errorf("stderr\n%s\nwant\n%s", got, c.want)
			}
		})
	}
}
