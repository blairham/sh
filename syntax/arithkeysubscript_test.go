// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A subscript that will not read as an expression still parses, because on an
// associative name it is not one: it is a key, read exactly as `${m[k]}` reads
// it. Which kind of name it follows is an attribute the parser cannot see —
// the name need not be declared yet, or at all — so the text is kept, Index is
// left nil, and the reading is finished where the attribute is known.
//
// It was a parse failure, and the shape that found it is not a corner: a
// widget name with a dot in it, `bind_count=$(( counts[$widget] ))` with
// `$widget` holding `.accept-line`. The refusal fired 69 times in one
// interactive session (#1875).
func TestAKeySubscriptThatIsNoExpressionParses(t *testing.T) {
	d := Core()
	for _, tc := range []struct {
		name, src, sub string
	}{
		{"a dotted key", `m[.accept-line]`, ".accept-line"},
		{"a key that is only punctuation", `m[!?]`, "!?"},
		{"a key holding a quote", `m['x']`, "'x'"},
		{"a key with an operator and no operand", `m[+]`, "+"},
		// The subscript reader stops at the matching bracket, so a key is
		// carried whole rather than up to the first byte that puzzles it.
		{"a key ending in a space", `m[.k ]`, ".k "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := parseArithOf(t, tc.src, d)
			x, ok := got.(*ArithIndex)
			if !ok {
				t.Fatalf("%s parsed as %T, want *ArithIndex", tc.src, got)
			}
			if x.Name != "m" || x.Sub != tc.sub {
				t.Errorf("%s = %+v, want name m with sub %q", tc.src, x, tc.sub)
			}
			if x.Index != nil {
				t.Errorf("%s has an index expression, want the text alone", tc.src)
			}
			if x.Empty {
				t.Errorf("%s is marked empty, which is a different subscript", tc.src)
			}
		})
	}
}

// The same text as an assignment target, which is the other place a subscript
// is read — `(( m[.k] = 3 ))` writes the element in both shells with the
// attribute, so the target has to carry the key too.
func TestAKeySubscriptThatIsNoExpressionIsAnAssignmentTarget(t *testing.T) {
	got := parseArithOf(t, `m[.k] = 3`, Core())
	x, ok := got.(*ArithAssign)
	if !ok {
		t.Fatalf("m[.k] = 3 parsed as %T, want *ArithAssign", got)
	}
	if x.Name != "m" || x.Sub != ".k" || x.Index != nil || x.Op != "=" {
		t.Errorf("m[.k] = 3 = %+v, want the name and the key with no index", x)
	}
}

// Nothing else about the expression is loosened. A subscript that is no
// expression is carried because of what a *name* might be; text outside the
// brackets is refused exactly as before, and so is the whole expression when
// the brackets are only part of what cannot be read.
func TestCarryingAKeySubscriptDoesNotLoosenTheExpression(t *testing.T) {
	for _, src := range []string{
		`.accept-line`,
		`1 + .accept-line`,
		`m[.k] .accept-line`,
		`m[.k] +`,
	} {
		if got := parseArithOf(t, src, Core()); got != nil {
			t.Errorf("%s parsed as %T, want a refusal", src, got)
		}
	}
}

// The reader inside the brackets shares the outer parser's error slot, so a
// text it could not read left a refusal behind even once the refusal stopped
// being returned. A second operand after the subscript is what shows it: the
// expression is whole, and a leaked error would refuse it anyway.
func TestAKeySubscriptLeavesNoRefusalBehind(t *testing.T) {
	for _, src := range []string{
		`m[.k] + 1`,
		`1 + m[.k]`,
		`m[.k] + m[.j]`,
		`m[.k] ? 1 : 2`,
	} {
		if got := parseArithOf(t, src, Core()); got == nil {
			t.Errorf("%s did not parse", src)
		}
	}
}
