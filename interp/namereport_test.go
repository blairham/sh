// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// How a `type`-family sentence spells the name it is reporting about, and
// which names that family speaks for at all. Named for the fields rather than
// for the shells that answer them; the presets' picks are asserted in
// dialect/.

// TestANameReportQuotesTheOperandWhereTheDialectSaysSo is
// Diagnostics.NameReportQuoting: three columns write the operand exactly as
// it was given and one writes it back shell-quoted, using the same spelling
// its trace uses — which is why the alphabet is TraceMetacharacters read
// again rather than a second list.
func TestANameReportQuotesTheOperandWhereTheDialectSaysSo(t *testing.T) {
	meta := TraceMetacharacters{Anywhere: "*?[]{}~#", Leading: "="}
	for _, c := range []struct {
		operand, bare, quoted string
	}{
		{"nosuchcmd", "nosuchcmd", "nosuchcmd"},
		{"a b", "a b", "'a b'"},
		{"x]", "x]", "'x]'"},
		// The position rule carries over from the same alphabet: a leading
		// `=` counts and a trailing one does not.
		{"=ab", "=ab", "'=ab'"},
		{"ab=", "ab=", "ab="},
		// And so does the `$'…'` spelling for a control character.
		{"a\tb", "a\tb", `$'a\tb'`},
	} {
		for _, q := range []struct {
			quoting TraceQuoting
			want    string
		}{
			{QuoteNever, c.bare},
			{QuoteDollar, c.quoted},
		} {
			out, _ := run(t, `type "`+c.operand+`"`, func(r *Runner) {
				sem := permissive()
				dg := Diagnostics{
					TypeNotFound:        "type: %[1]s: not found",
					NameReportQuoting:   q.quoting,
					TraceMetacharacters: meta,
				}
				r.Semantics, r.Diagnostics = &sem, &dg
			})
			want := "type: " + q.want + ": not found"
			if !strings.Contains(out, want) {
				t.Errorf("%q at %v: out = %q, want %q", c.operand, q.quoting, strings.TrimSpace(out), want)
			}
		}
	}
}

// TestAnAliasTheDialectDoesNotReportIsStillAnAlias is
// Runner.SetAliasNotReported: the `type` family declines to speak for the
// name while everything else about the alias stays exactly as it was.
func TestAnAliasTheDialectDoesNotReportIsStillAnAlias(t *testing.T) {
	hide := func(r *Runner) {
		sem := permissive()
		dg := Diagnostics{TypeNotFound: "type: %[1]s: not found"}
		sem.TypeNamesAnAliasOnlyWhenExpanded = No
		sem.AliasParsesOptions = No
		sem.AliasQuoting = ListingQuoteAlwaysEscaped
		sem.AliasRemembersTheNamesItNames = No
		r.Semantics, r.Diagnostics = &sem, &dg
		r.SetAlias("hidden", "echo one")
		r.SetAlias("shown", "echo two")
		r.SetAliasNotReported("hidden")
	}
	for _, c := range []struct{ name, src, want string }{
		{"the marked name is not accounted for", `type hidden`, "type: hidden: not found"},
		{"its neighbor is", `type shown`, "shown is an alias for echo two"},
		{"`command -V` agrees", `command -V hidden`, "command: hidden: not found"},
		// The alias itself is untouched: the listing has it, and a removal
		// removes it.
		{"the listing still holds it", `alias hidden`, "hidden='echo one'"},
		// A new value keeps the mark; taking the name away and defining it
		// again does not.
		{"a redefinition keeps the mark", `alias hidden='echo three'; type hidden`, "type: hidden: not found"},
		{"a removal takes it off", `unalias hidden; alias hidden=ls; type hidden`, "hidden is an alias for ls"},
	} {
		out, _ := run(t, c.src, hide)
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: out = %q, want %q", c.name, strings.TrimSpace(out), c.want)
		}
	}
}
