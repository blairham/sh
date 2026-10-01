// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **`+g` asks for a local, and under `-m` it is the only thing that does.**
// Measured against zsh 5.9.2 with a global x=1 and each line in a function
// that then assigns x=2: a plain declaration under `+g` is a fresh local, as
// it is without the letter; a `-m` line reaches the matches where they are
// unless `+g` is written, and then makes fresh locals of them.
func TestPlusGIsLocalAndDecidesWhetherMatchingMakesLocals(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `for flags in '+g' '-h +g' '-i +g' '-h' '-i' '-h -g'; do
  for form in "-m x*" x; do
    unset x; x=1
    fn() { typeset ${=flags} ${=form}; print -rn -- "[${x-unset}] $(typeset +m x) "; x=2; }
    fn
    print -r -- "$flags $form out=$x"
  done
done`)
	want := `[] local x +g -m x* out=1
[] local x +g x out=1
[] local x -h +g -m x* out=1
[] local x -h +g x out=1
[0] integer local x -i +g -m x* out=1
[0] integer local x -i +g x out=1
[1] x -h -m x* out=2
[] local x -h x out=1
[1] integer x -i -m x* out=2
[0] integer local x -i x out=1
[1] x -h -g -m x* out=2
[1] x -h -g x out=2
`
	if out != want || st != 0 {
		t.Errorf("got (status %d):\n%s\nwant:\n%s", st, out, want)
	}
}
