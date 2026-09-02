// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The stack is the core's and the names are the dialect's, so this asks the
// core's question: what frames is the shell inside, innermost first.
func TestTheCallStackFollowsWhereTheShellIs(t *testing.T) {
	t.Run("nothing called, and no file", func(t *testing.T) {
		// `-c` has no script frame, which is measured rather than assumed:
		// bash reports no frames at all outside a function there.
		var r *Runner
		run(t, `:`, func(rr *Runner) { r = rr })
		if got := r.CallStack(); len(got) != 0 {
			t.Errorf("stack = %v, want nothing", got)
		}
		if r.HasScriptFrame() {
			t.Error("HasScriptFrame with no file, want false")
		}
	})

	t.Run("nothing called, with a file", func(t *testing.T) {
		var r *Runner
		run(t, `:`, func(rr *Runner) { rr.SetScriptFile("/s/main.sh"); r = rr })
		got := r.CallStack()
		if len(got) != 1 || got[0].File != "/s/main.sh" || got[0].Name != "" {
			t.Errorf("stack = %v, want the script's own frame", got)
		}
		if r.InCall() {
			t.Error("InCall at the top level, want false")
		}
	})

	t.Run("inside a function", func(t *testing.T) {
		out, _ := runStack(t, `f(){ stack; }; f`, "/s/main.sh")
		// Innermost first, and the function's frame names the file it was
		// *defined* in rather than the one that called it.
		if out != "f:/s/main.sh :/s/main.sh" {
			t.Errorf("stack = %q, want the function then the script", out)
		}
	})

	t.Run("nested", func(t *testing.T) {
		out, _ := runStack(t, `g(){ stack; }; f(){ g; }; f`, "/s/main.sh")
		if out != "g:/s/main.sh f:/s/main.sh :/s/main.sh" {
			t.Errorf("stack = %q, want innermost first", out)
		}
	})

	t.Run("a function defined where there is no file", func(t *testing.T) {
		// It belongs to whatever the shell calls itself, which is what bash
		// puts there for `-c`.
		out, _ := runStack(t, `f(){ stack; }; f`, "")
		if out != "f:testsh" {
			t.Errorf("stack = %q, want the shell's own name and no script frame", out)
		}
	})
}

// A sourced file is a place a script can be *in*, so it is a frame — and the
// functions it declares remember it rather than whatever sourced them. A
// stack that only counted function calls would name the wrong file for every
// library a script uses, which is the case that matters.
func TestASourcedFileIsAFrame(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib.sh")
	if err := os.WriteFile(lib, []byte("stack\ng(){ stack; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runStack(t, `. `+lib+`; g`, "/s/main.sh")
	// While sourcing: the library, then the script. Then inside g, called
	// after the sourcing finished: g's frame still names the library.
	want := "source:" + lib + " :/s/main.sh|g:" + lib + " :/s/main.sh"
	if out != want {
		t.Errorf("stack = %q, want %q", out, want)
	}
}

// runStack runs src with a `stack` builtin that prints the frames, and
// returns the printings joined by `|`.
func runStack(t *testing.T, src, file string) (string, int) {
	t.Helper()
	var lines []string
	out, st := run(t, src, func(r *Runner) {
		r.Name = "testsh"
		if file != "" {
			r.SetScriptFile(file)
		}
		r.Register("stack", func(rr *Runner, _ context.Context, _ []string) int {
			var parts []string
			for _, f := range rr.CallStack() {
				parts = append(parts, f.Name+":"+f.File)
			}
			lines = append(lines, strings.Join(parts, " "))
			return 0
		})
	})
	_ = out
	return strings.Join(lines, "|"), st
}
