// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// The report said for months that a refused parse forfeited a whole file.
// Both halves of that were false and this file is the pair of measurements
// that say so, taken on every run rather than written down once (#2381).

// TestARefusedStaticReadForfeitsNothing is the first half.
//
// The static whole-file read is recorded and then the file is run anyway, by
// both shells, and scored like any other. A grader that returned early on a
// refusal would report the file as absent instead of as evidence — which is
// what the header claimed was happening.
func TestARefusedStaticReadForfeitsNothing(t *testing.T) {
	tests := testDir(t, map[string]string{"bad.tests": "echo a\necho 'unterminated\n"})
	res := gradeFile(t, tests, "bad.tests", "/bin/sh", "/bin/sh")
	if res.Parsed {
		t.Fatal("a file with an unterminated quote passed a whole-file read")
	}
	if !res.Scored {
		t.Fatal("a refused file was not scored; the refusal forfeited the file's evidence")
	}
	if !res.Strict {
		t.Errorf("a shell graded against itself on a refused file is not strict: %+v", res)
	}
	if res.Longest == 0 {
		t.Error("a refused file contributed no lines to the agreement")
	}
}

// TestTheRefusedBandIsMeasuredRatherThanAsserted is the same fact one level
// up, where the report reads it.
//
// What a refusal costs is a number this instrument already holds, so it is
// aggregated rather than described. A run whose refused files score as well
// as its read ones is a run where the static read explains nothing about the
// other two columns, and that has to be visible in the report rather than
// contradicted by it.
func TestTheRefusedBandIsMeasuredRatherThanAsserted(t *testing.T) {
	tests := testDir(t, map[string]string{
		"bad.tests": "echo a\necho 'unterminated\n",
		"ok.tests":  "echo fine\n",
	})
	s := Suite{Name: "bash", Dialect: "bash", TestDir: "tests", Ext: ".tests", ShellVar: "THIS_SH"}
	rep, err := Sweep(context.Background(), s, filepath.Dir(tests), "/bin/sh", "/bin/sh",
		Options{Timeout: 10 * time.Second, Jobs: 2})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 2 || rep.Parsed != 1 {
		t.Fatalf("files=%d parsed=%d, want 2 and 1", rep.Files, rep.Parsed)
	}
	if rep.Refused.Files != 1 {
		t.Fatalf("the refused band holds %d files, want 1", rep.Refused.Files)
	}
	if rep.Refused.Scored != 1 || rep.Refused.Strict != 1 {
		t.Errorf("the refused band scored %d/%d strict, want 1/1 — the refused file ran",
			rep.Refused.Strict, rep.Refused.Scored)
	}
	if rep.Refused.LineRate() != 1 {
		t.Errorf("the refused band's line agreement is %v, want 1", rep.Refused.LineRate())
	}
	if rep.Scored != 2 || rep.Strict != 2 {
		t.Errorf("the whole population scored %d/%d strict, want 2/2 — a refused file is "+
			"in the figures as well as in the band", rep.Strict, rep.Scored)
	}
}

// TestTheReferenceStaticReadTellsAGapFromAnImpossibility is the second half,
// and it is a measurement of one binary contradicting itself on one file.
//
// `shopt -s extglob` is decided at run time and a static read has no run
// time, so the reference refuses its own construct while running the same
// file to completion at status 0. No parser change moves that file into the
// read column, so ranking it beside a construct this parser genuinely cannot
// read would send somebody to close a gap nobody can close.
func TestTheReferenceStaticReadTellsAGapFromAnImpossibility(t *testing.T) {
	s, ok := Find("bash")
	if !ok {
		t.Fatal("no bash column")
	}
	reference, found := Locate(s.Lookup)
	if !found {
		t.Skip("no bash on this machine to be the oracle")
	}
	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	valid := write("valid.sh", "echo a\n")
	broken := write("broken.sh", "echo a\necho 'unterminated\n")
	runtimeOption := write("extglob.sh", "shopt -s extglob\ncase x in @(a|b)) echo hi;; esac\n")

	ctx := context.Background()
	if !staticParse(ctx, reference, valid, 10*time.Second) {
		t.Error("the reference refused a static read of a file it has no quarrel with; " +
			"a discriminator that always says no discriminates nothing")
	}
	if staticParse(ctx, reference, broken, 10*time.Second) {
		t.Error("the reference accepted a static read of an unterminated quote")
	}
	if staticParse(ctx, reference, runtimeOption, 10*time.Second) {
		t.Error("the reference read a run-time option's construct statically; if it can, " +
			"then so should we, and this is a gap rather than an impossibility")
	}
	// The same binary, the same file, run rather than read.
	cmd := exec.Command(reference, runtimeOption)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	if err := cmd.Run(); err != nil {
		t.Fatalf("the reference will not run the file it refuses to read: %v — "+
			"without that contradiction this classification measures nothing", err)
	}

	// And the classification the grader draws from it.
	tests := testDir(t, map[string]string{
		"eg.tests": "shopt -s extglob\ncase x in @(a|b)) echo hi;; esac\n",
	})
	res := gradeFile(t, tests, "eg.tests", reference, reference)
	if res.Parsed {
		t.Skip("this parser now reads the construct statically; the case needs another one")
	}
	if res.ReferenceRead {
		t.Error("a file no static read reaches was recorded as one the reference read")
	}
}
