// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A produced *array* a script may write to needs somewhere for the write to
// go, which is the whole of SetDynamicArrayWriter.
//
// The failure it prevents is silent, and it is the one the association seam
// beside this documents: the producer answers ahead of the store, so a write
// that landed in the store would be accepted with nothing about the name
// changing and no diagnostic written — and then read back as whatever the
// producer says. A script would simply find that assigning to the parameter
// did nothing.
//
// Four spellings arrive at the writer and they are asserted together, because
// they are four different code paths into one store and a hook written beside
// only one of them would leave the other three landing nowhere: the whole
// array, an append, one element, and an `unset`.
func TestAWriteToAProducedArrayReachesItsWriter(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	held := []string{"a", "b"}
	r.SetDynamicArray("view", func(*Runner) []string {
		return append([]string(nil), held...)
	})
	r.SetDynamicArrayWriter("view", func(_ *Runner, values []string) {
		// Deliberately marked, so the test can tell a write that went
		// through the hook from one that went into a stored array.
		held = append([]string{"through"}, values...)
	})
	runSeam(t, r, `printf '[%s]' "${view[@]}"
view=(x y)
printf '[%s]' "${view[@]}"`)
	want := "[a][b][through][x][y]"
	if out.String() != want {
		t.Errorf("a whole-array write through a producer = %q, want %q", out.String(), want)
	}
}

// The other three routes into the same store. Each starts from what the
// *producer* reports rather than from an empty array, which is the half that
// a store-reading implementation gets wrong: an element assignment on a
// produced name began at nothing, so the write landed at the subscript it
// named and every element the producer would have reported was gone.
func TestEveryWriteToAProducedArrayStartsFromWhatItHolds(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an element assignment", `view[2]=Z; printf '[%s]' "${view[@]}"`, "[a][Z][c]"},
		{"an append", `view+=(d); printf '[%s]' "${view[@]}"`, "[a][b][c][d]"},
		{"an unset", `unset view; printf '[%s]' "${view[@]}" "end"`, "[end]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs strings.Builder
			r := seamRunner(t, &out, &errs)
			held := []string{"a", "b", "c"}
			r.SetDynamicArray("view", func(*Runner) []string {
				return append([]string(nil), held...)
			})
			r.SetDynamicArrayWriter("view", func(_ *Runner, values []string) {
				held = append([]string(nil), values...)
			})
			runSeam(t, r, tc.src)
			if out.String() != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out.String(), tc.want)
			}
		})
	}
}

// And a subscript on the left is an *element* of the produced array, not a
// character of its scalar view. The name has nothing in the array store to say
// it is an array, so a reading that consults only that store found the joined
// value instead and spliced a character into a stray stored name — with the
// producer going on answering every read, so nothing said the write had gone
// nowhere.
func TestASubscriptOnAProducedArrayIsAnElement(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	held := []string{"aa", "bb"}
	r.SetDynamicArray("view", func(*Runner) []string {
		return append([]string(nil), held...)
	})
	r.SetDynamicArrayWriter("view", func(_ *Runner, values []string) {
		held = append([]string(nil), values...)
	})
	runSeam(t, r, `view[1]=Z
printf '[%s]' "${view[@]}"`)
	want := "[Z][bb]"
	if out.String() != want {
		t.Errorf("a subscript on a produced array = %q, want %q", out.String(), want)
	}
}

// An array *literal* through a subscript splices into a produced array, and
// the rule that lets it is keyed on the **producer** rather than on whichever
// parameter a dialect happens to produce.
//
// That distinction is the whole reason this test is here and not only beside
// the dialect that found it. The report was about one shell's positional
// parameters, and "the positional parameters splice" and "a produced array
// splices" agree on every row that shell can write: the positionals are the
// only array it produces *and* lets a script assign one element of. `view` is
// nobody's positional parameters, so it holds the producer fixed and takes the
// parameters away — and it refused the literal for exactly as long as the
// question "is this name an array" was asked of the store, where a produced
// name has nothing.
//
// The four rows are the four shapes the splice has, and each one is a
// different arithmetic on the length: grow, shrink, a span, and an append that
// keeps the element it follows. A reading that replaced the whole array would
// pass none of them; a reading that inserted after the element would pass the
// first and fail the rest.
func TestAnArrayLiteralThroughASubscriptSplicesAProducedArray(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"one element becomes two", `view[2]=(p q)`, "[a][p][q][c]"},
		{"one element becomes none", `view[2]=()`, "[a][c]"},
		{"a span becomes three", `view[2,3]=(p q r)`, "[a][p][q][r]"},
		{"an append keeps the element", `view[2]+=(p)`, "[a][b][p][c]"},
		// The scalar spelling of the same span, which carries one word. A
		// produced name read as a plain string here spliced *characters* of
		// its joined elements instead, so the two spellings of one construct
		// came apart.
		{"a span carrying one word", `view[2,3]=Z`, "[a][Z]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs strings.Builder
			r := spliceSeamRunner(t, &out, &errs)
			held := []string{"a", "b", "c"}
			r.SetDynamicArray("view", func(*Runner) []string {
				return append([]string(nil), held...)
			})
			r.SetDynamicArrayWriter("view", func(_ *Runner, values []string) {
				held = append([]string(nil), values...)
			})
			runSeam(t, r, tc.src+"\nprintf '[%s]' \"${view[@]}\"")
			if out.String() != tc.want {
				t.Errorf("%s = %q, want %q (stderr %q)", tc.src, out.String(), tc.want, errs.String())
			}
		})
	}
}

// spliceSeamRunner is seamRunner with the axes an array literal through a
// subscript asks: what the literal means, whether a comma in a subscript
// separates a pair, and which number the first element answers to.
// Unanswered, each reports itself rather than acting, which would be a
// diagnostic in place of the write under test.
func spliceSeamRunner(t *testing.T, out, errs *strings.Builder) *Runner {
	t.Helper()
	dg := Diagnostics{Location: LocationTightLine, BuiltinLocation: LocationTightLine}
	sem := Semantics{
		FatalErrorStatusIsOne:   Yes,
		SubscriptedArrayLiteral: SubscriptedArrayLiteralSplices,
		SubscriptCommaIsARange:  Yes,
		// The base a written subscript counts from, which the splice asks at
		// the edge where a script wrote a number. `No` is the shell this
		// miniature dialect resembles; the rows below are written in it.
		ArrayBaseIsZero: No,
	}
	return newTestRunner(t, &Runner{
		Stdout: out, Stderr: errs, Diagnostics: &dg, Semantics: &sem, Name: "testsh",
	})
}
