// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A `[[ ]]` whose arithmetic failed runs to its end, every later arithmetic
// in it reading 0, and the shell ends with the condition's own status where a
// command string or a subshell ends and with 1 where a script file does.
// Measured 2026-10-03 on zsh 5.9.2 under `env -i PATH=/usr/bin:/bin`, `-f`
// (#5588). See interp.Semantics.ConditionFinishesAfterAnArithmeticError.
func TestAConditionFinishesAfterItsArithmeticFails(t *testing.T) {
	run := func(t *testing.T, argv ...string) (string, int) {
		t.Helper()
		var out, errs strings.Builder
		sh := scratchShell(t)
		sh.Stdout, sh.Stderr = &out, &errs
		sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
		code := driver.MainArgs(sh, argv)
		return out.String() + errs.String(), code
	}
	const arr = "a=(x y z); "
	for _, c := range []struct {
		src       string
		cmd, file int
		out       string
	}{
		{`[[ -v "a[1/0]" ]]; echo after`, 1, 1, "division by zero"},
		{`[[ ! -v "a[1/0]" ]]; echo after`, 0, 1, "division by zero"},
		{`[[ ! ! -v "a[1/0]" ]]; echo after`, 1, 1, "division by zero"},
		{`[[ -v "a[1/0]" || 1 -eq 2 ]]; echo after`, 0, 1, "division by zero"},
		{`[[ -v "a[1/0]" && 1 -eq 1 ]]; echo after`, 1, 1, "division by zero"},
		{`[[ 1/0 -eq 1 ]]; echo after`, 0, 1, "division by zero"},
		{`[[ 1 -eq 1/0 ]]; echo after`, 1, 1, "division by zero"},
		{`( [[ ! -v "a[1/0]" ]] ); echo st=$?`, 0, 0, "st=0"},
		{`eval '[[ ! -v "a[1/0]" ]]'; echo st=$?`, 0, 0, "st=1"},
	} {
		out, code := run(t, "zsh", "-fc", arr+c.src)
		if code != c.cmd || !strings.Contains(out, c.out) || (c.out == "division by zero" && strings.Contains(out, "after")) {
			t.Errorf("-c %s\n got %q at %d, want %q at %d", c.src, out, code, c.out, c.cmd)
		}
		dir := t.TempDir()
		f := filepath.Join(dir, "s.zsh")
		if err := os.WriteFile(f, []byte(arr+c.src+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		out, code = run(t, "zsh", "-f", f)
		if code != c.file || !strings.Contains(out, c.out) {
			t.Errorf("file %s\n got %q at %d, want %q at %d", c.src, out, code, c.out, c.file)
		}
	}
}
