// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `unset a[@]` empties the array here, and `unset a[*]` does the same.
// Measured against bash 5.3.15 and bash 3.2.57 (2026-09-05); the two builds
// agree on everything asserted below.
func TestUnsetOfEveryElementEmptiesTheArray(t *testing.T) {
	for _, sub := range []string{"@", "*"} {
		src := `a=(p q r); unset "a[` + sub + `]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
		out, st := runBash(t, t.TempDir(), src)
		if out != "[] n=0\n" || st != 0 {
			t.Errorf("[%s] = %q (status %d), want %q", sub, out, st, "[] n=0\n")
		}
	}
}

// Nothing is left for an append to land after, which is what tells this
// answer from one that leaves a single empty element behind.
func TestUnsetOfEveryElementLeavesNothingForAnAppend(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		`a=(p q); unset "a[@]"; a+=(z); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if out != "[z] n=1\n" || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, "[z] n=1\n")
	}
}

// A scalar has no elements to take away, so the spelling is refused at 1 and
// the value is left alone. Same wording in bash 3.2.
func TestUnsetOfEveryElementRefusesAScalar(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `a=hello; unset "a[@]"; echo "st=$? [$a]"`)
	want := "bash: line 1: unset: a: not an array variable\nst=1 [hello]\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// A single subscript takes the element away here, so unsetting the last one
// shortens the array. Measured against bash 5.3.15 and bash 3.2.57
// (2026-09-05), which agree.
func TestUnsetOfTheLastElementRemovesIt(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		`a=(x y z); unset "a[2]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if out != "[x][y] n=2\n" || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, "[x][y] n=2\n")
	}
}

// And the next append lands in the place it left, which is the reading a
// shell that blanks does not have.
func TestUnsetOfTheLastElementFreesThePlaceAnAppendTakes(t *testing.T) {
	out, st := runBash(t, t.TempDir(),
		`a=(p q r); unset "a[2]"; a+=(z); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if out != "[p][q][z] n=3\n" || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, "[p][q][z] n=3\n")
	}
}
