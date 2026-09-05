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
	// Kept rather than discarded, which is the difference between a failure
	// that can be read and one that cannot. This said `2>/dev/null` for a
	// year, and when it failed on CI the only evidence left was an empty
	// file — while the reason, a fatal runtime error from the shell half,
	// had been written to the descriptor and thrown away. See #695: it took
	// three reports and a container to recover a sentence the test had had
	// in its hands every time.
	complaint := filepath.Join(dir, "complaint")
	out := runAsShellReplacingItself(t, "TestAReplacementsDescriptorNumbersAreTheShellsWithGapsLeftClosed",
		"exec 5>"+five+"\nexec /bin/sh -c 'echo onfive >&5; echo onthree >&3' 2>"+complaint+"\n")
	b, _ := os.ReadFile(five)
	if !strings.Contains(string(b), "onfive") {
		t.Errorf("five = %q, want the replacement's line%s", b, saidWhat(t, out, complaint))
	}
	if strings.Contains(string(b), "onthree") {
		t.Errorf("descriptor 3 in the replacement reached the file parked on 5: %q", b)
	}
}

// saidWhat is everything the shell half and the replacement had to say, for a
// failure message to carry.
//
// A descriptor that did not arrive leaves an empty file and nothing else, and
// the two places an explanation could be — what the shell half printed before
// it was replaced, and what the replacement printed when the number was not
// what it expected — are both routinely redirected away by the very scripts
// these tests run. Reading them back costs nothing and is the difference
// between diagnosing the next occurrence and re-investigating it.
func saidWhat(t *testing.T, out, complaint string) string {
	t.Helper()
	said := ""
	if b, err := os.ReadFile(complaint); err == nil && len(b) > 0 {
		said += "\nthe replacement said: " + string(b)
	}
	if out != "" {
		said += "\nthe shell half said: " + out
	}
	if said == "" {
		said = "\nand neither the shell half nor the replacement said anything"
	}
	return said
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

	out := runAsShellReplacingItself(t, "TestDescriptorsCrossAReplacementWhateverOrderTheyWereOpenedIn", src)
	for _, name := range names {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		if strings.TrimSpace(string(b)) != name {
			t.Errorf("%s = %q, want its own line — the descriptors crossed over each other%s",
				name, b, saidWhat(t, out, ""))
		}
	}
}

// A redirection the script has already applied is the replacement's too.
//
// This is the half of the table that has no second route across. A descriptor
// above 2 is placed because Go opened it close-on-exec; standard output is
// placed because after `exec >log` the file is on whatever number Go had free,
// and the process's own 1 is still the caller's — so the replacement wrote
// there, past a redirection the script had already made, in every form of it.
func TestARedirectedStandardOutputReachesAReplacement(t *testing.T) {
	beTheShell()
	dir := t.TempDir()
	target := filepath.Join(dir, "log")
	out := runAsShellReplacingItself(t, "TestARedirectedStandardOutputReachesAReplacement",
		"exec >"+target+"\nexec /bin/echo hi\n")
	if strings.Contains(out, "hi") {
		t.Errorf("the replacement wrote to the shell's caller: %q", out)
	}
	if b, _ := os.ReadFile(target); strings.TrimSpace(string(b)) != "hi" {
		t.Errorf("file = %q, want the replacement's line", b)
	}
}

// The other form of the same thing, and the one a script is likelier to write:
// the redirection is on the `exec` itself rather than on an `exec` before it.
func TestAReplacementsOwnRedirectionOfStandardOutputCrosses(t *testing.T) {
	beTheShell()
	dir := t.TempDir()
	target := filepath.Join(dir, "log")
	out := runAsShellReplacingItself(t, "TestAReplacementsOwnRedirectionOfStandardOutputCrosses",
		"exec /bin/echo hi >"+target+"\n")
	if strings.Contains(out, "hi") {
		t.Errorf("the replacement wrote to the shell's caller: %q", out)
	}
	if b, _ := os.ReadFile(target); strings.TrimSpace(string(b)) != "hi" {
		t.Errorf("file = %q, want the replacement's line", b)
	}
}

// Standard error is placed for the same reason and is worth its own case,
// because a shell that reported the failure to place anything would report it
// there and could not be trusted to say so.
func TestARedirectedStandardErrorReachesAReplacement(t *testing.T) {
	beTheShell()
	dir := t.TempDir()
	target := filepath.Join(dir, "err")
	runAsShellReplacingItself(t, "TestARedirectedStandardErrorReachesAReplacement",
		"exec 2>"+target+"\nexec /bin/sh -c 'echo complaint >&2'\n")
	if b, _ := os.ReadFile(target); strings.TrimSpace(string(b)) != "complaint" {
		t.Errorf("file = %q, want the replacement's line", b)
	}
}

// The reading half. `exec <data; exec /bin/cat` read the *caller's* standard
// input, which with a terminal there is a shell that hangs rather than one
// that prints the file.
func TestARedirectedStandardInputReachesAReplacement(t *testing.T) {
	beTheShell()
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.WriteFile(data, []byte("fromthefile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := runAsShellReplacingItself(t, "TestARedirectedStandardInputReachesAReplacement",
		"exec <"+data+"\nexec /bin/cat\n")
	if !strings.Contains(out, "fromthefile") {
		t.Errorf("the replacement read %q, want the file the script opened", out)
	}
}

// And a named stream the script *closed* is closed there, which is what makes
// a nil one rule rather than two. Every shell in the panel leaves the command
// a closed descriptor here, and it complains rather than writing anywhere.
//
// What the command reported is the assertion and not what it wrote: with
// standard output closed there is nowhere for it to write, which is the point.
func TestAClosedStandardStreamIsClosedForAReplacement(t *testing.T) {
	beTheShell()
	dir := t.TempDir()
	report := filepath.Join(dir, "report")
	runAsShellReplacingItself(t, "TestAClosedStandardStreamIsClosedForAReplacement",
		"exec >&-\nexec /bin/sh -c '/bin/echo hi; echo st=$? >"+report+"'\n")
	b, _ := os.ReadFile(report)
	if !strings.Contains(string(b), "st=") {
		t.Fatalf("the replacement did not run, so nothing was proved: %q", b)
	}
	if strings.Contains(string(b), "st=0") {
		t.Errorf("a closed standard output was open in the replacement: %q", b)
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
