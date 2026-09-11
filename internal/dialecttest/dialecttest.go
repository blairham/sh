// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package dialecttest builds the [interp.Runner] a dialect package's own tests
// run their snippets on.
//
// It exists because of one field. [interp.Runner.Dialect] is nil-means-core,
// and it is read at every nested parse — an `eval`, a command substitution, a
// sourced file, a trap body, a re-parsed function body — as well as by pattern
// alternation, extended patterns and float arithmetic. A dialect suite whose
// runner was built without it asserts the *core's* answer while claiming to
// assert the dialect's, and nothing says so: the snippet runs, the status is a
// number, and only a case that reaches one of those sixteen readers comes back
// wrong. That is #849, found the long way round by #826, and #860 found the
// same omission in nineteen more files.
//
// Nineteen correct call sites do not fix that, because the twentieth is
// written by copying one of the several near-identical helpers each dialect
// package grew — `answersRun`, `refuseInScript`, `runBash`/`runDash`/`runKsh`
// /`runZsh`, all the same six lines with a different package name. A second
// helper is how the omission spreads. So there is one constructor here, it
// takes the dialect as part of the preset rather than as an optional field,
// and there is no way to get a runner out of it that has not been told which
// shell it is. TestEveryRunnerUnderDialectIsToldWhichShellItIs, in this
// package, is the other half: it reads every Go file under dialect/ and fails
// on an [interp.Runner] literal built by hand without the field.
package dialecttest

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Preset is one dialect package's four exported vectors, as function values so
// that every runner gets its own mutable Semantics and Diagnostics rather than
// a copy shared with the last test that changed one.
//
// A dialect package builds one of these once, in its own test files:
//
//	var preset = dialecttest.Preset{
//		Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics,
//		Diagnostics: bash.Diagnostics, Apply: bash.Apply,
//	}
type Preset struct {
	// Name is `$0` — what the shell calls itself in a diagnostic. Several
	// tests are about exactly that string, so it is part of the preset.
	Name string

	Dialect     func() syntax.Dialect
	Semantics   func() interp.Semantics
	Diagnostics func() interp.Diagnostics
	Apply       func(*interp.Runner)

	// Prelude is the dialect written as shell, for the cases that are about
	// what it defines. A function value like the rest, and read only by
	// [Preset.CombinedWithPrelude].
	Prelude func() string
}

// Base is what varies between one dialect test and the next. Everything a
// dialect *is* comes from the [Preset]; everything a case needs is here.
type Base struct {
	Stdout, Stderr io.Writer
	// Name overrides Preset.Name when set, for the tests about a shell
	// invoked under another name.
	Name string
	Dir  string
	Vars map[string]string
	Env  []string
	// Interactive makes the runner one, for the behaviors a shell keeps for
	// a person. `autocd` is the case this was added for: with the option on,
	// bash 5.3.15 reads a bare directory name as a `cd` at a prompt and says
	// `command not found` for the same word under `-c`, so a test that could
	// only build a non-interactive runner could not tell the option working
	// from the option missing.
	//
	// False is the default because it is the route nearly every case here
	// takes, and because a runner that claimed to be interactive without one
	// would answer `$-` with an `i` no terminal backs.
	Interactive bool
	// Terminal gives the runner a terminal, which is a different fact from
	// Interactive and is the one job control turns on: `set -m` and zsh's
	// `setopt monitor` are granted with one and refused or declined without,
	// in every shell in the panel. A test that could only build a runner
	// without one could not tell the option working from the option missing.
	//
	// False is the default because a pipe is what nearly every case here
	// runs on, and because a runner claiming a terminal it does not have
	// would grant job control nothing backs.
	Terminal bool
}

// Runner returns a runner wired to all four of the preset's vectors, with
// Apply already run.
//
// The dialect is handed to the runner as well as to the parser, and both
// halves are load-bearing: the parser decides what the source *is*, and the
// runner decides what every later parse of nested input is. There is
// deliberately no way to ask this for a runner without one.
func (p Preset) Runner(b Base) *interp.Runner {
	d := p.Dialect()
	sem, diag := p.Semantics(), p.Diagnostics()
	name := p.Name
	if b.Name != "" {
		name = b.Name
	}
	r := &interp.Runner{
		Stdout: b.Stdout, Stderr: b.Stderr,
		Semantics: &sem, Diagnostics: &diag,
		Dialect: &d,
		Name:    name, Dir: b.Dir, Vars: b.Vars, Env: b.Env,
		Interactive: b.Interactive,
		Terminal:    b.Terminal,
	}
	p.Apply(r)
	return r
}

// Parse reads src with the preset's grammar, failing the test if it will not
// parse: a dialect test that cannot parse its own snippet has nothing to say
// about what the snippet means.
func (p Preset) Parse(t testing.TB, src string) *syntax.File {
	t.Helper()
	f, err := syntax.Parse(src, p.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return f
}

// Combined parses and runs src with stdout and stderr joined into one string,
// which is the shape nearly every dialect test wants — the cases here are
// about what a shell *says*, and a shell says it on whichever stream it
// chooses.
//
// b.Stdout and b.Stderr are ignored. The run error is returned rather than
// fatal, because the helpers differ on it: a case about an unsupported
// construct wants to report it as output, and a case about behavior wants the
// test to stop.
func (p Preset) Combined(t testing.TB, b Base, src string) (out string, status int, err error) {
	t.Helper()
	f := p.Parse(t, src)
	var buf strings.Builder
	b.Stdout, b.Stderr = &buf, &buf
	r := p.Runner(b)
	st, rerr := r.Run(context.Background(), f)
	return buf.String(), st, rerr
}

// CombinedWithPrelude is [Preset.Combined] with the dialect's prelude installed
// first, the way a front end installs it.
//
// Prepending Prelude() to the snippet is the other thing, and it is wrong for a
// reason that only shows up in what the shell *says*: the functions then arrive
// as the script's own, so they speak with no location in front of them and
// cannot reach the seam that gives them one. A test written that way pins the
// bare sentence and calls it the dialect's — which is how five corpus rows came
// to be graded on status alone (#603).
//
// Two runs on one runner, which is exactly what driver.Shell does: the prelude
// is not a special entry point, and the script's own line numbering starts
// again at one because the prelude was never part of its text.
func (p Preset) CombinedWithPrelude(t testing.TB, b Base, src string) (out string, status int, err error) {
	t.Helper()
	pre := p.Parse(t, p.Prelude())
	f := p.Parse(t, src)
	var buf strings.Builder
	b.Stdout, b.Stderr = &buf, &buf
	r := p.Runner(b)
	r.SourcingPrelude(true)
	if _, perr := r.Run(context.Background(), pre); perr != nil {
		t.Fatalf("prelude: %v", perr)
	}
	r.SourcingPrelude(false)
	st, rerr := r.Run(context.Background(), f)
	return buf.String(), st, rerr
}

// PromptTableInstalled fails unless a runner this preset builds answers prompt
// escapes from the very table the dialect hands the prompt drawer.
//
// The table reaches a session two ways — [interp.Runner.SetPromptStyle] from
// the dialect's Apply, and `driver.Shell.PromptStyle` from the front end — and
// the whole point of them being one value is that they agree. For a long time
// only zsh's Apply installed it, so the other three dialects had a prompt
// language that existed in `PromptStyle()` and never reached a Runner unless a
// front end happened to hand it over separately; `sh -dialect bash` drew `\u`
// and `\h` as text where `cmd/bash` drew the user and the host (#1455).
//
// Here rather than written out in each dialect's suite, and that is the rule
// this package was written for: four copies of one check is how the fifth
// dialect gets written without it. `drawn` is passed in rather than added to
// [Preset] so that a dialect cannot satisfy this by declaring the same missing
// value twice.
//
// The comparison is the whole struct, not a spot check: a row added to one
// copy and not the other is exactly the drift this guards, and only a whole
// comparison cannot miss one. Expand is the single field that cannot be
// compared by value — [reflect.DeepEqual] calls two non-nil funcs different
// however they were built — so it is compared by identity and then removed
// from both.
func (p Preset) PromptTableInstalled(t testing.TB, b Base, drawn interp.PromptStyle) {
	t.Helper()
	got := p.Runner(b).PromptStyleValue()
	drawnExpand := reflect.ValueOf(drawn.Expand).Pointer()
	gotExpand := reflect.ValueOf(got.Expand).Pointer()
	if drawn.Expand == nil {
		t.Errorf("%s: the dialect's Expand is nil, so a prompt would never be expanded", p.Name)
	}
	if gotExpand != drawnExpand {
		t.Errorf("%s: the interpreter's Expand is not the drawer's: %v against %v",
			p.Name, gotExpand, drawnExpand)
	}
	got.Expand, drawn.Expand = nil, nil
	if !reflect.DeepEqual(got, drawn) {
		t.Errorf("%s: the interpreter's prompt table is not the drawer's:\n interp: %+v\n drawer: %+v",
			p.Name, got, drawn)
	}
}
