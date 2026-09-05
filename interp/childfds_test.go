// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A descriptor the script parked with `exec 3>f` is the script's to hand out,
// and handing it to a child is the whole of the flock and shared-log idioms.
// Go opens everything close-on-exec, so the boundary a real shell gets from
// fork and exec has to be rebuilt: without that this wrote nothing at all and
// the child said "Bad file descriptor".
func TestAParkedDescriptorReachesAnExternalChild(t *testing.T) {
	f := filepath.Join(t.TempDir(), "f")
	_, errOut, st := runSplit(t,
		`exec 3>`+f+`; /bin/sh -c 'echo child >&3'; exec 3>&-`)
	if st != 0 {
		t.Errorf("status = %d, stderr %q", st, errOut)
	}
	if b, _ := os.ReadFile(f); string(b) != "child\n" {
		t.Errorf("file = %q, want the child's line; stderr %q", b, errOut)
	}
}

// The child's table is the shell's table by number, not a packing of it.
//
// With 3 and 4 never opened, the file parked on 5 is on 5 in the child and 3
// is closed there — so the rebuilt table has to keep the holes rather than
// shift everything down to fill them.
func TestTheChildsDescriptorNumbersAreTheShellsWithGapsLeftClosed(t *testing.T) {
	dir := t.TempDir()
	five := filepath.Join(dir, "five")

	_, errOut, st := runSplit(t,
		`exec 5>`+five+`; /bin/sh -c 'echo onfive >&5'; exec 5>&-`)
	if st != 0 {
		t.Errorf("status = %d, stderr %q", st, errOut)
	}
	if b, _ := os.ReadFile(five); string(b) != "onfive\n" {
		t.Errorf("five = %q, want the child's line; stderr %q", b, errOut)
	}

	// The same shell state, and the gap below it is a hole rather than the
	// file: a child writing to 3 must fail.
	out, _, _ := runSplit(t,
		`exec 5>`+five+`; /bin/sh -c 'echo onthree >&3'; echo "st=$?"; exec 5>&-`)
	if strings.Contains(out, "st=0") {
		t.Errorf("a write to an unopened descriptor should fail in the child, got %q", out)
	}
	if b, _ := os.ReadFile(five); strings.Contains(string(b), "onthree") {
		t.Errorf("descriptor 3 in the child reached the file parked on 5: %q", b)
	}
}

// The coprocess's own ends are in the table so that `>&${NAME[0]}` can find
// them, and they are still not the script's to hand out. A child holding the
// write end open is a coprocess that never reads end-of-file, which is the
// same leak that ruled /dev/fd out for process substitution.
func TestACoprocessDescriptorDoesNotReachAnExternalChild(t *testing.T) {
	// runCoproc rather than runSplit: the clause needs the grammar flag that
	// admits it, which the core parser does not set.
	out := runCoproc(t, `coproc /bin/cat
v=${COPROC[1]}
/bin/sh -c 'echo leaked >&'"$v"
echo "child=$?"
exec {v}>&-
wait "$COPROC_PID"
`)
	if !strings.Contains(out, "child=") {
		t.Fatalf("the probe did not run: %q", out)
	}
	if strings.Contains(out, "child=0") {
		t.Errorf("the coprocess feed reached an external child: %q", out)
	}
}
