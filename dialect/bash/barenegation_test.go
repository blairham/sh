// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// A `!` with no pipeline after it is a pipeline of its own here, and it
// answers 1 — the negation of a success, since nothing ran. Measured
// 2026-09-12 on bash 5.3.15 and on the same binary invoked as `sh`, `env -i
// PATH=/usr/bin:/bin` with a scratch HOME.
//
//	$ bash -c 'true; !; echo "st=$?"'    st=1
//	$ bash -c 'false; !; echo "st=$?"'   st=1
//
// bash 3.2 refuses every one of these, and `bash --posix` on the 5.3 build
// takes them, so it is the version rather than POSIX mode (#948).
func TestABareNegationStandsBeforeATerminatorHere(t *testing.T) {
	if got := bash.Dialect().BareNegationReach; got != syntax.BareNegationBeforeATerminator {
		t.Errorf("reach = %v, want BareNegationBeforeATerminator", got)
	}
	for _, tc := range []struct{ src, want string }{
		{`true; !; echo "st=$?"`, "st=1"},
		{`false; !; echo "st=$?"`, "st=1"},
		{"!\n" + `echo "st=$?"`, "st=1"},
		{`{ ! ; echo "st=$?"; }`, "st=1"},
		{`x() { ! ; }; x; echo "st=$?"`, "st=1"},
		// The `&` is in this reach and the background job's own 0 is what
		// stands, which is the row that says the `&` really was taken.
		{`! & echo "st=$?"`, "st=0"},
		// And a second `!` toggles rather than being refused.
		{`! ! true; echo "st=$?"`, "st=0"},
		{`! ! false; echo "st=$?"`, "st=1"},
		{`! !; echo "st=$?"`, "st=0"},
		{`! ! !; echo "st=$?"`, "st=1"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
}

// And the reach stops at a terminator: a closer or an and-or operator is not
// one, so those lines are refused here and run in the two shells whose reach
// is the list's end.
func TestABareNegationDoesNotReachAListEndHere(t *testing.T) {
	for _, src := range []string{
		"( ! )\n",
		"{ ! }\n",
		"case x in x) ! ;; esac\n",
		"! && echo two\n",
		"! || echo two\n",
		// No column takes a bar, which is what says the reach is about
		// where a *list* may end rather than about operators in general.
		"! | cat\n",
	} {
		if _, err := syntax.Parse(src, bash.Dialect()); err == nil {
			t.Errorf("%q parsed, want a syntax error", src)
		}
	}
}
