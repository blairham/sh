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

// runZshRefusal runs argv through this binary's front end and hands back what it
// wrote and the status it exited with.
func runZshRefusal(t *testing.T, argv ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &o, &e
	code := driver.MainArgs(sh, append([]string{"zsh"}, argv...))
	return o.String(), e.String(), code
}

// Each row measured 2026-10-03 on zsh 5.9.2 under `-c`, and each is one of
// the refusals #5717 lists: the stdout and the status are the reference's, and
// the stderr is compared where the row is about its wording.
func TestRefusalsAnswerAsTheReferenceDoes(t *testing.T) {
	for _, tc := range []struct {
		name, script, out, errs string
		code                    int
	}{
		{
			// A `$` in text that is already expanded is the process id,
			// name behind it or not, so the name is the leftover operand.
			"an expanded dollar is the pid", `x=7; q='$x'; [[ $q -eq 7 ]] && printf twice || printf once`,
			"", "zsh:1: bad math expression: operator expected at `x'\n", 1,
		},
		{
			// And an `&&` behind the condition that ran to its end is 1,
			// where the condition alone is 0.
			"the condition alone", `[[ 1/0 -eq 1 ]]`, "", "zsh:1: division by zero\n", 0,
		},
		{"an && behind it", `[[ 1/0 -eq 1 ]] && :`, "", "zsh:1: division by zero\n", 1},
		{"an || behind it", `[[ 1/0 -eq 1 ]] || :`, "", "zsh:1: division by zero\n", 0},
		{
			"a newline before more operands", "[[ -n x\n-z \"\" ]] && echo yes",
			"", "zsh:1: unknown condition: -n\n", 2,
		},
		{"a newline before an operand", "[[ -n\nx ]] && echo yes", "yes\n", "", 0},
		{"a newline after a binary operator", "[[ a ==\na ]] && echo yes", "yes\n", "", 0},
		{
			"a pattern that will not compile", `p='['; [[ 'a[' =~ $p ]]; echo "st=$?"`,
			"st=1\n", "zsh:1: failed to compile regex: brackets ([ ]) not balanced\n", 0,
		},
		{
			// The function's name and not the builtin's, and the line ends.
			"a return operand that is not arithmetic", `f(){ return 3abc; }; f; echo "st=$?"`,
			"", "f: bad math expression: operator expected at `abc'\n", 0,
		},
		{"the input ends on the operator", `echo one &&`, "one\n", "", 0},
		{"and on the other one", `false ||`, "", "", 1},
		{"a pipeline never takes it", `echo one |`, "", "zsh:1: parse error near `|'\n", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, code := runZshRefusal(t, "-c", tc.script)
			if out != tc.out || code != tc.code || errs != tc.errs {
				t.Errorf("got %q / %q / %d, want %q / %q / %d", out, errs, code, tc.out, tc.errs, tc.code)
			}
		})
	}
}

// From a script file the same two endings answer 1, which is the route split
// an abandoned line already carries — and the operator at the end of the file
// still ends the list there.
func TestRefusalsFromAScriptFile(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, script, out string
		code              int
	}{
		{"a return operand", "f(){ return 3abc; }\nf\necho \"st=$?\"\n", "", 1},
		{"the file ends on the operator", "echo one &&", "one\n", 0},
		{"the operator before the next line", "echo one &&\necho two\n", "one\ntwo\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "-")+".zsh")
			if err := os.WriteFile(path, []byte(tc.script), 0o600); err != nil {
				t.Fatal(err)
			}
			out, _, code := runZshRefusal(t, path)
			if out != tc.out || code != tc.code {
				t.Errorf("got %q / %d, want %q / %d", out, code, tc.out, tc.code)
			}
		})
	}
}

// `cd -` goes where the shell last was and not where OLDPWD says, and `cd -s`
// cancels the operand's own `..` pairs before it looks for a link. Measured
// 2026-10-03 on zsh 5.9.2 beside a `link` to `real/deep`'s parent.
func TestCdAnswersAsTheReferenceDoes(t *testing.T) {
	// Physical, because backing out of a link is answered against the whole
	// path the shell stands in, and a temporary directory on macOS is reached
	// through one. See interp's operandCrossesASymlink.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "real", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	for _, tc := range []struct{ name, script, out string }{
		{"an assigned OLDPWD", `cd /; OLDPWD=/nonexistent-oldpwd; cd -; echo "st=$?"; [[ $PWD == /* && $PWD != / ]] && echo back`, "st=0\nback\n"},
		{"and ~- with it", `cd /; OLDPWD=/usr; [[ ~- != /usr ]] && echo record`, "record\n"},
		{"a pair the operand cancels", `cd -s link/../real; echo "st=$?"`, "st=0\n"},
		{"a link the canceling leaves", `cd -s real/../link 2>/dev/null; echo "st=$?"`, "st=1\n"},
		{"a link named outright", `cd -s link/deep 2>/dev/null; echo "st=$?"`, "st=1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, _ := runZshRefusal(t, "-c", tc.script)
			if out != tc.out {
				t.Errorf("got %q (stderr %q), want %q", out, errs, tc.out)
			}
		})
	}
}

// Under `posixbuiltins` an `eval` stops catching an error about how a special
// builtin was called, and still catches the rest. Measured 2026-10-03.
func TestPosixBuiltinsLetsAUsageErrorOutOfAnEval(t *testing.T) {
	for _, tc := range []struct{ name, script, out string }{
		{"off", `( eval 'set -Z' 2>/dev/null; echo alive ); echo "st=$?"`, "alive\nst=0\n"},
		{"on", `setopt posixbuiltins; ( eval 'set -Z' 2>/dev/null; echo alive ); echo "st=$?"`, "st=1\n"},
		{"on, another error", `setopt posixbuiltins; ( eval 'echo $((1/0))' 2>/dev/null; echo alive ); echo "st=$?"`, "alive\nst=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, errs, _ := runZshRefusal(t, "-c", tc.script); out != tc.out {
				t.Errorf("got %q (stderr %q), want %q", out, errs, tc.out)
			}
		})
	}
}
