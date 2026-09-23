// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestAssigningThroughAReferenceInACycleWarnsRatherThanAiming — a reference in
// a cycle is **aimed**, not unaimed: the walk cannot hand back a name, but the
// reference has one written in it, so an assignment is a write through it
// rather than an invitation to re-point it. The write lands nowhere and the
// shell says why.
//
// This shell read the walk's "not aimed" as the shape `typeset -n r; r=v` has,
// and tried to aim the reference at the value — which it then refused as a bad
// name, a sentence about the value where the reference's is about the cycle.
//
// Measured 2026-09-23 against bash 5.3.15 in the pinned debian:sid-slim and
// bash 5.3.20 on macOS, which agree (#4178).
func TestAssigningThroughAReferenceInACycleWarnsRatherThanAiming(t *testing.T) {
	out, errs := runBashSplitFatal(t, `typeset -n ref=re re=ref
re=4
typeset -p ref re
echo "read: ${ref-unset}"
`)
	if want := "declare -n ref=\"re\"\ndeclare -n re=\"ref\"\nread: unset\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if !strings.Contains(errs, "warning: re: circular name reference") {
		t.Errorf("stderr = %q, want the cycle named for the name assigned to", errs)
	}
	if strings.Contains(errs, "not a valid identifier") {
		t.Errorf("stderr = %q, want no sentence about the value", errs)
	}
	// The control, and it is what says a cycle is not the unaimed shape: a
	// reference with nothing written in it is aimed by its first value.
	out, errs = runBashSplitFatal(t, "typeset -n r\nr=v\ntypeset -p r\n")
	if want := "declare -n r=\"v\"\n"; out != want {
		t.Errorf("control: got %q, want %q", out, want)
	}
	if errs != "" {
		t.Errorf("control: stderr = %q, want nothing", errs)
	}
}
