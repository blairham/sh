// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/blairham/sh/interp"
)

// `kill` aimed at a background program reaches it whatever this shell has
// ignored, because the job is that program and not a body of the shell's.
//
// The job here is a body on a goroutine whose trap table starts with the
// shell's ignores, and the body stood in for the program as long as that table
// was not empty — so `kill -USR1 $!` found the ignore in it, sent nothing and
// returned 0, and the program, which had put USR1 back to its default, ran on.
// A real fork execs its last command and the ignore goes with it as a
// disposition, which needs nobody to answer it. Measured 2026-11-01, each
// shell in a process group of its own: bash 5.3.20, bash 3.2.57, ksh93u+,
// dash and zsh 5.9.2 kill the program in every row below that says so, and
// leave the program that kept the ignore running (#7248).
//
// The program is perl, because it is the one at hand that can put back a
// signal it was handed ignored: a shell started without a terminal may not.
func TestKillReachesAProgramWhateverTheShellIgnores(t *testing.T) {
	const perl = "/usr/bin/perl"
	if _, err := os.Stat(perl); err != nil {
		t.Skipf("no %s to put a signal back to its default: %v", perl, err)
	}
	restoreDispositionsAfter(t, []string{"USR1", "TERM"})
	usr1, term := 128+int(syscall.SIGUSR1), 128+int(syscall.SIGTERM)
	// The program says when it has done what it does with the signal, so the
	// kill is never sent before the reset it is about.
	program := func(reset, nap string) string {
		return perl + ` -e '` + reset + `open F, ">ready"; close F; ` +
			`select undef, undef, undef, ` + nap + `; print "woke\n"' &` + "\n" +
			"until [ -e ready ]; do /bin/sleep 0.01; done\n"
	}
	for _, tc := range []struct {
		name, src string
		woke      bool
		status    int
		// linger outlasts the program's nap, for a row where the program
		// would be left running past the wait rather than waited for.
		linger bool
	}{
		{
			"reset to the default, USR1 by pid",
			"trap '' USR1\n" + program(`$SIG{USR1}="DEFAULT"; `, "5") + "kill -USR1 $!",
			false, usr1, false,
		},
		{
			"reset to the default, TERM by pid",
			"trap '' TERM\n" + program(`$SIG{TERM}="DEFAULT"; `, "5") + "kill -TERM $!",
			false, term, false,
		},
		{
			"reset to the default, USR1 by job",
			"trap '' USR1\n" + program(`$SIG{USR1}="DEFAULT"; `, "5") + "kill -USR1 %1",
			false, usr1, false,
		},
		// The control: the ignore goes to the program, and is still there.
		{
			"kept the ignore, USR1 by pid",
			"trap '' USR1\n" + program("", "0.5") + "kill -USR1 $!",
			true, 0, false,
		},
		// And a signal nobody ignores ends the program rather than a body
		// standing in front of it, which used to leave it running on its own.
		{
			"kept the ignore, TERM by pid",
			"trap '' USR1\n" + program("", "0.5") + "kill -TERM $!",
			false, term, true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src + "\necho k=$?\nwait $!\necho st=$?\n"
			if tc.linger {
				src += "/bin/sleep 1\n"
			}
			src += "echo done\n"
			out, _ := runUnderBashWith(t, src, func(r *interp.Runner) {
				ownsTheProcess(r)
				// Aimed at the process, as every column but one aims a job
				// spec; the one that aims at the group is its own question.
				r.Semantics.KillJobSpecAimsAtTheGroup = interp.No
			})
			if !strings.Contains(out, "done") {
				t.Fatalf("never ran to its end: %q", out)
			}
			if !strings.Contains(out, "k=0\n") {
				t.Errorf("kill did not succeed: %q", out)
			}
			if got := strings.Contains(out, "woke"); got != tc.woke {
				t.Errorf("the program ran on = %v, want %v: %q", got, tc.woke, out)
			}
			if want := "st=" + strconv.Itoa(tc.status) + "\n"; !strings.Contains(out, want) {
				t.Errorf("want %q: %q", want, out)
			}
		})
	}
}
