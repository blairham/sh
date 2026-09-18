// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// elementAxes is the vector a subscripted read needs answered before any of
// these questions can be asked at all: where an array starts, that a gap is
// an element, and that a bare name is not the whole array.
func elementAxes(sem *Semantics) {
	sem.ArrayBaseIsZero = Yes
	sem.ArraysAreSparse = Yes
	sem.ArrayScalarIsTheWholeArray = No
	sem.ArrayLengthWithoutSubscriptIsCount = No
	sem.UnsetArraySpan = UnsetArraySpanRemovesTheElements
	sem.ScalarSubscriptIsACharacter = No
	sem.EmptyParamSubscriptIsAnError = Yes
}

// runErrorOperator runs src with one answer to
// Semantics.ErrorOperatorSeesOnlyTheBareElement and one to
// Diagnostics.ParamErrorNamesTheArray, so the operator's two halves can be
// moved apart.
func runErrorOperator(t *testing.T, bare Answer, namesArray bool, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		elementAxes(&sem)
		sem.ErrorOperatorSeesOnlyTheBareElement = bare
		r.Semantics = &sem
		r.Diagnostics = &Diagnostics{
			UnboundVariable:         "%s: parameter not set",
			ParamErrorMessage:       "%[1]s: %[2]s",
			ParamErrorNamesTheArray: namesArray,
		}
	})
}

// `${a[9]?word}` names the element the brackets reached, exactly as `set -u`
// does one branch away. This wrote the bare array for every dialect, so a
// script's `${cfg[port]?port is required}` reported `cfg` and told its reader
// the wrong thing was missing (#3241).
func TestTheErrorOperatorNamesTheElementItsSubscriptReached(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an index past the end", `a=(x y z); echo "[${a[9]?m}]"`, "a[9]: m"},
		{"a name holding nothing at all", `echo "[${nope[1]?m}]"`, "nope[1]: m"},
		{"a key the table does not have", `typeset -A m; m[k]=v; echo "[${m[q]?m}]"`, "m[q]: m"},
		{"the colon form of the same", `a=(x y z); echo "[${a[9]:?m}]"`, "a[9]: m"},
		{"a subscript that had to be expanded", `i=9; a=(x); echo "[${a[$i]?m}]"`, "a[$i]: m"},
		{"and no subscript is the control", `echo "[${z?m}]"`, "z: m"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runErrorOperator(t, No, false, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q in it", out, tc.want)
			}
			if st == 0 {
				t.Errorf("status = 0, want a refusal")
			}
		})
	}
}

// The other answer to the subject: one column writes the array back and drops
// the brackets, for the colon form that is the only one it reaches.
func TestTheErrorOperatorCanNameTheArrayInstead(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an index past the end", `a=(x y z); echo "[${a[9]:?m}]"`, "a: m"},
		{"a name holding nothing at all", `echo "[${nope[1]:?m}]"`, "nope: m"},
		{"a key the table does not have", `typeset -A m; m[k]=v; echo "[${m[q]:?m}]"`, "m: m"},
		{"and no subscript is the control", `echo "[${z:?m}]"`, "z: m"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runErrorOperator(t, Yes, true, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q in it", out, tc.want)
			}
			if strings.Contains(out, "[") && strings.Contains(out, "]") {
				t.Errorf("got %q, want no subscript written back", out)
			}
			if st == 0 {
				t.Errorf("status = 0, want a refusal")
			}
		})
	}
}

// The firing half. Where the colon-less `?` reaches only the element a bare
// read of the name means, brackets pointing anywhere else leave it quiet —
// and the last two rows are what keep that from being "brackets switch the
// operator off": the subscript that *does* name the bare element refuses, and
// so does the spelling with no subscript at all.
func TestTheErrorOperatorCanReachOnlyTheBareElement(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		refused         bool
	}{
		{name: "an index past the end", src: `a=(x y z); echo "[${a[9]?m}]"`, want: "[]\n"},
		{name: "a name holding nothing at all", src: `echo "[${nope[1]?m}]"`, want: "[]\n"},
		{name: "a key the table does not have", src: `typeset -A m; m[k]=v; echo "[${m[q]?m}]"`, want: "[]\n"},
		{name: "a subscript on a name holding one string", src: `v=abc; echo "[${v[9]?m}]"`, want: "[]\n"},
		{name: "the base subscript on an absent name", src: `echo "[${nope[0]?m}]"`, want: "nope: m", refused: true},
		{name: "an expression that comes to the base", src: `echo "[${nope[1-1]?m}]"`, want: "nope: m", refused: true},
		{name: "the base subscript on a removed element", src: `a=(x y z); unset "a[0]"; echo "[${a[0]?m}]"`, want: "a: m", refused: true},
		{name: "no subscript at all", src: `echo "[${nope?m}]"`, want: "nope: m", refused: true},
		{name: "and the base subscript on an element that is there", src: `a=(x y z); echo "[${a[0]?m}]"`, want: "[x]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runErrorOperator(t, Yes, true, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q in it", out, tc.want)
			}
			if tc.refused != (st != 0) {
				t.Errorf("status = %d, refused = %v", st, tc.refused)
			}
		})
	}
}

// The sharper control, and the reason this is the `?` operator's own axis
// rather than a reading of a subscript: the neighboring conditionals see the
// missing element in every column, including the one that answers this axis
// yes.
func TestTheOtherConditionalsStillSeeTheMissingElement(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the default operator", `a=(x y z); echo "[${a[9]-D}]"`, "[D]\n"},
		{"the alternate operator", `a=(x y z); echo "[${a[9]+S}]"`, "[]\n"},
		{"the default on an absent name", `echo "[${nope[1]-D}]"`, "[D]\n"},
		{"the alternate on an absent name", `echo "[${nope[1]+S}]"`, "[]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runErrorOperator(t, Yes, true, tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// runNounsetSubscript runs src under `set -u` with the two subscripted
// refusals answered separately, so each can be moved on its own.
func runNounsetSubscript(t *testing.T, length, wholeArray Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, nil, func(r *Runner) {
		sem := *r.Semantics
		elementAxes(&sem)
		sem.LengthOfAMissingElementIsRefused = length
		sem.UnsetNameWithAWholeArraySubscriptIsRefused = wholeArray
		r.Semantics = &sem
		r.Diagnostics = &Diagnostics{UnboundVariable: "%s: parameter not set"}
	})
}

// A length is counted before any refusal is reached, so `set -u` was never
// asked about one at all: `${#a[9]}` and `${#nope[9]}` were both `0` at status
// 0, and the second of those is refused in every column that has arrays
// (#2980).
func TestTheLengthOfAMissingElementIsAskedOfNounset(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		refused         bool
		length          Answer
	}{
		{
			name: "an element that is not there, where the dialect refuses it", length: Yes,
			src: `a=(x y z); set -u; echo "[${#a[9]}]"; echo after`, want: "a[9]: parameter not set", refused: true,
		},
		{
			name: "the same where it does not", length: No,
			src: `a=(x y z); set -u; echo "[${#a[9]}]"; echo after`, want: "[0]\nafter\n",
		},
		{
			name: "a name that is not there at all", length: No,
			src: `set -u; echo "[${#nope[9]}]"; echo after`, want: "nope[9]: parameter not set", refused: true,
		},
		{
			name: "and the same with the axis the other way", length: Yes,
			src: `set -u; echo "[${#nope[9]}]"; echo after`, want: "nope[9]: parameter not set", refused: true,
		},
		{
			name: "a declared table with no key of that name", length: No,
			src: `typeset -A m; m[k]=v; set -u; echo "[${#m[q]}]"; echo after`, want: "[0]\nafter\n",
		},
		{
			name: "a count is not a length", length: Yes,
			src: `set -u; echo "[${#nope[@]}]"; echo after`, want: "[0]\nafter\n",
		},
		{
			name: "and an element that is there is neither", length: Yes,
			src: `a=(xx y z); set -u; echo "[${#a[0]}]"; echo after`, want: "[2]\nafter\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNounsetSubscript(t, tc.length, No, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q in it", out, tc.want)
			}
			if tc.refused != (st != 0) {
				t.Errorf("status = %d, refused = %v", st, tc.refused)
			}
		})
	}
}

// A whole-array subscript on a name holding nothing at all is a question
// about existence, and the columns split on it. The empty-array rows are the
// control: a name that is there and holds no elements is quiet whichever way
// the axis is answered (#2980).
func TestAWholeArraySubscriptOnAnAbsentNameIsAskedOfNounset(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		refused         bool
		whole           Answer
	}{
		{
			name: "an absent name, where the dialect refuses it", whole: Yes,
			src: `set -u; echo "[${nope[@]}]"; echo after`, want: "nope[@]: parameter not set", refused: true,
		},
		{
			name: "the star spelling of the same", whole: Yes,
			src: `set -u; echo "[${nope[*]}]"; echo after`, want: "nope[*]: parameter not set", refused: true,
		},
		{
			name: "the same where it does not", whole: No,
			src: `set -u; echo "[${nope[@]}]"; echo after`, want: "[]\nafter\n",
		},
		{
			name: "an array that is there and empty", whole: Yes,
			src: `b=(); set -u; echo "[${b[@]}]"; echo after`, want: "[]\nafter\n",
		},
		{
			name: "a table that is there and empty", whole: Yes,
			src: `typeset -A t; set -u; echo "[${t[@]}]"; echo after`, want: "[]\nafter\n",
		},
		{
			name: "and an element subscript is not this question", whole: Yes,
			src: `b=(); set -u; echo "[${b[0]-D}]"; echo after`, want: "[D]\nafter\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNounsetSubscript(t, No, tc.whole, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q in it", out, tc.want)
			}
			if tc.refused != (st != 0) {
				t.Errorf("status = %d, refused = %v", st, tc.refused)
			}
		})
	}
}

// The `set -u` sigil follows the **spelling**: a positional written in braces
// is named without the `$` the same parameter wears without them. Not an axis
// — one column writes a sigil at all — so the two wordings this already has
// are what the spelling chooses between (#3466).
func TestABracedPositionalIsNamedWithoutItsSigil(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a bare positional", `set -u; echo "[$7]"`, "$7: unbound variable"},
		{"the braced spelling", `set -u; echo "[${7}]"`, "7: unbound variable"},
		{"a braced positional of two digits", `set -u; echo "[${10}]"`, "10: unbound variable"},
		{"the last background pid, bare", `set -u; echo "[$!]"`, "$!: unbound variable"},
		{"and braced", `set -u; echo "[${!}]"`, "!: unbound variable"},
		{"a name is the control, bare", `set -u; echo "[$x]"`, "x: unbound variable"},
		{"and braced", `set -u; echo "[${x}]"`, "x: unbound variable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, func(d *syntax.Dialect) {
				d.ParamIndirection = false
			}, func(r *Runner) {
				sem := *r.Semantics
				sem.UnsetPositionalIsAllowed = No
				sem.LastBackgroundPidIsUnsetBeforeAnyJob = Yes
				r.Semantics = &sem
				r.Diagnostics = &Diagnostics{
					UnboundVariable:   "%s: unbound variable",
					UnboundPositional: "$%s: unbound variable",
				}
			})
			// The whole sentence and not a substring of it: `7: unbound
			// variable` is inside `$7: unbound variable`, so a Contains
			// against the braced row passes for the spelling it is there to
			// rule out. Found by mutation — dropping the spelling test
			// changed no answer this suite could see.
			if !strings.HasSuffix(out, ": "+tc.want+"\n") {
				t.Errorf("got %q, want the refusal to name exactly %q", out, tc.want)
			}
			if st == 0 {
				t.Errorf("status = 0, want a refusal")
			}
		})
	}
}

// Where `${!x}` yields the **name** it was written on the value is never
// read, so `set -u` has nothing to be about unless the parameter itself is
// absent. Rows one and three are the controls on either side: a wholly absent
// name is still refused, and the same subscript without the `!` is refused
// too — so this is the indirection's split and not the subscript's (#3242).
func TestTheNameYieldingIndirectionRefusesOnlyAnAbsentName(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		refused         bool
	}{
		{
			name: "an absent name with no subscript", src: `set -u; echo "[${!v}]"; echo after`,
			want: "v: parameter not set", refused: true,
		},
		{
			name: "an absent name with a subscript", src: `set -u; echo "[${!nosucharr[9]}]"; echo after`,
			want: "nosucharr[9]: parameter not set", refused: true,
		},
		{
			name: "an array that is there", src: `a=(x y z); set -u; echo "[${!a[9]}]"; echo after`,
			want: "[a[9]]\nafter\n",
		},
		{
			name: "a table that is there", src: `typeset -A m; m[k]=1; set -u; echo "[${!m[zz]}]"; echo after`,
			want: "[m[zz]]\nafter\n",
		},
		{
			name: "and the same subscript without the indirection", src: `a=(x y z); set -u; echo "[${a[9]}]"; echo after`,
			want: "a[9]: parameter not set", refused: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, func(d *syntax.Dialect) {
				d.ParamIndirection = true
			}, func(r *Runner) {
				sem := *r.Semantics
				elementAxes(&sem)
				sem.IndirectionYieldsName = Yes
				r.Semantics = &sem
				r.Diagnostics = &Diagnostics{UnboundVariable: "%s: parameter not set"}
			})
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q in it", out, tc.want)
			}
			if tc.refused != (st != 0) {
				t.Errorf("status = %d, refused = %v", st, tc.refused)
			}
		})
	}
}
