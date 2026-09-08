// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// The reserved precommand words — see Dialect.ReservedPrecommands. One shell
// in the panel has one of them, `nocorrect`, and it is grammar there: it is
// taken away before the command is read, so what follows it is still an
// assignment prefix, and quoting it or producing it from an expansion leaves
// an ordinary command name. Measured on zsh 5.9.2, 2026-09-08 (#1526).

func precommandGrammar() Dialect {
	d := Core()
	d.ReservedPrecommands = map[string]bool{"nocorrect": true}
	return d
}

// simpleOf is the first command of src, which every row here is.
func simpleOf(t *testing.T, src string, d Dialect) *SimpleCmd {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	c, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if !ok {
		t.Fatalf("%s: first command is %T, want a simple command", src, f.Stmts[0].Expr.(*Pipeline).Cmds[0])
	}
	return c
}

func words(ws []*Word) string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.Literal())
	}
	return strings.Join(out, " ")
}

// TestAReservedPrecommandIsTakenOffTheCommand: the word leaves the argument
// list, and the words behind it are read as if it had never been written —
// which is what keeps `nocorrect x=1 echo hi` an assignment prefix and a
// command, rather than a command called `x=1`.
func TestAReservedPrecommandIsTakenOffTheCommand(t *testing.T) {
	d := precommandGrammar()
	for _, tc := range []struct{ src, mods, args, assigns string }{
		{`nocorrect echo hi`, "nocorrect", "echo hi", ""},
		{`nocorrect nocorrect echo hi`, "nocorrect nocorrect", "echo hi", ""},
		{`nocorrect x=1 echo hi`, "nocorrect", "echo hi", "x"},
		// Recognized where a command word may first stand, which is after a
		// prefix as well as at the very start.
		{`x=1 nocorrect echo hi`, "nocorrect", "echo hi", "x"},
		{`>f nocorrect echo hi`, "nocorrect", "echo hi", ""},
		// Only in command position: behind a word it is an argument.
		{`echo nocorrect hi`, "", "echo nocorrect hi", ""},
		// Nothing behind it is still a command, and one that runs nothing.
		{`nocorrect`, "nocorrect", "", ""},
	} {
		c := simpleOf(t, tc.src, d)
		if got := words(c.Precommands); got != tc.mods {
			t.Errorf("%s: precommands = %q, want %q", tc.src, got, tc.mods)
		}
		if got := words(c.Args); got != tc.args {
			t.Errorf("%s: args = %q, want %q", tc.src, got, tc.args)
		}
		var names []string
		for _, a := range c.Assigns {
			names = append(names, a.Name)
		}
		if got := strings.Join(names, " "); got != tc.assigns {
			t.Errorf("%s: assigns = %q, want %q", tc.src, got, tc.assigns)
		}
	}
}

// TestAQuotedReservedPrecommandIsACommandName: quoting removes the
// reservation, exactly as it does for `if`. Without this the word could not be
// used as a command name at all, and the shell it comes from reports `command
// not found` for `\nocorrect echo hi`.
func TestAQuotedReservedPrecommandIsACommandName(t *testing.T) {
	d := precommandGrammar()
	for _, src := range []string{`\nocorrect echo hi`, `"nocorrect" echo hi`, `'nocorrect' echo hi`} {
		c := simpleOf(t, src, d)
		if len(c.Precommands) != 0 {
			t.Errorf("%s: precommands = %q, want none", src, words(c.Precommands))
		}
		if c.Args[0].Literal() != "nocorrect" {
			t.Errorf("%s: first arg = %q, want nocorrect", src, c.Args[0].Literal())
		}
	}
}

// TestADialectWithoutTheWordReadsAnOrdinaryCommand is the control: the same
// four shapes in a grammar that names no precommand.
func TestADialectWithoutTheWordReadsAnOrdinaryCommand(t *testing.T) {
	c := simpleOf(t, `nocorrect echo hi`, Core())
	if len(c.Precommands) != 0 {
		t.Errorf("precommands = %q, want none", words(c.Precommands))
	}
	if got := words(c.Args); got != "nocorrect echo hi" {
		t.Errorf("args = %q, want %q", got, "nocorrect echo hi")
	}
	// And a reserved word behind it is an ordinary argument there, where the
	// grammar that has the precommand refuses the same three words.
	if got := words(simpleOf(t, `nocorrect if true`, Core()).Args); got != "nocorrect if true" {
		t.Errorf("args = %q, want %q", got, "nocorrect if true")
	}
	if _, err := Parse(`nocorrect if true`, precommandGrammar()); err == nil {
		t.Error("`nocorrect if true` parsed with the word reserved, want a refusal")
	}
}

// TestOnlyASimpleCommandMayFollowAReservedPrecommand: a reserved word has
// nowhere to go behind one, which is what says the word was consumed by the
// grammar rather than kept as an argument.
func TestOnlyASimpleCommandMayFollowAReservedPrecommand(t *testing.T) {
	d := precommandGrammar()
	for _, src := range []string{
		`nocorrect if true; then echo hi; fi`,
		`nocorrect while true; do echo hi; done`,
		`nocorrect case x in x) echo hi;; esac`,
		`nocorrect for i in a; do echo $i; done`,
	} {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("%s parsed, want a refusal", src)
		}
	}
	for _, src := range []string{`nocorrect echo hi`, `nocorrect`, `nocorrect x=1`} {
		if _, err := Parse(src, d); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
}

// TestPrintingKeepsAReservedPrecommand. The word is kept on the tree rather
// than dropped precisely so this holds: printing a tree prints the program
// that was read, and `nocorrect mv a b` printed back as `mv a b` is a
// different program in a shell that corrects spelling.
func TestPrintingKeepsAReservedPrecommand(t *testing.T) {
	d := precommandGrammar()
	for _, tc := range []struct{ src, want string }{
		{`nocorrect echo hi`, "nocorrect echo hi"},
		{`nocorrect nocorrect echo hi`, "nocorrect nocorrect echo hi"},
		{`nocorrect x=1 echo hi`, "nocorrect x=1 echo hi"},
		{`nocorrect`, "nocorrect"},
	} {
		f, err := Parse(tc.src, d)
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		if got := strings.TrimRight(Print(f), "\n"); got != tc.want {
			t.Errorf("%s printed as %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestAnAliasBehindAReservedPrecommandExpands. The word after the modifier
// stands where a command word stands, so the table is consulted there — which
// is what makes `alias mv='nocorrect mv'` in one file and `nocorrect ll` in
// another behave alike. Without the second lookup the alias is an ordinary
// name and the shell reports `command not found`.
func TestAnAliasBehindAReservedPrecommandExpands(t *testing.T) {
	d := precommandGrammar()
	aliases := func(name string) (string, bool) {
		v, ok := map[string]string{"e": "echo", "n": "nocorrect "}[name]
		return v, ok
	}
	for _, tc := range []struct{ src, want string }{
		{`nocorrect e hi`, "nocorrect echo hi"},
		// And the other way round: an alias whose value *is* the modifier,
		// with the trailing space that makes the next word eligible too.
		{`n e hi`, "nocorrect echo hi"},
		// An alias is still not consulted in argument position.
		{`nocorrect echo e`, "nocorrect echo e"},
	} {
		p := NewParser(tc.src, d)
		p.Aliases = aliases
		f := p.Parse()
		if err := p.Err(); err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		if got := strings.TrimRight(Print(f), "\n"); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}
