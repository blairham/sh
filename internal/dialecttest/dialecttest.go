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
