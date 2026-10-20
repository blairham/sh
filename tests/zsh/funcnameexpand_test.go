// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A function definition's name is a word here, run rather than only parsed.
//
// Behavioral because parsing is not the promise: the name has to be the one
// the *expansion* produced, and a definition that flattened it would parse
// perfectly and define a plausible neighbor — `_p_${w}` becoming `_p_w`,
// which is what the keyword form did, at status 0, with no diagnostic
// anywhere (#1256).
func TestAFunctionNameMayHoldAnExpansion(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`w=foo; _p_${w}() { echo HI; }; _p_foo`, "HI"},
		{`w=foo; _p_$w() { echo HI; }; _p_foo`, "HI"},
		{`w=foo; function _p_${w} { echo HI; }; _p_foo`, "HI"},
		{`w=foo; function _p_${w}() { echo HI; }; _p_foo`, "HI"},
		// The name it did *not* define, which is what says the expansion
		// happened rather than the literal being kept.
		{`w=foo; _p_${w}() { echo HI; }; _p_w 2>/dev/null || echo nolit`, "nolit"},
		// Every kind of expansion, not only a parameter.
		{`_p_$(echo sub)() { echo HI; }; _p_sub`, "HI"},
		{`_p_$((1+1))() { echo HI; }; _p_2`, "HI"},
		// The name is fixed at the *definition* and never looked at again.
		{`w=foo; _p_${w}() { echo HI; }; w=bar; _p_foo`, "HI"},
		{`w=foo; _p_${w}() { echo HI; }; w=bar; _p_bar 2>/dev/null || echo nobar`, "nobar"},
		// The body is not expanded then, which is the control for it: the
		// name is early and the body is late.
		{`w=foo; _p_${w}() { echo "w=$w"; }; w=bar; _p_foo`, "w=bar"},
		// An expansion producing nothing still names a function.
		{`w=; _p_${w}() { echo HI; }; _p_`, "HI"},
		// And one holding a blank is one name rather than two, because an
		// unquoted expansion is not field-split in this shell.
		{`w="a b"; _p_${w}() { echo HI; }; print -l ${(k)functions}`, "_p_a b"},
		// A glob character in the produced name is a character: the name is
		// not matched against anything.
		{`w="*"; _p_${w}() { echo HI; }; _p_\*`, "HI"},
		// `$@` is several fields, so it is several definitions — the one
		// shape where a single definition names more than one function.
		{`set -- x y; _p_$@() { echo HI; }; _p_x; y`, "HI\nHI"},
		// An `=` after the expansion is part of the *name*: it is not an
		// assignment, because an assignment's name half is read as literal
		// text and this one is not. Measured on zsh 5.9.2, which lists the
		// function and leaves no variable behind.
		{`w=foo; _p_${w}=() { echo HI; }; print -rl -- ${(k)functions} | grep _p_`, "_p_foo="},
		// And the shape it has to be told apart from, which is the one a
		// loop writes: a subscript is literal text with an expansion inside
		// it, so `a[$i]=()` is an assignment and defines nothing. This
		// parsed before the flag and stopped parsing with it, which is how
		// it was found — a real plugin, not a hypothetical.
		//
		// Two elements and not none: the literal goes where the subscript
		// points, so an empty one *removes* the element it names rather than
		// emptying the array. The count was written here as `n=0` when the
		// row was about the parse alone, which recorded the semantics that
		// were wrong underneath it (#1330).
		{`i=1; a=(x y z); a[$i]=(); print -r -- "n=$#a"`, "n=2"},
		// A command word holding an expansion and *no* parentheses is a
		// command, which is the far commoner thing to write and the control
		// this needed: the parentheses are the whole announcement, and
		// dropping that check left `$c HI` looking like a definition.
		{`c=echo; $c HI`, "HI"},
		{`c=echo; ${c} HI`, "HI"},
		{`c=echo; $(echo echo) HI`, "HI"},
		{`c=ec; ${c}ho HI`, "HI"},
		// And an expansion producing *no* field at all defines nothing, at
		// status 0 — the other end of the `$@` row above, and the one where
		// falling back to the token's literal text would invent a function
		// called `@`.
		{`set --; $@() { echo HI; }; echo "st=$?"; print -rl -- ${(k)functions}`, "st=0"},
		// And the ordinary name still works, which is the control.
		{`plain() { echo HI; }; plain`, "HI"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}
