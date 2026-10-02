// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAListsReplacementIsReadOnce: `${arr[@]/pat/repl}` expands its
// replacement once for the whole list, not once per element and not again for
// a trial of a joined reading this dialect never takes — and a pattern with
// `(#b)` reads it once per match, each after its `$match`. Measured
// 2026-10-02 on zsh 5.9.2 (#5394). zi's turbo scheduler calls a math function
// that appends to a queue from inside that replacement, so each extra reading
// queued a task: the queue gained an empty entry and `.zi-run-task` was
// reached with no index, `bad set of key/value pairs for associative array`.
func TestAListsReplacementIsReadOnce(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`i=0; c=(ab cd); x=(${c[@]/b/$((++i))}); print -r -- $x $i`, "a1 cd 1\n"},
		{`i=0; set -- ab cd ab; x=${@/b/$((++i))}; print -r -- $x $i`, "a1 cd a1 1\n"},
		{`i=0; s=xx; x=${s/b/$((++i))}; print -r -- $x $i`, "xx 1\n"},
		{
			`setopt extendedglob; i=0; c=(10y x 20y); x=(${c[@]/(#b)([0-9]##)y/$match[1]:$((++i))}); print -r -- $x $i`,
			"10:1 x 20:2 2\n",
		},
		// The scheduler's own shape, reduced: a math function called from
		// the replacement's subscript appends once per matching element.
		{
			`f() { q+=( "${a[$1]}" ); return $1 }; functions -M g 1 1 f; q=(); a=(x 10y 20y)
setopt extendedglob; integer i=2; a=( ${a[@]/(#b)([0-9]##)y/${a[$(( g(i++) ))]}} ); print -r -- "${(j:|:)q}" ${#q}`,
			"10y|20y 2\n",
		},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}
