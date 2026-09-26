// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// backslashIFSRun splits, does not glob, and varies nothing else: a
// backslash written into `IFS` is a separator in every column of the panel,
// so there is no axis here and the only answer that has to be pinned is that
// splitting happens at all. See interp/valuebackslashseparator.go for the
// measurement.
func backslashIFSRun(t *testing.T, src string, pattern ValueBackslashPolicy, join Answer) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *Semantics) {
		s.SplitParamExpansion = Yes
		s.GlobExpansionResults = No
		s.ArrayScalarIsTheWholeArray = Yes
		s.ArrayNameWithoutSubscriptIsTheList = Yes
		s.UnquotedListJoinsOnIFS = join
		s.UnquotedListBoundaryIsIFSWhitespace = No
		s.ValueBackslashInAPattern = pattern
	})
}

// A backslash that arrived in a value separates when `IFS` holds one, and it
// did not here because the escaped form writes it as a mark of its own rather
// than as a backslash — so the character the splitter tested against `IFS`
// was never the character the value held (#4585).
//
// Every row is unanimous across bash 5.3.20, ksh93u+ 2012, dash 0.5.12 and
// zsh 5.9.2 under `shwordsplit`, which is why this is core and carries no
// axis. It is run against all three readings of
// Semantics.ValueBackslashInAPattern, because a *pattern's* reading of a
// value's backslash is a different question from whether that backslash is a
// field separator, and none of the three may move a row.
func TestABackslashInIFSSeparates(t *testing.T) {
	for _, c := range []struct{ name, setup, word, want string }{
		// The row the issue was filed on: the backslash is plainly a
		// character of the value — `printf '[%s]' "$v"` is `[b\c]` here and
		// everywhere — and it cuts the value in two.
		{"a backslash between two characters", `IFS='\'; v='b\c'`, `x${v}y`, `2[xb][cy]`},
		// The issue's own spelling, where a word follows the expansion: the
		// value's two characters are two fields and `y` is a third word.
		{"the issue's spelling", `IFS='\'; v='b\c'`, `x${v} y`, `3[xb][c][y]`},

		// The edges. A value that is nothing but the separator, one that
		// ends on it — which is the doubled mark, a backslash that ran out
		// of value — and one that opens on it.
		{"a value that is only the separator", `IFS='\'; v='\'`, `x${v}y`, `2[x][y]`},
		{"a value that ends on it", `IFS='\'; v='b\'`, `x${v}y`, `2[xb][y]`},
		{"a value that opens on it", `IFS='\'; v='\c'`, `x${v}y`, `2[x][cy]`},

		// **The discriminating row.** A value holding two backslashes
		// already split on the second before this change and not on the
		// first, because the second is written as an ordinary marked
		// backslash and the walk could see it. One character separated and
		// its neighbor did not, from one value and one `IFS` — which is what
		// says the defect is the *form* rather than the separator test. Two
		// adjacent non-whitespace separators are one delimiter with an empty
		// field between them, by the ordinary rule.
		{"two backslashes in a row", `IFS='\'; v='b\\c'`, `x${v}y`, `3[xb][][cy]`},

		// Beside a separator of another kind, so that the backslash is one
		// entry of `IFS` rather than the whole of it.
		{"an IFS holding a backslash and a colon", `IFS='\:'; v='b\c:d'`, `x${v}y`, `3[xb][c][dy]`},

		// With nothing written beside the expansion, where there is no text
		// for a lost boundary to join across.
		{"nothing beside the expansion", `IFS='\'; v='b\c'`, `${v}`, `2[b][c]`},

		// The list path reaches the same splitter, so it moves with it.
		{"an element holding one", `IFS='\'; set -- 'b\c' d`, `x${@}y`, `3[xb][c][dy]`},

		// **The controls, and they are what say this is the splitting and
		// not the escaping.** A quoted expansion does not split, so the
		// backslash stays a character of the one field; an `IFS` with no
		// backslash in it leaves the value whole; and an `IFS` set to
		// nothing splits nothing at all. Each keeps the backslash visible in
		// the output, so a row that produced no text would be seen as such
		// rather than compared against an expectation of nothing.
		{"quoted, so nothing splits", `IFS='\'; v='b\c'`, `"x${v}y"`, `1[xb\cy]`},
		{"an IFS without a backslash", `IFS=:; v='b\c'`, `x${v}y`, `1[xb\cy]`},
		{"an IFS set to nothing", `IFS=; v='b\c'`, `x${v}y`, `1[xb\cy]`},
		{"a backslash in IFS and none in the value", `IFS='\'; v='bc'`, `x${v}y`, `1[xbcy]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := c.setup + "\n" + wordFieldProbe(c.word)
			for _, p := range []ValueBackslashPolicy{
				ValueBackslashQuotesWhatFollows,
				ValueBackslashIsData,
				ValueBackslashDisarmsWhatFollows,
			} {
				for _, join := range []Answer{Yes, No} {
					out, st := backslashIFSRun(t, src, p, join)
					if st != 0 {
						t.Fatalf("pattern=%v join=%v: status %d, out %q, want a clean run", p, join, st, out)
					}
					if out != c.want {
						t.Errorf("pattern=%v join=%v: %s = %q, want %q", p, join, c.word, out, c.want)
					}
				}
			}
		})
	}
}
