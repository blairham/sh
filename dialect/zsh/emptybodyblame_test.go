// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// A pipeline or and-or operator standing where a body or a condition must
// begin is refused at a reserved word, where the rest of the panel names the
// operator (#2235).
//
// Measured 2026-09-14 on zsh 5.9.2, `-n` over a script file under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME and ZDOTDIR. A condition is named
// at the keyword that ends its header however much stands in between; a body
// is named at a keyword standing next and at the operator otherwise.
func TestAPipelineOperatorBeforeAnEmptyBodyIsBlamedOnAKeyword(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"if | :; then :; fi\n", "then"},
		{"if | :; :; then :; fi\n", "then"},
		{"if | { :; }; then :; fi\n", "then"},
		{"if | ; then :; fi\n", "then"},
		{"if |\nthen :; fi\n", "then"},
		{"if | :; fi\n", "fi"},
		{"while | :; do :; done\n", "do"},
		{"while | :; done\n", "done"},
		{"until | :; do :; done\n", "do"},
		{"if :; then :; elif | :; then :; fi\n", "then"},
		{"if && then :; fi\n", "then"},
		{"if || then :; fi\n", "then"},
		{"if |& then :; fi\n", "then"},
		// A body, where the keyword has to stand next.
		{"if :; then | fi\n", "fi"},
		{"for i in 1; do | :; done\n", "|"},
		// The closing brace is the reserved word this shell never names.
		{"{ | }\n", "|"},
		{"( | )\n", "|"},
		{"case x in x) | ;; esac\n", "|"},
		// And an `&` there names itself, which is the reverse of ksh93's
		// answer and why the two shells need different sets.
		{"if & then :; fi\n", "&"},
		{"while & do :; done\n", "&"},
		{"if ;; then :; fi\n", ";;"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.Parse(tc.src, zsh.Dialect())
			se, ok := err.(*syntax.Error)
			if !ok {
				t.Fatalf("%q: %v, want a parse error naming %q", tc.src, err, tc.want)
			}
			if se.Token != tc.want {
				t.Errorf("%q: named %q, want %q", tc.src, se.Token, tc.want)
			}
		})
	}
}

// The control the rule above needs: this shell takes an empty body and an
// empty condition, so nothing here may turn one into a refusal.
func TestAnEmptyBodyIsStillTakenHere(t *testing.T) {
	for _, src := range []string{
		"{ }\n", "( )\n", "if ; then :; fi\n", "if :; then fi\n",
		"while :; do done\n", "case x in x) ;; esac\n", "x=$( )\n",
	} {
		if _, err := syntax.Parse(src, zsh.Dialect()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}
