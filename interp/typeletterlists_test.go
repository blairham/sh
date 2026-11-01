// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// TestTheTwoTypeLetterListsAreOneList gives a shell with no letters anywhere
// one type letter at a time, and asks both of the questions that read the
// list: whether the name carries one, and whether any name does. The second
// is the common path's way of not asking the first (#6698), and a letter it
// left out would make every prefix to a program in a shell holding only that
// letter skip the fold — invisible from the zsh and ksh tests, whose shells
// start with a typed parameter of their own and so never take the short way.
func TestTheTwoTypeLetterListsAreOneList(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		give func(r *Runner)
	}{
		{"integer", func(r *Runner) { r.attrSet("v", attrInteger) }},
		{"lower", func(r *Runner) { r.attrSet("v", attrLower) }},
		{"upper", func(r *Runner) { r.attrSet("v", attrUpper) }},
		{"capitalized", func(r *Runner) { r.capitalized = map[string]bool{"v": true} }},
		{"float", func(r *Runner) { r.floatPrecision = map[string]int{"v": 2} }},
		{"width", func(r *Runner) { r.fieldWidth = map[string]fieldWidth{"v": {}} }},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newTestRunner(t, &Runner{})
			if r.anyNameCarriesATypeAttribute() {
				t.Fatal("a shell with no letters already answers that a name carries one")
			}
			c.give(r)
			if !r.nameCarriesATypeAttribute("v") {
				t.Fatal("the name does not carry the letter it was given")
			}
			if !r.anyNameCarriesATypeAttribute() {
				t.Error("no name carries a type letter, by the count, while v carries one")
			}
		})
	}
}
