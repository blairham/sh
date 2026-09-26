// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A run of digits in a match's name read as the number it spells when a
// pathname expansion is put in order — see
// Runner.SortsGlobMatchesNumerically. A session switch and not an axis, so
// this names the switch and no shell.
//
// Every row runs in both states, and the *off* row is the control: a shell
// that ordered its matches by accident would agree with one of the two and
// not with both.
func TestSortsGlobMatchesNumerically(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		on    string
		off   string
	}{
		{
			"the number and not its spelling",
			[]string{"f1", "f2", "f10"},
			"[f1][f2][f10]", "[f1][f10][f2]",
		},
		{
			"a run inside the name counts, and every run in it",
			[]string{"a1b2", "a1b10", "a2b2"},
			"[a1b2][a1b10][a2b2]", "[a1b10][a1b2][a2b2]",
		},
		{
			"two runs of equal value fall through to the byte",
			[]string{"f01z", "f1a"},
			"[f01z][f1a]", "[f01z][f1a]",
		},
		{
			"and so does everything that is not a pair of digits",
			[]string{"f+2", "f-1", "f 1"},
			"[f 1][f+2][f-1]", "[f 1][f+2][f-1]",
		},
		{
			"leading zeros tie, and the shorter name wins the tie",
			[]string{"0", "00", "000"},
			"[0][00][000]", "[0][00][000]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, on := range []bool{true, false} {
				dir := t.TempDir()
				for _, f := range tc.files {
					if err := os.WriteFile(filepath.Join(dir, f), nil, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				out, _ := run(t, `printf "[%s]" *`, func(r *Runner) {
					r.Dir = dir
					r.SetSortsGlobMatchesNumerically(on)
				})
				want := tc.off
				if on {
					want = tc.on
				}
				if out != want {
					t.Errorf("on=%v: got %q, want %q", on, out, want)
				}
			}
		})
	}
}

// It is not the `numeric` key of the sort-order parameter one dialect has,
// and this is the pair that says so rather than a note: the same three names
// answer differently under the two, so a reuse would have been a wrong answer
// and not a tidy one. See globSortNumericLess beside numericSegmentOrder.
func TestTheGlobNumericSwitchIsNotTheNumericSortKey(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"f1", "f2", "f10"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	key, _ := run(t, `printf "[%s]" *`, func(r *Runner) {
		r.Dir = dir
		s := *r.Semantics
		s.SortOrderVariable = "GLOBSORT"
		r.Semantics = &s
		r.Vars = map[string]string{"GLOBSORT": "numeric"}
	})
	if want := "[f1][f10][f2]"; key != want {
		t.Errorf("the sort key gave %q, want %q — both whole names must be numbers", key, want)
	}
	sw, _ := run(t, `printf "[%s]" *`, func(r *Runner) {
		r.Dir = dir
		r.SetSortsGlobMatchesNumerically(true)
	})
	if want := "[f1][f2][f10]"; sw != want {
		t.Errorf("the switch gave %q, want %q — the run inside the name counts", sw, want)
	}
	if key == sw {
		t.Errorf("the two agreed at %q, so this row cannot tell them apart", key)
	}
}

// The order reaches every level of a multi-component pattern rather than only
// the last, which is what says the switch is on the comparator the walk uses
// and not on a final pass.
func TestSortsGlobMatchesNumericallyReachesEveryComponent(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"d1", "d2", "d10"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"d1/f2", "d1/f10", "d2/f1", "d10/f1"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out, _ := run(t, `printf "[%s]" */*`, func(r *Runner) {
		r.Dir = dir
		r.SetSortsGlobMatchesNumerically(true)
	})
	if want := "[d1/f2][d1/f10][d2/f1][d10/f1]"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
