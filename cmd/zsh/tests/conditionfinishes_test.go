// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package tests_test

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
// (#5588). See interp.Semantics.ConditionFinishesAfterAFailure.
func TestAConditionFinishesAfterItsArithmeticFails(t *testing.T) {
	run := runFinishing
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

// An operand that would not expand finishes the condition the same way: the
// failed word reads as what was written ahead of its first substitution,
// every later word as the script wrote it — so nothing in one runs — and the
// shell ends with the condition's own status where a command string or a
// subshell ends. `${x?}` is not this; it ends the shell at 1 itself. Measured
// 2026-10-30 on zsh 5.9.2 under `env -i PATH=/usr/bin:/bin`, `-f` (#6722).
// See interp.Semantics.ConditionFinishesAfterAFailure.
func TestAConditionFinishesAfterAnOperandFailsToExpand(t *testing.T) {
	const bad = "bad substitution"
	for _, c := range []struct {
		src       string
		cmd, file int
		out       string
	}{
		{`[[ -z ${(P)'a b'} ]]`, 0, 1, bad},
		{`[[ -n ${(P)'a b'} ]]`, 1, 1, bad},
		{`[[ ! -n ${(P)'a b'} ]]`, 0, 1, bad},
		{`[[ ${(P)'a b'} = x ]]`, 1, 1, bad},
		{`[[ ${(P)'a b'} != x ]]`, 0, 1, bad},
		{`[[ -n ${(P)'a b'} || 1 = 1 ]]`, 0, 1, bad},
		{`[[ ${(K)x} = '' ]]`, 0, 1, "error in flags"},
		// The row that says an arithmetic expansion is this too: abandoned
		// at the word it would be 1, and `[[ ! $((1/0)) -eq 0 ]]` is 1
		// either way.
		{`[[ $((1/0)) -eq 1 ]]`, 0, 1, "division by zero"},
		{`[[ ! $((1/0)) -eq 0 ]]`, 1, 1, "division by zero"},
		{`[[ a${(P)'a b'}b = a ]]`, 0, 1, bad},
		{`[[ a$PATH${(P)'a b'} = a ]]`, 0, 1, bad},
		{`[[ -z "${(P)'a b'}" ]]`, 1, 1, bad},
		{`[[ -z ${(P)'a b'} && -n $(echo RAN >&2) ]]`, 0, 1, bad},
		{`[[ -z ${(P)'a b'} && -z "" ]]`, 1, 1, bad},
		{`[[ -z ${(P)'a b'} && $PATH = '$PATH' ]]`, 0, 1, bad},
		{`[[ -z ${(P)'a b'} && $x = $'x' ]]`, 0, 1, bad},
		{`[[ ${(P)'a b'} = * ]]`, 1, 1, bad},
		{`[[ ${(P)'a b'} != * ]]`, 0, 1, bad},
		{`[[ -z ${(P)'a b'} && x* = 'x*' ]]`, 0, 1, bad},
		{`[[ -z ${(P)'a b'} && $(echo x) = "$(echo x)" ]]`, 1, 1, bad},
		{`[[ ${(P)'a b'} =~ '' ]]`, 1, 1, bad},
		{`[[ -z ${(P)'a b'} ]] && :`, 1, 1, bad},
		{`[[ -z ${(P)'a b'} ]] || :`, 0, 1, bad},
		{`[[ -z ${x?} ]]`, 1, 1, "x: parameter not set"},
		{`[[ 1/0 -eq 1 && -n $(echo RAN >&2) ]]`, 0, 1, "division by zero"},
		{`( [[ -z ${(P)'a b'} ]] ); echo st=$?`, 0, 0, "st=0"},
		{`( [[ -n ${(P)'a b'} ]] ); echo st=$?`, 0, 0, "st=1"},
		{`eval '[[ -z ${(P)"a b"} ]]'; echo st=$?`, 0, 0, "st=1"},
		{`f() { [[ -z ${(P)'a b'} ]]; }; f`, 0, 1, bad},
	} {
		src := "(exit 5); " + c.src
		if !strings.Contains(c.src, "echo st=") {
			src += "; echo NEXT"
		}
		check := func(how, out string, code, want int) {
			t.Helper()
			if code != want || !strings.Contains(out, c.out) || strings.Contains(out, "NEXT") || strings.Contains(out, "RAN") {
				t.Errorf("%s %s\n got %q at %d, want %q at %d", how, c.src, out, code, c.out, want)
			}
		}
		out, code := runFinishing(t, "zsh", "-fc", src)
		check("-c", out, code, c.cmd)
		f := filepath.Join(t.TempDir(), "s.zsh")
		if err := os.WriteFile(f, []byte(src+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		out, code = runFinishing(t, "zsh", "-f", f)
		check("file", out, code, c.file)
	}
}

// runFinishing runs the dialect binary's front end over argv in a scratch
// environment and gives back both streams, joined, and the exit status.
func runFinishing(t *testing.T, argv ...string) (string, int) {
	t.Helper()
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	code := driver.MainArgs(sh, argv)
	return out.String() + errs.String(), code
}
