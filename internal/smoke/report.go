// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package smoke

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Render turns a run into the table, and into the status it should exit with.
//
// It is here rather than in the command because the table is the deliverable —
// it is the thing a person reads to decide whether this shell is usable yet —
// and a deliverable assembled inside a main function is one nothing can check.
// The command is left with a flag set and a Print.
func Render(reports []Report, took time.Duration, verbose bool) (text string, status int) {
	var p page

	features := Features()
	width := len("FEATURE")
	for _, f := range features {
		if len(f) > width {
			width = len(f)
		}
	}

	p.printf("\nan interactive session, driven through a pseudo-terminal\n\n")
	p.printf("  %-*s", width, "FEATURE")
	for _, rep := range reports {
		p.printf("  %-*s", cellWidth, strings.ToUpper(rep.Dialect))
	}
	p.printf("\n  %s", strings.Repeat("-", width))
	for range reports {
		p.printf("  %s", strings.Repeat("-", cellWidth))
	}
	p.printf("\n")

	for _, f := range features {
		p.printf("  %-*s", width, f)
		for _, rep := range reports {
			p.printf("  %-*s", cellWidth, cell(rep, f))
		}
		p.printf("\n")
	}

	for _, rep := range reports {
		if rep.Err != nil {
			p.printf("\n%s: the session could not be driven at all: %v\n", rep.Dialect, rep.Err)
			if rep.Startup != "" {
				p.printf("  drawn before it gave up: %s\n", rep.Startup)
			}
			continue
		}
		lines := details(rep, verbose)
		if len(lines) == 0 && rep.Restarts == 0 {
			continue
		}
		p.printf("\n%s (%s)\n", rep.Dialect, rep.Binary)
		for _, l := range lines {
			p.printf("%s\n", l)
		}
		if rep.Restarts > 0 {
			// Worth printing rather than hiding: a suite that restarts on
			// every row is measuring its own recovery as much as the shell's
			// behavior, and a reader should be able to see that happening.
			p.printf("  the session had to be started again %d time(s), after a row left it unable to prompt\n",
				rep.Restarts)
		}
	}

	// The news, which is the reason the known list exists at all.
	for _, rep := range reports {
		for _, r := range rep.Fixed() {
			p.printf("\nFIXED: %q now passes for %s — %s no longer owns it\n",
				r.Feature, rep.Dialect, r.Known)
		}
		if rep.Unexpected() > 0 || rep.Err != nil {
			status = 1
		}
	}

	p.printf("\n")
	for _, rep := range reports {
		p.printf("  %-5s %d of %d, %d known-missing, %d unexplained\n",
			rep.Dialect, len(rep.Results)-rep.Failures(), len(rep.Results),
			rep.Failures()-rep.Unexpected(), rep.Unexpected())
	}
	p.printf("\n  %v\n\n", took.Round(time.Millisecond))
	return p.b.String(), status
}

// cellWidth is wide enough for the longest thing a cell says.
const cellWidth = 24

// cell is one dialect's answer for one feature.
func cell(rep Report, feature string) string {
	for _, r := range rep.Results {
		if r.Feature != feature {
			continue
		}
		switch {
		case r.Fixed():
			return "PASS  (" + r.Known + " FIXED)"
		case r.Outcome == Pass:
			return "PASS"
		case r.Known != "":
			return r.Outcome.String() + "  (known " + r.Known + ")"
		default:
			return r.Outcome.String() + "  (unexplained)"
		}
	}
	// A row the session never reached, because an earlier one ended it or
	// because the shell could not be started. Not blank: an empty cell reads
	// as a pass to anyone skimming.
	return "not reached"
}

// details is what a reader needs under the table: why each row that did not
// pass did not, and — with -v — how each row that did was satisfied.
func details(rep Report, verbose bool) []string {
	var lines []string
	for _, r := range rep.Results {
		if !verbose && r.Outcome == Pass && !r.Fixed() {
			continue
		}
		lines = append(lines, fmt.Sprintf("  %-8s %s: %s", r.Outcome, r.Feature, r.Detail))
	}
	return lines
}

// KnownList is the graded-against list, for a reader who wants it without
// reading the source.
func KnownList() string {
	var p page
	pairs := SortedKnown()
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i][1] < pairs[j][1] })
	for _, kv := range pairs {
		p.printf("  %-8s %s\n", kv[1], kv[0])
	}
	return p.b.String()
}

// page is a string being built.
//
// A strings.Builder cannot fail — its Write is on memory — so the error is
// discarded, once, here, rather than at every line of the table.
type page struct{ b strings.Builder }

func (p *page) printf(format string, a ...any) {
	_, _ = fmt.Fprintf(&p.b, format, a...)
}
