// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The prompt engine's seam is reachable from a prelude function and from
// nowhere else, which is the property that keeps the name a person types a
// *function* rather than a builtin this shell has and no real shell does.
//
// diagnoseCommand established the shape; these say the second member of the
// family inherits it rather than having been written beside it. Nothing here
// names a shell: the question is the seam's, not any dialect's.

// engineRun installs a prelude and an engine and then runs src, the way a
// front end does — two runs rather than one concatenation, since a prelude
// pasted onto the front of a snippet arrives as the script's own text and
// whose the function is decides the whole question.
func engineRun(t *testing.T, pre, src string, engine Builtin) (string, string, int) {
	t.Helper()
	sem := permissive()
	var out, errs bytes.Buffer
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &dg,
		Dir: t.TempDir(), Name: "testsh",
	})
	if engine != nil {
		r.SetPromptEngine(engine)
	}
	if pre != "" {
		pf, err := syntax.Parse(pre, syntax.Core())
		if err != nil {
			t.Fatalf("parse prelude %q: %v", pre, err)
		}
		r.SourcingPrelude(true)
		if _, perr := r.Run(context.Background(), pf); perr != nil {
			t.Fatalf("prelude %q: %v", pre, perr)
		}
		r.SourcingPrelude(false)
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

// engineEcho is an engine that says it ran and what it was given, so a test
// can tell "the word reached the engine" from "the word reached something".
func engineEcho(r *Runner, _ context.Context, args []string) int {
	_, _ = r.Out().Write([]byte("engine[" + strings.Join(args, ",") + "]\n"))
	return 0
}

// The prelude's function reaches it, arguments and all.
func TestAPreludeFunctionReachesThePromptEngine(t *testing.T) {
	pre := "prompt() { " + PromptEngineName + ` "$@"; }` + "\n"
	out, errs, st := engineRun(t, pre, "prompt show extra\n", engineEcho)
	if want := "engine[show,extra]\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if errs != "" || st != 0 {
		t.Errorf("stderr = %q, status = %d, want empty and 0", errs, st)
	}
}

// A script running the word reaches nothing, whether or not an engine is
// installed. That is the containment: a shell that answered the word to a
// script would have gained a command no real shell has, which is the whole
// thing this arrangement exists to avoid.
func TestAScriptCannotReachThePromptEngine(t *testing.T) {
	pre := "prompt() { " + PromptEngineName + ` "$@"; }` + "\n"
	out, errs, st := engineRun(t, pre, PromptEngineName+" show\n", engineEcho)
	if out != "" {
		t.Errorf("stdout = %q, want nothing — a script reached the engine", out)
	}
	if st == 0 {
		t.Error("status = 0, want a failure: the word is not a command a script has")
	}
	if !strings.Contains(errs, PromptEngineName) {
		t.Errorf("stderr = %q, want it to name the word that was not found", errs)
	}
}

// And a function of the person's own does not borrow the voice. The speaker
// is the prelude's declaration and not the name, so redefining `prompt`
// takes the seam away with it — the same rule that moves a diagnostic's
// voice back to a script that shadows `pushd`.
func TestARedefinedPromptFunctionCannotReachTheEngine(t *testing.T) {
	pre := "prompt() { echo prelude; }\n"
	src := "prompt() { " + PromptEngineName + ` "$@"; }` + "\nprompt show\n"
	out, _, st := engineRun(t, pre, src, engineEcho)
	if strings.Contains(out, "engine[") {
		t.Errorf("a script's own function reached the engine: %q", out)
	}
	if st == 0 {
		t.Error("status = 0, want a failure")
	}
}

// With no engine installed the word is not a command at all, even from
// inside the prelude. A front end that has not wired one has a shell with no
// prompt engine, and the honest answer is the one every other absent command
// gets.
func TestWithNoEngineInstalledTheWordIsNotACommand(t *testing.T) {
	pre := "prompt() { " + PromptEngineName + ` "$@"; }` + "\n"
	out, _, st := engineRun(t, pre, "prompt show\n", nil)
	if out != "" {
		t.Errorf("stdout = %q, want nothing", out)
	}
	if st == 0 {
		t.Error("status = 0, want a failure with no engine wired")
	}
}

// HasPromptEngine answers what a front end composing a prelude asks: is
// there anything behind the function worth defining?
func TestHasPromptEngineAnswersWhetherOneIsWired(t *testing.T) {
	// testrunner:bare — the unset field *is* the subject: a runner nobody
	// has wired an engine onto is exactly the library case this answers
	// about, and it runs nothing, so it needs no directory of its own.
	r := &Runner{}
	if r.HasPromptEngine() {
		t.Error("a fresh runner claims a prompt engine")
	}
	r.SetPromptEngine(engineEcho)
	if !r.HasPromptEngine() {
		t.Error("a runner with an engine installed says it has none")
	}
}
