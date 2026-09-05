// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// helperScript names the script a re-executed copy of this test binary should
// run as a shell. Set, it means "you are the shell"; unset, "you are the test".
const helperScript = "SH_TEST_INHERITED_FD_SCRIPT"

// runAsShellWithADescriptor re-executes this test binary as a shell, with data
// open on descriptor 3 exactly as `sh 3<data script` opens it, and returns
// what the script produced.
//
// A second process is unavoidable and is the point. The question is what a
// shell does with a descriptor its *caller* opened, and a test cannot be its
// own caller: a file opened inside one process is close-on-exec, which is the
// property that marks it as this program's own rather than as one handed in.
// So the situation has to be built the way a caller builds it.
func runAsShellWithADescriptor(t *testing.T, name, data, src string) string {
	t.Helper()
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "data")
	if err := os.WriteFile(dataPath, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "case.sh")
	if err := os.WriteFile(script, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	cmd := exec.Command(os.Args[0], "-test.run="+name)
	cmd.Env = append(os.Environ(), helperScript+"="+script)
	// Entry 0 is descriptor 3 in the child, which is what `3<data` does.
	cmd.ExtraFiles = []*os.File{f}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the shell half failed: %v\n%s", err, out)
	}
	return string(out)
}

// beTheShell turns this process into one when it was re-executed as one, and
// does nothing otherwise. os.Exit rather than a return: this process is a
// shell now, and its status is the script's.
func beTheShell() {
	if path := os.Getenv(helperScript); path != "" {
		os.Exit(driver.MainArgs(shell(), []string{"testsh", path}))
	}
}

// The front end is what looks, so it is the front end that has to be caught
// doing it: every part of this seam can be right while nothing fills the
// field in, and a feature is not reachable until the invocation reaches it.
func TestAScriptSeesTheDescriptorsItsCallerOpened(t *testing.T) {
	beTheShell()
	out := runAsShellWithADescriptor(t, "TestAScriptSeesTheDescriptorsItsCallerOpened",
		"hello\n", "read x <&3\necho \"got:$x\"\n")
	if !strings.Contains(out, "got:hello") {
		t.Errorf("the script did not read its caller's descriptor: %q", out)
	}
}

// And closing it closes it for whatever the script runs next, which is the
// half a table rebuilt only from what the shell holds would miss. The
// descriptor is still open in this process and is not close-on-exec — that is
// how it was recognized as inherited — so unless the rebuilt table reaches
// its number with a nil, it passes to the child through the kernel, behind
// the table's back. All four shells find it closed there.
//
// What the child *reads* is the assertion, and its status deliberately is
// not: /bin/sh is bash on macOS and dash on Debian, and the two number a
// failed redirect differently — 1 and 2. That is a fact about whatever the
// machine ships rather than about this shell, and asserting it is how this
// passed locally and failed on Linux. The corpus case for the outbound half
// discards the child's complaint for the same reason.
func TestAClosedInheritedDescriptorIsClosedForAChildToo(t *testing.T) {
	beTheShell()
	out := runAsShellWithADescriptor(t, "TestAClosedInheritedDescriptorIsClosedForAChildToo",
		"hello\n", "exec 3<&-\n/bin/sh -c 'read y <&3; echo \"child:[$y]\"' 2>/dev/null\n")
	if strings.Contains(out, "hello") {
		t.Errorf("a closed descriptor still reached the child: %q", out)
	}
	if !strings.Contains(out, "child:[]") {
		t.Errorf("the child did not run, so nothing was proved: %q", out)
	}
}
