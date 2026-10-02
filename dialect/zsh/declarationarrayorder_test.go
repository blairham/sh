// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnArrayOperandIsStoredBeforeALaterOperandOnItsName: in one declaration,
// an array literal is stored where it was written, so an element operand on
// the same name after it lands in the array rather than being replaced by it.
// Measured 2026-10-02 on zsh 5.9.2, B02typeset's `can set empty array`
// (#5142).
func TestAnArrayOperandIsStoredBeforeALaterOperandOnItsName(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `fn() {
  typeset g=() g[1]=yes g[2]=no; print -l ${(t)g} $g
  typeset y=(a b) y[1]=c; print -r -- $y
  typeset w=(a b) v=2 w[2]=X; print -r -- $w $v
  local q=(1 2) q=(3); print -r -- $q
}
fn
g=(); typeset g[2]=x g=(a); print -r -- $g`)
	// The last row is the order the other way round: the element first, so
	// the literal written after it is what stays.
	if want := "array-local\nyes\nno\nc b\na X 2\n3\na\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
