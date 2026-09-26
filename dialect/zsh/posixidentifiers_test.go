// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `POSIX_IDENTIFIERS` turns the **arithmetic declaration** off: an assignment
// inside arithmetic to a name that does not exist leaves an ordinary scalar
// instead of declaring a numeric one. It was in the option table and nothing
// read it until #4664 — `setopt posix_identifiers` succeeded, `[[ -o
// posixidentifiers ]]` and the `setopt` listing reported it back faithfully,
// and `(( number = 3 ))` went on leaving `integer` in either state.
//
// Every case below is run in **both** states for that reason: a shell that
// ignores an option fails one half of every pair, where a row that only ever
// asked it one question can pass without the option being read at all.
//
// **The noun is the declaration, not the identifier**, and that is the trap
// this rule invites, because the option is nominally about which characters
// may appear in a name. zsh's own manual calls the arithmetic effect
// *another* difference and the measurement agrees:
// TestPosixIdentifiersIsNotAboutWhatTheNameIsSpelledWith below holds the name
// fixed at `xx` — nothing but POSIX identifier characters, so the character
// rule cannot reach it — and the answer moves with the option anyway, while
// `typeset a.b=1` is refused in **either** state.
//
// Measured against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — run `-f`, 2026-09-26. `go version -m` on
// that binary says *not a Go executable*. See
// Semantics.ArithmeticAssignmentDeclaresANumber, which this reads backwards,
// and dialect/zsh/arithdeclaredtype_test.go for #4605's neighboring rule
// about *which* attribute a declaration gives.

// The grid, in both states. `typeset -p` beside `${(t)xx}` because the
// attribute and the characters the name is left holding move together: the
// option takes the attribute away and the value stored becomes the
// expression's own rendering.
func TestPosixIdentifiersOverTheGridInBothStates(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src, off, on string }{
		{
			// The issue's own row, and the suite's: *type of variable when
			// created in arithmetic context* in Test/C01arith.ztst.
			"an integer value", `(( xx = 5 ))`,
			"integer\ntypeset -i xx=5\n", "scalar\ntypeset xx=5\n",
		},
		{
			// #4605's row moves the same way and says it louder, the value
			// itself differing and not only the listing.
			"a float value", `(( xx = 1.5 ))`,
			"float\ntypeset -F xx=1.5000000000\n", "scalar\ntypeset xx=1.5\n",
		},
		{
			// A whole float, which is the row that says the option reaches
			// the declaration as such rather than the float attribute: the
			// rendering `1.` is what a scalar gets in either reading.
			"a whole float value", `(( xx = 1.0 ))`,
			"float\ntypeset -F xx=1.0000000000\n", "scalar\ntypeset xx=1.\n",
		},
		{
			// Every digit the expression produced is in the characters,
			// which `typeset -F`'s ten places could not show.
			"the digits past a float's rendering", `(( xx = 1.0/3 ))`,
			"float\ntypeset -F xx=0.3333333333\n",
			"scalar\ntypeset xx=0.33333333333333331\n",
		},
		{
			"a float past the word", `(( xx = 1e30 ))`,
			"float\ntypeset -F xx=1000000000000000019884624838656.0000000000\n",
			"scalar\ntypeset xx=1e+30\n",
		},
		{
			"a whole integer quotient", `(( xx = 3/2 ))`,
			"integer\ntypeset -i xx=1\n", "scalar\ntypeset xx=1\n",
		},
		{
			// **No base comes with the scalar either.** The declaration
			// route learns a base from the radix the expression wrote and
			// stores the plain number; with the option on the six characters
			// of the rendering are what is stored.
			"an output base", `print -n "$(( xx = [#16] 255 ))"$'\n'`,
			"16#FF\ninteger\ntypeset -i16 xx=255\n",
			"16#FF\nscalar\ntypeset xx='16#FF'\n",
		},
		{
			// A float reached through a name, with no point written
			// anywhere — #4605's own discriminating row, which this option
			// flattens like the rest.
			"a float through a name", `float ff=2; (( xx = ff ))`,
			"float\ntypeset -F xx=2.0000000000\n", "scalar\ntypeset xx=2.\n",
		},
		// Every construct that assigns inside arithmetic gives the same
		// answer, which is why the option is read where the operator is
		// applied rather than at any one construct.
		{
			"through let", `let "xx = 5"`,
			"integer\ntypeset -i xx=5\n", "scalar\ntypeset xx=5\n",
		},
		{
			"through an expansion", `print -n "$(( xx = 5 ))"$'\n'`,
			"5\ninteger\ntypeset -i xx=5\n", "5\nscalar\ntypeset xx=5\n",
		},
		{
			"through a C-style for header", `for (( xx = 0; xx < 2; xx++ )); do :; done`,
			"integer\ntypeset -i xx=2\n", "scalar\ntypeset xx=2\n",
		},
		{
			// The operator is not a second noun for this rule any more than
			// it is for #4605's: `+=` on an unset name declares exactly as
			// `=` does, and the option stops both.
			"a compound assignment", `(( xx += 1.5 ))`,
			"float\ntypeset -F xx=1.5000000000\n", "scalar\ntypeset xx=1.5\n",
		},
		{
			"a step", `(( xx++ ))`,
			"integer\ntypeset -i xx=1\n", "scalar\ntypeset xx=1\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, setopt, want string }{
				{"off", "unsetopt posix_identifiers\n", c.off},
				{"on", "setopt posix_identifiers\n", c.on},
			} {
				t.Run(state.name, func(t *testing.T) {
					src := state.setopt + c.src + "\nprint -r -- \"${(t)xx}\"\ntypeset -p xx\n"
					out, st := runZsh(t, dir, src)
					if out != state.want || st != 0 {
						t.Errorf("%s = %q at %d, want %q at 0", src, out, st, state.want)
					}
				})
			}
		})
	}
}

// **The name is still created.** Only the attribute is withheld, which is the
// row that separates "the declaration is off" from "the assignment is off" —
// a reading under which `xx` would be unset and `$(( xx = 5 ))` would have no
// value.
func TestPosixIdentifiersWithholdsTheAttributeAndNotTheName(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src, want string }{
		{"the name exists", `(( xx = 5 )); print -r -- "[${xx-UNSET}]"`, "[5]\n"},
		{"and holds the value", `(( xx = 5 )); print -r -- "[$xx]"`, "[5]\n"},
		{
			// Read through a route that is not a listing: a later plain
			// assignment is evaluated by an integer name and stored as text
			// by a scalar, so `2+3` tells the two apart without asking
			// `typeset` about its own rendering.
			"and a later assignment is text", `(( xx = 5 )); xx=2+3; print -r -- "[$xx]"`,
			"[2+3]\n",
		},
		{"the expression still has its value", `print -r -- "[$(( xx = 5 ))]"`, "[5]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, dir, "setopt posix_identifiers\n"+c.src+"\n")
			if out != c.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// **The controls #4605 left, which must not move.** The option reaches a name
// the assignment *creates* and nothing else — so a name that already carries
// an attribute keeps it and goes on converting, and an element is not a
// declaration at all. Every row here is identical in both states, which is
// what keeps the rule from reading as "the option switches arithmetic
// attributes off".
func TestPosixIdentifiersReachesOnlyADeclarationItWouldHaveMade(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src, want string }{
		{
			"an integer name already declared", `typeset -i ii; (( ii = 5 ))
print -r -- "${(t)ii}"; typeset -p ii`,
			"integer\ntypeset -i ii=5\n",
		},
		{
			"a float name already declared", `float ff; (( ff = 1.5 ))
print -r -- "${(t)ff}"; typeset -p ff`,
			"float\ntypeset -E ff=1.500000000e+00\n",
		},
		{
			"an integer name with a base of its own", `typeset -i8 ii; (( ii = 255 ))
typeset -p ii`,
			"typeset -i8 ii=255\n",
		},
		{
			"a scalar that already holds a value", `xx=3; (( xx = 1.5 ))
print -r -- "${(t)xx}"; typeset -p xx`,
			"scalar\ntypeset xx=1.5\n",
		},
		{
			"a name declared with no type", `typeset xx; (( xx = 1.5 ))
print -r -- "${(t)xx}"; typeset -p xx`,
			"scalar\ntypeset xx=1.5\n",
		},
		{
			"an element, which is never a declaration", `(( b[2] = 1.5 ))
typeset -p b`,
			"typeset -a b=( '' 1.5 )\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, setopt string }{
				{"off", "unsetopt posix_identifiers\n"},
				{"on", "setopt posix_identifiers\n"},
			} {
				t.Run(state.name, func(t *testing.T) {
					out, st := runZsh(t, dir, state.setopt+c.src+"\n")
					if out != c.want || st != 0 {
						t.Errorf("%s = %q at %d, want %q at 0", c.src, out, st, c.want)
					}
				})
			}
		})
	}
}

// **The rule is keyed on the declaration and not on the name's spelling**,
// which is the pair that had to be built because the option's own name points
// the other way. `xx` holds nothing but the characters `POSIX_IDENTIFIERS`
// allows, so a rule about which characters may appear in an identifier cannot
// reach it — and every row above that uses `xx` moves anyway.
//
// The two rows here hold the *name* fixed and vary only the option, and then
// hold the *option* fixed at each state and vary the name into one the
// character rule would be about. The second pair is the one that says the
// character rule is a separate behavior this change does not touch: `a.b` is
// refused identically in both states on this shell and on zsh 5.9.2, so the
// declaration effect cannot be a consequence of it.
func TestPosixIdentifiersIsNotAboutWhatTheNameIsSpelledWith(t *testing.T) {
	dir := t.TempDir()
	// The name held fixed at a plain POSIX identifier, the option varied.
	for _, c := range []struct{ name, setopt, want string }{
		{"off", "unsetopt posix_identifiers\n", "integer\n"},
		{"on", "setopt posix_identifiers\n", "scalar\n"},
	} {
		t.Run("a POSIX identifier, option "+c.name, func(t *testing.T) {
			out, st := runZsh(t, dir, c.setopt+"(( xx = 5 ))\nprint -r -- \"${(t)xx}\"\n")
			if out != c.want || st != 0 {
				t.Errorf("got %q at %d, want %q at 0", out, st, c.want)
			}
		})
	}
	// The option varied, and a name the character rule would be about: the
	// refusal is the same in both states, so nothing here is keyed on it.
	for _, c := range []struct{ name, setopt string }{
		{"off", "unsetopt posix_identifiers\n"},
		{"on", "setopt posix_identifiers\n"},
	} {
		t.Run("a name with a dot, option "+c.name, func(t *testing.T) {
			// runZsh merges the two streams, so the refusal is in `out`
			// and `reached` would be there too if the line had run. The
			// answer is identical in both states, which is the finding.
			out, st := runZsh(t, dir, c.setopt+"typeset a.b=1\nprint -r -- reached\n")
			const want = "zsh:typeset:2: not valid in this context: a.b\n"
			if out != want || st != 1 {
				t.Errorf("got %q at %d, want %q at 1", out, st, want)
			}
		})
	}
}

// **The option is read when the assignment runs**, not when it is parsed —
// which is worth measuring rather than assuming, because the option's other
// documented half goes the other way: zsh's manual says both options must be
// set *before a script or function is parsed* for the character rule to reach
// it. A grid built on the wrong moment gets half its rows backwards, which is
// what #4562 found.
//
// The second moment a rule like this could be read at is when the name is
// first *created*, and the last two rows vary the option between the two: a
// name created under one state and written again under the other keeps
// whatever the first assignment left it, because the second assignment
// declares nothing at all.
func TestPosixIdentifiersIsReadWhenTheAssignmentRuns(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src, want string }{
		{
			"a body parsed with it off and called with it on",
			"f() { (( xx = 5 )); print -r -- \"${(t)xx}\"; }\nsetopt posix_identifiers\nf",
			"scalar\n",
		},
		{
			"a body parsed with it on and called with it off",
			"setopt posix_identifiers\nf() { (( xx = 5 )); print -r -- \"${(t)xx}\"; }\nunsetopt posix_identifiers\nf",
			"integer\n",
		},
		{
			"an eval of a string built with it off",
			"unsetopt posix_identifiers\ns='(( xx = 5 ))'\nsetopt posix_identifiers\neval \"$s\"\nprint -r -- \"${(t)xx}\"",
			"scalar\n",
		},
		{
			"an eval of a string built with it on",
			"setopt posix_identifiers\ns='(( xx = 5 ))'\nunsetopt posix_identifiers\neval \"$s\"\nprint -r -- \"${(t)xx}\"",
			"integer\n",
		},
		{
			// The name is created under the option and written again
			// without it: the second assignment finds a name that exists,
			// so it declares nothing and the scalar stands.
			"created with it on, written again with it off",
			"setopt posix_identifiers\n(( xx = 5 ))\nunsetopt posix_identifiers\n(( xx = 6 ))\nprint -r -- \"${(t)xx}\"; typeset -p xx",
			"scalar\ntypeset xx=6\n",
		},
		{
			// And the mirror: created without it, written again under it,
			// and the integer attribute the first assignment made is still
			// there doing the converting.
			"created with it off, written again with it on",
			"unsetopt posix_identifiers\n(( xx = 5 ))\nsetopt posix_identifiers\n(( xx = 1.5 ))\nprint -r -- \"${(t)xx}\"; typeset -p xx",
			"integer\ntypeset -i xx=1\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, dir, c.src+"\n")
			if out != c.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// Nobody has to type it. `posixidentifiers` is in emulationAlwaysReset and
// sh's and ksh's defaults for it are on, so `emulate sh` — a common opening
// line in a zsh function library — turns the arithmetic declaration off
// without the word appearing anywhere. It is read off the axis rather than
// off a stored bit, so `(setopt posix_identifiers)` stays in the subshell and
// `localoptions` puts it back at the function's return. All measured on zsh
// 5.9.2, 2026-09-26.
func TestPosixIdentifiersThroughEmulationAndScope(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src, want string }{
		{"emulate sh", "emulate sh\n(( xx = 5 ))\nprint -r -- \"${(t)xx}\"", "scalar\n"},
		{"emulate ksh", "emulate ksh\n(( xx = 5 ))\nprint -r -- \"${(t)xx}\"", "scalar\n"},
		{
			// The control that keeps the pair above from reading as "any
			// emulation turns it off".
			"emulate zsh", "emulate zsh\n(( xx = 5 ))\nprint -r -- \"${(t)xx}\"",
			"integer\n",
		},
		{
			"a subshell keeps it to itself",
			"(setopt posix_identifiers; (( aa = 5 )); print -r -- \"${(t)aa}\")\n(( bb = 5 ))\nprint -r -- \"${(t)bb}\"",
			"scalar\ninteger\n",
		},
		{
			"localoptions puts it back at the return",
			"f() { setopt localoptions posix_identifiers; (( aa = 5 )); print -r -- \"${(t)aa}\"; }\nf\n(( bb = 5 ))\nprint -r -- \"${(t)bb}\"",
			"scalar\ninteger\n",
		},
		{
			// It is still reported back by every surface that names it,
			// which it was before this option was read by anything — the
			// half that was already right.
			"reported back by -o and by the listing",
			"setopt posix_identifiers\n[[ -o posixidentifiers ]] && print on || print off\nprint -r -- \"${options[posixidentifiers]}\"",
			"on\non\n",
		},
		{
			"and off by default",
			"[[ -o posixidentifiers ]] && print on || print off",
			"off\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, dir, c.src+"\n")
			if out != c.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}
