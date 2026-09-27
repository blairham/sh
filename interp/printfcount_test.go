// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runCount runs a snippet with the two streams apart and the printf axes
// under the test's control, so that a row states the axis it depends on
// rather than a shell that happens to hold it.
func runCount(t *testing.T, src string, tweak func(*Semantics)) (out, errOut string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := CoreSemantics()
	// The axes an output operand is judged by, which these rows are not
	// about: a name is a plain name here unless a row says otherwise, and
	// the array the subscript rows reach is indexed from zero.
	sem.ReadNameOperands = PlainNamesOnly
	sem.StoreOperandTakesASubscript = Yes
	sem.ArrayBaseIsZero = Yes
	// And what a refusal costs the script, which is a question of its own:
	// these rows are about what `printf` does with the refusal, so the
	// script is left standing for the next command to be read.
	sem.BadNameToPrintfFatal = No
	sem.ReadonlyRefusalInABuiltinIsFatal = No
	sem.PrintfCountConversion = Yes
	sem.PrintfCountAttribute = PrintfCountLeavesTheAttribute
	sem.PrintfCountOperandTakesASubscript = No
	sem.PrintfCountEmptyNameIsIgnored = Yes
	sem.PrintfCountBadNameStopsThePass = Yes
	sem.PrintfCountFrozenNameStopsThePass = Yes
	if tweak != nil {
		tweak(&sem)
	}
	var o, e bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &o, Stderr: &e, Semantics: &sem})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return o.String(), e.String(), st
}

// `%n` writes nothing and stores the bytes the pass has produced.
//
// The store is read back from the parameter rather than from the output,
// because that is the only thing the directive does: a row that compared what
// was printed would pass for a shell that ignored the directive outright.
//
// Semantics.PrintfCountConversion is the axis; the counts themselves are
// unanimous in every column that has the directive, so they are asked on the
// common path.
func TestTheCountConversionStoresTheBytesOfItsPass(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		// Ten digits of precision, the point, the leading digit and `e+00`.
		// A number worth checking rather than a coincidence: an
		// implementation that stored the operand count, or the format's own
		// length, would answer something else.
		{
			"the bytes a conversion produced",
			`printf '%.10e%n' 1 c; printf '[%s]' "$c"`,
			"[16]",
		},
		{
			"the bytes in front of it, and none behind",
			`printf 'abcd%nefgh' c; printf '[%s]' "$c"`,
			"[4]",
		},
		{
			"nothing in front of it",
			`printf '%nxy' c; printf '[%s]' "$c"`,
			"[0]",
		},
		// Bytes and not characters, which is the one reading a Go
		// implementation would most easily get wrong.
		{
			"a multibyte character counts its bytes",
			`printf 'αβ%n' c; printf '[%s]' "$c"`,
			"[4]",
		},
		{
			"an escape counts what it produced",
			"printf 'a\\tb%n' c; printf '[%s]' \"$c\"",
			"[3]",
		},
		// The *pass* and not the builtin: a running total would leave 1, 2
		// and 3 here.
		{
			"the count starts again on every pass",
			`printf '%s%n' a x b y c z; printf '[%s%s%s]' "$x" "$y" "$z"`,
			"[111]",
		},
		// Read and then ignored, with nothing laid out. A directive that
		// went through the field machinery would have padded what it writes,
		// which is nothing.
		{
			"a width is ignored",
			`printf 'xy%5n' c; printf '[%s]' "$c"`,
			"[2]",
		},
		{
			"a precision is ignored",
			`printf 'xy%.3n' c; printf '[%s]' "$c"`,
			"[2]",
		},
		// An operand that is not there at all is not an error anywhere, and
		// the directive still consumes its place in the operand list.
		{
			"an absent operand is silent",
			`printf 'abc%n'; st=$?; printf '[%d]' "$st"`,
			"abc[0]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, st := runCount(t, tc.src, nil)
			if errOut != "" {
				t.Errorf("stderr = %q, want none", errOut)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
			if !strings.HasSuffix(out, tc.want) {
				t.Errorf("got %q, want it to end %q", out, tc.want)
			}
		})
	}
}

// The directive itself is an axis, and a dialect without it refuses the
// letter the way it refuses any conversion it has not got.
//
// The second row is what makes the first one evidence: `n` has to arrive at
// the ordinary bad-conversion refusal rather than being quietly dropped, and
// the store must not happen.
func TestTheCountConversionIsAnAxis(t *testing.T) {
	const src = `printf 'ab%ncd' c; printf '[%s]' "$c"`
	out, errOut, st := runCount(t, src, nil)
	if want := "abcd[2]"; out != want {
		t.Errorf("with the conversion: got %q, want %q", out, want)
	}
	if errOut != "" || st != 0 {
		t.Errorf("with the conversion: stderr %q status %d, want none and 0", errOut, st)
	}

	out, errOut, _ = runCount(t, `printf 'ab%ncd' c; st=$?; printf '[%s][%d]' "$c" "$st"`,
		func(s *Semantics) { s.PrintfCountConversion = No })
	if want := "ab[][1]"; out != want {
		t.Errorf("without it: got %q, want %q", out, want)
	}
	if !strings.Contains(errOut, "%n") {
		t.Errorf("without it: stderr = %q, want the refused directive named", errOut)
	}
}

// An operand that is present and empty is a name that is not one, or nothing
// to store, and the panel parts on which.
//
// The *absent* operand is asked in the table above, on the common path,
// because it is unanimous — which is what makes this row about the empty one
// rather than about the two together.
func TestAnEmptyCountOperandIsAnAxis(t *testing.T) {
	const src = `printf 'ab%ncd' ''; st=$?; printf '[%d]' "$st"`

	out, errOut, st := runCount(t, src, nil)
	if want := "abcd[0]"; out != want {
		t.Errorf("ignored: got %q, want %q", out, want)
	}
	if errOut != "" || st != 0 {
		t.Errorf("ignored: stderr %q status %d, want none and 0", errOut, st)
	}

	out, errOut, _ = runCount(t, src, func(s *Semantics) { s.PrintfCountEmptyNameIsIgnored = No })
	if want := "ab[1]"; out != want {
		t.Errorf("refused: got %q, want %q", out, want)
	}
	if errOut == "" {
		t.Error("refused: stderr is empty, want the refusal")
	}
}

// A word that cannot be a name is refused, and whether that gives up the rest
// of the format is an axis of its own.
//
// The stored value is asserted as well as the output, because the refusal has
// to leave the parameter alone: a shell that stored under `1bad` and then
// complained would pass a row that read only what was printed — which is the
// shape #3515 had at `printf -v`.
func TestABadCountNameIsRefused(t *testing.T) {
	const src = `printf 'abc%nXYZ' '1bad'; st=$?; printf '[%d]' "$st"`

	out, errOut, _ := runCount(t, src, nil)
	if want := "abc[1]"; out != want {
		t.Errorf("stopping: got %q, want %q", out, want)
	}
	if !strings.Contains(errOut, "1bad") {
		t.Errorf("stopping: stderr = %q, want the operand named", errOut)
	}

	out, errOut, _ = runCount(t, src, func(s *Semantics) { s.PrintfCountBadNameStopsThePass = No })
	if want := "abcXYZ[1]"; out != want {
		t.Errorf("running on: got %q, want %q", out, want)
	}
	if !strings.Contains(errOut, "1bad") {
		t.Errorf("running on: stderr = %q, want the operand named", errOut)
	}

	// And the pass that ran on still stores through a later `%n`, which is
	// what says the refusal cost that one directive and not the rest.
	out, _, _ = runCount(t, `printf 'abc%nX%n' 1bad good; printf '[%s]' "$good"`,
		func(s *Semantics) { s.PrintfCountBadNameStopsThePass = No })
	if want := "abcX[4]"; out != want {
		t.Errorf("a later store: got %q, want %q", out, want)
	}
}

// An operand carrying a subscript is an element to fill, or a name that is
// not one, and that is a gate of its own rather than the general
// StoreOperandTakesASubscript.
//
// Both readings are asserted, because a column holds each: the general field
// is Yes in the dialect that refuses this route, so a fix reading it would
// have filled the element there.
func TestACountOperandsSubscriptIsItsOwnGate(t *testing.T) {
	const src = `arr=(x y z); printf 'abcd%n' 'arr[2]'; printf '[%s]' "${arr[2]}"`

	out, errOut, _ := runCount(t, src, func(s *Semantics) {
		s.PrintfCountOperandTakesASubscript = Yes
	})
	if want := "abcd[4]"; out != want {
		t.Errorf("taking it: got %q, want %q", out, want)
	}
	if errOut != "" {
		t.Errorf("taking it: stderr = %q, want none", errOut)
	}

	out, errOut, _ = runCount(t, src, nil)
	if want := "abcd[z]"; out != want {
		t.Errorf("refusing it: got %q, want the element untouched: %q", out, want)
	}
	if !strings.Contains(errOut, "arr[2]") {
		t.Errorf("refusing it: stderr = %q, want the operand named", errOut)
	}
}

// A name the script has frozen is reported, and whether that gives up the
// rest of the format is a second axis — the same column answers it and the
// bad-name one differently, which is why they are two fields.
func TestAFrozenCountNameIsAnAxis(t *testing.T) {
	const src = `ro=1; readonly ro; printf 'ab%ncd' ro; st=$?; printf '[%d][%s]' "$st" "$ro"`

	out, errOut, _ := runCount(t, src, nil)
	if want := "ab[1][1]"; out != want {
		t.Errorf("stopping: got %q, want %q", out, want)
	}
	if errOut == "" {
		t.Error("stopping: stderr is empty, want the freeze reported")
	}

	out, errOut, _ = runCount(t, src, func(s *Semantics) {
		s.PrintfCountFrozenNameStopsThePass = No
	})
	if want := "abcd[0][1]"; out != want {
		t.Errorf("running on: got %q, want %q", out, want)
	}
	if errOut == "" {
		t.Error("running on: stderr is empty, want the freeze reported")
	}
}

// The attribute a `%n` leaves on the name it stored through, which is three
// readings and not a bool.
//
// Asserted by what a *later assignment means* rather than by a listing,
// because that is what the attribute is for: a row reading `typeset -p` back
// would pass for a shell that printed the letter and did nothing with it.
// The existing name is the discriminator — the two readings that type
// anything agree about a new one — so a table without it would have been the
// same question asked twice.
func TestWhatTheCountConversionLeavesOnTheNameIsAnAxis(t *testing.T) {
	const fresh = `printf 'ab%n' m; m=1+1; printf '[%s]' "$m"`
	const made = `q=plain; printf 'abc%n' q; q=2+2; printf '[%s]' "$q"`

	for _, tc := range []struct {
		name        string
		policy      PrintfCountAttributePolicy
		fresh, made string
	}{
		{"left alone", PrintfCountLeavesTheAttribute, "ab[1+1]", "abc[2+2]"},
		{"integer on a new name", PrintfCountIntegerOnANewName, "ab[2]", "abc[2+2]"},
		{"integer always", PrintfCountIntegerAlways, "ab[2]", "abc[4]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, st := runCount(t, fresh, func(s *Semantics) { s.PrintfCountAttribute = tc.policy })
			if out != tc.fresh || errOut != "" || st != 0 {
				t.Errorf("a new name: got %q stderr %q status %d, want %q", out, errOut, st, tc.fresh)
			}
			out, errOut, st = runCount(t, made, func(s *Semantics) { s.PrintfCountAttribute = tc.policy })
			if out != tc.made || errOut != "" || st != 0 {
				t.Errorf("a name the script made: got %q stderr %q status %d, want %q", out, errOut, st, tc.made)
			}
		})
	}

	// And with no reading at all it is refused rather than guessed, which is
	// what says the three rows above are answers and not a default.
	out, errOut, _ := runCount(t, `printf 'ab%n' m; st=$?; printf '[%d][%s]' "$st" "$m"`,
		func(s *Semantics) { s.PrintfCountAttribute = PrintfCountAttributeUnspecified })
	if !strings.Contains(errOut, "%n") {
		t.Errorf("unanswered: stderr = %q, want the refusal to name the directive", errOut)
	}
	if want := "ab[2][]"; out != want {
		t.Errorf("unanswered: got %q, want %q — the store must not happen", out, want)
	}
}
