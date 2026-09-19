// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// A function defined under a name the alias table holds is refused outright
// here, before any parse of the body, in this shell's own words and then the
// ordinary parse failure at the parentheses.
//
// Measured 2026-09-18 on zsh 5.9.2, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on /dev/null. With
// `alias zz='typeset -n'` on line 1 and `printf 'after\n'` behind it:
//
//	zz() { :; }    defining function based on alias `zz'
//	               parse error near `()'                      1
//	zz () { :; }   the same                                   1
//
// and the same two lines under `alias zz=echo`, whose expansion **would**
// have been a legal definition — which is what says it is the name being an
// alias that is refused rather than anything about the value.
//
// This shell expanded and then read `typeset -n () { :; }` as a multi-name
// definition, which is a legal shape here, so two functions called `typeset`
// and `-n` were defined in silence at 0 (#3643).
func TestAFunctionNamedByAnAliasIsRefused(t *testing.T) {
	if got := zsh.Dialect().AliasAtAFunctionName; got != syntax.AliasRefusesAFunctionName {
		t.Fatalf("AliasAtAFunctionName is %v, want AliasRefusesAFunctionName", got)
	}
	table := func(name string) (string, bool) {
		v, ok := map[string]string{"zz": "typeset -n", "ee": "echo"}[name]
		return v, ok
	}
	for _, c := range []struct{ name, src string }{
		{"the paren adjacent", "zz() { :; }\n"},
		{"a blank before it", "zz () { :; }\n"},
		{"a value that would have been legal", "ee() { :; }\n"},
	} {
		p := syntax.NewParser(c.src, zsh.Dialect())
		p.Aliases = table
		p.Parse()
		err := p.Err()
		if err == nil {
			t.Errorf("%s: parsed, want the refusal", c.name)
			continue
		}
		remarks := p.Remarks()
		if len(remarks) != 1 || remarks[0].Kind != syntax.RemarkFunctionNameIsAnAlias {
			t.Errorf("%s: remarks %v, want one RemarkFunctionNameIsAnAlias", c.name, remarks)
			continue
		}
		// The sentence, in this dialect's words, and the parse failure
		// behind it naming the pair — which is the shell's second line.
		dg := zsh.Diagnostics()
		if got := dg.Remark(remarks[0]); !strings.Contains(got, "defining function based on alias `") {
			t.Errorf("%s: remark reads %q", c.name, got)
		}
		if got := dg.ParseDiagnostic("zsh", c.src, err, c.src); !strings.Contains(got, "parse error near `()'") {
			t.Errorf("%s: failure reads %q", c.name, got)
		}
	}
	// The control: a name the table does not hold is an ordinary definition,
	// with nothing said.
	p := syntax.NewParser("plain() { :; }\n", zsh.Dialect())
	p.Aliases = table
	p.Parse()
	if err := p.Err(); err != nil {
		t.Errorf("a name the table does not hold: %v, want the definition", err)
	}
	if rs := p.Remarks(); len(rs) != 0 {
		t.Errorf("a name the table does not hold left %d remarks, want none", len(rs))
	}
	// And the keyword spelling, where the name is not a command word at all.
	p = syntax.NewParser("function zz { :; }\n", zsh.Dialect())
	p.Aliases = table
	p.Parse()
	if err := p.Err(); err != nil {
		t.Errorf("the keyword spelling: %v, want the definition", err)
	}
}
