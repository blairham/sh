// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// A `;` between the parentheses of an array literal stands exactly where a
// newline already does here. Measured 2026-09-12 on zsh 5.9.2, `-n` and then a
// run, over a script file under `env -i` with a scratch HOME and ZDOTDIR;
// ksh93 takes only a single one at the very end and every bash column refuses
// all of it.
//
//	$ zsh s.sh          # a=( x; y ); echo "n=${#a[@]}"
//	n=2
func TestASemicolonSeparatesArrayElementsHere(t *testing.T) {
	if got := zsh.Dialect().SemicolonInAnArrayLiteral; got != syntax.SemicolonSeparatesArrayElementsLikeANewline {
		t.Errorf("SemicolonInAnArrayLiteral = %v, want SemicolonSeparatesArrayElementsLikeANewline", got)
	}
}

func TestTheSeparatorStandsWhereANewlineDoesHere(t *testing.T) {
	for _, tc := range []struct {
		src, out string
	}{
		{src: `a=( x; ); echo "n=${#a[@]} all=[${a[@]}]"`, out: "n=1 all=[x]\n"},
		{src: `a=( x; y ); echo "n=${#a[@]} all=[${a[@]}]"`, out: "n=2 all=[x y]\n"},
		{src: `a=( x; y; ); echo "n=${#a[@]} all=[${a[@]}]"`, out: "n=2 all=[x y]\n"},
		// No element is needed in front of one, and two standing together
		// leave no empty element — which is the whole of "like a newline".
		{src: `a=( ; ); echo "n=${#a[@]}"`, out: "n=0\n"},
		{src: `a=( x; ; ); echo "n=${#a[@]} all=[${a[@]}]"`, out: "n=1 all=[x]\n"},
		{src: "a=( x;\ny ); echo \"n=${#a[@]}\"", out: "n=2\n"},
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

// The controls, by the token each refusal names: `;;` is still its own token
// here, and no other control operator stands between elements.
func TestTheSeparatorIsTheSemicolonAloneHere(t *testing.T) {
	for _, tc := range []struct{ src, token string }{
		{`a=( x;; y )`, ";;"},
		{`a=( x & )`, "&"},
		{`a=( x && y )`, "&&"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
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
