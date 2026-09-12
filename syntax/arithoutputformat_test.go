// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// outputFormat is the core plus the bracketed output-format specifier. The
// flag is named here and the shell that sets it is not.
func outputFormat(on bool) syntax.Dialect {
	d := syntax.Core()
	d.ArithOutputFormat = on
	return d
}

// What each spelling of the specifier says.
func TestTheOutputFormatSpecifierIsRead(t *testing.T) {
	for _, tc := range []struct {
		name     string
		src      string
		base     int
		based    bool
		prefixed bool
		group    int
	}{
		{"a base", "echo $(( [#16] 255 ))", 16, true, true, 0},
		{"a base with no prefix", "echo $(( [##16] 255 ))", 16, true, false, 0},
		{"the smallest base", "echo $(( [#2] 5 ))", 2, true, true, 0},
		{"the largest base", "echo $(( [#36] 5 ))", 36, true, true, 0},
		{"a leading zero in the base", "echo $(( [#016] 255 ))", 16, true, true, 0},
		{"a base out of range is still read", "echo $(( [#37] 5 ))", 37, true, true, 0},
		{"zero is a base and not the absence of one", "echo $(( [#0] 5 ))", 0, true, true, 0},
		{"grouping with a base", "echo $(( [#16_4] 255 ))", 16, true, true, 4},
		{"grouping with a base and no prefix", "echo $(( [##16_4] 255 ))", 16, true, false, 4},
		{"grouping alone", "echo $(( [#_] 1234567 ))", 0, false, true, 3},
		{"grouping alone with a size", "echo $(( [#_5] 1234567 ))", 0, false, true, 5},
		{"a bare underscore after a base is three", "echo $(( [#16_] 255 ))", 16, true, true, 3},
		{"grouping turned off", "echo $(( [#16_0] 255 ))", 16, true, true, 0},
		{"no space is needed", "echo $(( [#16]255 ))", 16, true, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x, ok := arithOf(t, tc.src, outputFormat(true)).(*syntax.ArithOutput)
			if !ok {
				t.Fatalf("parse %q: not an output-format node", tc.src)
			}
			if x.Base != tc.base || x.Based != tc.based ||
				x.Prefixed != tc.prefixed || x.Group != tc.group {
				t.Errorf("parse %q: base %d based %v prefixed %v group %d, want %d %v %v %d",
					tc.src, x.Base, x.Based, x.Prefixed, x.Group,
					tc.base, tc.based, tc.prefixed, tc.group)
			}
		})
	}
}

// The specifier is lexical rather than positional: it stands where a token
// may, not where an operand may, and it is lifted to the top of the tree
// however deep in the text it was written.
//
// Each of these is a separate way the obvious reading — a prefix operator over
// the expression beside it — gets one wrong. A node built where the specifier
// stood could not answer the third at all, because the branch holding it is
// never evaluated.
func TestTheOutputFormatSpecifierStandsAnywhere(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		base int
	}{
		{"in front", "echo $(( [#16] 255 ))", 16},
		{"in the middle", "echo $(( 255 + [#16] 1 ))", 16},
		{"after a value", "echo $(( 2[#8] ))", 8},
		{"after a name", "echo $(( a [#8] ))", 8},
		{"inside parentheses", "echo $(( ([#16] 255) ))", 16},
		{"in a branch never taken", "echo $(( 0 ? [#16] 1 : 2 ))", 16},
		{"before an assignment", "echo $(( [#16] x = 255 ))", 16},
		{"alone", "echo $(( [#16] ))", 16},
		{"the last of several wins", "echo $(( [#16] 255 + [#8] 1 ))", 8},
		{"the last of two touching", "echo $(( [#16][#8] 255 ))", 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x, ok := arithOf(t, tc.src, outputFormat(true)).(*syntax.ArithOutput)
			if !ok {
				t.Fatalf("parse %q: not an output-format node", tc.src)
			}
			if x.Base != tc.base {
				t.Errorf("parse %q: base %d, want %d", tc.src, x.Base, tc.base)
			}
		})
	}
}

// A specifier with nothing after it still has an expression slot, and it is
// empty rather than a failure.
func TestTheOutputFormatSpecifierMayStandAlone(t *testing.T) {
	x, ok := arithOf(t, "echo $(( [#16] ))", outputFormat(true)).(*syntax.ArithOutput)
	if !ok {
		t.Fatalf("not an output-format node")
	}
	if x.X != nil {
		t.Errorf("operand %#v, want none", x.X)
	}
}

// The two ways a bracketed group is refused, which the grammar tells apart
// because they are worded apart: digits alone are a base syntax that is not
// this one, and anything else is a specifier that would not read.
func TestAnUnreadableOutputFormatIsRefused(t *testing.T) {
	for _, tc := range []struct {
		src  string
		kind syntax.ErrorKind
	}{
		{"[16] 255", syntax.ErrArithBadBaseSyntax},
		{"[2] 5", syntax.ErrArithBadBaseSyntax},
		{"1 + [16] 2", syntax.ErrArithBadBaseSyntax},
		{"[] 255", syntax.ErrArithBadOutputFormat},
		{"[foo] 255", syntax.ErrArithBadOutputFormat},
		{"[#] 255", syntax.ErrArithBadOutputFormat},
		{"[##] 255", syntax.ErrArithBadOutputFormat},
		{"[###16] 255", syntax.ErrArithBadOutputFormat},
		{"[# 16] 255", syntax.ErrArithBadOutputFormat},
		{"[#16 ] 255", syntax.ErrArithBadOutputFormat},
		{"[#16 _4] 255", syntax.ErrArithBadOutputFormat},
		{"[#-16] 255", syntax.ErrArithBadOutputFormat},
		{"[#16x] 255", syntax.ErrArithBadOutputFormat},
		{"[#16_4x] 255", syntax.ErrArithBadOutputFormat},
	} {
		t.Run(tc.src, func(t *testing.T) {
			err := arithErr(tc.src, outputFormat(true))
			var se *syntax.Error
			if !errors.As(err, &se) {
				t.Fatalf("read %q: %v, want a syntax error", tc.src, err)
			}
			if se.Kind != tc.kind {
				t.Errorf("read %q: kind %v, want %v", tc.src, se.Kind, tc.kind)
			}
		})
	}
}

// Without the flag the bracket is no specifier at all, and the refusal is the
// ordinary missing-operand one — which is what the rest of the panel says
// about the same text.
func TestWithoutTheFlagTheBracketIsNoSpecifier(t *testing.T) {
	for _, src := range []string{"[#16] 255", "[##16] 255", "[#_] 1234567", "[16] 255"} {
		err := arithErr(src, outputFormat(false))
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Fatalf("read %q without the flag: %v, want a syntax error", src, err)
		}
		if se.Kind != syntax.ErrArithOperand {
			t.Errorf("read %q without the flag: kind %v, want ErrArithOperand", src, se.Kind)
		}
	}
}

// A subscript is untouched in both directions: its bracket touches the name in
// front of it and is read by the name, where this one stands where a token
// begins.
func TestASubscriptIsNotAnOutputFormat(t *testing.T) {
	for _, on := range []bool{true, false} {
		x, ok := arithOf(t, "echo $(( a[#8] ))", outputFormat(on)).(*syntax.ArithIndex)
		if !ok {
			t.Fatalf("with the flag %v: not a subscript", on)
		}
		if x.Name != "a" || x.Sub != "#8" {
			t.Errorf("with the flag %v: name %q sub %q, want %q %q", on, x.Name, x.Sub, "a", "#8")
		}
	}
}
