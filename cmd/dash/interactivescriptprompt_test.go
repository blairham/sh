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

// `dash -i FILE` reads the file at a prompt: PS1 before each line that starts
// a command, PS2 before each that continues one, the file's own line numbers,
// and the closing newline at the end of the input. `-i -c` draws nothing
// (#6805).
//
// Measured 2026-10-16 on the macOS /bin/dash, `env -i PS1='P> ' PS2='C> '
// dash -i FILE </dev/null`; every expected string is that shell's, after the
// job-control remark this test strips. This shell drew nothing for any of
// them.
func TestAnInteractiveDashScriptIsReadAtAPrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	t.Setenv("PS1", "P> ")
	t.Setenv("PS2", "C> ")
	t.Chdir(home)
	for _, c := range []struct {
		name, text      string
		wantOut, wantEr string
		wantCode        int
	}{
		{"two lines", "echo a\necho b\n", "a\nb\n", "P> P> P> \n", 0},
		{
			"a construct, a failure, and the file's numbering",
			"echo a\nif true\nthen echo $LINENO\nfi\nnosuch_dq\necho $LINENO\n",
			"a\n3\n6\n", "P> P> C> C> P> t.sh: 5: nosuch_dq: not found\nP> P> \n", 0,
		},
		{
			"a blank line, a comment, a here-document and an exit",
			"echo a; echo b\n\n# c\ncat <<E\nx\nE\nexit 3\necho no\n",
			"a\nb\nx\n", "P> P> P> P> C> C> P> ", 3,
		},
		{
			"a substitution over two lines, and PS1 changed by a line",
			"echo $(\necho z)\nx=1 PS1=\"Q> \"\necho done\n",
			"z\ndone\n", "P> C> P> Q> Q> \n", 0,
		},
		{"a backslash-continued line", "echo a \\\nb\necho c\n", "a b\nc\n", "P> C> P> P> \n", 0},
		{
			"a line that will not parse", "echo a\n)\necho b\n", "a\n",
			"P> P> t.sh: 2: Syntax error: \")\" unexpected\nP> \n", 2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(home, "t.sh"), []byte(c.text), 0o600); err != nil {
				t.Fatal(err)
			}
			var o, e bytes.Buffer
			sh := shell()
			sh.SystemStartupDirectory = t.TempDir()
			sh.Stdout, sh.Stderr = &o, &e
			null, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = null.Close() })
			sh.Stdin = null
			code := driver.MainArgs(sh, []string{"dash", "-i", "t.sh"})
			errs := e.String()
			if i := strings.Index(errs, "job control turned off\n"); i >= 0 {
				errs = errs[i+len("job control turned off\n"):]
			}
			if o.String() != c.wantOut || errs != c.wantEr || code != c.wantCode {
				t.Errorf("got (%q, %q, %d), want (%q, %q, %d)", o.String(), errs, code, c.wantOut, c.wantEr, c.wantCode)
			}
		})
	}
	// And the command string draws nothing: the pair that says the route is
	// the noun.
	var o, e bytes.Buffer
	sh := shell()
	sh.SystemStartupDirectory = t.TempDir()
	sh.Stdout, sh.Stderr = &o, &e
	code := driver.MainArgs(sh, []string{"dash", "-i", "-c", "echo a; echo b"})
	if strings.Contains(e.String(), "P> ") || o.String() != "a\nb\n" || code != 0 {
		t.Errorf("-i -c: got (%q, %q, %d), want no prompt", o.String(), e.String(), code)
	}
}
