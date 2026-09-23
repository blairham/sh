// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// `typeset +A` and `+a` are refused here whatever the name holds, the sentence
// names the letter set rather than the variable, and the script is given up.
//
// Measured 2026-09-23 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C ksh f.sh`, standard input on the null device, ksh93u+ 2012-08-01:
// `typeset -A a; a[x]=1; typeset +A a; echo reached` writes
// `typeset: cannot unset attribute C or A or a` and nothing after it, and the
// same holds over an empty table, over a scalar and over a name that does not
// exist. bash refuses only where the name is an array and carries on; zsh
// refuses none of them (#4241).
func TestTheArrayAttributeLetterIsRefusedOutrightHere(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a populated table", `typeset -A a; a[x]=1; typeset +A a; printf reached`},
		{"an empty table", `typeset -A e; typeset +A e; printf reached`},
		{"a scalar", `s=plain; typeset +A s; printf reached`},
		{"a name that does not exist", `typeset +A nosuchname; printf reached`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, "cannot unset attribute C or A or a") {
				t.Errorf("= %q, want the letter-set refusal", out)
			}
			if strings.Contains(out, "reached") {
				t.Errorf("= %q, want the script given up before the next command", out)
			}
		})
	}
}

// The policy, pinned so that no preset here drifts off the column it was
// measured from.
func TestTheArrayAttributeRemovalIsThisDialectsPolicy(t *testing.T) {
	if got := ksh.Semantics().ArrayAttributeRemoval; got != interp.ArrayAttributeRemovalEndsTheScript {
		t.Errorf("ArrayAttributeRemoval = %v, want ends the script", got)
	}
}
