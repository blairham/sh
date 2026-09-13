// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"sort"
	"strings"
)

// The other half of a disagreement is the lines we printed that the reference
// never asked for, and those are ours — so they can be reported.
//
// The report has said for a long time that our runtime diagnostics cannot be
// printed here because they would quote the file back, and that is true of a
// whole line: a diagnostic names the command, the word or the option that
// provoked it, and those come from a file nobody here may read. It is not
// true of our *catalogue*. "not implemented yet" is our sentence about our
// own shell and carries nothing of the file, and counting how many of our
// unmatched lines carry it ranks what this shell refused without reading
// anything.
//
// That is the same standard the static read's causes already meet — our own
// diagnostic with the file's words taken out — applied to the runtime half,
// which up to now has been a table of exit statuses and nothing else.

// Excuse is one phrase of our own diagnostics and how many lines of this
// column's output carry it, counted only in lines the reference never asked
// for.
type Excuse struct {
	Phrase string
	Lines  int
}

// Catalogue is the phrases, lowercase, and every one of them is ours.
//
// Deliberately a fixed list rather than a shape. A rule like "a line with two
// colons in it" would sweep up whatever a file printed and call it our
// diagnostic; a list of our own sentences cannot, because a line only counts
// when it carries a phrase this project wrote.
//
// Ordered by nothing — the report ranks by count.
var Catalogue = []string{
	"not implemented yet",
	"not implemented",
	"command not found",
	"invalid option",
	"unknown option",
	"invalid option name",
	"not a valid identifier",
	"bad substitution",
	"bad array subscript",
	"ambiguous redirect",
	"readonly variable",
	"unbound variable",
	"syntax error",
	"division by zero",
	"operand expected",
	"operator expected",
	"numeric argument required",
	"invalid number",
	"invalid symbolic mode",
	"too many arguments",
	"no such file or directory",
	"permission denied",
	"is a directory",
	"not a function",
	"no such job",
	"restricted",
	"usage:",
}

// excuses counts, among the lines we printed that the reference never asked
// for, how many carry each phrase of the catalogue.
//
// The unmatched side is a multiset difference for the reason [Doc.Attribute]
// gives one step over: a line the reference also printed is not a line it
// never asked for, wherever the two landed.
//
// A line carrying two phrases counts once, under the first that matches, so
// the column sums to lines rather than to mentions.
func excuses(mine, theirs []string) []int {
	counts := make([]int, len(Catalogue))
	want := map[string]int{}
	for _, line := range theirs {
		want[strings.TrimSpace(line)]++
	}
	for _, line := range mine {
		trimmed := strings.TrimSpace(line)
		if want[trimmed] > 0 {
			want[trimmed]--
			continue
		}
		lower := strings.ToLower(trimmed)
		for i, phrase := range Catalogue {
			if strings.Contains(lower, phrase) {
				counts[i]++
				break
			}
		}
	}
	return counts
}

// RankExcuses is the catalogue with the empty rows dropped, commonest first.
func RankExcuses(counts []int) []Excuse {
	var out []Excuse
	for i, n := range counts {
		if n > 0 {
			out = append(out, Excuse{Phrase: Catalogue[i], Lines: n})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Lines != out[j].Lines {
			return out[i].Lines > out[j].Lines
		}
		return out[i].Phrase < out[j].Phrase
	})
	return out
}
