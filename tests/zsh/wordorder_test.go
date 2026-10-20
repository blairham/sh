// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// Three surfaces order words and they give **one** order.
//
// A pathname expansion's matches, the words a glob qualifier list's modifiers
// produced and then re-sorted, and the `o`/`O` flags of a parameter
// expansion. All three are ordering words a shell is about to hand a command,
// so a shell that sorted `*` one way and `${(o)a}` another would be wrong
// about one of them whichever answer is right — and the answer here is
// contested, which is what makes three copies of it a thing that drifts
// (#1675).
//
// Measured on zsh 5.9.2 under `LC_ALL=C`, which is the locale both sweeps
// here pin and the one every shell in the panel agrees in. Outside it three
// of the four collate and this shell does not; see interp/order.go for the
// full table and for why the collation itself is not attempted.
//
// The names are chosen so that byte order and every plausible alternative
// part company: a digit, an upper-case letter, an underscore and a lower-case
// one all sort differently under a collation than under a byte comparison.
func TestOneOrderReachesEverySurfaceThatSortsWords(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a", "b", "Z", "A_upper", "1digit", "_under", "banana", "Cherry", "date"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const globbed = "[1digit][A_upper][Cherry][Z][_under][a][b][banana][date]"
	for _, tc := range []struct{ name, src, want string }{
		{"a pathname expansion", `printf "[%s]" *`, globbed},
		{"and one with a qualifier list", `printf "[%s]" *(N)`, globbed},
		// The modifiers re-sort what they produced rather than keeping the
		// order they were given, so this is the same question asked after a
		// transformation: every name is lower case here and the order is the
		// order of the *results*.
		{
			"the words a modifier produced",
			`printf "[%s]" *(N:l)`,
			"[1digit][_under][a][a_upper][b][banana][cherry][date][z]",
		},
		// And the parameter expansion's own flags, over the same names
		// written in a different order.
		{
			"a parameter expansion's `o` flag",
			`a=(date b Z 1digit _under A_upper); printf "[%s]" ${(o)a}`,
			"[1digit][A_upper][Z][_under][b][date]",
		},
		{
			"and `O`, which is that reversed",
			`a=(date b Z 1digit _under A_upper); printf "[%s]" ${(O)a}`,
			"[date][b][_under][Z][A_upper][1digit]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, "LC_ALL=C; "+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
	// The pair that makes the rows above an assertion about *one* order
	// rather than about two that happen to agree: the same six names, once
	// as files and once as an array, must come back the same way.
	sub := filepath.Join(dir, "same")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"date", "b", "Z", "1digit", "_under", "A_upper"} {
		if err := os.WriteFile(filepath.Join(sub, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	byGlob, _ := runZsh(t, sub, `LC_ALL=C; printf "[%s]" *`)
	byFlag, _ := runZsh(t, sub, `LC_ALL=C; a=(date b Z 1digit _under A_upper); printf "[%s]" ${(o)a}`)
	if byGlob != byFlag {
		t.Errorf("a glob orders the same names %q and `${(o)a}` orders them %q", byGlob, byFlag)
	}
}
