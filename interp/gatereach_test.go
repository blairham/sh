// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/blairham/sh/internal/testenv"
	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The gate is consulted however the interpreter is re-entered.
//
// This is the package's own claim about itself: a policy enforced outside the
// interpreter does not reach inside it, because eval, source, command
// substitution and subshells all re-enter with their own input, so the first
// eval walks around anything that is not at the point of execution.
//
// Nothing else grades it. No binary sets a Gate, so the conformance harness
// cannot see this seam at all, and a hole in it would look exactly like a
// shell that works.
func TestTheGateSeesEveryWayIn(t *testing.T) {
	dir := t.TempDir()
	sourced := filepath.Join(dir, "sourced.sh")
	if err := os.WriteFile(sourced, []byte("/bin/echo from-source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, src string }{
		{"a plain command", "/bin/echo hi"},
		{"eval", `eval '/bin/echo hi'`},
		{"source", ". " + sourced},
		{"command substitution", "x=$(/bin/echo hi)"},
		{"backquotes", "x=`/bin/echo hi`"},
		{"a subshell", "( /bin/echo hi )"},
		{"a group", "{ /bin/echo hi; }"},
		{"a background job", "/bin/echo hi & wait"},
		{"a function body", "f() { /bin/echo hi; }; f"},
		{"a loop body", "for i in 1; do /bin/echo hi; done"},
		{"a case arm", "case x in x) /bin/echo hi;; esac"},
		{"the right of &&", "true && /bin/echo hi"},
		{"through the command builtin", "command /bin/echo hi"},
		{"a here-document's expansion", "/bin/cat <<EOF\n$(/bin/echo hi)\nEOF"},
		{"a process substitution", "/bin/cat <(/bin/echo hi)"},
		{"a trap firing at exit", "trap '/bin/echo hi' EXIT; exit 0"},
		{"an expansion in an assignment", "x=$(/bin/echo hi); :"},
		{"an expansion inside arithmetic", "x=$(( $(/bin/echo 1) + 1 ))"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Guarded: a background job asks the gate from its own
			// goroutine, which is the contract and is why this is a mutex
			// and not a plain bool.
			var mu sync.Mutex
			var seen bool
			sem := PosixSemantics()
			r := newTestRunner(t, &Runner{
				Semantics: &sem,
				Stdout:    &strings.Builder{}, Stderr: &strings.Builder{},
				Gate: GateFunc(func(_ context.Context, a Action) Decision {
					mu.Lock()
					defer mu.Unlock()
					if a.Kind == ActionExec && filepath.Base(a.Path) == "echo" {
						seen = true
					}
					return Allow
				}),
			})
			// The process-substitution row makes a directory for its pipe,
			// and only CleanUp removes one.
			t.Cleanup(r.CleanUp)
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatalf("run: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if !seen {
				t.Errorf("the gate was never asked about the command in %q", tc.src)
			}
		})
	}
}

// The probes pass the gate too. A stat is an existence oracle — a file test
// against a path learns something real about the filesystem — and a
// directory read is an enumeration, so each is an action with a kind of its
// own rather than a detail of whatever asked.
func TestTheGateSeesTheProbes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "present")
	if err := testenv.WriteExecutable(file, []byte(":\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "d")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, src string
		kind      ActionKind
		path      string
	}{
		{"a conditional file test", "[[ -f " + file + " ]]", ActionStat, file},
		{"the test builtin", "test -f " + file, ActionStat, file},
		{"a symlink test", "test -L " + file, ActionStat, file},
		{"cd checking its destination", "cd " + sub, ActionStat, sub},
		{"a CDPATH candidate", "CDPATH=" + dir + "\ncd d", ActionStat, sub},
		{"a PATH candidate", "PATH=" + dir + "\npresent", ActionStat, file},
		{"a glob listing the directory", "echo *", ActionReadDir, dir},
		{"glob descent deciding what to enter", "echo */present", ActionStat, sub},
		{"the readability probe of a sourced file", "PATH=" + dir + "\n. present", ActionStat, file},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var seen bool
			sem := PosixSemantics()
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Dir: dir,
				Stdout: &strings.Builder{}, Stderr: &strings.Builder{},
				Gate: GateFunc(func(_ context.Context, a Action) Decision {
					mu.Lock()
					defer mu.Unlock()
					if a.Kind == tc.kind && a.Path == tc.path {
						seen = true
					}
					return Allow
				}),
			})
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatalf("run: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if !seen {
				t.Errorf("the gate was never asked a %s about %s in %q", tc.kind, tc.path, tc.src)
			}
		})
	}
}

// Re-entry by file is a file action, not only the commands inside it: `.`
// opens what it reads, and a process substitution opens its own end of the
// pipe. The main table above already proves the *inner* commands are seen,
// which is exactly what masked these two.
//
// Observed on the event stream rather than at the gate, because the two rows
// answer differently there and the difference is a decision. `.` reads a path
// the script wrote and is put to the gate. A substitution's pipe is a path the
// *interpreter* chose — the script wrote `<(cmd)` and could not have written
// the pipe's name — so it is recorded and never refused; Runner.ownPipe
// carries that argument, and the `asked` column below is what keeps the two
// from drifting into each other.
func TestTheBoundarySeesTheFilesBehindReentry(t *testing.T) {
	dir := t.TempDir()
	sourced := filepath.Join(dir, "sourced.sh")
	if err := os.WriteFile(sourced, []byte("/bin/echo from-source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, src string
		match     func(Action) bool
		asked     bool
	}{
		{". opens the file it reads", ". " + sourced, func(a Action) bool {
			return a.Kind == ActionOpen && a.Path == sourced && !a.Write
		}, true},
		{"a process substitution opens its pipe", "/bin/cat <(/bin/echo hi)", func(a Action) bool {
			// The path is one the shell just made for itself — a descriptor
			// of its own, since #2893 — and the write is the shell's own end,
			// feeding the inner command's output in.
			return a.Kind == ActionOpen && a.Write && strings.HasPrefix(a.Path, "/dev/fd/")
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var recorded, consulted bool
			sem := PosixSemantics()
			r := newTestRunner(t, &Runner{
				Semantics: &sem,
				Stdout:    &strings.Builder{}, Stderr: &strings.Builder{},
				Gate: GateFunc(func(_ context.Context, a Action) Decision {
					mu.Lock()
					defer mu.Unlock()
					if tc.match(a) {
						consulted = true
					}
					return Allow
				}),
				Events: SinkFunc(func(_ context.Context, e Event) {
					mu.Lock()
					defer mu.Unlock()
					if e.Kind == EventAccess && tc.match(e.Action) {
						recorded = true
					}
				}),
			})
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatalf("run: %v", err)
			}
			r.CleanUp()
			mu.Lock()
			defer mu.Unlock()
			if !recorded {
				t.Errorf("the event stream never carried the file action behind %q", tc.src)
			}
			if consulted != tc.asked {
				t.Errorf("the gate was consulted = %v for %q, want %v — see ownPipe for why "+
					"a substitution's own pipe is recorded and not refused", consulted, tc.src, tc.asked)
			}
		})
	}
}

// Every signal that reaches the kernel passes the gate, by every route that
// can send one.
//
// `kill` is a builtin, and making it one is what moved this out of the
// boundary: an external kill was an exec and the gate saw it. The routes
// differ enough to be worth naming individually — a pid, a job spec that means
// a process group, and the probe that delivers nothing. There used to be a
// fourth, `kill -CONT $$`, as a signal aimed at this process that only the
// kernel could carry out; nothing aimed at this process alone reaches the
// kernel now, and that row is the next test's (#7040).
func TestTheGateSeesEverySignalThatLeaves(t *testing.T) {
	pid, _ := aLiveProcess(t)

	for _, tc := range []struct {
		name, src string
		// want reports whether this is the action the route should produce.
		want func(a Action, jobPID int) bool
	}{
		{"a pid", "kill -TERM " + itoa(pid), func(a Action, _ int) bool {
			return a.PID == pid && a.Signal == syscall.SIGTERM
		}},
		{"the existence probe", "kill -0 " + itoa(pid), func(a Action, _ int) bool {
			return a.PID == pid && a.Signal == 0
		}},
		{"a probe aimed at this shell", "kill -0 $$", func(a Action, _ int) bool {
			return a.PID == os.Getpid() && a.Signal == 0
		}},
		{"a job spec under the monitor", "set -m\n/bin/sleep 30 &\nkill -TERM %1", func(a Action, jobPID int) bool {
			// Negative, because a job under the monitor is a process group
			// and that is how the kernel is told to mean one. Nonzero as
			// well: a job that never started would make an unasked gate and a
			// gate asked about nothing the same answer.
			return jobPID != 0 && a.PID == -jobPID && a.Signal == syscall.SIGTERM
		}},
		{"a job spec with the monitor off", "/bin/sleep 30 &\nkill -TERM %1", func(a Action, jobPID int) bool {
			// Positive, because with the monitor off the job has no group of
			// its own: it runs in this shell's, so the group is not the job's
			// to name and the process is what `%1` means (#1738). The gate
			// still sees it — this one goes through killProcess rather than
			// through the embedder's group hook, and every signal this
			// package delivers itself is visible to the boundary.
			return jobPID != 0 && a.PID == jobPID && a.Signal == syscall.SIGTERM
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var seen []Action
			sem := PosixSemantics()
			var out strings.Builder
			r := newTestRunner(t, &Runner{
				Semantics: &sem,
				Stdout:    &out, Stderr: &strings.Builder{},
				Gate: GateFunc(func(_ context.Context, a Action) Decision {
					mu.Lock()
					defer mu.Unlock()
					if a.Kind == ActionSignal {
						seen = append(seen, a)
					}
					// Refused, so nothing is really sent: the claim here is
					// that the gate is asked, and a signal that went out
					// anyway would be a live process the test then has to
					// chase.
					if a.Kind == ActionSignal {
						return Deny
					}
					return Allow
				}),
				// A group is the driver's to reach, so a runner with no hook
				// cannot signal one at all — and the gate belongs above the
				// hook, which this fake is here to demonstrate.
				SignalGroup: func(int, syscall.Signal) error { return nil },
				// A terminal, so the `set -m` in the row above is granted
				// rather than refused over an axis this test is not about.
				Terminal: true,
			})
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatalf("run: %v", err)
			}
			jobPID := lastJobPID(t, r)
			if jobPID > 0 {
				// The gate refused the signal, so the job is still running
				// and would outlive this test by half a minute — with its
				// output pipes open on the low numbers every later test's
				// substitutions are counted from. Killed here, outside the
				// shell, because nothing in the shell was allowed to.
				t.Cleanup(func() { _ = syscall.Kill(jobPID, syscall.SIGKILL) })
			}
			mu.Lock()
			defer mu.Unlock()
			for _, a := range seen {
				if tc.want(a, jobPID) {
					return
				}
			}
			t.Errorf("the gate was asked %v, want the signal %q sends", seen, tc.src)
		})
	}
}

// A signal that stays inside this process is not an action that leaves it, so
// the gate is not asked and must not be.
//
// This is the boundary drawn at the system call rather than at the builtin.
// `kill -TERM $$` with no trap for it never calls kill(2): the script stops
// and the dying is the driver's, through a hook a library leaves nil. Gating
// it would mean a policy could refuse a shell the right to stop running its
// own script, which is not a boundary anything crosses.
//
// The gate is the instrument here because it sees every kill(2) this package
// makes, and so it is the one place where "nothing left the process" can be
// read rather than inferred. The rows past the first are #7040's: an ignored
// signal, one whose default does nothing to a running process, and one whose
// default stops it all reached the kernel at our own pid, which is to say
// every other Runner in the process. The gate refuses, so that on a tree
// where one of them still leaves, nothing is really sent and the test binary
// is not the thing that stops.
//
// The last row is the control: the same harness, with a probe that does
// reach the kernel, has to see the gate asked — otherwise every other row
// would pass on a gate that was never wired to anything.
func TestTheGateIsNotAskedAboutASignalThatNeverLeaves(t *testing.T) {
	usr1 := itoa(int(syscall.SIGUSR1))
	for _, tc := range []struct {
		name, src, want string
		status          int
		asked           bool
	}{
		{name: "an untrapped fatal signal", src: "kill -TERM $$\necho after", status: 128 + int(syscall.SIGTERM)},
		{name: "an ignored signal", src: "trap '' USR1\nkill -USR1 $$\necho \"st=$?\"", want: "st=0\n"},
		{name: "an ignored signal by number", src: "trap '' USR1\nkill -" + usr1 + " $$\necho \"st=$?\"", want: "st=0\n"},
		{name: "an ignored default signal", src: "trap '' TERM\nkill $$\necho \"st=$?\"", want: "st=0\n"},
		{name: "an ignored signal twice", src: "trap '' USR1\nkill -s USR1 $$ $$\necho \"st=$?\"", want: "st=0\n"},
		{
			name: "an ignored signal from a subshell",
			src:  "trap '' USR1\n(kill -USR1 $$; echo \"in=$?\")\necho \"out=$?\"",
			want: "in=0\nout=0\n",
		},
		{name: "an ignored stop", src: "trap '' TSTP\nkill -TSTP $$\necho \"st=$?\"", want: "st=0\n"},
		{name: "a signal whose default is to discard it", src: "kill -WINCH $$\necho \"st=$?\"", want: "st=0\n"},
		{name: "a child status nobody has", src: "kill -CHLD $$\necho \"st=$?\"", want: "st=0\n"},
		{name: "urgent data", src: "kill -URG $$\necho \"st=$?\"", want: "st=0\n"},
		{name: "a continue to a running shell", src: "kill -CONT $$\necho \"st=$?\"", want: "st=0\n"},
		{name: "a keyboard stop, with no hook", src: "kill -TSTP $$\necho \"st=$?\"", want: "st=0\n"},
		{name: "a terminal-read stop, with no hook", src: "kill -TTIN $$\necho \"st=$?\"", want: "st=0\n"},
		{name: "a terminal-write stop, with no hook", src: "kill -TTOU $$\necho \"st=$?\"", want: "st=0\n"},
		{name: "a stop nobody can refuse, with no hook", src: "kill -STOP $$\necho \"st=$?\"", want: "st=0\n"},
		// The control. Refused by the gate, so `kill` reports it.
		{name: "the probe, which does leave", src: "kill -0 $$ 2>/dev/null\necho \"st=$?\"", want: "st=1\n", asked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var seen []Action
			var out strings.Builder
			sem := PosixSemantics()
			r := newTestRunner(t, &Runner{
				Semantics: &sem,
				Stdout:    &out, Stderr: &out,
				Gate: GateFunc(func(_ context.Context, a Action) Decision {
					mu.Lock()
					defer mu.Unlock()
					if a.Kind != ActionSignal {
						return Allow
					}
					seen = append(seen, a)
					return Deny
				}),
			})
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			status, err := r.Run(context.Background(), f)
			if err != nil {
				t.Fatal(err)
			}
			if status != tc.status {
				t.Errorf("status = %d, want %d", status, tc.status)
			}
			if out.String() != tc.want {
				t.Errorf("out = %q, want %q", out.String(), tc.want)
			}
			mu.Lock()
			defer mu.Unlock()
			if tc.asked && len(seen) == 0 {
				t.Errorf("the gate was never asked about %q — the instrument is not wired to anything", tc.src)
			}
			if !tc.asked && len(seen) != 0 {
				t.Errorf("the gate was asked about %v, want nothing: no signal left this process", seen)
			}
		})
	}
}

// A stop aimed at this shell is the process's to take, so it goes to the hook
// `suspend` uses, with the signal the script named — and only a stop does.
//
// The signal is the half worth asserting. A hook handed SIGSTOP for `kill
// -TSTP $$` would stop a shell in an orphaned process group that every
// reference leaves running, because the kernel discards the keyboard's three
// stops there and never discards SIGSTOP (#7040).
func TestAStopAimedAtThisShellGoesToTheHook(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want []syscall.Signal
	}{
		{"kill -TSTP $$", []syscall.Signal{syscall.SIGTSTP}},
		{"kill -s TTIN $$", []syscall.Signal{syscall.SIGTTIN}},
		{"kill -TTOU $$", []syscall.Signal{syscall.SIGTTOU}},
		{"kill -STOP $$", []syscall.Signal{syscall.SIGSTOP}},
		{"kill -TSTP $$ $$", []syscall.Signal{syscall.SIGTSTP, syscall.SIGTSTP}},
		{"(kill -TSTP $$)", []syscall.Signal{syscall.SIGTSTP}},
		// An ignore is nothing, a trap is the handler, and neither is a stop.
		{"trap '' TSTP; kill -TSTP $$", nil},
		{"trap ':' TSTP; kill -TSTP $$", nil},
		// And a signal that does not stop anything never reaches the hook.
		{"kill -CONT $$", nil},
		{"kill -WINCH $$", nil},
	} {
		t.Run(tc.src, func(t *testing.T) {
			var got []syscall.Signal
			var out strings.Builder
			sem := PosixSemantics()
			r := newTestRunner(t, &Runner{
				Semantics: &sem,
				Stdout:    &out, Stderr: &out,
				StopThisProcess: func(sig syscall.Signal) error {
					got = append(got, sig)
					return nil
				},
				Gate: GateFunc(func(_ context.Context, a Action) Decision {
					if a.Kind == ActionSignal {
						return Deny
					}
					return Allow
				}),
			})
			f, err := syntax.Parse(tc.src+"\necho \"st=$?\"", syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("the hook was handed %v, want %v", got, tc.want)
			}
			if want := "st=0\n"; out.String() != want {
				t.Errorf("out = %q, want %q", out.String(), want)
			}
		})
	}
}

// aLiveProcess starts a real process for a signal to be aimed at, and reports
// a channel that closes when it ends.
//
// A real one, because the question these tests ask is whether something
// outside this shell was reached, and a pid nobody owns cannot answer it: a
// signal to a process that is not there fails identically whether the gate
// refused it or the kernel did.
func aLiveProcess(t *testing.T) (pid int, ended <-chan struct{}) {
	t.Helper()
	cmd := exec.Command("/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("no process to signal: %v", err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-done
	})
	return cmd.Process.Pid, done
}

// lastJobPID is the pid the script's own `&` produced, which only the runner
// knows: the test cannot predict it and the gate has to be checked against it.
func lastJobPID(t *testing.T, r *Runner) int {
	t.Helper()
	jobs := r.Jobs()
	if len(jobs) == 0 {
		return 0
	}
	return jobs[len(jobs)-1].PID
}

// A pipeline asks about both halves, which is worth its own case: each is a
// process of its own and one of them is not the one being waited for.
func TestTheGateSeesBothHalvesOfAPipeline(t *testing.T) {
	t.Parallel()
	// Each half runs on its own goroutine, so the record of what was asked
	// is shared and has to be guarded — the contract on Gate, demonstrated.
	var mu sync.Mutex
	var seen []string
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{
		Semantics: &sem,
		Stdout:    &strings.Builder{}, Stderr: &strings.Builder{},
		Gate: GateFunc(func(_ context.Context, a Action) Decision {
			mu.Lock()
			defer mu.Unlock()
			// Only the execs: resolving each half stats its path, and the
			// probe is an action of its own kind.
			if a.Kind == ActionExec {
				seen = append(seen, filepath.Base(a.Path))
			}
			return Allow
		}),
	})
	f, err := syntax.Parse("/bin/echo hi | /bin/cat", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Errorf("gate saw %v, want both halves", seen)
	}
}

// The expansion a caller can ask for directly goes through it too.
//
// That entry point is how a prompt with a command substitution in it is drawn
// and how `ENV=$HOME/.shrc` is turned into a path — both of them run commands
// on behalf of a caller who never wrote a script.
func TestTheGateSeesAnExpansionAskedForDirectly(t *testing.T) {
	t.Parallel()
	var seen []string
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{
		Semantics: &sem,
		Stdout:    &strings.Builder{}, Stderr: &strings.Builder{},
		Gate: GateFunc(func(_ context.Context, a Action) Decision {
			// Only the exec is recorded; the stat that resolves the command
			// is denied too, and a denied stat quietly reads as "not there".
			if a.Kind == ActionExec {
				seen = append(seen, filepath.Base(a.Path))
			}
			return Deny
		}),
	})
	if got := r.Expand("[$(/bin/echo hi)]"); got != "[]" {
		t.Errorf("Expand gave %q, want the refused command to have produced nothing", got)
	}
	if len(seen) != 1 {
		t.Errorf("gate saw %v, want the command inside the expansion", seen)
	}
}
