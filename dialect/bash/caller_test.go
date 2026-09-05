// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"
)

// `caller` reads the stack the three arrays name, one step up. Measured
// 2026-09-04, bash 5.3, from a script two calls deep: bare caller prints the
// line the current frame was entered from and the file of the frame above —
// `NULL` where there is none — and `caller N` adds the function, with the
// script's own frame answering as `main`. A depth past the stack is silence
// and 1; the top level of a script still answers bare caller with `0 NULL`.
func TestCallerWalksTheStack(t *testing.T) {
	for _, tc := range []struct{ name, src, file, want string }{
		{
			// Laid out as the measured script was: f on line 1, g calling it
			// on line 2, the call to g on line 3.
			"bare, two deep",
			"f(){ caller; }\ng(){ f; }\ng",
			"/s/main.sh", "2 /s/main.sh",
		},
		{
			"depth 0 names the calling function",
			"f(){ caller 0; }\ng(){ f; }\ng",
			"/s/main.sh", "2 g /s/main.sh",
		},
		{
			"depth 1 reaches main",
			"f(){ caller 1; }\ng(){ f; }\ng",
			"/s/main.sh", "3 main /s/main.sh",
		},
		{
			"past the stack: silence, and 1",
			`f(){ caller 2; echo st=$?; }; g(){ f; }; g`,
			"/s/main.sh", "st=1",
		},
		{
			"the top of a script is 0 NULL, status 0",
			`caller; echo st=$?`,
			"/s/main.sh", "0 NULL\nst=0",
		},
		{
			// No script file, so no frame above the function — measured, -c
			// prints NULL where a script prints its own path.
			"from -c, the file is NULL",
			`f(){ caller; }; f`,
			"", "1 NULL",
		},
		{
			"the top of -c has no frame at all",
			`caller; echo st=$?`,
			"", "st=1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runWithScriptFile(t, tc.src, tc.file); got != tc.want {
				t.Errorf("out = %q, want %q", got, tc.want)
			}
		})
	}
}

// The refusals, measured: a word that is not a plain number is `invalid
// number` — `1+1` among them, so no arithmetic happens here — and one leading
// with `-` is `invalid option`, each with the usage line after it and 2.
func TestCallerRefusesWhatIsNotADepth(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			`f(){ caller x; echo st=$?; }; f`,
			"testsh: line 1: caller: x: invalid number\ncaller: usage: caller [expr]\nst=2",
		},
		{
			`f(){ caller 1+1; echo st=$?; }; f`,
			"testsh: line 1: caller: 1+1: invalid number\ncaller: usage: caller [expr]\nst=2",
		},
		{
			`f(){ caller -1; echo st=$?; }; f`,
			"testsh: line 1: caller: -1: invalid option\ncaller: usage: caller [expr]\nst=2",
		},
	} {
		if got := runWithScriptFile(t, tc.src, ""); got != tc.want {
			t.Errorf("%s: out = %q, want %q", tc.src, got, tc.want)
		}
	}
}
