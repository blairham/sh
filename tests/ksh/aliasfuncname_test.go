// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// An alias standing where a function name is being defined is declined here
// when the `(` is immediately after the word, and expanded when a blank
// separates them.
//
// Measured 2026-09-18 on ksh93u+ 2012-08-01, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on /dev/null. With
// `alias zz='typeset -n'`, whose expansion cannot be a legal definition:
//
//	zz() { :; }    silent, 0
//	zz () { :; }   syntax error at line 2: `(' unexpected, 3
//
// It is what the preset aliases cost rather than a curiosity: this shell
// ships eight whose values are declaration words, so `nameref() { :; }` and
// `float() { :; }` were a **parse error** here — which costs every line of
// the file rather than its own — and a silent 0 there (#3643).
func TestAnAliasIsDeclinedAtAnAdjacentFunctionParen(t *testing.T) {
	if got := ksh.Dialect().AliasAtAFunctionName; got != syntax.AliasSuppressedWhereTheParenIsAdjacent {
		t.Fatalf("AliasAtAFunctionName is %v, want AliasSuppressedWhereTheParenIsAdjacent", got)
	}
	// The rows that run, through the shell's **own** preset table: these
	// names need no `alias` line, which is also what puts them in the table
	// when the file is parsed.
	dir := t.TempDir()
	for _, c := range []struct{ name, src string }{
		{"adjacent, so the definition keeps the alias's own name", "nameref() { :; }\nprintf 'after\\n'\n"},
		{"and another declaration word", "float() { :; }\nprintf 'after\\n'\n"},
		// A preset alias whose value *is* a legal function name is declined
		// the same way, which says the reading is about the parenthesis and
		// not about what the expansion would have come to.
		{"a value that would have been legal", "source() { :; }\nprintf 'after\\n'\n"},
		// The keyword spelling has no `(` to be adjacent to.
		{"the keyword spelling", "function nameref { :; }\nprintf 'after\\n'\n"},
	} {
		out, st, err := preset.CombinedThroughTheAliases(t, dialecttest.Base{Dir: dir}, c.src)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if st != 0 || !strings.Contains(out, "after") {
			t.Errorf("%s: %q at %d, want `after` at 0", c.name, out, st)
		}
	}
	// And the rows that do not parse, at the parser, since a harness that
	// runs the file cannot hand back a refusal the file never got past. The
	// table is stood up by hand here; that the shipped one really holds
	// these names is TestTheShellStartsWithItsOwnAliases.
	table := func(name string) (string, bool) {
		v, ok := map[string]string{"nameref": "typeset -n", "float": "typeset -lE"}[name]
		return v, ok
	}
	for _, c := range []struct{ name, src string }{
		{"a blank, so the expansion happens", "nameref () { :; }\n"},
		{"and there too", "float () { :; }\n"},
	} {
		p := syntax.NewParser(c.src, ksh.Dialect())
		p.Aliases = table
		p.Parse()
		if err := p.Err(); err == nil || !strings.Contains(err.Error(), `"(" unexpected`) {
			t.Errorf("%s: %v, want the syntax error", c.name, err)
		}
	}
	// The control for that pair: with the parenthesis adjacent the same
	// table leaves the definition alone.
	p := syntax.NewParser("nameref() { :; }\n", ksh.Dialect())
	p.Aliases = table
	p.Parse()
	if err := p.Err(); err != nil {
		t.Errorf("adjacent at the parser: %v, want the definition", err)
	}
}
