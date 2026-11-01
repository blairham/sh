// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/driver"
)

// A word whose group is still open at the end of a line reads on into the
// next on standard input, as it does from a script file and from `-c`: the
// lines after it are the word's and never run as commands (#6614). Measured
// 2026-10-15 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f < s.zsh`).
func TestAnOpenGroupReadsOnIntoTheNextLineOnStandardInput(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, out, errs string }{
		{
			"print a(b\necho \"after st=$?\"\n", "",
			"zsh: bad pattern: a(b\\necho after st=0\\n\n",
		},
		{
			"/usr/bin/true a(b\necho \"after st=$?\"\necho third\n", "",
			"zsh: bad pattern: a(b\\necho after st=0\\necho third\\n\n",
		},
		// Closed on the next line, the group is one word ending in a
		// qualifier list, and the `b` in it is refused.
		{"print -r -- a(b\nc)\necho after\n", "", "zsh: unknown file attribute: b\n"},
		// The controls: a `;` ends the word inside the group, and a closed
		// group on one line leaves the next line its own.
		{"print -r -- a(b; echo x\necho y\n", "", "zsh: bad pattern: a(b\n"},
		{"print -r -- *(N)\necho after\n", "stdin.sh\nafter\n", ""},
	} {
		out, errs := zshOnStdin(t, c.src)
		if out != c.out || errs != c.errs {
			t.Errorf("%q:\n got %q %q\nwant %q %q", c.src, out, errs, c.out, c.errs)
		}
	}
}

func zshOnStdin(t *testing.T, src string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "stdin.sh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out, errs bytes.Buffer
	sh := zshShell()
	sh.Dir = dir
	sh.Stdin = f
	sh.Stdout = &out
	sh.Stderr = &errs
	driver.MainArgs(sh, []string{"zsh", "-f"})
	return out.String(), errs.String()
}
