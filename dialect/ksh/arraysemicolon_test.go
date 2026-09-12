// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// A `;` between the parentheses of an array literal *ends* the element list
// here rather than standing between two elements. Measured 2026-09-12 on
// ksh93u+, `-n` and then a run, over a script file under `env -i` with a
// scratch HOME; every bash column refuses all of it and zsh takes all of it.
//
//	$ ksh s.sh          # a=( x; y ); echo "n=${#a[@]}"
//	s.sh: syntax error at line 1: `y' unexpected
func TestASemicolonEndsTheArrayElementsHere(t *testing.T) {
	if got := ksh.Dialect().SemicolonInAnArrayLiteral; got != syntax.OneSemicolonEndsTheArrayElements {
		t.Errorf("SemicolonInAnArrayLiteral = %v, want OneSemicolonEndsTheArrayElements", got)
	}
}

func TestWhereTheTerminatorMayStandHere(t *testing.T) {
	for _, tc := range []struct {
		src, out string
	}{
		{src: `a=( x; ); echo "n=${#a[@]} all=[${a[@]}]"`, out: "n=1 all=[x]\n"},
		{src: `a=( x y; ); echo "n=${#a[@]} all=[${a[@]}]"`, out: "n=2 all=[x y]\n"},
		{src: "a=( x\n; ); echo \"n=${#a[@]}\"", out: "n=1\n"},
		{src: "a=( x;\n); echo \"n=${#a[@]}\"", out: "n=1\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)
			continue
		}
		if out != tc.out {
			t.Errorf("%q: out = %q, want %q", tc.src, out, tc.out)
		}
	}
}

// And what it refuses, by the token each refusal names — which is the half a
// person reads, and the half the shell's own answer is quoted against.
func TestWhatTheTerminatorRefusesHere(t *testing.T) {
	for _, tc := range []struct{ src, token string }{
		// A terminator stands between nothing, so the word after one is the
		// surprise rather than the `;`.
		{`a=( x; y )`, "y"},
		{`a=( x; y; )`, "y"},
		// It needs an element in front of it, and it may be written once.
		{`a=( ; )`, ";"},
		{`a=( x; ; )`, ";"},
		// The controls: `;;` is its own token, and no other control operator
		// stands between elements in any shell in the panel.
		{`a=( x;; y )`, ";;"},
		{`a=( x & )`, "&"},
		{`a=( x && y )`, "&&"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Errorf("%q: err = %v, want a syntax error", tc.src, err)
			continue
		}
		if se.Token != tc.token {
			t.Errorf("%q: named %q, want %q", tc.src, se.Token, tc.token)
		}
	}
}
