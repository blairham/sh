// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// answering reads a menu with the given replies, returning what the loop wrote
// to standard error — the menu and the prompt both go there.
func answering(t *testing.T, src, input string, tune func(*Semantics)) (out, errOut string) {
	t.Helper()
	var stderr strings.Builder
	setup := func(r *Runner) {
		r.Stdin = strings.NewReader(input)
		r.Stderr = &stderr
		if tune != nil {
			s := *r.Semantics
			tune(&s)
			r.Semantics = &s
		}
	}
	out, _ = run(t, src, setup)
	return out, stderr.String()
}

// The parts every shell agrees on: what the reply selects, what REPLY holds,
// and that a blank line reprints the menu without running the body.
func TestSelectReadsAReplyAndNamesTheItem(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"in range", "1\n", "got=a rep=1"},
		{"the other one", "2\n", "got=b rep=2"},
		// A reply naming no item is not an error: the name is empty, REPLY
		// still holds what was typed, and the body runs anyway.
		{"out of range", "9\n", "got= rep=9"},
		{"not a number", "zz\n", "got= rep=zz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answering(t, `select x in a b; do echo "got=$x rep=$REPLY"; break; done`, tc.input, nil)
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// A blank reply is the only way to see the menu again. Every other iteration
// reprints the prompt alone.
func TestABlankReplyReprintsTheMenu(t *testing.T) {
	out, errOut := answering(t, `select x in a b; do echo "got=$x"; break; done`, "\n2\n", nil)
	if strings.TrimSpace(out) != "got=b" {
		t.Errorf("body: got %q", out)
	}
	if n := strings.Count(errOut, "1) a"); n != 2 {
		t.Errorf("menu printed %d times, want 2 — once at the start and once for the blank line", n)
	}
	if n := strings.Count(errOut, "#? "); n != 2 {
		t.Errorf("prompt printed %d times, want 2", n)
	}
}

// The menu is printed once and the prompt every time, which is what makes a
// long menu bearable.
func TestTheMenuIsPrintedOnceAndThePromptEachTime(t *testing.T) {
	_, errOut := answering(t, `select x in a b; do echo "$x"; done`, "1\n2\n", nil)
	if n := strings.Count(errOut, "1) a"); n != 1 {
		t.Errorf("menu printed %d times, want 1", n)
	}
	if n := strings.Count(errOut, "#? "); n != 3 {
		t.Errorf("prompt printed %d times, want 3 — two replies and the one that met the end", n)
	}
}

// An empty menu does not prompt at all. The alternative is a loop that asks a
// question with no answers, forever.
func TestAnEmptyMenuDoesNotRun(t *testing.T) {
	out, errOut := answering(t, `select x in; do echo hi; done; echo "st=$?"`, "1\n", nil)
	if strings.TrimSpace(out) != "st=0" {
		t.Errorf("got %q, want st=0", out)
	}
	if errOut != "" {
		t.Errorf("printed %q, want nothing", errOut)
	}
}

// `in` omitted builds the menu from the positional parameters, the same
// distinction the for-loop draws between an absent list and an empty one.
func TestSelectWithNoListUsesThePositionals(t *testing.T) {
	out, _ := answering(t, `set -- p q; select x; do echo "got=$x"; break; done`, "2\n", nil)
	if strings.TrimSpace(out) != "got=q" {
		t.Errorf("got %q, want got=q", out)
	}
}

// PS3 is read fresh each time, so a body that changes it changes the next
// prompt.
func TestThePromptIsReadEachIteration(t *testing.T) {
	_, errOut := answering(t, `PS3=A; select x in a b; do PS3=B; echo "$x"; done`, "1\n2\n", nil)
	if want := "1) a\n2) b\nABB"; errOut != want {
		t.Errorf("got %q, want %q", errOut, want)
	}
}

// The three layouts, named by layout rather than by shell.
func TestSelectMenuLayouts(t *testing.T) {
	const twelve = `select x in 1 2 3 4 5 6 7 8 9 10 11 12; do break; done`
	const three = `select x in a b c; do break; done`
	for _, tc := range []struct {
		name   string
		layout SelectMenuLayout
		src    string
		want   string
	}{
		// Right-aligned numbers, which is what makes `9)` and `10)` line up.
		{
			"vertical aligns the numbers", SelectMenuVertical, twelve,
			" 1) 1\n 2) 2\n 3) 3\n 4) 4\n 5) 5\n 6) 6\n 7) 7\n 8) 8\n 9) 9\n10) 10\n11) 11\n12) 12\n",
		},
		{"vertical with one digit", SelectMenuVertical, three, "1) a\n2) b\n3) c\n"},
		// Fits on one line, so this layout goes vertical — the opposite way
		// round from how it sounds.
		{"tab columns stay vertical when the list fits", SelectMenuVerticalThenColumns, three, "1) a\n2) b\n3) c\n"},
		{
			"tab columns once it does not fit", SelectMenuVerticalThenColumns, twelve,
			"1) 1\t 3) 3\t 5) 5\t 7) 7\t 9) 9\t11) 11\n2) 2\t 4) 4\t 6) 6\t 8) 8\t10) 10\t12) 12\n",
		},
		// Always packed, so even three items share a line, and every cell is
		// padded — the last one included, so a row ends in spaces.
		{"space columns pack even three items", SelectMenuColumns, three, "1) a  2) b  3) c  \n"},
		{
			"space columns fill downwards", SelectMenuColumns, twelve,
			"1) 1    3) 3    5) 5    7) 7    9) 9    11) 11  \n2) 2    4) 4    6) 6    8) 8    10) 10  12) 12  \n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errOut := answering(t, "COLUMNS=80; "+tc.src, "1\n", func(s *Semantics) {
				s.SelectLayout = tc.layout
			})
			if got := strings.TrimSuffix(errOut, "#? "); got != tc.want {
				t.Errorf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

// An unset COLUMNS is 80 in one layout and no limit at all in the other, which
// is why forty items are four lines in one shell and one line in another.
func TestSelectAssumesUnboundedWidthIsAnAxis(t *testing.T) {
	const src = `select x in 1 2 3 4 5 6 7 8 9 10 11 12; do break; done`
	for _, tc := range []struct {
		a    Answer
		rows int
	}{
		{No, 2},
		{Yes, 1},
	} {
		_, errOut := answering(t, src, "1\n", func(s *Semantics) {
			s.SelectLayout = SelectMenuColumns
			s.SelectAssumesUnboundedWidth = tc.a
		})
		if got := strings.Count(errOut, "\n"); got != tc.rows {
			t.Errorf("%v: %d rows, want %d", tc.a, got, tc.rows)
		}
	}
}

// What happens when the input runs out: a status and two newlines, and no
// shell in the panel answers all three the same way.
func TestSelectEndOfInputIsThreeQuestions(t *testing.T) {
	for _, tc := range []struct {
		name              string
		success, nl, line Answer
		wantStatus        int
		wantOut, wantErr  string
	}{
		{"status and a newline on output", No, Yes, No, 1, "\n", "1) a\n#? "},
		{"status and a newline on error", No, No, Yes, 1, "", "1) a\n#? \n"},
		{"success and neither", Yes, No, No, 0, "", "1) a\n#? "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut := answering(t, `select x in a; do :; done`, "", func(s *Semantics) {
				s.SelectEofIsSuccess, s.SelectEofPrintsNewline, s.SelectEofEndsPromptLine = tc.success, tc.nl, tc.line
			})
			if out != tc.wantOut {
				t.Errorf("stdout %q, want %q", out, tc.wantOut)
			}
			if errOut != tc.wantErr {
				t.Errorf("stderr %q, want %q", errOut, tc.wantErr)
			}
		})
	}
}

// The status the loop leaves when the input ends.
func TestSelectEofStatusIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		a    Answer
		want int
	}{{Yes, 0}, {No, 1}} {
		var stderr strings.Builder
		_, status := run(t, `select x in a; do :; done`, func(r *Runner) {
			r.Stdin, r.Stderr = strings.NewReader(""), &stderr
			s := *r.Semantics
			s.SelectEofIsSuccess, s.SelectEofPrintsNewline = tc.a, No
			r.Semantics = &s
		})
		if status != tc.want {
			t.Errorf("%v: status %d, want %d", tc.a, status, tc.want)
		}
	}
}

// The prompt is withheld unless the input is a terminal, which one shell does
// and the others do not. A strings.Reader is not one, so the axis decides.
func TestSelectPromptNeedsTerminalIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		a       Answer
		wantErr string
	}{
		{No, "1) a\n#? "},
		{Yes, "1) a\n"},
	} {
		_, errOut := answering(t, `select x in a; do :; done`, "", func(s *Semantics) {
			s.SelectPromptNeedsTerminal = tc.a
			s.SelectEofPrintsNewline, s.SelectEofEndsPromptLine = No, No
		})
		if errOut != tc.wantErr {
			t.Errorf("%v: got %q, want %q", tc.a, errOut, tc.wantErr)
		}
	}
}
