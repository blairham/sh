// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// The two `if` spellings compose in either direction: a long `if … ; then …`
// may carry an `elif` whose condition ended itself and whose body is written
// with braces, and from there the chain is a short one with no `fi`.
func TestALongIfMayCarryABraceBodiedElif(t *testing.T) {
	for _, src := range []string{
		"if (( 0 )); then echo A; elif (( 1 )) { echo B }\n",
		"if (( 0 )); then echo A; elif [[ -n y ]] { echo B }\n",
		"if (( 0 )); then echo A; elif (( 1 )) { echo B } else { echo C }\n",
		"if (( 0 )); then echo A; elif (( 1 )) { echo B } elif (( 1 )) { echo C }\n",
		// A statement after it, which is what says the chain really ended.
		"if (( 0 )); then echo A; elif (( 1 )) { echo B }\necho tail\n",
		// And the two spellings the other way round, which already worked.
		"if (( 0 )) { echo A } elif (( 1 )) { echo B }\n",
		"if (( 0 )); then echo A; elif (( 1 )); then echo B; fi\n",
	} {
		if _, err := syntax.Parse(src, short()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
		if _, err := syntax.Parse(src, syntax.Core()); err == nil {
			if src == "if (( 0 )); then echo A; elif (( 1 )); then echo B; fi\n" {
				continue // the long form, which the core reads.
			}
			t.Errorf("%q parsed under the core, want a refusal", src)
		}
	}
}

// It is shortIf's own test that decides, so an `elif` whose condition did not
// end itself takes no brace body — the `{` is another of the condition's
// words and the `}` is what the refusal names.
func TestAnElifConditionMustEndItselfForABraceBody(t *testing.T) {
	for _, src := range []string{
		"if :; then echo A; elif : { echo B }\n",
		"if :; then echo A; elif echo x { echo B }\n",
	} {
		if _, err := syntax.Parse(src, short()); err == nil {
			t.Errorf("%q parsed; the condition did not end itself", src)
		}
	}
	// The `else` of a long `if` is not this rule and takes no brace body:
	// what follows it is an ordinary brace group and the `fi` is still
	// required.
	if _, err := syntax.Parse("if (( 0 )); then echo A; else { echo C }\n", short()); err == nil {
		t.Error("a long `else` with a brace body and no `fi` parsed")
	}
	if _, err := syntax.Parse("if (( 0 )); then echo A; else { echo C; } fi\n", short()); err != nil {
		t.Errorf("a long `else` holding a brace group: %v", err)
	}
}
