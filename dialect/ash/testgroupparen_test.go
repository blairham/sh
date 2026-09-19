// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// Every `test` expression that gives up with a group still open is one
// sentence in this applet — `closing paren expected` — where this engine
// named the word its own reader stopped at.
//
// Measured 2026-09-18 against BusyBox v1.37.0 in the pinned Alpine image
// (alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b),
// `cmd/ash` cross-compiled for linux/arm64 and run in the same container,
// each probe a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with
// standard input on /dev/null. Thirty probes, and after this change all
// thirty agree byte for byte (#3665).
//
// The status is 2 in both columns throughout, so what moves is the sentence.
func TestAnUnclosedGroupIsOneSentence(t *testing.T) {
	for _, src := range []string{
		`[ \( x y \) ]`,
		`[ \( -Q x \) ]`,
		`[ \( -a x \) ]`,
		`[ \( -n x y \) ]`,
		`[ \( -n x ]`,
		`[ \( \) ]`,
		`[ \( \) x ]`,
		`[ \( -a ]`,
		`[ \( \( x \) ]`,
		`[ \( \( \) \) ]`,
		`[ \( x y z \) ]`,
		`[ \( x y \) junk ]`,
		`[ \( x y \) -a z ]`,
		`[ x -a \( y ]`,
		`[ x -a \( y z \) ]`,
		`[ x -a \( \) ]`,
		`[ \( x \) -a \( y z \) ]`,
		`[ \( \) junk ]`,
	} {
		out, st := run(t, src)
		if st != 2 {
			t.Errorf("%s: status %d, want 2 (%q)", src, st, out)
		}
		if !strings.Contains(out, "closing paren expected") {
			t.Errorf("%s: said %q, want `closing paren expected`", src, out)
		}
	}
}

// The controls, and they are what say the rule is about a group still being
// open rather than about a parenthesis being present at all: once the group
// has closed, the leftover is named as an ordinary word — including a second
// `(`, which is not read as an opener — and an expression that never opened
// one is untouched.
func TestALeftoverPastAClosedGroupIsNamedAsAWord(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`[ \( x \) junk ]`, "junk: unknown operand"},
		{`[ \( x \) junk more ]`, "junk: unknown operand"},
		{`[ \( x \) \( y \) ]`, "(: unknown operand"},
		{`[ x \) ]`, "): unknown operand"},
	} {
		out, st := run(t, c.src)
		if st != 2 || !strings.Contains(out, c.want) {
			t.Errorf("%s: %q at %d, want %q", c.src, out, st, c.want)
		}
		if strings.Contains(out, "closing paren expected") {
			t.Errorf("%s: said %q, and this group closed", c.src, out)
		}
	}
	// And the expressions that are simply true or false stay so.
	for _, c := range []struct {
		src string
		st  int
	}{
		{`[ \( x \) ]`, 0},
		{`[ \( x = x \) ]`, 0},
		{`[ \( 1 -eq 1 \) ]`, 0},
		{`[ \( x \) -a \( y \) ]`, 0},
		{`[ ! \( x \) ]`, 1},
		{`[ \( ! x \) ]`, 1},
	} {
		if out, st := run(t, c.src); st != c.st {
			t.Errorf("%s: %q at %d, want %d", c.src, out, st, c.st)
		}
	}
}
