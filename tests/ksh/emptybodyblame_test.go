// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// An `&` standing where a body or a condition must have something in it is
// refused at the token *after* it, and it is the reverse of what this shell
// does with a `;` in the same place (#2235).
//
// Measured 2026-09-14 on ksh93u+ 2012-08-01, `-n` over a script file under
// `env -i PATH=/usr/bin:/bin` with a scratch HOME. dash, bash 5.3, bash 3.2
// and zsh name the `&` in every one of these.
func TestAnAmpersandBeforeAnEmptyBodyIsBlamedOnWhatFollows(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"if & then :; fi\n", "then"},
		{"while & do :; done\n", "do"},
		{"until & do :; done\n", "do"},
		{"if & fi\n", "fi"},
		{"{ & }\n", "}"},
		{"( & )\n", ")"},
		{"case x in x) & ;; esac\n", ";;"},
		{"if & ; then :; fi\n", ";"},
		{"if & || :; then :; fi\n", "||"},
		{"for i in 1; do & done\n", "done"},
		{"if :; then & fi\n", "fi"},
		// A `;` in the same position names itself here, which is the row
		// #2023 pinned and the reason this is not one rule over both
		// terminators.
		{"if ; then :; fi\n", ";"},
		// And the terminators outside the set name themselves.
		{"if | :; then :; fi\n", "|"},
		{"if && then :; fi\n", "&&"},
		{"if ;; then :; fi\n", ";;"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.Parse(tc.src, ksh.Dialect())
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
