// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheExternalRouteStopsAtARefusedPrefixEntry pins that a prefix in front
// of an external command is walked as far as the first refused entry and no
// further: a value in front of it, and the refused entry's own value, are
// expanded; a value behind it is not, because the command is given up there.
// The builtin and function routes already stopped there; the external one
// expanded every value on its way to the child's environment and refused
// afterwards (#6511).
//
// And that an array literal written as a prefix has its elements expanded,
// refused name or not, where the scalar spelling of the same row already was
// (#6512).
//
// Every row measured 2026-10-15 against zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`
// from `-c` and from a script file under `env -i PATH=/usr/bin:/bin`), output
// compared byte for byte.
func TestTheExternalRouteStopsAtARefusedPrefixEntry(t *testing.T) {
	const ro = "readonly r=1; "
	const roSentence = "zsh:1: read-only variable: r\n"
	const home = "zsh:1: HOME: attempt to assign array value to non-array\n"
	for _, tc := range []struct{ name, src, want string }{
		// #6511: nothing behind the refused entry is expanded.
		{"readonly, a value behind it", ro + "r=2 B=$(echo s >&2) /bin/echo ran; echo rc=$?", roSentence + "rc=1\n"},
		{"readonly, values on both sides", ro + "A=$(echo a >&2) r=$(echo R >&2) B=$(echo s >&2) /bin/echo ran; echo rc=$?", "a\nR\n" + roSentence + "rc=1\n"},
		{"readonly, a second frozen name behind it", "readonly r=1 z=1; r=2 B=$(echo s >&2) z=3 /bin/echo ran; echo rc=$?", roSentence + "rc=1\n"},
		{"readonly, a name nothing runs", ro + "r=2 B=$(echo s >&2) nosuchcmd; echo rc=$?", roSentence + "rc=1\n"},
		{"a fixed slot, a value behind it", "HOME=(x) B=$(echo s >&2) /bin/echo ran; echo rc=$?", home + "rc=1\n"},
		{"a fixed slot, a value in front of it", "B=$(echo s >&2) HOME=(x) /bin/echo ran; echo rc=$?", "s\n" + home + "rc=1\n"},

		// #6512: an array literal's own elements are expanded.
		{"a literal to a readonly name", ro + "r=($(echo s >&2)) true", "s\n" + roSentence},
		{"a literal to a readonly name, external", ro + "r=($(echo s >&2)) /bin/echo ran; echo rc=$?", "s\n" + roSentence + "rc=1\n"},
		{"a literal to a fixed slot", "HOME=($(echo s >&2)) true", "s\n" + home},
		{"a literal to a fixed slot, external", "HOME=($(echo s >&2)) /bin/echo ran; echo rc=$?", "s\n" + home + "rc=1\n"},
		{"a literal that will not expand", ro + "r=($((1/0))) /bin/echo ran; echo rc=$?", "zsh:1: division by zero\nrc=1\n"},
		{"an ordinary literal, external", "x=($(echo s >&2) b) /bin/echo ran", "s\nran\n"},
		{"an ordinary literal, builtin", "x=($(echo s >&2) b) true; echo rc=$?", "s\nrc=0\n"},
		{"an ordinary literal, function", "f() { echo in; }; x=($(echo s >&2) b) f", "s\nin\n"},

		// The controls: the scalar spelling, and a plain prefix.
		{"a scalar to a readonly name", ro + "r=$(echo s >&2) /bin/echo ran; echo rc=$?", "s\n" + roSentence + "rc=1\n"},
		{"nothing refused", "A=$(echo a >&2) B=$(echo s >&2) /bin/echo ran", "a\ns\nran\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := runZshOnPath(t, t.TempDir(), tc.src)
			if got != tc.want {
				t.Errorf("%s\ngot  %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}
