// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// relativeHitTree is `<root>/b3/qq` printing `rel` and `<root>/b2/qq`
// printing `abs`, two different scripts under one name, so which one ran is
// in the output; and `<root>/b4` a link to `b2`.
func relativeHitTree(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for dir, word := range map[string]string{"b3": "rel", "b2": "abs"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		script := "#!/bin/sh\necho " + word + "\n"
		if err := os.WriteFile(filepath.Join(root, dir, "qq"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("b2", filepath.Join(root, "b4")); err != nil {
		t.Fatal(err)
	}
	return root
}

func relativeHitRun(t *testing.T, root, src string, set func(*Semantics), diag Diagnostics) string {
	t.Helper()
	out, _ := run(t, src, func(r *Runner) {
		sem := testSemantics()
		sem.HashTakesAPathToRemember = Yes
		sem.HashReportsThePath = Yes
		sem.HashListsAsCommands = Yes
		set(&sem)
		d := diag
		r.Semantics, r.Diagnostics, r.Dir = &sem, &d, root
	})
	return out
}

// TestAHitFromARelativeEntryIsRemembered is #6069, one row per reading. Each
// row was measured against the shell holding that reading, and each answer
// was written down before this shell gave it.
func TestAHitFromARelativeEntryIsRemembered(t *testing.T) {
	root := relativeHitTree(t)
	none := func(*Semantics) {}
	listed := Diagnostics{HashListing: HashListingNameEqualsPath}
	for _, c := range []struct {
		name, src string
		set       func(*Semantics)
		diag      Diagnostics
		want      string
	}{
		{
			// An explicit `hash` remembers the hit as a run would, where it
			// wrote the absolute path.
			"explicit hash is spelled", "PATH=b3; hash qq; hash", none, listed, "qq=b3/qq\n",
		},
		{
			"a table of absolute entries", "PATH=b3:" + root + "/b2; qq; qq; hash",
			func(s *Semantics) { s.CommandTableHoldsOnlyAbsoluteEntries = Yes }, listed,
			"rel\nrel\nqq=" + root + "/b2/qq\n",
		},
		{
			"the control: a table that keeps the relative hit", "PATH=b3:" + root + "/b2; qq; qq; hash",
			none, listed, "rel\nrel\nqq=b3/qq\n",
		},
		{
			"a relative entry alone leaves nothing", "PATH=b3; hash qq; echo st=$?; hash",
			func(s *Semantics) {
				s.CommandTableHoldsOnlyAbsoluteEntries = Yes
				s.HashReportsAMissingName = No
			}, listed, "st=0\n",
		},
		{
			// What the script wrote stands in front of a relative entry,
			// where a search-made entry does not.
			"a written entry still shadows", "PATH=b3:/bin; hash -p " + root + "/b2/qq qq; qq",
			func(s *Semantics) { s.CommandTableHoldsOnlyAbsoluteEntries = Yes }, listed, "abs\n",
		},
		{
			"a remembered relative path under dot", "hash -p b3/qq qq; command -v qq; hash -t qq; hash -l",
			func(s *Semantics) { s.HashedRelativePathReportedUnderDot = Yes },
			Diagnostics{},
			"./b3/qq\n./b3/qq\nbuiltin hash -p b3/qq qq\n",
		},
		{
			"a current-directory hit listed bare", "PATH=.:/bin; cd b3; qq; hash",
			func(s *Semantics) {
				s.PathHitFromTheCurrentDirectoryRunsBare = Yes
				s.PathHitSpelled = PathHitFromTheWorkingDirectory
			}, listed, "rel\nqq=qq\n",
		},
		{
			"type -P is the spelled hit", "PATH=b3; type -P qq",
			func(s *Semantics) {
				s.TypeEndsOptionsWithDashDash = Yes
				s.TypeOptions = "afptP"
			},
			Diagnostics{},
			"b3/qq\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := relativeHitRun(t, root, c.src, c.set, c.diag); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestEachDirectoryIsListedOnce pins LookPathAllOncePerDirectory: the same
// directory by a second spelling, by a link, and by a relative entry is
// dropped, and the control — LookPathAll — keeps every one.
func TestEachDirectoryIsListedOnce(t *testing.T) {
	root := relativeHitTree(t)
	r := newTestRunner(t, &Runner{Dir: root})
	runPart(t, r, "PATH="+root+"/b2:"+root+"/b4:b2:"+root+"/b3/../b2:"+root+"/b3\n")
	if got := r.LookPathAllOncePerDirectory("qq"); len(got) != 2 ||
		got[0] != root+"/b2/qq" || got[1] != root+"/b3/qq" {
		t.Errorf("once per directory = %q, want b2's and b3's", got)
	}
	if got := r.LookPathAll("qq"); len(got) != 5 {
		t.Errorf("LookPathAll = %q, want all five", got)
	}
}

// TestAnExplicitHashSearchesAgain is #6110: `hash qq` for a name the table
// already holds, with a copy that has since appeared earlier on PATH. One
// reading searches again and remembers the new copy; the other keeps the
// entry. And a search that finds nothing leaves the table as it was.
func TestAnExplicitHashSearchesAgain(t *testing.T) {
	root := relativeHitTree(t)
	src := "PATH=" + root + "/b4:" + root + "/b3; hash -p " + root + "/b3/qq qq; hash qq; command -v qq; " +
		"PATH=/nowhere; hash -p " + root + "/b3/qq qq; hash qq; command -v qq\n"
	for _, c := range []struct {
		again Answer
		want  string
	}{
		{Yes, root + "/b4/qq\n" + root + "/b3/qq\n"},
		{No, root + "/b3/qq\n" + root + "/b3/qq\n"},
	} {
		got := relativeHitRun(t, root, src, func(s *Semantics) {
			s.HashNameSearchesAgain = c.again
			s.HashReportsAMissingName = No
		}, Diagnostics{})
		if got != c.want {
			t.Errorf("searches again = %v: got %q, want %q", c.again, got, c.want)
		}
	}
}
