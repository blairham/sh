// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// The unique attribute on a **tied** name, where the value it drops duplicates
// out of is a joined string on one side and a list of elements on the other.
//
// Measured 2026-09-25 against zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f`,
// from a script file. It is the one shell in the panel with both the letter
// and the tie.
//
// The rule is keyed on **the name that is written**, and that is the whole of
// it: a write to a name carrying `-U` drops duplicates, and the mirror into
// the other half of the tie carries the result verbatim and never consults the
// other half's attribute. The letter is not a property of the *pair*.
//
// Two rows hold the written name fixed and move only where the letter sits,
// which is what says the letter and not the pair decides:
//
//	typeset -T S s; typeset -U S; S=m:n:m   S is m:n     and s is (m n)
//	typeset -T S s; typeset -U s; S=m:n:m   S is m:n:m   and s is (m n m)
//
// and two more hold the letter fixed on the array and move the written name,
// which is the same statement from the other side:
//
//	typeset -T S s; typeset -U s; s=(p q p) s is (p q)
//	typeset -T S s; typeset -U s; S=p:q:p   s is (p q p)
//
// So a listing tells the halves apart rather than agreeing about them:
// `typeset -T S=a:b:a s; typeset -U S` lists `typeset -UT S s=( a b )` for the
// scalar and `typeset -aT S s=( a b )` for the array — deduplicated on both,
// and the letter written on one.
//
// The rest of the measurements, each a row of dialect/zsh/tiedunique_test.go:
//
//   - The **first** occurrence is the one kept, on the scalar side as on the
//     array side: `S=3:1:2:3` is `3:1:2` and `S=1:3:1` is `1:3`.
//   - The fields are the *tie's*, not a word split: with `#` as the separator
//     `S=a#b#a` is `a#b`, and an empty field is a value like any other —
//     `S=a::b::c` is `a::b:c`, four fields rather than five.
//   - The attribute applies to the value the name is **already holding** and
//     not only to later writes, which is the half the reduction in #4495 is
//     made of: the `-T` a line earlier is what assigned it.
//   - An untied scalar has nothing to split, so `v=a:b:a; typeset -U v` is
//     `a:b:a`. The letter says nothing about a scalar that is not half of a
//     tie, and that is the control this file's rule is read against.
//
// See interp/tiedscalar.go for the tie itself, and storeArray for the array
// side of the same attribute.

// uniqueStrings keeps the first occurrence of each value, in the order the
// first occurrences stand — the whole of what `-U` means, in the one place
// both halves of a tie reach it from.
//
// One function rather than a copy on each side, because the two sides were
// measured to agree: `b=(1 2 3); b=(3 $b)` reads back `3 1 2` and
// `S=3:1:2:3` reads back `3:1:2`, which is the same sentence about order and
// about which copy goes.
func uniqueStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, e := range in {
		if seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}

// tiedUniqueFold is what the unique attribute makes of a value on its way into
// the **scalar** half of a tie: the separator names the fields, the first
// occurrence of each stands, and they are joined back up.
//
// The mirror is where this is *not* asked, and that is measured rather than
// convenient: a write to the array half joins into the scalar without the
// scalar's letter having a say — `typeset -T S s; typeset -U S; s=(x y x z)`
// leaves `$S` as `x:y:x:z`. r.mirroring is true for exactly the two writes
// that are a mirror rather than a script's, which is why it is the guard here
// and in storeArray.
func (r *Runner) tiedUniqueFold(name, value string) string {
	if r.mirroring || !r.unique[name] {
		return value
	}
	t, ok := r.tied[name]
	if !ok || t.scalar != name || r.tieDetached(t) {
		return value
	}
	fields := strings.Split(value, t.sep)
	unique := uniqueStrings(fields)
	if len(unique) == len(fields) {
		return value
	}
	return strings.Join(unique, t.sep)
}

// uniquifyTiedScalar applies the attribute to the value the scalar half is
// **already holding**, which is what makes `typeset -T SCALAR=l:o:c:a:l array`
// followed by `typeset -U SCALAR` read back `l:o:c:a` (#4495).
//
// The same question `typeset -E` and `typeset -F` had to answer for a standing
// number in #4484, and the same shape of answer: an attribute that arrives at
// a name with a value is applied to that value there and then. The array half
// reaches it by being re-stored through storeArray, which is the one choke
// point every array write goes through; a scalar's value is not in that table,
// so this is its half of the same line.
//
// Not setVarAs, for the reason rereadStandingValue is not: this is an
// attribute being applied rather than an assignment, so a readonly name is
// re-read rather than refused. The mirror is called by hand for the same
// reason — it is the only part of a store this needs.
func (r *Runner) uniquifyTiedScalar(name string) {
	v, ok := r.Vars[name]
	if !ok {
		// A name still living in the inherited environment, which is where
		// an exported tie's scalar is until something writes it.
		if v, ok = r.inheritedValue(name); !ok {
			return
		}
	}
	folded := r.tiedUniqueFold(name, v)
	if folded == v {
		return
	}
	if r.Vars == nil {
		r.Vars = map[string]string{}
	}
	r.Vars[name] = folded
	r.mirrorScalarToArray(name, folded)
}
