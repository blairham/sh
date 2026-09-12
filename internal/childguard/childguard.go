// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package childguard fails a test binary that leaves a process behind whose
// command line names a marker the caller chooses.
//
// The marker is what makes this a guard rather than a census, and there are
// two in this tree: the directory the interpreter's process-substitution
// pipes live in, and the directory internal/plugin's fixtures live in. Both
// are *paths*, which is not a coincidence — a path is the one thing a stray
// carries in its command line that says whose it was.
//
// A process substitution is a named pipe and a command started beside the one
// that was given its path. If the shell stops without closing its end, or
// without waiting for what it started, the command it started stays: a `cat`
// blocked in open(2) on a FIFO no writer is coming to, sleeping, killable, and
// costing 0% of a processor. Harmless one at a time and unbounded over a day
// of test runs — thirteen were found alive at once, aged half an hour to five
// hours, and they had to be killed by hand (#972).
//
// Nothing reported them, and that is the part worth fixing. The test that
// leaked most sharply was the one asserting that the directory is cleaned up:
// it asserted cleanup while leaking, passed, and said nothing. Correcting the
// three call sites that were leaking on the day would not stop the fourth,
// which is the argument #860 and #890 both landed on and the reason this is a
// guard around the whole run rather than a kill in three tests.
//
// # What it guards, and what it does not
//
// A surviving *descendant of this test binary* whose command line names the
// marker. Both halves are load-bearing:
//
//   - A descendant, because a leak is this run's doing. Several agents run
//     these suites at once on one machine, and reporting somebody else's stray
//     would make the guard cry wolf where a real one is hardest to see.
//   - Naming the marker, because it is not a census of every child. Some
//     tests mean to leave a process running — a job started with `&` and not
//     waited for is the subject of several — and a guard that fired on those
//     would be turned off within the day. The marker is what separates a leak
//     from a background job that was the point.
//
// So this does not catch every kind of stray child. It catches the class the
// leak belonged to, at every site rather than three, which is what makes it a
// guard rather than a patch.
//
// # And it cannot catch a run that was killed
//
// The census is taken after the run returns. A test binary that is killed
// rather than finished — a `go test` timeout, a SIGKILL — never reaches it,
// so nothing is reported however many strays there are; and a killed run is
// the one that leaves the most, because no deferred shutdown runs either.
//
// That is not a gap to be closed here. It is the reason a child of these
// tests has to be able to end *itself*: #1093 was seven orphaned plugin
// fixtures spinning at 11% of a core each, from a run that was killed, and
// what fixed it was the fixture noticing its own end of input rather than
// anything this package could have said afterwards. A guard reports the
// ordinary leak. Self-defense is what covers the other kind.
package childguard

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PipeMarker is the name the interpreter gives the directory its substitution
// pipes live in, and so the string a command line has to contain for the
// command to be holding one.
//
// Here rather than in the interpreter because every package whose tests run a
// shell wants the same guard, and only one of them can own the constant. The
// interpreter keeps its own — a directory prefix is its business, not a test
// helper's — and its tests pin the two together, so a prefix that changed in
// one place fails the build rather than leaving the guard quietly finding
// nothing.
//
// It is not the only marker, and a marker belongs to whoever writes the path.
// internal/plugin's fixtures are named by their own directory and that
// constant lives there, pinned to a real fixture path by a test of its own
// for the same reason this one is pinned to the interpreter's prefix.
const PipeMarker = "sh-procsub"

// Grace is how long a straggler is given to finish before it is called a leak.
//
// A substitution's own goroutine can still be closing its end of the pipe when
// the last test returns, and the command reading that end has to notice the
// end-of-file and exit — microseconds, but not zero, and a guard that reported
// those would be reporting the framework's scheduling. Long enough to cover
// that under load; short enough that a run which really did leak is not made
// to sit through it more than once.
const Grace = 5 * time.Second

// Process is one surviving process: what it is, and enough to find it with.
type Process struct {
	PID     int
	PPID    int
	Command string
}

// Wrap returns a run that fails if the tests leave a process whose command
// line contains marker.
//
// A package guards itself with
//
//	func TestMain(m *testing.M) { os.Exit(treeguard.Run(childguard.Wrap(m, childguard.PipeMarker))) }
//
// and the status is the inner run's unless something was left behind, in which
// case a passing package still fails — the same rule treeguard follows, and
// for the same reason: the damage is silent precisely because the test that
// did it passed.
func Wrap(m interface{ Run() int }, marker string) interface{ Run() int } {
	return guarded{m: m, marker: marker, grace: Grace}
}

type guarded struct {
	m      interface{ Run() int }
	marker string
	// grace is Grace everywhere but this package's own tests, which leave a
	// stray on purpose and would otherwise sit out the whole wait twice over
	// to be told what they already know.
	grace time.Duration
}

func (g guarded) Run() int {
	code := g.m.Run()
	left, dead, err := waitForLeaks(os.Getpid(), g.marker, g.grace)
	if err != nil {
		// A census that cannot be taken is reported and not enforced.
		// Refusing to report the tests over a missing `ps` would trade a real
		// suite for a hygiene check.
		fmt.Fprintf(os.Stderr, "childguard: %v — not guarding\n", err)
		return code
	}
	if len(left) == 0 && len(dead) == 0 {
		return code
	}
	if len(left) > 0 {
		fmt.Fprintf(os.Stderr, "\nchildguard: the tests left %d process(es) whose command line names %q:\n", len(left), g.marker)
		for _, p := range left {
			fmt.Fprintf(os.Stderr, "\t%d (parent %d)\t%s\n", p.PID, p.PPID, p.Command)
		}
		fmt.Fprint(os.Stderr, "Each of these is a process this run started and did not "+
			"wait for, or that was never told there was nothing left for it to do. "+
			"They reparent to init when this binary exits and stay until something "+
			"kills them.\n")
	}
	if len(dead) > 0 {
		fmt.Fprintf(os.Stderr, "\nchildguard: the tests left %d process(es) exited and unreaped:\n", len(dead))
		for _, p := range dead {
			fmt.Fprintf(os.Stderr, "\t%d (parent %d)\t%s\n", p.PID, p.PPID, p.Command)
		}
		fmt.Fprint(os.Stderr, "Each of these is a child this binary started and never "+
			"called wait on. The kernel clears them when the binary exits, so this "+
			"is bounded where a stray above is not — but \"started and never waited "+
			"for\" is the same sentence, one step short of the same consequence, and "+
			"a shell that does it in a long session accumulates them for real.\n")
	}
	if code == 0 {
		return 1
	}
	return code
}

// Holding is the surviving descendants of root whose command line names
// marker, right now and without waiting.
//
// Exported for the test whose subject is the cleanup itself: asserting that a
// directory was removed says nothing about whether anything is still reading
// what was in it, which is exactly the gap that let the sharpest leak pass.
func Holding(root int, marker string) ([]Process, error) {
	all, err := census()
	if err != nil {
		return nil, err
	}
	return strays(all, root, marker), nil
}

// Unreaped is the descendants of root that have exited and that nobody has
// called wait on — zombies.
//
// A separate question from Holding and deliberately not folded into it. A
// stray holds a descriptor, a pipe and memory, and lasts until something kills
// it; a zombie holds a process-table slot and is cleared by the kernel when
// this binary exits. The second is bounded where the first is not, which is
// why #972 shipped the marker census without this one.
//
// It is here now because the bound is not the whole story. "Started and never
// waited for" is the same sentence as a stray's, and a shell that does it at a
// prompt rather than in a test accumulates zombies over a session. Measured on
// interp's suite, a clean run ended with 55 of them — every one from a test
// fake standing in for the front end's waitpid and doing none of the waiting,
// and none of them the shell's (#1006). At zero it is worth keeping at zero,
// which a census can say and a review cannot.
//
// No marker, unlike Holding, and it needs none: a marker exists to tell a leak
// from a background job that was the point, and a *live* background job is not
// defunct. A zombie is unambiguous.
func Unreaped(root int) ([]Process, error) {
	all, err := census()
	if err != nil {
		return nil, err
	}
	return unreaped(all, root), nil
}

// waitForLeaks polls until nothing is left or the grace period is up.
//
// Both questions on one clock. Asking them in sequence would make a run that
// leaks neither pay the grace period twice, and a run that leaks one of them
// wait out the other's.
func waitForLeaks(root int, marker string, grace time.Duration) (left, dead []Process, err error) {
	const step = 100 * time.Millisecond
	deadline := time.Now().Add(grace)
	for {
		all, cerr := census()
		if cerr != nil {
			return nil, nil, cerr
		}
		left, dead = strays(all, root, marker), unreaped(all, root)
		if (len(left) == 0 && len(dead) == 0) || !time.Now().Before(deadline) {
			return left, dead, nil
		}
		time.Sleep(step)
	}
}

// census reads the process table.
//
// Through `ps` rather than through /proc, because this has to answer on both
// of the platforms every route into this program is a POSIX one on, and one of
// them has no /proc. The format is the portable spelling: empty headers, three
// columns, the last one the whole command line.
func census() ([]Process, error) {
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=,args=").Output()
	if err != nil {
		return nil, fmt.Errorf("reading the process table: %w", err)
	}
	var all []Process
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimLeft(sc.Text(), " ")
		pid, rest, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		rest = strings.TrimLeft(rest, " ")
		ppid, command, ok := strings.Cut(rest, " ")
		if !ok {
			continue
		}
		p, perr := strconv.Atoi(pid)
		q, qerr := strconv.Atoi(ppid)
		if perr != nil || qerr != nil {
			continue
		}
		all = append(all, Process{PID: p, PPID: q, Command: strings.TrimLeft(command, " ")})
	}
	return all, sc.Err()
}

// strays is the descendants of root, other than root, whose command line names
// marker.
//
// Descendants rather than children: a substitution's command can be started by
// a subshell, and a process one generation further down is no less this run's
// to have left behind.
func strays(all []Process, root int, marker string) []Process {
	return below(all, root, func(p Process) bool { return strings.Contains(p.Command, marker) })
}

// defunctMark is how `ps` renders a process that has exited and not been
// waited for. Darwin prints it alone and Linux prints it after the command in
// brackets, so the test is containment rather than equality.
const defunctMark = "<defunct>"

// unreaped is the descendants of root that `ps` calls defunct. Children only
// in practice — a zombie's children are reparented, so it has none — but the
// walk is the same one, since the question is still "is this ours".
func unreaped(all []Process, root int) []Process {
	return below(all, root, func(p Process) bool { return strings.Contains(p.Command, defunctMark) })
}

// below is the descendants of root, other than root, that want reports.
func below(all []Process, root int, want func(Process) bool) []Process {
	mine := map[int]bool{root: true}
	// Repeated until it stops growing, because `ps` orders by pid and a child
	// can be listed before its parent — a single pass would miss a
	// grandchild whose parent came later in the table.
	for grew := true; grew; {
		grew = false
		for _, p := range all {
			if !mine[p.PID] && mine[p.PPID] {
				mine[p.PID] = true
				grew = true
			}
		}
	}
	var found []Process
	for _, p := range all {
		if p.PID != root && mine[p.PID] && want(p) {
			found = append(found, p)
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].PID < found[j].PID })
	return found
}
