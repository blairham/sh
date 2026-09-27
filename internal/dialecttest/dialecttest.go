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
	"fmt"
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
	// LocaleDatabase points the locale numeric data at a fixture, so that a
	// case about a locale's radix character does not depend on which locales
	// somebody has installed. Empty is the host's own. See
	// interp/localeradix.go.
	LocaleDatabase string
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
	// Rlimits gives the runner resource limits of its own, so a case about
	// `ulimit`, `limit` or `unlimit` reads a table the test wrote rather
	// than whatever the machine running it happens to have. Nil leaves the
	// runner with no limits at all, which is what a library embedder gets
	// and is the state every other case here runs in.
	Rlimits *Rlimits
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
	// LoginShell makes the runner one, which a front end reads off `argv[0]`
	// or `-l` and which two builtins turn on. `suspend` is the case this was
	// added for: zsh refuses to stop a login shell and stops any other, so a
	// test that could only build a non-login runner could see the stop and
	// never the refusal.
	//
	// False is the default because it is what every other case here wants:
	// a login shell reads different startup files and answers `$-` with an
	// `l` in the dialect that spells one.
	LoginShell bool
	// Route is where the program came from, which a front end reads off the
	// invocation and which two measured behaviors turn on: `set -A` with a
	// bad name and a declaration whose store refuses its element each leave
	// a different status behind under `-c` than from a script file, with
	// byte-identical output on both.
	//
	// RouteUnspecified is the default and is neither of the three, which is
	// what a runner built by hand has always answered — so a case that does
	// not mean to ask about the invocation is unchanged by this existing.
	Route interp.Route
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
		LocaleDatabase: b.LocaleDatabase,
		Interactive:    b.Interactive,
		Terminal:       b.Terminal,
		LoginShell:     b.LoginShell,
		Route:          b.Route,
	}
	if b.Rlimits != nil {
		b.Rlimits.apply(r)
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

// RunLinesOn reads src through r the way a front end reads a script file: one
// line at a time, running each as it parses, and stopping at the first line
// this dialect refuses.
//
// It returns the status a front end would exit with, and writes the refusal's
// diagnostic to the runner's standard error.
//
// [Preset.Parse] is the older shape and is still right for a snippet that
// parses: it reads the whole text first, so a test written on it asserts what
// a *runner* does with a tree. What it cannot model is a line one dialect
// refuses where another defers — see [syntax.Lexer.bodyRefusalRefusesTheLine],
// where a `$( … )` body refused at its closing parenthesis refuses the line it
// is written on, so the commands in front of it never run. There the whole
// text does not parse, Preset.Parse fails the test before anything runs, and
// the message and the status both come from the front end rather than from an
// expansion (#4859).
//
// One helper rather than a copy in each suite, for the reason this package
// exists at all: the near-identical local runner is how an omission spreads.
func (p Preset) RunLinesOn(t testing.TB, r *interp.Runner, src string) int {
	t.Helper()
	d := p.Diagnostics().ForScript()
	name := r.Name
	if name == "" {
		name = p.Name
	}
	ctx := context.Background()
	if f, err := syntax.Parse(src, p.Dialect()); err == nil {
		// The whole text reads, so the older shape is the right one and is
		// kept byte for byte: one tree, one Run, one teardown.
		st, err := r.Run(ctx, f)
		if err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
		return st
	}
	pr := syntax.NewParser(src, p.Dialect())
	for {
		line, ok := pr.NextLine()
		if !ok || pr.Err() != nil {
			break
		}
		// RunPart and not Run, which is what a front end reading a line at a
		// time uses: Run tears the shell down at the end of every call, so an
		// EXIT trap would fire on the first line rather than once at the end.
		if err := r.RunPart(ctx, line); err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
		if r.Exited() {
			break
		}
	}
	perr := pr.Err()
	if perr == nil {
		return r.Finish(ctx)
	}
	if _, err := fmt.Fprint(r.Stderr, d.ParseDiagnostic(name, "", perr, src)); err != nil {
		t.Fatalf("write the diagnostic: %v", err)
	}
	// The status the refusal leaves, set before the teardown so that an EXIT
	// trap sees it — which is what every column of the panel was measured
	// doing.
	r.SetExitStatus(d.StatusForParseError(perr))
	return r.Finish(ctx)
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

// CombinedThroughTheAliases is [Preset.CombinedWithPrelude] with the snippet
// parsed the way a shell parses the text it reads *next*: with the runner's
// alias tables in hand.
//
// It exists because a preset alias is not a name the interpreter knows. The
// two helpers above hand the snippet to `syntax.Parse` before the prelude has
// run — CombinedWithPrelude parses both texts up front — so a word the dialect
// ships an alias for arrives at the runner unexpanded and is answered by
// whatever builtin happens to carry the name. That is how five words ksh93 has
// only as aliases went on being graded here as builtins: the suite could not
// tell "the alias expands to the right command" from "a builtin of the same
// name does the right thing", because it never took the alias route at all
// (#3371).
//
// Two runs on one runner, exactly as CombinedWithPrelude does, and the second
// parse happens after the first run rather than beside it — which is the whole
// point, since the prelude is what puts the aliases in the table.
func (p Preset) CombinedThroughTheAliases(t testing.TB, b Base, src string) (out string, status int, err error) {
	t.Helper()
	pre := p.Parse(t, p.Prelude())
	var buf strings.Builder
	b.Stdout, b.Stderr = &buf, &buf
	r := p.Runner(b)
	// What a front end does before it reads anything: the switch the parser
	// hook reads is off until somebody sets it, and a helper that handed the
	// parser three tables it would never consult would have looked exactly
	// like the tables being empty. See driver/session.go, which sets the
	// same base from the same field.
	r.SetAliasExpansionBase(p.Dialect().AliasesExpandUnlessTold)
	r.SourcingPrelude(true)
	if _, perr := r.Run(context.Background(), pre); perr != nil {
		t.Fatalf("prelude: %v", perr)
	}
	r.SourcingPrelude(false)
	parser := r.ParseWithAliases(src, p.Dialect())
	f := parser.Parse()
	if perr := parser.Err(); perr != nil {
		t.Fatalf("parse %q: %v", src, perr)
	}
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

// Rlimits is a resource-limit table a test owns, standing in for the
// kernel's.
//
// It exists because a limit is the one piece of shell state that is neither
// the script's nor the dialect's: `ulimit -a` and `limit` read what the
// machine happens to grant, so a case written against a real limit passes on
// a laptop and fails on a runner, or the reverse. A test that supplies its
// own reads the same table everywhere — and can reach a limit no real process
// would let it set.
//
// The order is the kernel's numbering, which is what `limit 2` and
// `ulimit -N 2` address a row by, so a test that cares about those has to say
// what it is.
type Rlimits struct {
	// Order is the resources this fixture's kernel has, in its own numbering.
	Order []interp.Resource
	// Soft and Hard are the limits themselves. A resource in Order with no
	// entry here is unlimited.
	Soft, Hard map[interp.Resource]int64
}

func (l *Rlimits) apply(r *interp.Runner) {
	has := make(map[interp.Resource]bool, len(l.Order))
	for _, res := range l.Order {
		has[res] = true
	}
	get := func(res interp.Resource) (int64, int64) {
		soft, hard := interp.RlimitInfinity, interp.RlimitInfinity
		if v, ok := l.Soft[res]; ok {
			soft = v
		}
		if v, ok := l.Hard[res]; ok {
			hard = v
		}
		return soft, hard
	}
	r.RlimitOrder = l.Order
	r.HasRlimit = func(res interp.Resource) bool { return has[res] }
	r.GetRlimit = func(res interp.Resource) (int64, int64, error) {
		soft, hard := get(res)
		return soft, hard, nil
	}
	r.SetRlimit = func(res interp.Resource, soft, hard int64) error {
		if l.Soft == nil {
			l.Soft = map[interp.Resource]int64{}
		}
		if l.Hard == nil {
			l.Hard = map[interp.Resource]int64{}
		}
		l.Soft[res], l.Hard[res] = soft, hard
		return nil
	}
}
