// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// A run-time option can change the grammar, and the front end is where the
// runner's answer reaches the parser — the same joint as aliases. The test
// names the flag and registers its own builtin to flip it, because which
// builtin flips it under which name is a dialect's business.
func TestARunTimeGrammarChangeReachesTheNextLine(t *testing.T) {
	withToggle := func() driver.Shell {
		sh := shell()
		sh.Register = func(r *interp.Runner) {
			r.Register("groups-on", func(r *interp.Runner, _ context.Context, _ []string) int {
				r.SetMatchOption(interp.QuantifiedGroupsEverywhere, true)
				return 0
			})
		}
		return sh
	}

	// The core has no quantified groups, so the same second line is a group
	// once the first line has run and a syntax error when it has not.
	out, _, code := runArgs(t, withToggle(), "testsh", "-c",
		"groups-on\ncase ab in @(ab|cd)) echo hit;; esac\n")
	if code != 0 || strings.TrimSpace(out) != "hit" {
		t.Errorf("after the toggle: %q status %d, want hit", out, code)
	}

	_, errs, code := runArgs(t, withToggle(), "testsh", "-c",
		"case ab in @(ab|cd)) echo hit;; esac\n")
	if code == 0 || errs == "" {
		t.Errorf("without the toggle: status %d stderr %q, want a parse failure", code, errs)
	}

	// A script file goes through the same loop, so the same script answers
	// the same way — the route must not change the grammar either.
	out, _, code = runArgs(t, withToggle(), "testsh",
		writeScript(t, "groups-on\ncase ab in @(ab|cd)) echo hit;; esac\n"))
	if code != 0 || strings.TrimSpace(out) != "hit" {
		t.Errorf("by script file: %q status %d, want hit", out, code)
	}
}
