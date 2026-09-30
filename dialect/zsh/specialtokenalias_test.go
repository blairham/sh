// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// A **global alias named for a special token** is expanded where a command
// begins.
//
// `alias -g '&&=print X;'` and a line beginning `&&` runs the value in zsh
// 5.9.2, where the token on its own is a parse error — which is what
// `A02alias.ztst` means by "we can now alias special tokens".
//
// Measured 2026-09-30, the alias defined for each token and the token then
// written at the start of a line: `&&`, `||`, `|`, `&`, `(`, `)`, `<` and `>`
// all run the value there.
//
// **The negative row is the one that keeps the grammar**, and it is not a
// formality: in operator position the token wins, so `true && print two`
// prints `two` in the reference with that same alias defined. A rule that
// expanded the token wherever it stood would pass every positive row here and
// break every `&&` in every script.
func TestAGlobalAliasNamedForASpecialToken(t *testing.T) {
	table := func(name string) (string, bool) {
		v, ok := map[string]string{"&&": "print SPLICED", "|": "print SPLICED"}[name]
		return v, ok
	}
	// The dialect needs the global kind at all; only one in the panel has it,
	// which is what gates this without a flag.
	d := zsh.Dialect()
	parse := func(t *testing.T, src string) string {
		t.Helper()
		p := syntax.NewParser(src, d)
		p.GlobalAliases = table
		f := p.Parse()
		if err := p.Err(); err != nil {
			return "ERR: " + err.Error()
		}
		return syntax.Print(f)
	}
	for _, c := range []struct {
		name, src string
		spliced   bool
	}{
		// Where a command begins, the alias claims the token.
		{"&& at the start of a line", "print one\n&& print two\n", true},
		{"| at the start of a line", "print one\n| print two\n", true},
		// And in operator position it does not: the grammar keeps it.
		{"&& between two commands", "true && print two\n", false},
		{"| between two commands", "print one | cat\n", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := parse(t, c.src)
			if strings.HasPrefix(got, "ERR: ") {
				t.Fatalf("%q: %s", c.src, got)
			}
			if has := strings.Contains(got, "SPLICED"); has != c.spliced {
				t.Errorf("%q parsed to %q; spliced=%v, want %v", c.src, got, has, c.spliced)
			}
		})
	}
}

// And a dialect without the global kind is untouched, which is what says no
// flag is needed to keep this out of the other grammars.
//
// The parser asks nothing when `GlobalAliases` is nil, so the same line is a
// parse error there — as it is in every shell in the panel but one.
func TestASpecialTokenIsNotAliasedWithoutGlobalAliases(t *testing.T) {
	p := syntax.NewParser("print one\n&& print two\n", zsh.Dialect())
	p.Parse()
	if p.Err() == nil {
		t.Error("a leading `&&` parsed with no global aliases, want the grammar's refusal")
	}
}

// A value that ends in the token it is named for stops rather than splicing
// for ever.
//
// The guard is the parser's alias **chain** and not a per-command set: a
// mutant removing the set changed no row, and one removing the chain check
// changes this one. Measured on zsh 5.9.2, `alias -g '&&=print A; &&'` with a
// line beginning `&&` is `parse error near '&&'` — the value's own trailing
// `&&` is left as the token, which is then a parse error exactly as it was
// before any alias existed.
func TestASpecialTokenAliasEndingInItselfStops(t *testing.T) {
	table := func(name string) (string, bool) {
		v, ok := map[string]string{"&&": "print A; &&"}[name]
		return v, ok
	}
	p := syntax.NewParser("print one\n&& print two\n", zsh.Dialect())
	p.GlobalAliases = table
	p.Parse()
	// It has to *stop*, and it has to stop the way the reference does. A
	// parse without the refusal would mean the trailing token was swallowed.
	if p.Err() == nil {
		t.Error("the self-referential value parsed, want the refusal the reference gives")
	}
}
