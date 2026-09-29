// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runZshUTF8 is runZshSplitOnRoute with a **UTF-8 locale named**, which every
// row below needs and which the shared helper deliberately does not set.
//
// Without it a `\u0080` escape is refused — `character not in range` — and
// that refusal is the locale's answer rather than this test's subject. It is
// a helper of its own rather than a change to the shared one, because a
// locale is an input and the other tests chose not to have one.
func runZshUTF8(t *testing.T, src string) (out string, status int, errs string) {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	dir := t.TempDir()
	r := &interp.Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "zsh", Route: interp.RouteScriptFile,
		Vars:    map[string]string{"PATH": dir, "LC_ALL": "en_US.UTF-8"},
		Dialect: presetDialect(),
	}
	zsh.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return o.String(), st, e.String()
}

// A character range writes an unprintable character in this shell's escape
// form rather than as itself.
//
// Here rather than in `interp` for the rows that need the whole vector: a
// `\u` escape, a NUL inside `$'…'`, and the character reading a non-ASCII
// body wants are each a question of their own, and answering eight axes by
// hand to ask one is how a test ends up measuring the harness.
//
// Measured 2026-09-29 on zsh 5.9.2, **each row a range of one element**, so
// that the rendering is separated from everything a longer range does and
// from `print`, which writes the same character raw. The last row is the
// span `D09brace.ztst` itself exercises.
func TestACharacterRangeEscapesAnUnprintableCharacterHere(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, probe, want string }{
		// C0, including the NUL that `interp`'s harness cannot ask for.
		{"nul", `{$'\x00'..$'\x00'}`, "^@"},
		{"one", `{$'\x01'..$'\x01'}`, "^A"},
		{"tab", `{$'\x09'..$'\x09'}`, `\t`},
		{"newline", `{$'\x0a'..$'\x0a'}`, `\n`},
		{"delete", `{$'\x7f'..$'\x7f'}`, "^?"},
		// The C1 range is `\M-` plus the C0 form of the low half — one rule
		// rather than a second table, which is what these four say
		// together: 0x80 is `\M-^@` because 0x00 is `^@`.
		{"c1 low", `{$'\u0080'..$'\u0080'}`, `\M-^@`},
		{"c1 one", `{$'\u0081'..$'\u0081'}`, `\M-^A`},
		{"c1 next line", `{$'\u0085'..$'\u0085'}`, `\M-^E`},
		{"c1 high", `{$'\u009f'..$'\u009f'}`, `\M-^_`},
		// Printable is identity, and these two are the bound above the
		// escapes: a no-break space and a Latin letter.
		{"latin1 printable", `{$'\u00a0'..$'\u00a0'}`, "\u00a0"},
		{"beyond latin1", `{$'\u0100'..$'\u0100'}`, "\u0100"},
		// The span the suite file exercises, across the boundary.
		{"a span", `{$'\x7e'..$'\u0081'}`, "~ ^? " + `\M-^@ \M-^A`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, st, errs := runZshUTF8(t, "print -rn -- "+tc.probe+"\n")
			if out != tc.want || st != 0 {
				t.Errorf("print -rn -- %s = %q (status %d, stderr %q), want %q",
					tc.probe, out, st, errs, tc.want)
			}
		})
	}
}

// Only a character range renders. An alternative, a class and a bare word
// write the character raw on the same shell, so a rendering put in the
// shared span builder would have moved three constructs that agree today.
func TestOnlyACharacterRangeEscapesHere(t *testing.T) {
	t.Parallel()
	const raw = "\x01"
	for _, tc := range []struct{ name, src, want string }{
		{"a character range", `print -rn -- {$'\x01'..$'\x01'}`, "^A"},
		{"an alternative", `print -rn -- {$'\x01',b}`, raw + " b"},
		{"a bare word", `print -rn -- $'\x01'`, raw},
		{"a numeric range", `print -rn -- {1..2}`, "1 2"},
		// Raw bytes rather than `$'…'`, because a class body holding an
		// escape reaches a separate gap of its own (#5154) and this row is
		// about which construct renders.
		{"a character class", "setopt brace_ccl\nprint -rn -- {\x01\x02}\n", raw + " \x02"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, _, errs := runZshUTF8(t, tc.src+"\n")
			if out != tc.want {
				t.Errorf("%s = %q (stderr %q), want %q", tc.src, out, errs, tc.want)
			}
		})
	}
}

// Above Latin-1 this shell asks the locale rather than a table, and that is
// **not modeled** — these rows are the measurement standing as a record of
// what is unimplemented, not an assertion that our answer is right.
//
// Measured on zsh 5.9.2 in a UTF-8 locale: `$'\u00ad'` (soft hyphen) is
// `\M--` and `$'\u00a0'` (no-break space) is the character, so the split is
// `iswprint`'s and not the codepoint's; `$'\u200b'` and `$'\u0378'` come
// back as the escapes `\u200b` and `\u0378` while `$'\ue000'` is the
// character. We render all four as themselves. Recorded on #5154 and in
// Runner.niceRangeChar.
func TestTheLocaleDependentEscapesAreNotModeledHere(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, probe, ours, reference string }{
		{"soft hyphen", `{$'\u00ad'..$'\u00ad'}`, "\u00ad", `\M--`},
		{"zero width space", `{$'\u200b'..$'\u200b'}`, "\u200b", `\u200b`},
		{"unassigned", `{$'\u0378'..$'\u0378'}`, "\u0378", `\u0378`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, _, _ := runZshUTF8(t, "print -rn -- "+tc.probe+"\n")
			if out != tc.ours {
				t.Errorf("%s = %q, want %q — this row records what we do, and "+
					"the reference answers %q; if this row now matches the "+
					"reference the gap has closed and the test should move",
					tc.probe, out, tc.ours, tc.reference)
			}
		})
	}
}
