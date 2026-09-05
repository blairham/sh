// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// `a[i]+=v` joins what the element already holds.
//
// It replaced it instead, which is the silent kind of wrong: the array stays
// the same length and the loop that built it reports no error, so an
// accumulate loop ends holding only its last iteration. The subscript is
// `-1` so nothing here depends on the array base.
func TestAppendingToAnArrayElement(t *testing.T) {
	out, st := runArray(t, `a=(x y); a[-1]+=Q; printf "[%s]" "${a[@]}"`)
	if out != "[x][yQ]" {
		t.Errorf("got %q, want the value joined to the element", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The subscript is what tells the two operations apart. Without one, `+=`
// adds an element after the last; with one, it appends to that element — and
// a fix that confused them would pass a test that only looked at one.
func TestASubscriptDecidesWhichAppend(t *testing.T) {
	out, _ := runArray(t, `a=(x y); a+=(z); printf "[%s]" "${a[@]}"`)
	if out != "[x][y][z]" {
		t.Errorf("array append gave %q, want an element added at the end", out)
	}
	out, _ = runArray(t, `a=(x y); a[-1]+=z; printf "[%s]" "${a[@]}"`)
	if out != "[x][yz]" {
		t.Errorf("element append gave %q, want the last element joined", out)
	}
}

// An element that was never assigned has nothing to append to, so the same
// spelling stores the value as it stands rather than refusing or leaving the
// element empty.
func TestAppendingToAnUnsetElement(t *testing.T) {
	out, st := runArray(t, `a[3]+=Q; printf "[%s]" "${a[3]}"; echo " n=${#a[@]}"`)
	if strings.TrimSpace(out) != "[Q] n=1" {
		t.Errorf("got %q, want the value placed at the subscript alone", strings.TrimSpace(out))
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	// And nothing above it is invented on the way there.
	out, _ = runArray(t, `a=(x y); a[5]+=Q; echo "${!a[@]}"`)
	if strings.TrimSpace(out) != "0 1 5" {
		t.Errorf("subscripts are %q, want only the ones assigned", strings.TrimSpace(out))
	}
}

// The array base reaches the append exactly as it reaches a plain element
// assignment: the same numeral names a different element under each answer,
// and appending is not a form with a subscript rule of its own.
func TestAppendingToAnElementInheritsTheBase(t *testing.T) {
	src := `a=(x y); a[1]+=Q; printf "[%s]" "${a[@]}"`
	if out, _ := run(t, src, withSem(baseVectors()[0])); out != "[x][yQ]" {
		t.Errorf("zero-based gave %q, want the second element joined", out)
	}
	if out, _ := run(t, src, withSem(baseVectors()[1])); out != "[xQ][y]" {
		t.Errorf("one-based gave %q, want the first element joined", out)
	}
}

// A subscript below the base is refused, and the refusal ends the script — so
// nothing appends to a position that does not exist, and nothing after the
// assignment reads the array as though something had.
func TestAppendingThroughASubscriptOutOfRange(t *testing.T) {
	out, st := run(t, `a=(x y); a[0]+=Q; printf "[%s]" "${a[@]}"`, withSem(baseVectors()[1]))
	if !strings.Contains(out, "a[0]") {
		t.Errorf("said %q, want the subscript named", out)
	}
	if strings.Contains(out, "[x][y]") {
		t.Errorf("left %q, want nothing after the refusal", out)
	}
	if st == 0 {
		t.Errorf("status 0, want a failure")
	}
}

// The declared form appends by key just as the indexed form appends by
// subscript, and an absent key is the plain assignment again.
func TestAppendingToAnAssociativeElement(t *testing.T) {
	out, _ := runArray(t, `typeset -A m; m[k]+=x; m[k]+=Q; printf "[%s]" "${m[k]}"`)
	if out != "[xQ]" {
		t.Errorf("got %q, want the first append to place and the second to join", out)
	}
	// The key is the subscript as written, so appending never evaluates it.
	out, _ = runArray(t, `typeset -A m; m[1+1]=v; m[1+1]+=Q; printf "[%s]" "${m[1+1]}"`)
	if out != "[vQ]" {
		t.Errorf("got %q, want the three characters used as one key", out)
	}
}

// A scalar `+=` still appends, and an element append must not have been
// bought by breaking it.
func TestAppendingToAScalarStillAppends(t *testing.T) {
	out, _ := runArray(t, `s=x; s+=Q; printf "[%s]" "$s"`)
	if out != "[xQ]" {
		t.Errorf("got %q, want the scalar joined", out)
	}
}
