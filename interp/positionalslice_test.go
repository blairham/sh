// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// A slice of `$@` or `$*` takes the *parameters*, and it counts over
// `$0 $1 … $n` — so `${@:1}` is every parameter and `${@:0}` is the shell's own
// name in front of them.
//
// It took a substring of the parameters *joined* instead: on `set -- ax bx cx`,
// `${@:1:2}` was `[x ]` — the string `ax bx cx` cut from offset 1 for two
// characters — where every shell with the construct answers `[ax][bx]`. Quoted,
// that one string was also one field, so `"${@:2}"` could not forward arguments
// at all (#1589).
//
// Measured 2026-09-10 on `set -- ax bx cx` against bash 5.3.15 and zsh 5.9.2,
// which agree on every row below. ksh93 has the slice too; dash refuses it as a
// bad substitution, so its answer is a refusal rather than a disagreement and
// this is a core rule rather than a Semantics axis.
//
// The offset is not 1-based, which is the part worth stating because it looks
// like it is: the positional list simply has one more element at the front.
// `${@:0}` is `$0` and all three parameters, `${@:1}` is the three, and
// `${a[@]:1}` on a three-element array drops one — the same counting over a
// list that is one shorter.
func TestASliceOfThePositionalParametersTakesTheParameters(t *testing.T) {
	for _, c := range []struct {
		src, want, why string
	}{
		{
			src:  `set -- ax bx cx; printf '[%s]' ${@:1:2}`,
			want: "[ax][bx]",
			why:  "offset 1 is $1, so a length of two is $1 and $2",
		},
		{
			src:  `set -- ax bx cx; printf '[%s]' "${@:1:2}"`,
			want: "[ax][bx]",
			why:  "quoting keeps one field per parameter, as it does for a bare \"$@\"",
		},
		{
			src:  `set -- ax bx cx; printf '[%s]' "${@:1}"`,
			want: "[ax][bx][cx]",
			why:  "no length is every parameter from the offset on",
		},
		{
			src:  `set -- ax bx cx; printf '[%s]' "${@:2}"`,
			want: "[bx][cx]",
			why:  "the idiom that forwards every argument but the first",
		},
		{
			src:  `set -- ax bx cx; printf '[%s]' "${@:3}"`,
			want: "[cx]",
			why:  "the last parameter alone",
		},
		{
			src:  `set -- ax bx cx; printf '[%s]' "${@: -1}"`,
			want: "[cx]",
			why:  "a negative offset counts back from the end, which the extra front element never reaches",
		},
		{
			src:  `set -- ax bx cx; printf '[%s]' "${@: -2}"`,
			want: "[bx][cx]",
			why:  "and two back from the end is the last two parameters, not one of them and $0",
		},
		// `[]` rather than nothing, and that is printf and not the slice:
		// the format runs once whatever it is handed, so these two rows say
		// the expansion produced no *value* and cannot say whether it
		// produced no field. The field count for both is asserted in
		// TestTheFieldCountOfAPositionalSlice, which is where that shape can
		// be seen. Measured `[]` in bash 5.3.15 and zsh 5.9.2 alike.
		{
			src:  `set -- ax bx cx; printf '[%s]' "${@:9}"`,
			want: "[]",
			why:  "an offset past the end is quiet, at status 0",
		},
		{
			src:  `set -- ax bx cx; printf '[%s]' "${@:1:0}"`,
			want: "[]",
			why:  "a length of zero is not the whole list",
		},
		// `$*` joins in quotes and splits without them, which is the same
		// division the two spellings keep with no operator at all. A rewrite
		// that gave both names `[@]` would answer this row with two fields.
		{
			src:  `set -- ax bx cx; printf '[%s]' "${*:1:2}"`,
			want: "[ax bx]",
			why:  "`$*` joins the slice into one field when quoted",
		},
		{
			src:  `set -- ax bx cx; printf '[%s]' ${*:1:2}`,
			want: "[ax][bx]",
			why:  "and unquoted it is field-split back apart",
		},
	} {
		out, st := run(t, c.src, nil)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%s = %q, want %q — %s", c.src, got, c.want, c.why)
		}
		if st != 0 {
			t.Errorf("%s: status = %d, want 0", c.src, st)
		}
	}
}

// The field *count* is the half a contents check cannot see, and the contents
// are the half a count cannot see. Both are asserted because each hid this bug
// from a probe built on the other:
//
//   - `"${@:2}"` on `a b c` gave the right three-ish shape unquoted, because a
//     joined `x bx cx` field-splits back into three words.
//   - `${@:1}` unquoted gave the right *count* of three with the wrong contents,
//     `[x][bx][cx]` for `[ax][bx][cx]`, because only the first word was eaten.
//
// Counted through `set --` rather than `printf`, which runs its format once
// whatever it is handed.
func TestTheFieldCountOfAPositionalSlice(t *testing.T) {
	for _, c := range []struct {
		src, want, why string
	}{
		{
			src:  `set -- ax bx cx; set -- "${@:2}"; echo $#`,
			want: "2",
			why:  "two parameters forwarded, not one joined field",
		},
		{
			src:  `set -- a "b b" c; set -- "${@:2}"; echo $#`,
			want: "2",
			why:  "a parameter holding a space stays one parameter",
		},
		{
			src:  `set -- a "b b" c; printf '[%s]' "${@:2}"`,
			want: "[b b][c]",
			why:  "and its space survives, which is the whole reason quoting keeps the fields",
		},
		{
			src:  `set -- ax bx cx; set -- "${*:2}"; echo $#`,
			want: "1",
			why:  "`$*` is the one that joins",
		},
		{
			src:  `set --; set -- "${@:1}"; echo $#`,
			want: "0",
			why:  "no parameters is no field, not one empty one",
		},
		{
			src:  `set -- ax bx cx; set -- "${@:9}"; echo $#`,
			want: "0",
			why:  "an offset past the end is no field — the row printf could not tell apart",
		},
		{
			src:  `set -- ax bx cx; set -- "${@:1:0}"; echo $#`,
			want: "0",
			why:  "and so is a length of zero",
		},
	} {
		out, st := run(t, c.src, nil)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%s = %q, want %q — %s", c.src, got, c.want, c.why)
		}
		if st != 0 {
			t.Errorf("%s: status = %d, want 0", c.src, st)
		}
	}
}

// Offset zero reaches `$0`, which is what says the list being counted really is
// `$0 $1 … $n` rather than the parameters with a 1-based offset. Compared against
// `$0` itself rather than against a literal, because what `$0` holds depends on
// how the shell was started.
func TestOffsetZeroOfAPositionalSliceIsDollarZero(t *testing.T) {
	for _, c := range []struct {
		src, want, why string
	}{
		{
			src:  `set -- ax bx cx; [ "${@:0:1}" = "$0" ] && echo same`,
			want: "same",
			why:  "element zero of the positional list is $0",
		},
		{
			src:  `set -- ax bx cx; set -- "${@:0}"; echo $#`,
			want: "4",
			why:  "$0 and three parameters, one more field than ${@:1} gives",
		},
		{
			src:  `set -- ax bx cx; set -- "${@:1}"; echo $#`,
			want: "3",
			why:  "and offset one leaves $0 out, which is the pair that pins the counting",
		},
	} {
		out, st := run(t, c.src, nil)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%s = %q, want %q — %s", c.src, got, c.want, c.why)
		}
		if st != 0 {
			t.Errorf("%s: status = %d, want 0", c.src, st)
		}
	}
}

// The extra element is the *slice's* alone. Every other operator distributes
// over the parameters and `$0` is not one of them — measured, `${@#a}` on a
// list of three is three fields — so a change that put `$0` where the elements
// are fetched rather than where they are sliced would show up here.
func TestDollarZeroIsNotInTheListForAnyOtherOperator(t *testing.T) {
	for _, c := range []struct {
		src, want, why string
	}{
		{
			src:  `set -- ax bx cx; set -- "${@#a}"; echo $#`,
			want: "3",
			why:  "a trim distributes over the parameters only",
		},
		{
			src:  `set -- ax bx cx; printf '[%s]' "${@#a}"`,
			want: "[x][bx][cx]",
			why:  "and $0 is nowhere in what comes back",
		},
		{
			src:  `set -- ax bx cx; set -- "$@"; echo $#`,
			want: "3",
			why:  "nor with no operator at all",
		},
	} {
		out, st := run(t, c.src, nil)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%s = %q, want %q — %s", c.src, got, c.want, c.why)
		}
		if st != 0 {
			t.Errorf("%s: status = %d, want 0", c.src, st)
		}
	}
}
