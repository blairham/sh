// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The parameters the substrate provides itself, which are the three the panel
// agrees on. This names the behavior rather than a shell, as the rule for
// this package requires; which dialect has UID or RANDOM is asserted in
// dialect/.
func TestTheSubstrateProvidesTheUnanimousParameters(t *testing.T) {
	// IFS is the one that looked least urgent and mattered most: splitting
	// already used a default when it was unset, so everything *worked* while
	// a script could neither read it nor tell it had been changed.
	if out, _ := run(t, `printf '%s' "$IFS"`, nil); out != " \t\n" {
		t.Errorf("IFS = %q, want space, tab and newline", out)
	}
	if out, _ := run(t, `[ -n "$PPID" ] && echo have`, nil); strings.TrimSpace(out) != "have" {
		t.Errorf("PPID: got %q", out)
	}
	// Produced when read: a stored copy would be the line the shell started
	// on, so the two lines here would print the same number.
	out, _ := run(t, "echo \"$LINENO\"\necho \"$LINENO\"", nil)
	if out != "1\n2\n" {
		t.Errorf("LINENO gave %q, want each line to report itself", out)
	}
}

// A produced parameter cannot be overwritten by assigning to it, and what was
// assigned is kept where its producer can see it.
func TestAssigningAProducedParameterReachesItsProducer(t *testing.T) {
	setup := func(r *Runner) {
		r.SetDynamic("COUNTER", func(rr *Runner) string {
			if v, ok := rr.Assigned("COUNTER"); ok {
				return "from:" + v
			}
			return "produced"
		})
	}
	if out, _ := run(t, `echo "$COUNTER"`, setup); strings.TrimSpace(out) != "produced" {
		t.Errorf("got %q, want the produced value", out)
	}
	// The assignment does not shadow the producer — it reaches it.
	if out, _ := run(t, `COUNTER=7; echo "$COUNTER"`, setup); strings.TrimSpace(out) != "from:7" {
		t.Errorf("got %q, want the producer to have seen the assignment", out)
	}
	// And `unset` still takes it away, ahead of both.
	if out, _ := run(t, `unset COUNTER; echo "[${COUNTER-gone}]"`, setup); strings.TrimSpace(out) != "[gone]" {
		t.Errorf("got %q, want it removed", out)
	}
}

// A parameter a dialect does not provide stays absent, which is what makes
// `${RANDOM-}` a usable test for having it.
func TestAnUnprovidedParameterIsAbsent(t *testing.T) {
	if out, _ := run(t, `[ -n "${NOSUCHPARAM-}" ] && echo have || echo none`, nil); strings.TrimSpace(out) != "none" {
		t.Errorf("got %q, want none", out)
	}
}
