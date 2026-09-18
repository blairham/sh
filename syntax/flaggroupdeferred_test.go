// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// deferredDialect is a grammar with no expansion flag groups that refuses an
// unreadable expansion while reading, and carries a *group's* refusal to the
// run instead.
func deferredDialect(defer_ bool) syntax.Dialect {
	d := syntax.Core()
	d.BadSubstitutionAtParseTime = true
	d.FlagGroupRefusedAtExpansion = defer_
	return d
}

// carried is the first parse failure a file holds on a node rather than
// having raised.
func carried(f *syntax.File) *syntax.Error {
	for _, st := range f.Stmts {
		pipe, ok := st.Expr.(*syntax.Pipeline)
		if !ok {
			continue
		}
		for _, c := range pipe.Cmds {
			cmd, ok := c.(*syntax.SimpleCmd)
			if !ok {
				continue
			}
			for _, w := range cmd.Args {
				for _, sp := range w.Spans {
					if sp.Param != nil && sp.Param.RefusedAtExpansion != nil {
						return sp.Param.RefusedAtExpansion
					}
				}
			}
		}
	}
	return nil
}

// A grammar without flag groups refuses `${(U)x}`, and where the flag says so
// it refuses it *later*: the parse succeeds and the failure rides on the node
// for whatever expands the word. The failure itself does not move — same
// kind, same token, same word tail — because it is the failure the parser
// already built rather than a second one written here.
func TestAFlagGroupsRefusalIsCarriedWhereTheGrammarSaysSo(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`echo ${(U)x}`,
		`echo "[${(U)x}]"`,
		`echo a${(f)x}b c`,
		`echo ${(j:|:)x}`,
	} {
		raised, err := syntax.Parse(src+"\n", deferredDialect(false))
		if err == nil {
			t.Errorf("%s: parsed with the refusal at the read, want a failure", src)
			continue
		}
		if raised != nil && carried(raised) != nil {
			t.Errorf("%s: the raising grammar also carried one", src)
		}
		f, derr := syntax.Parse(src+"\n", deferredDialect(true))
		if derr != nil {
			t.Errorf("%s: carried grammar: %v, want the parse to stand", src, derr)
			continue
		}
		got := carried(f)
		if got == nil {
			t.Errorf("%s: nothing carried, want the refusal on the node", src)
			continue
		}
		want, _ := err.(*syntax.Error)
		if want == nil {
			t.Fatalf("%s: the raised failure is not a *syntax.Error", src)
		}
		if got.Kind != want.Kind || got.Token != want.Token ||
			got.FlagGroupWordTail != want.FlagGroupWordTail || got.Msg != want.Msg {
			t.Errorf("%s: carried %+v, want the raised %+v", src, got, want)
		}
		if got.FlagGroupWordTail == "" {
			t.Errorf("%s: the word tail was not written onto the carried failure", src)
		}
	}
}

// The deferral is the group's alone: everything else this grammar cannot read
// inside a `${…}` still gives up where it is read, so a rule written over the
// whole construct would have moved four refusals that do not move.
func TestOnlyTheFlagGroupsRefusalIsCarried(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`echo ${%%%}`,
		`echo ${x!!!}`,
		`echo ${x[}`,
	} {
		f, err := syntax.Parse(src+"\n", deferredDialect(true))
		if err == nil {
			t.Errorf("%s: parsed, want the refusal still raised while reading", src)
			continue
		}
		if f != nil && carried(f) != nil {
			t.Errorf("%s: carried a refusal it should have raised", src)
		}
	}
}

// A group the rule *does* fit is a construct rather than a refusal, so the
// carrying grammar must not turn a working spelling into a deferred failure.
func TestACarriedRefusalIsNotWrittenOverAConstructThatParses(t *testing.T) {
	t.Parallel()
	d := deferredDialect(true)
	d.SubshellSubstitution = true
	f, err := syntax.Parse("echo ${(echo hi)}\n", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := carried(f); got != nil {
		t.Errorf("carried %v, want the subshell body read as itself", got)
	}
	if !strings.Contains(syntax.Print(f), "${(echo hi)}") {
		t.Errorf("printed %q, want the body written back", syntax.Print(f))
	}
}
