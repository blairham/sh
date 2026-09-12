// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp_test

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// The coprocess's near ends are parked on 10 and 11 by
// Semantics.FirstAllocatedDescriptor, so this is the number the shell's table
// holds and does not hand over.
const coprocFeedFd = 11

// TestACoprocessNumberIsClosedInAChildEvenWhenTheProcessHasOneThere is #1917's
// mechanism, and it is a hole rather than a race.
//
// TestACoprocessDescriptorDoesNotReachAnExternalChild asserts that the feed
// does not reach a child, and it passed for the wrong reason: `childFiles`
// measured how far the table reached from the entries it was *handing over*,
// and a coprocess's ends are entries it is deliberately not handing over. With
// nothing else in the table, the reach came out as zero, os/exec was given no
// ExtraFiles at all, and **nothing was closed in the child** — so `echo leaked
// >&11` there failed with `Bad file descriptor` only because raw 11 in this
// process is usually shut. Put something inheritable on it and the child
// writes straight through: `child=0`, and the bytes come out of the pipe.
//
// That is exactly the failure recorded on the macOS runner — `child=0` with
// `leaked` in the output — and it is why the same commit passed on a re-run
// and on ubuntu. Nothing about the shell changed between those runs; what
// changed was whether the process had an inheritable descriptor sitting on the
// number the coprocess had claimed.
//
// So the fix is the reach and not a flag: every number the table knows about
// is now covered, whatever the entry turns out to be worth, and below the
// reach a nil is a close. This test is the mutation that tells the two apart —
// it fails on the old reading and passes on the new one — and it is the only
// shape that can, because a probe that does not put something on the number is
// asking a question the answer to which was always "nothing was there".
func TestACoprocessNumberIsClosedInAChildEvenWhenTheProcessHasOneThere(t *testing.T) {
	// Never clobber a descriptor the runtime is using. dup2 closes whatever
	// is on the target, so the one safe way to take a fixed number is to
	// check first and decline if it is busy — a skip says the probe could not
	// be set up, where a silent pass would say the property holds.
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, coprocFeedFd, syscall.F_GETFD, 0); errno == 0 {
		t.Skipf("descriptor %d is already in use in this process", coprocFeedFd)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	// dup2 clears close-on-exec on the target, which is the whole point: this
	// is a descriptor the kernel would hand to any child that is not told
	// otherwise, and being told otherwise is what the shell owes it.
	if err := syscall.Dup2(int(w.Fd()), coprocFeedFd); err != nil {
		t.Fatalf("dup2 onto %d: %v", coprocFeedFd, err)
	}
	t.Cleanup(func() { _ = syscall.Close(coprocFeedFd) })

	out := runCoproc(t, `coproc /bin/cat
v=${COPROC[1]}
echo "feed=$v"
/bin/sh -c 'echo leaked >&'"$v"
echo "child=$?"
exec {COPROC[1]}>&-
wait "$COPROC_PID"
`)
	if !strings.Contains(out, "feed="+strconv.Itoa(coprocFeedFd)) {
		t.Skipf("the coprocess did not land on %d: %q", coprocFeedFd, out)
	}
	if strings.Contains(out, "child=0") {
		t.Errorf("the coprocess feed reached an external child: %q", out)
	}
	// And the descriptor itself, which is the half the status cannot report:
	// a child that wrote and then failed for some other reason would still
	// have written. Read raw, because the file has been handed to the kernel
	// by number and the runtime poller is not what is being asked.
	rfd := int(r.Fd())
	if err := syscall.SetNonblock(rfd, true); err != nil {
		t.Fatalf("SetNonblock: %v", err)
	}
	buf := make([]byte, 64)
	n, rerr := syscall.Read(rfd, buf)
	if n > 0 {
		t.Errorf("%q came through descriptor %d; the child had it open", buf[:n], coprocFeedFd)
	}
	// Nothing to read and both writers still open is EAGAIN, which is the
	// answer this wants. End of file would mean the writers had gone and the
	// read proved nothing; the test holds one itself, so it cannot happen.
	if n == 0 && rerr == nil {
		t.Errorf("descriptor %d reported end of file; this test still holds a writer, so the read proved nothing", coprocFeedFd)
	}
}
