// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// This file is the suite's environment, and the reason it exists is that the
// front end starts *shells* — a shell reads a person's startup files, writes a
// person's history file and appends to a person's block store, and it does all
// three by reading the environment it was handed. A test that hands it the
// developer's environment has aimed those writes at the developer's home.
//
// Measured before this file was written, `go test ./driver/` appended 12,394
// records to `~/.local/state/sh/blocks/index.jsonl` — 3.0 MB, each record
// naming the command and the working directory it ran in — and wrote five
// lines into whatever `$HISTFILE` pointed at, which for anyone whose shell
// exports it is their real history (#890).
//
// Two other packages already had the answer. internal/smoke and
// internal/oracle both *assemble* the environment a shell is started with
// rather than passing the process's along, and internal/oracle does it while
// running interactive cases — so nothing about interactive coverage requires
// giving this up.

// tripwireHome names the home the whole suite runs under, and doubles as the
// mark that says this process was assembled by the code below.
//
// The second job is what makes it a variable rather than a package-level
// string. Several tests re-execute this binary to *be* the shell — the only
// way to ask what happens to a process that gets killed — and the environment
// such a child is handed is the assertion that test is making. So a child
// takes what it was given untouched: this name arriving already set is how it
// knows it is one.
//
// SH_TEST_-prefixed because the assembly below keeps that prefix, which is the
// channel every re-executing test in this package already uses.
const tripwireHome = "SH_TEST_HOME"

// keptExactly and keptPrefixes are the whole of what a test inherits from
// whoever ran `go test`.
//
// An allowlist and not a list of names to remove, which is the difference
// between a guard and a patch. Removing HISTFILE and ZDOTDIR would fix the two
// variables that were caught; it would say nothing about the next one a
// dialect learns to read, and the startup surface has at least four —
// Semantics.NonInteractiveStartupVariable, Semantics.StartupDirectoryVariable,
// `$ENV`, and `$HOME` behind them all. Assembling from nothing covers the
// variable that has not been added yet.
//
// PATH is here for the reason internal/smoke keeps it: a shell that cannot
// find `sleep` is not testing job control. TMPDIR is here because t.TempDir
// reads it, and the GO prefix because the test binary's own machinery does —
// GOCOVERDIR, GORACE, GOTRACEBACK. SH_TEST_ is this package's re-execution
// channel: a child that lost it would run the test again instead of becoming
// the shell the test needs.
var (
	keptExactly  = []string{"PATH", "TMPDIR"}
	keptPrefixes = []string{"GO", "SH_TEST_"}
)

func TestMain(m *testing.M) {
	// A copy of this binary re-executed as a shell, for the background-job
	// tests. Here rather than at the top of the test, which is where the
	// signal tests put theirs: those shells are killed and never return, so
	// they never reach an os.Exit inside a test — and Go 1.26 panics on one
	// that does. Measured the hard way: two of the four cases passed anyway,
	// because their evidence was a child process that survived the panic.
	bgRunningAsAShell()
	if os.Getenv(tripwireHome) != "" {
		// A shell half re-executed by a test above. Its environment was
		// assembled by the parent and then aimed by the test that started it,
		// so touching it here would overwrite the question being asked.
		os.Exit(m.Run())
	}
	os.Exit(runIsolated(m))
}

// runIsolated assembles the environment, runs the suite in it, and then checks
// that the home it handed out is still untouched.
func runIsolated(m *testing.M) int {
	home, err := os.MkdirTemp("", "sh-driver-home-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "driver tests: no scratch home: %v\n", err)
		return 1
	}
	if err := assembleEnv(home); err != nil {
		fmt.Fprintf(os.Stderr, "driver tests: %v\n", err)
		return 1
	}
	code := m.Run()
	// Reported before the removal, and the removal happens either way: a
	// leaked scratch directory is a worse second failure than the first.
	left := entriesUnder(home)
	if err := os.RemoveAll(home); err != nil {
		fmt.Fprintf(os.Stderr, "driver tests: scratch home not removed: %v\n", err)
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
		name, _, ok := strings.Cut(kv, "=")
		if ok && isKept(name) {
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
		// Where every $HOME-derived path lands, and where the check at the
		// end of the run looks.
		"HOME": home,
		// The same name, for a child that is handed a different HOME by the
		// test that starts it. See tripwireHome.
		tripwireHome: home,
		// An empty HISTFILE is the shell's own way of being told *do not
		// remember this session*, and it turns off both halves of the record:
		// the line file and the block store, which reads it first for exactly
		// this reason (internal/blocks.DirFrom). Set rather than removed,
		// because removing it means "use the default", and the default is a
		// file under the home.
		//
		// A test that is about history says so by setting its own, which is
		// what the two in this package that need one already do.
		"HISTFILE": "",
		// A prompt is drawn into a pipe here, never a terminal, and a
		// developer's own TERM would decide whether it is drawn with colour
		// in it.
		"TERM": "dumb",
	}
	for name, value := range assembled {
		if err := os.Setenv(name, value); err != nil {
			return fmt.Errorf("setting %s: %w", name, err)
		}
	}
	// The scrub is worth nothing if it silently did not happen, and there is
	// one cheap way to know: the standard library answers this question by
	// reading $HOME, so it agrees with the shells below or the environment was
	// not the one this function built.
	got, err := os.UserHomeDir()
	if err != nil || got != home {
		return fmt.Errorf("the suite is still running against a real home: %q (%v)", got, err)
	}
	return nil
}

// scratchHome gives one test a home of its own, and a history file under it.
//
// Both, because HISTFILE outranks HOME: a test that sets only the home still
// writes its history wherever the environment already pointed, and a test that
// sets only the history file still appends to the block store under the home.
// That pair is the whole of #890, and a helper that does one of them would
// have left half of it standing.
//
// Returned so a test can look at what the session left there, which is the
// other reason to want one.
func scratchHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("HISTFILE", filepath.Join(dir, ".history"))
	return dir
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
// A named directory that has to stay empty, rather than a sweep for files
// written outside some allowed root. The sweep is the tempting shape and it
// does not work: t.TempDir sits under $TMPDIR, the shells started here write
// their scratch files there too, and a check that exempts $TMPDIR exempts
// almost everything this suite does — it passes while the hole is wide open.
// This asks the one question that cannot be answered by an exemption: did
// anything at all appear in the home the suite was given?
//
// Directories count, not only files. A shell that created ~/.local/state and
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
// person who trips this will be reading it a year from now with no memory of
// #890.
func homeTouchedMessage(left []string) string {
	var b strings.Builder
	b.WriteString("\ndriver tests: something wrote into the home directory.\n\n")
	b.WriteString("These appeared under the suite's scratch $HOME:\n")
	for _, rel := range left {
		fmt.Fprintf(&b, "\t%s\n", rel)
	}
	b.WriteString(`
Outside this guard that $HOME is a person's own, so this is a test writing
into a real home directory: history, a block store, a startup file. Give the
test a home of its own with t.Setenv("HOME", t.TempDir()), and if it is about
history, a HISTFILE of its own under it too — HISTFILE outranks HOME, so
setting the home alone does not move the history file. See #890.
`)
	return b.String()
}

// TestTheSuiteRunsInAHomeOfItsOwn asserts the assembly above actually
// happened, from inside the run it assembled.
//
// A test-isolation change whose whole evidence is that nothing failed proves
// nothing: everything already passed, which is how the suite spent a day
// writing into a person's home with a green board. This is the assertion the
// scrub can fail. Deleting any line of assembleEnv's work turns it red.
func TestTheSuiteRunsInAHomeOfItsOwn(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" || home != os.Getenv(tripwireHome) {
		t.Fatalf("HOME = %q, want the assembled scratch home %q", home, os.Getenv(tripwireHome))
	}
	if left := entriesUnder(home); len(left) > 0 {
		t.Errorf("the suite's home already holds %v; it is meant to stay empty", left)
	}
	// Set and empty, which is not the same as absent: absent means "use the
	// default", and the default is a file under the home.
	if v, ok := os.LookupEnv("HISTFILE"); !ok || v != "" {
		t.Errorf("HISTFILE = %q, %v; want it set and empty, which is recording off", v, ok)
	}
	// Everything the startup surface reads and this suite is not about. Each
	// one aims a read or a write at a real directory when it survives:
	// ZDOTDIR and $ENV name startup files, BASH_ENV names the one a
	// non-interactive shell reads, XDG_STATE_HOME and SH_BLOCKS_DIR name the
	// block store, and SHELLOPTS turns options on before a line has run.
	for _, name := range []string{
		"ZDOTDIR", "ENV", "BASH_ENV", "SHELLOPTS",
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
	// The shape the real one takes: a file at the bottom of a path a shell
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
		".local",
		".local/state",
		".local/state/sh",
		".local/state/sh/blocks",
		".local/state/sh/blocks/index.jsonl",
	}
	got := entriesUnder(dir)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %v, want %v", got, want)
	}
}
