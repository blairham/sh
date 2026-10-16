// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Where a diagnostic is located when the script arrives on standard input:
// the way a line typed at a prompt is, rather than in the route's shape
// everywhere (#6622).
//
// Measured 2026-10-15 on zsh 5.9.2 under `-f`, each row piped to the shell,
// and `-f -i` gives the same answer for every row. The text read from standard
// input, and `eval`'s text outside a function, are named for the shell with no
// line. A file that text sources, and `eval`'s text inside a function, keep
// the script's location, name and line both. Ours named the first group
// `(eval):` and wrote no line for the second.
func TestStandardInputIsLocatedAsAPromptIs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	src := filepath.Join(dir, "src.zsh")
	if err := os.WriteFile(src, []byte("echo ${q?b}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	perr := filepath.Join(dir, "perr.zsh")
	if err := os.WriteFile(perr, []byte("echo )\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rows := []struct{ line, errs string }{
		{`eval "x=\${q?b}"`, "zsh: q: b\n"},
		{`eval 'echo $((1/0))'`, "zsh: division by zero\n"},
		{`eval 'eval "echo \${q?b}"'`, "zsh: q: b\n"},
		{`eval $':\necho $((1/0))'`, "zsh: division by zero\n"},
		{`eval 'echo )'`, "zsh: parse error near `)'\n"},
		{`( eval 'echo $((1/0))' )`, "zsh: division by zero\n"},
		{`x=$( eval 'echo $((1/0))' )`, "zsh: division by zero\n"},
		{`f(){ eval 'echo $((1/0))'; }; f`, "(eval):1: division by zero\n"},
		{`f(){ eval $':\necho $((1/0))'; }; f`, "(eval):2: division by zero\n"},
		{`f(){ eval 'echo )'; }; f`, "(eval):1: parse error near `)'\n"},
		{`f(){ eval 'cd /nonexistent-6622'; }; f`, "(eval):cd:1: no such file or directory: /nonexistent-6622\n"},
		{`. ` + src, src + ":1: q: b\n"},
		{`eval ". ` + src + `"`, src + ":1: q: b\n"},
		{`. ` + perr, perr + ":1: parse error near `)'\n"},
		// Controls: what the route's shape already answered.
		{`eval 'cd /nonexistent-6622'`, "cd: no such file or directory: /nonexistent-6622\n"},
		{`echo ${q?b}`, "zsh: q: b\n"},
		{`f(){ echo $((1/0)); }; f`, "f: division by zero\n"},
		{`eval 'f(){ echo ${q?b}; }; f'`, "f: q: b\n"},
	}
	for _, c := range rows {
		_, errs, _ := prompt(t, c.line+"\n", "zsh", "-f")
		if errs != c.errs {
			t.Errorf("%s\n got %q\nwant %q", c.line, errs, c.errs)
		}
	}
}
