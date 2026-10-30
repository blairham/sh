// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The EXIT trap runs inside the `eval` text or the sourced file the stop was
// raised in, once the function frames are left, so that text is where the
// trap's location is — its name and its line — and a `.`'s operands are its
// `$1`. See Semantics.ExitTrapRunsInsideTheExitingText (#6954).
//
// Measured 2026-10-30 on zsh 5.9.2 under `-f` from script files, with the
// three-line body the neighboring tests use; `inc.sh` holds `exit 0`. Every
// row below wrote the script's own name before, at the `.`'s line for the
// sourced rows, and the `$1` and redirection rows wrote `1=top` and `T`.
func TestTheExitTrapRunsInsideTheTextThatExited(t *testing.T) {
	const body = "trap \":\nnosuch_t\necho L=\\$LINENO\" EXIT\n"
	const miss = ": command not found: nosuch_t\n"
	for _, c := range []struct{ name, src, want string }{
		{"a subshell's, after eval", ":\n(\n" + body + ":\neval 'exit 0'\n:\n)\n:\n", "(eval):1" + miss + "L=1\n"},
		{"a subshell's, after a sourced file", ":\n(\n" + body + ":\n. ./inc.sh\n:\n)\n:\n", "./inc.sh:1" + miss + "L=1\n"},
		{"the shell's own, after eval", ":\n" + body + ":\neval ':\nexit 0'\n:\n", "(eval):2" + miss + "L=2\n"},
		{"the shell's own, after a sourced file", ":\n" + body + ":\n. ./inc.sh\n:\n", "./inc.sh:1" + miss + "L=1\n"},
		{"eval in a sourced file", body + ":\n. ./evals.sh\n:\n", "(eval):1" + miss + "L=1\n"},
		{"a sourced file in eval", body + ":\neval '. ./inc.sh'\n:\n", "./inc.sh:1" + miss + "L=1\n"},
		{"set -e in a sourced file", body + "set -e\n:\n. ./fails.sh\n:\n", "./fails.sh:2" + miss},
		{"${x?word} in eval", body + ":\neval ':\n: ${x?boom}'\n:\n", "(eval):2: x: boom\n(eval):2" + miss + "L=2\n"},
		{"a top-level return in eval", body + ":\neval ':\nreturn 0'\n:\n", "(eval):2" + miss + "L=2\n"},
		// The function's frame is left first, so the trap runs in whatever
		// text stood below the call: the text that called it, or none.
		{"a function the text calls", body + "f() { exit 0; }\n:\neval ':\nf'\n:\n", "(eval):2" + miss + "L=2\n"},
		{"a function the sourced file calls", body + ":\n. ./calls.sh\n:\n", "./calls.sh:2" + miss + "L=2\n"},
		{"eval inside a function", body + "f() { eval 'exit 0'; }\n:\nf\n:\n", "s.zsh:6" + miss + "L=6\n"},
		{"a sourced file inside a function", body + "f() { . ./inc.sh; }\n:\nf\n:\n", "s.zsh:6" + miss + "L=6\n"},
		// The same two with the caller's trap left standing through the call,
		// which `posixtraps` does: the trap is there to fire at the end of
		// the text, and the call standing below the text is what stops it.
		{"eval inside a function, posixtraps", "setopt posixtraps\n" + body + "f() { eval 'exit 0'; }\n:\nf\n:\n", "s.zsh:7" + miss + "L=7\n"},
		{"a sourced file inside a function, posixtraps", "setopt posixtraps\n" + body + "f() { . ./inc.sh; }\n:\nf\n:\n", "s.zsh:7" + miss + "L=7\n"},
		// A signal trap's body is no different: the text it runs is still
		// where the shell is, numbered as the body is from where it fired.
		{"eval in a signal trap's body", body + "trap 'eval \"exit 0\"' USR1\n:\nkill -USR1 $$\n:\n", "(eval):6" + miss + "L=6\n"},
		{"a sourced file in a signal trap's body", body + "trap '. ./inc.sh' USR1\n:\nkill -USR1 $$\n:\n", "./inc.sh:2" + miss + "L=2\n"},
		// And what the trap sees there besides its location.
		{"the sourced file's operands", "trap 'echo 1=$1' EXIT\nset -- top\n. ./inc.sh inner\n", "1=inner\n"},
		{"a redirection around the `.`", "trap 'echo T' EXIT\n{ . ./inc.sh; } >/dev/null\n", ""},
		{"a redirection around the eval", "trap 'echo T' EXIT\n{ eval 'exit 0'; } >/dev/null\n", ""},
		// Controls: a sourced file's own `return`, and a text that runs out,
		// stop nothing, and the trap runs at the end as it always did.
		{"a sourced file that returns", "trap 'echo 1=$1' EXIT\nset -- top\n. ./returns.sh inner\n", "1=top\n"},
		{"eval text that runs out", "trap 'echo 1=$1' EXIT\nset -- top\neval 'set -- ev'\n", "1=ev\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := runZshScriptBeside(t, c.src); got != c.want {
				t.Errorf("wrote %q, want %q", got, c.want)
			}
		})
	}
}

// runZshScriptBeside runs src as the script file `s.zsh`, in a directory that
// holds the files the rows above source, with standard output and error on
// one stream.
func runZshScriptBeside(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range map[string]string{
		"inc.sh":     "exit 0\n",
		"evals.sh":   "eval 'exit 0'\n",
		"fails.sh":   ":\nfalse\n",
		"calls.sh":   "g() { exit 0; }\ng\n",
		"returns.sh": "return 0\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	sem, dg := zsh.Semantics(), zsh.Diagnostics()
	r := (&interp.Runner{
		Semantics: &sem, Diagnostics: &dg,
		Stdout: &out, Stderr: &out, Dir: dir,
		Name: "s.zsh", Dialect: presetDialect(), Route: interp.RouteScriptFile,
	}).SeedVars(map[string]string{"PATH": dir})
	zsh.Apply(r)
	r.SetScriptFile("s.zsh")
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out.String()
}
