// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
)

// The order a pathname expansion comes back in, where the dialect has a
// parameter that says so — bash 5.3's `GLOBSORT`, and no other column in the
// panel has one. See Semantics.SortOrderVariable. Which dialect names the
// parameter is asserted in the dialect packages and never here.

// sortSem is the permissive base with a parameter named, so that the rows
// below measure what the parameter *does* and the row that measures whether a
// dialect has one at all can leave it empty.
func sortSem(name string) Semantics {
	s := permissive()
	s.SortOrderVariable = name
	return s
}

// sortDir builds five names whose size, modification time and numeric value
// each rank them differently, so that no two keys agree by accident.
//
//	name    size  mtime      number
//	10      0     third      10
//	100     0     fourth     100
//	9       0     fifth      9
//	big     5     first      —
//	tiny    1     second     —
func sortDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"10": "", "100": "", "9": "", "big": "12345", "tiny": "1",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Now().Add(-time.Hour)
	for i, name := range []string{"big", "tiny", "10", "100", "9"} {
		when := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(filepath.Join(dir, name), when, when); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// sorted runs one expansion with the parameter set to value.
func sorted(t *testing.T, dir, value string) string {
	t.Helper()
	src := `GLOBSORT=` + value + `; printf "[%s]" *`
	out, st := run(t, src, func(r *Runner) {
		s := sortSem("GLOBSORT")
		r.Semantics, r.Dir = &s, dir
	})
	if st != 0 {
		t.Fatalf("GLOBSORT=%s: status %d, out %q", value, st, out)
	}
	return out
}

// Every key, and the sign that reverses it.
//
// Measured 2026-09-16 against bash 5.3.20 under LC_ALL=C. The name is the
// tie-break for every key and it is reversed with the rest, which is the row
// three files of equal size make.
func TestTheSortParameterOrdersAnExpansion(t *testing.T) {
	dir := sortDir(t)
	for _, tc := range []struct{ value, want string }{
		{"name", "[10][100][9][big][tiny]"},
		{"+name", "[10][100][9][big][tiny]"},
		{"-name", "[tiny][big][9][100][10]"},
		// Three zero-length names tie and fall to the name; `big` and `tiny`
		// rank behind them by size and ahead of them reversed.
		{"size", "[10][100][9][tiny][big]"},
		{"-size", "[big][tiny][9][100][10]"},
		{"mtime", "[big][tiny][10][100][9]"},
		{"-mtime", "[9][100][10][tiny][big]"},
		// Numbers rank as numbers; the two names that are not numbers tie at
		// nothing and fall to the name, behind them.
		{"numeric", "[9][10][100][big][tiny]"},
		{"-numeric", "[tiny][big][100][10][9]"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			if got := sorted(t, dir, tc.value); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// What the value may be, which is four rules a reader would assume the other
// way round and one that is simply the default.
func TestTheSortParameterReadsItsValueExactly(t *testing.T) {
	dir := sortDir(t)
	const asc = "[10][100][9][big][tiny]"
	const desc = "[tiny][big][9][100][10]"
	for _, tc := range []struct{ name, value, want string }{
		{"a bare sign is the name key", "-", desc},
		{"and so is a bare plus", "+", asc},
		{"a leading blank is trimmed", `" -name"`, desc},
		{"a trailing blank is not", `"-name "`, asc},
		{"the key is case sensitive", "-NAME", asc},
		{"and is matched whole", "-nam", asc},
		{"a key it has not got says nothing", "bogus", asc},
		{"and neither does an empty value", `""`, asc},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sorted(t, dir, tc.value); got != tc.want {
				t.Errorf("GLOBSORT=%s: got %s, want %s", tc.value, got, tc.want)
			}
		})
	}
}

// `nosort` is the directory's own order **reversed**, which is measured
// rather than reasoned — see the note in interp/globsort.go. The expectation
// is read from the directory here rather than written down, because what the
// order *is* belongs to the filesystem and only what the shell does with it
// belongs to this repository.
func TestTheSortParameterCanAskForTheDirectorysOwnOrder(t *testing.T) {
	dir := sortDir(t)
	f, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := f.ReadDir(-1)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Reverse(names)
	want := "[" + strings.Join(names, "][") + "]"
	for _, value := range []string{"nosort", "-nosort", "+nosort"} {
		// The sign says nothing here, which is the one key it does not
		// reach — measured, all three are one answer.
		if got := sorted(t, dir, value); got != want {
			t.Errorf("GLOBSORT=%s: got %s, want %s", value, got, want)
		}
	}
	// And it is not simply the sorted list, or the row above would pass
	// against a shell that ignored the parameter entirely.
	if want == "[10][100][9][big][tiny]" {
		t.Skip("this filesystem hands its listing back in name order, so the row cannot discriminate")
	}
}

// The order reaches the *walk* and not only the result: with two levels, a
// directory's own order decides which subdirectory is entered first, and a
// walk that sorted each level and reversed the answer would give the name
// order backwards instead.
func TestTheDirectorysOwnOrderReachesEveryLevel(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"d1", "d2"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "d1", "b"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d2", "a"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := run(t, `GLOBSORT=nosort; printf "[%s]" */*`, func(r *Runner) {
		s := sortSem("GLOBSORT")
		r.Semantics, r.Dir = &s, dir
	})
	if st != 0 {
		t.Fatalf("status %d, out %q", st, out)
	}
	// Whichever way the directory hands its two names back, the two words
	// come out in the opposite order — never in the name order, which is
	// what a sort at each level would leave.
	if out != "[d2/a][d1/b]" && out != "[d1/b][d2/a]" {
		t.Errorf("got %s, want the two words in one order or the other", out)
	}
	sortedOut, _ := run(t, `printf "[%s]" */*`, func(r *Runner) {
		s := sortSem("GLOBSORT")
		r.Semantics, r.Dir = &s, dir
	})
	if sortedOut != "[d1/b][d2/a]" {
		t.Errorf("the ordinary order: got %s", sortedOut)
	}
}

// A dialect with no such parameter is not asked: the same variable set in a
// shell that has never heard of it changes nothing, which is what keeps every
// other column — and every script that happens to export one — on the order
// it has always had.
func TestAParameterTheDialectDoesNotNameIsNotRead(t *testing.T) {
	dir := sortDir(t)
	out, st := run(t, `GLOBSORT=-name; printf "[%s]" *`, func(r *Runner) {
		s := sortSem("")
		r.Semantics, r.Dir = &s, dir
	})
	if st != 0 || out != "[10][100][9][big][tiny]" {
		t.Errorf("got %s status %d, want the name order", out, st)
	}
	// And a dialect that names a *different* parameter reads that one and
	// not this, which is what makes the field a name rather than a switch.
	out, _ = run(t, `GLOBSORT=-name; SORTBY=-name; printf "[%s]" *`, func(r *Runner) {
		s := sortSem("SORTBY")
		r.Semantics, r.Dir = &s, dir
	})
	if out != "[tiny][big][9][100][10]" {
		t.Errorf("a differently named parameter: got %s", out)
	}
}
