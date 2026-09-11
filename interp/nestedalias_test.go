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

// Every place this shell reads shell source is a place an alias is expanded
// (#2096). Six parsers were built inside the interpreter and none of them was
// handed the table, so an alias that worked at the top of a script stopped
// working one level down — in a sourced file, in `eval`, in either spelling of
// a command substitution, and in a trap body.

// nestedRun runs src with aliases expanding and both readers answered, which
// is the shape every shell in the panel that expands aliases has.
func nestedRun(t *testing.T, dir, src string, byLine Answer) (string, int) {
	t.Helper()
	return nestedRunSwitch(t, dir, src, byLine, true)
}

// nestedRunSwitch is nestedRun with the expansion switch under the test's
// control, which is what a shell whose option is off looks like.
//
// A helper of its own rather than sourceRun, because the *outer* program is
// parsed by the test with no table — it is the parsers the interpreter builds
// that are under test, and giving the outer one a table too would let a pass
// come from the wrong parser.
func nestedRunSwitch(t *testing.T, dir, src string, byLine Answer, on bool) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.EvalRunsWhatItParsed = byLine
	sem.SourcedFileRunsWhatItParsed = byLine
	// `alias` is the only builtin these snippets need that asks anything of
	// its own, and none of them uses an option letter — the answers are the
	// majority's so that the row under test is the only thing being read.
	sem.AliasParsesOptions = Yes
	sem.AliasHasPrintOption = No
	sem.GlobalAliases = No
	sem.SuffixAliases = No
	sem.AliasReportsNotFound = Yes
	sem.AliasQuoting = ListingQuoteAlwaysEscaped
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: dir, Name: "testsh",
	})
	r.Vars = map[string]string{"PATH": dir}
	r.SetAliasExpansion(on)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// **The caller's alias works inside borrowed text**, in all four routes. This
// half is unanimous across the panel, so it is the core's behavior and not a
// dialect's.
func TestBorrowedTextUsesTheShellsAliases(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inc.sh", "t INSIDE-FILE\n")
	for _, c := range []struct{ name, src, want string }{
		{"a sourced file", "alias t=echo\n. ./inc.sh", "INSIDE-FILE\n"},
		{"eval", "alias t=echo\neval 't INSIDE-EVAL'", "INSIDE-EVAL\n"},
		{"a command substitution", "alias t=echo\nv=$(t SUB)\necho \"v=$v\"", "v=SUB\n"},
		{"a backquoted one", "alias t=echo\nv=`t BACK`\necho \"v=$v\"", "v=BACK\n"},
		{"a trap body", "alias t=echo\ntrap 't TRAPPED' EXIT\n:", "TRAPPED\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := nestedRun(t, dir, c.src, Yes)
			if out != c.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", c.src, out, st, c.want)
			}
		})
	}
}

// **A definition inside borrowed text reaches its own later lines only where
// the text is read as it runs.** That is the reader axis and not a second
// one: a text parsed through first has been parsed before its first line
// runs, so line 1 cannot change how line 2 reads.
func TestADefinitionInsideBorrowedTextFollowsTheReader(t *testing.T) {
	for _, c := range []struct {
		name, src string
		byLine    Answer
		want      string
	}{
		{"a file read as it runs", ". ./def.sh", Yes, "INNER\n"},
		{"a file read through first", ". ./def.sh", No, "testsh: inner: not found\n"},
		{"eval read as it runs", "eval 'alias inner=\"echo INNER\"\ninner'", Yes, "INNER\n"},
		{"eval read through first", "eval 'alias inner=\"echo INNER\"\ninner'", No, "testsh: inner: not found\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "def.sh", "alias inner='echo INNER'\ninner\n")
			out, _ := nestedRun(t, dir, c.src, c.byLine)
			if out != c.want {
				t.Errorf("%s = %q, want %q", c.src, out, c.want)
			}
		})
	}
}

// **A substitution is read through first whatever the reader says**, which is
// measured: nothing in the panel lets an alias defined on a substitution's
// first line reach its second.
//
// A guard rather than a discriminator, and worth saying so: `subst.go` parses
// with `Parse` and has no reader question in it, so no small change to it
// fails this today — the mutation that would is a substitution rebuilt around
// the run-as-you-read loop `runSourced` has. It is here because that loop is
// the obvious thing to reach for the next time a substitution needs to see
// something its own first line did, and the panel says it must not.
func TestASubstitutionIsAlwaysReadThrough(t *testing.T) {
	for _, byLine := range []Answer{Yes, No} {
		dir := t.TempDir()
		src := "v=$(alias inner='echo INNER'\ninner)\necho \"v=[$v]\""
		out, _ := nestedRun(t, dir, src, byLine)
		if !strings.Contains(out, "v=[]") {
			t.Errorf("with the reader %v, a substitution said %q, want the inner alias unexpanded", byLine, out)
		}
	}
}

// **The option is the gate, not the route.** A shell whose alias expansion is
// switched off expands nothing in borrowed text either — measured, `shopt -u
// expand_aliases` and `unsetopt aliases` both reach a sourced file and an
// `eval`. This is what the three hooks carry, and a nested parser handed the
// *tables* instead would expand past a switch that was off.
func TestBorrowedTextHonorsTheExpansionSwitch(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inc.sh", "t OFF\n")
	for _, c := range []struct {
		name string
		on   bool
		want string
	}{
		{"on", true, "OFF\n"},
		{"off", false, "testsh: t: not found\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := nestedRunSwitch(t, dir, "alias t=echo\n. ./inc.sh", Yes, c.on)
			if out != c.want {
				t.Errorf("with expansion %v, said %q, want %q", c.on, out, c.want)
			}
		})
	}
}
