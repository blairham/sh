// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// runNounsetElement runs src under `set -u` with the axes a subscript read
// needs answered: the base, whether a gap is an element, and — the one this
// suite is about — whether a subscript on a name holding one string counts
// characters. The diagnostics are a wording with a subject in it, so what the
// refusal *names* is readable from the output.
func runNounsetElement(t *testing.T, chars Answer, names bool, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		sem.ArrayBaseIsZero = Yes
		sem.ArraysAreSparse = Yes
		sem.ArrayScalarIsTheWholeArray = No
		sem.ArrayLengthWithoutSubscriptIsCount = No
		sem.UnsetArraySpan = UnsetArraySpanRemovesTheElements
		sem.ScalarSubscriptIsACharacter = chars
		sem.EmptyParamSubscriptIsAnError = Yes
		r.Semantics = &sem
		diag := Diagnostics{UnboundVariable: "%s: parameter not set"}
		diag.UnboundElementNamesTheSubscriptsValue = names
		r.Diagnostics = &diag
	})
}

// A subscript that named no element is what `set -u` is about, and the refusal
// names the element rather than the array. Every shell in the panel that has
// arrays at all makes it — measured 2026-09-15, `a=(x y z); set -u; echo
// "${a[9]}"` is `a[9]: unbound variable` in bash 5.3 and `a[9]: parameter not
// set` in ksh93u+ and zsh 5.9.2 — and this answered every row with an empty
// string at status 0, which nothing downstream can tell from a real element
// holding nothing (#2911).
//
// `echo after` is in every row because the status alone cannot show it: the
// refusal gives up the script, and a shell that wrote the sentence and carried
// on would look the same without it.
func TestASubscriptThatNamesNoElementIsRefusedUnderNounset(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an index past the end", `a=(x y z); set -u; echo "[${a[9]}]"; echo after`, "a[9]: parameter not set"},
		{"an index below the base", `a=(x y z); set -u; echo "[${a[-9]}]"; echo after`, "a[-9]: parameter not set"},
		{"a name holding nothing at all", `set -u; echo "[${nope[1]}]"; echo after`, "nope[1]: parameter not set"},
		{"an element that was removed", `a=(x y z); unset "a[1]"; set -u; echo "[${a[1]}]"; echo after`, "a[1]: parameter not set"},
		{"a key the table does not have", `typeset -A m; m[k]=v; set -u; echo "[${m[q]}]"; echo after`, "m[q]: parameter not set"},
		{"and the same read under an operator", `a=(x y z); set -u; echo "[${a[9]#x}]"; echo after`, "a[9]: parameter not set"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNounsetElement(t, No, false, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q in it", out, tc.want)
			}
			if strings.Contains(out, "after") {
				t.Errorf("got %q, want the script abandoned", out)
			}
			if st == 0 {
				t.Errorf("status = 0, want a refusal")
			}
		})
	}
}

// What the refusal must *not* reach, and each row is a way it could have been
// made too wide. A whole-array subscript naming no element is an empty list
// rather than an unset parameter — `for` over one runs zero times in every
// column — and an element that is there and empty is set, which is the
// distinction `${a[0]:-d}` already turns on.
//
// The `[@]` and `[*]` rows are on a name that holds *nothing*, which is where
// the whole-array reading is genuinely empty-handed rather than merely empty:
// an array that exists yields a list of no elements, and only an absent name
// yields no list at all. The columns split on it — measured 2026-09-15, zsh
// 5.9.2 and bash 3.2.57 refuse `${nope[@]}` while bash 5.3 and ksh93u+ print
// nothing and carry on — so it is a question of its own rather than this
// refusal reaching one more spelling, and these rows are what keeps the
// answer from being given here by accident.
func TestASubscriptThatNamesNothingMissingIsQuietUnderNounset(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an empty array under [@]", `a=(); set -u; echo "n=${#a[@]} at=[${a[@]}]"; echo after`, "n=0 at=[]\nafter\n"},
		{"a name that holds nothing under [@]", `set -u; echo "at=[${nope[@]}]"; echo after`, "at=[]\nafter\n"},
		{"and under [*]", `set -u; echo "star=[${nope[*]}]"; echo after`, "star=[]\nafter\n"},
		{"an element holding the empty string", `a=(x "" z); set -u; echo "[${a[1]}]"; echo after`, "[]\nafter\n"},
		{"a conditional that supplies a word", `a=(x); set -u; echo "[${a[9]-d}]"; echo after`, "[d]\nafter\n"},
		{"a conditional that asks whether it is set", `a=(x); set -u; echo "[${a[9]+s}]"; echo after`, "[]\nafter\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNounsetElement(t, No, false, tc.src)
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// A subscript on a name holding one *string* is the one place the columns
// part, and it is Semantics.ScalarSubscriptIsACharacter that parts them rather
// than an axis about `set -u`: where the brackets count characters, a position
// past the end of the string is empty and quiet; where they name the elements
// of an array of one, element 5 is missing and refused. Measured 2026-09-15,
// `v=x; set -u; echo "${v[5]}"` is empty at 0 in zsh 5.9.2 and `v[5]: unbound
// variable` in bash 5.3.
func TestACharacterPastTheEndOfAStringIsNotUnsetUnderNounset(t *testing.T) {
	src := `v=x; set -u; echo "[${v[5]}]"; echo after`
	t.Run("counted as characters", func(t *testing.T) {
		out, st := runNounsetElement(t, Yes, false, src)
		if want := "[]\nafter\n"; out != want {
			t.Errorf("out = %q, want %q", out, want)
		}
		if st != 0 {
			t.Errorf("status = %d, want 0", st)
		}
	})
	t.Run("read as an array of one", func(t *testing.T) {
		out, _ := runNounsetElement(t, No, false, src)
		if !strings.Contains(out, "v[5]: parameter not set") || strings.Contains(out, "after") {
			t.Errorf("got %q, want the element refused", out)
		}
	})
}

// Which text the refusal writes back, which only a subscript that had to be
// expanded can ask: measured 2026-09-15 with `i=9; a=(x); set -u; echo
// "${a[$i]}"`, bash 5.3 and zsh 5.9.2 both say `a[$i]` and ksh93u+ says
// `a[9]`. See Diagnostics.UnboundElementNamesTheSubscriptsValue, which also
// records the shape it does not reach.
func TestAnUnboundElementCanBeNamedByTheSubscriptsValue(t *testing.T) {
	src := `i=9; a=(x); set -u; echo "[${a[$i]}]"`
	t.Run("the text that was typed", func(t *testing.T) {
		out, _ := runNounsetElement(t, No, false, src)
		if !strings.Contains(out, "a[$i]: parameter not set") {
			t.Errorf("got %q, want the subscript as written", out)
		}
	})
	t.Run("what the text came to", func(t *testing.T) {
		out, _ := runNounsetElement(t, No, true, src)
		if !strings.Contains(out, "a[9]: parameter not set") || strings.Contains(out, "$i") {
			t.Errorf("got %q, want the subscript's value", out)
		}
	})
}

// A subscript the shell has already refused to read gets one complaint and not
// two. The element is missing in both rows because nothing could name one, and
// a second sentence about it would be this check reporting the first one's
// aftermath.
func TestARefusedSubscriptIsNotAlsoReportedAsUnbound(t *testing.T) {
	for _, tc := range []struct{ name, src, unwanted string }{
		{"an expression that will not evaluate", `a=(x y z); set -u; echo "[${a[b c]}]"`, "a[b c]: parameter not set"},
		{"brackets with nothing in them", `a=(x y z); set -u; echo "[${a[]}]"`, "a[]: parameter not set"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNounsetElement(t, No, false, tc.src)
			if strings.Contains(out, tc.unwanted) {
				t.Errorf("got %q, want no %q in it", out, tc.unwanted)
			}
			if st == 0 {
				t.Errorf("status = 0, want the refusal that was already made")
			}
		})
	}
}
