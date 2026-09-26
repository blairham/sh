// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An unquoted list is joined on `IFS` before it is split here, as in bash —
// and this column answered the opposite until #4586, on a measurement taken
// in the one state where the question cannot be asked.
//
// `shwordsplit` is what makes this shell split a list at all. With it off, an
// unquoted list is one word per element with the empty ones elided and
// nothing is split, so every probe comes back saying "no join" whatever the
// shell does with a list it *is* splitting. Every row below therefore turns
// the option on, and the rows with it off are kept beside them as the control
// that the two states really do differ.
//
// Measured 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// /opt/homebrew/bin/zsh under `-f`; `go version -m` on that path says *not a
// Go executable*. `w(){ printf '%d |' $#; for x in "$@"; do printf ' [%s]'
// "$x"; done; }` is the probe, and the count is asserted beside the fields
// because a lost field boundary reads back through `"$@"` as the same
// characters.
func TestAnUnquotedListIsJoinedOnIFSHere(t *testing.T) {
	for _, c := range []struct{ name, setup, word, want string }{
		// **The discriminating rows**, and what says it is the join: one
		// empty element is *no* field and two are *two*, which splitting
		// each element on its own cannot produce either way — keep the
		// empty element's field and the first row is 1, remove it and the
		// second is 0.
		{"one empty element", `IFS=:; set -- ''`, `${@}`, `0[]`},
		{"two empty elements", `IFS=:; set -- '' ''`, `${@}`, `2[][]`},
		{"three empty elements", `IFS=:; set -- '' '' ''`, `${@}`, `3[][][]`},

		// The rows #4586 was filed on.
		{"an empty element between two", `IFS=:; set -- b '' c`, `x${@}y`, `3[xb][][cy]`},
		{"the same with nothing beside it", `IFS=:; set -- b '' c`, `${@}`, `3[b][][c]`},
		{"two empty elements between", `IFS=:; set -- b '' '' c`, `x${@}y`, `4[xb][][][cy]`},
		{"an empty element at the front", `IFS=:; set -- '' 2`, `${@}`, `2[][2]`},
		{"an empty element at the end", `IFS=:; set -- 2 ''`, `${@}`, `2[2][]`},
		{"four elements, one empty", `IFS=:; set -- b '' c d`, `${@}`, `4[b][][c][d]`},

		// The rows #4582 was filed on: an `IFS` holding both a whitespace
		// and a non-whitespace separator, where the element's own trailing
		// separator and the join's are one delimiter.
		{"a separator element under a mixed IFS", `IFS=' :'; set -- ':' 2`, `x${@}y`, `2[x][2y]`},
		{"a separator ending the list", `IFS=' :'; set -- b ':'`, `x${@}y`, `2[xb][y]`},
		{"a separator ending an element", `IFS=' :'; set -- 'a:' b`, `${@}`, `2[a][b]`},
		// And the same two under an `IFS` with no whitespace in it, which is
		// where the two readings part: there the join writes a separator of
		// its own beside the element's and the empty field between them
		// shows.
		{"a separator element under IFS=:", `IFS=:; set -- ':' 2`, `x${@}y`, `3[x][][2y]`},
		{"a separator ending an element under IFS=:", `IFS=:; set -- 'a:' b`, `${@}`, `3[a][][b]`},

		// The trailing separator is this column's own answer and not the
		// join's, which is the whole of what is left between this shell and
		// bash on these rows: `2 ''` joins to `2:` in both and only here
		// does that closing separator open a field.
		{"a trailing separator opens a field", `IFS=:; set -- 2 ''`, `${@}`, `2[2][]`},
		{"the scalar spelling of the same", `IFS=:; v='a:'`, `${v}`, `2[a][]`},

		// **The controls.** Under the default `IFS` the two readings
		// coincide — a boundary and a run of blanks are one delimiter
		// either way — so nothing here may move a row with `IFS` left
		// alone. And a quoted list keeps one field per element whatever the
		// join says.
		{"the default IFS", `set -- b '' c`, `x${@}y`, `2[xb][cy]`},
		{"quoted, under IFS=:", `IFS=:; set -- b '' c`, `"${@}"`, `3[b][][c]`},
		{"an IFS set to nothing", `IFS=; set -- b '' c`, `x${@}y`, `2[xb][cy]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "setopt shwordsplit\n" + c.setup + "\n" +
				"set -- " + c.word + "\nprintf '%d' \"$#\"\nprintf '[%s]' \"$@\"\n"
			if got := joinRun(t, src); got != c.want {
				t.Errorf("%s; %s = %q, want %q", c.setup, c.word, got, c.want)
			}
		})
	}
}

// With `shwordsplit` off nothing is split and the join is unreachable, which
// is the state the wrong answer was measured in. These rows are the control
// for the suite above: they must be unchanged by it, and they are what a
// probe run in this state reports whatever the join does.
func TestAnUnquotedListIsNotSplitWithoutTheOption(t *testing.T) {
	for _, c := range []struct{ name, setup, word, want string }{
		{"an empty element between two", `IFS=:; set -- b '' c`, `x${@}y`, `2[xb][cy]`},
		{"two empty elements", `IFS=:; set -- '' ''`, `${@}`, `0[]`},
		{"an element holding a separator", `IFS=:; set -- 'a:b' c`, `${@}`, `2[a:b][c]`},
		{"an element holding a space", `IFS=:; set -- 'x y' z`, `${@}`, `2[x y][z]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := c.setup + "\nset -- " + c.word + "\nprintf '%d' \"$#\"\nprintf '[%s]' \"$@\"\n"
			if got := joinRun(t, src); got != c.want {
				t.Errorf("%s; %s = %q, want %q", c.setup, c.word, got, c.want)
			}
		})
	}
}

// joinRun runs a snippet and hands back everything it wrote. `printf` writes
// its format once for no arguments at all, so a count of zero comes back as
// `0[]`.
func joinRun(t *testing.T, src string) string {
	t.Helper()
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out
}
