// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A command's assignment prefixes are applied left to right and each one's
// right-hand side sees the ones in front of it. Unanimous across the panel —
// see the note at the top of interp/prefixsees.go for the measurement — so
// these name no shell and assert the same answer whichever way the one axis
// that reaches this code is turned.
//
// Both positions of PrefixExpandedBeforeTheRedirections are run for every
// row, because they are two different routes to the value: the ordered walk
// one of them makes ahead of its redirections expands every value before the
// route applies any, and the other leaves each route to expand as it stores.
// A fix in one and not the other is the shape this repository keeps finding.
func prefixSeesRun(t *testing.T, src string, order PrefixRedirectionOrder) string {
	t.Helper()
	sem := testSemantics()
	sem.PrefixExpandedBeforeTheRedirections = order
	return prefixAssignRun(t, src, sem)
}

func TestAPrefixSeesTheOnesInFrontOfIt(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a function",
			`f(){ printf "[%s]" "$A"; }` + "\n" + `K=v1 A=${K#v} f`,
			"[1]",
		},
		{
			"a special builtin",
			`K=v4 A=${K#v} eval 'printf "[%s]" "$A"'`,
			"[4]",
		},
		{
			"a regular builtin",
			`K=v2 A=${K#v} typeset -p A`,
			`A="2"`,
		},
		{
			"an external command",
			`K=v5 A=${K#v} /usr/bin/env`,
			"\nA=5\n",
		},
		{
			"three of them in a row",
			`f(){ printf "[%s]" "$C"; }` + "\n" + `A=a B=$A.b C=$B.c f`,
			"[a.b.c]",
		},
	} {
		for _, order := range []PrefixRedirectionOrder{
			PrefixExpandedBeforeRedirectionsAlways,
			PrefixExpandedBeforeRedirectionsNever,
			PrefixExpandedBeforeRedirectionsWhereItPersists,
		} {
			if got := prefixSeesRun(t, c.src, order); !strings.Contains(got, c.want) {
				t.Errorf("%s with %v: got %q, want it to hold %q", c.name, order, got, c.want)
			}
		}
	}
}

// And what the prefix does **not** reach, measured beside it so a wider
// reading cannot creep in: the command's own argument words are expanded
// before the prefix is applied, so `K=v3 A=${K#v} echo "[$A]"` is `[]` in
// every column of the panel.
func TestTheCommandsOwnWordsDoNotSeeThePrefix(t *testing.T) {
	const src = `K=v3 A=${K#v} printf "[%s]" "$A"`
	for _, order := range []PrefixRedirectionOrder{
		PrefixExpandedBeforeRedirectionsAlways,
		PrefixExpandedBeforeRedirectionsNever,
	} {
		if got := prefixSeesRun(t, src, order); got != "[]" {
			t.Errorf("with %v: got %q, want %q", order, got, "[]")
		}
	}
}

// And `set -x` must not change the answer. The trace expands the whole prefix
// ahead of the command so it can be written before the command's own line
// runs, which is a third place that has every value in hand before the route
// has applied one — so without the same hold, turning tracing on would have
// made `K=v1 A=${K#v} f` hand the body nothing in exactly the dialects whose
// routes expand as they store. Measured on bash 5.3.20, dash 0.5.12, zsh
// 5.9.2 and ksh93u+: all four print `fn:[1]` traced and untraced alike.
func TestTracingDoesNotChangeWhatAPrefixSees(t *testing.T) {
	const src = "f(){ printf \"[%s]\" \"$A\"; }\nset -x\nK=v1 A=${K#v} f"
	for _, order := range []PrefixRedirectionOrder{
		PrefixExpandedBeforeRedirectionsAlways,
		PrefixExpandedBeforeRedirectionsNever,
		PrefixExpandedBeforeRedirectionsWhereItPersists,
	} {
		if got := prefixSeesRun(t, src, order); got != "[1]" {
			t.Errorf("with %v: got %q, want %q", order, got, "[1]")
		}
	}
}

// The hold is taken back, so the names a prefix made visible to its siblings
// do not outlive the command by that route. Asserted at the external command,
// which is the route that makes no store of its own and so is the one a hold
// left standing would be visible in.
func TestTheHoldDoesNotOutliveTheCommand(t *testing.T) {
	const src = `A=start
K=v5 A=${K#v} /usr/bin/env >/dev/null
printf "[%s]" "$A"`
	for _, order := range []PrefixRedirectionOrder{
		PrefixExpandedBeforeRedirectionsAlways,
		PrefixExpandedBeforeRedirectionsNever,
	} {
		if got := prefixSeesRun(t, src, order); got != "[start]" {
			t.Errorf("with %v: got %q, want %q", order, got, "[start]")
		}
	}
}
