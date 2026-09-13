// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// A compound assignment holding unkeyed words is refused on a name with the
// associative attribute.
//
// Measured 2026-09-13 against ksh93u+ 2012-08-01: `typeset -A m=(alpha one)`
// is `cannot append index array to associative array m` at status 1 and the
// shell ends, where bash and zsh both read the words as alternating keys and
// values. The sentence names an index array the line does not mention and
// says "append" for a plain `=`; both are kept as written (#2611).
func TestAKeyedLiteralMayNotHoldUnkeyedWords(t *testing.T) {
	for _, src := range []string{
		`typeset -A m=(alpha one); echo after`,
		`typeset -A m=(a 1 b 2); echo after`,
		`typeset -A m; m=(a b); echo after`,
		`typeset -A m; m+=(a b); echo after`,
	} {
		out, st := kshOut(t, src)
		if st == 0 {
			t.Errorf("%s = %q at status 0, want a refusal", src, out)
			continue
		}
		if want := "cannot append index array to associative array m"; !strings.Contains(out, want) {
			t.Errorf("%s = %q, want it to name %q", src, out, want)
		}
		// The refusal ends the shell rather than the line: nothing after it
		// runs, which is the half a wording comparison cannot see.
		if strings.Contains(out, "after") {
			t.Errorf("%s = %q, want the line after it not to run", src, out)
		}
	}
}

// What is *not* refused, which is what makes this a rule about the unkeyed
// shape rather than about compound assignment to a table.
func TestAKeyedLiteralOfKeyedElementsIsTaken(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`typeset -A m=([a]=1 [b]=2); printf "[%s]" "${m[a]}" "${m[b]}"`, `[1][2]`},
		{`typeset -A m=(); printf "[%s]" "${#m[@]}"`, `[0]`},
		{`typeset -A m; m[k]=v; printf "[%s]" "${m[k]}"`, `[v]`},
		// An element that expands to nothing contributes no word, so the
		// literal is empty rather than unkeyed.
		{`typeset -A m=($nosuch); printf "[%s]" "${#m[@]}"`, `[0]`},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// The table the refusal did not replace is still standing, which is where the
// shell that refuses leaves it — and the check that the refusal is asked
// before anything is written rather than after a clear.
func TestARefusedKeyedLiteralLeavesTheTableAlone(t *testing.T) {
	out, st := kshOut(t, `typeset -A m=([x]=1); m+=(a b)`)
	if st == 0 {
		t.Fatalf("= %q at status 0, want a refusal", out)
	}
	// Read in a second run, because the first one ended.
	if out, st := kshOut(t, `typeset -A m=([x]=1); ( m+=(a b) ) 2>/dev/null; printf "[%s]" "${m[x]}"`); out != `[1]` || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, `[1]`)
	}
}

// The axis and the wording, so a preset that stopped holding either fails
// here rather than only in a conformance run.
func TestTheKeyedLiteralRefusalIsAnAxis(t *testing.T) {
	if got := ksh.Semantics().KeyedLiteralBareWordsArePairs; got != interp.No {
		t.Errorf("KeyedLiteralBareWordsArePairs = %v, want no", got)
	}
	if got := ksh.Diagnostics().KeyedLiteralBareWords; got == "" {
		t.Error("KeyedLiteralBareWords is empty, want the sentence")
	}
}
