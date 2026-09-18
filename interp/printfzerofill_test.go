// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether the `0x` C's `#` wrote counts against the width its `0` flag fills,
// both ways round.
//
// The axis is Semantics.PrintfZeroFillCountsTheAlternatePrefix. The second
// column is Go's `fmt` and is also ksh93u+, which is what makes this a
// reading rather than a defect: this shell answered that way before it was
// asked, and a blanket "match C" would have broken the one column that was
// already right (#3066). Both readings are asserted here so that moving the
// axis moves the answer rather than only one of them being checked.
func TestTheZeroFillAgainstAnAlternatePrefixIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name               string
		src                string
		counted, uncounted string
		unanimousInBoth    bool
	}{
		// The bare reading. Five characters where the prefix is counted and
		// seven where it is not, which is the whole report.
		{name: "a hexadecimal", src: `printf '[%#05x]' 7`, counted: "[0x007]", uncounted: "[0x00007]"},
		{name: "a capital one", src: `printf '[%#05X]' 255`, counted: "[0X0FF]", uncounted: "[0X000FF]"},
		{name: "a wider field", src: `printf '[%#010x]' 255`, counted: "[0x000000ff]", uncounted: "[0x00000000ff]"},
		{name: "the flags the other way round", src: `printf '[%0#5x]' 7`, counted: "[0x007]", uncounted: "[0x00007]"},
		// The width the prefix eats into. C's fill goes empty here and
		// `fmt`'s does not, so the two still part — and a fix that let the
		// subtraction go negative would have written a short field.
		{name: "a width the prefix overflows", src: `printf '[%#05x]' 65535`, counted: "[0xffff]", uncounted: "[0x0ffff]"},
		{name: "a width the prefix exactly fills", src: `printf '[%#06x]' 65535`, counted: "[0xffff]", uncounted: "[0x00ffff]"},
		{name: "one past that", src: `printf '[%#07x]' 65535`, counted: "[0x0ffff]", uncounted: "[0x000ffff]"},
		// An unsigned conversion reads the operand's bit pattern, so a
		// negative one is sixteen digits and the prefix is what the width
		// has room for either way.
		{name: "a negative operand", src: `printf '[%#020x]' -1`, counted: "[0x00ffffffffffffffff]", uncounted: "[0x0000ffffffffffffffff]"},

		// The controls, and a table without them would have been read far
		// too widely. An octal's alternate form is a leading `0` that is
		// itself a fill character, so nothing can tell the readings apart
		// there; a space fill goes in front of the whole field under both;
		// `-` voids a zero fill outright; a precision ignores the `0` flag;
		// a sign is counted in the width by everyone; and a width the digits
		// alone already reach leaves no fill to place.
		{name: "an octal", src: `printf '[%#06o]' 255`, counted: "[000377]", uncounted: "[000377]", unanimousInBoth: true},
		{name: "an octal that needs its leading zero", src: `printf '[%#05o]' 8`, counted: "[00010]", uncounted: "[00010]", unanimousInBoth: true},
		{name: "a space fill", src: `printf '[%#8x]' 7`, counted: "[     0x7]", uncounted: "[     0x7]", unanimousInBoth: true},
		{name: "left-justified", src: `printf '[%#-08x]' 7`, counted: "[0x7     ]", uncounted: "[0x7     ]", unanimousInBoth: true},
		{name: "a precision voids the zero flag", src: `printf '[%#08.3x]' 7`, counted: "[   0x007]", uncounted: "[   0x007]", unanimousInBoth: true},
		{name: "a sign is counted by everyone", src: `printf '[%05d]' -42`, counted: "[-0042]", uncounted: "[-0042]", unanimousInBoth: true},
		{name: "no fill to place", src: `printf '[%#03x]' 4095`, counted: "[0xfff]", uncounted: "[0xfff]", unanimousInBoth: true},
		{name: "no alternate form", src: `printf '[%08x]' 255`, counted: "[000000ff]", uncounted: "[000000ff]", unanimousInBoth: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.unanimousInBoth == (tc.counted != tc.uncounted) {
				t.Fatalf("the row says it is %v that the two readings agree, and they %v",
					tc.unanimousInBoth, tc.counted == tc.uncounted)
			}
			for _, r := range []struct {
				name string
				a    Answer
				want string
			}{
				{"counted against the width", Yes, tc.counted},
				{"written past the width", No, tc.uncounted},
			} {
				t.Run(r.name, func(t *testing.T) {
					sem := CoreSemantics()
					sem.PrintfZeroFillCountsTheAlternatePrefix = r.a
					// And C's reading of the other fill question, which
					// one row here reaches and none is about.
					sem.PrintfZeroFlagSurvivesAPrecision = No
					out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
					if out != r.want || st != 0 {
						t.Errorf("got %q status %d, want %q and 0", out, st, r.want)
					}
				})
			}
		})
	}
}

// The axis is asked where it decides the answer and nowhere else.
//
// An unanswered axis refuses, which is how the core reports a question the
// panel splits on — so this is also the assertion that a shell with no
// dialect is not quietly handed one reading. The rows that must *not* refuse
// are the ones worth having: every one of them is a construct all seven
// columns write identically, and a conversion that asked anyway would refuse
// a script nobody disagrees about.
func TestTheZeroFillAxisIsAskedOnlyWhereTheReadingsPart(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		want    string
		refuses bool
	}{
		{name: "a fill to place", src: `printf '[%#05x]' 7`, refuses: true},
		{name: "a fill the prefix empties", src: `printf '[%#05x]' 65535`, refuses: true},
		{name: "a capital hexadecimal", src: `printf '[%#05X]' 7`, refuses: true},

		{name: "an octal", src: `printf '[%#06o]' 255`, want: "[000377]"},
		{name: "a space fill", src: `printf '[%#8x]' 7`, want: "[     0x7]"},
		{name: "left-justified", src: `printf '[%#-08x]' 7`, want: "[0x7     ]"},
		{name: "a precision", src: `printf '[%#08.3x]' 7`, want: "[   0x007]"},
		{name: "no alternate form", src: `printf '[%08x]' 255`, want: "[000000ff]"},
		{name: "a width the digits already reach", src: `printf '[%#03x]' 4095`, want: "[0xfff]"},
		{name: "no width at all", src: `printf '[%#0x]' 7`, want: "[0x7]"},
		// A nought is the other axis's row: the columns that keep a prefix
		// there are the columns that do not count it, so the two never both
		// bite. printfNoughtField answers this one, and under the core it
		// refuses for PrintfAlternateFormAsksTheValue rather than for this.
		{name: "a nought with no alternate form", src: `printf '[%05x]' 0`, want: "[00000]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			// The precision row reaches the other fill axis, which this
			// case is not about — see PrintfZeroFlagSurvivesAPrecision.
			sem.PrintfZeroFlagSurvivesAPrecision = No
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if !tc.refuses {
				if out != tc.want || st != 0 {
					t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
				}
				return
			}
			if st != 2 {
				t.Errorf("status %d, want 2", st)
			}
			if !strings.Contains(out, "no dialect was chosen") {
				t.Errorf("the refusal does not say a dialect is missing: %q", out)
			}
		})
	}
}

// A nought under a zero fill is the *other* axis and not this one, in both of
// its readings.
//
// This is the row #3070 pinned, and it is here to say that the fix beside it
// did not move it: PrintfAlternateFormAsksTheValue alone decides
// `printf '%#05x' 0`, and it decides it the same way whichever answer this
// axis carries. The two are never both live, because the column that keeps a
// prefix at a nought is the column that writes it past the width.
func TestANoughtIsDecidedByTheOtherAxisAlone(t *testing.T) {
	for _, form := range []struct {
		name string
		a    Answer
		want string
	}{
		{"the prefix comes off at a nought", Yes, "[00000]"},
		{"the prefix follows the digits", No, "[0x00000]"},
	} {
		for _, fill := range []struct {
			name string
			a    Answer
		}{{"counted against the width", Yes}, {"written past the width", No}} {
			t.Run(form.name+", "+fill.name, func(t *testing.T) {
				sem := CoreSemantics()
				sem.PrintfAlternateFormAsksTheValue = form.a
				sem.PrintfZeroFillCountsTheAlternatePrefix = fill.a
				out, st := run(t, `printf '[%#05x]' 0`, func(r *Runner) { r.Semantics = &sem })
				if out != form.want || st != 0 {
					t.Errorf("got %q status %d, want %q and 0", out, st, form.want)
				}
			})
		}
	}
}
