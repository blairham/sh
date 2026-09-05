// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// emptyBodies are every shape of compound command with nothing in it, which
// is one question rather than a rule about braces: the body of a group, of a
// subshell, of each loop and of each arm of an `if`, and the condition as
// well as the body.
var emptyBodies = []string{
	`{ }`,
	`( )`,
	`f() { }`,
	`f() ( )`,
	`{ } > /dev/null`,
	`while false; do done`,
	`until true; do done`,
	`while ; do :; done`,
	`until ; do :; done`,
	`for i in 1; do done`,
	`if true; then fi`,
	`if ; then :; fi`,
	`if true; then :; else fi`,
	`if true; then :; elif true; then fi`,
	`{
}`,
}

// TestAnEmptyCompoundBodyNeedsTheFlag: the core refuses a body with nothing
// in it, and a dialect adds it.
//
// The core is the language every panel shell accepts, and this is outside
// that set: four of the five refuse every shape above and one takes every
// shape. So the flag is off in the core and the grammar grows by a dialect
// setting it, which is the additive direction.
func TestAnEmptyCompoundBodyNeedsTheFlag(t *testing.T) {
	allowed := Core()
	allowed.EmptyCompoundBody = true
	allowed.CStyleFor = true
	allowed.Select = true
	allowed.ForBraceBody = true

	strict := Core()
	strict.CStyleFor = true
	strict.Select = true
	strict.ForBraceBody = true

	for _, src := range emptyBodies {
		if _, err := Parse(src, allowed); err != nil {
			t.Errorf("with the flag, %q: %v — the dialect that takes one takes all of them", src, err)
		}
		if _, err := Parse(src, strict); err == nil {
			t.Errorf("without the flag, %q parsed — the core refuses a body with nothing in it", src)
		}
	}
	// The constructs a dialect has to have before the question arises.
	for _, src := range []string{
		`select x in a; do done`,
		`for ((i=0;i<1;i++)) { }`,
		`for ((i=0;i<1;i++)); do done`,
		`for i in a; { }`,
	} {
		if _, err := Parse(src, allowed); err != nil {
			t.Errorf("with the flag, %q: %v", src, err)
		}
		if _, err := Parse(src, strict); err == nil {
			t.Errorf("without the flag, %q parsed", src)
		}
	}
}

// TestAnEmptyBodyIsNotEveryEmptyThing: two neighbors are a different question
// and neither moves with the flag.
//
// A `case` with no arms is a list of arms rather than a command list, and the
// panel splits the other way on it. A command substitution's body is a whole
// program, and every shell in the panel takes an empty one.
func TestAnEmptyBodyIsNotEveryEmptyThing(t *testing.T) {
	strict := Core()
	for _, src := range []string{
		`case x in esac`,
		`case x in (x) ;; esac`,
		`x=$( )`,
		"x=`  `",
		`{ :; }`,
		`( : )`,
		`if true; then :; fi`,
	} {
		if _, err := Parse(src, strict); err != nil {
			t.Errorf("%q: %v — this is not an empty compound body", src, err)
		}
	}
}

// TestAnEmptyBodyNamesTheTokenItStoppedOn: the refusal is the ordinary one,
// naming the word the parser met where a command belonged — and naming
// nothing as expected alongside it, because an empty body has nothing to be
// in the middle of.
func TestAnEmptyBodyNamesTheTokenItStoppedOn(t *testing.T) {
	for _, tc := range []struct{ src, token string }{
		{`{ }`, "}"},
		{`( )`, ")"},
		{`if true; then fi`, "fi"},
		{`while false; do done`, "done"},
		{`if true; then :; else fi`, "fi"},
	} {
		_, err := Parse(tc.src, Core())
		se, ok := err.(*Error)
		if !ok {
			t.Errorf("%q: error %v, want a syntax error", tc.src, err)
			continue
		}
		if se.Kind != ErrUnexpected || se.Token != tc.token {
			t.Errorf("%q: token %q kind %v, want %q unexpected", tc.src, se.Token, se.Kind, tc.token)
		}
		if se.Expected != "" {
			t.Errorf("%q: expected %q, want nothing named", tc.src, se.Expected)
		}
	}
}

// TestAnUnfinishedBodyIsStillUnfinished: input that ran out is not an empty
// body. The caller knows which construct is open and reports it as waiting
// for more, which is what a prompt needs and what a script reports as an
// unterminated construct.
func TestAnUnfinishedBodyIsStillUnfinished(t *testing.T) {
	for _, src := range []string{`{`, `(`, `if true; then`, `while false; do`} {
		p := NewParser(src, Core())
		p.Parse()
		if !p.Incomplete() {
			t.Errorf("%q: not reported as incomplete — the input ran out, the body is not empty", src)
		}
	}
}
