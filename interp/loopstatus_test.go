// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// A loop's status is its body's last command once the body has run, and 0
// when it never did. The two halves are one rule and each hides the other's
// failure: a shell that only resets the status passes every zero-iteration
// case, and a shell that only reports the last command run passes every case
// where something preceded the loop and failed.
func TestALoopAnswersWithItsBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		{"a while whose body failed", `i=0; while [ $i -lt 1 ]; do i=1; false; done`, 1},
		// The row that discriminates. The condition that ended this loop is
		// false and the body's last command succeeded, so a shell reporting
		// the condition would say 1 here and the row above cannot tell.
		{"a while whose condition failed and whose body did not", `i=0; while [ $i -lt 1 ]; do i=1; true; done`, 0},
		{"an until whose body failed", `i=0; until [ $i -ge 1 ]; do i=1; false; done`, 1},
		{"an until whose condition ended it and whose body did not fail", `i=0; until [ $i -ge 1 ]; do i=1; true; done`, 0},
		{"a for whose body failed", `for i in a; do false; done`, 1},
		{"the last command of the body and not the first", `i=0; while [ $i -lt 2 ]; do false; i=$((i+1)); done`, 0},
		// `break` is a command of the body and it succeeds, so this is the
		// same rule rather than an exception to it.
		{"a break is the last command the body ran", `i=0; while [ $i -lt 3 ]; do i=$((i+1)); false; break; done`, 0},
		{"a continue is too", `i=0; while [ $i -lt 2 ]; do i=$((i+1)); false; continue; done`, 0},
		{"a body that never ran", `while false; do :; done`, 0},
		{"a body that never ran does not inherit what preceded it", `false; while false; do :; done`, 0},
		{"an until body that never ran", `false; until true; do :; done`, 0},
		{"an empty for", `false; for i in; do :; done`, 0},
		{"a redirection does not change the answer", `i=0; while [ $i -lt 1 ]; do i=1; false; done >/dev/null`, 1},
		{"a function whose last statement is a loop", `f() { i=0; while [ $i -lt 1 ]; do i=1; false; done; }; f`, 1},
		{"a C-style for whose body failed", `for ((i=0;i<1;i++)); do false; done`, 1},
		{"a C-style for whose body never ran", `false; for ((i=0;i<0;i++)); do :; done`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, st := run(t, tc.src, nil); st != tc.want {
				t.Errorf("%s: status %d, want %d", tc.src, st, tc.want)
			}
		})
	}
}

// The status a loop reports is the one an and-or chain judges, which is the
// consequence that makes the difference visible without `$?`.
func TestALoopSStatusIsWhatAChainJudges(t *testing.T) {
	src := `i=0; while [ $i -lt 1 ]; do i=1; false; done && echo yes || echo no`
	if out, _ := run(t, src, nil); out != "no\n" {
		t.Errorf("%s: got %q, want %q", src, out, "no\n")
	}
}

// What `$?` is *inside* a loop before the body has set one is a different
// question from what the loop reports, and one assignment answered both. The
// live status belongs to the last command that ran until the loop has
// something of its own to say, so the first iteration and the condition both
// see what preceded the loop.
func TestALoopDoesNotZeroTheStatusOnTheWayIn(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the first iteration of a for sees what preceded the loop",
			`false; for i in a b; do echo "it=$?"; done`,
			"it=1\nit=0\n",
		},
		{
			// The sharper one: here the answer decides what runs at all.
			"a while condition sees what preceded the loop",
			`false; while [ $? -eq 0 ]; do echo ran; break; done; echo end`,
			"end\n",
		},
		{
			"an until condition sees it too",
			`false; until [ $? -ne 0 ]; do echo ran; break; done; echo end`,
			"end\n",
		},
		{
			// And the body of a conditional loop sees the condition, which
			// ran last.
			"a while body sees its own condition",
			`false; while true; do echo "it=$?"; break; done`,
			"it=0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := run(t, tc.src, nil); out != tc.want {
				t.Errorf("%s: got %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
