// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
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

// `posixaliases` keeps a **reserved word** out of alias expansion.
//
// The option had been remembered and ignored, so `A02alias.ztst` stopped on
// `! true` running an alias named `!` where the reference negates. The flag
// it needs already exists — it is the one bash's POSIX mode moves — so this
// is a name being wired to a rule rather than a new rule.
//
// Measured 2026-09-30 on zsh 5.9.2 from a script file, an alias defined for
// each word and the word then written where a command goes: with the option
// on the **word** runs for `!`, `if`, `then`, `while`, `do`, `for`, `case`,
// `function` and `time`, and with it off the alias does. An ordinary name
// expands either way.
//
// Two levels, because the change has two halves. The parser rows are the
// behavior — what actually runs — and the runner rows are the wiring, which
// is the half this change adds and the half a parser test cannot see.
func TestPosixAliasesKeepsAReservedWordOutOfAliasExpansion(t *testing.T) {
	table := func(name string) (string, bool) {
		v, ok := map[string]string{"!": "print GOT", "myword": "print GOT"}[name]
		return v, ok
	}
	expanded := func(t *testing.T, d syntax.Dialect, src string) bool {
		t.Helper()
		p := syntax.NewParser(src, d)
		p.Aliases = table
		f := p.Parse()
		if err := p.Err(); err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		return strings.Contains(syntax.Print(f), "GOT")
	}
	on, off := zsh.Dialect(), zsh.Dialect()
	on.AliasesExpandReservedWords = false // what `setopt posixaliases` means
	off.AliasesExpandReservedWords = true

	if expanded(t, on, "! true\n") {
		t.Error("with posixaliases the `!` alias was expanded, want the reserved word")
	}
	// The control: the same alias, the same text, the flag the other way.
	if !expanded(t, off, "! true\n") {
		t.Error("without posixaliases the `!` alias was not expanded")
	}
	// And an ordinary name is untouched either way, which is what says the
	// option is about reserved words and not about aliases.
	for _, d := range []syntax.Dialect{on, off} {
		if !expanded(t, d, "myword\n") {
			t.Error("an ordinary name did not expand")
		}
	}
}

// And the wiring: `setopt posixaliases` moves that flag, and `unsetopt` puts
// it back.
//
// The option was `recorded` — accepted, remembered, and acted on by nothing —
// so this is the half that makes the rows above reachable from a script. The
// published count of recorded names drops by one with it, which a test in
// this package enforces against `docs/spec/semantics.md`.
func TestPosixAliasesMovesTheGrammarFlag(t *testing.T) {
	d := zsh.Dialect()
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	dir := t.TempDir()
	r := &interp.Runner{
		Semantics: &sem, Diagnostics: &diag, Name: "zsh",
		Dialect: &d, Dir: dir, Vars: map[string]string{"PATH": dir},
	}
	zsh.Apply(r)
	if !r.Dialect.AliasesExpandReservedWords {
		t.Fatal("this dialect starts with reserved-word aliases off; the rows below assume on")
	}
	run := func(src string) {
		t.Helper()
		f, err := syntax.Parse(src, zsh.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	run("setopt posixaliases\n")
	if r.Dialect.AliasesExpandReservedWords {
		t.Error("setopt posixaliases left reserved-word alias expansion on")
	}
	run("unsetopt posixaliases\n")
	if !r.Dialect.AliasesExpandReservedWords {
		t.Error("unsetopt posixaliases did not put reserved-word alias expansion back")
	}
	// And the option reads back, which is a separate half: the setter moves
	// the flag and the getter reports it, and an un-inverted getter passes
	// every row above while telling a script the opposite of the truth.
	var out bytes.Buffer
	r.Stdout = &out
	run("setopt posixaliases\nif [[ -o posixaliases ]]; then print ON; else print OFF; fi\n")
	run("unsetopt posixaliases\nif [[ -o posixaliases ]]; then print ON; else print OFF; fi\n")
	if got := out.String(); got != "ON\nOFF\n" {
		t.Errorf("the option reads back %q, want \"ON\\nOFF\\n\"", got)
	}
}
