// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The two answers to the array-base axis, named rather than borrowed from a
// preset: everything below that says "both bases" iterates these.
func baseVectors() []Semantics {
	zero := permissive()
	zero.ArrayBaseIsZero = Yes
	one := permissive()
	one.ArrayBaseIsZero = No
	return []Semantics{zero, one}
}

// A negative subscript counts back from the end, and asks nothing: `${a[-1]}`
// is the last element in all three shells with arrays, the one whose
// subscripts count from 1 included — so the base axis plays no part in it,
// and a shell with no answers can still read one.
func TestANegativeSubscriptCountsFromTheEnd(t *testing.T) {
	out, st := run(t, `a=(one two three); printf "[%s]" "${a[-1]}" "${a[-2]}" "${a[-3]}"`,
		withSem(CoreSemantics()))
	if out != "[three][two][one]" {
		t.Errorf("got %q, want the elements from the end", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	if strings.Contains(out, "disagree") {
		t.Errorf("got %q, want no axis asked", out)
	}
}

// The base decides what `${a[1]}` names and has no say over `${a[-1]}`: the
// two dialect answers move the positive subscript and leave the negative one
// on the last element.
func TestANegativeSubscriptIgnoresTheBase(t *testing.T) {
	src := `a=(one two three); printf "%s %s" "${a[1]}" "${a[-1]}"`
	if out, _ := run(t, src, withSem(baseVectors()[0])); out != "two three" {
		t.Errorf("zero-based gave %q, want %q", out, "two three")
	}
	if out, _ := run(t, src, withSem(baseVectors()[1])); out != "one three" {
		t.Errorf("one-based gave %q, want %q", out, "one three")
	}
}

// `a[-1]=x` replaces the last element — unanimous, whichever number the first
// element answers to.
func TestAssigningThroughANegativeSubscript(t *testing.T) {
	src := `a=(x y z); a[-1]=Q; a[-3]=P; echo "${a[@]}"`
	for _, sem := range baseVectors() {
		if out, _ := run(t, src, withSem(sem)); strings.TrimSpace(out) != "P y Q" {
			t.Errorf("got %q, want %q", strings.TrimSpace(out), "P y Q")
		}
	}
}

// Past the start, a negative subscript names nothing: reading one is no
// element at all — the same nothing `${a[9]}` on three elements is — and
// writing through one is refused rather than landing somewhere.
func TestANegativeSubscriptOutOfRange(t *testing.T) {
	out, st := run(t, `a=(x); printf "[%s]" "${a[-5]}"`, nil)
	if out != "[]" || st != 0 {
		t.Errorf("read gave %q status %d, want [] and 0", out, st)
	}
	// Writing through one is refused, and the refusal ends the script: the
	// `echo` after it never runs, which is what every shell that refuses the
	// subscript does. It used to report and carry on at 0, so the next
	// command read an array the assignment had not touched.
	out, st = run(t, `a=(x); a[-5]=q; echo "[${a[@]}]"`, nil)
	if !strings.Contains(out, "a[-5]") {
		t.Errorf("write said %q, want the subscript named", out)
	}
	if strings.Contains(out, "[x]") {
		t.Errorf("write left %q, want nothing after the refusal", out)
	}
	if st == 0 {
		t.Errorf("write status 0, want a failure")
	}
}

// The same reading inside arithmetic, where `a[-1]` needs no dollar sign: the
// element is read and written back from the end, in both bases.
func TestNegativeSubscriptsInArithmetic(t *testing.T) {
	src := `a=(10 20 30); echo $((a[-1] + a[-3])); : $((a[-2] = a[-2] + 1)); echo "${a[-2]}"`
	for _, sem := range baseVectors() {
		out, _ := run(t, src, withSem(sem))
		if strings.TrimSpace(out) != "40\n21" {
			t.Errorf("got %q, want %q", strings.TrimSpace(out), "40\n21")
		}
	}
	// Out of range is zero, exactly as a subscript past the other end is.
	if out, _ := run(t, `a=(10); echo $((a[-9]))`, withSem(baseVectors()[0])); strings.TrimSpace(out) != "0" {
		t.Errorf("out of range gave %q, want 0", strings.TrimSpace(out))
	}
}

// `unset "a[-1]"` removes the last element — the same end-relative reading,
// reached through the builtin.
func TestUnsettingANegativeSubscript(t *testing.T) {
	out, _ := runArray(t, `a=(x y z); unset "a[-1]"; echo "[${a[@]}]"`)
	if strings.TrimSpace(out) != "[x y]" {
		t.Errorf("got %q, want the last element gone", strings.TrimSpace(out))
	}
}

// The end a negative subscript counts from is one past the highest
// *subscript*, not the element count: with subscripts 0 and 5, `${a[-1]}` is
// the element at 5 and `${a[-2]}` is the unassigned 4 — nothing. And the
// positions have to be the store's for the positive spelling too, or the loop
// `${!a[@]}` exists for reads the wrong elements back.
func TestNegativeCountsFromTheHighestSubscript(t *testing.T) {
	out, _ := runArray(t, `a=(x); a[5]=y; printf "[%s]" "${a[-1]}" "${a[-2]:-gap}" "${a[5]}" "${a[1]:-gap}"`)
	if out != "[y][gap][y][gap]" {
		t.Errorf("got %q, want positions counted by subscript", out)
	}
	out, _ = runArray(t, `a=(x); a[5]=y; a[-1]=Z; echo "${!a[@]}" "${a[5]}"`)
	if strings.TrimSpace(out) != "0 5 Z" {
		t.Errorf("got %q, want the write to land on the highest subscript", strings.TrimSpace(out))
	}
}

// An associative subscript is a key, never a position: `-1` on a declared
// name is the two characters, and nothing counts from any end.
func TestAnAssociativeKeyIsNotANegativeSubscript(t *testing.T) {
	out, _ := runArray(t, `typeset -A m; m[x]=v; m[-1]=q; printf "[%s]" "${m[-1]}" "${m[x]}"`)
	if out != "[q][v]" {
		t.Errorf("got %q, want -1 stored and read as a key", out)
	}
}
