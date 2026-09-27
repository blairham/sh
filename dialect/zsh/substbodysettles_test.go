// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// A `$( … )` body this grammar refuses at a token settles the read, so the
// line never parses and the commands written before the substitution on it
// never run.
//
// Measured 2026-09-27 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh`, `-f` over a script file holding
// `echo b; v=$(X); echo a`, under `set -n` and run. `refused` is the `set -n`
// answer and the run writes no `b` for exactly the same rows.
func TestARefusedSubstitutionBodyIsTheLinesHere(t *testing.T) {
	if !zsh.Dialect().SubstitutionBodyRefusalEndsTheRead {
		t.Fatal("the flag is off, so the rows below say nothing about this dialect")
	}
	for _, tc := range []struct {
		body    string
		refused bool
	}{
		{"&&", true},
		{";;", true},
		{"fi", true},
		{"done", true},
		{"esac", true},
		// The controls, and they are two different ones. A body with nothing
		// wrong in it reads; and a body the read merely **ran out** of is
		// deferred to the moment the substitution is expanded, which is the
		// reference's answer too — `$(if)` writes `b` there and then fails at
		// the substitution. That is the zsh column the corpus records for
		// `subst/a-body-that-will-not-parse-stops-the-line`, and a change
		// that settled every refused body would have taken it with it.
		{"echo hi", false},
		{"if", false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			src := "echo b; v=$(" + tc.body + "); echo a\n"
			_, err := syntax.Parse(src, zsh.Dialect())
			if got := err != nil; got != tc.refused {
				t.Fatalf("refused=%v (%v), want %v", got, err, tc.refused)
			}
			if err == nil {
				return
			}
			// And both complaints are there: the body's own, and the
			// substitution's about never closing. One dialect writes the
			// pair and it is the only thing that carries the first.
			e, ok := err.(*syntax.Error)
			if !ok {
				t.Fatalf("came back as %T, want an *Error", err)
			}
			if e.BodyRefusal == nil {
				t.Fatal("the body's own refusal was dropped, so only the second complaint is left")
			}
			if !strings.Contains(e.BodyRefusal.Error(), tc.body) {
				t.Errorf("the body's refusal does not name %q: %v", tc.body, e.BodyRefusal)
			}
		})
	}
}
