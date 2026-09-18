// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// runCarriedRefusal runs src under a grammar that carries a flag group's
// refusal to the run rather than raising it while reading.
func runCarriedRefusal(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.BadSubstitutionAtParseTime = true
		d.FlagGroupRefusedAtExpansion = true
	}, nil)
}

// The commands in front of the word run, and the ones behind it do not: the
// refusal is the *word's* and reaches the shell when the word is expanded.
// Before this the whole input was refused before anything ran, so a script
// whose only flag group stood in a branch it never took was refused for a
// word it would never have expanded.
func TestACarriedRefusalIsWrittenWhenTheWordExpands(t *testing.T) {
	out, st := runCarriedRefusal(t, `echo one; echo "${(f)x}"; echo two`)
	if !strings.Contains(out, "one") {
		t.Errorf("got %q, want the command in front of the word to have run", out)
	}
	if strings.Contains(out, "two") {
		t.Errorf("got %q, want nothing behind the word to run", out)
	}
	if !strings.Contains(out, "unknown operator") {
		t.Errorf("got %q, want the refusal written", out)
	}
	if st == 0 {
		t.Errorf("status = 0, want the shell to end on the refusal")
	}
}

// The control that says it is the expansion and not the line: a word that is
// never reached is never refused, where a read-time refusal would have taken
// the whole input.
func TestACarriedRefusalInABranchNeverTakenSaysNothing(t *testing.T) {
	out, st := runCarriedRefusal(t, `echo one; if false; then echo "${(f)x}"; fi; echo two`)
	if !strings.Contains(out, "one") || !strings.Contains(out, "two") {
		t.Errorf("got %q, want both commands to run", out)
	}
	if strings.Contains(out, "unknown operator") {
		t.Errorf("got %q, want nothing said about a word that was not expanded", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The sentence is the parse's own, because the failure carried here is the
// failure the parser built rather than a second one written at the run — so
// the text of the expansion reaches the reader the same way it would have.
// What the *dialect* words around it is that dialect's own test.
func TestACarriedRefusalWritesTheParsesOwnSentence(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`echo "${(f)x}"`, "${(f)x}"},
		{`echo a${(U)x}b c`, "${(U)x}"},
	} {
		out, _ := runCarriedRefusal(t, c.src)
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: got %q, want it to hold %q", c.src, out, c.want)
		}
	}
}

// Every route to the word reaches the same refusal, which is what says the
// rule is on the expansion rather than on a statement: a subshell, a
// function, a command substitution, a pipeline and an `eval` all write it.
func TestEveryRouteToACarriedRefusalWritesIt(t *testing.T) {
	for _, src := range []string{
		`( echo "${(f)x}" )`,
		`f() { echo "${(f)x}"; }; f`,
		`v=$(echo "${(f)x}")`,
		`echo "${(f)x}" | cat`,
		`eval 'echo "${(f)x}"'`,
	} {
		if out, _ := runCarriedRefusal(t, src); !strings.Contains(out, "unknown operator") {
			t.Errorf("%s: got %q, want the refusal written", src, out)
		}
	}
}
