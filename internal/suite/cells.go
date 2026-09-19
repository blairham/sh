// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"fmt"
	"sort"
	"strings"
)

// CellState is what a `(column, area)` cell is.
type CellState string

const (
	// CellClosed is a gated column that runs a file for the area. The
	// column's reference is pinned inside a digest-pinned image, so the
	// figure behind this cell is the same on a runner as on a laptop.
	CellClosed CellState = "closed"
	// CellOpen is a column that runs a file for the area and is not gated.
	// It is work: pin the reference, or say why it cannot be pinned and
	// move the cell to the ledger.
	CellOpen CellState = "open"
	// CellLedgered is a cell no amount of correct work can close, with the
	// measurement that says so in [UnclosableByConstruction].
	CellLedgered CellState = "ledgered"
	// CellNoFile is a column that runs no file for the area at all. It is
	// counted apart from open, because "nobody has written the case" and
	// "the case is written and the column is not pinned" are two different
	// pieces of work and folding them together is how leg 2's first number
	// stopped meaning anything.
	CellNoFile CellState = "no file"
)

// Cell is one `(column, area)` of the space #2291's second leg counts.
type Cell struct {
	Column string
	Area   Area
	State  CellState
	// Why is filled in for a ledgered cell, from the ledger entry.
	Why string
	// Issue is the ledger entry's issue, for a ledgered cell.
	Issue int
}

// Unclosable is a cell that cannot be closed by any amount of correct work,
// together with the measurement that says so.
//
// This is the hand-kept list #3481 exists to create, and it is kept on
// [coverage.UnreachableByConstruction]'s terms, deliberately: prose beside the
// files rots silently and this repository has the scars for it — the `lint`
// target that was "documented as discouraged" while three agents ran it
// anyway, and the suite metric whose prose misdescribed it into three briefs.
// So an entry here is not a forgiveness. It is a cell **closed by measurement
// rather than by a file**, which is a different fact and one the roll-up
// prints beside the work list rather than folding into it.
//
// Each entry carries the issue holding the evidence and the measurement
// itself, where the next reader meets it instead of re-deriving it. And every
// entry is checked against the columns it is about on every run: an entry
// naming a column that has since been gated, or a column or area that is no
// longer in the tree, is **stale** rather than quietly subtracted. A ledger
// that could only grow would be the overstatement this mechanism exists to
// prevent, arriving by a side door — which is why
// [TestTheLedgerIsCheckedAgainstTheColumns] mutates a gate on and requires the
// entry to fail.
type Unclosable struct {
	// Column is the [Ours] column the entry is about, by Name.
	Column string
	// Area is the area's name, or empty for every area of that column.
	Area string
	// Issue is where the measurement is recorded in full.
	Issue int
	// Measured is the measurement, short enough to print.
	Measured string
}

// UnclosableByConstruction is the ledger. See [Unclosable].
var UnclosableByConstruction = []Unclosable{
	{
		Column: "ksh93",
		Issue:  3480,
		Measured: "there is no image of the build this column is graded against and there " +
			"will not be one: `Against` is AT&T ksh93 93u+ 2012-08-01, which no " +
			"distribution packages, and the maintained lineage is ksh93u+m — a fork " +
			"twelve years on. Gating this column against ksh93u+m would pin the figure " +
			"and measure the fork, which is a worse answer than an unpinned one and " +
			"looks better. The fragment that tells the two apart is the whole date, " +
			"because `93u+` is a prefix of `93u+m/1.0.8` and clears it (#3135); a " +
			"gate built on the shorter fragment would report all-clear on the exact " +
			"case it was written for. #3480 excludes this column for that reason and " +
			"keeps it report-only, so its 25 cells are closed by this measurement " +
			"rather than waiting on work nobody can do.",
	},
}

// Cells is the whole space, one entry per column per area, sorted by column
// and then by area.
//
// root is where our own suite lives, and the files are read from it rather
// than assumed: a cell is about what a column *runs*, so a column claiming a
// tier that is not there has to come out as [CellNoFile] rather than as a
// closed cell nobody can check.
func Cells(root string) ([]Cell, error) {
	held, err := Basenames(root)
	if err != nil {
		return nil, err
	}
	var cells []Cell
	for _, col := range Ours {
		runs := map[string]bool{}
		for _, d := range col.Dirs {
			runs[d] = true
		}
		for _, area := range Areas {
			state := CellNoFile
			for _, base := range area.Files {
				for _, dir := range held[base] {
					if runs[dir] {
						state = CellOpen
					}
				}
			}
			if state == CellOpen && col.Contained() {
				state = CellClosed
			}
			cell := Cell{Column: col.Name, Area: area, State: state}
			if u, ok := ledgered(col.Name, area.Name); ok {
				cell.State, cell.Why, cell.Issue = CellLedgered, u.Measured, u.Issue
			}
			cells = append(cells, cell)
		}
	}
	return cells, nil
}

// ledgered is the ledger entry for a cell, if there is one.
func ledgered(column, area string) (Unclosable, bool) {
	for _, u := range UnclosableByConstruction {
		if u.Column == column && (u.Area == "" || u.Area == area) {
			return u, true
		}
	}
	return Unclosable{}, false
}

// Roll is the count the campaign's second leg is quoted from.
type Roll struct {
	Cells    []Cell
	Closed   int
	Open     int
	Ledgered int
	NoFile   int
	// Stale is the ledger entries that no longer describe the tree, in the
	// words the roll-up prints. An entry that has stopped being true is a
	// finding rather than a silent subtraction.
	Stale []string
}

// Total is the denominator.
func (r Roll) Total() int { return len(r.Cells) }

// RollUp counts the cells and checks the ledger against them.
func RollUp(root string) (Roll, error) {
	cells, err := Cells(root)
	if err != nil {
		return Roll{}, err
	}
	roll := Roll{Cells: cells}
	for _, c := range cells {
		switch c.State {
		case CellClosed:
			roll.Closed++
		case CellOpen:
			roll.Open++
		case CellLedgered:
			roll.Ledgered++
		case CellNoFile:
			roll.NoFile++
		}
	}
	roll.Stale = StaleLedgerEntries(cells)
	return roll, nil
}

// StaleLedgerEntries is every way a ledger entry can have stopped being true,
// in the words the roll-up prints.
//
// Four ways, and each is a different thing that happened:
//
//	the column is gone      renamed or retired out of Ours
//	the area is gone        renamed or retired out of Areas
//	the column is gated     somebody pinned it, so the cells are closable
//	                        after all and the entry is now hiding work
//	nothing to hold         an entry with no issue or no measurement is a
//	                        forgiveness, which is not what a ledger is for
//
// The third is the one this mechanism is for. A ledger nobody re-checks is a
// count that only ever falls, and the day a column becomes gatable is exactly
// the day nobody is looking at the reason it was not.
func StaleLedgerEntries(cells []Cell) []string {
	var stale []string
	for _, u := range UnclosableByConstruction {
		col, ok := findOursByName(u.Column)
		if !ok {
			stale = append(stale, fmt.Sprintf("%s — no column of that name is in the suite panel", u.Column))
			continue
		}
		if u.Area != "" {
			if _, found := findArea(u.Area); !found {
				stale = append(stale, fmt.Sprintf("%s %s — no area of that name is in Areas", u.Column, u.Area))
				continue
			}
		}
		if col.Contained() {
			stale = append(stale, fmt.Sprintf(
				"%s%s — the column is gated now, so its cells are closable and this entry is holding work out of the open count",
				u.Column, areaSuffix(u.Area)))
			continue
		}
		if u.Issue == 0 || len(u.Measured) < 40 {
			stale = append(stale, fmt.Sprintf(
				"%s%s — ledgered without the measurement that says it cannot be closed",
				u.Column, areaSuffix(u.Area)))
		}
	}
	// A cell the ledger claims and the space does not hold is the same
	// failure one level down, and it is checked from the cells rather than
	// from the table so that a rename on either side is caught from both.
	claimed := map[string]bool{}
	for _, c := range cells {
		if c.State == CellLedgered {
			claimed[c.Column] = true
		}
	}
	for _, u := range UnclosableByConstruction {
		if _, ok := findOursByName(u.Column); ok && !claimed[u.Column] {
			stale = append(stale, fmt.Sprintf("%s%s — the ledger entry reaches no cell", u.Column, areaSuffix(u.Area)))
		}
	}
	sort.Strings(stale)
	return stale
}

func areaSuffix(area string) string {
	if area == "" {
		return ""
	}
	return " " + area
}

func findArea(name string) (Area, bool) {
	for _, a := range Areas {
		if a.Name == name {
			return a, true
		}
	}
	return Area{}, false
}

// StaleUnclaimedFiles is the same check for the smaller ledger: a file named
// in [UnclaimedByShape] that an area has since claimed, or that is no longer
// in the tree.
func StaleUnclaimedFiles(root string) ([]string, error) {
	held, err := Basenames(root)
	if err != nil {
		return nil, err
	}
	var stale []string
	for _, u := range UnclaimedByShape {
		if _, claimed := FindArea(u.File); claimed {
			stale = append(stale, fmt.Sprintf("%s — an area claims it now, so it is not unclaimed", u.File))
			continue
		}
		if len(held[u.File]) == 0 {
			stale = append(stale, fmt.Sprintf("%s — no tier holds a file of that name any more", u.File))
		}
	}
	sort.Strings(stale)
	return stale, nil
}

// Report is the roll-up as the instruments print it.
func (r Roll) Report() string {
	var b strings.Builder
	fmt.Fprintf(&b, "  cells      %d of %d closed · %d open · %d closed by measurement · %d with no file\n",
		r.Closed, r.Total(), r.Open, r.Ledgered, r.NoFile)
	fmt.Fprintf(&b, "             %d columns x %d areas. A cell is closed when the column's\n",
		len(Ours), len(Areas))
	b.WriteString("             reference is pinned and it runs a file for that area — #2291's\n")
	b.WriteString("             leg 2, counted from something a reader can re-derive.\n")
	byColumn := map[string]map[CellState]int{}
	for _, c := range r.Cells {
		if byColumn[c.Column] == nil {
			byColumn[c.Column] = map[CellState]int{}
		}
		byColumn[c.Column][c.State]++
	}
	for _, col := range Ours {
		counts := byColumn[col.Name]
		fmt.Fprintf(&b, "    %-7s closed %2d · open %2d · measured %2d · no file %2d\n",
			col.Name, counts[CellClosed], counts[CellOpen], counts[CellLedgered], counts[CellNoFile])
	}
	if len(UnclosableByConstruction) > 0 {
		b.WriteString("\n  closed by measurement rather than by a file\n")
		for _, u := range UnclosableByConstruction {
			fmt.Fprintf(&b, "    %s\n", fold(
				fmt.Sprintf("%s%s (#%d) — %s", u.Column, areaSuffix(u.Area), u.Issue, u.Measured), "      "))
		}
	}
	if len(UnclaimedByShape) > 0 {
		b.WriteString("\n  files no area claims\n")
		for _, u := range UnclaimedByShape {
			fmt.Fprintf(&b, "    %s\n", fold(u.File+" — "+u.Why, "      "))
		}
	}
	if len(r.Stale) > 0 {
		b.WriteString("\n  STALE LEDGER ENTRIES\n")
		for _, s := range r.Stale {
			fmt.Fprintf(&b, "    %s\n", s)
		}
	}
	return b.String()
}

// fold wraps a ledger entry so a measurement long enough to be evidence is
// still readable in a terminal. The measurements here are paragraphs on
// purpose — a one-line reason is the forgiveness this ledger is not for.
func fold(text, indent string) string {
	const width = 74
	var out strings.Builder
	line := 0
	for i, word := range strings.Fields(text) {
		switch {
		case line > 0 && line+1+len(word) > width:
			out.WriteString("\n" + indent)
			line = 0
		case i > 0:
			out.WriteString(" ")
			line++
		}
		out.WriteString(word)
		line += len(word)
	}
	return out.String()
}
