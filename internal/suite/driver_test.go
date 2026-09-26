// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The three mechanisms the ksh93 column needed, and the one measurement that
// says why each is a field rather than a shrug: without any one of them, that
// column grades a **foreign shell** at `strict 1/1, 100.0%`. Measured in the
// pinned image with `dash` in the graded slot, which is the check #4440 asks
// for before any of the column's figures count.

// TestADriversOwnOptionsAreNotTheShells. `--posix` to ksh93's shtests is one
// of three passes to run; to a shell it is a script called `--posix.sh`,
// which is the diagnostic the first attempt produced.
func TestADriversOwnOptionsAreNotTheShells(t *testing.T) {
	s := Suite{Driver: "shtests", DriverArgs: []string{"-f"}, DriverFlags: []string{"--posix"}}
	got := strings.Join(argv(s, "case.sh"), " ")
	if want := "-f ./shtests --posix ./case.sh"; got != want {
		t.Errorf("argv is %q, want %q", got, want)
	}
	// A column with neither list is untouched, and one with no driver at all
	// runs the file: the other four columns are both of those.
	if got := strings.Join(argv(Suite{}, "f.tests"), " "); got != "./f.tests" {
		t.Errorf("a column with no driver builds %q", got)
	}
	if got := strings.Join(argv(Suite{Driver: "d"}, "f"), " "); got != "./d ./f" {
		t.Errorf("a driver with no options builds %q", got)
	}
}

// TestADriverThatIsTheReferencesRunsUnderTheReference, while the graded shell
// is still the one the environment names.
//
// This inverts the usual shape, and the control below is the point: with the
// flag off, the two runs start two different binaries, which is what every
// other column does.
func TestADriverThatIsTheReferencesRunsUnderTheReference(t *testing.T) {
	ref, ours := "/bin/ksh", "/build/ksh"

	inverted := Suite{Driver: "shtests", DriverRunsUnderTheReference: true}
	if got := inverted.under(ours, ref); got != ref {
		t.Errorf("the graded run started %q, want the reference %q", got, ref)
	}
	if got := inverted.under(ref, ref); got != ref {
		t.Errorf("the reference run started %q, want %q", got, ref)
	}
	// With no reference to hand — our own columns, and the tier checks —
	// nothing is inverted, because there is nothing to invert to.
	if got := inverted.under(ours, ""); got != ours {
		t.Errorf("with no reference the run started %q, want %q", got, ours)
	}

	usual := Suite{Driver: "ztst.zsh"}
	if got := usual.under(ours, ref); got != ours {
		t.Errorf("an ordinary driver column started %q, want the graded shell %q", got, ours)
	}

	// And the half that makes the inversion measure anything at all: the
	// environment names the shell being graded, never the one being started.
	// If this ever stopped being true the column would grade the reference
	// against itself and report every file perfect.
	env := environ(Suite{ShellVar: "SHELL"}, t.TempDir(), ours)
	if !contains(env, "SHELL="+ours) {
		t.Errorf("the environment does not name the graded shell: %v", env)
	}
}

// TestADriversOwnBookkeepingIsNotEvidence. A line that differs between two
// runs of the *same* shell says nothing about either, and a column whose
// every file carries one is a column where every file comes back unstable and
// the report prints `0 differing lines` over nothing.
func TestADriversOwnBookkeepingIsNotEvidence(t *testing.T) {
	s := Suite{Noise: []Noise{
		{regexp.MustCompile(`\d{4}-\d{2}-\d{2}\+\d{2}:\d{2}:\d{2}`), "<time>"},
		{regexp.MustCompile(`(?m)^(main|tests):\s+\d+m\d+\.\d+s\s+\d+m\d+\.\d+s$`), "$1: <cpu>"},
	}}
	first := "test case begins at 2026-09-26+22:11:43\nmain:      0m00.002s    0m00.000s\n"
	again := "test case begins at 2026-09-26+22:19:07\nmain:      0m00.000s    0m00.001s\n"
	if normalize(s, first, "/bin/ksh", "") != normalize(s, again, "/bin/ksh", "") {
		t.Errorf("two runs of one shell still differ:\n%q\n%q",
			normalize(s, first, "/bin/ksh", ""), normalize(s, again, "/bin/ksh", ""))
	}

	// The positive control, and it is the half that matters: folding the
	// clock must not fold the answers. A column that normalized its way to
	// agreement would be the thing this whole instrument exists to catch.
	mine := "test case begins at 2026-09-26+22:11:43\n\tcase.sh[77]: FAIL: empty case list\nmain:      0m00.002s    0m00.000s\n"
	if normalize(s, mine, "/bin/ksh", "") == normalize(s, first, "/bin/ksh", "") {
		t.Error("a FAIL line was folded away with the timestamps")
	}

	// And a column that declares none is untouched, which is the other four.
	if got := normalize(Suite{}, first, "/bin/ksh", ""); got != first {
		t.Errorf("a column with no Noise had its output rewritten: %q", got)
	}
}

// TestAFileTheDriverNeedsFromOutsideTheTestDirectoryIsPlaced.
//
// A run copies the test directory and nothing else. ksh93's shtests sources
// `../SHOPT.sh`, one level above it, and without the file the driver stops
// before a single assertion — under *both* shells, at the same line, which is
// why the column scored a foreign shell perfect until this existed.
func TestAFileTheDriverNeedsFromOutsideTheTestDirectoryIsPlaced(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src/cmd/ksh93"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src/cmd/ksh93/SHOPT.sh"), []byte("SHOPT_X=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	owned := t.TempDir()
	run := filepath.Join(owned, "t")
	if err := os.MkdirAll(run, 0o700); err != nil {
		t.Fatal(err)
	}
	s := Suite{Name: "ksh93", Beside: []Beside{{At: "../SHOPT.sh", From: "src/cmd/ksh93/SHOPT.sh"}}}
	if err := placeBeside(s, root, run); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(owned, "SHOPT.sh"))
	if err != nil {
		t.Fatalf("the driver's file is not where it looks for it: %v", err)
	}
	if string(got) != "SHOPT_X=1\n" {
		t.Errorf("placed %q", got)
	}

	// One level out is the run's own temporary directory and is this run's to
	// write in. Two is somebody else's.
	away := Suite{Name: "ksh93", Beside: []Beside{{At: "../../escaped", From: "src/cmd/ksh93/SHOPT.sh"}}}
	if err := placeBeside(away, root, run); err == nil {
		t.Error("a Beside that climbs out of the run's own directory was allowed")
	}

	// A column with none does nothing at all, which is the other four.
	if err := placeBeside(Suite{}, root, run); err != nil {
		t.Errorf("a column with no Beside failed: %v", err)
	}
}

// TestTheKsh93ColumnCarriesWhatMadeItDiscriminate.
//
// Each of these was added because the column did not have it and reported
// `strict 1/1, 100.0%` for `dash` — a foreign shell graded perfect, which is
// what "nothing was measured" looks like from outside. The shape is pinned
// here so that removing one is a failing test rather than a column that goes
// quietly green.
func TestTheKsh93ColumnCarriesWhatMadeItDiscriminate(t *testing.T) {
	s, ok := Find("ksh")
	if !ok {
		t.Fatal("no ksh column")
	}
	if s.NotYet != "" {
		t.Fatalf("the ksh93 column is not built: %s", s.NotYet)
	}
	switch {
	case s.Driver == "":
		t.Error("no driver: the files refuse to run outside shtests")
	case len(s.DriverFlags) == 0:
		t.Error("no driver flags: the default runs a shcomp pass this fetch does not unpack")
	case !s.DriverRunsUnderTheReference:
		t.Error("the driver would run under the graded shell, which cannot read it")
	case s.ShellVar == "":
		t.Error("nothing names the graded shell, so both runs would measure the reference")
	case len(s.Beside) == 0:
		t.Error("SHOPT.sh is not placed, so the driver stops identically under both shells")
	case len(s.Noise) == 0:
		t.Error("no Noise: the driver's clock makes every file unstable")
	}
	// The whole release and not the lineage, for #3135's reason: `93u+` is a
	// prefix of `93u+m/1.0.8`.
	if s.AgainstReport == "93u+" || !strings.Contains(s.AgainstReport, "1.0.8") {
		t.Errorf("AgainstReport is %q; a fragment a different lineage clears is not a gate", s.AgainstReport)
	}
}
