// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

// CleanUp takes the directory as well. A library caller running many scripts
// on one Runner has nothing else that would.
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
	if n := len(subdirs(t, dir)); n != 1 {
		t.Fatalf("%d directories under TMPDIR, want the one the shell made", n)
	}
	r.CleanUp()
	if n := len(subdirs(t, dir)); n != 0 {
		t.Errorf("%d directories left after CleanUp, want none", n)
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
	t.Cleanup(r.CleanUp)
	if n := len(subdirs(t, decoy)); n != 0 {
		t.Errorf("%d directories under the process's TMPDIR, want none — the Runner's was handed in", n)
	}
	if n := len(subdirs(t, mine)); n != 1 {
		t.Errorf("%d directories under the Runner's TMPDIR, want the one its shell made", n)
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
	t.Cleanup(r.CleanUp)
	if n := len(subdirs(t, assigned)); n != 1 {
		t.Errorf("%d directories under the assigned TMPDIR, want the one the shell made", n)
	}
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

// TestASubstitutionsDirectoryIsTakenAwayAgain, which is what the shared helper
// is for and the thing no individual test would ever notice.
//
// A substitution makes a directory under the shell's TMPDIR for its pipes. The
// pipes themselves go as soon as the command that named them is done — that is
// removeProcSubs, and it is why a long session does not fill its directory —
// but the directory outlives them and only CleanUp removes it. A test has no
// reason to call CleanUp, so the suite left one behind per substituting Runner,
// in /tmp, since a Runner with no TMPDIR falls back to it. They had reached
// four figures on this machine.
//
// Asserted in both directions, because only the pair says anything: that the
// directory is there afterwards is what makes leaving it a leak, and that it is
// gone after CleanUp is what makes registering CleanUp the fix.
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
	if n := len(subdirs(t, tmp)); n != 1 {
		t.Fatalf("%d directories under TMPDIR after the run, want the one the shell made", n)
	}
	r.CleanUp()
	if n := len(subdirs(t, tmp)); n != 0 {
		t.Errorf("%d directories under TMPDIR after CleanUp, want none", n)
	}
}

// TestTheHelperRegistersTheCleanUpSoNoTestHasTo: the same property, asked of
// the helper rather than of CleanUp — and the one that pins the fix, since a
// test calling CleanUp itself proves nothing about the 191 that do not.
//
// The registration runs when the test that made the Runner ends, so a test
// cannot watch its own. It can watch an inner one: a subtest's cleanups have
// all run by the time t.Run returns, so the directory is either gone by then
// or it was never going to be. That is the whole leak, reproduced and
// observed, in the shape the suite actually has.
//
// The TMPDIR is this test's rather than the inner one's for the same reason —
// a directory the framework is about to remove anyway could not tell us who
// removed it.
func TestTheHelperRegistersTheCleanUpSoNoTestHasTo(t *testing.T) {
	tmp := t.TempDir()
	t.Run("a runner that substitutes and never cleans up after itself", func(t *testing.T) {
		out, st := run(t, `cat <(echo hi)`, func(rr *Runner) {
			rr.Env = append(testPATH(), "TMPDIR="+tmp)
		})
		if st != 0 || out != "hi\n" {
			t.Fatalf("out = %q status %d, want the substitution to work", out, st)
		}
		if n := len(subdirs(t, tmp)); n != 1 {
			t.Fatalf("%d directories during the run, want the one the shell made", n)
		}
	})
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
