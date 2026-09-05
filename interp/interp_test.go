// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/treeguard"
	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// TestMain guards the source tree, because this package's tests are the ones
// that can damage it. `go test` runs them in `interp/`, a Runner with no Dir
// resolves a relative path against wherever it is running, and a test that
// redirects to one therefore writes into the checkout — silently, since the
// write succeeds and the test passes. One such file reached `main` twice
// before anybody noticed it was not part of either change.
//
// newTestRunner is the fix and this is the reason the fix cannot quietly stop
// working: a test that builds its own Runner without a Dir, and writes, fails
// the package here rather than leaving a file for the next `git add -A`.
func TestMain(m *testing.M) { os.Exit(treeguard.Run(m)) }

// TestSurvivableShiftSpeaksOnlyWhereTheDialectHasWords names the field rather
// than a shell, which is the rule for a test in this package.
//
// An empty wording is an answer — one shell in the panel prints nothing at all
// for a shift it survives — so this path deliberately has no fallback, and
// that is the part worth pinning: a fallback here would put a sentence in the
// mouth of a shell that stays quiet.
func TestSurvivableShiftSpeaksOnlyWhereTheDialectHasWords(t *testing.T) {
	sem := CoreSemantics()
	sem.ShiftPastEndFatal = No
	for _, tc := range []struct{ name, wording, want string }{
		{"silent", "", ""},
		{"speaks", "shift: too far", "sh: shift: too far\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dg := Diagnostics{ShiftTooMany: tc.wording}
			out, st := run(t, `set -- a; shift 5; echo "st=$?"`, func(r *Runner) {
				r.Semantics, r.Diagnostics = &sem, &dg
			})
			if want := tc.want + "st=1\n"; out != want {
				t.Errorf("got %q, want %q", out, want)
			}
			if st != 0 {
				t.Errorf("status %d, want 0 — the script survives", st)
			}
		})
	}
}

// testPATH is the environment a test hands a Runner that reaches externals.
//
// Handed in rather than inherited, because there is nothing to inherit: a nil
// Env is genuinely empty — the package never falls back to the process
// environment — so `cat`, `sh` and `ls` need a PATH like everything else. The
// two directories every supported platform keeps the basics in, not whatever
// the developer's machine has accumulated.
func testPATH() []string { return []string{"PATH=/usr/bin:/bin"} }

// newTestRunner is how a test in this package builds a Runner.
//
// It exists because the two pieces of state a Runner reads from its own fields
// — where relative paths start and where temporary files go — are exactly the
// two a test that sets neither borrows from the machine instead. A Runner with
// no Dir resolves `>f` against the process's directory, which under `go test`
// is the package's own source directory, so the shell writes into the
// checkout; a Runner with no TMPDIR puts a process substitution's pipes in the
// machine's real temporary directory, where only CleanUp would ever remove
// them. Both failures are silent, and the first one has twice been committed
// by a later change that swept up the file it found.
//
// So the answer to "may a test construct a Runner without a directory" is no,
// and this is the shape that makes it so: two directories the framework takes
// away again, and the CleanUp registered rather than remembered. The Dir and
// the TMPDIR are deliberately different directories — a substitution's pipes
// appearing inside the shell's own working directory would be visible to a
// glob, which is a difference no real shell has.
//
// It takes the Runner rather than returning a bare one because that is what
// made it adoptable. There were 192 ad-hoc Runner literals in this package's
// tests, each with its own fields, and a helper that returned a Runner for the
// caller to fill in would have meant rewriting every one of them into a
// construction plus a list of assignments. Wrapping the literal instead leaves
// each test saying exactly what it said before, and the guard in
// runnerguard_test.go can then require the wrapper on all of them.
//
// Only what the test did not say is supplied. A test that sets its own Dir,
// or its own TMPDIR, keeps it — several are about precisely that.
func newTestRunner(t *testing.T, r *Runner) *Runner {
	t.Helper()
	if r.Dir == "" {
		r.Dir = t.TempDir()
	}
	if !hasEnv(r.Env, "TMPDIR") {
		r.Env = append(r.Env, "TMPDIR="+t.TempDir())
	}
	// Registered here rather than left to each test, which is the whole of
	// the leak: CleanUp removes the directory a process substitution made for
	// its pipes, and a test has no reason to think about it. The suite left
	// one behind per substituting Runner — in /tmp, since a Runner with no
	// TMPDIR falls back to it — and they accumulated in the thousands.
	t.Cleanup(r.CleanUp)
	return r
}

// hasEnv says whether an environment already answers for a name, so the helper
// supplies a default rather than overriding what a test meant.
func hasEnv(env []string, name string) bool {
	for _, kv := range env {
		if strings.HasPrefix(kv, name+"=") {
			return true
		}
	}
	return false
}

// output is the buffer a test's Runner writes into, and it is synchronized
// because the shell's writers outlive the statement that made them.
//
// `&` returns as soon as the job's process has a pid — that is what makes `$!`
// answerable on the next line — so `Run` returns, the helper reads the buffer,
// and the job carries on writing into it. A bare bytes.Buffer read while
// anything is still writing it is a data race whether or not the bytes ever
// mattered to anybody, and this one was reported three times against three
// *innocent* tests, because the detector charges the write to whichever test
// the binary happens to be in when it lands (#722, #726).
//
// Waiting for the jobs, below, is the other half and not a replacement for
// this one: a job a *subshell* started belongs to the clone that started it
// and never appears in this runner's table, so there is nothing to wait for
// and the lock is the only thing between it and the read.
type output struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *output) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}

// String is everything written so far, taken under the same lock the writers
// hold.
func (o *output) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

// settle waits for the background jobs the script started.
//
// So that what a test reads is what the script produced, rather than however
// much of it had arrived by the time the last foreground command finished.
// Without this, `cmd & echo done` reads back `done` and nothing else however
// loudly cmd complains — deterministically, since starting a process takes
// longer than returning from `&` — and a test asserting on a background job's
// output would be asserting on a coin toss.
//
// A *stopped* job is skipped: it has not finished and is not going to, because
// it is waiting to be told to go on and nothing here is going to tell it.
func settle(r *Runner) {
	for _, j := range r.Jobs() {
		if j.Stopped {
			continue
		}
		j.Wait()
	}
}

func run(t *testing.T, src string, setup func(*Runner)) (out string, status int) {
	t.Helper()
	return runGrammar(t, src, nil, setup)
}

// runGrammar is run() for a source that needs a construct the core grammar
// does not have: enable turns each needed flag on by name, so a test states
// the construct it depends on rather than a shell that happens to have it.
func runGrammar(t *testing.T, src string, enable func(*syntax.Dialect), setup func(*Runner)) (out string, status int) {
	t.Helper()
	return runScript(t, src, enable, setup, true)
}

// runLeavingJobsRunning is run() for a test whose subject is a job that is
// still going.
//
// The exception rather than the rule, and there should stay very few of it: a
// process group can only be asked about while the process is there, and run()
// waits until it is not. Anything asserting on *output* wants run(), because
// the output of a job is not in the buffer until the job has finished writing
// it — which is the whole point of the wait.
func runLeavingJobsRunning(t *testing.T, src string, setup func(*Runner)) (out string, status int) {
	t.Helper()
	return runScript(t, src, nil, setup, false)
}

func runScript(t *testing.T, src string, enable func(*syntax.Dialect), setup func(*Runner), wait bool) (out string, status int) {
	t.Helper()
	d := syntax.Core()
	if enable != nil {
		enable(&d)
	}
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf output
	// Bash's answers unless a test says otherwise. A test asserting a
	// *behavior* has to name a dialect, because the default is the strict
	// core and the core refuses anything the shells disagree about — which
	// is exactly what these tests are full of.
	bash := bash.Semantics()
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &bash, Env: testPATH()})
	if setup != nil {
		setup(r)
	}
	st, rerr := r.Run(context.Background(), f)
	if wait {
		// Before the buffer is read, and on the refusal path too: a script
		// the runner gave up on can have started a job before it did.
		settle(r)
	}
	if rerr != nil {
		return buf.String() + "unsupported: " + rerr.Error(), -1
	}
	return buf.String(), st
}

// TestARelativeRedirectStaysInTheRunnersOwnDirectory, which is the property
// the shared helper exists for. A Runner resolves a relative path against its
// Dir, and a Runner with no Dir resolves it against wherever the process is —
// under `go test` that is this source directory, so the shell writes into the
// checkout and says nothing about it.
//
// Asserted from both sides: the file has to be somewhere the framework takes
// away, and it has to not be here. The second half is what a passing test
// without this could never notice.
func TestARelativeRedirectStaysInTheRunnersOwnDirectory(t *testing.T) {
	const name = "written-by-a-relative-redirect"
	var dir string
	out, st := run(t, `echo content >`+name+`; cat `+name, func(r *Runner) {
		dir = r.Dir
	})
	if out != "content\n" || st != 0 {
		t.Fatalf("got %q status %d, want the file written and read back", out, st)
	}
	if dir == "" {
		t.Fatal("the helper left Dir empty, so a relative path resolves against the source tree")
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		t.Errorf("the file is not in the runner's directory: %v", err)
	}
	if _, err := os.Stat(name); err == nil {
		t.Errorf("%s was written into the source tree", name)
		// Removed as well as reported, and only this name, which nothing
		// but this test writes. A stray left in place fails every later run
		// for a reason that has already been fixed, and — the original
		// complaint — waits in the tree for somebody's `git add -A`.
		if err := os.Remove(name); err != nil {
			t.Errorf("and could not be removed: %v", err)
		}
	}
}

// TestTheHelperGivesTemporaryFilesSomewhereTemporary. A process substitution
// makes a directory for its pipes under the shell's TMPDIR, and only CleanUp
// removes it — which most tests have no reason to call. Left to the machine's
// real temporary directory that is one directory per substituting Runner,
// forever; the helper registers the cleanup so no test has to remember.
func TestTheHelperGivesTemporaryFilesSomewhereTemporary(t *testing.T) {
	r := newTestRunner(t, &Runner{})
	var tmp string
	for _, kv := range r.Env {
		if rest, ok := strings.CutPrefix(kv, "TMPDIR="); ok {
			tmp = rest
		}
	}
	if tmp == "" {
		t.Fatal("the helper left TMPDIR unset, so pipes go in the machine's own")
	}
	if tmp == r.Dir {
		t.Error("TMPDIR is the working directory, so a substitution's pipes would be visible to a glob")
	}
}

// TestTheHelperSuppliesOnlyWhatTheTestDidNot.
//
// The helper fills in a Dir and a TMPDIR, and both have to be defaults rather
// than decisions: several tests are about a particular directory — where `cd`
// lands, where a substitution's pipes go, what `pwd` reports — and a helper
// that overwrote them would quietly move the thing under test and leave the
// assertion looking at somewhere else.
//
// Asked of the helper directly, which is the gap it fills. Every other test
// sets its directories in the setup function that runs *after* the helper, so
// none of them would notice the helper overriding what it was handed.
func TestTheHelperSuppliesOnlyWhatTheTestDidNot(t *testing.T) {
	mine, myTmp := t.TempDir(), t.TempDir()
	r := newTestRunner(t, &Runner{Dir: mine, Env: append(testPATH(), "TMPDIR="+myTmp)})
	if r.Dir != mine {
		t.Errorf("Dir = %q, want the one handed in (%q)", r.Dir, mine)
	}
	var tmp []string
	for _, kv := range r.Env {
		if rest, ok := strings.CutPrefix(kv, "TMPDIR="); ok {
			tmp = append(tmp, rest)
		}
	}
	if len(tmp) != 1 || tmp[0] != myTmp {
		t.Errorf("TMPDIR = %q, want exactly the one handed in (%q)", tmp, myTmp)
	}
}

// TestTheHelperReadsTheBufferOnlyOnceTheJobsHaveStoppedWriting.
//
// The property both halves of the fix are for, asserted where a missing wait
// is visible rather than where it is merely unsafe: a background job that
// writes has to have written by the time the helper hands the output back.
//
// The race detector is not the test. It found this three times, on three
// platforms and against three tests that had backgrounded nothing at all, and
// each time it needed the write to land inside its window — which is why it
// took three reports to recognize as one thing. The content assertion is
// deterministic in the other direction: starting a process takes longer than
// returning from `&` does, so without the wait this reads back `done` and
// nothing else, every time, on every machine.
func TestTheHelperReadsTheBufferOnlyOnceTheJobsHaveStoppedWriting(t *testing.T) {
	// An external, because it is the gap: a builtin's `&` finishes inside the
	// goroutine before the pid is settled, so the shell's own writers happen
	// to be ordered already and only a real process shows the hole.
	out, st := run(t, `/bin/sh -c 'echo from-the-job >&2' & echo done`, nil)
	if st != 0 {
		t.Fatalf("status %d, want 0 — starting a job succeeds either way", st)
	}
	if !strings.Contains(out, "from-the-job") {
		t.Errorf("got %q, want the job's output too — the buffer was read while the job was still writing it", out)
	}
	if !strings.Contains(out, "done") {
		t.Errorf("got %q, want the foreground command's output as well", out)
	}
}

// TestTheBufferIsSafeToReadWhileAJobASubshellStartedIsStillWriting.
//
// The half the wait cannot do, and the reason the buffer is synchronized as
// well as waited for. `( cmd & )` files the job in the *clone's* table, so the
// runner this helper holds never hears about it and has nothing to wait for;
// the lock is the only thing between that job's writes and this read.
//
// There is no output to assert on — which of the job's bytes have arrived by
// the time the subshell returns is genuinely a race, and pinning it would be
// pinning a coin toss. What is asserted is that reading is safe, which is a
// statement `go test -race` is what makes visible: with a bare bytes.Buffer
// here the detector reports it, and CI runs the suite that way.
func TestTheBufferIsSafeToReadWhileAJobASubshellStartedIsStillWriting(t *testing.T) {
	for i := 0; i < 8; i++ {
		out, st := run(t, `( /bin/sh -c 'echo from-the-subshells-job >&2' & ) ; echo done`, nil)
		if st != 0 {
			t.Fatalf("status %d, want 0", st)
		}
		if !strings.Contains(out, "done") {
			t.Fatalf("got %q, want the foreground command's output", out)
		}
	}
}

// TestTheHelpersExceptionLeavesTheJobRunning, which is the other side of the
// wait and the reason it has an exception at all.
//
// A test that waits cannot see a job, and a job is a process: runLeavingJobsRunning
// exists so the one property that has to be asked of a live process still can
// be. Asserted by the pair, because a variant that quietly waited anyway would
// pass every test that uses it — they would simply be looking at a corpse.
func TestTheHelpersExceptionLeavesTheJobRunning(t *testing.T) {
	var r *Runner
	if _, st := runLeavingJobsRunning(t, `/bin/sleep 1 &`, func(rr *Runner) { r = rr }); st != 0 {
		t.Fatalf("status %d", st)
	}
	jobs := r.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("%d jobs, want one", len(jobs))
	}
	if jobs[0].Finished() {
		t.Error("the job had already finished, so the variant waited for it after all")
	}
	// And run() is the other answer, on the same script.
	if _, st := run(t, `/bin/sleep 0.05 &`, func(rr *Runner) { r = rr }); st != 0 {
		t.Fatalf("status %d", st)
	}
	if jobs := r.Jobs(); len(jobs) != 1 || !jobs[0].Finished() {
		t.Errorf("run() returned with the job unfinished: %v", jobs)
	}
}

func TestFieldSplittingMatchesTheSpec(t *testing.T) {
	// The rows here are the ones docs/spec/grammar/word-splitting.md measured,
	// including the asymmetry a symmetric implementation gets wrong: a
	// trailing separator is absorbed and a leading one is not.
	tests := []struct{ name, src, want string }{
		{"whitespace runs collapse", `x="a  b   c"; printf "[%s]" $x`, `[a][b][c]`},
		{"edges are stripped", `x="  a  b  "; printf "[%s]" $x`, `[a][b]`},
		{"quoted is one field", `x="a b"; printf "[%s]" "$x"`, `[a b]`},
		{"adjacent separators make one empty field", `IFS=:; x="a::b"; printf "[%s]" $x`, `[a][][b]`},
		{"leading separator makes an empty field", `IFS=:; x=":a"; printf "[%s]" $x`, `[][a]`},
		{"trailing separator does not", `IFS=:; x="a:"; printf "[%s]" $x`, `[a]`},
		{"whitespace around a separator is one delimiter", `IFS=" :"; x="a : b"; printf "[%s]" $x`, `[a][b]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := run(t, tc.src, nil)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestColonExtendsTheTest(t *testing.T) {
	// The one rule behind all four conditionals.
	tests := []struct{ src, want string }{
		// `unset` is a builtin this slice does not have; a fresh runner has
		// no variables anyway, which is the state the case is about.
		{`printf "[%s]" "${u:-D}"`, `[D]`},
		{`e=; printf "[%s]" "${e:-D}"`, `[D]`},
		{`e=; printf "[%s]" "${e-D}"`, `[]`},
		{`s=S; printf "[%s]" "${s:-D}"`, `[S]`},
		{`e=; printf "[%s]" "${e:+A}"`, `[]`},
		{`e=; printf "[%s]" "${e+A}"`, `[A]`},
	}
	for _, tc := range tests {
		got, _ := run(t, tc.src, nil)
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestAssignHasASideEffectThatOutlives(t *testing.T) {
	got, _ := run(t, `printf "[%s]" "${u:=V}"; printf "[%s]" "$u"`, nil)
	if got != `[V][V]` {
		t.Errorf("got %q, want [V][V]", got)
	}
}

func TestExitStatusAndAndOr(t *testing.T) {
	tests := []struct {
		src    string
		want   string
		status int
	}{
		{`true`, ``, 0},
		{`false`, ``, 1},
		{`! true`, ``, 1},
		{`! false`, ``, 0},
		{`true && echo yes`, "yes\n", 0},
		{`false && echo yes`, ``, 1},
		{`false || echo no`, "no\n", 0},
		// The tree already encodes one left-associative level; nothing in the
		// interpreter re-decides it.
		{`true || echo A && echo B`, "B\n", 0},
	}
	for _, tc := range tests {
		got, st := run(t, tc.src, nil)
		if got != tc.want || st != tc.status {
			t.Errorf("%s: got %q status %d, want %q status %d", tc.src, got, st, tc.want, tc.status)
		}
	}
}

func TestCommandNotFoundIs127(t *testing.T) {
	out, st := run(t, `definitely-not-a-command-xyz`, nil)
	if st != 127 {
		t.Errorf("status = %d, want 127 — the status every panel shell uses", st)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("output %q should say what happened", out)
	}
}

func TestGateRefusesAndTheShellCarriesOn(t *testing.T) {
	// A denied command is a command that failed, not a broken shell, so the
	// status is set and execution continues.
	// External commands, deliberately: `true` and `echo` are builtins and do
	// not leave the process, so the gate does not see them — which is what
	// TestBuiltinsAreNotGated asserts from the other side.
	// Only the execs are counted: resolving each command stats its path, and
	// those probes pass the gate as actions of their own kind.
	var seen []Action
	out, st := run(t, `/usr/bin/false; /bin/echo after`, func(r *Runner) {
		r.Gate = GateFunc(func(_ context.Context, a Action) Decision {
			if a.Kind == ActionExec {
				seen = append(seen, a)
			}
			if a.Kind == ActionExec && strings.HasSuffix(a.Path, "false") {
				return Deny
			}
			return Allow
		})
	})
	if len(seen) != 2 {
		t.Fatalf("the gate saw %d execs, want 2", len(seen))
	}
	if !strings.Contains(out, "refused") {
		t.Errorf("a refusal must be reported, got %q", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("the shell must carry on after a refusal, got %q", out)
	}
	if st != 0 {
		t.Errorf("status = %d; the second command succeeded", st)
	}
}

func TestGateSeesRedirectionsToo(t *testing.T) {
	// A sandbox that only gates execution has not gated the thing that writes
	// to the filesystem.
	var opens int
	_, _ = run(t, `echo hi >`+t.TempDir()+`/f`, func(r *Runner) {
		r.Gate = GateFunc(func(_ context.Context, a Action) Decision {
			if a.Kind == ActionOpen {
				opens++
				if !a.Write {
					t.Error("a > redirection should be reported as a write")
				}
			}
			return Allow
		})
	})
	if opens != 1 {
		t.Errorf("the gate saw %d opens, want 1", opens)
	}
}

func TestEventsDescribeWhatHappened(t *testing.T) {
	// Only the exec's own events: resolving the command stats its path, and
	// that probe is recorded as an access event of its own.
	var kinds []EventKind
	_, _ = run(t, `/usr/bin/true`, func(r *Runner) {
		r.Events = SinkFunc(func(_ context.Context, e Event) {
			if e.Action.Kind == ActionExec {
				kinds = append(kinds, e.Kind)
			}
		})
	})
	if len(kinds) != 2 || kinds[0] != EventCommandStart || kinds[1] != EventCommandEnd {
		t.Errorf("events = %v, want a start then an end", kinds)
	}
}

func TestEveryConstructTheParserProducesCanRun(t *testing.T) {
	// This began as a list of what was refused — pipelines, subshells,
	// groups, `if`, then `[[ ]]` and `(( ))`, then background commands — and
	// it shrank to nothing. Inverted, it is worth more than it was: every
	// node the parser can produce must be executable, so a construct added to
	// the grammar without an interpreter for it fails here rather than
	// silently doing nothing at a prompt.
	for _, src := range []string{
		`:`,                              // simple command
		`: | :`,                          // pipeline
		`: && : || :`,                    // and-or
		`( : )`,                          // subshell
		`{ :; }`,                         // group
		`if :; then :; fi`,               // if
		`while false; do :; done`,        // while
		`until :; do :; done`,            // until
		`for i in a; do :; done`,         // for
		`case x in x) :;; esac`,          // case
		`f() { :; }; f`,                  // function
		`[[ -n x ]]`,                     // test clause
		`(( 1 ))`,                        // arithmetic command
		`: &`,                            // background
		`x=1`,                            // assignment
		`a=(1 2)`,                        // array assignment
		`: >/dev/null`,                   // redirection
		`echo "$(:)" "${x:-y}" "$((1))"`, // the three substitutions
	} {
		out, st := run(t, src, nil)
		if strings.Contains(out, "not implemented") {
			t.Errorf("%s: %s", src, strings.TrimSpace(out))
		}
		if st == -1 {
			t.Errorf("%s: refused as unsupported", src)
		}
	}
}

func TestPositionalParameters(t *testing.T) {
	// The rows docs/spec/grammar/word-splitting.md measured for `"$@"` and
	// `"$*"`, which are the only place quoting produces *more* than one field.
	tests := []struct{ name, src, want string }{
		{"at keeps one field per parameter", `set -- p q r; printf "[%s]" "$@"`, `[p][q][r]`},
		{"at keeps spaces within a parameter", `set -- "a b" c; printf "[%s]" "$@"`, `[a b][c]`},
		{"star joins into one field", `set -- p q r; printf "[%s]" "$*"`, `[p q r]`},
		{"star joins with IFS's first character", `set -- a b; IFS=:; printf "[%s]" "$*"`, `[a:b]`},
		{"count", `set -- p q r; echo $#`, "3\n"},
		{"a numbered parameter", `set -- p q r; echo $2`, "q\n"},
		{"past the end is empty", `set -- p; echo "[$5]"`, "[]\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := run(t, tc.src, nil)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAtIsZeroFieldsAndStarIsOne(t *testing.T) {
	// Why `set -- "$@"` is safe on an empty list and `set -- "$*"` is not.
	if got, _ := run(t, `set --; set -- "$@"; echo $#`, nil); got != "0\n" {
		t.Errorf(`"$@" with no parameters gave %q, want 0`, got)
	}
	if got, _ := run(t, `set --; set -- "$*"; echo $#`, nil); got != "1\n" {
		t.Errorf(`"$*" with no parameters gave %q, want 1 empty field`, got)
	}
}

func TestBuiltinsRunInThisShell(t *testing.T) {
	// The reason they are builtins: a child process could not do this.
	if got, _ := run(t, `set -- a b; shift; printf "[%s]" "$@"`, nil); got != "[b]" {
		t.Errorf("shift gave %q", got)
	}
	if got, _ := run(t, `x=1; unset x; printf "[%s]" "${x-gone}"`, nil); got != "[gone]" {
		t.Errorf("unset gave %q", got)
	}
	if _, st := run(t, `:`, nil); st != 0 {
		t.Errorf(": exited %d", st)
	}
}

func TestBuiltinsAreNotGated(t *testing.T) {
	// The gate covers what leaves the process. A builtin does not, so gating
	// it would be reporting an action that never happened.
	var seen int
	_, _ = run(t, `set -- a; shift; :`, func(r *Runner) {
		r.Gate = GateFunc(func(context.Context, Action) Decision { seen++; return Allow })
	})
	if seen != 0 {
		t.Errorf("the gate saw %d actions for builtins alone, want 0", seen)
	}
}

func TestAssignmentPrefixIsAnAxisWithTwoSides(t *testing.T) {
	// POSIX keeps an assignment prefixed to a special builtin; dash and
	// ksh93 comply and bash and zsh do not. An axis is only pinned by
	// testing both of its positions — asserting one is asserting a default.
	src := `set -- a b; x=1 shift; printf "[%s]" "$x"`

	persists := PosixSemantics()
	if got, _ := run(t, src, func(r *Runner) { r.Semantics = &persists }); got != "[1]" {
		t.Errorf("with the axis set to Yes the assignment should persist, got %q", got)
	}
	drops := PosixSemantics()
	drops.AssignmentPrefixPersistsOnSpecialBuiltin = No
	if got, _ := run(t, src, func(r *Runner) { r.Semantics = &drops }); got != "[]" {
		t.Errorf("with it set to No it should not, got %q", got)
	}
}

func TestOnlyExportedVariablesReachTheEnvironment(t *testing.T) {
	out, _ := run(t, `x=private; export y=shared; env`, nil)
	if strings.Contains(out, "x=private") {
		t.Error("an unexported shell variable reached the environment")
	}
	if !strings.Contains(out, "y=shared") {
		t.Error("an exported variable did not reach the environment")
	}
}

// TestRunPartLeavesTheShellOpen: a front end reading its input a line at a
// time hands the interpreter one chunk after another, and everything a chunk
// established has to still be there for the next one.
func TestRunPartLeavesTheShellOpen(t *testing.T) {
	var out bytes.Buffer
	s := bash.Semantics()
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Semantics: &s})
	for _, src := range []string{`x=1; f() { echo "f says $x"; }`, `x=2`, `f`} {
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		if err := r.RunPart(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	if got := out.String(); got != "f says 2\n" {
		t.Errorf("got %q, want the variable and the function to have survived", got)
	}
}

// TestFinishRunsTheExitTrapOnce: the teardown belongs to the session and not
// to a chunk, or a trap would fire after every line.
func TestFinishRunsTheExitTrapOnce(t *testing.T) {
	var out bytes.Buffer
	s := bash.Semantics()
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Semantics: &s})
	for _, src := range []string{`trap 'echo bye' EXIT`, `echo one`, `echo two`} {
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		if err := r.RunPart(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	if got := out.String(); got != "one\ntwo\n" {
		t.Errorf("before Finish: got %q, want no trap yet", got)
	}
	r.Finish(context.Background())
	if got := out.String(); got != "one\ntwo\nbye\n" {
		t.Errorf("after Finish: got %q, want the trap once at the end", got)
	}
}

// TestExitedStopsTheCaller: `exit` in one chunk must stop the front end
// reading another, or the rest of the script would run after it.
func TestExitedStopsTheCaller(t *testing.T) {
	var out bytes.Buffer
	s := bash.Semantics()
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Semantics: &s})
	f, err := syntax.Parse(`echo one; exit 3; echo two`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RunPart(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if !r.Exited() {
		t.Error("Exited() is false after `exit`")
	}
	if got := r.Finish(context.Background()); got != 3 {
		t.Errorf("status %d, want 3", got)
	}
}

// TestAHeredocBodyIsNotAWord: what reaches the command on the other end.
//
// The body was being run through the word lexer, so shell quoting applied to
// it: `don't` arrived as `dont` and `\n` as `n`. Nothing reported it.
func TestAHeredocBodyIsNotAWord(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"quotes are content",
			"x=VAL\ncat <<EOF\ndon't say \"hi\"\nEOF\n",
			"don't say \"hi\"\n",
		},
		{
			"a backslash escapes three things and stays before the rest",
			"x=VAL\ncat <<EOF\n\\$x \\\\ \\n \\' \\\"\nEOF\n",
			"$x \\ \\n \\' \\\"\n",
		},
		{
			"substitutions happen",
			"x=VAL\ncat <<EOF\n$x ${x} $(echo sub) $((1+2))\nEOF\n",
			"VAL VAL sub 3\n",
		},
		{
			"a continued line is joined",
			"cat <<EOF\nabc\\\ndef\nEOF\n",
			"abcdef\n",
		},
		{
			// The other side of the switch, which was already right and is
			// here so that a change to one is not mistaken for both.
			"a quoted delimiter takes the body whole",
			"x=VAL\ncat <<'EOF'\ndon't $x \\$x \\\\ \"hi\"\nEOF\n",
			"don't $x \\$x \\\\ \"hi\"\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := run(t, tc.src, nil); out != tc.want {
				t.Errorf("got  %q\nwant %q", out, tc.want)
			}
		})
	}
}

// TestRegexModeEndsWithTheOperand: after the regular expression, a `(` is the
// shell's again — grouping inside the condition rather than part of a word.
//
// It has to be checked by what it *answers*, not by whether it parses: a
// group swallowed into a word parses perfectly well and becomes a test for
// non-emptiness, which is true where the grouped condition is false.
func TestRegexModeEndsWithTheOperand(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[[ abc =~ b && (a = z) ]] && echo y || echo n`, "n"},
		{`[[ abc =~ b && (a = a) ]] && echo y || echo n`, "y"},
		{`[[ abc =~ (b) && (a = z) ]] && echo y || echo n`, "n"},
		// And a subshell after the condition is still a subshell.
		{`[[ abc =~ b ]] && ( echo sub )`, "sub"},
	} {
		if out, _ := run(t, tc.src, nil); strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
		}
	}
}

// TestShiftReadsALeadingDashTwoWays. The same operand is an *option* in two
// dialects and a count in the other two, and the two complaints are different
// kinds of thing rather than two wordings of one.
func TestShiftReadsALeadingDashTwoWays(t *testing.T) {
	asOption := CoreSemantics()
	asOption.ShiftReadsOptions = Yes
	asOption.BadOptionToSpecialBuiltinFatal = No
	out, _ := run(t, `shift -x; echo "st=$?"`, func(r *Runner) {
		dg := Diagnostics{BuiltinBadOption: "shift: bad option: %[2]s", BuiltinBadOptionStatus: 1}
		r.Semantics, r.Diagnostics = &asOption, &dg
	})
	if !strings.Contains(out, "bad option: -x") || !strings.Contains(out, "st=1") {
		t.Errorf("as an option: got %q", out)
	}

	asCount := CoreSemantics()
	asCount.ShiftReadsOptions = No
	asCount.ShiftCountIsArithmetic = No
	asCount.BadOptionToSpecialBuiltinFatal = No
	out, _ = run(t, `shift -x; echo "st=$?"`, func(r *Runner) {
		dg := Diagnostics{ShiftBadNumber: "shift: Illegal number: %[1]s"}
		r.Semantics, r.Diagnostics = &asCount, &dg
	})
	if !strings.Contains(out, "Illegal number: -x") {
		t.Errorf("as a count: got %q", out)
	}
}

// TestABundleIsRefusedByItsFirstLetter, so `shift --help` is `-h` and not
// `--help` — the dashes are stripped and the letter after them named.
func TestABundleIsRefusedByItsFirstLetter(t *testing.T) {
	sem := CoreSemantics()
	sem.ShiftReadsOptions = Yes
	sem.BadOptionToSpecialBuiltinFatal = No
	out, _ := run(t, `shift --help`, func(r *Runner) {
		dg := Diagnostics{BuiltinBadOption: "shift: bad option: %[2]s"}
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	if !strings.Contains(out, "bad option: -h") {
		t.Errorf("got %q, want the first letter named", out)
	}
}

// TestAShiftCountCanBeAnExpression, where an unset name is zero — which is
// why one shape of this shifts nothing and succeeds rather than failing.
func TestAShiftCountCanBeAnExpression(t *testing.T) {
	sem := CoreSemantics()
	sem.ShiftCountIsArithmetic = Yes
	sem.ArrayBaseIsZero = Yes
	for _, tc := range []struct{ src, want string }{
		{`set -- a b c; shift 1+1; echo "[$*]"`, "[c]"},
		{`n=2; set -- a b c; shift n; echo "[$*]"`, "[c]"},
		{`set -- a b c; shift nosuchname; echo "[$*] st=$?"`, "[a b c] st=0"},
	} {
		out, _ := run(t, tc.src, func(r *Runner) {
			dg := Diagnostics{}
			r.Semantics, r.Diagnostics = &sem, &dg
		})
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: got %q, want %q in it", tc.src, out, tc.want)
		}
	}
}

// TestAPlainNumberAsksNothing, which keeps the axis off the path every
// ordinary `shift` takes: both readings agree on a number, so a shell with no
// answers still shifts.
func TestAPlainNumberAsksNothing(t *testing.T) {
	sem := CoreSemantics()
	out, _ := run(t, `set -- a b c; shift 2; echo "[$*]"`, func(r *Runner) {
		dg := Diagnostics{}
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	if !strings.Contains(out, "[c]") {
		t.Errorf("got %q, want an ordinary shift to need no answer", out)
	}
	if strings.Contains(out, "disagree") {
		t.Errorf("got %q, want nothing asked", out)
	}
}

// TestABadShiftOperandCanEndTheScript, under the rule a special builtin's
// failure already gets — `shift` is one, and two of the four stop for it.
func TestABadShiftOperandCanEndTheScript(t *testing.T) {
	fatal := CoreSemantics()
	fatal.ShiftReadsOptions = No
	fatal.ShiftCountIsArithmetic = No
	fatal.BadOptionToSpecialBuiltinFatal = Yes
	fatal.FatalErrorStatusIsOne = No
	out, st := run(t, `shift -x; echo after`, func(r *Runner) {
		dg := Diagnostics{}
		r.Semantics, r.Diagnostics = &fatal, &dg
	})
	if strings.Contains(out, "after") {
		t.Errorf("got %q, want the script to stop", out)
	}
	if st == 0 {
		t.Error("status = 0, want the failure to stand")
	}

	carry := CoreSemantics()
	carry.ShiftReadsOptions = No
	carry.ShiftCountIsArithmetic = No
	carry.BadOptionToSpecialBuiltinFatal = No
	out, _ = run(t, `shift -x; echo after`, func(r *Runner) {
		dg := Diagnostics{}
		r.Semantics, r.Diagnostics = &carry, &dg
	})
	if !strings.Contains(out, "after") {
		t.Errorf("got %q, want the script to carry on", out)
	}
}
