// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The hole #3148 records, in the only shape that can still reach it: a shell
// answering neither `--version` nor `${.sh.version}`, whose `--help` exits
// **zero** with usage on the first line.
//
// The two guards that were already here cannot see this one. The exit-status
// check passes — the shell succeeded — and asking `${.sh.version}` first only
// helps the shells that answer it. BusyBox is why `--help` is asked at all and
// BusyBox leads with its version; this is the next shell that does not.
//
// A usage line reads like a build at a glance, which is what made the same
// shape survive in internal/suite for as long as that job existed (#3135).
func TestAUsageLineIsNotRecordedAsABuild(t *testing.T) {
	shell := versionProbeShell(t, "--version) exit 2 ;;\n"+
		"-c) exit 2 ;;\n"+
		"--help) echo 'Usage: /opt/fakesh-2.1/bin/sh [ options ] [arg ...]' ;;\n"+
		"*) exit 2 ;;")
	if got := Version(context.Background(), shell); got != "unknown" {
		t.Fatalf("a usage line was recorded as this shell's build: %q", got)
	}
}

// The other half of the rule, and the one the usage prefix cannot reach: a
// refusal that is not spelled `usage:` at all.
//
// A blocklist of the refusals seen so far is beaten by the first one nobody
// has met, so the rule is about what an *answer* has — a release number or
// the date of one. This line has neither and is not a usage line either, so
// only that half of the rule refuses it.
func TestARefusalWithNoBuildNumberIsNotABuildString(t *testing.T) {
	shell := versionProbeShell(t, "--version) exit 2 ;;\n"+
		"-c) exit 2 ;;\n"+
		"--help) echo 'fakesh: unrecognized option' ;;\n"+
		"*) exit 2 ;;")
	if got := Version(context.Background(), shell); got != "unknown" {
		t.Fatalf("a refusal with no build number in it was recorded as this shell's build: %q", got)
	}
}

// The gate makes a refusing probe fall *through* rather than end the loop,
// which is the half a rule applied only to the last spelling would lose.
//
// Written as a shell that refuses the first spelling by succeeding at it —
// `usage: …` on a zero exit — and identifies itself on the last. Without the
// gate the loop stops at the first, and the recorded build is the refusal.
func TestARefusedProbeFallsThroughToTheNextSpelling(t *testing.T) {
	shell := versionProbeShell(t, "--version) echo 'usage: fakesh-2.1 [-x] [file]' ;;\n"+
		"-c) exit 2 ;;\n"+
		"--help) echo 'FakeSh v2.1.0 (2026-09-18) multi-call binary.' ;;\n"+
		"*) exit 2 ;;")
	got := Version(context.Background(), shell)
	if !strings.Contains(got, "v2.1.0") {
		t.Fatalf("the shell identified itself on a later spelling and the record took the earlier refusal: %q", got)
	}
}

// The gate can only ever be too strict, and too strict has a cost: a column
// that could have been identified records `unknown` instead. So it is held
// against the record it has to leave alone — every build string the panel has
// ever reported passes it, which is what says folding the rule into the probe
// re-records to an identical file.
//
// `unknown` is the sentinel for a shell that answered nothing and is excluded
// by name rather than by the rule, which would otherwise pass it silently.
func TestEveryRecordedBuildStringPassesTheGate(t *testing.T) {
	golden, err := Load(filepath.Join("testdata", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(golden.Shells) == 0 {
		t.Fatal("the record names no shells, so this check cannot fail and is not one")
	}
	for _, s := range golden.Shells {
		if s.Version == "unknown" {
			continue
		}
		if !LooksLikeABuild(s.Version) {
			t.Errorf("%s: the gate would turn a recorded build into `unknown`: %q", s.Name, s.Version)
		}
	}
}

// versionProbeShell writes an executable that answers the version probes the
// way a named shell does, and nothing else. It is how a refusal can be tested
// without depending on which shells this machine has — and the refusals that
// matter here are ones no shell on this machine gives.
func versionProbeShell(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-version-shell")
	if err := os.WriteFile(path, []byte("#!/bin/sh\ncase \"$1\" in\n"+body+"\nesac\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
