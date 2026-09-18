// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `readonly -n`, which bash takes and this shell refused as an invalid
// option. The letter suppresses the freeze this call would have made and does
// nothing else — it is not `export -n` under another word, since nothing is
// taken off.
//
// Measured 2026-09-18 on bash 5.3.20 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME (#3464).

func TestReadonlyTakesTheNLetterAndDeclaresAnUnfrozenName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct {
		src, want string
		st        int
	}{
		{
			`v=1; readonly -n r=v; echo a=$?; declare -p r; r=5; echo "b=$? v=$v r=$r"`,
			"a=0\ndeclare -- r=\"v\"\nb=0 v=1 r=5\n", 0,
		},
		// Nothing is unfrozen: a name the shell already froze stays frozen,
		// and a write through the letter to one is the ordinary refusal.
		{
			`readonly q=1; readonly -n q; echo c=$?; declare -p q`,
			"c=0\ndeclare -r q=\"1\"\n", 0,
		},
		// The append is joined and the result is still unfrozen.
		{
			`w=a; readonly -n w+=b; echo d=$?; declare -p w; w=c; echo "e=$? w=$w"`,
			"d=0\ndeclare -- w=\"ab\"\ne=0 w=c\n", 0,
		},
	} {
		out, st := runBash(t, dir, row.src)
		if out != row.want || st != row.st {
			t.Errorf("%s\n got %q (status %d)\nwant %q (status %d)",
				row.src, out, st, row.want, row.st)
		}
	}
}

// With no operand it is the listing a bare `readonly` writes, which is what
// keeps the letter from reaching the name loop with nothing to declare.
func TestReadonlyWithTheNLetterAndNoOperandIsTheListing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runBash(t, dir, `readonly a=1; b=2; readonly -n`)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	bare, _ := runBash(t, dir, `readonly a=1; b=2; readonly`)
	if out != bare {
		t.Errorf("readonly -n = %q, readonly = %q; want the same listing", out, bare)
	}
}

// And the letters beside it are unchanged: `-x` is still refused, which is
// the control that says `n` was added and the set was not widened.
func TestReadonlyStillRefusesTheLettersItHasNotGot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runBash(t, dir, `readonly -nx s=t`)
	if st == 0 {
		t.Fatalf("readonly -nx = %q at 0; want the x refused", out)
	}
}
