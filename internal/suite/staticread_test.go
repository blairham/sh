// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// The `parsed` column says `our parser read the whole file`, and for as long
// as the static read was a bare [syntax.Parse] it was saying something else:
// a shell holds an alias table before it reads a line, alias expansion happens
// while a line is parsed, and a read given no table has a grammar no binary in
// this tree runs in. One file of the ksh93 column was reported refused on that
// account while `cmd/ksh -n` read it whole (#4433).
//
// The defect is silent in one direction only — it can nothing but understate —
// so the tests here are written to fail when the table goes away again rather
// than to describe what it does when it is there.

// declarationAlias is the shape the ksh93 column tripped on: `compound` is one
// of that shell's preset aliases, so this line is `typeset -C ce=(a=1)` to
// every ksh of ours and an ordinary command word with a stray `(` to anything
// that reads without the table.
const declarationAlias = "compound ce=(a=1)\n"

// TestTheStaticReadCarriesTheDialectsPresetAliases is the measurement, with
// its own controls beside it.
//
// Three assertions and only the first is about the fix. The other two are
// there because a passing first assertion means nothing on its own: it would
// pass just as readily against a snippet the bare parser already accepts, and
// against a reader that accepts everything.
func TestTheStaticReadCarriesTheDialectsPresetAliases(t *testing.T) {
	read, ok := Find("ksh")
	if !ok {
		t.Fatal("no ksh column in the panel")
	}
	sr, have := read.StaticReader()
	if !have {
		t.Fatal("the ksh column has no dialect of ours to read with")
	}

	// The control that says the input exercises the alias route at all. A
	// line the bare parser already accepts would make the assertion below
	// pass with the table dropped, which is the one failure this whole file
	// exists to prevent.
	if _, err := syntax.Parse(declarationAlias, sr.Dial); err == nil {
		t.Fatal("a parser with no alias table accepted the declaration alias; " +
			"this case no longer exercises the table and proves nothing")
	}

	// The control that says the reader can still refuse. A reader that
	// accepted every byte would pass the assertion below and would have
	// stopped measuring anything.
	if err := sr.Parse("if true; then\n"); err == nil {
		t.Fatal("the static read accepted a truncated `if`; it can no longer report a refusal")
	}

	// The measurement.
	if err := sr.Parse(declarationAlias); err != nil {
		t.Fatalf("the static read refused a line the shell's own preset alias makes a declaration: %v", err)
	}
}

// TestDroppingTheAliasTableIsCaught is the mutant, carried in the tree rather
// than run by hand once.
//
// [StaticRead] with no shell on it *is* the old code — `syntax.Parse(src,
// dial)` and nothing else — so this is the whole regression reproduced through
// the same door the sweep uses. A change that stops building the table, or
// that hands the parser a table the runner is not holding, fails here.
func TestDroppingTheAliasTableIsCaught(t *testing.T) {
	read, ok := Find("ksh")
	if !ok {
		t.Fatal("no ksh column in the panel")
	}
	sr, have := read.StaticReader()
	if !have {
		t.Fatal("the ksh column has no dialect of ours to read with")
	}
	if sr.Shell == nil {
		t.Fatal("the ksh column's static read holds no alias table")
	}
	dropped := StaticRead{Dial: sr.Dial}
	if err := dropped.Parse(declarationAlias); err == nil {
		t.Fatal("a read with the alias table dropped accepted the declaration alias; " +
			"the table is not what makes the line parse and this test guards nothing")
	}
}

// TestAColumnReadsWithTheTableItsShellWouldHave pins the gate rather than the
// table, because the two questions behind it are separate and one of them has
// a dissenting column.
//
// bash is the dissenter and it is right to be: `expand_aliases` is off in a
// script, so a bash reading a suite file expands nothing, and a read that gave
// it a table would be the same defect pointed the other way. Every other
// column expands on a script file's route and gets one.
func TestAColumnReadsWithTheTableItsShellWouldHave(t *testing.T) {
	for _, s := range append(append([]Suite{}, Panel...), Ours...) {
		dial, ok := s.Syntax()
		if !ok {
			continue
		}
		sr, have := s.StaticReader()
		if !have {
			t.Fatalf("%s: a column with a dialect has no static read", s.Dialect)
		}
		want := dial.AliasesExpandUnlessTold &&
			dial.ExpandAliasesInProgramText.Has(syntax.RouteFromScriptFile)
		if got := sr.Shell != nil; got != want {
			t.Errorf("%s: static read holds an alias table = %v, want %v "+
				"(AliasesExpandUnlessTold=%v, script file in ExpandAliasesInProgramText=%v)",
				s.Dialect, got, want, dial.AliasesExpandUnlessTold,
				dial.ExpandAliasesInProgramText.Has(syntax.RouteFromScriptFile))
		}
	}
}
