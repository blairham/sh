// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestFunctionsWMarksOneBodyForTheNestedLint pins `functions -W`, and the
// word the nested lint uses for a name holding a number. Measured 2026-10-02
// on zsh 5.9.2 under `-f` (#5155).
func TestFunctionsWMarksOneBodyForTheNestedLint(t *testing.T) {
	const warn = "f: scalar parameter g set in enclosing scope in function f\n"
	cases := []struct{ src, want string }{
		{"g=1; f(){ g=2 }; functions -W f; f", warn},
		{"g=1; f(){ g=2 }; functions -W f; functions +W f; f; print done", "done\n"},
		{"g=1; f(){ g=2 }; functions -W f; f(){ g=3 }; f; print done", "done\n"},
		{"g=1; f(){ g=2; h(){ g=4 }; h }; functions -W f; f", warn},
		{"functions -W nosuch; print st=$?", "st=1\n"},
		{"f(){ :; }; g(){ :; }; functions -W g; functions -W", "g () {\n\t:\n}\n"},
		{
			"integer g=5; f(){ setopt warnnestedvar; (( g=8 )) }; f",
			"f: numeric parameter g set in enclosing scope in function f\n",
		},
	}
	for _, c := range cases {
		if got, _ := runZshOnPath(t, t.TempDir(), c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
