// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestASubstitutedWordsOwnBlanksSeparateItFromItsNeighbors pins that the
// blanks at either end of a word a `-` or `+` substituted end the field in
// front of the expansion and open the one behind it, as the same text in
// `$v` does. Measured 2026-10-03 on bash 5.3.20 and dash 0.5.12.
func TestASubstitutedWordsOwnBlanksSeparateItFromItsNeighbors(t *testing.T) {
	const f = "f(){ printf '<%s>' \"$@\"; echo; }\n"
	for _, tc := range []struct{ src, want string }{
		{`f x${u:- p }y`, "<x><p><y>\n"},
		{`f x${u:-p }y`, "<xp><y>\n"},
		{`f x${u:- p}y`, "<x><py>\n"},
		{`f x${u:- }y`, "<x><y>\n"},
		{`v=1; f x${v:+ p }y`, "<x><p><y>\n"},
		{`f ${u:- p }`, "<p>\n"},
		{`IFS=:; f x${u:-p:}y`, "<xp><y>\n"},
		// An expansion's own edges inside the word count too (#5592).
		{`v=' p '; f x${u:-$v}y`, "<x><p><y>\n"},
		{`v=' p '; f x${u:- $v}y x${u:-$v }y`, "<x><p><y><x><p><y>\n"},
		{`v=' p '; f x${u:-a$v}y; w=1; f x${w:+$v}y`, "<xa><p><y>\n<x><p><y>\n"},
		{`IFS=:; v=:p:; f x${u:-$v}y`, "<x><p><y>\n"},
		// The controls: quoted blanks, and no splitting at all.
		{`f x${u:-" p" q}y`, "<x p><qy>\n"},
		{`IFS=; f x${u:- p }y`, "<x p y>\n"},
	} {
		got, _ := runBash(t, t.TempDir(), f+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
