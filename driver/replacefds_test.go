// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// runAsShellReplacingItself re-executes this test binary as a shell and lets
// the script inside it replace that process with something else.
//
// A second process is the whole point and cannot be avoided. `exec cmd` here
// is a real execve: the test binary stops being a test, so the shell has to be
// somewhere other than the process asserting on it. What comes back is
// whatever the *replacement* wrote, which is what a caller of a shell sees.
func runAsShellReplacingItself(t *testing.T, name, src string) string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "case.sh")
	if err := os.WriteFile(script, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run="+name)
	cmd.Env = append(os.Environ(), helperScript+"="+script)
	// Deliberately no ExtraFiles: descriptor 3 is unopened here, so what the
	// replacement finds on it is the script's doing and nothing else's.
	out, err := cmd.CombinedOutput()
	// A status is deliberately not asserted, and not even tolerated silently
	// by accident: what comes back is the *replacement's* status, and a
	// replacement that could not write to a descriptor reports 1 in bash and
	// 2 in dash — /bin/sh being one on macOS and the other on Debian. That is
	// a fact about the machine, so every assertion here is about what was
	// written instead.
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatalf("the shell half could not be started: %v\n%s", err, out)
	}
	return string(out)
}

// The descriptor a script parks is parked *for* whatever runs next, and `exec
// cmd` is the sharpest case of that: the command is not a child, it is this
// process. Go opens everything close-on-exec and a Runner's descriptor 3 is
// not the process's 3, so this wrote nothing and the replacement said "Bad
// file descriptor" where bash, dash and zsh all write.
func TestAParkedDescriptorSurvivesAProcessReplacement(t *testing.T) {
	beTheShell()
	dir := t.TempDir()
	target := filepath.Join(dir, "parked")
	runAsShellReplacingItself(t, "TestAParkedDescriptorSurvivesAProcessReplacement",
		"exec 3>"+target+"\nexec /bin/sh -c 'echo replacement >&3'\n")
	if b, _ := os.ReadFile(target); string(b) != "replacement\n" {
		t.Errorf("file = %q, want the replacement's line", b)
	}
}

// The replacement's numbers are the shell's numbers, holes included. With 3
// and 4 never opened, the file parked on 5 is on 5 and 3 is closed — so the
// table is placed by number rather than packed down.
//
// What the replacement *wrote* is the assertion and its status is not: a
// failed redirect is status 1 in bash and 2 in dash, and /bin/sh is bash on
// macOS and dash on Debian, so the number is a fact about the machine.
func TestAReplacementsDescriptorNumbersAreTheShellsWithGapsLeftClosed(t *testing.T) {
	beTheShell()
	dir := t.TempDir()
	five := filepath.Join(dir, "five")
	runAsShellReplacingItself(t, "TestAReplacementsDescriptorNumbersAreTheShellsWithGapsLeftClosed",
		"exec 5>"+five+"\nexec /bin/sh -c 'echo onfive >&5; echo onthree >&3' 2>/dev/null\n")
	b, _ := os.ReadFile(five)
	if !strings.Contains(string(b), "onfive") {
		t.Errorf("five = %q, want the replacement's line", b)
	}
	if strings.Contains(string(b), "onthree") {
		t.Errorf("descriptor 3 in the replacement reached the file parked on 5: %q", b)
	}
}

// Placing the table cannot be a walk from three upward, because the shell's
// own numbering need not agree with the script's: the file the script calls 7
// may be sitting on the process's 3, and writing 3 first would destroy it.
// Opening them in descending order is what puts the two orders furthest
// apart, and it failed exactly that way before the sources were moved clear.
func TestDescriptorsCrossAReplacementWhateverOrderTheyWereOpenedIn(t *testing.T) {
	beTheShell()
	dir := t.TempDir()
	names := []string{"three", "four", "five", "six", "seven"}
	src := "exec"
	for i := len(names) - 1; i >= 0; i-- {
		src += " " + strconv.Itoa(3+i) + ">" + filepath.Join(dir, names[i])
	}
	src += "\nexec /bin/sh -c '"
	for i, name := range names {
		src += "echo " + name + " >&" + strconv.Itoa(3+i) + "; "
	}
	src += "true'\n"

	runAsShellReplacingItself(t, "TestDescriptorsCrossAReplacementWhateverOrderTheyWereOpenedIn", src)
	for _, name := range names {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		if strings.TrimSpace(string(b)) != name {
			t.Errorf("%s = %q, want its own line — the descriptors crossed over each other", name, b)
		}
	}
}

// And a descriptor the *caller* opened and the script closed is closed for the
// replacement, which is the half that has nothing to do with the flag.
//
// The original is still open in this process and is not close-on-exec — that
// is how it was recognized as inherited — so it passes to the replacement
// through the kernel, behind the table's back, unless the number is reached
// and closed. Every shell in the panel finds it closed there, ksh93 included.
func TestAClosedInheritedDescriptorIsClosedForAReplacementToo(t *testing.T) {
	beTheShell()
	out := runAsShellWithADescriptor(t, "TestAClosedInheritedDescriptorIsClosedForAReplacementToo",
		"hello\n", "exec 3<&-\nexec /bin/sh -c 'read y <&3; echo \"replacement:[$y]\"' 2>/dev/null\n")
	if strings.Contains(out, "hello") {
		t.Errorf("a closed descriptor still reached the replacement: %q", out)
	}
	if !strings.Contains(out, "replacement:[]") {
		t.Errorf("the replacement did not run, so nothing was proved: %q", out)
	}
}

// And one it left open is still open there, which is what makes the closing
// half a rule about the table rather than a habit of closing things.
func TestAnInheritedDescriptorReachesAReplacement(t *testing.T) {
	beTheShell()
	out := runAsShellWithADescriptor(t, "TestAnInheritedDescriptorReachesAReplacement",
		"hello\n", "exec /bin/sh -c 'read y <&3; echo \"replacement:[$y]\"'\n")
	if !strings.Contains(out, "replacement:[hello]") {
		t.Errorf("the replacement did not read the caller's descriptor: %q", out)
	}
}
