// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A builtin writing past `ulimit -f` is answered by SIGXFSZ's disposition,
// as a builtin writing into a broken pipe is answered by SIGPIPE's: left
// alone the signal ends the shell where it stands, and ignored or handled it
// leaves an ordinary failed write (#7057). See filesizedeath.go for the
// panel.

// fileTooLarge is a stream past the file-size limit: every write fails the
// way the kernel fails one past `ulimit -f`.
type fileTooLarge struct{}

func (fileTooLarge) Write([]byte) (int, error) { return 0, syscall.EFBIG }

// xfszDeathStatus is what a command killed by SIGXFSZ reports, in the
// encoding fileSizeSem asks for.
var xfszDeathStatus = 128 + int(syscall.SIGXFSZ)

// fileSizeSem is the EXIT-trap death vector with a failed write failing the
// command, and the EXIT trap run on a death or not as runsTheExitTrap says.
func fileSizeSem(runsTheExitTrap Answer) Semantics {
	s := exitTrapDeathSem(No, No)
	s.BuiltinWriteErrorFailsTheCommand = Yes
	s.ExitTrapRunsOnSignalDeath = runsTheExitTrap
	// The majority's answer: a caught write past the limit is still a death.
	// TestACaughtWritePastTheLimitIsGivenUpOverWhereTheAxisSaysSo has the
	// other.
	s.CaughtWritePastTheFileSizeLimitIsAFatalError = No
	return s
}

// pastTheLimit runs src with its standard output past the file-size limit,
// and with a DieBySignal that records the signal rather than ending this
// process.
func pastTheLimit(t *testing.T, src string, sem Semantics) leaving {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var e bytes.Buffer
	var got leaving
	asked := 0
	r := newTestRunner(t, &Runner{
		Stdout: fileTooLarge{}, Stderr: &e, Semantics: &sem, Name: "testsh",
		Diagnostics: &Diagnostics{BuiltinWriteError: "%[1]s: write error: %[2]s"},
		DieBySignal: func(s syscall.Signal) error {
			got.died, asked = s, asked+1
			return nil
		},
	})
	r.ListenForFatalSignals = func() {}
	got.status, err = r.Run(context.Background(), f)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	if asked > 1 {
		t.Errorf("asked to die %d times", asked)
	}
	got.errs = e.String()
	return got
}

func TestAWritePastTheFileSizeLimitEndsTheShellWhereItStands(t *testing.T) {
	for _, src := range []string{
		`echo hi; echo after >&2`,
		`printf '%s\n' hi; echo after >&2`,
		`f() { echo hi; echo in >&2; }; f; echo after >&2`,
	} {
		got := pastTheLimit(t, src, fileSizeSem(No))
		want := leaving{status: xfszDeathStatus, died: syscall.SIGXFSZ}
		if got != want {
			t.Errorf("%s: ended %+v, want %+v — killed where it stands, saying nothing", src, got, want)
		}
	}
}

// The three arrangements over one shape, so the difference between dying and
// carrying on is the trap and nothing else.
func TestOnlyTheDefaultActionOfAWritePastTheLimitEndsTheShell(t *testing.T) {
	for _, c := range []struct {
		name, setup string
		dies        bool
	}{
		{"nothing arranged", ``, true},
		{"ignored", `trap '' XFSZ; `, false},
		{"handled", `trap ':' XFSZ; `, false},
	} {
		got := pastTheLimit(t, c.setup+`echo hi; echo "st=$?" >&2`, fileSizeSem(No))
		if died := got.died == syscall.SIGXFSZ; died != c.dies {
			t.Errorf("%s: ended %+v, died = %v, want %v", c.name, got, died, c.dies)
		}
		if !c.dies && !strings.Contains(got.errs, "echo: write error:") ||
			!c.dies && !strings.Contains(got.errs, "st=1") {
			t.Errorf("%s: stderr = %q, want the ordinary failed write and its status", c.name, got.errs)
		}
	}
}

// A handler runs between commands, after the builtin's complaint, and once:
// the shell answers the signal at the write rather than waiting for the
// runtime's copy of it.
func TestAHandledWritePastTheLimitRunsTheHandlerAfterTheComplaint(t *testing.T) {
	got := pastTheLimit(t, `trap 'echo handled >&2' XFSZ; echo hi; echo after >&2`, fileSizeSem(No))
	if got.died != 0 {
		t.Errorf("died of %v, want the shell to carry on", got.died)
	}
	complaint, handled, after := strings.Index(got.errs, "echo: write error:"),
		strings.Index(got.errs, "handled"), strings.Index(got.errs, "after")
	if complaint < 0 || handled < complaint || after < handled {
		t.Errorf("stderr = %q, want the complaint, the handler and the next command in that order", got.errs)
	}
	if n := strings.Count(got.errs, "handled"); n != 1 {
		t.Errorf("stderr = %q, handler ran %d times, want once", got.errs, n)
	}
}

// A subshell's write is the subshell's death and not the shell's, which reads
// it as a child killed by the signal — and a handler the shell set is not the
// subshell's, so it neither saves the subshell nor runs for it.
func TestASubshellsWritePastTheLimitEndsTheSubshell(t *testing.T) {
	st := "st=" + strconv.Itoa(xfszDeathStatus)
	for _, src := range []string{
		`(echo hi; echo in >&2); echo "st=$?" >&2`,
		`trap 'echo outer >&2' XFSZ; (echo hi; echo in >&2); echo "st=$?" >&2`,
	} {
		got := pastTheLimit(t, src, fileSizeSem(No))
		if got.died != 0 || !strings.Contains(got.errs, st) ||
			strings.Contains(got.errs, "in\n") || strings.Contains(got.errs, "outer") {
			t.Errorf("%s: ended %+v, want the subshell killed, %s, and nothing else run", src, got, st)
		}
	}
}

// The EXIT trap runs on the death in the dialects that run one on a death, and
// reads the status from before the builtin that died: `t 5` in bash 5.3 and
// ksh93, measured 2026-10-31; nothing in dash and zsh.
func TestTheExitTrapOfAWritePastTheLimitReadsTheStatusBeforeTheBuiltin(t *testing.T) {
	const src = `trap 'echo "t $?" >&2' EXIT; (exit 5); echo hi; echo after >&2`
	for _, c := range []struct {
		runs Answer
		errs string
	}{
		{Yes, "t 5\n"},
		{No, ""},
	} {
		got := pastTheLimit(t, src, fileSizeSem(c.runs))
		want := leaving{errs: c.errs, status: xfszDeathStatus, died: syscall.SIGXFSZ}
		if got != want {
			t.Errorf("ExitTrapRunsOnSignalDeath %v: ended %+v, want %+v", c.runs, got, want)
		}
	}
}

// Once an EXIT trap has caught the signal, one dialect gives up over the
// write rather than dying of it: the builtin's complaint, then the trap
// reading the status from before the builtin, then the shell leaving at 1
// (#7173). See Semantics.CaughtWritePastTheFileSizeLimitIsAFatalError.
func TestACaughtWritePastTheLimitIsGivenUpOverWhereTheAxisSaysSo(t *testing.T) {
	const complaint = "testsh: echo: write error: File too large\n"
	for _, c := range []struct {
		name, src string
		yes, no   leaving
	}{
		{
			name: "the trap reads the status before the builtin",
			src:  `trap 'echo "t $?" >&2' EXIT; (exit 5); echo hi; echo after >&2`,
			yes:  leaving{errs: complaint + "t 5\n", status: 1},
			no:   leaving{errs: "t 5\n", status: xfszDeathStatus, died: syscall.SIGXFSZ},
		},
		{
			name: "an exit in the trap names the status",
			src:  `trap 'echo "t $?" >&2; exit 6' EXIT; (exit 5); echo hi; echo after >&2`,
			yes:  leaving{errs: complaint + "t 5\n", status: 6},
			no:   leaving{errs: "t 5\n", status: 6, died: syscall.SIGXFSZ},
		},
		{
			name: "the trap's own last status does not",
			src:  `trap 'echo "t $?" >&2; (exit 4)' EXIT; (exit 5); echo hi; echo after >&2`,
			yes:  leaving{errs: complaint + "t 5\n", status: 1},
			no:   leaving{errs: "t 5\n", status: xfszDeathStatus, died: syscall.SIGXFSZ},
		},
		{
			name: "caught by a trap since taken away",
			src:  `trap 'echo "t $?" >&2' EXIT; trap - EXIT; echo hi; echo after >&2`,
			yes:  leaving{errs: complaint, status: 1},
			no:   leaving{status: xfszDeathStatus, died: syscall.SIGXFSZ},
		},
		{
			// The route and not the signal: the same signal sent with
			// `kill` is a death under either answer, its trap reading what
			// `kill` left in this vector.
			name: "kill is still a death",
			src:  `trap 'echo "t $?" >&2' EXIT; (exit 5); kill -XFSZ $$; echo after >&2`,
			yes:  leaving{errs: "t 0\n", status: xfszDeathStatus, died: syscall.SIGXFSZ},
			no:   leaving{errs: "t 0\n", status: xfszDeathStatus, died: syscall.SIGXFSZ},
		},
		{
			// And a subshell keeps its write its own death, the axis not
			// reaching it.
			name: "a subshell's write",
			src:  `trap 'echo "t $?" >&2' EXIT; (echo hi; echo in >&2); echo "st=$?" >&2`,
			yes:  leaving{errs: "st=" + strconv.Itoa(xfszDeathStatus) + "\nt 0\n"},
			no:   leaving{errs: "st=" + strconv.Itoa(xfszDeathStatus) + "\nt 0\n"},
		},
	} {
		for _, answer := range []Answer{Yes, No} {
			sem := fileSizeSem(Yes)
			sem.CaughtWritePastTheFileSizeLimitIsAFatalError = answer
			// What the dialect that answers yes holds beside it: a give-up's
			// trap reads the status from before the command, and a killed
			// subshell is not announced.
			sem.ExitTrapAfterAGiveUpSeesEarlierStatus = Yes
			sem.ReportsACommandKilledBySignal = No
			want := c.no
			if answer == Yes {
				want = c.yes
			}
			if got := pastTheLimit(t, c.src, sem); got != want {
				t.Errorf("%s, answered %v: ended %+v, want %+v", c.name, answer, got, want)
			}
		}
	}
}
