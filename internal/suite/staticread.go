// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// StaticRead is the whole-file read the `parsed` figure comes from.
//
// It is a type rather than a call to [syntax.Parse] because a parser is not
// the whole of what a shell reads a file with. A shell holds an **alias
// table** before it reads the first line — the dialect's prelude installs it —
// and alias expansion happens while a line is *parsed*, so a table the read
// was not given is a grammar the read does not have. `syntax.Parse(src, dial)`
// and nothing else was therefore measuring a parser configuration no binary in
// this tree ever runs in: ksh93's `compound ce=(a=1)` is `typeset -C ce=(a=1)`
// to every one of our ksh binaries and an ordinary command word with a stray
// `(` to that call, so one file of the ksh93 column was reported refused while
// `cmd/ksh -n` read it whole (#4433).
//
// The defect is silent in exactly one direction — a missing table can only
// ever make the number look *worse* — which is the direction nobody
// investigates, so the table is carried here for every column rather than
// patched in for the one that noticed.
//
// What it is not: the file's own `alias` lines. A shell reads a script a line
// at a time and an `alias` on line 1 has run by the time line 2 is read; a
// whole-file read has no run time at all, and neither has `-n`, which is the
// route this figure is a statement about. The startup table is the whole of
// what both have.
type StaticRead struct {
	// Dial is the column's parser configuration, on the route a suite file
	// arrives by: every file here is run as a named script.
	Dial syntax.Dialect
	// Shell is a runner holding the dialect's startup alias table, or nil
	// where this column's shell expands no alias in a script's own text —
	// bash, whose `expand_aliases` is off until something asks.
	//
	// It is read from many goroutines at once and never written after this
	// is built: the prelude has run, and nothing a static read does can
	// reach the `alias` builtin.
	Shell *interp.Runner
}

// Parse reads src the way the column's shell reads a file, and returns why it
// refused, or nil.
//
// [interp.Runner.ExpandAliasesIn] rather than the three table fields by hand:
// that method is the one seam between the runner's tables and a parser, and a
// second copy of the three lines is how a route drifts away from every other
// one (#2096).
func (sr StaticRead) Parse(src string) error {
	p := syntax.NewParser(src, sr.Dial)
	if sr.Shell != nil {
		sr.Shell.ExpandAliasesIn(p)
	}
	p.Parse()
	return p.Err()
}

// StaticReader builds the read for a column, and reports whether this suite
// has a dialect of ours at all.
//
// The false return is [Suite.Syntax]'s: a column with no dialect binary is not
// read by this parser, and the `parsed` figure is not asked about it.
func (s Suite) StaticReader() (StaticRead, bool) {
	dial, ok := s.Syntax()
	if !ok {
		return StaticRead{}, false
	}
	// A suite file is a named script, and the route is part of the parser's
	// configuration — one dialect ends an unterminated quote at the end of a
	// command string and refuses the same text in a file. Stated rather than
	// left at the zero value, which is "no route" and is a configuration no
	// program arrives by.
	dial = dial.On(syntax.RouteFromScriptFile)
	return StaticRead{Dial: dial, Shell: preludeShell(s.Dialect, dial)}, true
}

// preludeShell runs a dialect's prelude and hands back the runner holding what
// it left behind, or nil where a script's own text expands no alias here.
//
// The two questions are the ones driver asks of the same dialect before it
// parses a program's first line, and they are separate on purpose (#2109):
// [syntax.Dialect.AliasesExpandUnlessTold] is whether this shell expands at
// all without being asked, and [syntax.Dialect.ExpandAliasesInProgramText] is
// which routes' *own text* expands. bash answers no to the first — its
// `expand_aliases` is off in a script — so the bash column reads with no table
// and is right not to; zsh answers no to the second for `-c` alone, which a
// file is not.
//
// A prelude that will not parse or will not run returns nil rather than
// failing the sweep: this is the parsed figure's configuration and not the
// column's result, and a column that reads without a table is what every
// column did before #4433. The dialect's own tests are what hold the prelude
// up — see dialect/ksh's preset tests.
func preludeShell(name string, dial syntax.Dialect) *interp.Runner {
	if !dial.AliasesExpandUnlessTold || !dial.ExpandAliasesInProgramText.Has(syntax.RouteFromScriptFile) {
		return nil
	}
	// An empty prelude is not a short circuit. A column whose shell expands
	// aliases reads with that shell's table whether or not the table has
	// anything in it, and a nil returned for "nothing to install" would make
	// the two states indistinguishable from outside — which is how a column
	// that lost its prelude would come to look like a column that never had
	// one. dash's prelude is empty today and its read holds an empty table.
	prelude, sem, dg, apply, ok := dialectParts(name)
	if !ok {
		return nil
	}
	f, err := syntax.Parse(prelude, dial)
	if err != nil {
		return nil
	}
	r := &interp.Runner{
		Route:       interp.RouteScriptFile,
		Dialect:     &dial,
		Semantics:   &sem,
		Diagnostics: &dg,
		Name:        name,
	}
	apply(r)
	r.SetAliasExpansionBase(dial.AliasesExpandUnlessTold)
	// The dialect's own text rather than a script's, which is what lets a
	// function it defines speak for the shell. Set here for the same reason
	// driver.Shell.source sets it: this is the one place a prelude is
	// installed on this side.
	r.SourcingPrelude(true)
	if err := r.RunPart(context.Background(), f); err != nil {
		return nil
	}
	r.SourcingPrelude(false)
	return r
}

// dialectParts is the rest of "which shell is this column", beside
// [Suite.Syntax], which answers the grammar half.
//
// The same switch and deliberately beside it: this package is an instrument
// and may name every shell, where a binary meant to be liftable may not — see
// internal/acpboot for that rule and for why it is a rule about binaries.
func dialectParts(name string) (prelude string, sem interp.Semantics, dg interp.Diagnostics, apply func(*interp.Runner), ok bool) {
	switch name {
	case "bash":
		return bash.Prelude(), bash.Semantics(), bash.Diagnostics(), bash.Apply, true
	case "zsh":
		return zsh.Prelude(), zsh.Semantics(), zsh.Diagnostics(), zsh.Apply, true
	case "ksh":
		return ksh.Prelude(), ksh.Semantics(), ksh.Diagnostics(), ksh.Apply, true
	case "dash":
		return dash.Prelude(), dash.Semantics(), dash.Diagnostics(), dash.Apply, true
	case "ash":
		return ash.Prelude(), ash.Semantics(), ash.Diagnostics(), ash.Apply, true
	default:
		return "", interp.Semantics{}, interp.Diagnostics{}, nil, false
	}
}
