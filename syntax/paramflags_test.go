// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// flagged parses src under a dialect with the flag group enabled and returns
// the first parameter expansion.
//
// BareBraceNestsInExpansion comes with it because a flag's argument may be
// delimited by braces — `${(g{o})v}` — and without the nesting the expansion
// closes at the `}` of the argument, so the text is not a flag group at all.
// The one shell in the panel with the flag group has both, so the pair is
// what the construct is measured under; see #1587.
func flagged(t *testing.T, src string) *ParamExpr {
	t.Helper()
	d := Core()
	d.ParamExpansionFlags = true
	d.BareBraceNestsInExpansion = true
	return firstParam(t, src, d)
}

func TestParamExpansionFlagsParse(t *testing.T) {
	tests := []struct {
		src      string
		flags    string
		splitSep string
		joinSep  string
	}{
		{`echo ${(U)x}`, "U", "", ""},
		{`echo ${(L)x}`, "L", "", ""},
		{`echo ${(Uq)x}`, "Uq", "", ""},
		{`echo ${(qq)x}`, "qq", "", ""},
		{`echo ${(f)x}`, "f", "", ""},
		{`echo ${(s.:.)x}`, "s", ":", ""},
		{`echo ${(s:,:)x}`, "s", ",", ""},
		{`echo ${(s[,])x}`, "s", ",", ""},
		{`echo ${(s<XY>)x}`, "s", "XY", ""},
		{`echo ${(j.,.)a}`, "j", "", ","},
		{`echo ${(kv)m}`, "kv", "", ""},
		{`echo ${(@f)x}`, "@f", "", ""},
		{`echo ${(P)x}`, "P", "", ""},
		{`echo ${()x}`, "", "", ""},
	}
	for _, tc := range tests {
		e := flagged(t, tc.src)
		if e == nil {
			t.Fatalf("%q: no parameter expansion parsed", tc.src)
		}
		if !e.HasFlags {
			t.Errorf("%q: HasFlags = false, want a flag group", tc.src)
		}
		if e.Flags != tc.flags || e.SplitSep != tc.splitSep || e.JoinSep != tc.joinSep {
			t.Errorf("%q: flags %q sep %q/%q, want %q sep %q/%q",
				tc.src, e.Flags, e.SplitSep, e.JoinSep, tc.flags, tc.splitSep, tc.joinSep)
		}
		if e.FlagsErrPos != 0 {
			t.Errorf("%q: FlagsErrPos = %d, want a clean read", tc.src, e.FlagsErrPos)
		}
	}
}

// The flags apply before the parameter's own grammar, which stays whole: an
// operator, a subscript and the length prefix all still parse behind a group.
func TestParamExpansionFlagsLeaveTheRestOfTheGrammarAlone(t *testing.T) {
	tests := []struct{ src, rest string }{
		{`echo ${(U)x:-def}`, "x :- def"},
		{`echo ${(U)x#h}`, "x # h"},
		{`echo ${(U)a[2]}`, "a[2]"},
		{`echo ${(U)#x}`, "#x"},
		{`echo ${(%):-%x}`, " :- %x"},
		{`echo ${(U)}`, ""},
	}
	for _, tc := range tests {
		e := flagged(t, tc.src)
		if got := param(e); got != tc.rest {
			t.Errorf("%q parsed as %q, want %q", tc.src, got, tc.rest)
		}
	}
}

// A character the group cannot carry is recorded with its position — counted
// from the `$`, measured — and diagnosed at run time, not here.
func TestParamExpansionFlagsErrorPosition(t *testing.T) {
	tests := []struct {
		src string
		pos int
	}{
		{`echo ${(!)x}`, 4},    // not a flag at all
		{`echo ${(Ux}`, 5},     // the closing parenthesis never arrives
		{`echo ${(s)x}`, 5},    // an argument-taking flag with no argument
		{`echo ${(s:x)y}`, 5},  // an argument whose delimiter never closes
		{`echo ${(s:::)x}`, 7}, // a third delimiter where a flag should be
	}
	for _, tc := range tests {
		e := flagged(t, tc.src)
		if e == nil {
			t.Fatalf("%q: no parameter expansion parsed", tc.src)
		}
		if e.FlagsErrPos != tc.pos {
			t.Errorf("%q: FlagsErrPos = %d, want %d", tc.src, e.FlagsErrPos, tc.pos)
		}
	}
}

// Off, the group follows the same split every unreadable expansion follows:
// deferred to the run for the majority, refused while reading where the
// dialect diagnoses bad substitutions at parse time.
func TestParamExpansionFlagsOffDefersToTheRun(t *testing.T) {
	e := firstParam(t, `echo ${(U)x}`, Core())
	if e == nil || !e.Bad {
		t.Fatalf("Core: ${(U)x} = %+v, want a deferred Bad node", e)
	}
	if e.Src != "(U)x" {
		t.Errorf("Src = %q, want the inside kept for the diagnostic", e.Src)
	}

	d := Core()
	d.BadSubstitutionAtParseTime = true
	if _, err := Parse(`echo ${(U)x}`, d); err == nil {
		t.Errorf("parse-time dialect accepted ${(U)x}, want a refusal")
	}
}

func TestParamExpansionFlagsRoundTrip(t *testing.T) {
	d := Core()
	d.ParamExpansionFlags = true
	srcs := []string{
		"echo ${(U)x}\n",
		"echo ${(s.:.)x}\n",
		"echo ${(j:,:)a}\n",
		"echo ${(%):-%x}\n",
		"echo \"${(@f)x}\"\n",
	}
	for _, src := range srcs {
		f, err := Parse(src, d)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		printed := Print(f)
		if !strings.Contains(printed, strings.TrimSuffix(strings.TrimPrefix(src, "echo "), "\n")) {
			t.Errorf("Print(%q) = %q, want the construct written back as it was read", src, printed)
		}
		second, err := Parse(printed, d)
		if err != nil {
			t.Fatalf("reparse %q: %v", printed, err)
		}
		if again := Print(second); again != printed {
			t.Errorf("print of %q is not stable: %q then %q", src, printed, again)
		}
	}
}

// The padding pair's three slots, filled in by position. The last two are
// optional and an empty one is not the same as one left out, so the test
// asserts on the Set fields rather than only on the text — see ParamPad.
func TestPaddingFlagArgumentsParse(t *testing.T) {
	for _, tc := range []struct {
		src   string
		flags string
		left  ParamPad
		right ParamPad
	}{
		{`echo ${(l:5:)x}`, "l", ParamPad{Width: "5"}, ParamPad{}},
		{`echo ${(l:5::-:)x}`, "l", ParamPad{Width: "5", Fill: "-", FillSet: true}, ParamPad{}},
		{
			`echo ${(l:5::-::+:)x}`, "l",
			ParamPad{Width: "5", Fill: "-", FillSet: true, Insert: "+", InsertSet: true},
			ParamPad{},
		},
		// An argument written empty is written, and the interpreter reads
		// `$IFS` for it — so the Set field is the whole of the difference
		// between these two and the first row above.
		{`echo ${(l:5:::)x}`, "l", ParamPad{Width: "5", FillSet: true}, ParamPad{}},
		{
			`echo ${(l:5::x:::)x}`, "l",
			ParamPad{Width: "5", Fill: "x", FillSet: true, InsertSet: true},
			ParamPad{},
		},
		// The width is text rather than a number: it is an arithmetic
		// expression and is read when the expansion runs.
		{`echo ${(l:COLUMNS-1:)x}`, "l", ParamPad{Width: "COLUMNS-1"}, ParamPad{}},
		{`echo ${(l:$n:)x}`, "l", ParamPad{Width: "$n"}, ParamPad{}},
		// The delimiter is whichever punctuation follows the letter, and the
		// matched pairs count.
		{`echo ${(l.3..0.)x}`, "l", ParamPad{Width: "3", Fill: "0", FillSet: true}, ParamPad{}},
		{`echo ${(l#2##0#)x}`, "l", ParamPad{Width: "2", Fill: "0", FillSet: true}, ParamPad{}},
		{`echo ${(l<3><0>)x}`, "l", ParamPad{Width: "3", Fill: "0", FillSet: true}, ParamPad{}},
		// The right-hand mirror, and both in one group.
		{`echo ${(r:5::-:)x}`, "r", ParamPad{}, ParamPad{Width: "5", Fill: "-", FillSet: true}},
		{
			`echo ${(l:5::x:r:7::y:)x}`, "lr",
			ParamPad{Width: "5", Fill: "x", FillSet: true},
			ParamPad{Width: "7", Fill: "y", FillSet: true},
		},
		// Written twice, the slots are filled in one at a time: the later
		// width replaces the earlier one and the fills it did not write stay.
		{
			`echo ${(l:3::x::y:l:5:)x}`, "ll",
			ParamPad{Width: "5", Fill: "x", FillSet: true, Insert: "y", InsertSet: true},
			ParamPad{},
		},
		{
			`echo ${(l:3::x::y:l:5::z:)x}`, "ll",
			ParamPad{Width: "5", Fill: "z", FillSet: true, Insert: "y", InsertSet: true},
			ParamPad{},
		},
	} {
		e := flagged(t, tc.src)
		if e == nil {
			t.Fatalf("%q: no parameter expansion parsed", tc.src)
		}
		if e.FlagsErrPos != 0 {
			t.Errorf("%q: FlagsErrPos = %d, want a clean read", tc.src, e.FlagsErrPos)
		}
		if e.Flags != tc.flags || e.PadLeft != tc.left || e.PadRight != tc.right {
			t.Errorf("%q: flags %q pad %+v/%+v, want %q %+v/%+v",
				tc.src, e.Flags, e.PadLeft, e.PadRight, tc.flags, tc.left, tc.right)
		}
	}
}

// A padding flag with no argument at all, and one whose delimiter never
// closes, are both errors *in the flags* at the character that arrived
// instead — the same shape every other unreadable group has, reported when
// the expansion is reached rather than while it is read.
func TestPaddingFlagWithoutItsArgument(t *testing.T) {
	for _, tc := range []struct {
		src string
		pos int
	}{
		{`echo ${(l)x}`, 5},
		{`echo ${(lU:4::x:)x}`, 5},
		{`echo ${(r)x}`, 5},
		{`echo ${(l:5::::)x}`, 10},
		{`echo ${(l:5::-::)x}`, 11},
	} {
		e := flagged(t, tc.src)
		if e == nil {
			t.Fatalf("%q: no parameter expansion parsed", tc.src)
		}
		if e.FlagsErrPos != tc.pos {
			t.Errorf("%q: FlagsErrPos = %d, want %d", tc.src, e.FlagsErrPos, tc.pos)
		}
	}
}
