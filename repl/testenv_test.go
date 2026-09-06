// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/childguard"
)

// This file is the suite's environment, and it is here for the reason #890
// put one in driver: this package *is* the thing that writes a person's
// history. It opens HISTFILE, it appends to the block store under
// $HOME/.local/state, and a session it starts does both without being asked.
// driver's suite spent a day writing 3.0 MB of records into whoever ran it,
// with a green board the whole time, and the only thing standing between this
// package and the same outcome is that every test here happens to build its
// Runner with an explicit Env.
//
// "Happens to" is the problem. Nothing under repl/ reads the process
// environment — a Runner holds its own — so today the isolation is a property
// of newTestRunner rather than of the suite. One test that hands a session the
// process's environment, or one seam that learns to read a variable, and the
// property is gone with nothing to say so. The tripwire below is what says so.
//
// The guard is #890's, and deliberately the same shape rather than a variation
// on it: assemble the environment from an allowlist, hand out a named scratch
// home, and **require that home to be empty when the run ends**. A sweep for
// writes outside an allowed root is the tempting alternative and it does not
// work — it exempts $TMPDIR, which is where t.TempDir and every scratch file
// this suite makes already live, so it passes while the hole is wide open.

// tripwireHome names the home the whole suite runs under.
const tripwireHome = "SH_TEST_REPL_HOME"

// keptExactly and keptPrefixes are the whole of what a test inherits.
//
// An allowlist rather than a list of names to remove. HISTFILE and ZDOTDIR are
// the two that were caught in driver; the next variable a dialect learns to
// read is the one a deny-list would miss, and this package's own seams are a
// second source of them — a history source or a recorder a front end supplies
// is code that may read anything. PATH is kept because a session here starts
// `sleep` and `cat` and a shell that cannot find them is not testing job
// control; TMPDIR because t.TempDir reads it; GO because the test binary's own
// machinery does.
// SH_TEST_ is this file's own channel to a re-executed copy of the test
// binary: the only way to ask what the guard does when it trips is to run a
// suite that trips it, and a child that lost the variable would run the test
// again instead of being the run under examination.
var (
	keptExactly  = []string{"PATH", "TMPDIR"}
	keptPrefixes = []string{"GO", "SH_TEST_"}
)

// touchHomeVar asks a re-executed copy of this binary to write into the home
// it is given, so the guard can be observed tripping.
const touchHomeVar = "SH_TEST_TOUCH_HOME"

// The process table as well as the scratch home. This package runs shells too,
// so a process substitution that leaks its reader leaks it here — and the
// argument for guarding every such package rather than the one where it was
// first noticed is the one #860 and #890 landed on: correcting the sites that
// were leaking on the day does not stop the next one.
func TestMain(m *testing.M) {
	os.Exit(runIsolated(childguard.Wrap(m, childguard.PipeMarker)))
}

// runIsolated assembles the environment, runs the suite in it, and then checks
// that the home it handed out was never touched.
func runIsolated(m interface{ Run() int }) int {
	home, err := os.MkdirTemp("", "sh-repl-home-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "repl tests: no scratch home: %v\n", err)
		return 1
	}
	if err := assembleEnv(home); err != nil {
		fmt.Fprintf(os.Stderr, "repl tests: %v\n", err)
		return 1
	}
	code := m.Run()
	// Reported before the removal, and the removal happens either way: a
	// leaked scratch directory is a worse second failure than the first.
	left := entriesUnder(home)
	if err := os.RemoveAll(home); err != nil {
		fmt.Fprintf(os.Stderr, "repl tests: scratch home not removed: %v\n", err)
	}
	if len(left) > 0 {
		fmt.Fprintf(os.Stderr, "%s", homeTouchedMessage(left))
		return 1
	}
	return code
}

// assembleEnv replaces the process environment with one built from nothing.
func assembleEnv(home string) error {
	kept := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if name, _, ok := strings.Cut(kv, "="); ok && isKept(name) {
			kept = append(kept, kv)
		}
	}
	os.Clearenv()
	for _, kv := range kept {
		name, value, _ := strings.Cut(kv, "=")
		if err := os.Setenv(name, value); err != nil {
			return fmt.Errorf("restoring %s: %w", name, err)
		}
	}
	assembled := map[string]string{
		"HOME":       home,
		tripwireHome: home,
		// Set and empty, which is not the same as absent: absent means "use
		// the default", and the default is a file under the home. An empty
		// HISTFILE is the shell's own way of being told not to remember this
		// session, and it turns off both halves of the record — the line file
		// and the block store, which reads the same variable first for
		// exactly this reason.
		"HISTFILE": "",
		// A prompt here is drawn into a buffer or a pseudo-terminal this
		// suite made, never a person's terminal, and a developer's own TERM
		// would decide what a session draws into it.
		"TERM": "dumb",
	}
	for name, value := range assembled {
		if err := os.Setenv(name, value); err != nil {
			return fmt.Errorf("setting %s: %w", name, err)
		}
	}
	// The scrub is worth nothing if it silently did not happen, and the
	// standard library answers this question by reading $HOME, so it agrees
	// with the sessions below or the environment is not the one built here.
	got, err := os.UserHomeDir()
	if err != nil || got != home {
		return fmt.Errorf("the suite is still running against a real home: %q (%v)", got, err)
	}
	return nil
}

func isKept(name string) bool {
	for _, k := range keptExactly {
		if name == k {
			return true
		}
	}
	for _, p := range keptPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// entriesUnder lists everything under the scratch home, relative to it.
//
// Directories count, not only files: a session that created ~/.local/state and
// wrote nothing into it still reached into a person's home.
func entriesUnder(home string) []string {
	var found []string
	_ = filepath.WalkDir(home, func(path string, _ fs.DirEntry, err error) error {
		if err != nil || path == home {
			return nil //nolint:nilerr // an unreadable entry is still an entry; the walk goes on
		}
		rel, relErr := filepath.Rel(home, path)
		if relErr != nil {
			rel = path
		}
		found = append(found, rel)
		return nil
	})
	sort.Strings(found)
	return found
}

// homeTouchedMessage says what was found and what to do about it, because the
// person who trips this will be reading it with no memory of #890.
func homeTouchedMessage(left []string) string {
	var b strings.Builder
	b.WriteString("\nrepl tests: something wrote into the home directory.\n\n")
	b.WriteString("These appeared under the suite's scratch $HOME:\n")
	for _, rel := range left {
		fmt.Fprintf(&b, "\t%s\n", rel)
	}
	b.WriteString(`
Outside this guard that $HOME is a person's own, so this is a session writing
into a real home: a history file, a block store. A test that starts a session
gives its Runner an explicit HISTFILE and HOME — newTestRunner already sets an
empty HISTFILE, which is what turns both off — and a test that is about the
history file names one under t.TempDir(). HISTFILE outranks HOME, so setting
the home alone does not move the history file. See #890.
`)
	return b.String()
}

// TestTheSuiteRunsInAHomeOfItsOwn asserts the assembly happened, from inside
// the run it assembled.
//
// A test-isolation change whose whole evidence is that nothing failed proves
// nothing: everything already passed while driver's suite was writing into a
// person's home. This is the assertion the scrub can fail.
func TestTheSuiteRunsInAHomeOfItsOwn(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" || home != os.Getenv(tripwireHome) {
		t.Fatalf("HOME = %q, want the assembled scratch home %q", home, os.Getenv(tripwireHome))
	}
	if left := entriesUnder(home); len(left) > 0 {
		t.Errorf("the suite's home already holds %v; it is meant to stay empty", left)
	}
	if v, ok := os.LookupEnv("HISTFILE"); !ok || v != "" {
		t.Errorf("HISTFILE = %q, %v; want it set and empty, which is recording off", v, ok)
	}
	// Everything the startup and history surface reads and this suite is not
	// about. Each aims a read or a write at a real directory when it survives.
	for _, name := range []string{
		"ZDOTDIR", "ENV", "BASH_ENV", "SHELLOPTS", "HISTSIZE", "HISTFILESIZE",
		"XDG_STATE_HOME", "SH_BLOCKS_DIR", "SH_BLOCKS_OUTPUT",
	} {
		if v, ok := os.LookupEnv(name); ok {
			t.Errorf("%s = %q survived the scrub", name, v)
		}
	}
}

// TestTheHomeCheckSeesWhatWasLeftBehind is the other half, and the half that
// is easy to leave out: a check that reports nothing passes every run whether
// or not it can see anything.
func TestTheHomeCheckSeesWhatWasLeftBehind(t *testing.T) {
	dir := t.TempDir()
	if left := entriesUnder(dir); len(left) != 0 {
		t.Errorf("an empty directory reported %v, want nothing", left)
	}
	// The shape the real one takes: a file at the bottom of a path a session
	// made on its way there, so a check that looked only at the top level
	// would miss it.
	nested := filepath.Join(dir, ".local", "state", "sh", "blocks")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "index.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := []string{
		".local", ".local/state", ".local/state/sh",
		".local/state/sh/blocks", ".local/state/sh/blocks/index.jsonl",
	}
	if got := entriesUnder(dir); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestTheHomeCheckFailsTheRun is the assertion the two above cannot make.
//
// A check whose evidence is that nothing failed proves nothing, and the end of
// the run is the one place a test inside the run cannot look at: by the time
// entriesUnder is called the suite is over. So the suite is run again, as a
// child, with one test in it that writes into the home it was handed — and the
// claim is that the child fails and says why.
//
// This is the mutant that survived the first pass. Blanking the check in
// runIsolated left every test green, which is exactly the shape #890 describes
// having lived with for a day.
func TestTheHomeCheckFailsTheRun(t *testing.T) {
	if os.Getenv(touchHomeVar) != "" {
		// The child. Reach into the home the suite handed out, the way a
		// session that was never isolated would.
		dir := filepath.Join(os.Getenv("HOME"), ".local", "state", "sh")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.jsonl"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestTheHomeCheckFailsTheRun$")
	cmd.Env = append(os.Environ(), touchHomeVar+"=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("a run that wrote into its home passed:\n%s", out)
	}
	for _, want := range []string{
		"something wrote into the home directory",
		".local/state/sh/index.jsonl",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("the failure does not mention %q:\n%s", want, out)
		}
	}
}
