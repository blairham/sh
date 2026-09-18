// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// The colon form of a conditional operator on a whole array tests this
// shell's **first element** alone, however many non-empty elements follow it.
// See Semantics.WholeArrayColonTest for the panel (#3425).
func TestTheColonTestReadsTheFirstElement(t *testing.T) {
	if got, want := ksh.Semantics().WholeArrayColonTest,
		interp.WholeArrayColonTestReadsTheFirstElement; got != want {
		t.Errorf("WholeArrayColonTest = %v, want %v", got, want)
	}
	for _, tc := range []struct{ name, src, want string }{
		{"an empty element first takes the default", `a=("" c); printf "[%s]" "${a[@]:-x}"`, "[x]"},
		{"and drops the alternate", `a=("" c); printf "[%s]" "${a[@]:+y}"`, "[]"},
		{"the star spelling answers with it", `a=("" c); printf "[%s]" "${a[*]:-x}"`, "[x]"},
		{"an empty element last is the control", `b=(c ""); printf "[%s]" "${b[@]:-x}"`, "[c][]"},
		{"and so is a list of ordinary values", `h=(a b); printf "[%s]" "${h[@]:-x}"`, "[a][b]"},
		{"the positional list is the same question", `set -- "" c; printf "[%s]" "${@:-x}"`, "[x]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := answersRun(t, tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
