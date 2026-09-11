// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// TestInSubshellAnswersForEveryBodyARealShellWouldFork is the seam a dialect
// asks before it hands a body a value the body will read as an identity.
//
// A real shell forks every one of these, so a parameter reporting "which
// process am I" answers differently in each. Here they are cloned Runners on
// goroutines of one process, and a dialect that answered the process's own
// number in all five let a prompt theme signal the shell's process group as
// though it were its own — see dialect/zsh's subshellPid and #2046.
//
// Asserted through a registered builtin because that is where a dialect reads
// it: the body has to be able to ask from inside itself.
func TestInSubshellAnswersForEveryBodyARealShellWouldFork(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the shell itself", "where", "false\n"},
		{"a subshell", "( where )", "true\n"},
		{"a command substitution", ": $(where)", "true\n"},
		{"a process substitution", "read line < <(where)", "true\n"},
		{"a background job", "{ where; } &\nwait", "true\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var mu sync.Mutex
			var seen []string
			r := newTestRunner(t, &interp.Runner{Stdout: io.Discard})
			// Recorded beside the shell rather than written to the body's
			// stdout, so that what a substitution would have captured is not
			// what is being asserted on. Under a lock because a background
			// job and a process substitution run on goroutines of their own.
			r.Register("where", func(rr *interp.Runner, _ context.Context, _ []string) int {
				mu.Lock()
				defer mu.Unlock()
				seen = append(seen, fmt.Sprintf("%v\n", rr.InSubshell()))
				return 0
			})
			f, err := syntax.Parse(c.src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if got := strings.Join(seen, ""); got != c.want {
				t.Errorf("InSubshell reported %q, want %q", got, c.want)
			}
		})
	}
}
