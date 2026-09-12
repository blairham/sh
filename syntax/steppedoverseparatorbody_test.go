// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// withSeparatorBody is the pair a dialect needs to write a body as one
// stepped-over `;`: the step-over itself, and the answer that says the
// step-over may be the whole of a body.
func withSeparatorBody() Dialect {
	d := withOneSeparator()
	d.SteppedOverSeparatorIsABody = true
	return d
}

// A `;` the dialect steps over may be the whole of a compound command's body,
// and it reaches every construct that has one.
func TestASteppedOverSeparatorMayStandForABody(t *testing.T) {
	for _, src := range []string{
		"{ ; }\n",
		"( ; )\n",
		"if :; then ; fi\n",
		"if :; then :; else ; fi\n",
		"if :; then :; elif :; then ; fi\n",
		"while :; do ; done\n",
		"until :; do ; done\n",
		"for i in a; do ; done\n",
		"x() { ; }\n",
	} {
		if _, err := Parse(src, withSeparatorBody()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
		// The step-over on its own is not enough: without the answer above,
		// a body still has to have something in it.
		if _, err := Parse(src, withOneSeparator()); err == nil {
			t.Errorf("%q parsed with only the step-over, want a refusal", src)
		}
		if _, err := Parse(src, Core()); err == nil {
			t.Errorf("%q parsed under the core, want a refusal", src)
		}
	}
}

// It is the *step-over* that stands there and not a new kind of emptiness, so
// how many separators may be written is still the count the dialect carries.
// A body written as two is refused, and the refusal names the second `;`.
func TestOnlyAsManySeparatorsAsTheDialectStepsOverMayBeABody(t *testing.T) {
	d := withSeparatorBody()
	f, err := Parse("{ ; ; }\n", d)
	if err == nil {
		t.Fatalf("`{ ; ; }` parsed under a dialect that steps over one, got %v", f)
	}
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("err = %T, want *Error", err)
	}
	// Named where it stands: the second `;`, at column 5.
	if e.Pos.Col != 5 {
		t.Errorf("refused at column %d, want 5 — the second `;`", e.Pos.Col)
	}
	// And a body written as nothing at all is still refused, which is what
	// keeps this apart from EmptyCompoundBody.
	if _, err := Parse("{ }\n", d); err == nil {
		t.Error("`{ }` parsed; only the separator spelling is added here")
	}
}

// A dialect that takes an empty body has no use for the flag, and the two are
// independent rather than one implying the other.
func TestAnEmptyBodyDialectNeedsNoSeparatorBody(t *testing.T) {
	d := Core()
	d.EmptyCompoundBody = true
	d.SeparatorWhereACommandBelongs = AnySeparatorWhereACommandBelongs
	for _, src := range []string{"{ }\n", "{ ; }\n", "{ ; ; }\n"} {
		if _, err := Parse(src, d); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}
