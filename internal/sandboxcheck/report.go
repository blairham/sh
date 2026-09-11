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
	fmt.Fprintf(w, "sandbox: %d routes × dialects, against %s\n\n", len(Routes()), r.Shell)

	byRoute := map[string]map[string]Result{}
	var order []string
	for _, res := range r.Results {
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

	n := r.Counts()
	fmt.Fprintf(w, "\n  contained %d   ESCAPED %d   OVERBLOCKED %d   inert %d\n",
		n[Contained], n[Escaped], n[Overblocked], n[Inert])

	// The inert ledger. Printed always rather than under -v, because these
	// are the rows that become escapes the day the feature lands, and a list
	// nobody sees is not a ledger.
	var inert []string
	for _, res := range r.Results {
		if res.Verdict == Inert {
			inert = append(inert, res.Route+"/"+res.Dialect)
		}
	}
	if len(inert) > 0 {
		fmt.Fprintf(w, "\n  not reachable yet — each becomes an escape the day it works:\n")
		for _, s := range inert {
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
		fmt.Fprintf(w, "\n  %s (%s): %s\n", res.Route, res.Dialect, res.Verdict)
		for i, label := range []string{"ungated", "denied ", "allowed"} {
			o := res.Runs[i]
			fmt.Fprintf(w, "    %s  code=%d out=%q err=%q\n",
				label, o.Code, trim(o.Out), trim(o.Err))
		}
	}
	return b.String()
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
