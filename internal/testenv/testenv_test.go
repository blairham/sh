// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package testenv_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/testenv"
)

// touchHomeVar asks a re-executed copy of this binary to write into the home
// it is given, so the guard can be observed tripping.
const touchHomeVar = "SH_TEST_TOUCH_HOME"

// This package's own suite runs under the guard it provides, which is the only
// way to assert from inside a run that the assembly actually happened.
func TestMain(m *testing.M) {
	os.Exit(testenv.Run("internal/testenv", m))
}

// TestTheSuiteRunsInAHomeOfItsOwn asserts the assembly happened, from inside
// the run it assembled.
//
// A test-isolation change whose whole evidence is that nothing failed proves
// nothing: everything already passed, which is how a suite spent a day writing
// into a person's home with a green board. This is the assertion the scrub can
// fail. Deleting any line of the assembly turns it red.
func TestTheSuiteRunsInAHomeOfItsOwn(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" || home != os.Getenv(testenv.TripwireHome) {
		t.Fatalf("HOME = %q, want the assembled scratch home %q", home, os.Getenv(testenv.TripwireHome))
	}
	if !testenv.Assembled() {
		t.Error("Assembled() is false inside a run Run assembled")
	}
	if left := testenv.EntriesUnder(home); len(left) > 0 {
		t.Errorf("the suite's home already holds %v; it is meant to stay empty", left)
	}
	// Set and empty, which is not the same as absent: absent means "use the
	// default", and the default is a file under the home.
	if v, ok := os.LookupEnv("HISTFILE"); !ok || v != "" {
		t.Errorf("HISTFILE = %q, %v; want it set and empty, which is recording off", v, ok)
	}
	// Everything the startup and history surface reads and no suite here is
	// about. Each one aims a read or a write at a real directory when it
	// survives: ZDOTDIR and $ENV name startup files, BASH_ENV names the one a
	// non-interactive shell reads, XDG_STATE_HOME and SH_BLOCKS_DIR name the
	// block store, FPATH names the directories an autoload searches, and
	// SHELLOPTS turns options on before a line has run.
	for _, name := range []string{
		"ZDOTDIR", "ENV", "BASH_ENV", "SHELLOPTS", "FPATH",
		"HISTSIZE", "HISTFILESIZE",
		"XDG_STATE_HOME", "SH_BLOCKS_DIR", "SH_BLOCKS_OUTPUT",
	} {
		if v, ok := os.LookupEnv(name); ok {
			t.Errorf("%s = %q survived the scrub", name, v)
		}
	}
}

// TestScratchMovesBothTheHomeAndTheHistoryFile pins the pair, which is the
// half of #890 a helper that set only HOME would have left standing.
func TestScratchMovesBothTheHomeAndTheHistoryFile(t *testing.T) {
	dir := testenv.Scratch(t)
	if got := os.Getenv("HOME"); got != dir {
		t.Errorf("HOME = %q, want the scratch home %q", got, dir)
	}
	if want := filepath.Join(dir, ".history"); os.Getenv("HISTFILE") != want {
		t.Errorf("HISTFILE = %q, want %q under the scratch home", os.Getenv("HISTFILE"), want)
	}
}

// TestTheHomeCheckSeesWhatWasLeftBehind is the other half, and the half that
// is easy to leave out: a check that reports nothing passes every run whether
// or not it can see anything.
func TestTheHomeCheckSeesWhatWasLeftBehind(t *testing.T) {
	dir := t.TempDir()
	if left := testenv.EntriesUnder(dir); len(left) != 0 {
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
		".local", ".local/state", ".local/state/sh",
		".local/state/sh/blocks", ".local/state/sh/blocks/index.jsonl",
	}
	if got := testenv.EntriesUnder(dir); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestTheHomeCheckFailsTheRun is the assertion the two above cannot make.
//
// A check whose evidence is that nothing failed proves nothing, and the end of
// the run is the one place a test inside the run cannot look at: by the time
// the entries are counted the suite is over. So the suite is run again, as a
// child, with one test in it that writes into the home it was handed — and the
// claim is that the child fails and says why.
//
// This is the mutant that survived the first pass in the suite this guard was
// lifted out of. Blanking the check left every test green, which is exactly
// the shape #890 describes having lived with for a day.
func TestTheHomeCheckFailsTheRun(t *testing.T) {
	if os.Getenv(touchHomeVar) != "" {
		// The child. Reach into the home the run handed out, the way a shell
		// that was never isolated would.
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

// TestTheAssembledEnvironmentKeepsWhatARunNeeds is the other direction: a
// scrub that kept nothing would break every suite that starts a program, and
// would do it in a way that reads as a shell bug.
func TestTheAssembledEnvironmentKeepsWhatARunNeeds(t *testing.T) {
	// PATH only: TMPDIR is on the same allowlist but is genuinely unset on
	// most Linux runners, so an empty one there is the environment and not
	// the scrub.
	if os.Getenv("PATH") == "" {
		t.Error("PATH did not survive the scrub; a shell that cannot find a program is not being tested")
	}
	if os.Getenv("TERM") != "dumb" {
		t.Errorf("TERM = %q, want dumb — a prompt here is never drawn at a person's terminal", os.Getenv("TERM"))
	}
}
