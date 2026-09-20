// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// namerefElementSemantics is namerefAimSemantics with the two builtins this
// file is about given their operands, and with the axis left for each case to
// set: what `export` and `readonly` do with a reference aimed at one element.
//
// Everything a case walks past on the way is answered here rather than in the
// cases, so that a row asserts the axis and not the scaffolding.
func namerefElementSemantics(answer Answer) Semantics {
	sem := namerefAimSemantics()
	sem.ExportOrReadonlyTakesAReferenceToAnElement = answer
	sem.ExportOptions = "np"
	sem.ReadonlyOptions = "p"
	sem.ExportListing = DeclareListingClustered
	sem.ReadonlyListing = DeclareListingClustered
	sem.ReadonlyDeclaresALocal = No
	// Walked past by the appending declaration: whose name a refusal reached
	// through a reference carries. Nothing here reaches a refusal of that
	// kind, and the axis is its own subject — see
	// Semantics.DeclarationThroughAReferenceNamesTheOperand.
	sem.DeclarationThroughAReferenceNamesTheOperand = Yes
	return sem
}

// The two answers, and the two things that part between them: whether the
// attribute lands on the container, and whether anything is said.
//
// The **value** does not part. Both columns write the element the reference
// names, which is why it is asserted on every row here — this shell wrote
// element 0 under both answers, silently and at status 0, because the
// operand's name had already been replaced with the array's for the attribute
// and the value went along with it (#3881).
func TestExportOverAReferenceToAnElement(t *testing.T) {
	const src = `a=(p q r)
typeset -n b='a[1]'
export b=Z
echo "st=$? [${a[*]}]"
typeset -p a`
	for _, tc := range []struct {
		name   string
		answer Answer
		want   []string
		absent []string
	}{
		{
			name:   "taken, and the letter lands on the container",
			answer: Yes,
			want:   []string{"st=0 [p Z r]", "declare -ax a="},
			absent: []string{"not a valid identifier"},
		},
		{
			name:   "refused by the target's own text, and the value still lands",
			answer: No,
			want: []string{
				"export: `a[1]': not a valid identifier",
				"st=0 [p Z r]",
				"declare -a a=",
			},
			absent: []string{"declare -ax a="},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := namerefElementSemantics(tc.answer)
			out, _ := runNamerefWith(t, sem, src)
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("got %q, want it to contain %q", out, w)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(out, a) {
					t.Errorf("got %q, want it not to contain %q", out, a)
				}
			}
		})
	}
}

// `readonly` is the row that costs a script something, and it is the freeze
// rather than the sentence that costs it: under the taking answer the whole
// array is frozen and the next ordinary write to any element is refused,
// under the refusing one the array is never frozen at all.
func TestReadonlyOverAReferenceToAnElement(t *testing.T) {
	const src = `a=(p q r)
typeset -n b='a[1]'
readonly b=Y
echo "st=$? [${a[*]}]"
a[2]=NEW
echo "after=$? [${a[*]}]"`
	t.Run("taken: the container is frozen", func(t *testing.T) {
		out, _ := runNamerefWith(t, namerefElementSemantics(Yes), src)
		if !strings.Contains(out, "st=0 [p Y r]") {
			t.Errorf("got %q, want the element written at 0", out)
		}
		if strings.Contains(out, "after=0 [p Y NEW]") {
			t.Errorf("got %q, want the later element write refused", out)
		}
	})
	t.Run("refused: nothing is frozen", func(t *testing.T) {
		out, _ := runNamerefWith(t, namerefElementSemantics(No), src)
		if !strings.Contains(out, "readonly: `a[1]': not a valid identifier") {
			t.Errorf("got %q, want the identifier complaint under readonly", out)
		}
		if !strings.Contains(out, "st=0 [p Y r]") {
			t.Errorf("got %q, want the element written at 0", out)
		}
		if !strings.Contains(out, "after=0 [p Y NEW]") {
			t.Errorf("got %q, want the later element write taken", out)
		}
	})
}

// An append through a reference aimed at an element joins that element, and
// it is the **same read** at both spellings: the bare `b+=X` statement and
// the declaration's `typeset b+=Y` operand.
//
// No axis — both shells that spell a reference answer alike, measured
// 2026-09-20 on bash 5.3.20 and ksh93u+. Each route had a copy of "what does
// this name hold" and both copies asked for the reference's own name, which
// is the empty string: the right cell was written with the wrong text, at
// status 0, with nothing said (#3880). See Runner.storedVar.
func TestAnAppendThroughAReferenceToAnElementJoinsIt(t *testing.T) {
	sem := namerefElementSemantics(Yes)
	sem.DeclarationTakesAnAppendOperand = Yes
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "the bare statement",
			src:  `a=(p q r); typeset -n b='a[1]'; b+=X; echo "st=$? [${a[*]}]"`,
			want: "st=0 [p qX r]",
		},
		{
			name: "the declaration's operand",
			src:  `a=(p q r); typeset -n b='a[1]'; typeset b+=Y; echo "st=$? [${a[*]}]"`,
			want: "st=0 [p qY r]",
		},
		{
			// The read was right before either fix, which is what said the
			// append's own read was the missing half rather than the
			// resolution the store already does.
			name: "the control: a read of the same reference",
			src:  `a=(p q r); typeset -n b='a[1]'; echo "[$b]"`,
			want: "[q]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runNamerefWith(t, sem, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q", out, tc.want)
			}
		})
	}
}
