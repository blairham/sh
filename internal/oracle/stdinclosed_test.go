// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// What Case.StdinClosed has to deliver, checked against the thing itself
// rather than against the field being set.
//
// The corpus rows are the other guard and the stronger one -- a mechanism
// that quietly went back to the null device would move bash 5.3's cell from
// status 126 to status 0 and `make oracle -check` would call it drift -- but
// they only speak on a machine that has the panel. This runs anywhere.

func TestAClosedStandardInputIsNotOpenInTheChild(t *testing.T) {
	// `exec 3<&0` is the discriminating probe: it fails only when fd 0 is
	// absent. Reading from fd 0 is not, because the null device reads as end
	// of file and so does nothing at all.
	//
	// It is run **in a subshell** because /bin/sh is not one shell. Where it
	// is dash a failed `exec` redirection is fatal, so the shell is gone
	// before any branch of an `if` runs and the test read the exit status of
	// a shell that had died rather than a marker — green on macOS, where
	// /bin/sh is bash and carries on, and red on Linux. The subshell turns
	// both behaviors into the one thing both agree on, which is its status.
	//
	// The marker is read off the last line for the same reason: a shell that
	// finds the descriptor missing says so on standard error, in its own
	// words, and that text is not the answer.
	const probe = `if ( exec 3<&0 ) 2>/dev/null; then echo OPEN; else echo CLOSED; fi`

	for _, c := range []struct {
		name   string
		closed bool
		want   string
	}{
		{"the default is the null device", false, "OPEN"},
		{"StdinClosed leaves no descriptor", true, "CLOSED"},
	} {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command("/bin/sh", "-c", probe)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			if c.closed {
				cmd.Stdin = closedDescriptor()
			}
			if err := cmd.Run(); err != nil {
				t.Fatalf("running the probe: %v (output %q)", err, out.String())
			}
			lines := strings.Fields(out.String())
			if len(lines) == 0 {
				t.Fatalf("the probe said nothing")
			}
			if got := lines[len(lines)-1]; got != c.want {
				t.Errorf("the child saw fd 0 %s, want %s (output %q)",
					got, c.want, out.String())
			}
		})
	}
}

// The file has to report an invalid descriptor, which is the whole mechanism:
// os/exec passes what Fd reports, and Go's fork-and-exec reads -1 as "close
// this one in the child".
func TestTheClosedDescriptorReportsAnInvalidFd(t *testing.T) {
	f := closedDescriptor()
	if f == nil {
		t.Fatal("closedDescriptor gave nothing; the null device would not open")
	}
	if got := f.Fd(); got != ^uintptr(0) {
		t.Errorf("Fd = %v, want the invalid descriptor %v", got, ^uintptr(0))
	}
}

// A case may say what is on standard input or that there is none, and not
// both: they are opposite requests and a silent precedence rule is exactly
// what the Args/Script pair already refuses.
func TestStdinAndStdinClosedAreExclusive(t *testing.T) {
	err := Case{Snippet: "echo hi", Stdin: "data\n", StdinClosed: true}.validate()
	if err == nil {
		t.Fatal("a case asking for both was accepted")
	}
	if !strings.Contains(err.Error(), "exclusive") {
		t.Errorf("error = %q, want it to say the two are exclusive", err)
	}
	if err := (Case{Snippet: "echo hi", StdinClosed: true}).validate(); err != nil {
		t.Errorf("StdinClosed alone was refused: %v", err)
	}
}

// And the builder honors it, which is what carries the field from the corpus
// to the child on both sides of a comparison.
func TestCommandGivesAClosedCaseTheClosedDescriptor(t *testing.T) {
	sh := Found{Shell: Shell{Name: "ours"}, Path: "/bin/sh"}
	cmd := command(t.Context(), sh, Case{Snippet: "echo hi", StdinClosed: true}, t.TempDir())
	f, ok := cmd.Stdin.(*os.File)
	if !ok {
		t.Fatalf("Stdin is %T, want the closed *os.File", cmd.Stdin)
	}
	if got := f.Fd(); got != ^uintptr(0) {
		t.Errorf("Fd = %v, want the invalid descriptor", got)
	}
}
