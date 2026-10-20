// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// A subscript whose brackets arrived already word-expanded is not a quoting
// context here either, and that is measured rather than inherited from the
// written spelling's answer.
//
// Measured 2026-09-20 against zsh 5.9.2 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with `typeset -A a; a[k]=1; q='"k"'; a[$q]=2`: `let '++a["k"]'` leaves the
// bare key at 1 and the three-character key at 3, and `(( ++a["k"] ))` does
// the same — where bash 5.3.20 takes the quotes off on both routes and
// ksh93u+ 2012-08-01 keeps them through the builtin and takes them off
// through `(( … ))`.
//
// A **pin and not a control**, and labeled as one, exactly as the neighboring
// ArithSubscriptQuotationMustClose is: SubscriptIsAQuotingContext is already
// no here, so no subscript of this preset is ever a quoting context and the
// two routes cannot part. It is answered rather than left open because an
// unanswered axis is a refusal, and a reading a dialect cannot reach must not
// be able to produce one. See
// interp.Semantics.ArrivedSubscriptIsAQuotingContext (#3871).
func TestTheArrivedSubscriptQuotingIsAnswered(t *testing.T) {
	if got := zsh.Semantics().ArrivedSubscriptIsAQuotingContext; got != interp.No {
		t.Errorf("ArrivedSubscriptIsAQuotingContext = %v, want no", got)
	}
	// The answer this pin follows, so that a preset moving the reachable one
	// without the other is what fails rather than a silent split.
	if got := zsh.Semantics().SubscriptIsAQuotingContext; got != interp.No {
		t.Errorf("SubscriptIsAQuotingContext = %v, want no", got)
	}
}

// The key each route reaches, which must be the same one under either answer
// here — the claim the pin above stands on.
func TestBothRoutesReachTheSameKeyHere(t *testing.T) {
	const read = `; printf "[%s][%s]" "${a[k]}" "${a[$q]}"`
	const quoted = `typeset -A a; a[k]=1; q='"k"'; a[$q]=2; `
	for _, tc := range []struct{ name, src string }{
		{"through the builtin", quoted + `let '++a["k"]'`},
		{"through the expression", quoted + `(( ++a["k"] ))`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The bare key is untouched and the quoted one moved: the
			// brackets named the three characters between them, which is
			// the key the table stored under, since a value's quotes are
			// characters here exactly as an operand's are.
			out, st := answersRun(t, tc.src+read)
			if out != "[1][3]" || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, "[1][3]")
			}
		})
	}
}
