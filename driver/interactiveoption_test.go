// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// An invocation's `-o <name>` decides whether the shell prompts, where the
// dialect has a name for it — #3195.
//
// The route decision used to be taken from the letter loop alone, before the
// invocation's options had reached the runner at all, so the two spellings of
// one request parted company: the option set the state — `$-` gained `i`, `[[
// -o interactive ]]` was true — and the shell read its program instead of
// prompting.
//
// The dialect declares the name and this front end has no option table of its
// own, which is what the last case here holds: the *same* invocation under a
// preset that declares nothing prompts no differently from one with no option
// at all. Without that case a front end that simply knew the word `interactive`
// would pass every row above it.
func TestAnInvocationOptionNameDecidesThePrompt(t *testing.T) {
	for _, tc := range []struct {
		name     string
		declares bool
		argv     []string
		prompts  bool
	}{
		{"the name under a minus", true, []string{"testsh", "-o", "interactive"}, true},
		{"the name under a plus", true, []string{"testsh", "+o", "interactive"}, false},
		{"the letter, for comparison", true, []string{"testsh", "-i"}, true},
		{"nothing written", true, []string{"testsh"}, false},
		// The letter and the name are one last-wins sequence.
		{"the letter then the name", true, []string{"testsh", "-i", "+o", "interactive"}, false},
		{"the name then the letter", true, []string{"testsh", "+o", "interactive", "-i"}, true},
		// And the same word under a preset that has not declared it: the
		// option is still applied — the runner has the name — and the route
		// is the program's, which is what every column without the notion
		// does with a word it has never heard of.
		{"a preset that declares no name", false, []string{"testsh", "-o", "interactive"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := shell()
			// The name has to exist in the runner's own namespace whether or
			// not the dialect declares it here, or the two arms of the last
			// case would differ by a refusal rather than by the route.
			sh.Register = func(r *interp.Runner) { r.AddSetOptions("interactive") }
			if tc.declares {
				sh.Semantics.InteractiveOptionName = "interactive"
			}
			sh.Stdin = pipeWith(t, "echo TYPED\n")
			out, errs, code := runArgs(t, sh, tc.argv...)
			both := out + errs
			if code != 0 {
				t.Fatalf("status %d, out %q, stderr %q", code, out, errs)
			}
			// Run either way, so a row that drew no prompt read its program
			// rather than doing nothing.
			if !strings.Contains(both, "TYPED") {
				t.Fatalf("said %q, want the line to have run under either answer", both)
			}
			if got := strings.Contains(both, "$ "); got != tc.prompts {
				t.Errorf("prompted = %v, want %v — said %q", got, tc.prompts, both)
			}
		})
	}
}
