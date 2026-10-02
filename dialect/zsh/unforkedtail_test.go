// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"sync"
	"testing"

	"github.com/blairham/sh/driver"
)

// lockedOutput is one buffer for both streams, safe for the copying
// goroutines a child's streams are fed through: the rows below interleave a
// builtin's complaint with another's output, so the two must land in order in
// one place.
type lockedOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *lockedOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}

func (o *lockedOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

// runZshC runs src as `zsh -f -c`, both streams into one.
func runZshC(src string) string {
	var out lockedOutput
	sh := zshShell()
	sh.Stdout, sh.Stderr = &out, &out
	driver.MainArgs(sh, []string{"zsh", "-f", "-c", src})
	return out.String()
}

// **The last command of a `-c` string is not forked, so a `( … )` there moves
// its markers** (#5320). Measured 2026-10-02 on zsh 5.9.2 under `-f -c`, byte
// for byte; each row's control is the same text with `; :` after it, which is
// forked and keeps the frozen markers. See interp/unforkedtail.go.
func TestTheLastSubshellOfACommandStringMovesItsMarkers(t *testing.T) {
	const j = "/bin/sleep 0.3 & jobs"
	row := "[2]  + running    /bin/sleep 0.3\n"
	bare := "[2]    running    /bin/sleep 0.3\n"
	for _, c := range []struct{ name, src, want string }{
		{"the last command", "(" + j + ")", row},
		{"control: followed by a command", "(" + j + "); :", bare},
		{"after a list", "true; (" + j + ")", row},
		{"the right of &&", "true && (" + j + ")", row},
		{"an if branch", "if true; then (" + j + "); fi", row},
		{"a case arm", "case x in x) (" + j + ");; esac", row},
		{"a brace group", "{ (" + j + ") }", row},
		{"nested, both last", "( :; (" + j + ") )", row},
		{"nested in a forked body", "( :; (" + j + ") ); :", bare},
		{"with a redirection", "(" + j + ") 2>/dev/null", row},
		{"a trailing newline and comment", "(" + j + ")\n# done\n", row},
		{"the last pass of a for", "for i in 1 2; do (" + j + "); done", bare + row},
		{"a C-style for", "for ((i=0;i<1;i++)); do (" + j + "); done", bare},
		{"the left of &&", "(" + j + ") && :", bare},
		{"negated", "! (" + j + ")", bare},
		{"a function body", "f() { (" + j + ") }; f", bare},
		{"eval", `eval "(` + j + `)"`, bare},
		{"a substitution", `x=$( (` + j + `) ); print -r -- "$x"`, bare},
		{"a trap with an action", `trap 'print u' USR1; (` + j + `)`, bare},
		{"a trap function", `TRAPUSR1() { : }; (` + j + `)`, bare},
		{"an ignored condition", `trap '' USR1; (` + j + `)`, row},
		{"an empty EXIT trap", `trap '' EXIT; (` + j + `)`, row},
		{
			"two jobs",
			"(/bin/sleep 0.3 & /bin/sleep 0.3 & jobs)",
			"[2]  - running    /bin/sleep 0.3\n[3]  + running    /bin/sleep 0.3\n",
		},
		{
			"the parent's jobs are not there",
			"/bin/sleep 0.3 & /bin/sleep 0.3 & /bin/sleep 0.3 & (" + j + ")", row,
		},
		{"the - is the parentheses", "(/bin/sleep 0.3 & jobs %-; echo s=$?)", "s=0\n"},
		{
			"control: forked, no previous job",
			"(/bin/sleep 0.3 & jobs %-; echo s=$?); :",
			"zsh:jobs:1: no previous job\ns=127\n",
		},
		{
			"waited for, the - is a slot",
			"(/bin/sleep 0.3 & wait %1; echo w=$?; jobs %-; echo s=$?)",
			"w=0\nzsh:jobs:1: %-: no such job\ns=127\n",
		},
		{
			"waited for, the + is no current job",
			"(/bin/sleep 0.1 & wait; jobs %%; echo s=$?; jobs %-; echo t=$?)",
			"zsh:jobs:1: no current job\ns=127\nzsh:jobs:1: no previous job\nt=127\n",
		},
		{
			"waited for, kill misses it",
			"(wait %1; kill -0 %1; echo k=$?)",
			"zsh:kill:1: %1: no such job\nk=1\n",
		},
		{
			"a job that ended hands the + back to the parentheses",
			"(/bin/sleep 0 & /bin/sleep 0.3; jobs; jobs %%; echo s=$?; jobs %-; echo t=$?)",
			"s=0\nzsh:jobs:1: no previous job\nt=127\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := runZshC(c.src); got != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, got, c.want)
			}
		})
	}
}

// **A bare `wait` reaches the `( … )` itself**, on the forked route too.
// Measured 2026-10-02 on zsh 5.9.2: `( wait; jobs %1 ); :` is `%1: no such
// job` where `( jobs %1 ); :` is 0.
func TestABareWaitReachesTheSubshellItself(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"(wait; jobs %1; echo s=$?); :", "zsh:jobs:1: %1: no such job\ns=127\n"},
		{"(jobs %1; echo s=$?); :", "s=0\n"},
		{"(wait; jobs %1; echo s=$?)", "zsh:jobs:1: %1: no such job\ns=127\n"},
	} {
		if got := runZshC(c.src); got != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, got, c.want)
		}
	}
}
