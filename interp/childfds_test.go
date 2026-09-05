// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
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

// runForReplacement runs src with the process-replacement hook installed and
// hands back the descriptor table `exec cmd` asked it to carry.
//
// The hook returns an error, which is the only thing a test can do with it: a
// real replacement never comes back, so "the image could not be replaced" is
// the one outcome that leaves a shell to make assertions in.
func runForReplacement(t *testing.T, dir, src string, coproc bool) []*os.File {
	t.Helper()
	d := syntax.Core()
	d.Coproc = coproc
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	called := false
	var got []*os.File
	r := &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: dir, Name: "testsh", Env: testPATH(),
		ReplaceProcess: func(_ string, _, _ []string, files []*os.File) error {
			called, got = true, files
			return os.ErrPermission
		},
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	if !called {
		t.Fatalf("the replacement hook was never reached: %q", buf.String())
	}
	return got
}

// A replacement is handed the table an external child is handed, because it
// inherits exactly what a child inherits: `exec 3>f; exec cmd` leaves the
// descriptor open for cmd in every shell in the panel but ksh93. The
// interpreter's half of that is the table; putting it on those numbers is the
// hook's, because a Runner may not rewrite the process's own descriptors.
func TestAReplacementIsHandedTheTableAChildIsHanded(t *testing.T) {
	dir := t.TempDir()
	five := filepath.Join(dir, "five")
	files := runForReplacement(t, dir, `exec 5>`+five+`; exec /bin/echo replaced`, false)

	// The numbers are the shell's, so the gap below five is a hole rather
	// than a packing: entry i is descriptor 3+i.
	if len(files) != 3 {
		t.Fatalf("table has %d entries, want 3 — descriptors 3, 4 and 5", len(files))
	}
	if files[0] != nil || files[1] != nil {
		t.Errorf("descriptors 3 and 4 were never opened and must be holes: %v", files[:2])
	}
	if files[2] == nil {
		t.Fatal("the descriptor the script parked on 5 did not reach the table")
	}
	// And it is the file the script parked, rather than merely something.
	if _, err := files[2].WriteString("through the table\n"); err != nil {
		t.Fatalf("the table's entry for 5 is not writable: %v", err)
	}
	if b, _ := os.ReadFile(five); string(b) != "through the table\n" {
		t.Errorf("entry for 5 wrote %q, want the file the script opened", b)
	}
}

// The coprocess exclusion holds on this side too, and bash says so on the
// harder half: with a coprocess running, a replacement finds nothing open on
// the number the shell reports in NAME[1], nor on a 3 duplicated from it,
// though the shell itself still writes through that 3. A replacement holding
// the write end open is a coprocess that never reads end-of-file, which is
// the whole reason the mark exists.
func TestACoprocessDescriptorDoesNotReachAReplacement(t *testing.T) {
	dir := t.TempDir()
	files := runForReplacement(t, dir, `coproc /bin/cat
v=${COPROC[1]}
exec 3>&$v
exec /bin/echo replaced
`, true)
	for i, f := range files {
		if f != nil {
			t.Errorf("descriptor %d reached a replacement from the coprocess's feed", firstExtraFdForTest+i)
		}
	}
}

// firstExtraFdForTest is interp's layout constant, spelled out here because
// this is an external test package: entry i of the table is descriptor 3+i.
const firstExtraFdForTest = 3
