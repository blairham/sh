// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// condRun is answersRun with the grammar handed to the runner as well as to
// the parser.
//
// The matcher asks Runner.Dialect whether a bare `(a|b)` is a group, so a
// runner built without one matches under the core grammar however the source
// was parsed — which is a `[[ $k == a(b|c) ]]` that parses here and then does
// not match. The dialect binaries set it; answersRun does not.
func condRun(t *testing.T, src string) (string, int) {
	t.Helper()
	d := zsh.Dialect()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag,
		Dialect: &d, Name: "sh", Env: []string{"PATH=/usr/bin:/bin"},
	}
	zsh.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1
	}
	return buf.String(), st
}

// parseFails reports whether this dialect refuses the source outright, which
// answersRun cannot say: it fails the test on a parse error rather than
// returning one.
func parseFails(t *testing.T, src string) bool {
	t.Helper()
	_, err := syntax.Parse(src, zsh.Dialect())
	return err != nil
}

// A group standing where a pattern is read, end to end through this dialect.
//
// It is the commonest idiom in this shell's completion files — `_docker` and
// `_rg` are unusable without it — which is what puts it far above its two-file
// count in the real-script sweep (#826, #815).
func TestAGroupMayStartAPatternOperand(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`k=a; [[ $k == (a|b) ]] && echo hit || echo miss`, "hit"},
		{`k=c; [[ $k == (a|b) ]] && echo hit || echo miss`, "miss"},
		{`k=a; [[ $k = (a|b) ]] && echo hit || echo miss`, "hit"},
		{`k=a; [[ $k != (a|b) ]] && echo hit || echo miss`, "miss"},
		{`k=abc; [[ $k == (a|b)* ]] && echo hit || echo miss`, "hit"},
		{`k=xbc; [[ $k == (a|b)* ]] && echo hit || echo miss`, "miss"},
		{`k=ab; [[ $k == ((a|b)|x)(b|c) ]] && echo hit || echo miss`, "hit"},
		// The group is one word, so a blank inside it is pattern text.
		{`k="a b"; [[ $k == (a b) ]] && echo hit || echo miss`, "hit"},
		{`k=a; [[ $k == (a b) ]] && echo hit || echo miss`, "miss"},
		// Quoted it is a literal, which is the same per-span rule `a*` has.
		{`k=a; [[ $k == "(a|b)" ]] && echo hit || echo miss`, "miss"},
		{`k="(a|b)"; [[ $k == "(a|b)" ]] && echo hit || echo miss`, "hit"},
	} {
		out, _ := condRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
}

// What the printer promises is that printed source *means* the same thing, so
// the check has to be that the printed form still matches — not that printing
// is settled. A printer that quoted the group would round-trip perfectly and
// turn every pattern into a literal; measured by mutation, that is exactly
// what a stability-only test lets through.
func TestAPrintedPatternOperandStillMatches(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`k=a; [[ $k == (a|b) ]] && echo hit || echo miss`, "hit"},
		{`k=c; [[ $k == (a|b) ]] && echo hit || echo miss`, "miss"},
		{`k=abc; [[ $k == (a|b)* ]] && echo hit || echo miss`, "hit"},
		{`k=ab; [[ $k == ((a|b)|x)(b|c) ]] && echo hit || echo miss`, "hit"},
		// And the quoted one has to stay quoted, or printing would turn a
		// literal into a pattern in the other direction.
		{`k=a; [[ $k == "(a|b)" ]] && echo hit || echo miss`, "miss"},
		{`k="(a|b)"; [[ $k == "(a|b)" ]] && echo hit || echo miss`, "hit"},
	} {
		f, err := syntax.Parse(tc.src, zsh.Dialect())
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		printed := syntax.Print(f)
		out, _ := condRun(t, printed)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s printed as %q, which said %q, want %q", tc.src, printed, got, tc.want)
		}
	}
}

// And the boundary: a group is read where a *pattern* is read and nowhere
// else in the construct, so these stay refused in this dialect too.
func TestAGroupIsRefusedWhereNoPatternIsRead(t *testing.T) {
	for _, src := range []string{
		`[[ -n (a|b) ]]`,
		`k=a; [[ (a|b) == $k ]]`,
		`k=a; [[ $k == () ]]`,
		`k=a; [[ $k ==(a|b) ]]`,
	} {
		if !parseFails(t, src) {
			t.Errorf("%s: parsed, want a syntax error", src)
		}
	}
	// And the reading does not escape the operand it was turned on for: a
	// `(` after the condition is the subshell it has always been. Found by
	// mutation — leaving the flag set past the operand left every `(` after
	// a `[[ … == … ]]` scanned as a word, so `&& (echo x)` became a command
	// named `(echo x)` and still parsed.
	for _, tc := range []struct{ src, want string }{
		{`[[ 1 == 1 ]] && (echo x)`, "x"},
		{`k=a; [[ $k == (a|b) ]]; (echo w)`, "w"},
		{`k=a; [[ $k == (a|b) ]] && (echo y)`, "y"},
	} {
		out, _ := condRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}

	// While the parentheses this shell has always had keep working.
	for _, tc := range []struct{ src, want string }{
		{`k=a; [[ ( -n $k ) ]] && echo hit || echo miss`, "hit"},
		{`k=a; [[ ( $k == a ) ]] && echo hit || echo miss`, "hit"},
		{`(( 1 + 1 == 2 )) && echo hit || echo miss`, "hit"},
		{`k=a; [[ $k =~ (a|b) ]] && echo hit || echo miss`, "hit"},
	} {
		out, _ := condRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s: said %q, want %q", tc.src, got, tc.want)
		}
	}
}
