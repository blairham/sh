// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"strings"
	"testing"
)

// probeShell writes a program that answers the version probes the way a named
// shell does, and nothing else. It is how the refusals can be tested without
// depending on which shells this machine has.
func probeShell(t *testing.T, body string) string {
	t.Helper()
	return fakeShell(t, t.TempDir(), "fake", "case \"$1\" in\n"+body+"\nesac")
}

// The whole of #3135 in one assertion: a shell that refuses the probe by
// printing its usage must not have that usage printed as its build.
//
// This is the exact output AT&T ksh93 gives — `--version` exits 2, `--help`
// exits zero with a usage line — and it was reported as this column's
// reference version on every run until now.
func TestAUsageLineIsNotABuildString(t *testing.T) {
	shell := probeShell(t, "--version) exit 2 ;;\n"+
		"--help) echo 'Usage: ksh [ options ] [arg ...]' ;;\n"+
		"*) exit 2 ;;")
	got := Version(context.Background(), shell)
	if got.Known {
		t.Fatalf("a usage line was accepted as a build string: %q", got.Version)
	}
	if strings.Contains(got.String(), "Usage:") {
		t.Fatalf("the report would print the refusal where a build belongs: %q", got.String())
	}
}

// The other half of the same probe: an error message is not a build string
// either. dash answers `--help` with `/bin/dash: 0: Illegal option --` on a
// non-zero exit, and this package's old probe took any non-empty output.
func TestADiagnosticIsNotABuildString(t *testing.T) {
	shell := probeShell(t, "*) echo 'fake: 0: Illegal option --' >&2 ; exit 2 ;;")
	if got := Version(context.Background(), shell); got.Known {
		t.Fatalf("a refusal was accepted as a build string: %q", got.Version)
	}
}

// ksh93 answers `${.sh.version}` and nothing else, which internal/oracle has
// asked since #1032 and this package's copy of that probe never did. Folding
// the two is what makes the ksh column report a build at all.
func TestTheKshSpellingIsAsked(t *testing.T) {
	shell := probeShell(t, "--version) exit 2 ;;\n"+
		"-c) echo 'Version AJM 93u+m/1.0.8 2024-01-01' ;;\n"+
		"--help) echo 'Usage: ksh [ options ] [arg ...]' ;;")
	got := Version(context.Background(), shell)
	if !got.Known || !strings.Contains(got.Version, "93u+m") {
		t.Fatalf("the ksh spelling was not asked, or its answer was refused: %+v", got)
	}
}

// The lineage the whole issue is about. A fragment that cannot tell AT&T's
// 93u+ from the fork's 93u+m clears the fork, which is the check reporting
// all-clear on the one case it exists for.
func TestADifferentLineageIsReportedAsOne(t *testing.T) {
	s, ok := FindOurs("ksh")
	if !ok {
		t.Fatal("there is no native ksh column")
	}
	label, why := s.Lineage(Build{Version: "Version AJM 93u+m/1.0.8 2024-01-01", Known: true})
	if label != "WRONG BUILD" {
		t.Fatalf("ksh93u+m was accepted as the build this column is graded against: %q %q", label, why)
	}
	if !strings.Contains(why, "93u+m/1.0.8") || !strings.Contains(why, s.Against) {
		t.Fatalf("the notice does not name both builds, so a reader cannot act on it: %q", why)
	}
	if label, why := s.Lineage(Build{Version: "Version AJM 93u+ 2012-08-01", Known: true}); why != "" {
		t.Fatalf("the build this column is written against was reported as wrong: %q %q", label, why)
	}
	// A shell that would not say what it is is not a clean bill of health.
	if label, _ := s.Lineage(Build{}); label != "UNIDENTIFIED" {
		t.Fatalf("an unidentified reference passed silently: %q", label)
	}
}

// A column that names no expected build cannot report a mismatch, so the
// three whose references do report a build have to name one. The two that are
// absent are absent for reasons written at their entries: dash answers no
// probe in any spelling, and ash's image is pinned by digest.
func TestTheColumnsWhoseReferenceReportsABuildNameIt(t *testing.T) {
	for _, dialect := range []string{"bash", "zsh", "ksh"} {
		s, ok := FindOurs(dialect)
		if !ok {
			t.Fatalf("there is no native %s column", dialect)
		}
		if s.Against == "" || s.AgainstReport == "" {
			t.Fatalf("the %s column names no build it is graded against, so a runner "+
				"grading it against a different one would say nothing", s.Name)
		}
		if s.AgainstReport != strings.ToLower(s.AgainstReport) {
			t.Fatalf("the %s column's fragment %q is not lowercase, and the comparison "+
				"lowercases the version — so it can never match", s.Name, s.AgainstReport)
		}
	}
}

// The live regression guard, and the reason it is worth having beyond the
// fakes: whatever ksh this machine has, the report must print a build for it
// rather than a usage line. On macOS that is AT&T's 93u+ and on the CI runner
// it is ksh93u+m; both carry `93u+`, and neither usage block does.
func TestTheKshOnThisMachineReportsABuild(t *testing.T) {
	s, ok := FindOurs("ksh")
	if !ok {
		t.Fatal("there is no native ksh column")
	}
	path, found := Locate(s.Lookup)
	if !found {
		t.Skip("no ksh93 on this machine")
	}
	got := Version(context.Background(), path)
	if !got.Known {
		t.Fatalf("%s answered no version probe", path)
	}
	if !strings.Contains(got.Version, "93u+") {
		t.Fatalf("%s reported %q, which is not a ksh93 build string", path, got.Version)
	}
}
