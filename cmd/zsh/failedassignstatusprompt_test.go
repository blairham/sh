// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// What `$?` is at a prompt after an assignment that would not expand.
//
// Measured 2026-10-15 on zsh 5.9.2 run `-f -i`, through a terminal and from
// standard input, one statement a line with `echo st=$?` after each. A
// declaration keeps `$?` as it was (0 or 7), even at 0, since an interactive
// shell does not end and so never turns that 0 into a failure. A `${q?word}`
// in a bare scalar or element assignment leaves 5, and the controls leave 1:
// a different failure, a command behind the assignment, an array literal, a
// failure ahead of it in the list, a command substitution's own failure.
// Inside a loop every one of them is 1. Ours left 1 in all of them (#6580).
func TestAFailedAssignmentsStatusAtAPrompt(t *testing.T) {
	rows := []struct{ before, line, want string }{
		{"true", "export x=$((1/0))", "0"},
		{"(exit 7)", "export x=$((1/0))", "7"},
		{"true", "f() { local y=$((1/0)); }; f", "0"},
		{"true", "export x=${q?b}", "0"},
		{"true", "case x in $((1/0))) ;; esac", "0"},
		{"true", "x=${q?boom}", "5"},
		{"(exit 7)", "x=${q:?}", "5"},
		{"true", "a[1]=${q?b}", "5"},
		{"true", "y=2 x=${q?b}", "5"},
		{"true", "g() { x=${q?b}; }; g", "5"},
		{"true", "(x=${q?b})", "5"},
		{"true", "x=$((1/0))", "1"},
		{"true", "x=${q?b} true", "1"},
		{"true", "a=(1 ${q?b})", "1"},
		{"true", "x=$((1/0)) y=${q?b}", "1"},
		{"true", "x=$(echo ${q?b})", "1"},
		{"true", "for i in 1; do x=${q?b}; done", "1"},
		{"(exit 7)", "for i in 1; do export x=$((1/0)); done", "1"},
	}
	var typed, want strings.Builder
	for i, c := range rows {
		typed.WriteString(c.before + "\n" + c.line + "\necho " + itoaRow(i) + "=$?\n")
		want.WriteString(itoaRow(i) + "=" + c.want + "\n")
	}
	t.Setenv("HOME", t.TempDir())
	out, errs, _ := prompt(t, typed.String(), "zsh", "-f", "-i")
	if out != want.String() {
		t.Errorf("stdout\n%s\nwant\n%s\nrows %v\n(stderr %q)", out, want.String(), rows, errs)
	}
}

func itoaRow(i int) string { return "row" + string(rune('a'+i)) }
