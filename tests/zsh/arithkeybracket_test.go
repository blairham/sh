// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// A subscript's expanded text is read again as subscript syntax here, so a
// key holding a bracket is not the key an arithmetic expression finds.
//
// Measured 2026-09-13 against zsh 5.9.2 with `key='x],b['` already stored:
// `(( m[$key]++ ))` is `not an identifier: b[]` at status 2 and the element
// is left at 1, where bash 5.3.15 and ksh93u+ both increment it. The `b[` in
// that sentence is the tail of the key, named as a second array with an
// empty subscript — which is what identifies the mechanism rather than only
// the outcome (#2581).
func TestAnExpandedSubscriptIsReadAgainAsSyntax(t *testing.T) {
	out, _ := answersRun(t, `typeset -A m; key='x],b['; m[$key]=1; (( m[$key]++ )); printf "[%s][%s]" "$?" "${m[$key]}"`)
	if !strings.Contains(out, "not an identifier: b[]") {
		t.Errorf("= %q, want it to name `b[]`", out)
	}
	// The status the arithmetic itself carried, and the element it did not
	// touch. Either alone would pass against a reading that complained and
	// wrote anyway, or one that answered quietly and did not.
	if !strings.HasSuffix(out, "[2][1]") {
		t.Errorf("= %q, want status 2 and the element left at 1", out)
	}
}

// The store and the read are not this and are right: the same key goes in and
// comes back out through the expansion route, which is what makes the
// divergence above the arithmetic route alone.
func TestTheSameKeyStoresAndReadsBack(t *testing.T) {
	if out, st := answersRun(t, `typeset -A m; key='x],b['; m[$key]=9; printf "[%s]" "${m[$key]}"`); out != "[9]" || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, "[9]")
	}
}

// The axis rather than the outcome, so a preset that stopped holding it fails
// here rather than only in a conformance run.
func TestTheRereadIsAnAxis(t *testing.T) {
	if got := zsh.Semantics().ArithSubscriptRereadsItsExpandedText; got != interp.Yes {
		t.Errorf("ArithSubscriptRereadsItsExpandedText = %v, want yes", got)
	}
}
