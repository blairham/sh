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

// A `-` on its own is an operand in three of the panel and an option in the
// fourth, which eats it and leaves the builtin one operand fewer.
//
// Invisible until something looks at the operands: `unset -` is quiet in bash
// because its bare form validates nothing, not because the dash was eaten.
// `unalias -` is where the difference shows, because a shell that eats the
// dash is then left with nothing to unalias.
func TestALoneDashIsAnOperandOrAnOption(t *testing.T) {
	for _, c := range []struct {
		name   string
		option Answer
		asked  bool
	}{
		{"kept, so the builtin is asked about it", No, true},
		{"eaten, so the builtin was given nothing to ask about", Yes, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Asserted on whether the dash reached the builtin rather than
			// on a wording: what a shell says about being given nothing is
			// its own, and this is about which operands it was given.
			out := loneDashRun(t, "unalias -\n", c.option)
			if asked := strings.Contains(out, "-: not found"); asked != c.asked {
				t.Errorf("said %q; the dash reached the builtin = %v, want %v", out, asked, c.asked)
			}
		})
	}
}

// Only a dash *on its own*, and only where the options are still being read.
func TestOnlyADashOnItsOwn(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// `--` ends the options, so the dash after it is an operand even in
		// the dialect that would otherwise eat one.
		{"a dash after the end of options", "unalias -- -\n", "-: not found"},
		// And the dash is eaten wherever it stands among the options, so
		// what follows it is what the builtin is asked about. Measured:
		// `unset - X` unsets `X` in the shell that eats the dash.
		{"a dash with something after it", "unalias - nope\n", "nope: not found"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out := loneDashRun(t, c.src, Yes); !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q in it", out, c.want)
			}
		})
	}
}

// A shell with no answer refuses rather than guessing, because the two
// answers give the builtin different operands.
func TestALoneDashWithNoAnswerIsRefused(t *testing.T) {
	out, st := loneDashRun2(t, "unalias -\n", Unspecified)
	if !strings.Contains(out, "lone `-`") || !strings.Contains(out, "disagree") {
		t.Errorf("said %q, want the axis named", out)
	}
	// Refused rather than reported and carried on: naming the axis and then
	// running the builtin anyway would leave it working on operands the two
	// answers disagree about, which is the thing being refused.
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
	if strings.Contains(out, "not found") {
		t.Errorf("said %q, want the builtin not to have been asked about anything", out)
	}
}

func loneDashRun(t *testing.T, src string, option Answer) string {
	t.Helper()
	out, _ := loneDashRun2(t, src, option)
	return out
}

func loneDashRun2(t *testing.T, src string, option Answer) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.LoneDashIsAnOption = option
	// A second axis this reaches, and not the one under test: whether the
	// builtin says anything about an alias it does not have.
	sem.UnaliasReportsNotFound = Yes
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh"})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return buf.String(), st
}
