// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// The colon form of a conditional operator on a whole array counts the
// **elements** here rather than reading the join, so an array holding one
// empty string is a value under `[@]` and null under `[*]`. See
// Semantics.WholeArrayColonTest for the panel (#3425).
func TestTheColonTestCountsTheElements(t *testing.T) {
	if got, want := zsh.Semantics().WholeArrayColonTest,
		interp.WholeArrayColonTestCountsTheElementsUnderAt; got != want {
		t.Errorf("WholeArrayColonTest = %v, want %v", got, want)
	}
	for _, tc := range []struct{ name, src, want string }{
		{"one empty element is a value", `f=(""); printf "[%s]" "${f[@]:-x}"`, "[]"},
		{"and substitutes the alternate", `f=(""); printf "[%s]" "${f[@]:+y}"`, "[y]"},
		{"the star spelling still reads the join", `f=(""); printf "[%s]" "${f[*]:-x}"`, "[x]"},
		{"an empty element first is the control", `a=("" c); printf "[%s]" "${a[@]:-x}"`, "[][c]"},
		{"and a list of ordinary values", `h=(a b); printf "[%s]" "${h[@]:-x}"`, "[a][b]"},
		{"the positional list is the same question", `set -- ""; printf "[%s]" "${@:-x}"`, "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := answersRun(t, tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
