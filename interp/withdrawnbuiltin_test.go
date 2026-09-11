// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A withdrawn builtin is a third state beside registered and disabled: the
// name resolves to nothing at all, and the builtin is kept so that putting it
// back needs nothing to have been remembered.
//
// Each case asserts what the *word* does and not only what the accessor
// answers, because a seam that recorded the state and left the lookup alone
// would pass an accessor test and is exactly the bug this exists to fix.
func TestAWithdrawnBuiltinIsNotThere(t *testing.T) {
	install := func(r *Runner) {
		r.Register("zzmark", func(rr *Runner, _ context.Context, _ []string) int {
			_, _ = rr.Stdout.Write([]byte("RAN\n"))
			return 0
		})
	}

	out, _ := run(t, "zzmark; echo st=$?", install)
	if !strings.Contains(out, "RAN") {
		t.Fatalf("registered: got %q, want the builtin to have run", out)
	}

	out, _ = run(t, "zzmark; echo st=$?", func(r *Runner) {
		install(r)
		r.SetBuiltinWithdrawn("zzmark", true)
	})
	if strings.Contains(out, "RAN") || !strings.Contains(out, "st=127") {
		t.Errorf("withdrawn: got %q, want no run and a 127", out)
	}

	// And back, without the caller having kept the function.
	out, _ = run(t, "zzmark; echo st=$?", func(r *Runner) {
		install(r)
		r.SetBuiltinWithdrawn("zzmark", true)
		r.SetBuiltinWithdrawn("zzmark", false)
	})
	if !strings.Contains(out, "RAN") || !strings.Contains(out, "st=0") {
		t.Errorf("put back: got %q, want the same builtin to have run", out)
	}
}

// It is not `enable -n`, and the difference is what a *listing* says. A
// disabled builtin is a name the shell still knows about and will put back;
// a withdrawn one is not there, and must not appear in the list of names
// somebody switched off.
func TestAWithdrawnBuiltinIsNotADisabledOne(t *testing.T) {
	out, _ := run(t, "enable -n\necho end", func(r *Runner) {
		r.Register("zzmark", func(_ *Runner, _ context.Context, _ []string) int { return 0 })
		r.SetBuiltinWithdrawn("zzmark", true)
	})
	if strings.Contains(out, "zzmark") {
		t.Errorf("got %q, want the withdrawn name absent from the disabled listing", out)
	}
}

// The state is per-Runner and goes into a subshell with the rest of the
// tables, and a withdrawal made inside one does not reach back out.
func TestWithdrawalIsClonedIntoASubshellAndDoesNotEscapeIt(t *testing.T) {
	out, _ := run(t, "( zzmark; echo in=$? )\nzzmark; echo out=$?", func(r *Runner) {
		r.Register("zzmark", func(_ *Runner, _ context.Context, _ []string) int { return 7 })
	})
	if !strings.Contains(out, "in=7") || !strings.Contains(out, "out=7") {
		t.Fatalf("baseline: got %q, want the builtin in both", out)
	}

	out, _ = run(t, "( zzmark; echo in=$? )\nzzmark; echo out=$?", func(r *Runner) {
		r.Register("zzmark", func(_ *Runner, _ context.Context, _ []string) int { return 7 })
		r.SetBuiltinWithdrawn("zzmark", true)
	})
	if !strings.Contains(out, "in=127") || !strings.Contains(out, "out=127") {
		t.Errorf("withdrawn before the subshell: got %q, want 127 in both", out)
	}
}
