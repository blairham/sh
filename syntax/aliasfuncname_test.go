// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// What a dialect does with an alias standing where a **function name** is
// being defined — see syntax.Dialect.AliasAtAFunctionName. Named for the flag
// rather than for the shells that pick each value; the presets' picks are
// asserted in dialect/.
//
// The value the alias holds is deliberately one that cannot be a function
// name, so an expansion that happens is visible as a refusal rather than as a
// definition of something else.

func parsedAndRemarked(t *testing.T, d syntax.Dialect, a syntax.Aliases, src string) (string, []syntax.Remark) {
	t.Helper()
	p := syntax.NewParser(src, d)
	p.Aliases = a
	f := p.Parse()
	if err := p.Err(); err != nil {
		return "error: " + err.Error(), p.Remarks()
	}
	return strings.TrimSpace(syntax.Print(f)), p.Remarks()
}

func TestAnAliasAtAFunctionNameIsAThreeWayAnswer(t *testing.T) {
	t.Parallel()
	alias := table("zz", "typeset -n")
	for _, c := range []struct {
		name     string
		reading  syntax.AliasAtAFunctionName
		adjacent string // `zz() { :; }`
		spaced   string // `zz () { :; }`
	}{
		{
			// Expanded like any other command word, so the definition is
			// read as `typeset -n () { … }` and refused.
			"expands", syntax.AliasExpandsAtAFunctionName, "error", "error",
		},
		{
			// The blank decides it: with none the expansion is declined and
			// the function is defined under the alias's own name; with one
			// the expansion happens and the parse fails exactly as above.
			"suppressed where the paren is adjacent",
			syntax.AliasSuppressedWhereTheParenIsAdjacent, "zz", "error",
		},
		{
			// The definition itself is refused, whichever way it is written.
			"refuses the definition", syntax.AliasRefusesAFunctionName, "error", "error",
		},
	} {
		d := syntax.Core()
		d.AliasAtAFunctionName = c.reading
		for _, probe := range []struct{ src, want string }{
			{"zz() { :; }", c.adjacent},
			{"zz () { :; }", c.spaced},
		} {
			got, _ := parsedAndRemarked(t, d, alias, probe.src)
			failed := strings.HasPrefix(got, "error: ")
			if probe.want == "error" {
				if !failed {
					t.Errorf("%s on %q: parsed as %q, want a refusal", c.name, probe.src, got)
				}
				continue
			}
			if failed || !strings.Contains(got, probe.want+"()") {
				t.Errorf("%s on %q: got %q, want a definition of %q", c.name, probe.src, got, probe.want)
			}
		}
	}
}

// The refusing reading asks the *table*, which is what keeps an ordinary
// definition ordinary: `zz() { :; }` under a dialect that refuses is still a
// function when `zz` names no alias.
func TestOnlyAnAliasedNameIsRefused(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.AliasAtAFunctionName = syntax.AliasRefusesAFunctionName
	got, remarks := parsedAndRemarked(t, d, table("other", "echo"), "zz() { :; }")
	if strings.HasPrefix(got, "error: ") {
		t.Errorf("a name the table does not hold: %q, want a definition", got)
	}
	if len(remarks) != 0 {
		t.Errorf("a name the table does not hold left %d remarks, want none", len(remarks))
	}
}

// And the refusal carries a remark in front of the parse failure, which is
// the shape one shell writes: its own sentence about the alias and then the
// ordinary complaint at the parentheses.
func TestTheRefusalRemarksBeforeItFails(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.AliasAtAFunctionName = syntax.AliasRefusesAFunctionName
	got, remarks := parsedAndRemarked(t, d, table("zz", "typeset -n"), "zz() { :; }")
	if !strings.HasPrefix(got, "error: ") {
		t.Fatalf("got %q, want a refusal", got)
	}
	if len(remarks) != 1 {
		t.Fatalf("got %d remarks, want one", len(remarks))
	}
	if remarks[0].Kind != syntax.RemarkFunctionNameIsAnAlias || remarks[0].Token != "zz" {
		t.Errorf("remark is %v/%q, want RemarkFunctionNameIsAnAlias on `zz`",
			remarks[0].Kind, remarks[0].Token)
	}
	// The value the alias holds is never read: the same refusal stands for a
	// value whose expansion would have been a legal definition.
	got, remarks = parsedAndRemarked(t, d, table("zz", "echo"), "zz() { :; }")
	if !strings.HasPrefix(got, "error: ") || len(remarks) != 1 {
		t.Errorf("a legal expansion: %q with %d remarks, want the same refusal", got, len(remarks))
	}
}

// The `function name { … }` spelling is outside all of this: the word after
// the keyword is not a command word, so no reading expands there and none
// refuses.
func TestTheKeywordSpellingIsUntouched(t *testing.T) {
	t.Parallel()
	for _, reading := range []syntax.AliasAtAFunctionName{
		syntax.AliasExpandsAtAFunctionName,
		syntax.AliasSuppressedWhereTheParenIsAdjacent,
		syntax.AliasRefusesAFunctionName,
	} {
		d := syntax.Core()
		d.FunctionKeyword = true
		d.AliasAtAFunctionName = reading
		got, _ := parsedAndRemarked(t, d, table("zz", "typeset -n"), "function zz { :; }")
		if strings.HasPrefix(got, "error: ") || !strings.Contains(got, "zz") {
			t.Errorf("%v: got %q, want the definition", reading, got)
		}
	}
}
