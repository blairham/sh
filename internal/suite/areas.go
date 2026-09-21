// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// The area set, and the cell space #2291's second leg is counted from.
//
// # Why the areas had to come into the tree before anything could count them
//
// Leg 2 of #2291 counts `(column, area)` cells, and the number it was counted
// from — 120 of 230 — reconstructs from nothing: the area set lived on the
// board as issues #2300–#2324 and the column set lived in [Ours], so no
// instrument could derive the denominator and nobody could say what a cell
// had been. #3481 is the issue that stopped guessing at it. The areas are
// here now, checked against the files rather than asserted, so the count is a
// derivation and the next reader can see what it is a count of.
//
// # Three cell spaces were on the table, and the measurement chose between them
//
//	(a) directory x basename      385 cells, and mostly unreachable by
//	                              construction — dash has no ext/ and never
//	                              will, so most of the space is a gap that
//	                              is not one
//	(b) dialect x area            125 cells, closed where the dialect runs
//	                              some file for that area
//	(c) column x area             125 cells, closed where a **gated** column
//	                              passes that area
//
// **(b) is saturated, and that is what ruled it out.** Measured on `main`
// 2026-09-19 and re-derived by [TestOptionBIsSaturated], which is committed
// so the finding cannot quietly stop being true: `core/` alone carries a file
// for 24 of the 25 areas, and the twenty-fifth — arrays — has a file in every
// one of the five dialect tiers, `dash/arrays.tests` and `ash/arrays.tests`
// included. So **125 of 125** cells are closed under (b) the day the mapping
// lands, the ledger this issue is about would hold nothing, and the staleness
// test would have nothing to be stale about.
//
// That is not a near miss. What (b) measures is *a file exists*, which is
// leg 2 asking leg 1's question badly: whether a column **passes** an area is
// where all the remaining work is, and (b) cannot see it.
//
// # So a cell is (c), and a column is one that is gated
//
// [Cells] is five columns by 25 areas. A cell is **closed** when the column's
// reference is pinned — both shells inside a digest-pinned image, so the
// figure is the same on a runner as on a laptop (#3480) — and the column runs
// at least one file for that area. It is **open** when the column runs a file
// for the area and is not gated. It is **ledgered** when no amount of correct
// work can close it, and then [UnclosableByConstruction] says why.
//
// **Five columns and not seven.** The earlier arithmetic on #3481 read
// `7 x 25 = 175` off the seven directories under `share/suite`, and two of
// those seven are tiers rather than columns: `core/` and `ext/` are run *by*
// the columns, never as one. What `make suite` grades is [Ours], which is one
// entry per dialect binary under `cmd/`, so the denominator is 125 — the same
// denominator (b) has, which is the useful part. The two options differ only
// in what "closed" means, so the saturation finding above is a statement
// about this same 125 cells rather than about a space that was discarded.
type Area struct {
	// Name is the area, as the roll-up prints it.
	Name string
	// Issue is the child of #2291 that owns it.
	Issue int
	// Files are the suite basenames, without the extension, that belong to
	// this area. Every basename under share/suite is claimed by exactly one
	// area or is named in [UnclaimedByShape], and [TestEveryBasenameIsClaimedOnce]
	// is what keeps that true.
	Files []string
}

// Areas is the 25 areas of #2291, each with the child issue that owns it.
//
// The names are the issue titles rather than words of this table's own, so
// that a reader can go from a cell to the issue holding its work without a
// second mapping to get wrong.
var Areas = []Area{
	{"quoting and tokenization", 2300, []string{"quoting", "tokenization", "dollarquote"}},
	{
		// `namespace` is here because it is a compound shape: a reserved
		// word, a name, and a brace group that takes redirections, standing
		// where any other command stands. What it *means* is name
		// resolution, which would put it beside `variables` — but the area
		// a file is filed under is the construct it is written in, and the
		// file is a grammar file (#3990).
		"command language and compound shapes", 2301,
		[]string{"grammar", "control", "cfor", "casefall", "select", "namespace"},
	},
	{"function definition, scope and return", 2302, []string{"functions", "funcform"}},
	{"IFS and field splitting", 2303, []string{"word-splitting"}},
	{
		"parameter expansion operators", 2304,
		[]string{
			"params", "expansion", "substrings", "patsub", "plusassign",
			"substitution-closer", "substitution-echo", "substitution-fatality",
			"substitution-line",
		},
	},
	{"arithmetic evaluation", 2305, []string{"arith"}},
	{"pattern matching and globbing", 2306, []string{"globbing", "patterns"}},
	{"command and process substitution", 2307, []string{"cmdsub", "procsub"}},
	{"redirection operators and file descriptors", 2308, []string{"redirect", "heredoc", "herestring"}},
	{"the special builtins", 2309, []string{"builtins-special"}},
	{"the regular builtins", 2310, []string{"builtins", "builtins-common"}},
	{"printf", 2311, []string{"printf"}},
	{"test, [ and [[ ]]", 2312, []string{"test", "conditions", "dblbracket"}},
	{"set options and shopt", 2313, []string{"setopts", "shell-options", "options"}},
	{"traps, signals and exit", 2314, []string{"signals"}},
	{"invocation and argv", 2315, []string{"invocation", "argv", "shelllevel"}},
	{"variables, declarations and scope", 2316, []string{"variables", "typeset", "namerefs"}},
	{"indexed and associative arrays", 2317, []string{"arrays"}},
	{"job control and background commands", 2318, []string{"jobs"}},
	{"aliases", 2319, []string{"aliases"}},
	{"command lookup order", 2320, []string{"lookup"}},
	{"getopts", 2321, []string{"getopts", "getopts-local-optind"}},
	{"eval, dot and source", 2322, []string{"evaldot"}},
	{"exit status and pipeline semantics", 2323, []string{"status"}},
	{"diagnostic wording and destination", 2324, []string{"diagnostics"}},
}

// Unclaimed is a suite file that belongs to no area, with the reason.
//
// It is the smaller of this package's two ledgers and it exists because two
// files resist the mapping *for a reason*, not because the mapping is short.
// An area is a construct or a builtin; these two are about a **tier boundary**
// and about an **axis set**, which are questions the area list does not have a
// row for and should not grow one for.
//
// Ledgered rather than dropped, on the same terms as everything else here: a
// cell space that cannot see two of the files somebody wrote is worth printing
// rather than rounding off, and [TestTheUnclaimedLedgerIsNotStale] fails the
// day one of them is claimed by an area after all or leaves the tree.
type Unclaimed struct {
	// File is the basename, without the extension.
	File string
	// Why is what the file is about instead of an area.
	Why string
}

// UnclaimedByShape is the ledger of files no area claims. See [Unclaimed].
var UnclaimedByShape = []Unclaimed{
	{
		File: "boundary",
		Why: "the ext/ boundary, measured from the shell's own side: it puts every " +
			"ext/ construct to dash and to BusyBox ash and records which refusal each " +
			"one gets. That is a claim about where the tier line falls, which is what " +
			"every area in the list is inside of — filing it under one of them would " +
			"make it a case about that construct, which is exactly what it is not.",
	},
	{
		File: "inherited",
		Why: "three answers BusyBox ash never gave, and so gave dash's. It is an axis " +
			"set rather than a construct — the file exists because dialect/ash " +
			"inherited PosixSemantics values nobody had measured against BusyBox — so " +
			"the area it would be filed under is whichever areas those three axes " +
			"happen to sit in this week.",
	},
}

// Basenames is every runnable file under root, by basename without the
// extension, and which tier directories hold it.
//
// Derived from the tree rather than from [Areas], which is the whole point:
// the check that the table describes the files is only worth having while the
// two are read from different places.
func Basenames(root string) (map[string][]string, error) {
	held := map[string][]string{}
	for _, dir := range dirsInPlay() {
		names, err := Files(filepath.Join(root, filepath.FromSlash(dir)), OurExt)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", dir, err)
		}
		for _, n := range names {
			base := strings.TrimSuffix(n, OurExt)
			held[base] = append(held[base], dir)
		}
	}
	return held, nil
}

// dirsInPlay is every directory a column runs, shared and per-dialect.
//
// Taken from [Tiers] and from each column's own [Suite.Dirs] rather than by
// listing the directories on disk, so a directory nothing runs is not counted
// as coverage. That is the same rule `make suite-guard` cross-checks itself
// against, and for the same reason: a walk of the tree and a walk of what the
// instrument runs are two different questions, and the interesting answers are
// where they differ.
func dirsInPlay() []string {
	seen := map[string]bool{}
	var dirs []string
	for _, t := range Tiers {
		if !seen[t] {
			seen[t] = true
			dirs = append(dirs, t)
		}
	}
	for _, s := range Ours {
		for _, d := range s.Dirs {
			if !seen[d] {
				seen[d] = true
				dirs = append(dirs, d)
			}
		}
	}
	sort.Strings(dirs)
	return dirs
}

// FindArea is the area a basename belongs to.
func FindArea(base string) (Area, bool) {
	for _, a := range Areas {
		for _, f := range a.Files {
			if f == base {
				return a, true
			}
		}
	}
	return Area{}, false
}
