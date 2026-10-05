// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"testing"
)

// The candidates a symlinked directory makes when the dialect withholds its
// slash until the word names it whole — readline's default. The line half is
// pinned through a pseudo-terminal in cmd/bash
// (TestTabMarksASymlinkedDirectoryOnceItIsWhole); this is the listing half,
// which a Tab that inserts never draws: measured on bash 5.3.20, the Tab that
// lists draws `linkdir/   linkother/`, marked, while the line gets `linkdir`.
func TestASymlinkedDirectoryIsMarkedInAListing(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"realdir", "linkother"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("realdir", filepath.Join(dir, "linkdir")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		whole bool
		word  string
		want  []Candidate
	}{
		{"withheld, partly typed", true, "link", []Candidate{
			{Word: "linkdir", Display: "linkdir/", Open: true},
			{Word: "linkother/"},
		}},
		{"withheld, named whole", true, "linkdir", []Candidate{{Word: "linkdir/"}}},
		{"marked at once", false, "link", []Candidate{{Word: "linkdir/"}, {Word: "linkother/"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := shellCompleter{dir: dir, symlinkMarkedWhenWhole: tc.whole}
			got := s.Complete(Completion{Word: tc.word})
			if len(got) != len(tc.want) {
				t.Fatalf("%q offered %+v, want %+v", tc.word, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("%q candidate %d is %+v, want %+v", tc.word, i, got[i], tc.want[i])
				}
			}
		})
	}
}
