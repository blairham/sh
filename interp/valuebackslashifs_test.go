// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// A value's backslash stands in the escaped form as a **mark**, and the byte
// standing at its position is not the character to test against `IFS`.
//
// The mirror of what interp/valuebackslashseparator.go closed. That file
// fixed the false negative — a backslash written into `IFS` not separating,
// because the walk tested the mark instead of the backslash. This is the
// false positive: one shell's default `IFS` **holds the mark's byte**. zsh's
// is space, tab, newline and NUL, and the mark is NUL, so a value's
// backslash looked like a separator that nobody wrote.
//
// Measured 2026-09-29 against zsh 5.9.2 at /opt/homebrew/bin/zsh with
// `${=v}`, and against bash 5.3.20, dash 0.5.12 and ksh93u+ on the unquoted
// path: all four agree on every row, so this is core with no axis.
func TestAValuesBackslashIsNotASeparatorBecauseIFSHoldsTheMark(t *testing.T) {
	mark := string(valueBackslashMark)
	// zsh's default IFS, which is the whole point: the fourth byte is the
	// mark's own.
	const zshIFS = " \t\n\x00"
	for _, tc := range []struct {
		name string
		s    string
		want []string
	}{
		// The row where the count changes rather than the content, which is
		// what says the mark was being read as a separator.
		{"a backslash b", "a" + mark + "\\b", []string{`a\b`}},
		// A value that *ends* in a backslash. The doubled mark stands for a
		// backslash that ran out of value, and it was opening a field behind
		// itself through the trailing-separator stage.
		{"lone backslash", valueBackslashRanOutOfValue, []string{`\`}},
		{
			"trailing backslash", "ab" + valueBackslashRanOutOfValue,
			[]string{`ab\`},
		},
		{
			"fields then a backslash", "+ : = " + valueBackslashRanOutOfValue,
			[]string{"+", ":", "=", `\`},
		},
		// And a backslash between two real separators still stands as its
		// own field rather than being eaten with one of them.
		{
			"a space backslash space b", "a " + mark + "\\ b",
			[]string{"a", `\`, "b"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRunner(t, &Runner{})
			got := r.splitFieldsAsking(tc.s, nil, zshIFS, true, false, true)
			// Compared as **values**, with the escaped form resolved, because
			// that is the level the measurement is at: the reference is a
			// shell and what it shows is the field, not the encoding.
			for i := range got {
				got[i] = globUnescape(got[i])
			}
			if len(got) != len(tc.want) {
				t.Fatalf("%q -> %d fields %q, want %d %q",
					tc.s, len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("%q field %d = %q, want %q", tc.s, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// The positive control, so the rows above are not a splitter that has stopped
// separating: a backslash **written into IFS** still separates, which is the
// half valuebackslashseparator.go exists for. Both faults are about the same
// position and they pull in opposite directions, so neither test is evidence
// without the other.
func TestABackslashWrittenIntoIFSStillSeparates(t *testing.T) {
	mark := string(valueBackslashMark)
	r := newTestRunner(t, &Runner{})
	got := r.splitFieldsAsking("b"+mark+"\\c", nil, "\\", true, false, true)
	for i := range got {
		got[i] = globUnescape(got[i])
	}
	want := []string{"b", "c"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf(`IFS='\', value "b\c" -> %d %q, want %q`, len(got), got, want)
	}
	// And the mark's own byte in IFS is *not* what makes it separate: with
	// zsh's default IFS the same value is one field. That pair is the whole
	// rule in two lines.
	if one := r.splitFieldsAsking("b"+mark+"\\c", nil, " \t\n\x00", true, false, true); len(one) != 1 {
		t.Errorf("default zsh IFS -> %d %q, want one field", len(one), one)
	}
}

// trailingRunSeparates walks the string a second time, and it had the same
// fault: a value ending in a backslash looked like a closing non-whitespace
// separator under an IFS holding the mark's byte.
//
// Tested directly because it is the stage the splitter's own rows cannot
// reach — the splitter had already been made right and `${=v}` was still two
// fields, which is what sent the search here.
func TestTrailingRunSeparatesReadsTheBackslashAndNotTheMark(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    string
		ifs  string
		want bool
	}{
		{"value ends in a backslash, zsh IFS", valueBackslashRanOutOfValue, " \t\n\x00", false},
		{"text then a backslash, zsh IFS", "ab" + valueBackslashRanOutOfValue, " \t\n\x00", false},
		// **Not** a positive control, which is the point: with a backslash
		// *in* IFS the answer is still false. Promoting the mark to the
		// backslash it stands for would make it true here and ask the
		// trailing question where nothing asked it before, and the measured
		// answer is two fields — see TestABackslashInIFSSeparates's "a value
		// that is only the separator". So the walk ends at a mark rather
		// than reading through it.
		{"value ends in a backslash, IFS is a backslash", valueBackslashRanOutOfValue, "\\", false},
		// And an ordinary non-whitespace separator still does.
		{"value ends in a colon", "ab:", ":", true},
		{"value ends in a space", "ab ", " ", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRunner(t, &Runner{})
			if got := trailingRunSeparates(tc.s, nil, tc.ifs, r.ifsSpace(tc.ifs), true, true); got != tc.want {
				t.Errorf("%q with IFS %q = %v, want %v", tc.s, tc.ifs, got, tc.want)
			}
		})
	}
}
