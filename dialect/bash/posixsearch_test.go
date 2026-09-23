// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runBashSplit runs a script through this dialect with the two streams kept
// apart, because these rows are about which of them a line landed on.
func runBashSplit(t *testing.T, src string) (out, errs string) {
	t.Helper()
	f, err := syntax.Parse(src, bash.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &diag,
		Dir: t.TempDir(), Name: "bash", Dialect: presetDialect(),
	}
	bash.Apply(r)
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return o.String(), e.String()
}

// TestPosixModePutsASpecialBuiltinAheadOfAFunction — in POSIX mode a special
// builtin the shell already holds is reached **before** a function of the same
// name, in the search and in what the name-reporting builtins say the word is.
//
// The function has to be defined before the mode is entered, because the mode
// refuses the definition itself — which is the other half of the same state and
// is Semantics.SpecialBuiltinNameIsNotAFunctionName. That makes this the only
// route to a shell holding both, and it is the route the reference's own suite
// takes.
//
// Measured 2026-09-23 on bash 5.3.20. Every row below answered the function
// before #4174, so `break` in POSIX mode ran shell code where the reference
// runs the builtin — a silent wrong answer at status 0.
func TestPosixModePutsASpecialBuiltinAheadOfAFunction(t *testing.T) {
	defined := "break() { echo inside function; }\nset -o posix\n"
	for _, tc := range []struct{ name, src, want string }{
		{"type names the builtin", "type break", "break is a special shell builtin\n"},
		{"the kind letter agrees", "type -t break", "builtin\n"},
		{"command -V agrees", "command -V break", "break is a special shell builtin\n"},
		// `-a` lists every resolution the name has, in resolution order — so
		// the builtin is the *first* row and the function is still listed.
		{
			"the listing puts the builtin first",
			"type -a break",
			"break is a special shell builtin\nbreak is a function\nbreak () \n{ \n    echo inside function\n}\n",
		},
		// And the word runs the builtin. `break` with no loop around it is
		// silent in the mode, so the whole of the answer is that the
		// function's own line is not printed.
		{"the word runs the builtin", "break; echo st=$?", "st=0\n"},
		// The controls. A listing is a question about the table rather than
		// about the search, so it still names the function; and `command -v`
		// prints the bare name either way.
		{
			"declare -f still lists the function",
			"declare -f break",
			"break () \n{ \n    echo inside function\n}\n",
		},
		{"command -v is the bare name", "command -v break", "break\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs := runBashSplit(t, defined+tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if errs != "" {
				t.Errorf("stderr = %q, want nothing", errs)
			}
		})
	}
	// And outside the mode the function wins, which is what says this is the
	// mode and not the dialect: every column that lets the function exist
	// reaches it there.
	out, _ := runBashSplit(t, "break() { echo inside function; }\ntype -t break\nbreak\n")
	if want := "function\ninside function\n"; out != want {
		t.Errorf("outside the mode: got %q, want %q", out, want)
	}
	// And leaving the mode puts the function back, which the saved answer is
	// what makes possible.
	out, _ = runBashSplit(t, "break() { echo inside function; }\nset -o posix\nset +o posix\ntype -t break\n")
	if want := "function\n"; out != want {
		t.Errorf("after leaving the mode: got %q, want %q", out, want)
	}
}

// TestPosixModeWithholdsTheLoopControlSentence — `break` with no loop around it
// names the three loops under this shell's own name and says nothing at all in
// POSIX mode, status 0 either way.
//
// Measured 2026-09-23 on bash 5.3.20 and 3.2.57 alike, so it is the mode and
// not the build. zsh invoked as `sh` keeps its own sentence, which is why the
// withholding is this dialect's answer rather than the mode's — see
// Semantics.LoopControlOutsideALoopSilentInPosixMode.
func TestPosixModeWithholdsTheLoopControlSentence(t *testing.T) {
	out, errs := runBashSplit(t, "break\necho st=$?\n")
	if !strings.Contains(errs, "only meaningful in a") {
		t.Errorf("outside the mode: stderr = %q, want the sentence", errs)
	}
	if out != "st=0\n" {
		t.Errorf("outside the mode: got %q at status 0", out)
	}
	out, errs = runBashSplit(t, "set -o posix\nbreak\necho st=$?\n")
	if errs != "" {
		t.Errorf("in the mode: stderr = %q, want nothing", errs)
	}
	if out != "st=0\n" {
		t.Errorf("in the mode: got %q, want st=0", out)
	}
	// And leaving the mode puts the sentence back, which is what the saved
	// capability is for.
	_, errs = runBashSplit(t, "set -o posix\nset +o posix\nbreak\n")
	if !strings.Contains(errs, "only meaningful in a") {
		t.Errorf("after leaving the mode: stderr = %q, want the sentence", errs)
	}
}
