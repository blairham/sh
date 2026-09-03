// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// table makes an Aliases from pairs, for tests that care about the expansion
// and not about where the table came from.
func table(pairs ...string) syntax.Aliases {
	m := map[string]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return func(name string) (string, bool) {
		v, ok := m[name]
		return v, ok
	}
}

// parsed renders what the parser made of src, as the printer would, so a test
// can say what the expansion came to rather than poking at the tree.
func parsed(t *testing.T, a syntax.Aliases, src string) string {
	t.Helper()
	p := syntax.NewParser(src, syntax.Core())
	p.Aliases = a
	f := p.Parse()
	if err := p.Err(); err != nil {
		return "error: " + err.Error()
	}
	return strings.TrimSpace(syntax.Print(f))
}

// The whole expansion algorithm is unanimous across every shell that has it,
// so it is the core's behavior and not a dialect's.
func TestAliasExpansion(t *testing.T) {
	for _, c := range []struct {
		name  string
		alias syntax.Aliases
		src   string
		want  string
	}{
		{"a word in command position", table("a", "echo hit"), "a", "echo hit"},
		{"arguments follow the expansion", table("a", "echo"), "a hi", "echo hi"},
		{"not in argument position", table("a", "NOPE"), "echo a", "echo a"},
		{"chained through another alias", table("one", "two", "two", "echo deep"), "one", "echo deep"},
		{
			// The rule that stops recursion: a name already used in this
			// command is left as an ordinary word.
			"a self-reference does not loop",
			table("echo", "echo x"), "echo hi", "echo x hi",
		},
		{
			"mutual recursion stops too",
			table("a", "b x", "b", "a y"), "a", "a y x",
		},
		{
			// Only the command word: the second `p` is an argument.
			"only the command word expands",
			table("p", "echo p", "q", "p p"), "q", "echo p p",
		},
		{
			// A value ending in a space makes the next word eligible.
			"a trailing space carries on",
			table("a", "echo ", "b", "BEE"), "a b", "echo BEE",
		},
		{
			"no trailing space stops it",
			table("a", "echo", "b", "BEE"), "a b", "echo b",
		},
		{
			// An alias may hold a keyword, which is the whole reason this
			// happens in the parser.
			"an alias may hold a keyword",
			table("iff", "if true; then"), "iff echo yes; fi",
			"if true; then echo yes; fi",
		},
		{"an empty value leaves the rest", table("a", ""), "a echo hi", "echo hi"},
		{"a value of only spaces leaves the rest", table("a", "  "), "a echo hi", "echo hi"},
		{"a name that is not one is untouched", table("a", "echo"), "b hi", "b hi"},
		{
			// Quoting removes the alias, which is how a script reaches the
			// real thing past one that shadows it.
			"a quoted use is not expanded",
			table("a", "echo hit"), `"a" hi`, `"a" hi`,
		},
		{
			// And not even when the table holds that exact spelling. The
			// lookup uses the word's source text, so without the quoting
			// check this one would match — which is what makes the check
			// load-bearing rather than decoration.
			"a quoted word is not a candidate at all",
			table(`"a"`, "echo hit"), `"a" hi`, `"a" hi`,
		},
		{
			// Words only. An operator is not a candidate however the table
			// is spelled, and nothing may make it one.
			"an operator is never expanded",
			table("&&", "NOPE", "b", "echo two"), "true && b", "true && echo two",
		},
		{
			// A tab ends a value as a space does.
			"a trailing tab carries on too",
			table("a", "echo\t", "b", "BEE"), "a b", "echo BEE",
		},
		{
			// The set that stops recursion is per command, so a name used
			// twice on one line expands twice.
			"each command gets a fresh set",
			table("e", "echo"), "e one; e two", "echo one; echo two",
		},
		{"no table expands nothing", nil, "a hi", "a hi"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := parsed(t, c.alias, c.src); got != c.want {
				t.Errorf("parsed %q as %q, want %q", c.src, got, c.want)
			}
		})
	}
}
