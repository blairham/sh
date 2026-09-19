// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// namespaceDialect is the core plus the one flag, so that every row below is
// a statement about [Dialect.NamespaceBlock] and not about a preset.
func namespaceDialect() Dialect {
	d := Core()
	d.NamespaceBlock = true
	return d
}

// The flag gates a production and nothing in the lexer, which is the whole of
// what keeps `namespace` a name the other five dialects may still use.
func TestNamespaceBlockGatesTheProduction(t *testing.T) {
	on, off := namespaceDialect(), Core()
	mustParse(t, "namespace ns { x=1; }", on, "the flag is on")
	mustFail(t, "namespace ns { x=1; }", off, "the flag is off")

	// Off, the word is an ordinary command name and the brace group after it
	// is where the grammar stops — which is what the four columns without the
	// construct report, each naming the `}`.
	mustParse(t, "namespace ns", off, "an ordinary command with an operand")

	// On or off, the word is still a variable name and still an operand. A
	// flag that took the word away from a script would be a worse trade than
	// the construct is worth, and this is the assertion that says it did not.
	for _, d := range []Dialect{on, off} {
		mustParse(t, "namespace=5", d, "an assignment")
		mustParse(t, "echo namespace", d, "an operand")
		mustParse(t, "echo a namespace b", d, "an operand in the middle")
		mustParse(t, "v=namespace", d, "a value")
	}
}

// The production: the word, one word standing where the name belongs, then a
// brace group. Each row measured on the one column that has it — see
// [Dialect.NamespaceBlock].
func TestNamespaceBlockReadsAWordAndABraceGroup(t *testing.T) {
	d := namespaceDialect()
	for _, src := range []string{
		"namespace ns { echo inside; }; echo after",
		"namespace ns\n{ echo hi; }",
		"if true; then namespace ns { echo in; }; fi",
		"namespace ns { echo a; } > out.txt",
		"namespace ns { x=1; } | cat",
		// A word that is no name parses and is refused when the clause runs,
		// which is the stage split this flag's doc comment records.
		"namespace .ns { x=1; }",
		"namespace 1x { x=1; }",
		"namespace $n { x=1; }",
		// Quoting may be on the *name*.
		`namespace "ns" { echo hi; }`,
		// And the construct nests, flat namespaces or not: the grammar knows
		// nothing about what the names do.
		"namespace a { namespace b { x=1; }; }",
	} {
		mustParse(t, src, d, "the production")
	}
	for _, src := range []string{
		"namespace ns; echo after",
		"namespace; echo two",
		"namespace ns{ echo hi; }",
		// The word that reserves the production may not be quoted, which is
		// the ordinary rule for every reserved word and is what tells this
		// row from the quoted *name* above.
		`"namespace" ns { echo hi; }`,
		"namespace ns extra { :; }",
		"namespace ns {",
		"namespace ns",
	} {
		mustFail(t, src, d, "not the production")
	}
}

// An unterminated block is reported as the construct rather than as its
// braces, which is the second construct to want that — see
// [Parser.parseGroupOpenedBy], and `function` before it.
func TestAnUnterminatedNamespaceIsNamedAfterTheConstruct(t *testing.T) {
	_, err := Parse("namespace ns {", namespaceDialect())
	if err == nil {
		t.Fatal("namespace ns { parsed, want a syntax error")
	}
	if got := err.Error(); !strings.Contains(got, "namespace") || strings.Contains(got, "`{'") {
		t.Errorf("got %q, want the construct named and not the brace", got)
	}
}

// The name is carried as written and never expanded, which is what lets the
// interpreter judge the word the script really wrote — and quoting is removed,
// which is what makes `namespace "ns"` the namespace `ns`.
func TestANamespaceNameIsCarriedAsItWasWritten(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"namespace ns { :; }", "ns"},
		{`namespace "ns" { :; }`, "ns"},
		{"namespace .ns { :; }", ".ns"},
		{"namespace $n { :; }", "$n"},
		{"namespace a[1] { :; }", "a[1]"},
	} {
		f, err := Parse(c.src, namespaceDialect())
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		n, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*NamespaceClause)
		if !ok {
			t.Fatalf("%s: got %T, want a namespace clause", c.src, f.Stmts[0].Expr)
		}
		if n.Name != c.want {
			t.Errorf("%s: name %q, want %q", c.src, n.Name, c.want)
		}
	}
}

// The word is claimed as a keyword by a report about what is runnable, in the
// dialect that has the construct and in no other — the same rule [[ and
// `coproc` are under. Measured: `whence -v namespace` and `type namespace`
// are both `namespace is a keyword` on the one column.
func TestNamespaceIsReservedOnlyWhereTheFlagIsOn(t *testing.T) {
	if !namespaceDialect().Reserves("namespace") {
		t.Error("the flag is on and the word is not reserved")
	}
	if Core().Reserves("namespace") {
		t.Error("the flag is off and the word is reserved")
	}
}
