// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Text `eval` is running is a place of its own in this shell: a failure in it
// is located at `(eval)` and the line within the evaluated text, wherever the
// `eval` was written.
//
// Measured on zsh 5.9.2, 2026-09-12, `env -i PATH=/usr/bin:/bin` with a
// scratch HOME and ZDOTDIR, `-f` over a script file. `eval` pushes no frame —
// `$0`, the call stack and `return` all see straight through it — so this is
// a third kind of place a line can be read from rather than a frame (#2133).
func TestAFailureInsideEvalIsLocatedAtTheEvaluatedText(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"at the top level", `eval 'zznotacommand'`, "(eval):1: command not found: zznotacommand"},
		{"inside a function", `q() { eval 'zznotacommand' }; q`, "(eval):1: command not found: zznotacommand"},
		{"inside an eval", `eval 'eval "zzt"'`, "(eval):1: command not found: zzt"},
		// The line is counted from the top of the evaluated text and not
		// from the top of the file, which the three lines in front of the
		// `eval` are what separate.
		{"the line is the text's", ":\n:\n:\neval '\n:\nzznotacommand'", "(eval):3: command not found: zznotacommand"},
		// A builtin still stands between the name and the line.
		{"a builtin speaking", `eval 'cd /nonexistent-zz'`, "(eval):cd:1: no such file or directory: /nonexistent-zz"},
		{"an unset parameter", `eval 'echo ${zzz?boom}'`, "(eval):1: zzz: boom"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src+"\n")
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: %q\n got %q\nwant %q", tc.name, tc.src, got, tc.want)
		}
	}
}

// And the innermost text is what answers, which is what makes this a place
// rather than a flag: a function called from evaluated text is named as the
// function, and a file sourced from it as the file. Measured in the same run.
func TestWhatEvaluatedTextCallsIsNamedForItself(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "inc"), []byte("zznotacommand\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, src, want string }{
		{"a function", "q() {\n  zznotacommand\n}\neval 'q'", "q:1: command not found: zznotacommand"},
		{"a sourced file", `eval 'source ./inc'`, "./inc:1: command not found: zznotacommand"},
		{"a file sourced by a function", "q() { source ./inc }\neval 'q'", "./inc:1: command not found: zznotacommand"},
	} {
		out, _ := runZsh(t, dir, tc.src+"\n")
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: %q\n got %q\nwant %q", tc.name, tc.src, got, tc.want)
		}
	}
}
