// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// Matches are laid out in columns, filled downwards.
//
// Measured against bash and zsh in the same directory and window: as many
// columns as fit, each as wide as the longest match plus two, filled down one
// column before starting the next — so that sorted matches read in order
// downwards. Reading across would put `b` beside `a` and `z` below it.
func TestLayingOutMatchesInColumns(t *testing.T) {
	four := []string{"a", "b", "c", "d"}
	for _, tc := range []struct {
		name  string
		in    []string
		width int
		want  []string
	}{
		{"nothing to show", nil, 80, nil},
		{"one match", []string{"only"}, 80, []string{"only"}},
		// Three columns of one, filled downwards: a and b in the first.
		// A cell is the longest match plus two, so single characters
		// take three columns each and seven fits two of them.
		{"down the columns", four, 7, []string{"a  c", "b  d"}},
		{"wider terminal, fewer rows", four, 80, []string{"a  b  c  d"}},
		{"narrow terminal, one per row", four, 3, []string{"a", "b", "c", "d"}},
		// A width of zero is a terminal that will not say, and one to a row
		// is the only arrangement that cannot be wrong on it.
		{"no width at all", four, 0, []string{"a", "b", "c", "d"}},
		// Each column is as wide as the longest match plus two.
		{
			"padded to the longest",
			[]string{"a", "bbbb", "c", "d"},
			14,
			[]string{"a     c", "bbbb  d"},
		},
		// A match wider than the screen is not cut: it takes its row and
		// wraps, which is what the terminal does with it.
		{"wider than the screen", []string{"aaaaaaaaaa", "b"}, 4, []string{"aaaaaaaaaa", "b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := columns(tc.in, tc.width)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("columns = %q, want %q", got, tc.want)
			}
		})
	}
}

// Nothing is padded past the last match on its row: trailing spaces are
// invisible until something copies them.
func TestNoPaddingAfterTheLastOnARow(t *testing.T) {
	for _, row := range columns([]string{"a", "bbbb", "c", "d", "e"}, 40) {
		if strings.TrimRight(row, " ") != row {
			t.Errorf("row %q ends in spaces", row)
		}
	}
}

// The editor draws what the layout produced, above the line being typed.
func TestListingDrawsTheColumns(t *testing.T) {
	var out strings.Builder
	e := &editor{out: &out, width: func() int { return 7 }}
	e.list([]string{"a", "b", "c", "d"}, "$ ")
	if got := out.String(); got != "\r\na  c\r\nb  d\r\n" {
		t.Errorf("drew %q, want the two rows", got)
	}
}
