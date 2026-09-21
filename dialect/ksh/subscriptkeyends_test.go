// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// A key ends at the apostrophe-quoted run that performed an expansion in it
// here, and whatever a script wrote after that run is gone.
//
// Measured 2026-09-20 against ksh93u+ 2012-08-01 from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, standard input on the null device,
// with `typeset -A m; kq=q` and one store, listing the key the table ended
// up holding — bash 5.3.20 keeps every character of all of them and zsh
// 5.9.2 does too. See interp.Semantics.SubscriptQuotationEndsTheKey (#3968).
func TestAKeyEndsAtAQuotedExpansionHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The run ends the subscript, so its value is the tail of the key.
		{"the run ends it", `(( m[q'$kq'] = 42 ))`, "<qq>"},
		// Text after it, so the run and the text are both dropped.
		{"text after the run", `(( m[q'$kq'z] = 42 ))`, "<q>"},
		// Nothing in front either, so the key is the empty string.
		{"nothing in front", `(( m['$kq'z] = 42 ))`, "<>"},
		// The last run decides and the prefix truncates by the same rule.
		{"two runs", `(( m['$kq'z'$kq'] = 42 ))`, "<q>"},
		{"two runs and a tail", `(( m[a'$kq'b'$kq'c] = 42 ))`, "<a>"},
		// What the run contributes is its content read again, so the
		// double quotation inside it comes off too.
		{"a double quotation inside", `(( m['"$kq"'] = 42 ))`, "<q>"},
		// A run with no expansion in it is not one of these, which is the
		// control: nothing is dropped.
		{"no expansion in the run", `(( m[q'r'z] = 42 ))`, "<qrz>"},
		// Nor is an apostrophe inside a double quotation.
		{"an apostrophe inside a double quotation", `(( m["'$kq'z"] = 42 ))`, "<'q'z>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `typeset -A m; kq=q; ` + tc.src +
				`; for k in "${!m[@]}"; do printf "<%s>" "$k"; done`
			out, st := answersRun(t, src)
			if out != tc.want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// It reaches an **indexed** subscript too, because it reaches the text before
// anything reads it: the brackets are left holding nothing and element zero
// is what the store writes.
func TestAnIndexedSubscriptEndsAtOneToo(t *testing.T) {
	out, st := answersRun(t, `b=(10 20 30); i=1; (( b['$i'x] = 9 )); printf "[%s]" "${b[*]}"`)
	if out != "[9 20 30]" || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, "[9 20 30]")
	}
}

// The axis, pinned against a preset drifting off the column it was measured
// from.
func TestTheKeyEndingIsAnAxis(t *testing.T) {
	if got := ksh.Semantics().SubscriptQuotationEndsTheKey; got != interp.Yes {
		t.Errorf("SubscriptQuotationEndsTheKey = %v, want yes", got)
	}
}
