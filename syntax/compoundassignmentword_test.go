// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// running is `declaring` with the word kept going past the array's closing
// parenthesis, which is the flag under test.
func running() syntax.Dialect {
	d := declaring()
	d.CompoundAssignmentWordRunsPastItsParenthesis = true
	return d
}

// Text written after a compound assignment's `)` either ends the word or does
// not, and where it does not the assignment is no assignment at all: the
// whole of it is one ordinary word.
func TestACompoundAssignmentWordPastItsParenthesis(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		src     string
		assigns int
		args    []string
	}{
		{
			"an operand that runs past the paren is a word",
			"declare a=(1 2)x", 0,
			[]string{"declare", "a=(1 2)x"},
		},
		{
			"and what follows it is the next word",
			"declare a=(1 2)x y", 0,
			[]string{"declare", "a=(1 2)x", "y"},
		},
		{
			"the elements are written out again with one blank",
			"declare a=(1    2)x", 0,
			[]string{"declare", "a=(1 2)x"},
		},
		{
			"an empty literal too",
			"declare a=()x", 0,
			[]string{"declare", "a=()x"},
		},
		{
			"a subscript on the name stays in front of it",
			"declare a[0]=(1 2)x", 0,
			[]string{"declare", "a[0]=(1 2)x"},
		},
		{
			// The control: with a blank between them the word does end at
			// the parenthesis, so the array is an operand again and `x` is
			// a word of its own — and an operand is an assignment here
			// rather than a word, so only `x` joins the utility.
			"a blank ends the word in every dialect",
			"declare a=(1 2) x", 1,
			[]string{"declare", "x"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := syntax.NewParser(c.src, running())
			f := p.Parse()
			if err := p.Err(); err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			cmd := onlySimple(t, f)
			if len(cmd.Assigns) != c.assigns {
				t.Errorf("%d assignments, want %d", len(cmd.Assigns), c.assigns)
			}
			var got []string
			for _, w := range cmd.Args {
				got = append(got, w.Literal())
			}
			if len(got) != len(c.args) {
				t.Fatalf("words %q, want %q", got, c.args)
			}
			for i := range got {
				if got[i] != c.args[i] {
					t.Errorf("word %d is %q, want %q", i, got[i], c.args[i])
				}
			}
		})
	}
}

// Without the flag the word ends at the `)`, the array is the operand it
// always was, and the trailing text is a word of its own. The two readings
// are what makes this a flag.
func TestACompoundAssignmentWordEndingAtItsParenthesis(t *testing.T) {
	t.Parallel()
	p := syntax.NewParser("declare a=(1 2)x", declaring())
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("parse: %v", err)
	}
	cmd := onlySimple(t, f)
	if len(cmd.Assigns) != 1 || !cmd.Assigns[0].IsArray {
		t.Fatalf("%d assignments, want one array literal", len(cmd.Assigns))
	}
	if len(cmd.Args) != 2 || cmd.Args[1].Literal() != "x" {
		t.Errorf("%d words, want the utility and `x`", len(cmd.Args))
	}
}

// A prefix assignment folds the same way, and there the result is a *scalar*:
// `a=(1 2)x` assigns the seven characters rather than declaring an array.
func TestAFoldedCompoundAssignmentPrefixIsAScalar(t *testing.T) {
	t.Parallel()
	p := syntax.NewParser("a=(1 2)x", running())
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("parse: %v", err)
	}
	cmd := onlySimple(t, f)
	if len(cmd.Assigns) != 1 {
		t.Fatalf("%d assignments, want one", len(cmd.Assigns))
	}
	a := cmd.Assigns[0]
	if a.IsArray || len(a.Elems) != 0 {
		t.Errorf("the assignment is still an array literal")
	}
	if a.Value == nil || a.Value.Literal() != "(1 2)x" {
		t.Errorf("value %v, want `(1 2)x`", a.Value)
	}
}
