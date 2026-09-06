// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `$'...'` listing has to escape every control byte, and the three engines
// that write one spell a byte three ways. Measured 2026-09-05 from
// `v=$'\a\b\t\n\v\f\r\e\001\037\177'` in each of them; the whole line is
// asserted, because the difference is in which bytes get a name and what the
// rest fall back to.
//
// The last of the three is the reason this is not a detail: that shell keeps a
// NUL in IFS, and a listing that writes it raw is binary — `grep` answers
// "Binary file (standard input) matches" instead of the line.
func TestListingControlEscapesAreThreeVocabularies(t *testing.T) {
	src := "v=$'\a\b\t\n\v\f\r\x1b\x01\x1f\x7f'\nset"
	for _, tc := range []struct {
		name  string
		style ControlEscapeStyle
		want  string
	}{
		{"octal", ControlEscapeOctal, `v=$'\a\b\t\n\v\f\r\E\001\037\177'` + "\n"},
		{"hex", ControlEscapeHex, `v=$'\a\b\t\n\x0b\f\r\E\x01\x1f\x7f'` + "\n"},
		{"caret", ControlEscapeCaret, `v=$'\C-G\C-H\t\n\C-K\C-L\C-M\C-[\C-A\C-_\C-?'` + "\n"},
	} {
		out, errs, st := listRun(t, src, func(s *Semantics) {
			s.SetListing = SetListingAssignments
			s.SetListingQuoting = ListingQuoteWhenNeededEscaped
			s.ListingControlEscape = tc.style
		})
		if errs != "" || st != 0 {
			t.Fatalf("%s: errs %q status %d, want a listing and 0", tc.name, errs, st)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: stdout = %q, want %q in it", tc.name, out, tc.want)
		}
	}
}

// A NUL is the byte that makes the difference visible: no style writes it as
// itself, and the caret one is the only place it has a name.
func TestAListedNulIsNeverWrittenRaw(t *testing.T) {
	for _, tc := range []struct {
		style ControlEscapeStyle
		want  string
	}{
		{ControlEscapeOctal, `v=$'a\000b'` + "\n"},
		{ControlEscapeHex, `v=$'a\x00b'` + "\n"},
		{ControlEscapeCaret, `v=$'a\C-@b'` + "\n"},
	} {
		out, _, _ := listRun(t, "v=$'a\x00b'\nset", func(s *Semantics) {
			s.SetListing = SetListingAssignments
			s.SetListingQuoting = ListingQuoteWhenNeededEscaped
			s.ListingControlEscape = tc.style
		})
		if !strings.Contains(out, tc.want) {
			t.Errorf("%v: stdout = %q, want %q in it", tc.style, out, tc.want)
		}
		if strings.Contains(out, "v=$'a\x00b'") {
			t.Errorf("%v: stdout = %q, wrote the NUL as itself", tc.style, out)
		}
	}
}

// The bare `export` and `readonly` are not always the shape `-p` writes: two
// shells drop the command word for the bare form alone and leave a plain
// assignment, which no `-p` anywhere writes.
func TestBareExportAndReadonlyHaveAShapeOfTheirOwn(t *testing.T) {
	// TMPDIR arrives exported from the harness and would list too; the case
	// is about the shape of a row rather than about which rows there are.
	src := "unset TMPDIR\nexport V='a b'\nreadonly R=2\nexport\nreadonly\nexport -p\nreadonly -p"
	out, errs, st := listRun(t, src, func(s *Semantics) {
		s.ExportListing = DeclareListingCommandWord
		s.ReadonlyListing = DeclareListingCommandWord
		s.BareDeclarationListing = DeclareListingPlainAssignment
		s.DeclareValueQuoting = ListingQuoteWhenNeededEscaped
	})
	want := "V='a b'\nR=2\nexport V='a b'\nreadonly R=2\n"
	if errs != "" || st != 0 || out != want {
		t.Errorf("out %q errs %q status %d, want %q at 0", out, errs, st, want)
	}

	// And where the two shapes agree, asking twice gives the same answer.
	out, _, _ = listRun(t, src, func(s *Semantics) {
		s.ExportListing = DeclareListingCommandWord
		s.ReadonlyListing = DeclareListingCommandWord
		s.BareDeclarationListing = DeclareListingCommandWord
		s.DeclareValueQuoting = ListingQuoteWhenNeededEscaped
	})
	want = "export V='a b'\nreadonly R=2\nexport V='a b'\nreadonly R=2\n"
	if out != want {
		t.Errorf("out %q, want %q", out, want)
	}
}
