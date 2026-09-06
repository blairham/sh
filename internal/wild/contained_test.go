// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// In-package, because the policy a contained run is handed is not exported
// and should not be: it is what this sweep means by contained, not a knob.
package wild

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/policy"
	"github.com/blairham/sh/interp"
)

// TestContainmentPolicyMeansWhatItSays asks the real parser the questions the
// comment on it makes claims about.
//
// Through internal/policy rather than by comparing text, and that is the
// point of the test: a policy file is a claim about decisions, and comparing
// strings would pass for a file whose rules had stopped meaning anything — a
// mistyped selector parses, matches nothing, and reads exactly like a rule
// that is being obeyed. A containment that silently contained nothing would
// make every sweep under it look clean.
func TestContainmentPolicyMeansWhatItSays(t *testing.T) {
	p, err := policy.Parse(strings.NewReader(containmentPolicy("/run")))
	if err != nil {
		t.Fatalf("the policy handed to a contained run does not parse: %v", err)
	}
	for _, tc := range []struct {
		name string
		a    interp.Action
		want interp.Decision
	}{
		{"writing where it was put", interp.Action{Kind: interp.ActionOpen, Path: "/run/out", Write: true}, interp.Allow},
		{"writing deeper in it", interp.Action{Kind: interp.ActionOpen, Path: "/run/a/b", Write: true}, interp.Allow},
		{"writing anywhere else", interp.Action{Kind: interp.ActionOpen, Path: "/etc/hosts", Write: true}, interp.Deny},
		{"writing to a neighbor named alike", interp.Action{Kind: interp.ActionOpen, Path: "/runner/out", Write: true}, interp.Deny},
		{"writing to /dev/null", interp.Action{Kind: interp.ActionOpen, Path: "/dev/null", Write: true}, interp.Allow},
		{"reading its own message", interp.Action{Kind: interp.ActionOpen, Path: "/usr/share/man/x"}, interp.Allow},
		{"running a program", interp.Action{Kind: interp.ActionExec, Path: "/bin/sed"}, interp.Allow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.Allow(context.Background(), tc.a); got != tc.want {
				t.Errorf("Allow(%v) = %v, want %v", tc.a, got, tc.want)
			}
		})
	}
}
