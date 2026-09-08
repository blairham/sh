// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// An expansion inside an extended pattern group is read as one, and its
// **value** is the pattern (#1331).
//
// This shell has the group always on, with no option to turn it off, so it is
// where the omission had the widest reach and the least to give it away: no
// parse failure, no option to blame, just a pattern that quietly held the two
// characters `$L`.
//
// Measured on ksh93u+m/1.0.10, 2026-09-08. The answers are bash's, and the
// one place they part is recorded in its own row below.
func TestAnExpansionInsideAnExtendedPatternGroupIsItsValue(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`L=wait; [[ wait == @($L) ]] && echo Y || echo N`, "Y"},
		{`L=wait; [[ wait == @(${L}) ]] && echo Y || echo N`, "Y"},
		{`L=wait; [[ wait == @($(echo wait)) ]] && echo Y || echo N`, "Y"},
		// The discriminating row.
		{`L=wait; [[ '$L' == @($L) ]] && echo Y || echo N`, "N"},
		// This shell globs the result of an expansion, so a `|` from a value
		// divides.
		{`M='a|b'; [[ a == @($M) ]] && echo Y || echo N`, "Y"},
		{`M='a|b'; [[ 'a|b' == @($M) ]] && echo Y || echo N`, "N"},
		{`M='a|b'; [[ a == @("$M") ]] && echo Y || echo N`, "N"},
		// The substitution route, which needs no option here.
		{`L=wait; v=xwaity; echo "[${v#*@($L)}][${v%%@($L)*}]"`, "[y][x]"},
		{`L=wait; case xwait in x@($L)) echo Y;; *) echo N;; esac`, "Y"},
		{`L=wait; case 'x$L' in x@($L)) echo Y;; *) echo N;; esac`, "N"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}
