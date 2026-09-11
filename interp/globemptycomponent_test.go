// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"path/filepath"
	"testing"
)

// TestAnEmptyComponentIsASeparatorTheWalkWritesBack is #1511.
//
// Two adjacent slashes leave an **empty** component, and the walk dropped it:
// `cx//*` came back `cx/ax` where all six columns answer `cx//ax`. Unanimous,
// so it is the core rather than an axis, and it was the last spelling a
// pattern carries that this walk still normalized away — #1350 put the
// trailing slash run back on each match and #1480 stopped the walk cleaning,
// so a `.` or `..` component survives.
//
// The assertions are on the strings for the same reason #1350's are: the
// wrong answer names the same file. Only something that compares the two
// spellings, or hands the name back to a shell that re-globs it, can tell.
func TestAnEmptyComponentIsASeparatorTheWalkWritesBack(t *testing.T) {
	dir := slashDir(t)
	for _, tc := range []struct{ name, src, want string }{
		// The reproduction, and the same shape one level further in.
		{"in the middle", `printf "[%s]" cx//*`, `[cx//ax][cx//dx]`},
		{"behind a dot component", `printf "[%s]" .//cx/*`, `[.//cx/ax][.//cx/dx]`},
		// The run is reproduced as written here too, which is the mid-pattern
		// half of the rule #1350 recorded for the end of the word. All six
		// answer three slashes for three.
		{"a run of three", `printf "[%s]" cx///*`, `[cx///ax][cx///dx]`},
		// A real component on both sides of the empty one, so the separator
		// is written back rather than merely tolerated at the end of the walk.
		{"a literal component behind it", `printf "[%s]" cx//dx/*`, `[cx//dx/ax]`},
		// A *matched* component in front of the empty one, which is where
		// bash parts company: dash, ksh93 and zsh answer `[cx//ax][sym//ax]`
		// and the three bash columns collapse the run to `[cx/ax][sym/ax]`.
		// That is the same divergence #1350 recorded for the trailing run and
		// the same reading of it — bash keeps the literal text in front of
		// the first pattern component and rebuilds the rest, so `cx//*` is
		// `cx//ax` there too. Reproducing the run is what this shell does in
		// every dialect.
		{"a matched component in front", `printf "[%s]" *//ax`, `[cx//ax][sym//ax]`},
		// The empty component and the trailing slash in one pattern: two
		// sources of a separator, and each writes its own. A fix that flushed
		// every empty component where it stood would answer `cx//dx//`.
		{"and a trailing slash too", `printf "[%s]" cx//*/`, `[cx//dx/]`},
		// The control: one slash stays one slash. Without this the rows above
		// pass for a walk that doubles every separator.
		{"one slash is still one", `printf "[%s]" cx/*`, `[cx/ax][cx/dx]`},
		{"and the trailing run alone", `printf "[%s]" *//`, `[ax_dir//][cx//][sym//]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runIn(t, dir, tc.src); got != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

// TestAnEmptyComponentOnAnAbsolutePattern: the absolute walk starts at a
// directory that already ends in a separator, so the count of slashes an
// empty component owes is one fewer there. `//t*` naming `//tmp` in all six
// is the row that says so, and this is the same question inside the tree the
// test owns.
func TestAnEmptyComponentOnAnAbsolutePattern(t *testing.T) {
	dir := slashDir(t)
	want := "[" + dir + "//ax]"
	if got := runIn(t, dir, `printf "[%s]" `+dir+`//a?`); got != want {
		t.Errorf("absolute = %s, want %s", got, want)
	}
	// And under a component the walk matched, where the separator before the
	// empty one came from the join rather than from the pattern's root.
	want = "[" + filepath.Join(dir, "cx") + "//ax]"
	if got := runIn(t, dir, `printf "[%s]" `+dir+`/c*//ax`); got != want {
		t.Errorf("absolute under a match = %s, want %s", got, want)
	}
}
