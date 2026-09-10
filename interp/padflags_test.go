// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// The padding flags, `${(l:expr::string1::string2:)x}` and its `r` mirror.
// Every expectation here is an oracle measurement recorded in
// docs/spec/grammar/parameter-expansion.md; the tests name the grammar flag
// and never a shell.

func TestPaddingFlagValues(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"l pads with spaces", `v=ab; printf "[%s]" "${(l:5:)v}"`, "[   ab]"},
		{"l pads with a fill", `v=ab; printf "[%s]" "${(l:5::-:)v}"`, "[---ab]"},
		{"a second fill goes once against the word", `v=ab; printf "[%s]" "${(l:5::-::+:)v}"`, "[--+ab]"},
		{"r pads on the other side", `v=ab; printf "[%s]" "${(r:5::-:)v}"`, "[ab---]"},
		{"r puts its second fill against the word too", `v=ab; printf "[%s]" "${(r:5::x::y:)v}"`, "[abyxx]"},

		// The truncation half, and it is not symmetric: a word wider than
		// the field keeps the end nearest the padding.
		{"l truncates from the far end", `v=ab; printf "[%s]" "${(l:1:)v}"`, "[b]"},
		{"r truncates from the far end", `v=ab; printf "[%s]" "${(r:1:)v}"`, "[a]"},
		{"l truncates a longer word", `w=abcdef; printf "[%s]" "${(l:3:)w}"`, "[def]"},
		{"r truncates a longer word", `w=abcdef; printf "[%s]" "${(r:3:)w}"`, "[abc]"},

		// A zero width is the flag doing nothing, which is a different
		// answer from truncating to nothing.
		{"a zero width leaves the word alone", `v=ab; printf "[%s]" "${(l:0:)v}"`, "[ab]"},
		{"a zero width leaves a fill unused", `v=ab; printf "[%s]" "${(l:0::x:)v}"`, "[ab]"},

		// A multi-unit fill repeats, and which end of the repetition is
		// kept is the half a symmetric implementation gets wrong.
		{"a multi-unit fill repeats", `v=ab; printf "[%s]" "${(l:6::ab:)v}"`, "[ababab]"},
		{"l keeps the tail of the repetition", `v=ab; printf "[%s]" "${(l:7::ab:)v}"`, "[bababab]"},
		{"r keeps the head of the repetition", `v=ab; printf "[%s]" "${(r:7::ab:)v}"`, "[abababa]"},
		{"the second fill is truncated for l", `v=ab; printf "[%s]" "${(l:3::x::yz:)v}"`, "[zab]"},
		{"the second fill is truncated for r", `v=ab; printf "[%s]" "${(r:3::x::yz:)v}"`, "[aby]"},
		{"a second fill that does not fit at all", `v=ab; printf "[%s]" "${(l:2::x::yz:)v}"`, "[ab]"},

		// The width is an arithmetic expression rather than a number.
		{"a width from a parameter", `n=4; v=ab; printf "[%s]" "${(l:$n:)v}"`, "[  ab]"},
		{"a width from a bare name", `n=4; v=ab; printf "[%s]" "${(l:n:)v}"`, "[  ab]"},
		{"a width that is an expression", `v=ab; printf "[%s]" "${(l:2+3:)v}"`, "[   ab]"},
		{"a width from an unset name is zero", `v=ab; printf "[%s]" "${(l:nosuch:)v}"`, "[ab]"},
		{"an empty width is zero", `v=ab; printf "[%s]" "${(l::)v}"`, "[ab]"},
		{"a negative width is its own size", `v=ab; printf "[%s]" "${(l:-3:)v}"`, "[ ab]"},
		{"a negative width still truncates", `v=ab; printf "[%s]" "${(l:-1:)v}"`, "[b]"},

		// An empty value is the shape the plugin ecosystem reaches for: a
		// run of one character, with no parameter written at all.
		{"an empty value is all fill", `printf "[%s]" "${(l:3::0:)}"`, "[000]"},
		{"an unset name is all fill", `printf "[%s]" "${(l:5::-:)nosuch}"`, "[-----]"},

		// The three slots survive a flag written twice, each on its own.
		{"a later width keeps the earlier fills", `v=ab; printf "[%s]" "${(l:3::x::y:l:5:)v}"`, "[xxyab]"},
		{"a later fill replaces only its own slot", `v=ab; printf "[%s]" "${(l:3::x::y:l:5::z:)v}"`, "[zzyab]"},

		// Padding is applied to the word the group has by the time it runs,
		// which the quoted join has already made one of.
		{"each element of a list is padded", `a=(one two); printf "[%s]" "${(@l:5::-:)a}"`, "[--one][--two]"},
		{"the quoted join happens first", `a=(one two three); printf "[%s]" "${(l:5::-:)a}"`, "[three]"},
		{"a split's fields are each padded", `v=a:b; printf "[%s]" "${(@s.:.l:3::x:)v}"`, "[xxa][xxb]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := flagsRun(t, tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%q: out=%q errs=%q st=%d, want %q clean", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// Where the step sits, on the probes that separate the two orders. A fix that
// puts padding anywhere else in the pipeline passes the table above and fails
// here.
func TestPaddingRunsLateInTheGroup(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"after the length", `v=abc; printf "[%s]" "${(l:5::x:)#v}"`, "[xxxx3]"},
		{"after the case conversion", `u=ab; printf "[%s]" "${(Ul:4::x:)u}"`, "[xxAB]"},
		{"after the quoting", `q="a b"; printf "[%s]" "${(ql:6::x:)q}"`, `[xxa\ b]`},
		{"after the unquoting", `v="'a b'"; printf "[%s]" "${(Ql:8::x:)v}"`, "[xxxxxa b]"},
		{"after the ordering", `b=(bb a); printf "[%s]" "${(@ol:3::x:)b}"`, "[xxa][xbb]"},
		{"after the operator", `v=ab; printf "[%s]" "${(l:4::x:)v:+SET}"`, "[xSET]"},
		{
			// The row the manual's rule numbers get backwards, and the only
			// one that does. Twelve characters as written and four once read
			// again: padded first, the field holds eight `x` and the four
			// the re-reading produced.
			"before the re-reading",
			`e='ab$(echo XY)'; printf "[%s]" "${(el:20::x:)e}"`,
			"[xxxxxxxxabXY]",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := flagsRun(t, tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%q: out=%q errs=%q st=%d, want %q clean", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// Both flags in one group is a third shape rather than one applied after the
// other: the word is cut in half and each half gets a field of its own.
//
// The five widths are one table because only the run of them says where the
// odd unit goes — a word of even width cannot tell the two halvings apart.
func TestPaddingOnBothSidesHalvesTheWord(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"an empty word", `v=""; printf "[%s]" "${(l:4::L:r:4::R:)v}"`, "[LLLLRRRR]"},
		{"one unit", `v=a; printf "[%s]" "${(l:4::L:r:4::R:)v}"`, "[LLLLaRRR]"},
		{"two units", `v=ab; printf "[%s]" "${(l:4::L:r:4::R:)v}"`, "[LLLabRRR]"},
		{"three units", `v=abc; printf "[%s]" "${(l:4::L:r:4::R:)v}"`, "[LLLabcRR]"},
		{"four units", `v=abcd; printf "[%s]" "${(l:4::L:r:4::R:)v}"`, "[LLabcdRR]"},
		{"five units", `v=abcde; printf "[%s]" "${(l:4::L:r:4::R:)v}"`, "[LLabcdeR]"},
		{"unequal fields", `v=abcd; printf "[%s]" "${(l:3::L:r:5::R:)v}"`, "[LabcdRRR]"},
		{"both halves truncated", `v=abcd; printf "[%s]" "${(l:1::L:r:1::R:)v}"`, "[bc]"},
		{"a zero left field leaves the whole word to the right", `v=abcd; printf "[%s]" "${(l:0::L:r:6::R:)v}"`, "[abcdRR]"},
		{"a zero right field leaves it to the left", `v=abcd; printf "[%s]" "${(l:6::L:r:0::R:)v}"`, "[LLabcd]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := flagsRun(t, tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%q: out=%q errs=%q st=%d, want %q clean", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// A fill written empty is not a fill left out: the first keeps `$IFS`'s first
// character and the second pads with spaces whatever `$IFS` holds. Under the
// default `$IFS` the two agree, so the axis is only visible with one set.
func TestPaddingReadsAnEmptyFillFromIFS(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"a fill written empty", `IFS=.; v=ab; printf "[%s]" "${(l:5:::)v}"`, "[...ab]"},
		{"a second fill written empty", `IFS=.; v=ab; printf "[%s]" "${(l:5::x:::)v}"`, "[xx.ab]"},
		{"on the right too", `IFS=.; v=ab; printf "[%s]" "${(r:5::x:::)v}"`, "[ab.xx]"},
		{"a fill left out is spaces", `IFS=.; v=ab; printf "[%s]" "${(l:5:)v}"`, "[   ab]"},
		{"a second fill left out is nothing", `IFS=.; v=ab; printf "[%s]" "${(l:5::x:)v}"`, "[xxxab]"},
		{"an IFS with no first character fills nothing", `IFS=; v=ab; printf "[%s]" "${(l:5:::)v}"`, "[ab]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := flagsRun(t, tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%q: out=%q errs=%q st=%d, want %q clean", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// A width that will not evaluate is the arithmetic failure it is, reported in
// the words every other expression gets and fatal to the *script* — not a
// field padded to a width nobody wrote.
//
// Fatal rather than skipped, measured: a failed expansion stops the shell
// reading further, so the `echo` behind it never runs.
func TestPaddingReportsAWidthThatWillNotEvaluate(t *testing.T) {
	out, errs, st := flagsRun(t, `v=ab; printf "[%s]" "${(l:x y:)v}"; echo after`)
	if !strings.Contains(errs, "operator expected") {
		t.Errorf("stderr = %q, want the arithmetic failure named", errs)
	}
	if out != "" {
		t.Errorf("stdout = %q, want nothing printed", out)
	}
	if st == 0 {
		t.Errorf("status 0, want the failure to be fatal")
	}
}

// The `(~)` flag marks the argument of a flag written behind it, and a marked
// *fill* is refused by name rather than answered. A fill has to be counted as
// text and escaped as text, and a marked one is neither.
func TestPaddingRefusesAMarkedFill(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a marked left fill",
			`v=ab; printf "[%s]" "${(~l:5::*:)v}"`,
			"testsh: ${(~l:5::*:)v}: the (~) expansion flag is not implemented for the (l) fill\n",
		},
		{
			"a marked right fill",
			`v=ab; printf "[%s]" "${(~r:5::*:)v}"`,
			"testsh: ${(~r:5::*:)v}: the (~) expansion flag is not implemented for the (r) fill\n",
		},
		{
			// A `~` in front of both marks the fill, and the fill is the
			// nearer refusal — so the join's own is reached only by a mark
			// that stands behind the padding letter.
			"a marked separator behind a padding flag",
			`a=(p q); printf "[%s]" "${(l:9::x:~j.|.)a}"`,
			"testsh: ${(l:9::x:~j.|.)a}: the (~) expansion flag is not implemented beside a padding flag\n",
		},
		{
			"a mark in front of both names the fill",
			`a=(p q); printf "[%s]" "${(~j.|.l:9::x:)a}"`,
			"testsh: ${(~j.|.l:9::x:)a}: the (~) expansion flag is not implemented for the (l) fill\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := flagsRun(t, tc.src)
			if errs != tc.want {
				t.Errorf("stderr = %q, want %q", errs, tc.want)
			}
			if out != "" || st == 0 {
				t.Errorf("out=%q st=%d, want nothing printed and a non-zero status", out, st)
			}
		})
	}
}
