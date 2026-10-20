// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// Under `-p` the `-T` letter is a **filter on a listing**, not a declaration
// (#5101).
//
// It was routed to the tie declaration, so `typeset -pT A` answered `-T
// requires names of scalar and array` — the arity of the declaring form, where
// the listing takes any number of names and none is a listing of its own.
//
// Measured 2026-09-29 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not
// another build of this one), script files under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and standard input on the null device.
//
// **`-pT NAMES` is `-p NAMES`.** That is measured as an identity and not
// inferred: the reference's output for the two spellings is byte-identical over
// a tied scalar, a tied array, a plain scalar, an array that is not tied, a
// name it does not have, and a mixture of those. So the letter selects nothing
// once operands have; it is with **no** operand that it narrows.
func TestTheTieLetterUnderThePrintLetterIsAListing(t *testing.T) {
	dir := t.TempDir()
	const tie = "typeset -T A a=(x y)\n"
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{"the scalar half", tie + "typeset -pT A\n", "typeset -T A a=( x y )\n", 0},
		{"the array half", tie + "typeset -pT a\n", "typeset -aT A a=( x y )\n", 0},
		{
			"both halves", tie + "typeset -pT A a\n",
			"typeset -T A a=( x y )\ntypeset -aT A a=( x y )\n", 0,
		},
		// Any number of names, which is the arity the declaring form does not
		// have: two is a tie there and four is `too many arguments for -T`.
		{
			"four names", tie + "typeset -pT A a A a\n",
			"typeset -T A a=( x y )\ntypeset -aT A a=( x y )\n" +
				"typeset -T A a=( x y )\ntypeset -aT A a=( x y )\n", 0,
		},
		{"one name", tie + "typeset -pT A\n", "typeset -T A a=( x y )\n", 0},
		// A name that is not tied at all is listed as `-p` lists it, which is
		// what says the letter is not selecting once operands are given.
		{"a plain scalar", "s=plain\ntypeset -pT s\n", "typeset s=plain\n", 0},
		{"an array that is not tied", "typeset -a arr=(1)\ntypeset -pT arr\n", "typeset -a arr=( 1 )\n", 0},
		// The separator is carried, as `-p` carries it.
		{
			"a tie with a separator", "typeset -T A a=(x y) +\ntypeset -pT A\n",
			"typeset -T A a=( x y ) +\n", 0,
		},
		{"inside a function", "f(){ " + strings.TrimSuffix(tie, "\n") + "; typeset -pT A }\nf\n", "typeset -T A a=( x y )\n", 0},
		{"under declare", tie + "declare -pT A\n", "typeset -T A a=( x y )\n", 0},
		{"the letters the other way round", tie + "typeset -Tp A\n", "typeset -T A a=( x y )\n", 0},
		{"the letters written apart", tie + "typeset -p -T A\n", "typeset -T A a=( x y )\n", 0},
		{"under readonly", tie + "readonly -pT A\n", "typeset -T A a=( x y )\n", 0},
		{"with a kind letter beside it", tie + "typeset -pTa A\n", "typeset -T A a=( x y )\n", 0},
		{"and with another", tie + "typeset -pTx A\n", "typeset -T A a=( x y )\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != tc.status {
				t.Errorf("out %q status %d, want %q at %d", out, st, tc.want, tc.status)
			}
		})
	}
}

// A name the shell does not have is `no such variable`, at 1, and the names it
// does have are still listed — which is `typeset -p`'s own answer and was the
// declaring form's refusal before.
func TestTheTiePrintListingReportsANameItDoesNotHave(t *testing.T) {
	dir := t.TempDir()
	const tie = "typeset -T A a=(x y)\n"
	for _, tc := range []struct{ name, src, wantOut, wantErr string }{
		{"one name it does not have", "typeset -pT nosuch\n", "", "zsh:typeset:1: no such variable: nosuch\n"},
		{
			"beside one it does", tie + "typeset -pT A nosuch\n",
			"typeset -T A a=( x y )\n", "zsh:typeset:2: no such variable: nosuch\n",
		},
		{
			"and written first", tie + "typeset -pT nosuch A\n",
			"typeset -T A a=( x y )\n", "zsh:typeset:2: no such variable: nosuch\n",
		},
		{
			"a name that was unset again", tie + "unset A\ntypeset -pT A\n",
			"", "zsh:typeset:3: no such variable: A\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errOut := runZshSplit(t, dir, tc.src)
			if out != tc.wantOut || errOut != tc.wantErr || st != 1 {
				t.Errorf("out %q err %q status %d, want %q / %q at 1",
					out, errOut, st, tc.wantOut, tc.wantErr)
			}
		})
	}
}

// With **no operand** the letter does narrow, and to the tied names: both
// halves of every pair, in `-p`'s form. The sign does not change it — under
// `-p` there is nothing to remove — where with operands `+T` is still the
// untie refusal.
func TestTheTiePrintListingWithNoOperand(t *testing.T) {
	dir := t.TempDir()
	// Whole-output containment rather than a filtered listing, because the
	// bare form writes the shell's own tied pairs too — `PATH`/`path` and the
	// rest — and those differ between any two installations. What is asked
	// here is that this script's pairs are in it and that a name which is not
	// half of a tie is not.
	const made = "typeset -T A a=(x y)\ntypeset -T B b=(p q)\nuntied=plain\n"
	for _, tc := range []struct{ name, src string }{
		{"the bare form", made + "typeset -pT\n"},
		// The sign does not change it: under `-p` there is nothing to remove.
		{"the plus form is the same listing", made + "typeset +pT\n"},
		// And the same table from a call, under the word that has a listing
		// of its own — it is `declare -p`'s table and not narrowed to the
		// running call, which is why `A` is in it at all.
		{"under local, from a call", made + "f(){ local -pT }\nf\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if st != 0 {
				t.Fatalf("status %d, want 0: %q", st, out)
			}
			// The rows without their leading word: this harness runs the
			// script where the shell's own names are a scope out, so its
			// listing writes `typeset -g -T A a=( x y )` where a script file
			// writes `typeset -T A a=( x y )`. The scope letter is not what
			// this test is about — the grid in the pull request grades the
			// whole line from a file — and matching the rest keeps the row
			// honest about the filter either way.
			for _, want := range []string{
				"-T A a=( x y )\n",
				"-aT A a=( x y )\n",
				"-T B b=( p q )\n",
				"-aT B b=( p q )\n",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("listing does not carry %q:\n%s", want, out)
				}
			}
			// The filter is the whole point: a name that is not half of a tie
			// is left out.
			if strings.Contains(out, "untied") {
				t.Errorf("listing carries the untied name:\n%s", out)
			}
		})
	}
	// And `-p` alone still has it, which says the narrowing is the letter's
	// and not something this change did to the listing.
	t.Run("the untied name is in the unfiltered listing", func(t *testing.T) {
		out, st := runZsh(t, dir, made+"typeset -p\n")
		if st != 0 || !strings.Contains(out, "untied") {
			t.Errorf("status %d, want the untied name listed by -p:\n%s", st, out)
		}
	})
}

// The declaring form is unmoved: the sign with operands is still the untie
// refusal, and without `-p` the letter still declares and still refuses the
// arities it refused.
func TestTheDeclaringFormOfTheTieLetterIsUnmoved(t *testing.T) {
	dir := t.TempDir()
	const tie = "typeset -T A a=(x y)\n"
	for _, tc := range []struct{ name, src, want string }{
		{"the plus form with operands", tie + "typeset +pT A\n", "zsh:typeset:2: use unset to remove tied variables\n"},
		{"under local too", "f(){ " + strings.TrimSuffix(tie, "\n") + "; local +pT A }\nf\n", "f:local: use unset to remove tied variables\n"},
		{"one operand still needs two", "typeset -T A\n", "zsh:typeset:1: -T requires names of scalar and array\n"},
		{"four operands are still too many", "typeset -T A a b c\n", "zsh:typeset:1: too many arguments for -T\n"},
		{"and the operand rules still hold", "typeset -T A=(1) a\n", "zsh:typeset:1: first argument of tie must be scalar: A\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, st, errOut := runZshSplit(t, dir, tc.src)
			if errOut != tc.want || st != 1 {
				t.Errorf("err %q status %d, want %q at 1", errOut, st, tc.want)
			}
		})
	}
}
