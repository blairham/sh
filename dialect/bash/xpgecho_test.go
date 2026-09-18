// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `shopt -s xpg_echo` makes `echo` interpret its backslash escapes with no
// `-e` in front of them, which is what a script written for a System V `echo`
// sets. It is the other side of Semantics.EchoInterpretsEscapes, and this is
// the only shell in the panel with a name for moving it.
//
// Refusing it was worse than one diagnostic: the name is set in an rc file
// rather than per call, so every later `echo` in the file answered the other
// way (#3059).
//
// Measured 2026-09-17 on bash 5.3.20 from a script file, each call inside a
// `$(…)` so the bytes are visible rather than joined by the terminal.
func TestXpgEchoExpandsEscapesWithoutTheLetter(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The default, and the same call with the option on.
		{`echo 'a\tb'`, "a\\tb\n"},
		{`shopt -s xpg_echo; echo 'a\tb'`, "a\tb\n"},
		// `-E` still wins, for the one call: the option moves the default
		// and the letters are answered ahead of it.
		{`shopt -s xpg_echo; echo -E 'a\tb'`, "a\\tb\n"},
		{`shopt -s xpg_echo; echo -e 'a\tb'`, "a\tb\n"},
		// And `-n` is untouched, which says the option is not about the
		// letters at all.
		{`shopt -s xpg_echo; echo -n x; echo -n y`, "xy"},
		// The escape *set* is not the option's either — which escapes exist
		// is each its own axis, and each answers the same either way.
		{`shopt -s xpg_echo; echo 'a\x41b'`, "aAb\n"},
		{`shopt -s xpg_echo; echo 'a\0101b'`, "aAb\n"},
		{`shopt -s xpg_echo; echo 'a\eb'`, "a\x1bb\n"},
		// `\c` ends the output, newline included.
		{`shopt -s xpg_echo; echo 'a\cb'; echo END`, "aEND\n"},
		// A word with no backslash in it needs no dialect and no option.
		{`shopt -s xpg_echo; echo hi there`, "hi there\n"},
		// And the way back.
		{`shopt -s xpg_echo; shopt -u xpg_echo; echo 'a\tb'`, "a\\tb\n"},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// The name is answered by the builtin the way every wired name is, and a
// subshell's request stays in the subshell — which the option gets for free by
// being state on the runner.
func TestXpgEchoIsAnOptionRatherThanARecordedName(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{`shopt -s xpg_echo; echo "st=$?"`, "st=0\n", 0},
		{`shopt -s xpg_echo; shopt -p xpg_echo`, "shopt -s xpg_echo\n", 0},
		{`shopt -p xpg_echo`, "shopt -u xpg_echo\n", 1},
		{`shopt -s xpg_echo; shopt xpg_echo`, "xpg_echo            \ton\n", 0},
		{`shopt -u xpg_echo; echo "st=$?"`, "st=0\n", 0},
		{
			`shopt -s xpg_echo; case ":$BASHOPTS:" in *:xpg_echo:*) echo in ;; *) echo out ;; esac`,
			"in\n", 0,
		},
		{`(shopt -s xpg_echo; echo 'a\tb'); echo 'a\tb'`, "a\tb\na\\tb\n", 0},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s = %q status %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}
