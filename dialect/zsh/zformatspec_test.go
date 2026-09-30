// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// zformat's format-string grammar, and the name it writes through.
//
// `V13zformat.ztst` ran one of the reference's six chunks and failed it. It
// now runs all six and agrees on every line, and these are the seven
// disagreements that stood between — each a row that fails on its own without
// the change beside it, measured against zsh 5.9.2 on 2026-09-29.
//
// They are one test because they are one builtin's grammar and because three
// of the seven are only visible *behind* another: the file stops at its first
// failing chunk, so the precision bug hid the nesting bug, which hid the
// operand-name bug, which hid the rest. A table keeps the ones that were
// invisible from being dropped when the one in front is fixed.
func TestZformatFormatGrammar(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		src  string
		want string
	}{
		// An empty precision is a precision that is **absent**, so `%5.s`
		// behaves as `%5s`. Rewinding past the dot left the specifier letter
		// unfindable and the whole spec was copied out as text.
		{
			"an empty precision is absent, not zero",
			`zformat -f o '%5.s' s:ab; print -r -- "[$o]"`,
			"[ab   ]\n",
		},
		// The reading it is not: `.0` is a real precision of zero, and this
		// row is what tells the two apart. Without it, "absent" and "zero"
		// both satisfy the row above by accident.
		{
			"a zero precision truncates to nothing",
			`zformat -f o '%.0s' s:ab; print -r -- "[$o]"`,
			"[]\n",
		},
		// A negative precision is not a precision, and this is the opposite
		// direction from the row above: the fix that accepts `.` with no
		// digits must not also accept `.-2`.
		{
			"a negative precision is not a spec at all",
			`zformat -f o '%5.-2s' s:ab; print -r -- "[$o]"`,
			"[%5.-2s]\n",
		},
		// A lone `-` is a sign with no width, and the spec stays valid.
		{
			"a sign with no digits still leaves a spec",
			`zformat -f o '%-.2s' s:abcdefgh; print -r -- "[$o]"`,
			"[ab]\n",
		},
		// The same read feeds a ternary, so the bare sign must arrive as
		// **no** number rather than as a test number of zero. `%-(c.y.n)`
		// behaves exactly as `%(c.y.n)`, which a zero would not.
		{
			"a bare sign before a ternary is not a test number",
			`zformat -f o '%-(c.y.n)' c:1; print -r -- "[$o]"`,
			"[n]\n",
		},
		// A nested `%(`…`)` is copied whole, so the inner group's delimiters
		// are not read as the outer half's. This is the chunk the file
		// stopped on second.
		{
			"a nested ternary keeps its own delimiters",
			`zformat -f o '%(8n.%(5j.yes.no).no)' n:8 j:5; print -r -- "[$o]"`,
			"[yes]\n",
		},
		// A bare paren opens nothing, which is the case that would break if
		// the nesting counted every paren rather than only `%(`.
		{
			"a bare paren is ordinary text",
			`zformat -f o '%(8n.a(b.c).d)' n:8; print -r -- "[$o]"`,
			"[a(b.d)]\n",
		},
		// Under -F the sign inverts the comparison, and `-0` is the one
		// number whose sign an int cannot carry: `%0` asks whether the value
		// is longer than nothing and `%-0` asks whether it is empty.
		{
			"a negative zero under -F is not a positive zero",
			`zformat -F o '%0(a.a.A)%-0(a.a.A)' a:; print -r -- "[$o]"`,
			"[Aa]\n",
		},
		// A backslash escapes whatever follows it, not only a colon.
		{
			"a backslash escapes the next byte, whatever it is",
			`zformat -a arr . 'a\\b' 'a\qb'; print -rl -- "${arr[@]}"`,
			"a\\b\naqb\n",
		},
		// And only the left half is unescaped, which is the asymmetry a
		// tidier reading would remove.
		{
			"the right half of a pair is left exactly as written",
			`zformat -a arr . 'x:y\\z'; print -r -- "${arr[1]}"`,
			"x.y\\\\z\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status, errs := runZshSplitOnRoute(t, interp.RouteScriptFile, c.src)
			if out != c.want {
				t.Errorf("%s\nwrote %q, want %q (stderr %q)", c.src, out, c.want, errs)
			}
			if status != 0 {
				t.Errorf("%s\nstatus %d, want 0 (stderr %q)", c.src, status, errs)
			}
		})
	}
}

// `zformat -f` writes through a **positional parameter**, and the name rule
// and the store are two separate facts.
//
// A name check that accepts `1` in front of a store that does not is the
// worse of the two failures: the builtin reports success and the value lands
// in a variable called `1`, which no script can read back, because `$1` is
// the positional. So the row reads the parameter rather than the status.
//
// Position 0 is `$0` and is not in the positional list at all, which is why
// it is a row of its own.
func TestZformatWritesThroughAPositionalParameter(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		src  string
		want string
	}{
		{
			"a position in the list",
			`() { zformat -f 1 'X%sY' s:hi; print -r -- "[$1]"; } a b`,
			"[XhiY]\n",
		},
		{
			"a position past the end of the list",
			`() { zformat -f 3 'X%sY' s:hi; print -r -- "[$3]"; } a b`,
			"[XhiY]\n",
		},
		{
			"position zero, which is not in the list",
			`() { zformat -f 0 'X%sY' s:hi; print -r -- "[$0]"; } a b`,
			"[XhiY]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status, errs := runZshSplitOnRoute(t, interp.RouteScriptFile, c.src)
			if out != c.want {
				t.Errorf("%s\nwrote %q, want %q (stderr %q)", c.src, out, c.want, errs)
			}
			if status != 0 {
				t.Errorf("%s\nstatus %d, want 0 (stderr %q)", c.src, status, errs)
			}
		})
	}
}

// A negative position is **not** a name, and both shells refuse it.
//
// This is the boundary the digit run is written for: a signed reader would
// have taken `-1` and stored through a position that does not exist. It is
// separate from the rows above because it is the case that must keep failing.
func TestZformatRefusesANegativePosition(t *testing.T) {
	t.Parallel()
	_, status, errs := runZshSplitOnRoute(t, interp.RouteScriptFile,
		`() { zformat -f -1 'X%sY' s:hi; } a b`)
	if status == 0 {
		t.Errorf("status 0, want non-zero (stderr %q)", errs)
	}
	if want := "not an identifier: -1"; !strings.Contains(errs, want) {
		t.Errorf("stderr %q, want it to hold %q", errs, want)
	}
}
