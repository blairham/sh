// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package sandboxcheck

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Text is the table and the summary.
//
// Grouped by route with the dialects across one line, because the question a
// reader has is "is this way in closed", and a route closed in three shells
// and open in the fourth is the interesting shape — which a list sorted by
// dialect would scatter down the page.
func (r Report) Text(verbose bool) string {
	var b strings.Builder
	w := &b
	fmt.Fprintf(w, "sandbox: %d routes × dialects × %d policy shapes, against %s\n",
		len(Routes()), len(Shapes), r.Shell)

	// One table per denied policy shape, rather than one table with two marks
	// per cell. The two shapes ask different questions of the same gate — a
	// boundary that holds and a deny that holds inside one — and a row is only
	// as good as the weaker of them, so each has to be readable on its own.
	for _, shape := range Shapes {
		fmt.Fprintf(w, "\n%s\n", shapeHeading(shape))
		r.table(w, shape)
	}

	// The inert ledger. Printed always rather than under -v, because these
	// are the rows that become escapes the day the feature lands, and a list
	// nobody sees is not a ledger.
	var inert []string
	for _, res := range r.Results {
		if res.Verdict == Inert {
			inert = append(inert, res.Route+"/"+res.Dialect+" ("+res.Shape.String()+")")
		}
	}
	if len(inert) > 0 {
		fmt.Fprintf(w, "\n  not reachable yet — each becomes an escape the day it works:\n")
		for _, s := range inert {
			fmt.Fprintf(w, "    %s\n", s)
		}
	}

	// The documented ledger, printed on the same terms and for the same
	// reason: these rows escape, they are the boundary's stated limits
	// rather than its defects, and a limit that is only ever described in a
	// document is one nothing re-measures. Each row names where the reason
	// is written, so the claim can be checked rather than taken.
	var documented []string
	for _, res := range r.Results {
		if res.Verdict == Documented {
			documented = append(documented,
				res.Route+"/"+res.Dialect+" ("+res.Shape.String()+") — "+res.Cites)
		}
	}
	if len(documented) > 0 {
		fmt.Fprintf(w, "\n  escapes, and is documented as escaping — each goes green by itself\n"+
			"  the day a Gate contains the process tree:\n")
		for _, s := range documented {
			fmt.Fprintf(w, "    %s\n", s)
		}
	}

	for _, res := range r.Results {
		if res.Verdict == Contained && !verbose {
			continue
		}
		if res.Verdict == Inert && !verbose {
			continue
		}
		// Same rule as inert, and for the same reason: the ledger above
		// already names every documented row and says where its reason is
		// written, so repeating three identical runs for each of them
		// buries the rows that are printed here because they need
		// explaining. `-v` still shows the leak.
		if res.Verdict == Documented && !verbose {
			continue
		}
		fmt.Fprintf(w, "\n  %s (%s, %s): %s\n", res.Route, res.Dialect, res.Shape, res.Verdict)
		for i, label := range []string{"ungated", "denied ", "allowed"} {
			o := res.Runs[i]
			fmt.Fprintf(w, "    %s  code=%d out=%q err=%q\n",
				label, o.Code, trim(o.Out), trim(o.Err))
		}
	}
	return b.String()
}

// shapeHeading says which question the table under it answers, in the words
// the reader needs rather than the one-word name.
//
// Both are spelled out on every run, including the run where they agree. The
// whole finding behind #2055 is that a table can be complete about the routes
// it has and silent about the policy shape those routes were graded under, so
// the shape a reader is looking at is never left implicit.
func shapeHeading(s Shape) string {
	if s == Carved {
		return "  carved-out of an allowed region — `allow <ws>/**` plus `deny <ws>/off`,\n" +
			"  with the route aiming at the denied region inside the workspace.\n" +
			"  Answers: does a deny hold inside a region the policy otherwise allows?"
	}
	return "  outside the workspace — `default deny` plus `allow <ws>/**`, with the\n" +
		"  route aiming beyond it.\n" +
		"  Answers: does the boundary of an allowed region hold?"
}

// table prints one shape's grid and its counts.
func (r Report) table(w *strings.Builder, shape Shape) {
	byRoute := map[string]map[string]Result{}
	var order []string
	for _, res := range r.Results {
		if res.Shape != shape {
			continue
		}
		if _, seen := byRoute[res.Route]; !seen {
			order = append(order, res.Route)
			byRoute[res.Route] = map[string]Result{}
		}
		byRoute[res.Route][res.Dialect] = res
	}
	sort.Strings(order)

	// The name column is measured rather than fixed. It was 26, which every
	// route fitted until one did not, and the row that overflowed pushed its
	// verdicts out of line with the column heading them — a table that is
	// wrong about which shell said what is worse than a wide one.
	nameCol := len("route")
	for _, name := range order {
		if len(name) > nameCol {
			nameCol = len(name)
		}
	}
	row := "  %-" + strconv.Itoa(nameCol) + "s"
	fmt.Fprintf(w, row+" %-10s %-10s %-10s %-10s\n", "route", "posix", "bash", "zsh", "ksh")
	fmt.Fprintf(w, "  %s\n", strings.Repeat("─", nameCol+4*11))
	for _, name := range order {
		fmt.Fprintf(w, row, name)
		for _, d := range AllDialects {
			res, ok := byRoute[name][d]
			if !ok {
				// The route does not exist in this shell, which is not the
				// same as being closed in it and must not read as a pass.
				fmt.Fprintf(w, " %-10s", "·")
				continue
			}
			fmt.Fprintf(w, " %-10s", mark(res.Verdict))
		}
		fmt.Fprintln(w)
	}

	n := r.CountsFor(shape)
	fmt.Fprintf(w, "\n  contained %d   ESCAPED %d   OVERBLOCKED %d   inert %d   documented %d\n",
		n[Contained], n[Escaped], n[Overblocked], n[Inert], n[Documented])
}

func mark(v Verdict) string {
	switch v {
	case Contained:
		return "ok"
	case Escaped:
		return "ESCAPED"
	case Overblocked:
		return "OVERBLOCK"
	case Inert:
		return "inert"
	case Documented:
		return "documented"
	}
	return "?"
}

func trim(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 90 {
		return s[:90] + "…"
	}
	return s
}
