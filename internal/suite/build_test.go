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
// three whose references do report a build have to name one. dash is not in
// this loop because its reference reports no build at all — it is identified
// by measurement instead, and [TestAProbeColumnNamesTheAnswerItExpects] is
// the same rule for that route. ash is absent because its image is pinned by
// digest, so the build is pinned with it.
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

// fingerprintShell answers no version probe at all — the ksh `-c` spelling
// included, which is the one a naive fake would answer by accident — and
// answers any other `-c` with the line given. That is the shape of a shell
// [Suite.AgainstProbe] exists for.
func fingerprintShell(t *testing.T, line string) string {
	t.Helper()
	return fakeShell(t, t.TempDir(), "fake",
		"case \"$1\" in\n"+
			"-c) case \"$2\" in *sh.version*) exit 2 ;; *) echo '"+line+"' ;; esac ;;\n"+
			"*) echo 'fake: 0: Illegal option --' >&2 ; exit 2 ;;\n"+
			"esac")
}

// A shell that will not say what it is can still be identified, and the dash
// column is the one that had to be. Before this, [Version] was the only route
// and dash reached none of it, so `Against` was left empty at its entry and
// the column carried no notice on any runner — which is how #2291 came to
// read its CI gap as unexcused when both files in it are the distribution's
// patch.
func TestAReferenceThatNamesNoVersionIsIdentifiedByMeasurement(t *testing.T) {
	s, ok := FindOurs("dash")
	if !ok {
		t.Fatal("there is no native dash column")
	}
	unpatched := fingerprintShell(t, s.AgainstReport)
	got := s.Identify(context.Background(), unpatched)
	if !got.Known {
		t.Fatalf("the probe answered and the column still could not tell: %+v", got)
	}
	if !got.ByProbe {
		t.Fatal("a fingerprint was reported as though the shell had named itself")
	}
	if !strings.Contains(got.String(), "answers no version probe") {
		t.Fatalf("the header does not say this is a measurement: %q", got.String())
	}
	if label, why := s.Lineage(got); why != "" {
		t.Fatalf("the build this column is written against was reported as wrong: %q %q", label, why)
	}
}

// The half that has to fire, and the reason the probe asks two questions
// rather than one. Debian and Ubuntu patch dash's printf escape and its
// option table; either answer alone identifies the patched build.
func TestAPatchedReferenceIsReportedAsADifferentBuild(t *testing.T) {
	s, ok := FindOurs("dash")
	if !ok {
		t.Fatal("there is no native dash column")
	}
	for _, line := range []string{
		"esc=3 pipefail-listed=y",
		"esc=3 pipefail-listed=n",
		"esc=4 pipefail-listed=y",
	} {
		got := s.Identify(context.Background(), fingerprintShell(t, line))
		label, why := s.Lineage(got)
		if label != "WRONG BUILD" {
			t.Fatalf("%q was accepted as the build this column is graded against: %q %q", line, label, why)
		}
		if !strings.Contains(why, line) || !strings.Contains(why, s.Against) {
			t.Fatalf("the notice does not name both builds, so a reader cannot act on it: %q", why)
		}
	}
}

// [looksLikeABuild]'s rule carried into the probe: an answer has to look like
// an answer. A shell that refuses the fingerprint, or writes it to standard
// error, or exits non-zero having printed it, has identified nothing — and
// the column must say so rather than print the refusal where a build belongs,
// which is #3135 in the new mechanism.
func TestARefusedFingerprintIsNotAnIdentification(t *testing.T) {
	s, ok := FindOurs("dash")
	if !ok {
		t.Fatal("there is no native dash column")
	}
	for name, body := range map[string]string{
		"refuses every spelling":    "echo 'fake: 0: Illegal option --' >&2 ; exit 2",
		"answers on standard error": "echo 'esc=4 pipefail-listed=n' >&2 ; exit 0",
		"answers and then fails":    "echo 'esc=4 pipefail-listed=n' ; exit 1",
		"answers with nothing":      "exit 0",
	} {
		shell := fakeShell(t, t.TempDir(), "fake", body)
		if got := s.Identify(context.Background(), shell); got.Known {
			t.Fatalf("%s: was taken as an identification: %q", name, got.Version)
		}
		if label, _ := s.Lineage(s.Identify(context.Background(), shell)); label != "UNIDENTIFIED" {
			t.Fatalf("%s: an unidentified reference passed silently: %q", name, label)
		}
	}
}

// A column that names a probe has to name what the probe should answer, or
// the comparison can never match and the notice fires on every machine
// including the right one.
func TestAProbeColumnNamesTheAnswerItExpects(t *testing.T) {
	for _, s := range Ours {
		if s.AgainstProbe == "" {
			continue
		}
		if s.Against == "" || s.AgainstReport == "" {
			t.Fatalf("the %s column probes for a build it does not name", s.Name)
		}
		if s.AgainstReport != strings.ToLower(s.AgainstReport) {
			t.Fatalf("the %s column's fragment %q is not lowercase, and the comparison "+
				"lowercases the answer — so it can never match", s.Name, s.AgainstReport)
		}
	}
}

// The live guard, and the one that would catch the probe going stale: the
// dash on this machine has to answer the fingerprint this column is written
// against. Measured 2026-09-16 on Apple's dash-16 and on upstream 0.5.12
// built from source here — both `esc=4 pipefail-listed=n`, and every one of
// the column's 52 files byte-identical between them.
func TestTheDashOnThisMachineAnswersTheFingerprint(t *testing.T) {
	s, ok := FindOurs("dash")
	if !ok {
		t.Fatal("there is no native dash column")
	}
	path, found := Locate(s.Lookup)
	if !found {
		t.Skip("no dash on this machine")
	}
	got := s.Identify(context.Background(), path)
	if !got.Known {
		t.Fatalf("%s answered neither a version probe nor the fingerprint", path)
	}
	if !got.ByProbe {
		t.Fatalf("%s named a build after all, so the probe is no longer the route: %q", path, got.Version)
	}
	t.Logf("%s answers %q", path, got.Version)
	// Not asserted equal to AgainstReport: a Debian-family runner answers
	// the other fingerprint and that is the notice working, not a failing
	// test. What is asserted is that the two spellings are the only ones the
	// probe can produce, so a probe that has stopped discriminating — a
	// changed builtin, a reworded listing — is caught here rather than by a
	// column that silently agrees with everything.
	if !strings.HasPrefix(got.Version, "esc=") || !strings.Contains(got.Version, " pipefail-listed=") {
		t.Fatalf("%s answered %q, which is not this probe's shape at all", path, got.Version)
	}
}

// A reference that answers no version probe can only be confirmed by its
// column's own fingerprint, so the confirmation has to be taken from
// [Suite.Identify] and never from [Version].
//
// Both halves are asserted, because either alone proves nothing: Version's
// answer has to fail the check and Identify's has to clear it. The first half
// was live rather than hypothetical. `internal/cmd/suiteinside` called Version
// for as long as ash was the only contained column — and ash answers a version
// probe, so nothing in the tree could see the difference. Gating a column that
// answers none would have refused it at the door, before a file was run, with
// "it answers no version probe" against a shell that had just identified
// itself by measurement (#3480).
func TestAReferenceIdentifiedOnlyByMeasurementIsBelievable(t *testing.T) {
	s, ok := FindOurs("dash")
	if !ok {
		t.Fatal("there is no native dash column")
	}
	if s.MustReport == "" {
		t.Fatal("the column names nothing its reference must report, so this proves nothing")
	}
	unpatched := fingerprintShell(t, s.AgainstReport)
	if err := s.Believable(Version(context.Background(), unpatched)); err == nil {
		t.Error("a reference identified only by measurement was believed from a version probe alone")
	}
	if err := s.Believable(s.Identify(context.Background(), unpatched)); err != nil {
		t.Errorf("the fingerprint this column identifies by does not clear its own MustReport: %v", err)
	}

	// And the check still refuses the wrong build, which is the half that
	// makes the one above a check rather than a formality.
	patched := fingerprintShell(t, "esc=3 pipefail-listed=y")
	if err := s.Believable(s.Identify(context.Background(), patched)); err == nil {
		t.Error("a reference answering a different fingerprint was believed")
	}
}
