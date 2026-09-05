// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

func TestArrays(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"assignment and subscript", `a=(1 2 3); printf "%s" "${a[0]}${a[2]}"`, "13"},
		{"count", `a=(1 2 3); printf "%s" "${#a[@]}"`, "3"},
		{"empty array is not a bare assignment", `a=(); printf "%s" "${#a[@]}"`, "0"},
		{"element assignment", `a=(1 2); a[1]=9; printf "%s" "${a[1]}"`, "9"},
		// Two elements and not four: an unassigned subscript is no element
		// at all in this dialect, so the gap between 0 and 3 is a gap rather
		// than two empty strings. The other reading — walking the extent —
		// is a dialect away and is tested beside the axis.
		{"assigning past the end leaves a gap", `a=(1); a[3]=x; printf "%s" "${#a[@]}"`, "2"},
		// A plain reference is the first element, which is what keeps `$a`
		// working on an array.
		{"scalar view", `a=(p q); printf "%s" "$a"`, "p"},
		// A scalar assignment replaces the array rather than leaving both.
		{"scalar replaces", `a=(p q); a=z; printf "%s" "${#a[@]}"`, "1"},
		// Elements are words, so an array can be built from an expansion.
		{"elements are words", `x="m n"; a=(l $x); printf "%s" "${#a[@]}"`, "3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, nil); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestQuotedArrayKeepsOneFieldPerElement(t *testing.T) {
	// The reason a subscript can produce several fields: joining them would
	// lose an element containing a space, exactly as it would for `"$@"`.
	if got, _ := run(t, `a=("x y" z); printf "[%s]" "${a[@]}"`, nil); got != "[x y][z]" {
		t.Errorf("got %q, want [x y][z]", got)
	}
	// Unquoted, each element is then split like any other expansion.
	if got, _ := run(t, `a=("x y" z); printf "[%s]" ${a[@]}`, nil); got != "[x][y][z]" {
		t.Errorf("got %q, want [x][y][z]", got)
	}
}

func TestArrayBaseIsAnAxis(t *testing.T) {
	// ArrayBaseIsZero — measured on both sides, and the reason a subscript
	// cannot be used as a slice offset without asking. Which preset gives
	// which answer is the dialect packages' claim.
	src := `a=(p q r); printf "%s" "${a[1]}"`
	zero := permissive()
	zero.ArrayBaseIsZero = Yes
	if got, _ := run(t, src, withSem(zero)); got != "q" {
		t.Errorf("zero-based gave %q, want q", got)
	}
	one := permissive()
	one.ArrayBaseIsZero = No
	if got, _ := run(t, src, withSem(one)); got != "p" {
		t.Errorf("one-based gave %q, want p", got)
	}
}

func TestEmptyArrayIsNotAFunctionDefinition(t *testing.T) {
	// `a=()` and `f()` both put a parenthesis pair after a word. A function
	// name is a name and cannot contain `=`, which is what tells them apart —
	// without it the empty array was read as defining a function called `a=`.
	if _, st := run(t, `a=(); printf "%s" "${#a[@]}"`, nil); st != 0 {
		t.Errorf("empty array assignment failed with status %d", st)
	}
	if got, _ := run(t, `f() { printf fn; }; f`, nil); got != "fn" {
		t.Errorf("function definitions still work: got %q", got)
	}
}

// An array assignment given to `local` lands in the function's scope, which
// takes shadowing the *array* table and not only the scalar one.
//
// Found by the wild sweep: `local -a x=()` is in three installed bats-core
// files and did not parse at all. Making it parse then showed the second half
// — the array outlived the function, because `local` had saved a scalar of
// that name and nothing had saved the array.
func TestALocalArrayStaysInTheFunction(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"set as an operand", `f(){ local a=(x y); }; f; echo "[${a[1]}]"`, "[]"},
		{"set after declaring", `f(){ local a; a=(x y); }; f; echo "[${a[1]}]"`, "[]"},
		{"visible inside", `f(){ local a=(x y); echo "[${a[1]}]"; }; f`, "[y]"},
		{"an outer array survives", `a=(g); f(){ local a=(x y); }; f; echo "[${a[0]}]"`, "[g]"},
		{"appending stays local", `f(){ local a=(); a+=(x); }; f; echo "[${a[0]}]"`, "[]"},
		{"nested scopes", `a=(g); f(){ local a=(f1); g; echo "[${a[0]}]"; }; g(){ local a=(g1); }; f`, "[f1]"},
		// Without `local` it is global, which is what makes the above a
		// statement about `local` rather than about arrays.
		{"no local is still global", `f(){ b=(x y); }; f; echo "[${b[1]}]"`, "[y]"},
		// The saved array has to be a *copy*. An Array is a map, so keeping
		// the value would keep a reference to the very table the function
		// then writes into, and putting it back would put back the change.
		{"an element written inside is put back", `a=(1 2); f(){ local a; a[0]=9; }; f; echo "[${a[0]}]"`, "[1]"},
		{"and is visible while inside", `a=(1 2); f(){ local a; a[0]=9; echo "[${a[0]}]"; }; f`, "[9]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := run(t, c.src, nil)
			if strings.TrimSpace(out) != c.want {
				t.Errorf("said %q, want %q", strings.TrimSpace(out), c.want)
			}
		})
	}
}

// `readonly a=(x)` sets the array before it locks the name, because locking it
// first would refuse the very assignment the command was given.
func TestReadonlyTakesItsArrayBeforeLocking(t *testing.T) {
	out, _ := run(t, `readonly a=(p q); echo "[${a[1]}]"`, nil)
	if strings.TrimSpace(out) != "[q]" {
		t.Errorf("said %q, want [q]", strings.TrimSpace(out))
	}
}

// An operand is not a prefix, and the plain name proves it: a prefix
// assignment of an array has no value to give, so treating one as a prefix
// sets the scalar to the empty string and `$a` reads empty instead of the
// first element.
func TestAnOperandIsNotAPrefixAssignment(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"readonly", `readonly a=(p q); echo "[$a]"`},
		{"export", `export a=(p q); echo "[$a]"`},
		{"typeset", `typeset a=(p q); echo "[$a]"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := run(t, c.src, nil)
			if strings.TrimSpace(out) != "[p]" {
				t.Errorf("said %q, want [p]", strings.TrimSpace(out))
			}
		})
	}
}

// A `-` or `+` keeps the fields of whatever it came to, which is the whole
// point of `"${a[@]+${a[@]}}"` — the standard way to expand a possibly-empty
// array under `set -u` without collapsing it into one string.
//
// Reachable only once the parser stopped ending a subscript at the last `]` in
// the word; before that this was a syntax error, so the joining went unseen.
func TestASubstitutedWordKeepsItsFields(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// The word is what it came to, and the word has fields of its own.
		{"alternate, nested", `a=(x y); printf "[%s]" "${a[@]+${a[@]}}"`, "[x][y]"},
		{"default, unset, nested", `b=(p q); printf "[%s]" "${n[@]-${b[@]}}"`, "[p][q]"},
		// The *parameter* is what it came to, and it keeps its fields too.
		{"default, set", `a=(x y); printf "[%s]" "${a[@]-${a[@]}}"`, "[x][y]"},
		// A literal is one field however many words are in it, because
		// nothing split it.
		{"a literal stays one field", `a=(x); printf "[%s]" "${a[@]+p q}"`, "[p q]"},
		{"one word is one field", `a=(x y); printf "[%s]" "${a[@]+Z}"`, "[Z]"},
		// The parameter's own shape does not decide it: the fields come from
		// the *word*, so a numeric subscript and a plain name get them too.
		{"a numeric subscript, nested", `a=(x); b=(p q); printf "[%s]" "${a[0]+${b[@]}}"`, "[p][q]"},
		{"a plain name, nested", `x=1; b=(p q); printf "[%s]" "${x+${b[@]}}"`, "[p][q]"},
		// And a literal after either is still one field, for the same reason
		// it is after `[@]` — nothing split it.
		{"a numeric subscript, literal", `a=(x); printf "[%s]" "${a[0]+p q}"`, "[p q]"},
		{"a plain name, literal", `x=1; printf "[%s]" "${x+p q}"`, "[p q]"},
		// The colon extends the test and changes nothing about the fields.
		{"with a colon", `a=(x y); printf "[%s]" "${a[@]:+${a[@]}}"`, "[x][y]"},
		// Nothing to substitute is no field at all rather than an empty one.
		{"unset yields nothing", `printf "[%s]" "${n[@]+${n[@]}}"`, "[]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := run(t, c.src, nil)
			if strings.TrimSpace(out) != c.want {
				t.Errorf("said %q, want %q", strings.TrimSpace(out), c.want)
			}
		})
	}
	// And the fields are real ones: they arrive as separate arguments and as
	// separate elements, which is what a joined string would silently break.
	for _, c := range []struct{ name, src, want string }{
		{"as arguments", `a=(x y); f(){ echo $#; }; f "${a[@]+${a[@]}}"`, "2"},
		{"as array elements", `a=(x y); b=("${a[@]+${a[@]}}"); echo "${#b[@]}"`, "2"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := run(t, c.src, nil)
			if strings.TrimSpace(out) != c.want {
				t.Errorf("said %q, want %q", strings.TrimSpace(out), c.want)
			}
		})
	}
}
