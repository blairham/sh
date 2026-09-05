// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A command is reported at the line it begins on, not at the line the
// statement holding it began on.
//
// `true &&` on one line and the command on the next is two commands and two
// lines. Taking the *statement's* line named the operator's instead, so a
// not-found in a `&&` chain was reported wherever the chain started — off by
// however many lines the chain had run for.
//
// Unanimous across the panel, so it is the core's behavior and not an axis.
func TestACommandIsReportedAtItsOwnLine(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want string
	}{
		{"after &&", "true &&\nnosuchcmd\n", ":2:"},
		{"after ||", "false ||\nnosuchcmd\n", ":2:"},
		{"after |", "true |\nnosuchcmd\n", ":2:"},
		{"a longer chain", "true &&\ntrue &&\nnosuchcmd\n", ":3:"},
		{"a blank line between", "true &&\n\nnosuchcmd\n", ":3:"},
		// Statements that never had the bug, so the fix must not move them.
		{"a plain second line", "true\nnosuchcmd\n", ":2:"},
		{"after a semicolon", "true;\nnosuchcmd\n", ":2:"},
		{"the first line", "nosuchcmd\n", ":1:"},
		// Inside compound commands, where the command's line is nested.
		{"in an if", "if true\nthen\nnosuchcmd\nfi\n", ":3:"},
		{"in a loop", "for i in 1\ndo\nnosuchcmd\ndone\n", ":3:"},
		{"in a brace group", "{\nnosuchcmd\n}\n", ":2:"},
		{"in a subshell", "(\nnosuchcmd\n)\n", ":2:"},
		{"after a compound", "while false\ndo :\ndone\nnosuchcmd\n", ":4:"},
		{"a group after &&", "true &&\n{ nosuchcmd; }\n", ":2:"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var errs strings.Builder
			sem := PosixSemantics()
			dg := Diagnostics{Location: LocationTightLine}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &dg, Name: "sh",
				Stdout: &strings.Builder{}, Stderr: &errs,
			})
			f, err := syntax.Parse(c.src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if got := errs.String(); !strings.Contains(got, c.want) {
				t.Errorf("said %q, want the line %s", got, c.want)
			}
		})
	}
}
