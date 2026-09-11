// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package testenv gives a test suite that starts shells an environment of its
// own, assembled from nothing rather than inherited from whoever ran `go test`.
//
// The reason a suite needs one is that a shell reads a person's startup files,
// writes a person's history file and appends to a person's block store, and it
// does all three by reading the environment it was handed. A test that hands a
// shell the developer's environment has aimed those reads and writes at the
// developer's home — so the suite is measuring the developer's dotfiles rather
// than the shell, and the same test is green on a runner whose home is empty
// and red on the machine the work is done on. That is the worst shape a gate
// can take, because the failure looks like the branch under it (#890, #1984,
// #1987).
//
// The guard has three parts and all three are load-bearing:
//
//  1. **Assemble from an allowlist**, never remove names. Removing HISTFILE
//     and ZDOTDIR fixes the two variables that were caught; it says nothing
//     about the next variable a dialect learns to read, and the startup
//     surface has at least four — Semantics.NonInteractiveStartupVariable,
//     Semantics.StartupDirectoryVariable, $ENV, and $HOME behind them all.
//  2. **Hand out a named scratch home** and put its name in the environment
//     too, so a test that re-executes the binary as a shell can tell it is the
//     child and leave the environment its parent aimed alone.
//  3. **Require that home to be empty when the run ends.** A sweep for writes
//     outside an allowed root is the tempting alternative and it does not
//     work: it has to exempt $TMPDIR, which is where t.TempDir and every
//     scratch file a suite makes already live, so it passes while the hole is
//     wide open.
//
// It lives here rather than in one suite's own file because it was written
// twice already — once in driver and once in repl, as near-copies — and the
// third package that needed it is the one that found #1984 by failing on a
// developer's machine with an rc file in it. A guard that has to be copied to
// reach a package is a guard the next package will not have.
package testenv

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TripwireHome names the home a suite runs under, and doubles as the mark that
// says this process had its environment assembled by Run.
//
// The second job is what makes it worth a name. Several suites re-execute
// their own test binary to *be* the shell — the only way to ask what happens
// to a process that gets killed — and the environment such a child is handed
// is the assertion that test is making. So a child takes what it was given
// untouched, and this name arriving already set is how it knows it is one.
//
// SH_TEST_-prefixed because the assembly below keeps that prefix, which is the
// channel every re-executing test in this module already uses.
const TripwireHome = "SH_TEST_HOME"

// keptExactly and keptPrefixes are the whole of what a suite inherits from
// whoever ran `go test`.
//
// PATH is here for the reason internal/smoke keeps it: a shell that cannot
// find `sleep` is not testing job control. TMPDIR is here because t.TempDir
// reads it, and the GO prefix because the test binary's own machinery does —
// GOCOVERDIR, GORACE, GOTRACEBACK. SH_TEST_ is the re-execution channel: a
// child that lost it would run the test again instead of becoming the shell
// the test needs.
var (
	keptExactly  = []string{"PATH", "TMPDIR"}
	keptPrefixes = []string{"GO", "SH_TEST_"}
)

// Assembled reports whether this process is already running inside an
// environment Run built — which is what a re-executed copy of a test binary
// is. Such a process leaves the environment alone: its parent aimed it.
func Assembled() bool { return os.Getenv(TripwireHome) != "" }

// Run assembles the environment, runs m in it, and then checks that the home
// it handed out is still untouched. suite names the package in the messages,
// since the person reading one will be reading it from a `go test ./...` line
// that says nothing about which suite tripped.
func Run(suite string, m interface{ Run() int }) int {
	home, err := os.MkdirTemp("", "sh-"+strings.ReplaceAll(suite, "/", "-")+"-home-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s tests: no scratch home: %v\n", suite, err)
		return 1
	}
	if err := assemble(home); err != nil {
		fmt.Fprintf(os.Stderr, "%s tests: %v\n", suite, err)
		return 1
	}
	code := m.Run()
	// Reported before the removal, and the removal happens either way: a
	// leaked scratch directory is a worse second failure than the first.
	left := EntriesUnder(home)
	if err := os.RemoveAll(home); err != nil {
		fmt.Fprintf(os.Stderr, "%s tests: scratch home not removed: %v\n", suite, err)
	}
	if len(left) > 0 {
		fmt.Fprintf(os.Stderr, "%s", HomeTouchedMessage(suite, left))
		return 1
	}
	return code
}

// assemble replaces the process environment with one built from nothing.
func assemble(home string) error {
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
		// Where every $HOME-derived path lands, and where the check at the
		// end of the run looks.
		"HOME": home,
		// The same name, for a child that is handed a different HOME by the
		// test that starts it. See TripwireHome.
		TripwireHome: home,
		// An empty HISTFILE is the shell's own way of being told *do not
		// remember this session*, and it turns off both halves of the record:
		// the line file and the block store, which reads it first for exactly
		// this reason (internal/blocks.DirFrom). Set rather than removed,
		// because removing it means "use the default", and the default is a
		// file under the home.
		//
		// A test that is about history says so by setting its own.
		"HISTFILE": "",
		// A prompt is drawn into a pipe or a pseudo-terminal a suite made,
		// never a person's terminal, and a developer's own TERM would decide
		// whether it is drawn with color in it.
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

// Scratch gives one test a home of its own, and a history file under it.
//
// Both, because HISTFILE outranks HOME: a test that sets only the home still
// writes its history wherever the environment already pointed, and a test that
// sets only the history file still appends to the block store under the home.
// That pair is the whole of #890, and a helper that did one of them would have
// left half of it standing.
//
// Returned so a test can look at what the session left there, which is the
// other reason to want one.
func Scratch(t *testing.T) string {
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

// EntriesUnder lists everything under a home, relative to it.
//
// A named directory that has to stay empty, rather than a sweep for files
// written outside some allowed root — see the package comment for why the
// sweep does not work.
//
// Directories count, not only files. A shell that created ~/.local/state and
// wrote nothing into it still reached into a person's home.
func EntriesUnder(home string) []string {
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

// HomeTouchedMessage says what was found and what to do about it, because the
// person who trips this will be reading it a year from now with no memory of
// #890.
func HomeTouchedMessage(suite string, left []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s tests: something wrote into the home directory.\n\n", suite)
	b.WriteString("These appeared under the suite's scratch $HOME:\n")
	for _, rel := range left {
		fmt.Fprintf(&b, "\t%s\n", rel)
	}
	b.WriteString(`
Outside this guard that $HOME is a person's own, so this is a test writing
into a real home directory: history, a block store, a startup file. Give the
test a home of its own with testenv.Scratch(t), and if it builds a Runner by
hand, an explicit HISTFILE under that home too — HISTFILE outranks HOME, so
setting the home alone does not move the history file. See #890.
`)
	return b.String()
}
