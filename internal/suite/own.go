// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// Our own conformance suite, in the shape of a shell's own tests/ — and run
// by the machinery already in this package rather than beside it.
//
// # Why this exists at all
//
// The fetched columns can never be more than one. zsh's suite is `.ztst`, a
// format its own driver interprets rather than one a shell runs; the
// maintained ksh93 suite is ksh93u+m's while the only ksh93 on this machine
// is AT&T's 2012 build, so that run would grade the fork; dash ships no suite
// of its own; and there is no BusyBox ash on a Mac. Four of five dialects can
// never get a column from somebody else's work, so the only route to
// per-dialect parity at suite scale is a suite we write.
//
// # Why it is not a second grading mechanism
//
// It is the same one. A native column is a [Suite] value with [Suite.Ours]
// set and its files on disk instead of in an archive; [Sweep] grades it with
// the same [grade], the same [normalize], the same [repeats] and the same
// [agreement] the bash column is graded with. Nothing here computes a score.
// A second scorer that drifted from the first is the failure this repository
// has made more than once, and the way to not make it again is to have only
// one.
//
// The corpus under internal/oracle is a different instrument, not a rival to
// this one: it is one snippet, the whole panel, a golden row — a microscope
// for a disagreement somebody already found. A suite file is a hundred
// assertions run whole, and the finding is which file stops and why. Of the
// bash column's 83 files, 49 ran to status 0 under both shells and still
// disagreed; no corpus row asks the question any of those asked.
//
// # The suite ships no expected output
//
// The reference shell on the machine is the expectation, exactly as it is for
// the corpus. A checked-in `.right` file would let us record our own bug as
// correct, and generating expectations from ourselves is the one failure the
// oracle exists to prevent. That constrains a case to being deterministic —
// no pids, no clocks, no scheduling order — and [repeats] already refuses a
// file the reference will not reproduce.
//
// # Three tiers, and each is a claim this project makes
//
// A directory is not filing. Each is an assertion out of
// docs/spec/shell-matrix.md, and running it is what turns the assertion into
// a measurement.
//
//	core/    every column, every reference. What a script may assume anywhere.
//	ext/     the substrate's core language beyond POSIX — arrays, [[ ]],
//	         $'…', +=, substrings, C-style for. Every column but dash and
//	         ash, which are the measured holdouts.
//	<shell>/ the answer only that shell has.
//
// Only core/ is written today, and [Tiers] and each column's Dirs say so
// rather than claiming the other two and finding them empty. An empty tier
// would report a column that ran and agreed, which is the failure this
// instrument is arranged to make impossible.
//
// [CrossCheck] runs a tier through the *reference shells alone* and asks
// whether they all wrote the same bytes. That is the half no fetched suite
// can have: grading our binary against one reference proves that dialect
// right, while asking whether the references agree proves the construct
// common. If they split, the case does not belong in that tier and the
// instrument says which case, by name. Naming it is allowed here and nowhere
// else — see [Suite.attribute].

// OurRoot is where our own suite lives in the tree, relative to the module
// root. Committed, Apache-2.0, and readable, which is the whole difference
// between it and anything fetched.
const OurRoot = "share/suite"

// OurExt is the suffix of a file that gets run. It matches the shape a shell's
// own suite has: a file of assertions that a shell executes from the top.
const OurExt = ".tests"

// Ours is one column per dialect binary under cmd/.
//
// Five rows from the first commit, for the reason the fetched panel has four:
// a column that is not built has to be visible as a column rather than as a
// silence, because a missing column that says nothing reads as one that
// passed.
var Ours = []Suite{
	{
		Name:    "bash",
		Dialect: "bash",
		Ours:    true,
		Dirs:    []string{"core"},
		Ext:     OurExt,
		Lookup:  []string{"/opt/homebrew/bin/bash", "/usr/local/bin/bash", "/bin/bash", "/usr/bin/bash"},
	},
	{
		Name:    "zsh",
		Dialect: "zsh",
		Ours:    true,
		Dirs:    []string{"core"},
		Ext:     OurExt,
		Lookup:  []string{"/opt/homebrew/bin/zsh", "/usr/local/bin/zsh", "/bin/zsh", "/usr/bin/zsh"},
	},
	{
		Name:    "ksh93",
		Dialect: "ksh",
		Ours:    true,
		Dirs:    []string{"core"},
		Ext:     OurExt,
		Lookup:  []string{"/bin/ksh", "/usr/bin/ksh", "/opt/homebrew/bin/ksh93"},
	},
	{
		// The column the fetched panel can never have: dash ships no suite,
		// so the only questions anyone will ever ask dash about are ours.
		Name:    "dash",
		Dialect: "dash",
		Ours:    true,
		Dirs:    []string{"core"},
		Ext:     OurExt,
		Lookup:  []string{"/bin/dash", "/usr/bin/dash", "/opt/homebrew/bin/dash"},
	},
	{
		Name:    "ash",
		Dialect: "ash",
		Ours:    true,
		Dirs:    []string{"core"},
		Ext:     OurExt,
		Lookup:  []string{"/bin/busybox", "/usr/bin/busybox", "/opt/homebrew/bin/busybox"},
		NotYet: "there is no BusyBox on a stock macOS machine, so cmd/ash has no reference " +
			"to be graded against — writing the cases changes nothing until one exists, " +
			"in a container or on a Linux runner",
	},
}

// OurColumns is the native columns, built and unbuilt.
func OurColumns() []Suite { return Ours }

// FindOurs is the native column for a dialect name.
func FindOurs(dialect string) (Suite, bool) {
	for _, s := range Ours {
		if s.Dialect == dialect {
			return s, true
		}
	}
	return Suite{}, false
}

// Tiers are the shared directories, in the order of how much they assume.
//
// Each is cross-checked against the references that are supposed to agree
// about it: every column for core/, everything but dash and ash for ext/.
//
// Only core/ is written today. ext/ and the per-dialect directories are the
// rest of the campaign and arrive with cases rather than with an empty
// directory — a directory with no files in it would report a column that ran
// and agreed, which is the mistake this whole instrument is arranged to
// avoid. [Suite.Missing] is what makes the omission structural: a column may
// not claim a directory that is not there.
var Tiers = []string{"core"}

// tier is the suite a cross-check run uses.
//
// It is a [Suite] rather than a bare path because [CrossCheck] runs those
// files through the reference shells with the same environment and the same
// per-run directory a graded run gets. A cross-check run on different terms
// from the graded run would be measuring something else.
func tier(name string) Suite {
	return Suite{Name: name, Dialect: "sh", Ours: true, Dirs: []string{name}, Ext: OurExt}
}

// CrossShells is the columns whose references are expected to agree about a
// tier: every one for core/, and for ext/ every one whose Dirs claim it.
func CrossShells(name string, refs []Reference) []Reference {
	if name == "core" {
		return refs
	}
	var kept []Reference
	for _, r := range refs {
		s, ok := findOursByName(r.Name)
		if !ok {
			continue
		}
		for _, d := range s.Dirs {
			if d == name {
				kept = append(kept, r)
				break
			}
		}
	}
	return kept
}

func findOursByName(name string) (Suite, bool) {
	for _, s := range Ours {
		if s.Name == name {
			return s, true
		}
	}
	return Suite{}, false
}

// attribute is the one place a file's name may enter a report, and it is what
// scopes the no-path rule to fetched suites.
//
// The rule itself does not change: another project's suite is its expression,
// CLEANROOM.md's red list covers it, and a path in a report is an invitation
// to go look. That reasoning is about *their* files. Ours are committed,
// Apache-2.0 and meant to be opened — a report that would not say which of
// our own cases failed is not a work list, and the campaign this feeds needs
// a work list.
//
// So the difference is built rather than remembered: [Result] still has
// nowhere to put a name, the name lives on [NamedResult], and the only
// function that fills one in returns the empty string for every fetched
// column. TestAFetchedColumnCannotNameAFile is the guard.
func (s Suite) attribute(name string) string {
	if !s.Ours {
		return ""
	}
	return name
}

// NamedResult is one file's outcome, with its name where naming it is allowed.
type NamedResult struct {
	// Name is the file, and it is empty for every fetched column. See
	// [Suite.attribute].
	Name   string
	Result Result
}

// Missing returns the directories a native column claims and does not have.
//
// A column whose dialect directory is absent would otherwise run only core/
// and report a healthy number for half a suite, which is the same mistake as
// a silent skip one level down.
func (s Suite) Missing(root string) []string {
	var missing []string
	for _, dir := range s.Dirs {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir))); err != nil {
			missing = append(missing, dir)
		}
	}
	return missing
}

// CrossSplit is one core file the reference shells did not answer alike, and
// the groups they split into.
//
// Each group is the names of the shells that wrote the same bytes. Two groups
// is the common shape — one shell is the odd one out — and the case then
// belongs in a dialect directory rather than in core/.
type CrossSplit struct {
	Name   string
	Groups [][]string
}

// Cross is one tier's claim, measured.
type Cross struct {
	// Tier is the directory the claim is about.
	Tier string
	// Shells are the references the check had, by name.
	Shells []string
	// Files is how many core files were asked, Agree how many every
	// reference answered identically.
	Files, Agree int
	// Split is the rest, by name. Naming them is the point: an unsplit
	// report says a case is not core without saying which.
	Split []CrossSplit
	// Unstable is a core file a reference would not reproduce, which says
	// nothing about whether the construct is core.
	Unstable []string
}

// Reference is a reference shell for the cross-check: a name to print and a
// binary to run.
type Reference struct {
	Name string
	Path string
}

// CrossCheck runs a tier's files under the reference shells that are supposed
// to agree about it, and reports the files they did not all answer alike.
//
// This is the half of the instrument the fetched columns cannot have. Grading
// our binary against one reference proves that dialect right; asking whether
// the references agree proves the *construct* common, which is the claim
// docs/spec/shell-matrix.md makes in prose and nothing until now has checked.
func CrossCheck(ctx context.Context, root, name string, refs []Reference, opts Options) (Cross, error) {
	cross := Cross{Tier: name}
	refs = CrossShells(name, refs)
	for _, r := range refs {
		cross.Shells = append(cross.Shells, r.Name)
	}
	dir := filepath.Join(root, name)
	names, err := Files(dir, OurExt)
	if err != nil {
		return cross, err
	}
	if opts.Only != nil {
		names = keep(names, opts.Only)
	}
	cross.Files = len(names)
	suite := tier(name)
	for _, name := range names {
		split, unstable := crossOne(ctx, suite, dir, name, refs, opts.timeout())
		switch {
		case unstable:
			cross.Unstable = append(cross.Unstable, name)
		case len(split.Groups) <= 1:
			cross.Agree++
		default:
			cross.Split = append(cross.Split, split)
		}
	}
	return cross, nil
}

// crossOne groups the references by what they wrote.
func crossOne(ctx context.Context, s Suite, dir, name string, refs []Reference, timeout time.Duration) (CrossSplit, bool) {
	split := CrossSplit{Name: name}
	groups := map[string][]string{}
	var order []string
	for _, r := range refs {
		out := runIn(ctx, s, dir, name, r.Path, timeout)
		if out.TimedOut {
			return split, true
		}
		key := keyOf(normalize(out.Output, r.Path, out.Dir), out.Status)
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], r.Name)
	}
	if len(order) > 1 {
		// Only where they differed, and only then: re-running every core
		// file under every shell to prove a case nobody disputes is four
		// runs bought for nothing.
		for _, r := range refs {
			again := runIn(ctx, s, dir, name, r.Path, timeout)
			if again.TimedOut {
				return split, true
			}
			key := keyOf(normalize(again.Output, r.Path, again.Dir), again.Status)
			if !contains(groups[key], r.Name) {
				return split, true
			}
		}
	}
	for _, key := range order {
		split.Groups = append(split.Groups, groups[key])
	}
	return split, false
}

func keyOf(out string, status int) string {
	return out + "\x00" + strconv.Itoa(status)
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func keep(names []string, only map[string]bool) []string {
	var kept []string
	for _, n := range names {
		if only[n] {
			kept = append(kept, n)
		}
	}
	sort.Strings(kept)
	return kept
}
