// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// TestAnIdentityAssignmentAsksTheProcess is #5157's E03posix row `EUID is not
// a special variable` as the suite's image runs it, which is as root: there
// zsh really does become the id it is given, so a second assignment is
// refused. Measured 2026-10-02 on zsh 5.9.2 in that image, as root:
//
//	EUID=1; echo $EUID; id -u; UID=2      1, 1, failed to change user ID:
//	                                      operation not permitted, status 1
//
// The process cannot be made root here, so the hook stands in for the
// kernel: it accepts the first change and refuses the next with what the
// kernel says, and the rows check that the shell asks, and passes on the
// refusal, worded as the reference words it.
func TestAnIdentityAssignmentAsksTheProcess(t *testing.T) {
	type call struct {
		which interp.Identity
		id    int
	}
	var calls []call
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{Dir: t.TempDir(), Stdout: &buf, Stderr: &buf})
	r.ChangeIdentity = func(which interp.Identity, id int) error {
		calls = append(calls, call{which, id})
		if len(calls) > 1 {
			return syscall.EPERM
		}
		return nil
	}
	other := strconv.Itoa(os.Geteuid() + 1)
	src := "EUID=" + other + "; echo a$?\nGID=" + other + "; echo b$?\n"
	st, err := r.Run(context.Background(), preset.ParseThrough(t, r, src))
	if err != nil {
		t.Fatal(err)
	}
	if want := "a0\nzsh:2: failed to change group ID: operation not permitted\n"; buf.String() != want || st != 1 {
		t.Errorf("got %q status %d, want %q at 1", buf.String(), st, want)
	}
	n, _ := strconv.Atoi(other)
	if len(calls) != 2 || calls[0] != (call{interp.IdentityEffectiveUser, n}) || calls[1] != (call{interp.IdentityGroup, n}) {
		t.Errorf("the hook was asked %v, want EUID then GID with %d", calls, n)
	}
}

// The id the process already has is not asked about, and a subshell is not
// the process: a real shell's subshell changes only itself, which this one
// cannot do without changing the whole program.
func TestAnIdentityAssignmentLeavesTheProcessAloneWhereItMust(t *testing.T) {
	asked := 0
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{Dir: t.TempDir(), Stdout: &buf, Stderr: &buf})
	r.ChangeIdentity = func(interp.Identity, int) error {
		asked++
		return nil
	}
	src := "EUID=" + strconv.Itoa(os.Geteuid()) + "; echo a$?\n( EUID=" + strconv.Itoa(os.Geteuid()+1) + " ) 2>/dev/null; echo b\n"
	if _, err := r.Run(context.Background(), preset.ParseThrough(t, r, src)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "a0\n") || asked != 0 {
		t.Errorf("got %q with the hook asked %d times, want a0 and none", buf.String(), asked)
	}
}
