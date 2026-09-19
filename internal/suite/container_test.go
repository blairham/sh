// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/oracle"
)

// Nothing here starts a container. Every one of these is a question about the
// table rather than about a machine, which is the point: the failures they
// guard against are silent ones that a run on a developer's laptop would not
// notice either.

func TestAContainedColumnNamesARouteThePanelHas(t *testing.T) {
	for _, s := range Ours {
		if s.Container == "" {
			continue
		}
		if _, ok := oracle.Container(s.Container); !ok {
			t.Fatalf("the %s column is reached through the oracle panel member %q, "+
				"and that member takes no container route — so this column has a second "+
				"account of how to get to the shell and the two will drift",
				s.Name, s.Container)
		}
	}
}

// A tag moves, and a column pinned to one would report a shell that changed
// with nothing able to tell that from a shell that behaved differently.
// `bash:5.3` moved twice in the two days the bash column was being argued
// about, which is that abstraction made concrete.
//
// Both spellings are asked the same question here, and that is the point of
// asking it in one place: the oracle's own guard covers the panel members,
// and a column carrying its own pin is reached by nothing the oracle checks.
func TestAContainedColumnIsPinnedByDigest(t *testing.T) {
	for _, s := range Ours {
		if !s.Contained() {
			continue
		}
		reach, err := s.Reach()
		if err != nil {
			t.Fatalf("the %s column says it is contained and names no route: %v", s.Name, err)
		}
		ref := reach.Ref()
		digest, ok := strings.CutPrefix(ref[strings.LastIndex(ref, "@")+1:], "sha256:")
		if !ok || len(digest) != 64 {
			t.Fatalf("the %s column's image %q is not pinned by a sha256 digest: a tag "+
				"moves, and a column that moved underneath itself would report a shell "+
				"that changed", s.Name, ref)
		}
	}
}

// A column that is not contained is already a reference on this machine, so
// CrossHere would be saying something about it that is not a question. The
// field exists only to tell two contained columns apart — the shell is also
// here, or it is not — and a value on a column the distinction cannot reach
// reads as a claim it does not make.
func TestCrossHereIsOnlyForAContainedColumn(t *testing.T) {
	for _, s := range Ours {
		if s.CrossHere && !s.Contained() {
			t.Fatalf("the %s column sets CrossHere and is graded against a binary on this "+
				"machine already, so the field claims something it cannot mean", s.Name)
		}
	}
}

// The #3480 rule, and the half a table guard can hold: a contained column
// contributes a cross-check reference exactly when it says it has one, and
// never merely because a path exists.
//
// The distinction is load bearing rather than tidy. ash's Lookup holds
// /bin/busybox, which is on every Linux host and is a multi-call binary
// rather than a shell when it is run by that path — so a contained column
// that found its reference by looking would hand the cross-check a program
// that answers nothing this suite asks, under the name of a shell.
func TestOnlyAColumnThatSaysSoLendsACrossCheckReference(t *testing.T) {
	dir := t.TempDir()
	here := filepath.Join(dir, "shell")
	if err := os.WriteFile(here, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	contained := Suite{Name: "x", Image: "img", Digest: "sha256:" + strings.Repeat("0", 64), Lookup: []string{here}}

	if path, ok := contained.CrossHereReference(); ok {
		t.Fatalf("a contained column that does not say it has a reference here lent %q anyway", path)
	}
	lending := contained
	lending.CrossHere = true
	path, ok := lending.CrossHereReference()
	if !ok {
		t.Fatal("a contained column that says the shell is here as well lent nothing, so " +
			"containing it would quietly take a reference out of the cross-check")
	}
	if path != here {
		t.Fatalf("lent %q, wanted %q", path, here)
	}
	// And never from a column that is not contained: that column's own
	// grading reference is already in refs, and lending a second would count
	// one shell twice.
	local := Suite{Name: "x", Lookup: []string{here}, CrossHere: true}
	if _, ok := local.CrossHereReference(); ok {
		t.Fatal("a column that is not contained lent a second reference under its own name")
	}
}

// Reach is the one place the two spellings of "contained" are resolved, so a
// column that is neither has to say so rather than hand back a zero route
// that would pull whatever `@` resolves to.
func TestReachRefusesAColumnThatIsNotContained(t *testing.T) {
	if _, err := (Suite{Name: "x"}).Reach(); err == nil {
		t.Fatal("a column reached as a binary on this machine was given a container route")
	}
	if _, err := (Suite{Name: "x", Image: "img"}).Reach(); err == nil {
		t.Fatal("a column naming an image and no digest was given a route, which is a tag pin")
	}
}

// A contained column must not also carry NotYet. The two say opposite things
// about the same column — one is "this runs", the other is "this is a row
// with a reason in it" — and the printed report would take the second, so the
// column would vanish with the work already done.
func TestAContainedColumnIsNotAlsoUnbuilt(t *testing.T) {
	for _, s := range Ours {
		if s.Contained() && s.NotYet != "" {
			t.Fatalf("the %s column is reached through a container and still says it is "+
				"not yet a column: %s", s.Name, s.NotYet)
		}
	}
}

// A column reached by a path alone can silently be some other shell — /bin/sh
// is BusyBox on Alpine and dash on Debian — and nobody is going to notice by
// eye what a path inside an image resolved to.
func TestAContainedColumnSaysWhatItMustReport(t *testing.T) {
	for _, s := range Ours {
		if s.Contained() && s.MustReport == "" {
			t.Fatalf("the %s column is reached inside an image and names nothing its "+
				"reference's version string must contain, so a different shell at the "+
				"same path would record as this one", s.Name)
		}
	}
}

func TestBelievableRefusesTheWrongShell(t *testing.T) {
	s := Suite{Name: "ash", MustReport: "busybox"}
	if err := s.Believable(Build{Version: "BusyBox v1.37.0 (2026-01-10 15:38:28 UTC) multi-call binary.", Known: true}); err != nil {
		t.Fatalf("BusyBox's own version line was refused: %v", err)
	}
	err := s.Believable(Build{Version: "Debian Almquist Shell", Known: true})
	if err == nil {
		t.Fatal("a shell reporting something else was believed")
	}
	if !strings.Contains(err.Error(), "busybox") {
		t.Fatalf("the refusal does not say what was wanted: %v", err)
	}
	// A shell that would not identify itself cannot be cleared either, and
	// before #3135 it could be: the probe handed back whatever the refusal
	// printed, and `Usage: ksh [ options ]` carries the name.
	if err := s.Believable(Build{}); err == nil {
		t.Fatal("a shell that answered no version probe was believed")
	}
	// No MustReport is the four columns on this machine, where the path was
	// typed by a person and the shell answers nothing useful to --version.
	if err := (Suite{}).Believable(Build{Version: "anything at all", Known: true}); err != nil {
		t.Fatalf("a column naming no requirement refused a version: %v", err)
	}
}

// A tier is a claim and an empty directory would report a column that ran and
// agreed, so every tier the instrument names has to have cases in it.
func TestEveryTierHasCases(t *testing.T) {
	root := repoRoot(t)
	for _, tier := range Tiers {
		names, err := Files(filepath.Join(root, OurRoot, tier), OurExt)
		if err != nil {
			t.Fatalf("the %s tier is named and is not there: %v", tier, err)
		}
		if len(names) == 0 {
			t.Fatalf("the %s tier has no cases in it, so every column claiming it would "+
				"report that it ran and agreed", tier)
		}
	}
}

// A column may not claim a directory nothing checks the claim of, and the
// tier list may not hold one no column runs — either way a directory and a
// report would be saying different things.
//
// There are exactly two kinds of claimed directory and each has its own
// check. A shared tier is in [Tiers] and [CrossCheck] asks whether the
// references agree about it. A dialect tier is named for the column's own
// dialect and [OnlyHere] asks the opposite question. A third spelling would
// be graded — Dirs is Dirs — and neither check would ever find it, so its
// files would pass forever without the directory's claim being measured at
// all.
func TestEveryClaimedDirIsATier(t *testing.T) {
	named := map[string]bool{}
	for _, tier := range Tiers {
		named[tier] = true
	}
	claimed := map[string]bool{}
	for _, s := range Ours {
		for _, dir := range s.Dirs {
			claimed[dir] = true
			if named[dir] || dir == s.Dialect {
				continue
			}
			t.Fatalf("the %s column runs %s/, which is neither a tier in Tiers nor its own "+
				"dialect name, so nothing measures what that directory claims", s.Name, dir)
		}
	}
	for _, tier := range Tiers {
		if !claimed[tier] {
			t.Fatalf("%s/ is cross-checked and no column runs it", tier)
		}
	}
}

// The tiers a column does not run are as much of the measurement as the ones
// it does: dash and ash are the shells docs/spec/shell-matrix.md drew the
// core boundary around, and a column that quietly picked up ext/ would be
// graded on constructs it has never claimed to have.
func TestTheHoldoutsDoNotRunExt(t *testing.T) {
	for _, s := range Ours {
		if s.Dialect != "dash" && s.Dialect != "ash" {
			continue
		}
		for _, dir := range s.Dirs {
			if dir == "ext" {
				t.Fatalf("the %s column runs ext/, and it is one of the two shells the "+
					"ext boundary was drawn around", s.Name)
			}
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no module root above the working directory")
		}
		dir = parent
	}
}
