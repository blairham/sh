// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/internal/childguard"
	"github.com/blairham/sh/syntax"

	. "github.com/blairham/sh/interp"
)

// A word that expands to a path something else can open, rather than to text.
// Every case here is measured against bash in internal/oracle; what these add
// is the parts a corpus case cannot see, like what is left in the temporary
// directory afterwards.
func TestProcessSubstitutionReadsACommandAsAFile(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"one", `cat <(echo hi)`, "hi\n"},
		// Two of them in one command each need their own pipe. The first
		// attempt shared the runner's pending list through clone(), so the
		// second substitution closed the first one's end and `one` was lost.
		{"two", `cat <(echo one) <(echo two)`, "one\ntwo\n"},
		{"nested in a substitution", `cat <(cat <(echo deep))`, "deep\n"},
		// A command substitution after one is a subshell — a cloned runner —
		// and a clone that carried the pending pipes removed this one before
		// `cat` had opened it. The order is the whole test: the clone runs
		// during expansion, before the command it is an argument to.
		{"a subshell expanded after one", `cat <(echo hi) $(echo)`, "hi\n"},
		// The reason it exists rather than a pipeline: the loop runs in this
		// shell, so what it read is still here afterwards.
		{"feeds a loop in this shell", `while read -r l; do n=$((n+1)); done < <(printf "a\nb\nc\n"); echo "$n"`, "3\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// A substitution runs beside the command that named it and writes to the same
// streams, which is concurrency the shell created rather than the caller. An
// io.Writer carries no promise of being safe to write from two places, so the
// shell has to add one — and the lock has to belong to the *stream*: a lock
// per writer gives two substitutions two locks over one io.Writer, which
// excludes nothing and is what this was first written as.
func TestProcessSubstitutionSharesTheCallersStreamsSafely(t *testing.T) {
	w := &overlapWriter{}
	if _, st := run(t, `echo a > >(cat); echo b > >(cat); sleep 0.5`, func(r *Runner) {
		r.Stdout = w
	}); st != 0 {
		t.Fatalf("status %d", st)
	}
	if n := w.writes(); n < 2 {
		t.Fatalf("%d writes to the shared stream, want both substitutions", n)
	}
	if w.overlapped() {
		t.Error("two substitutions wrote to the stream at once — the lock does not cover the stream")
	}
}

// The third stream is shared too, and was not guarded.
//
// A `<(cmd)` keeps the shell's own input — only the writing direction replaces
// it — so the shell and the command it named read one io.Reader at the same
// time. os/exec copies from a reader that is not a file on a goroutine of its
// own, so two external commands running at once out of one caller-supplied
// reader are two goroutines inside it: measured, `cat <(exec /bin/echo sub)`
// with a strings.Reader for input reports a data race in strings.Reader on
// every run of twenty rounds, with the two copiers traced to the outer
// command's exec and to the substitution's.
//
// The detector is the assertion here, so this case is only as strong as the
// suite being run with -race — which `make check` and both CI platforms do.
// What makes it a case rather than a note is that it is the shape a corpus row
// reached by accident: `procsub/a-background-job-after-the-body-execs` failed
// this way and nothing else in the corpus had a substitution starting an
// external command beside one.
func TestProcessSubstitutionSharesTheCallersInputSafely(t *testing.T) {
	for range 20 {
		out, st := run(t, `cat <(exec /bin/echo sub)`, func(r *Runner) {
			r.Stdin = strings.NewReader("input the shell was handed")
		})
		if st != 0 || out != "sub\n" {
			t.Fatalf("out = %q status %d, want %q", out, st, "sub\n")
		}
	}
}

// overlapWriter is a caller's io.Writer that notices being written to from two
// places at once. It holds a lock of its own, so the *test* is safe whatever
// the shell does; what it reports is whether the shell needed it to.
type overlapWriter struct {
	mu     sync.Mutex
	inside int
	n      int
	seen   bool
}

func (w *overlapWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.inside++
	w.n++
	if w.inside > 1 {
		w.seen = true
	}
	w.mu.Unlock()

	// Wide enough that two unsynchronized writers land in it together.
	time.Sleep(20 * time.Millisecond)

	w.mu.Lock()
	w.inside--
	w.mu.Unlock()
	return len(p), nil
}

func (w *overlapWriter) writes() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.n
}

func (w *overlapWriter) overlapped() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.seen
}

// The other direction: a path to write *to*, with a command on the far end.
//
// Through a file rather than the shell's stdout, because the substitution runs
// beside the command that named it — reading its output here would be reading
// it while it is still being written.
func TestProcessSubstitutionWritesIntoACommand(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	if _, st := run(t, `echo hi > >(tr a-z A-Z > `+out+`)`, nil); st != 0 {
		t.Fatalf("status %d", st)
	}
	if got := waitForFile(t, out); got != "HI\n" {
		t.Errorf("%s = %q, want %q — the command on the far end never ran", out, got, "HI\n")
	}
}

// More than a pipe buffer, in both directions.
//
// The reason the inner command runs concurrently rather than to completion
// first — a pipe holds 64 kibibytes and no more, so anything longer than that
// only moves if both ends are live at once. It is also the case that catches a
// descriptor left in nonblocking mode: the shell opens its own end without
// waiting, and O_NONBLOCK belongs to the open file description, so a flag left
// set travels into the command on the far side and turns a write it should
// have waited on into an EAGAIN it reports as an error. Under 64 kibibytes
// nothing ever waits and the flag is invisible.
func TestProcessSubstitutionCarriesMoreThanAPipeBuffer(t *testing.T) {
	const lines = 30000 // roughly 200 kibibytes, well past any pipe buffer
	t.Run("reading from one", func(t *testing.T) {
		out, st := run(t, `cat <(seq `+strconv.Itoa(lines)+`) | tail -n 1`, nil)
		if st != 0 || out != strconv.Itoa(lines)+"\n" {
			t.Errorf("out = %q status %d, want the last of %d lines", out, st, lines)
		}
	})
	t.Run("writing into one", func(t *testing.T) {
		dir := t.TempDir()
		out := filepath.Join(dir, "out")
		src := `seq ` + strconv.Itoa(lines) + ` > >(tail -n 1 > ` + out + `)`
		if _, st := run(t, src, nil); st != 0 {
			t.Fatalf("status %d", st)
		}
		if got := waitForFile(t, out); got != strconv.Itoa(lines)+"\n" {
			t.Errorf("%s = %q, want the last of %d lines", out, got, lines)
		}
	})
}

// What the path names is a pipe, and not a temporary file the shell filled in
// first. The difference is the whole point: a file would have to be complete
// before the reader started, and most things worth substituting are longer
// than that is affordable for.
func TestProcessSubstitutionExpandsToAPipe(t *testing.T) {
	out, st := run(t, `[ -p <(true) ] && echo pipe`, nil)
	if st != 0 || out != "pipe\n" {
		t.Errorf("out = %q status %d, want a pipe", out, st)
	}
}

// The pipe goes away with the command that named it, not at exit — a session
// that runs one in a loop would otherwise fill its directory.
func TestProcessSubstitutionRemovesItsPipe(t *testing.T) {
	out, st := run(t, `p=$(echo <(true)); [ -e "$p" ] && echo still || echo gone`, nil)
	if st != 0 || out != "gone\n" {
		t.Errorf("out = %q status %d, want the pipe removed with its command", out, st)
	}
}

// A shell that ends takes the directory with it. Nothing else ever would: it
// is under the machine's temporary directory, where a leak is permanent.
func TestProcessSubstitutionCleanUpRemovesTheDirectory(t *testing.T) {
	// The directory is made under the shell's temporary one, so pointing
	// that at a fresh directory is what makes it findable from here — and
	// the shell's is TMPDIR in the environment the Runner was handed, not
	// the process's own. t.Setenv would say nothing to this package.
	dir := t.TempDir()
	var r *Runner
	// Four substitutions across two commands, because the claim is one
	// directory per *shell* — made when the first one needs it and not per
	// substitution, which leaves one behind for every one but the last.
	src := "cat <(echo a) <(echo b) >/dev/null; cat <(echo c) <(echo d) >/dev/null"
	if _, st := run(t, src, func(rr *Runner) {
		r = rr
		rr.Env = append(testPATH(), "TMPDIR="+dir)
	}); st != 0 {
		t.Fatalf("status %d", st)
	}
	// One directory, and it is gone — asserted in that order and about the
	// same path, because only the pair says anything. That the shell made
	// one is what would make leaving it a leak, and naming it is what lets
	// the second half mean "this one" rather than "the TMPDIR looks tidy".
	gone(t, pipeDirMade(t, r, dir))
	if n := len(subdirs(t, dir)); n != 0 {
		t.Errorf("%d directories left under TMPDIR, want none", n)
	}
	// And nothing is still reading what was in it. Removing a directory says
	// nothing about that on a Unix — a process holding a pipe open does not
	// care that the name is gone — which is exactly the gap that let this
	// test assert cleanup while leaking a `cat` per run (#972).
	// Scoped to this test's own temporary directory rather than to the
	// prefix: the whole-run guard in TestMain looks for the prefix, and
	// asking the same question here would answer it about whatever else in
	// the package still has a substitution in flight.
	held, err := childguard.Holding(os.Getpid(), dir)
	if err != nil {
		t.Skipf("no process table here: %v", err)
	}
	for _, p := range held {
		t.Errorf("process %d is still holding a pipe after CleanUp: %s", p.PID, p.Command)
	}
}

// The path is a filename and reaches the command as one. A temporary
// directory whose name holds a glob character is not hypothetical — TMPDIR is
// whatever the user's environment says — and without escaping the path is
// matched as a pattern, matches nothing, and is passed through as itself only
// by the accident that an unmatched pattern usually is.
func TestProcessSubstitutionPathIsNotGlobbed(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "od[d]dir")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out, st := run(t, `cat <(echo hi)`, func(r *Runner) {
		r.Env = append(testPATH(), "TMPDIR="+dir)
		sem := CoreSemantics()
		// A pattern matching nothing is an error here, which is what turns
		// the unescaped path from silently wrong into visible.
		sem.GlobNoMatchIsError = Yes
		sem.GlobExpansionResults = Yes
		r.Semantics = &sem
	})
	if st != 0 || out != "hi\n" {
		t.Errorf("out = %q status %d, want the path used as a name", out, st)
	}
}

// Where the pipes go is the Runner's answer and never the process's.
//
// This is the library rule, not a compatibility one: nothing a real shell
// does is visible here, because every shell in the panel expands `<(cmd)` to
// a /dev/fd path and consults no temporary directory at all. Our named pipe
// is forced by Go's close-on-exec, so we alone have the question — and the
// answer has to come off the Runner, because the process's TMPDIR is one
// value every embedded shell in a program would share.
//
// The regression it pins: os.MkdirTemp with an empty first argument is
// os.TempDir, which is os.Getenv("TMPDIR"). Two shells in one process could
// not be told apart by it, and an embedder that handed one an environment
// got the machine's answer anyway.
func TestProcessSubstitutionIgnoresTheProcessTMPDIR(t *testing.T) {
	// The decoy is what a leak would use, and it is watched rather than
	// merely unused: an empty directory afterwards is the assertion.
	decoy := t.TempDir()
	t.Setenv("TMPDIR", decoy)
	mine := t.TempDir()

	var r *Runner
	out, st := run(t, `cat <(echo hi)`, func(rr *Runner) {
		r = rr
		rr.Env = append(testPATH(), "TMPDIR="+mine)
	})
	if st != 0 || out != "hi\n" {
		t.Fatalf("out = %q status %d, want the substitution to work", out, st)
	}
	// Where the shell put them, which it remembers after taking them away
	// again — the directory itself is gone by now, since the shell ended.
	pipeDirMade(t, r, mine)
	if n := len(subdirs(t, decoy)); n != 0 {
		t.Errorf("%d directories under the process's TMPDIR, want none — the Runner's was handed in", n)
	}
}

// A script that assigns TMPDIR moves its own shell's pipes.
//
// getVar reads the variable table before the environment, which is the same
// route `~` takes to HOME, and it is what keeps the answer per-Runner all the
// way down: the assignment is this shell's and no other shell in the process
// sees it. Recorded because the alternative — a field seeded once at
// construction — would leave the assignment inert with nothing saying so.
func TestProcessSubstitutionHonorsAnAssignedTMPDIR(t *testing.T) {
	assigned := t.TempDir()
	handedIn := t.TempDir()

	var r *Runner
	src := `TMPDIR=` + assigned + `; cat <(echo hi)`
	out, st := run(t, src, func(rr *Runner) {
		r = rr
		rr.Env = append(testPATH(), "TMPDIR="+handedIn)
	})
	if st != 0 || out != "hi\n" {
		t.Fatalf("out = %q status %d, want the substitution to work", out, st)
	}
	pipeDirMade(t, r, assigned)
	if n := len(subdirs(t, handedIn)); n != 0 {
		t.Errorf("%d directories under the inherited TMPDIR, want none — the assignment shadows it", n)
	}
}

// A Runner with no TMPDIR anywhere still works, because the zero value of
// this package is meant to be usable and a nil Env is genuinely empty.
func TestProcessSubstitutionWithoutATMPDIR(t *testing.T) {
	var r *Runner
	out, st := run(t, `cat <(echo hi)`, func(rr *Runner) {
		r = rr
		// Back to bare: the shared helper seeds one so the suite does not
		// litter, and this is the one test that must not have it.
		rr.Env = testPATH()
	})
	if st != 0 || out != "hi\n" {
		t.Errorf("out = %q status %d, want the substitution to work with no TMPDIR", out, st)
	}
	if r != nil {
		t.Cleanup(r.CleanUp)
	}
}

// TestASubstitutionsDirectoryIsTakenAwayAgain, which is the whole of #1284.
//
// A substitution makes a directory under the shell's TMPDIR for its pipes. The
// pipes themselves go as soon as the command that named them is done — that is
// removeProcSubs, and it is why a long session does not fill its directory —
// but the directory outlives them, and for a long time the only thing that
// removed it was a CleanUp nobody called. Every invocation of every dialect
// binary that used `<(…)` therefore left one in /tmp, permanently: 10,585 in
// one working session, and 4,188 more in the two days after they were swept.
//
// Finish removes it now, so a shell that ends has already done this and the
// binaries get it without any of them being wired up — which was the actual
// failure, since CleanUp existed and was tested the whole time.
//
// Asserted in both directions about the *same path*, because only the pair
// says anything: that the shell made one is what would make leaving it a leak,
// and naming it is what keeps the second half from passing against a shell
// that never made one at all.
func TestASubstitutionsDirectoryIsTakenAwayAgain(t *testing.T) {
	tmp := t.TempDir()
	var r *Runner
	out, st := run(t, `cat <(echo hi)`, func(rr *Runner) {
		r = rr
		rr.Env = append(testPATH(), "TMPDIR="+tmp)
	})
	if st != 0 || out != "hi\n" {
		t.Fatalf("out = %q status %d, want the substitution to work", out, st)
	}
	gone(t, pipeDirMade(t, r, tmp))
	if n := len(subdirs(t, tmp)); n != 0 {
		t.Errorf("%d directories under TMPDIR after the run, want none", n)
	}
}

// TestASubshellsDirectoryIsTheParentsToRemove: the one route that still leaked
// after Finish learned to clean up, and the reason clone asks for the box.
//
// `( cat <(echo hi) )` makes its pipe inside a subshell. The directory is one
// per shell *tree* — a clone shares the parent's rather than making a second —
// but a clone taken before the parent had ever needed one copied a nil pointer
// and the subshell then made a box of its own. The parent finished knowing
// nothing about the directory in it, and a subshell may not clean up for
// itself: it shares the box, so a copy removing it would take the original's
// pipes away mid-command.
//
// Both halves are here because either alone passes wrongly. That the parent
// knows the path is what makes the removal possible; that a later substitution
// in the *parent* still works is what says the sharing did not turn into the
// subshell tidying up behind a shell that was still running.
func TestASubshellsDirectoryIsTheParentsToRemove(t *testing.T) {
	tmp := t.TempDir()
	var r *Runner
	out, st := run(t, `( cat <(echo one) ); cat <(echo two)`, func(rr *Runner) {
		r = rr
		rr.Env = append(testPATH(), "TMPDIR="+tmp)
	})
	if want := "one\ntwo\n"; st != 0 || out != want {
		t.Fatalf("out = %q status %d, want %q at 0 — the parent's substitution "+
			"has to survive the subshell's", out, st, want)
	}
	gone(t, pipeDirMade(t, r, tmp))
	if n := len(subdirs(t, tmp)); n != 0 {
		t.Errorf("%d directories under TMPDIR, want none — a subshell's is the "+
			"parent's to remove, and one per tree is the whole claim", n)
	}
}

// And the two halves of a pipeline are clones of one parent running at once,
// so they number their pipes in a directory they share.
//
// The regression: sharing the directory without sharing the counter had both
// clones ask for `sub1`, and the second mkfifo failed with "file exists". One
// counter in the box the directory is in is what keeps the names distinct,
// and it is atomic because these two are goroutines.
func TestPipelineHalvesNumberTheirPipesApart(t *testing.T) {
	tmp := t.TempDir()
	var r *Runner
	out, st := run(t, `cat <(echo a) | ( cat <(echo b) )`, func(rr *Runner) {
		r = rr
		rr.Env = append(testPATH(), "TMPDIR="+tmp)
	})
	if want := "b\n"; st != 0 || out != want {
		t.Fatalf("out = %q status %d, want %q at 0", out, st, want)
	}
	gone(t, pipeDirMade(t, r, tmp))
}

// TestTheHelperRegistersTheCleanUpSoNoTestHasTo: the same property, asked of
// the helper rather than of Finish — and still worth having, because a Runner
// driven with RunPart never reaches Finish and a test that does that is the
// one shape the shell cannot clean up after.
//
// RunPart rather than Run, deliberately. Run ends with Finish, which removes
// the directory itself, so a test written that way would pass with the helper
// registering nothing at all — the non-discriminating probe this whole file
// has just been rewritten to get rid of. Leaving the shell open is what makes
// the registration the only thing that can remove the directory.
//
// The registration runs when the test that made the Runner ends, so a test
// cannot watch its own. It can watch an inner one: a subtest's cleanups have
// all run by the time t.Run returns, so the directory is either gone by then
// or it was never going to be. The TMPDIR is this test's rather than the inner
// one's for the same reason — a directory the framework is about to remove
// anyway could not tell us who removed it.
func TestTheHelperRegistersTheCleanUpSoNoTestHasTo(t *testing.T) {
	tmp := t.TempDir()
	var dir string
	t.Run("a runner left open, which nothing else cleans up after", func(t *testing.T) {
		f, err := syntax.Parse(`cat <(echo hi) >/dev/null`, syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		sem := testSemantics()
		r := newTestRunner(t, &Runner{
			Stdout: &strings.Builder{}, Stderr: &strings.Builder{},
			Semantics: &sem,
			Env:       append(testPATH(), "TMPDIR="+tmp),
		})
		if err := r.RunPart(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		dir = pipeDirMade(t, r, tmp)
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("%s: %v — the shell is still open, so its directory has to be", dir, err)
		}
	})
	gone(t, dir)
	if n := len(subdirs(t, tmp)); n != 0 {
		t.Errorf("%d directories left once the test that made them ended, want none — "+
			"the helper has to register CleanUp, because no test is going to", n)
	}
}

func subdirs(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return ents
}

// waitForFile reads a file the substitution is filling in beside us.
func waitForFile(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		b, err := os.ReadFile(path)
		if err == nil && len(b) > 0 {
			return string(b)
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			return strings.TrimSpace(string(b))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// pipeDirMade reports the directory a shell made for its substitution pipes,
// having asserted that it made one and that it is under want.
//
// The shared spelling of the probe four tests across three files need: they
// run a substitution nothing opens, so no command starts, no Action is gated
// and no Event is emitted — the directory is the only thing that happened.
// Globbing for it afterwards is what they used to do, and Finish removing it
// (#1284) is what ended that. Naming it does not depend on it still being
// there, which is the property that makes the same probe answer both "one was
// performed" and "it was taken away again".
func pipeDirMade(t *testing.T, r *Runner, want string) string {
	t.Helper()
	dir := r.PipeDirForTest()
	if dir == "" {
		t.Fatal("no process substitution ran: the shell never made a directory for a pipe")
	}
	if filepath.Dir(dir) != want {
		t.Fatalf("pipes went to %q, want a directory under %q", dir, want)
	}
	return dir
}

// gone asserts that a path is not there, which after a run is the whole of
// what #1284 asked for.
func gone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("%s is still there, want it removed", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("%s: %v", path, err)
	}
}

// TestTheExecReplacementCleansUpBeforeTheExecve.
//
// `exec cmd` is the one end of a shell that Finish is not on: a successful
// execve does not come back, so nothing after it is this shell and there is no
// later moment to clean up in. The directory would be left exactly as it was
// before #1284, on a route the ordinary tests cannot see.
//
// Observed from inside the hook, which is the only place it can be observed
// from. A test cannot let the replacement happen — the process it would
// replace is the test binary — and a hook that reports failure puts the shell
// back on the ordinary road, where Finish cleans up and the question is
// answered by the wrong code. So the assertion is made at the moment of the
// call, before the answer is given.
func TestTheExecReplacementCleansUpBeforeTheExecve(t *testing.T) {
	tmp := t.TempDir()
	var r *Runner
	var atTheExecve []os.DirEntry
	var called bool
	// Two commands: the first makes the directory, the second is the exec
	// that never returns. Split so the substitution is not part of the
	// replacement's own redirections, which is a different question.
	src := `cat <(echo hi) >/dev/null; exec /bin/echo`
	_, st := run(t, src, func(rr *Runner) {
		r = rr
		rr.Env = append(testPATH(), "TMPDIR="+tmp)
		rr.ReplaceProcess = func(string, []string, []string, []*os.File) error {
			called = true
			atTheExecve = subdirs(t, tmp)
			// Reported as a failure so the test binary survives. The status
			// afterwards is not what this test is about.
			return errors.New("not replacing a test binary")
		}
	})
	if !called {
		t.Fatalf("the replacement was never reached (status %d) — the test proves nothing", st)
	}
	if len(atTheExecve) != 0 {
		t.Errorf("%d directories still under TMPDIR at the execve: %v — nothing after "+
			"a successful one is this shell, so this is the last moment there is",
			len(atTheExecve), atTheExecve)
	}
	// And the shell did make one, so the assertion above is not vacuous.
	if r.PipeDirForTest() == "" {
		t.Error("no process substitution ran")
	}
}

// TestThePipeDirectoryCarriesTheMarkerTheGuardLooksFor.
//
// interp names the directory and internal/childguard finds it again in a
// process's command line, and they are two literals. The comment on
// procSubDirPrefix has said since it was written that a prefix changed in one
// place and not the other would leave the guard quietly finding nothing —
// quietly being the whole problem, since a guard that matches nothing reports
// nothing and passes.
//
// Written down here because a mutation run walked straight through it: renaming
// the interp constant killed no test at all. This is the assertion that makes
// the two literals one fact.
func TestThePipeDirectoryCarriesTheMarkerTheGuardLooksFor(t *testing.T) {
	tmp := t.TempDir()
	var r *Runner
	if _, st := run(t, `cat <(echo hi) >/dev/null`, func(rr *Runner) {
		r = rr
		rr.Env = append(testPATH(), "TMPDIR="+tmp)
	}); st != 0 {
		t.Fatalf("status %d", st)
	}
	dir := pipeDirMade(t, r, tmp)
	if base := filepath.Base(dir); !strings.HasPrefix(base, childguard.PipeMarker) {
		t.Errorf("the pipe directory is %q, want a name starting with childguard.PipeMarker (%q) — "+
			"the guard finds these by that string in a command line, and a prefix that "+
			"moved in one place and not the other finds nothing and says nothing",
			base, childguard.PipeMarker)
	}
}

// `exec > >(cmd)` puts a pipe's writing end on the shell's own standard
// output, and the body's bytes have to reach the caller's stream anyway.
//
// This is the one shape removeProcSubs cannot finish with, because the only
// holder of the end the body is reading until is the shell itself: waiting
// there would be waiting for itself. Every other shell in the panel gets the
// close for free, from the process exiting — so this shell does it at the end
// of the shell instead, which is Runner.endHeldProcSubs (#2198).
//
// Measured 2026-09-12: every want below is bash 5.3's and bash 3.2's, and the
// first two are zsh 5.9.2's as well. Before the join this shell answered the
// empty string — **not every time**, which is the part that makes a
// hand-run probe worthless here: the body is a goroutine racing the process
// exit, so it won `hi` in roughly one run in eight. A test that watched for
// the loss with a `sleep` would be a window rather than a question. What
// makes these deterministic is the join, and each of them was watched failing
// with it taken out.
func TestAWritingSubstitutionOnTheShellsOwnOutputIsJoinedAtItsEnd(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the shell's output", `exec > >(cat); printf hi`, "hi"},
		// A subshell is a shell ending too, and its exit is the close in
		// every shell of the panel.
		{"inside a subshell", `( exec > >(cat); printf hi )`, "hi"},
		// A command substitution collects what the body wrote, because the
		// body's stream is the buffer the substitution is reading — so the
		// join has to happen before the collection and not after it.
		{"collected by a command substitution", `v=$( exec > >(cat); printf hi ); printf "[%s]" "$v"`, "[hi]"},
		// A numbered descriptor is the same question with the pipe off the
		// three named streams, which is where the *first* version of the
		// held-descriptor clause looked and the only place it looked.
		{"a numbered descriptor", `exec 3> >(cat); printf hi >&3`, "hi"},
		// Two at once: every end is closed before any body is waited for, so
		// the first body is not waited for while the second's end is open.
		// Closing and waiting in step deadlocks this row and no other.
		{"two of them", `exec 3> >(cat) 4> >(cat); printf a >&3; printf b >&4`, "ab"},
		// A close the script writes for itself has to be a close: `>&-` took
		// the reference away and left the file open, so the body read on
		// forever and the join at the end never came back. That is a hang
		// rather than a loss, and it is the row that fails by timing out.
		{"closed by the script", `exec > >(cat); printf hi; exec >&-`, "hi"},
		{"a numbered one closed by the script", `exec 3> >(cat); printf hi >&3; exec 3>&-`, "hi"},
		// The control: a shell that closed its end and never opened one has
		// nothing to wait for, and must not wait anyway.
		{"nothing written into it", `exec 3> >(cat); exec 3>&-; printf done`, "done"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, nil)
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}
