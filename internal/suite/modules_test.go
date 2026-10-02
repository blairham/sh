// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeReference is a program that answers the module question the way a
// reference does, by printing a directory. It is a shell script, so the
// directory it names is the test's own.
func fakeReference(t *testing.T, prints string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "zsh")
	script := "#!/bin/sh\nprintf '%s\\n' '" + prints + "'\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTheReferencesModulesAreLinkedWhereTheDriverLooks(t *testing.T) {
	mods := t.TempDir()
	ref := fakeReference(t, mods)
	run := t.TempDir()
	s := Suite{Name: "zsh", ModulesAt: "Modules", ModulesQuery: "ignored"}
	if err := placeModules(context.Background(), s, run, ref); err != nil {
		t.Fatal(err)
	}
	got, err := os.Readlink(filepath.Join(run, "Modules"))
	if err != nil || got != mods {
		t.Fatalf("Modules links to %q (%v), want %q", got, err, mods)
	}
}

// A reference that names no directory that exists leaves the layout alone,
// so the file reports what the reference does without its modules.
func TestAReferenceWithNoModuleDirectoryLeavesTheLayoutAlone(t *testing.T) {
	ref := fakeReference(t, filepath.Join(t.TempDir(), "nowhere"))
	run := t.TempDir()
	s := Suite{Name: "zsh", ModulesAt: "Modules", ModulesQuery: "ignored"}
	if err := placeModules(context.Background(), s, run, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(run, "Modules")); !os.IsNotExist(err) {
		t.Fatalf("Modules exists (%v), want nothing placed", err)
	}
}

// And a suite that does not ask places nothing, whatever the reference says.
func TestASuiteWithoutModulesPlacesNothing(t *testing.T) {
	ref := fakeReference(t, t.TempDir())
	run := t.TempDir()
	if err := placeModules(context.Background(), Suite{Name: "bash"}, run, ref); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(run); len(entries) != 0 {
		t.Fatalf("the run directory holds %d entries, want none", len(entries))
	}
}

// The zsh column asks, and asks its reference for the first entry of its own
// module path. The control on the measurement in [Suite.ModulesAt].
func TestTheZshColumnLinksItsReferencesModules(t *testing.T) {
	for _, s := range Panel {
		if s.Name != "zsh" {
			continue
		}
		if s.ModulesAt != "Modules" || s.ModulesQuery == "" {
			t.Fatalf("zsh column: ModulesAt %q, ModulesQuery %q", s.ModulesAt, s.ModulesQuery)
		}
		return
	}
	t.Fatal("no zsh column in the panel")
}

// And the run itself places it: a file run through the harness finds the
// reference's modules beside it, under both shells. A test of placeModules
// alone would pass with the call taken out of the run.
func TestARunFindsTheReferencesModules(t *testing.T) {
	mods := t.TempDir()
	if err := os.WriteFile(filepath.Join(mods, "probe"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ref := fakeReference(t, mods)
	tests := testDir(t, map[string]string{"m.tests": "test -f Modules/probe && echo FOUND\n"})
	s := Suite{Name: "zsh", ShellVar: "THIS_SH", ModulesAt: "Modules", ModulesQuery: "ignored"}
	got := runIn(context.Background(), s, "", tests, "m.tests", "/bin/sh", ref, Options{Timeout: 5 * time.Second})
	if !strings.Contains(got.Output, "FOUND") {
		t.Errorf("the run wrote %q, want the module directory found at ./Modules", got.Output)
	}
}
