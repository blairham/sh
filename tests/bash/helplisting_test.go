// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// The listing a bare `help` writes: two columns, cells cut to the column with
// a `>` where they were cut.
//
// The geometry is measured rather than chosen. Read out of the real shell's
// own bytes on 2026-09-18 at COLUMNS 8, 10, 12, 40, 60, 80, 100 and 200, with
// the shell on a pipe and no terminal anywhere — it is a property of a
// program's output, which is why it can be matched where the header above it
// cannot. This shell wrote one topic per line, 75 lines against 47 (#3055).
func TestTheHelpListingIsTwoColumns(t *testing.T) {
	for _, columns := range []string{"40", "60", "80", "100", "8", "10"} {
		t.Run("COLUMNS="+columns, func(t *testing.T) {
			out, errs := runOptionScript(t, "COLUMNS="+columns+"\nhelp\n")
			if errs != "" {
				t.Fatalf("complained: %q", errs)
			}
			lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			// The one header line, which is the version and is not a row —
			// see TestTheHelpListingOpensWithOneVersionLine. The geometry
			// below is the rows'.
			lines = lines[1:]
			half := 0
			for _, c := range columns {
				half = half*10 + int(c-'0')
			}
			half /= 2

			for _, line := range lines {
				if len(line) > half*2-2 {
					t.Fatalf("line %q is %d characters, want at most %d", line, len(line), half*2-2)
				}
				if !strings.HasPrefix(line, " ") {
					t.Fatalf("line %q does not open with the one space every row has", line)
				}
			}
			// Every row but the last carries a right cell, and it starts at
			// the same place on every one of them.
			for _, line := range lines[:len(lines)-1] {
				if len(line) <= half {
					t.Fatalf("line %q has no right cell", line)
				}
				if line[half-1] != ' ' || line[half] != ' ' {
					t.Fatalf("line %q does not separate its cells with two spaces at %d", line, half-1)
				}
				if line[half+1] == ' ' {
					t.Fatalf("line %q pads past the separator", line)
				}
			}
			// A cut cell says so, and the narrow widths are where that is
			// every cell.
			if columns == "10" {
				for _, line := range lines {
					if !strings.HasSuffix(line, ">") {
						t.Errorf("line %q is not cut, at a width where every cell is", line)
					}
				}
			}
		})
	}
}

// The listing is half as many lines as the topics, column-major, and no line
// carries the `name: ` prefix the one-per-line form has.
func TestTheHelpListingIsColumnMajorAndHalfAsLong(t *testing.T) {
	perLine, _ := runOptionScript(t, "help -s ''\n")
	topics := strings.Split(strings.TrimSuffix(perLine, "\n"), "\n")
	listing, _ := runOptionScript(t, "COLUMNS=200\nhelp\n")
	rows := strings.Split(strings.TrimSuffix(listing, "\n"), "\n")[1:]
	if want := (len(topics) + 1) / 2; len(rows) != want {
		t.Fatalf("%d topics came to %d rows, want %d", len(topics), len(rows), want)
	}
	// Column-major: the left cell of row 0 is the first topic and the right
	// cell of row 0 is the one that opens the second half.
	first := strings.TrimPrefix(topics[0], strings.SplitN(topics[0], ": ", 2)[0]+": ")
	if !strings.HasPrefix(rows[0], " "+first) {
		t.Errorf("row 0 is %q, want it to open with %q", rows[0], first)
	}
	mid := topics[len(rows)]
	midSynopsis := strings.TrimPrefix(mid, strings.SplitN(mid, ": ", 2)[0]+": ")
	if !strings.Contains(rows[0], midSynopsis) {
		t.Errorf("row 0 is %q, want its right cell to be %q — the listing is column-major", rows[0], midSynopsis)
	}
	// And the cells are synopses, not `name: synopsis`.
	if strings.HasPrefix(strings.TrimSpace(rows[0]), strings.SplitN(topics[0], ":", 2)[0]+": ") {
		t.Errorf("row 0 %q carries the name prefix the listing does not have", rows[0])
	}
}

// An **empty** operand is a different answer from no operand at all, and this
// is the row that keeps the change above from taking both.
//
// Measured 2026-09-18: `help -s ”` writes 77 lines of `name: synopsis` and
// `help -s` writes the 47-line listing.
func TestAnEmptyOperandIsNotTheListing(t *testing.T) {
	perLine, errs := runOptionScript(t, "help -s ''\n")
	if errs != "" {
		t.Fatalf("complained: %q", errs)
	}
	if !strings.HasPrefix(perLine, "!: ! PIPELINE\n") {
		t.Errorf("an empty operand wrote %q…, want the one-per-line form", perLine[:40])
	}
	listing, _ := runOptionScript(t, "COLUMNS=80\nhelp -s\n")
	if strings.HasPrefix(listing, "!: ") {
		t.Errorf("no operand wrote the one-per-line form: %q…", listing[:40])
	}
}

// A width the shell cannot lay out in falls back to 80.
//
// Measured 2026-09-18 with the shell on a pipe: an unset COLUMNS, a value that
// is not a number, a negative one, and every value up to 7 all lay out exactly
// as 80 does; 8 is the first that is used.
func TestAnUnusableColumnsFallsBackToEighty(t *testing.T) {
	eighty, _ := runOptionScript(t, "COLUMNS=80\nhelp\n")
	for _, columns := range []string{"", "abc", "-5", "0", "1", "7"} {
		src := "COLUMNS=" + columns + "\nhelp\n"
		if columns == "" {
			src = "unset COLUMNS\nhelp\n"
		}
		got, errs := runOptionScript(t, src)
		if errs != "" {
			t.Fatalf("COLUMNS=%q complained: %q", columns, errs)
		}
		if got != eighty {
			t.Errorf("COLUMNS=%q laid out differently from 80", columns)
		}
	}
	// And 8 is used, which is the control: without it the row above would
	// pass for a shell that ignored COLUMNS entirely.
	if got, _ := runOptionScript(t, "COLUMNS=8\nhelp\n"); got == eighty {
		t.Error("COLUMNS=8 laid out as 80, so nothing here reads the parameter")
	}
}

// The listing opens with one line that is not a row: this shell's version,
// spelled the way `--version` spells it.
//
// The seven other header lines the real shell writes stay out — four
// sentences of another project's prose, the sentence about the `*` mark and
// two blanks — and this one is in because it has a measurable cost when it is
// missing. A script reaching past the header drops the first line, which is
// the natural way past a version line; with no header at all that takes a
// **row of the listing** instead, and the lost row is the first, which
// carries two topics. Measured 2026-09-21 against bash 5.3.20 over the
// `builtins.tests` row of #2298: 8 header lines + 39 rows there against
// 0 + 39 here (#4066).
//
// The spelling is `--version`'s and not `--help`'s, which differ by one
// character in the real shell and differ here for the same reason.
func TestTheHelpListingOpensWithOneVersionLine(t *testing.T) {
	listing, _ := runOptionScript(t, "COLUMNS=200\nhelp\n")
	version, _ := runOptionScript(t, "echo \"$BASH_VERSION\"\n")
	first, rest, _ := strings.Cut(listing, "\n")
	if !strings.HasPrefix(first, "GNU bash, version "+strings.TrimSpace(version)) {
		t.Errorf("the listing opens with %q, want the version line", first)
	}
	// And exactly one: the line under it is a row, which opens with the
	// space every row opens with and no row is blank.
	second, _, _ := strings.Cut(rest, "\n")
	if !strings.HasPrefix(second, " ") || strings.TrimSpace(second) == "" {
		t.Errorf("the line under the version is %q, want the first row", second)
	}
	// A script that strips the first line keeps every row, which is the
	// cost this line is here to remove.
	perLine, _ := runOptionScript(t, "help -s ''\n")
	topics := strings.Split(strings.TrimSuffix(perLine, "\n"), "\n")
	stripped := strings.Split(strings.TrimSuffix(rest, "\n"), "\n")
	if want := (len(topics) + 1) / 2; len(stripped) != want {
		t.Errorf("stripping the first line left %d rows, want all %d", len(stripped), want)
	}
}
