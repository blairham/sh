// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// Dialect.AliasBodyCountsLines: whether the newlines inside a substituted
// alias body are lines of the program.
//
// Both answers are in the panel, which is why this is a flag. The flag is the
// whole of what a test here may name — what each shell answers is asserted in
// its own dialect package.

// lineOfCommand parses src with the aliases given and reports the line each
// command's first word ended up on.
func lineOfCommand(t *testing.T, d syntax.Dialect, a syntax.Aliases, src string) []int {
	t.Helper()
	p := syntax.NewParser(src, d)
	p.Aliases = a
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var lines []int
	for _, st := range f.Stmts {
		pipe, ok := st.Expr.(*syntax.Pipeline)
		if !ok {
			continue
		}
		cmd, ok := pipe.Cmds[0].(*syntax.SimpleCmd)
		if !ok || len(cmd.Args) == 0 {
			continue
		}
		lines = append(lines, cmd.Args[0].Pos().Line)
	}
	return lines
}

func countingDialect(counts bool) syntax.Dialect {
	d := syntax.Core()
	d.AliasBodyCountsLines = counts
	return d
}

// The body is two commands and the word after it is on physical line 2. With
// the flag off everything the body produced sits on line 1 with the alias
// word and the next line stays where it was written; with it on the body's
// second command is on line 2 and the next line has moved to 3.
func TestAnAliasBodyNewlineMovesWhatFollowsIt(t *testing.T) {
	a := table("two", "echo one\necho body")
	const src = "two\necho after\n"

	if got, want := lineOfCommand(t, countingDialect(false), a, src), []int{1, 1, 2}; !equalInts(got, want) {
		t.Errorf("not counting: lines %v, want %v", got, want)
	}
	if got, want := lineOfCommand(t, countingDialect(true), a, src), []int{1, 2, 3}; !equalInts(got, want) {
		t.Errorf("counting: lines %v, want %v", got, want)
	}
}

// The shift is per newline, so a three-line body moves what follows by two.
func TestTheShiftIsOnePerNewlineInTheBody(t *testing.T) {
	a := table("three", "echo one\necho two\necho three")
	const src = "three\necho after\n"

	if got, want := lineOfCommand(t, countingDialect(true), a, src), []int{1, 2, 3, 4}; !equalInts(got, want) {
		t.Errorf("lines %v, want %v", got, want)
	}
}

// And it belongs to the expansion rather than to the table: an alias that is
// defined and never used moves nothing, whatever the flag says.
func TestAnAliasThatIsNotUsedMovesNothing(t *testing.T) {
	a := table("two", "echo one\necho body")
	const src = "echo first\necho after\n"

	for _, counts := range []bool{false, true} {
		if got, want := lineOfCommand(t, countingDialect(counts), a, src), []int{1, 2}; !equalInts(got, want) {
			t.Errorf("counting=%v: lines %v, want %v", counts, got, want)
		}
	}
}

// Using it twice shifts twice, which a single expansion cannot tell from a
// one-off adjustment.
func TestEachExpansionShiftsAgain(t *testing.T) {
	a := table("two", "echo one\necho body")
	const src = "two\ntwo\necho after\n"

	if got, want := lineOfCommand(t, countingDialect(true), a, src), []int{1, 2, 3, 4, 5}; !equalInts(got, want) {
		t.Errorf("lines %v, want %v", got, want)
	}
}

// LineShift is what a caller reading a program in pieces needs: the lines an
// expansion added are in no text, so counting newlines cannot find them.
func TestLineShiftReportsWhatTheExpansionsAdded(t *testing.T) {
	for _, c := range []struct {
		counts bool
		want   int
	}{{false, 0}, {true, 2}} {
		p := syntax.NewParser("two\ntwo\necho after\n", countingDialect(c.counts))
		p.Aliases = table("two", "echo one\necho body")
		p.Parse()
		if err := p.Err(); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got := p.LineShift(); got != c.want {
			t.Errorf("counting=%v: LineShift = %d, want %d", c.counts, got, c.want)
		}
	}
}

// The set of routes a dialect expands on, which is a set because one shell in
// the panel answers differently on two of the three.
func TestProgramRoutesIsASet(t *testing.T) {
	for _, c := range []struct {
		name  string
		set   syntax.ProgramRoutes
		route syntax.ProgramRoutes
		want  bool
	}{
		{"every route holds the command string", syntax.RouteOnEveryRoute, syntax.RouteFromCommandString, true},
		{"every route holds a file", syntax.RouteOnEveryRoute, syntax.RouteFromScriptFile, true},
		{"every route holds standard input", syntax.RouteOnEveryRoute, syntax.RouteOnStandardInput, true},
		{"no route holds nothing", syntax.RouteOnNoRoute, syntax.RouteFromScriptFile, false},
		{"a pair holds one of them", syntax.RouteFromScriptFile | syntax.RouteOnStandardInput, syntax.RouteFromScriptFile, true},
		{"and not the third", syntax.RouteFromScriptFile | syntax.RouteOnStandardInput, syntax.RouteFromCommandString, false},
	} {
		if got := c.set.Has(c.route); got != c.want {
			t.Errorf("%s: Has = %v, want %v", c.name, got, c.want)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
