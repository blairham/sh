// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// TestAnOperatorOverABareArrayRunsBeforeTheJoin — #2326.
//
// An unquoted bare array name in a context that keeps one word has its
// operator applied to each element and the results joined; this joined first
// and applied the operator to the join. The two spellings are each other's
// control, which is the whole of the report: the same characters in the same
// assignment, parting on where the join sits.
//
// Measured on zsh 5.9.2, 2026-09-12:
//
//	y=(ab ab); x=${y#ab}      ` `      both elements empty, and the
//	                                   separator between them is all there is
//	y=(ab ab); x="${y#ab}"    ` ab`    the join runs first and the trim takes
//	                                   one `ab` off the front of the pair
//	z=(x y);   x=${z:/x/Q}    `Q y`    the element-selecting operators are
//	                                   the same claim from the other side
//
// The other three shells with arrays read a bare name as its first element —
// ArrayScalarIsTheWholeArray — so this is reachable only where that axis says
// the bare name is the whole list, which is why it lives here.
func TestAnOperatorOverABareArrayRunsBeforeTheJoin(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			name: "a trim empties both elements",
			src:  `y=(ab ab); x=${y#ab}; printf '[%s]' "$x"`,
			want: "[ ]",
		},
		{
			// The control. Quoted, the join at rule 5 runs first — and this
			// is the answer the unquoted spelling used to give as well.
			name: "and quoted the join runs first",
			src:  `y=(ab ab); x="${y#ab}"; printf '[%s]' "$x"`,
			want: "[ ab]",
		},
		{name: "a suffix trim", src: `y=(ab ab); x=${y%b}; printf '[%s]' "$x"`, want: "[a a]"},
		{
			// Read as the join, `${y/a/Q}` replaces the *first* `a` in
			// `ab ab` and answers `Qb ab`.
			name: "a replacement reaches every element",
			src:  `y=(ab ab); x=${y/a/Q}; printf '[%s]' "$x"`,
			want: "[Qb Qb]",
		},
		{
			name: "an element-selecting operator",
			src:  `z=(x y); x=${z:/x/Q}; printf '[%s]' "$x"`,
			want: "[Q y]",
		},
		{name: "an exclusion", src: `z=(one two three); x=${z:#o*}; printf '[%s]' "$x"`, want: "[two three]"},
		{
			// A slice counts elements under the list reading and characters
			// under the join: `one two three` without its first character is
			// `ne two three`, which is what this answered.
			name: "a slice takes elements", src: `z=(one two three); x=${z:1}; printf '[%s]' "$x"`,
			want: "[two three]",
		},
		{name: "and a bounded slice", src: `z=(one two three); x=${z:0:2}; printf '[%s]' "$x"`, want: "[one two]"},
		{
			// The boundary. With no operator the two readings coincide in a
			// context that keeps one word, which is measured — `IFS=-; v=$a`
			// is `x-y-z` here too — so the scalar path still answers it and
			// the join is on IFS rather than on a space.
			name: "no operator still joins", src: `IFS=-; z=(one two); x=${z}; printf '[%s]' "$x"`,
			want: "[one-two]",
		},
		{
			// And with one, the join that runs after the operator is the
			// same IFS join.
			name: "the join after the operator is IFS's",
			src:  `IFS=-; z=(one two); x=${z#o}; printf '[%s]' "$x"`, want: "[ne-two]",
		},
		{
			// The flag-group path has answered this way since #2290, and a
			// word with no flag group in it must not be able to disagree.
			name: "the flag group agrees",
			src:  `y=(ab ab); a=${(j:+:)y#ab}; b=${y#ab}; printf '[%s][%s]' "$a" "$b"`, want: "[+][ ]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// TestAnEmptyElementSurvivesAJoinThatKeepsOneWord is the half of #2326 the
// row above cannot come out right without: an unquoted list reaching a
// context that keeps one word is *joined*, so an element the split reading
// would drop is a separator here rather than a field.
//
// Written against the positional parameters as well as an array, because
// that spelling is portable and is how the question was put to every column:
// measured 2026-09-12, dash, bash 5.3.15, ksh93u+ and zsh 5.9.2 all answer
// `a  b` to `set -- a "" b; x=$@`. bash 3.2 is the single deviation and only
// on `@` — it drops the element while the same build keeps it for `$*`, so
// the shell disagrees with itself, which is why nothing here models it.
func TestAnEmptyElementSurvivesAJoinThatKeepsOneWord(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		{name: "the positionals", src: `set -- a "" b; x=$@; printf '[%s]' "$x"`, want: "[a  b]"},
		{name: "the star spelling", src: `set -- a "" b; x=$*; printf '[%s]' "$x"`, want: "[a  b]"},
		{name: "nothing but empties", src: `set -- "" ""; x=$@; printf '[%s]' "$x"`, want: "[ ]"},
		{name: "an array", src: `w=(a '' b); x=${w[@]}; printf '[%s]' "$x"`, want: "[a  b]"},
		{name: "a bare array name", src: `w=(a '' b); x=${w}; printf '[%s]' "$x"`, want: "[a  b]"},
		{
			// And the join is still IFS's, so the separator that survives is
			// the one the script chose rather than a hard space.
			name: "under a non-whitespace IFS",
			src:  `IFS=-; set -- a "" b; x=$*; printf '[%s]' "$x"`, want: "[a--b]",
		},
		{
			// The boundary: a context that *keeps* fields still drops it.
			name: "a splitting context still drops it",
			src:  `w=(a '' b); printf '[%s]' $w`, want: "[a][b]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// TestCountingAnEmptyArrayThroughAnInnerExpansion and the refusal beside it
// are the two neighbors #2326 found: both are a name position that a count
// reaches, and both were a plausible value at status 0.
func TestCountingAnEmptyArrayThroughAnInnerExpansion(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// An inner expansion is a caller that can say "no fields", and
			// an empty array is the one value where that differs from a
			// single empty one. Both answered 1.
			name: "an empty array through a subscripted inner",
			src:  `u=(); printf '[%s]' ${#${u}[@]}`, want: "[0]",
		},
		{name: "and through a bare inner", src: `u=(); printf '[%s]' ${#${u}}`, want: "[0]"},
		{name: "a filled one still counts", src: `v=(a b); printf '[%s]' ${#${v}[@]}`, want: "[2]"},
		{name: "and the subscripted spelling agrees", src: `u=(); printf '[%s]' ${#u[@]}`, want: "[0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// TestALengthOverAQuotedInnerIsRefused — the quotes are the whole of it.
//
// Measured on zsh 5.9.2, 2026-09-12, with `v=abc`: `${#${v}}` is `3`,
// `${#$(echo a b)}` is `2` — a *count* of the fields the substitution came
// to — and every quoted spelling of the same inner is `bad substitution`.
// This answered `3` and `1`, which is the plausible-value failure this
// surface's refusals exist to prevent.
//
// The refusal is deferred to the run rather than raised at parse time, which
// is measured too: the same characters inside an `if false` branch are no
// error at all there.
func TestALengthOverAQuotedInnerIsRefused(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src string
		refused   bool
		want      string
	}{
		{name: "a quoted substitution", src: `printf '[%s]' ${#"$(echo abc)"}`, refused: true},
		{name: "a quoted parameter inner", src: `v=abc; printf '[%s]' ${#"${v}"}`, refused: true},
		{name: "a quoted whole-array inner", src: `v=(a b); printf '[%s]' ${#"${v[@]}"}`, refused: true},
		{name: "unquoted, a substitution counts its fields", src: `printf '[%s]' ${#$(echo a b)}`, want: "[2]"},
		{name: "unquoted, a parameter inner measures it", src: `v=abc; printf '[%s]' ${#${v}}`, want: "[3]"},
		{
			// The quote has to have been *written* in the braces. An inner
			// inherits the quoting of whatever encloses the whole expansion,
			// so a reading off the span's Quoting refuses this one too — and
			// zsh answers `3`.
			name: "an enclosing quote is not a written one",
			src:  `v=abc; print -r -- "[${#${v}}]"`, want: "[3]\n",
		},
		{
			// And the idiom the quotes exist for is untouched.
			name: "the split-by-line idiom still parses",
			src:  `printf '[%s]' ${(@f)"$(printf 'a\nb\n')"}`, want: "[a][b]",
		},
		{
			name: "the refusal is deferred to the run",
			src:  `if false; then printf '%s' ${#"$(echo ab)"}; fi; printf 'survived'`, want: "survived",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if tc.refused {
				// The wording is the dialect's and the refusal is the
				// claim, so this asserts on the status and on the shell
				// having said something — never on the value, which is the
				// thing that must not be there.
				if st == 0 || !strings.Contains(out, "bad substitution") ||
					strings.Contains(out, "[") {
					t.Errorf("%s = %q (status %d), want a bad substitution and no value", tc.src, out, st)
				}
				return
			}
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
