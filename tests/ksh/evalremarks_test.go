// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// **This dialect's two remarks stay unreachable from `eval`** (#5232).
//
// `eval` and `.` now write what the parser remarked about their text. Both of
// ksh93's remarks — an obsolete backquote, and two operators written with no
// blank between them — are ones the shell keeps to itself while it is
// running: `-n` alone writes them. `eval` only ever parses while running, so
// neither may appear from it. Measured 2026-09-30 against ksh93u+ (/bin/ksh):
// `eval 'x=`echo a`; echo $x'` writes `a` and nothing else.
//
// **The first two rows are the positive control**, and they are what make the
// last two mean anything: the same remarks *are* written, by this shell, at
// top level under `-n`. A guard that only ever saw silence could not tell
// "kept back" from "never produced".
func TestKshRemarksStayOutOfEval(t *testing.T) {
	dir := t.TempDir()
	run := func(t *testing.T, src string, args ...string) (string, string) {
		t.Helper()
		path := filepath.Join(dir, "case.sh")
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
		var out, errs bytes.Buffer
		driver.MainArgs(kshShell(&out, &errs), append(append([]string{"ksh"}, args...), path))
		return out.String(), strings.ReplaceAll(errs.String(), dir+"/", "")
	}
	for _, c := range []struct {
		name, src string
		args      []string
		out, err  string
	}{
		{
			"backquote at top level under -n", "x=`echo a`\necho $x\n",
			[]string{"-n"},
			"", "case.sh: warning: line 1: `...` obsolete, use $(...)\n",
		},
		{
			"operators at top level under -n", "true&;true\n",
			[]string{"-n"},
			"", "case.sh: warning: line 1: use space or tab to separate operators & and ;\n",
		},
		{"backquote in eval", "eval 'x=`echo a`; echo $x'\n", nil, "a\n", ""},
		{"operators in eval", "eval 'true&;true; echo done'\n", nil, "done\n", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, err := run(t, c.src, c.args...)
			if out != c.out || err != c.err {
				t.Errorf("out %q err %q, want out %q err %q", out, err, c.out, c.err)
			}
		})
	}
}
