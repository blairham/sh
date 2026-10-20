// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// An option name is read here exactly as it is written: no separator comes
// out of it and no `no` goes in front of it.
//
// Measured 2026-09-18 in the digest-pinned alpine image, BusyBox v1.37.0:
// `set -o err-exit`, `set -o no_err_exit` and `set -o noerrexit` are each
// `set: illegal option -o <word>` at 1, where `set -o errexit` beside them is 0.
//
// The row is worth pinning because the shell next door does fold both, on
// every route into its namespace, and the two axes that say so —
// Semantics.OptionNamespaceIgnoresSeparators and
// Semantics.OptionNamespaceTakesANoPrefix — govern the same substrate table
// this column uses. A `No` nothing exercises is a `No` a later change can
// turn into a `Yes` in silence (#3155, #3254).
func TestAnOptionNameIsReadAsItIsWritten(t *testing.T) {
	for _, word := range []string{"err-exit", "err_exit", "noerrexit", "no_err_exit"} {
		t.Run(word, func(t *testing.T) {
			out, st := run(t, "set -o "+word+"\n")
			if st == 0 {
				t.Errorf("set -o %s was taken at 0: %q", word, out)
			}
			if !strings.Contains(out, word) {
				t.Errorf("set -o %s said %q, want the word in it", word, out)
			}
		})
	}
	// And the name the fold would have arrived at is taken, so the rows above
	// are a refusal of the *spelling* rather than of the option.
	if out, st := run(t, "set -o errexit\n"); st != 0 {
		t.Errorf("set -o errexit ended at %d: %q", st, out)
	}
}
