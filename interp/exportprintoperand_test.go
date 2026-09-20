// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"regexp"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What `export -p` and `readonly -p` do once operands are written —
// Semantics.ExportOrReadonlyPrintWithOperands. Three readings, and the rows
// below are what need all three: a name the shell already carries separates
// "listed" from "not listed", a `name=value` operand separates "declared"
// from "not declared", and a *second* attributed name separates "narrowed to
// the operand" from "the whole table anyway". Tests name the axis and never a
// shell; see interp/exportprintoperand.go for the panel.

func printOperandSem(p ExportPrintOperandPolicy) Semantics {
	s := testSemantics()
	s.ExportListing = DeclareListingCommandWord
	s.ReadonlyListing = DeclareListingCommandWord
	s.DeclareValueQuoting = ListingQuoteWhenNeededDollar
	s.ExportOrReadonlyPrintWithOperands = p
	return s
}

// ownRow keeps the listing rows for names this suite wrote and drops the
// rest, so the environment the test process was started with — TMPDIR and
// its neighbors — does not decide whether a listing counts as narrowed.
// Everything that is not a listing row, an `echo` above all, is kept.
var ownRow = regexp.MustCompile(`^(export|readonly) (a1|a2|e|r|s|t|u|v|w)=`)

func printOperandRun(t *testing.T, src string, p ExportPrintOperandPolicy) (string, string) {
	t.Helper()
	out, errs := builtinPrefixRun(t, src, printOperandSem(p))
	var kept []string
	for _, line := range strings.SplitAfter(out, "\n") {
		if strings.HasPrefix(line, "export ") || strings.HasPrefix(line, "readonly ") {
			if !ownRow.MatchString(line) {
				continue
			}
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, ""), errs
}

// The letter is inert: nothing is listed and the operand is declared exactly
// as the same line without `-p` would declare it — the reading bash, ksh93
// and BusyBox ash share.
func TestThePrintLetterCanBeInertOnceOperandsAreWritten(t *testing.T) {
	t.Parallel()
	out, errs := printOperandRun(t,
		"export s=5\nexport -p s\necho \"[$s]\"", ExportPrintLetterIsInert)
	if want := "[5]\n"; out != want {
		t.Errorf("export -p s = %q (err %q), want %q — nothing listed", out, errs, want)
	}
	// The operand is *performed*, which is what parts this reading from the
	// one that drops its operands: a `name=value` that no earlier line set
	// is stored and exported by the `-p` line itself.
	out, errs = printOperandRun(t,
		"export -p w=8\necho \"[$w]\"\nexport -p", ExportPrintLetterIsInert)
	if want := "[8]\nexport w=8\n"; out != want {
		t.Errorf("export -p w=8 = %q (err %q), want %q", out, errs, want)
	}
	out, errs = printOperandRun(t,
		"readonly -p u=9\necho \"[$u]\"\nreadonly -p", ExportPrintLetterIsInert)
	if want := "[9]\nreadonly u=9\n"; out != want {
		t.Errorf("readonly -p u=9 = %q (err %q), want %q", out, errs, want)
	}
}

// The listing narrows to the operands and nothing is declared — zsh's
// reading. The second attributed name is the row that makes "narrowed" a
// claim rather than a coincidence.
func TestThePrintLetterCanNarrowToItsOperands(t *testing.T) {
	t.Parallel()
	out, errs := printOperandRun(t,
		"export a1=1\nexport a2=2\nexport -p a1", ExportPrintNarrowsToTheOperands)
	if want := "export a1=1\n"; out != want {
		t.Errorf("export -p a1 = %q (err %q), want %q — the named name alone", out, errs, want)
	}
	out, errs = printOperandRun(t,
		"readonly t=6\nreadonly v=7\nreadonly -p t", ExportPrintNarrowsToTheOperands)
	if want := "readonly t=6\n"; out != want {
		t.Errorf("readonly -p t = %q (err %q), want %q", out, errs, want)
	}
	// And nothing is declared: the operand is a name to list, so a `w` the
	// script never set is still unset after the line.
	out, _ = printOperandRun(t,
		"export -p w=8\necho \"[${w-gone}]\"", ExportPrintNarrowsToTheOperands)
	if !strings.HasSuffix(out, "[gone]\n") {
		t.Errorf("export -p w=8 left %q, want the name unset", out)
	}
}

// The operand's **name** is what the listing narrows to, and not the whole
// word — which is the half that costs something, because a name the script
// really has is then *listed* rather than reported missing (#3922).
//
// Measured 2026-09-20, zsh 5.9.2 at `/opt/homebrew`, a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null
// device: `export e1=1; export -p e1=9` writes `export e1=1` at 0, and
// `readonly -p u=9` over a name nothing set is `no such variable: u` — the
// name alone, with the value gone.
//
// The same reading and the same stripper as Semantics
// .DeclarePrintPerformsItsOperand's DeclarePrintOperandIsANameAlone, because
// it is one column saying one thing through two words.
func TestThePrintLetterNarrowsToTheOperandsNameAndNotTheWholeWord(t *testing.T) {
	t.Parallel()
	out, errs := printOperandRun(t,
		"export e=1\nexport -p e=9\necho \"[$e]\"", ExportPrintNarrowsToTheOperands)
	if want := "export e=1\n[1]\n"; out != want {
		t.Errorf("export -p e=9 = %q (err %q), want %q — the name is `e`, which the "+
			"script has, so the listing writes its row and nothing is stored",
			out, errs, want)
	}
	out, errs = printOperandRun(t,
		"readonly r=6\nreadonly -p r=7\necho \"[$r]\"", ExportPrintNarrowsToTheOperands)
	if want := "readonly r=6\n[6]\n"; out != want {
		t.Errorf("readonly -p r=7 = %q (err %q), want %q", out, errs, want)
	}
	// And the control on the other side: a name the script does *not* have is
	// still missing, reported under the name rather than under the word.
	_, errs = printOperandRun(t,
		"export -p w=8", ExportPrintNarrowsToTheOperands)
	if strings.Contains(errs, "w=8") {
		t.Errorf("export -p w=8 said %q, want the name alone and not the whole word", errs)
	}
}

// The operands are dropped: the whole-table listing runs and the names are
// neither listed nor declared — dash's reading, and the one the other two
// cannot imitate. The discriminating row is the *unnamed* export appearing
// anyway.
func TestThePrintLetterCanDropItsOperands(t *testing.T) {
	t.Parallel()
	out, errs := printOperandRun(t,
		"export a1=1\nexport a2=2\nexport -p a1", ExportPrintDropsTheOperands)
	if want := "export a1=1\nexport a2=2\n"; out != want {
		t.Errorf("export -p a1 = %q (err %q), want %q — the whole listing", out, errs, want)
	}
	out, _ = printOperandRun(t,
		"export -p w=8\necho \"[${w-gone}]\"", ExportPrintDropsTheOperands)
	if !strings.HasSuffix(out, "[gone]\n") {
		t.Errorf("export -p w=8 left %q, want the name unset", out)
	}
	out, errs = printOperandRun(t,
		"readonly t=6\nreadonly -p u=9\necho \"[${u-gone}]\"", ExportPrintDropsTheOperands)
	if want := "readonly t=6\n[gone]\n"; out != want {
		t.Errorf("readonly -p u=9 = %q (err %q), want %q", out, errs, want)
	}
}

// A dialect that has not said which of the three it means declares nothing
// and refuses by name, rather than being given one shell's reading. The three
// readings differ over whether the name is set afterwards, which is the kind
// of difference no later command can report.
func TestAnUnansweredPrintOperandAxisIsRefused(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, read string }{
		{"export -p w=8", "${w-gone}"},
		{"readonly -p u=9", "${u-gone}"},
	} {
		out, errs := printOperandRun(t, tc.src+"\necho \"["+tc.read+"]\"",
			ExportPrintOperandUnspecified)
		if !strings.Contains(errs, "no dialect was chosen") {
			t.Errorf("%s: stderr %q, want the axis refused by name", tc.src, errs)
		}
		if want := "[gone]\n"; out != want {
			t.Errorf("%s: %q, want %q — nothing listed and nothing declared", tc.src, out, want)
		}
	}
}

// The bare spellings raise no question at all, under every reading including
// the absent one: `export -p` and `readonly -p` with no operand are what
// every script writes and what every column of the panel answers alike.
func TestThePrintLetterWithNoOperandAsksNothing(t *testing.T) {
	t.Parallel()
	for _, p := range []ExportPrintOperandPolicy{
		ExportPrintOperandUnspecified,
		ExportPrintLetterIsInert,
		ExportPrintNarrowsToTheOperands,
		ExportPrintDropsTheOperands,
	} {
		out, errs := printOperandRun(t, "export e=1\nreadonly r=2\nexport -p\nreadonly -p", p)
		if want := "export e=1\nreadonly r=2\n"; out != want {
			t.Errorf("%v: %q (err %q), want %q", p, out, errs, want)
		}
		if errs != "" {
			t.Errorf("%v: stderr %q, want the bare listing to ask no axis", p, errs)
		}
	}
}
