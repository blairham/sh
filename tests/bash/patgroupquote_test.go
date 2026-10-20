// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// Quoted text inside an extended pattern group is literal here too (#1248).
//
// The bug was filed against the shell with *bare* groups, where it was a
// parse error and loud. This shell has the same group behind `@`, and there
// it was the silent half on its own: `[[ b == @("b") ]]` parsed, asked
// whether `b` is the three characters `"b"`, and answered no at status 1.
// Measured on bash 5.3.15 — it matches there — so the row is a divergence and
// not a preference.
func TestQuotedTextInsideAnExtendedPatternGroupIsLiteral(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`shopt -s extglob; [[ b == @("b") ]] && echo Y || echo N`, "Y"},
		{`shopt -s extglob; [[ 'a;b' == @("a;b") ]] && echo Y || echo N`, "Y"},
		{`shopt -s extglob; [[ b == @(a|"b") ]] && echo Y || echo N`, "Y"},
		// The discriminator, and the reason "does it match" is not enough on
		// its own: a quoted star is a star and matches nothing else.
		{`shopt -s extglob; [[ 'a*b' == @(a"*"b) ]] && echo Y || echo N`, "Y"},
		{`shopt -s extglob; [[ axb == @(a"*"b) ]] && echo Y || echo N`, "N"},
		// An escape was already right here, because this shell's backslash
		// reaches every character in a pattern — which is why the quoting
		// half could sit under it unnoticed.
		{`shopt -s extglob; [[ b == @(\b) ]] && echo Y || echo N`, "Y"},
		{`shopt -s extglob; [[ 'a|b' == @(a\|b) ]] && echo Y || echo N`, "Y"},
		// And a live group still alternates, which is the control.
		{`shopt -s extglob; [[ ax == @(a|b)x ]] && echo Y || echo N`, "Y"},
		{`shopt -s extglob; [[ cx == @(a|b)x ]] && echo Y || echo N`, "N"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

// An expansion inside an extended pattern group is read as one, and its
// **value** is the pattern (#1331).
//
// The same omission as the quoting half above and found the same way: the
// group scanner had scanWord's quoting cases copied into it and none of its
// expansion cases, so `L=wait; [[ wait == @($L) ]]` asked whether `wait` is
// the two characters `$L`. There was no parse failure standing beside it
// here, which is why the bare-group shell is where it was noticed.
//
// bash then differs from that shell on what the value is *worth*: it globs
// the result of an expansion, so an alternation arriving in one is a real
// alternation. Measured on bash 5.3.15, 2026-09-08.
func TestAnExpansionInsideAnExtendedPatternGroupIsItsValue(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`shopt -s extglob; L=wait; [[ wait == @($L) ]] && echo Y || echo N`, "Y"},
		{`shopt -s extglob; L=wait; [[ wait == @(${L}) ]] && echo Y || echo N`, "Y"},
		{`shopt -s extglob; L=wait; [[ wait == @($(echo wait)) ]] && echo Y || echo N`, "Y"},
		{`shopt -s extglob; L=wa; M=it; [[ wait == @($L$M) ]] && echo Y || echo N`, "Y"},
		// The discriminating row: the source text is not the pattern.
		{`shopt -s extglob; L=wait; [[ '$L' == @($L) ]] && echo Y || echo N`, "N"},
		// And the axis, where this shell parts company with the one that has
		// bare groups: a `|` from a value divides here.
		{`shopt -s extglob; M='a|b'; [[ a == @($M) ]] && echo Y || echo N`, "Y"},
		{`shopt -s extglob; M='a|b'; [[ 'a|b' == @($M) ]] && echo Y || echo N`, "N"},
		{`shopt -s extglob; M='a|b'; [[ a == @("$M") ]] && echo Y || echo N`, "N"},
		// The substitution route, which has the group without the option.
		{`L=wait; v=xwaity; echo "[${v#*@($L)}][${v%%@($L)*}]"`, "[xwaity][xwaity]"},
		{`shopt -s extglob; L=wait; v=xwaity; echo "[${v#*@($L)}][${v%%@($L)*}]"`, "[y][x]"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}
