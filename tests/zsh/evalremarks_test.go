// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// **Borrowed text says what the parser remarked about it** (#5232): a function
// defined under a name the alias table holds is refused with the same remark
// in `eval`'d and sourced text as at top level. It is the "ALIAS_FUNC_DEF off
// by default" chunk of A02alias.ztst.
//
// Measured 2026-09-30 against zsh 5.9.2 from /opt/homebrew/bin/zsh, each row a
// script file: the remark, then the parse error, both located the way the text
// names itself — `(eval):N` for `eval`'d text, the path for a sourced file.
//
// **Run through the front end with a script file, and not through runZsh.**
// runZsh does not reach this: every row here comes back from it with the
// function *defined*, no remark and no parse error, because it runs without
// the alias expansion a script gets. A table asserted through it would have
// passed against the code that dropped the remark.
func TestEvalAndSourceReportTheirRemarks(t *testing.T) {
	dir := t.TempDir()
	fn := filepath.Join(dir, "fn.sh")
	if err := os.WriteFile(fn, []byte("ba() { print no; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const refuse = "parse error near `()'\n"
	for _, c := range []struct {
		name, src, out, err string
	}{
		{
			"eval",
			"alias ba=notacommand\neval 'ba() { print no; }'\necho st=$?\n",
			"st=1\n", "(eval):1: defining function based on alias `ba'\n(eval):1: " + refuse,
		},
		{
			// zsh counts `eval`'s lines from one, so the second line is 2.
			"on the text's second line",
			"alias ba=notacommand\neval ':\nba() { print no; }'\necho st=$?\n",
			"st=1\n", "(eval):2: defining function based on alias `ba'\n(eval):2: " + refuse,
		},
		{
			// The suite's own shape: the alias defined in a subshell, which is
			// why the chunk needs a new parse to see it.
			"the suite's subshell",
			"(alias badalias=notacommand\neval 'badalias() { print does not work; }')\necho st=$?\n",
			"st=1\n", "(eval):1: defining function based on alias `badalias'\n(eval):1: " + refuse,
		},
		{
			// A sourced file is named by its path, and fails at 126.
			"a sourced file",
			"alias ba=notacommand\n. " + fn + "\necho st=$?\n",
			"st=126\n", "<d>/fn.sh:1: defining function based on alias `ba'\n<d>/fn.sh:1: " + refuse,
		},
		// The control: nothing to remark on, nothing said.
		{"clean eval", "eval 'print ok'\n", "ok\n", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, "case.sh")
			if err := os.WriteFile(path, []byte(c.src), 0o600); err != nil {
				t.Fatal(err)
			}
			var out, errs bytes.Buffer
			sh := zshShell()
			sh.Stdout, sh.Stderr = &out, &errs
			driver.MainArgs(sh, []string{"zsh", "-f", path})
			got := strings.ReplaceAll(errs.String(), dir, "<d>")
			if out.String() != c.out || got != c.err {
				t.Errorf("out %q err %q, want out %q err %q", out.String(), got, c.out, c.err)
			}
		})
	}
}
