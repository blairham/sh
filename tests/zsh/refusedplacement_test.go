// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An array literal one of whose elements is refused where it would go places
// nothing at all: a name holding an array keeps it, and any other is left an
// empty array. Measured 2026-10-30 on zsh 5.9.2 from a file (`-f`), this
// script, byte for byte. Here the elements in front of the refusal used to
// stay — in the stored array itself, for an append — and a name that was not
// an array was left as it was (#6852).
//
// The last row is the control: a literal that failed to *expand* leaves an
// unset name unset, so it is the refused placement and not any failure that
// makes the name an array.
func TestARefusedPlacementPlacesNothing(t *testing.T) {
	src := `row() { ( trap "typeset -p b" EXIT; eval "$1" ) 2>/dev/null; }` + "\n" +
		"row 'b=(7); b=(1 [-5]=x 3)'\n" +
		"row 'b=(7); b=(1 [1+]=x 3)'\n" +
		"row 'b=(7); b+=(1 [-5]=x 3)'\n" +
		"row 'b=(7); b+=(1 [1+]=x 3)'\n" +
		"row 'b=(7); b=(1 [$(true)]=x 3)'\n" +
		"row 'unset b; b=(1 [-5]=x 3)'\n" +
		"row 'b=s; b=(1 [-5]=x 3)'\n" +
		"row 'b=s; b+=(1 [-5]=x 3)'\n" +
		"row 'typeset -i b=3; b=(1 [-5]=x 3)'\n" +
		"row 'unset b; b=(1 $((1/0)) 3)'\n"
	want := "typeset -g -a b=( 7 )\n" +
		"typeset -g -a b=( 7 )\n" +
		"typeset -g -a b=( 7 )\n" +
		"typeset -g -a b=( 7 )\n" +
		"typeset -g -a b=( 7 )\n" +
		"typeset -g -a b=(  )\n" +
		"typeset -g -a b=(  )\n" +
		"typeset -g -a b=(  )\n" +
		"typeset -a b=(  )\n"
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}
