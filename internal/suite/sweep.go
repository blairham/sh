// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/blairham/sh/internal/wild"
	"github.com/blairham/sh/syntax"
)

// Result is one file's outcome, and it is deliberately impossible to say
// which file it was.
//
// There is no path field and no text field, and that is the CLEANROOM rule
// made structural rather than remembered. A path in a report is an invitation
// to go look, and these are files nobody in this project may read; `make
// wild` already refuses considerably less than that and prints its reason.
// Everything here is either a number or our own parser's diagnostic with the
// per-file words taken out.
type Result struct {
	// Parsed says a static read of the whole file, in the dialect's
	// defaults, succeeded. That is a route the shell itself never takes —
	// it parses incrementally — so this decides nothing about whether the
	// file ran or how it scored, and the fields below are filled in either
	// way.
	Parsed bool
	// Cause is our own parser's reason for refusing it, with the position
	// and any word from the file removed. Empty when Parsed.
	Cause string
	// ReferenceRead says the reference shell's own static read — the POSIX
	// `-n` — accepted the file. It is asked only where this parser refused
	// one, and it is what tells a gap in this parser from a file no static
	// read can reach: an option set at run time decides what a later line
	// means, and a static read has no run time. False when Parsed, where
	// the question was never put.
	ReferenceRead bool
	// Scored says both shells finished and the reference repeated itself, so
	// this file is evidence.
	Scored bool
	// Strict says the two runs were identical, output and status.
	Strict bool
	// Common and Longest are the line agreement: a longest common
	// subsequence and the length of the longer output.
	Common, Longest int
	// LineCapped says the outputs were longer than the comparison's bound.
	LineCapped bool
	// OurStatus and RefStatus are the exit statuses, and are meaningful only
	// when Scored.
	OurStatus, RefStatus int
	// OracleHung and DialectHung are the two ways a file is reported and
	// never scored. They are separate because they are different findings: a
	// killed dialect binary is a hang we published, and a killed reference is
	// the harness's fault — the shell that wrote the file does not hang on it.
	OracleHung, DialectHung bool
	// OracleFailed and DialectFailed say the shell never started at all: the
	// binary is missing, is not executable, or the harness could not lay out
	// a directory to run it in. Neither is a finding about a shell, and both
	// are kept out of the score for the same reason a hung run is — a run
	// that did not happen agreed with nothing and disagreed with nothing.
	// The pair is kept apart from the hung pair because the cause is
	// different: a hang is something the shell did, a failure to start is
	// something the harness or the machine did before the shell was reached.
	OracleFailed, DialectFailed bool
	// Unstable says the reference did not produce the same run twice, so the
	// file says nothing about either shell.
	Unstable bool
	// OurLines and RefLines are how many lines each run wrote. They are what
	// tells the two halves of a disagreement apart: the reference's lines we
	// never produced is RefLines-Common, ours it never asked for is
	// OurLines-Common, and the single differing-line figure is the larger of
	// the two. A file where we print more than the reference is one whose
	// count is driven by our own excess, which is a different kind of work
	// from a missing answer and used to be invisible.
	OurLines, RefLines int
	// Excuses counts our own catalog phrases in the lines we printed that
	// the reference never asked for, parallel to [Catalog]. It is what
	// ranks the runtime half of a disagreement, which up to now was a table
	// of exit statuses and nothing else.
	Excuses []int
	// Prose is how many of the differing lines are the reference printing
	// its own documentation — see [Doc]. Those are not work: matching them
	// means copying the text. It is a lower bound and it is never more than
	// Longest-Common.
	Prose int
	// Reordered is how many of the differing lines go away when order is
	// ignored — see reorder.go. An associative array has no order a script
	// can ask for, and the reference lists its keys in its own hash order,
	// which is in nothing but the source CLEANROOM.md's red list covers.
	//
	// An **upper** bound where Prose is a lower one, and for a different
	// reason: this is a per-side canonicalisation, so a line whose order is
	// genuinely ours to get right is inside it too. It corrects neither of
	// the other figures and nothing subtracts it.
	Reordered int
}

// Cause is one reason our parser refused a file, and how many files it
// stopped. There is no example and no list of files: the count and our own
// wording are the whole of it.
type Cause struct {
	Reason string
	Files  int
	// ReferenceRefuses is how many of those files the reference shell's own
	// `-n` refuses as well. Those are not a gap in this parser — no static
	// read reaches them — and a ranking that did not separate them would
	// send somebody to close a gap nobody can close.
	ReferenceRefuses int
}

// Band is the scored aggregate over one subset of the files.
//
// There is one for the whole population, spelled out on the [Report], and one
// for the files the static read refused. The second exists because "what did
// a refusal cost" is a question this instrument can answer from its own run,
// and it used to be answered in prose instead — wrongly, with a claim that a
// refusal forfeited a whole file.
type Band struct {
	// Files is how many files are in the band.
	Files int
	// Scored, Strict, Common and Longest are the same three numbers the
	// report carries for everything, restricted to the band.
	Scored, Strict  int
	Common, Longest int64
}

// StrictRate and LineRate are the band's two rates.
func (b Band) StrictRate() float64 { return ratio(b.Strict, b.Scored) }
func (b Band) LineRate() float64   { return ratio64(b.Common, b.Longest) }

// StatusPair is a pair of exit statuses and how many files ended that way.
// Numbers carry nothing of the file, which is why the runtime half of this
// report is statuses rather than diagnostics — ours would quote the file's
// own words back.
type StatusPair struct {
	Ours, Reference int
	Files           int
}

// Report is one column's run.
type Report struct {
	Suite     Suite
	Reference string
	// ReferenceVersion is the build the reference shell reported, and
	// whether it reported one at all. A struct rather than a string because
	// a probe that failed used to be indistinguishable from one that
	// answered — see [Build] and #3135.
	ReferenceVersion Build
	Ours             string
	Helpers          []string
	// Route is how the column was reached, and it is empty for a binary on
	// this machine. A column measured inside a container is a weaker and a
	// differently-scoped claim than one measured here — it pins the behavior
	// of an image rather than of this machine — so the report says so
	// instead of letting the two look alike.
	Route string

	Files  int
	Parsed int
	Scored int
	Strict int

	Common  int64
	Longest int64
	// MeanFile is the unweighted mean of the per-file agreement, kept beside
	// the weighted figure because they answer different questions: one file
	// printing ten thousand lines otherwise decides the corpus.
	MeanFile float64
	// LineCapped counts files whose outputs were longer than the comparison's
	// bound and were compared on their first lines.
	LineCapped int
	// Missing and Excess are the two halves of the disagreement, summed: the
	// reference's lines we never printed, and the lines we printed that it
	// never asked for. The differing-line figure this burndown tracks is
	// Longest-Common, which is the larger of the two per file — so a file
	// whose count is driven by Excess is one where we are printing too much
	// rather than answering too little, and those are different work.
	Missing, Excess int64
	// Excuses is our own diagnostics over the whole column, ranked: what this
	// shell said and the reference did not.
	Excuses []Excuse
	// Prose is the sum of [Result.Prose]: how many of this column's
	// differing lines are the reference quoting its own documentation, and
	// so are not available to be written here at all. Zero for a column with
	// no [Suite.SelfDoc], where the question was never put.
	Prose int64
	// Reordered is the sum of [Result.Reordered]: how many of this column's
	// differing lines go away when order is ignored. See reorder.go — it is
	// an upper bound on the floor an associative array's hash order puts
	// under this column, and it corrects nothing.
	Reordered int64

	OracleHung    int
	DialectHung   int
	OracleFailed  int
	DialectFailed int
	Unstable      int

	// Refused is the scored aggregate over the files the static read
	// refused, and it is the measurement that replaced a sentence. A
	// refusal forfeits nothing here: the file is run and scored like any
	// other, and this says what the refusals actually cost.
	Refused Band
	// ReferenceRefuses is how many of the refused files the reference
	// shell's own `-n` refuses too.
	ReferenceRefuses int

	Causes      []Cause
	StatusPairs []StatusPair

	// Cases is every file's result, in the order they were run. The name is
	// filled in only for our own suite — see [Suite.attribute] — so a
	// fetched column's cases are a list of anonymous outcomes and a native
	// column's is a work list.
	Cases []NamedResult
}

// NotStrict is the cases that ran and disagreed, named.
//
// Empty for a fetched column whatever happened, because there is nothing
// there to name. That is the rule doing its job rather than a shortfall.
func (r Report) NotStrict() []NamedResult {
	var out []NamedResult
	for _, c := range r.Cases {
		if c.Name == "" || c.Result.Strict {
			continue
		}
		out = append(out, c)
	}
	return out
}

// CaseDefects is the files of our own suite the reference would not
// reproduce, named.
//
// The same measurement as [Result.Unstable] read the other way round, and the
// difference is the whole of #2297. A fetched suite is somebody else's work
// and nobody here may edit it, so a file its own shell will not repeat is
// evidence about neither shell and dropping it from the scored set is the
// only honest thing available — the denominator moves and the report says by
// how much.
//
// Our own suite ships no expected output, so the reference on this machine
// *is* the expectation. A case the reference answers two different ways has
// no expectation at all, and it is therefore not an unlucky file but a bug in
// the case: a pid, a clock, a scheduling order, a path that escaped
// [normalize]. It is ours, so we can fix it, and a run that met one has to
// fail rather than quietly shrink its denominator.
//
// Empty for a fetched column whatever happened, because [Suite.attribute]
// leaves a fetched case unnamed and there is nothing here to report.
func (r Report) CaseDefects() []string {
	if !r.Suite.Ours {
		return nil
	}
	var out []string
	for _, c := range r.Cases {
		if c.Name == "" || !c.Result.Unstable {
			continue
		}
		out = append(out, c.Name)
	}
	return out
}

// CaseDefectReports is the same files with what actually moved, one line each.
//
// The names alone were what this had, and they are not enough to act on: a
// suite file is hundreds of lines and "it is not deterministic" sends a
// reader to read all of them. The lines come from [difference] and are our
// own file's own output, under the carve-out that already lets the name be
// printed.
func (r Report) CaseDefectReports() []string {
	if !r.Suite.Ours {
		return nil
	}
	var out []string
	for _, c := range r.Cases {
		if c.Name == "" || !c.Result.Unstable {
			continue
		}
		if c.Moved == "" {
			out = append(out, c.Name+" — what moved was not recorded")
			continue
		}
		out = append(out, c.Name+" — "+c.Moved)
	}
	return out
}

// ProseAsked says this column has a [Suite.SelfDoc], so [Report.Prose] is a
// measurement rather than a question nobody put. Zero means two different
// things without it, and the report may not print them alike.
func (r Report) ProseAsked() bool { return r.Suite.SelfDoc != "" }

// StrictRate, ParseRate and LineRate are the three numbers, as fractions.
func (r Report) StrictRate() float64 { return ratio(r.Strict, r.Scored) }
func (r Report) ParseRate() float64  { return ratio(r.Parsed, r.Files) }
func (r Report) LineRate() float64   { return ratio64(r.Common, r.Longest) }

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func ratio64(a, b int64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// Options are the knobs a run has.
type Options struct {
	// Timeout is how long one file gets under one shell.
	Timeout time.Duration
	// Jobs is how many files run at once. Bounded by default for the reason
	// the Makefile bounds `go test`: several instruments are often running on
	// this machine at once, and a sweep that took every core would be
	// measuring a loaded machine.
	Jobs int
	// Only, when set, restricts the run to files whose name is in it. For
	// developing the harness; a name here comes from a person, never from
	// the report.
	Only map[string]bool
	// Extra is environment added to *both* runs, and it is the attribution
	// instrument: naming a startup file that defines a builtin away, in both
	// columns at once, turns "how much of this file is that builtin" from a
	// suspicion into a line count. Both columns, because a neutralization
	// applied to one of them measures the neutralization.
	//
	// It reads nothing and prints nothing — the file it names is ours, and
	// what comes back is the same differing-line count as any other run.
	Extra []string
}

func (o Options) timeout() time.Duration {
	if o.Timeout <= 0 {
		return DefaultTimeout
	}
	return o.Timeout
}

func (o Options) jobs() int {
	if o.Jobs <= 0 {
		return 4
	}
	return o.Jobs
}

// Sweep runs every file of the suite through both shells and grades one
// against the other.
//
// reference is the oracle and it is a binary on this machine, never the
// suite's own expected output. Those files were generated by a different
// build of a different version and disagree with the shell installed here;
// they are also the most expression-like part of a suite, which is why this
// instrument never unpacks them.
func Sweep(ctx context.Context, s Suite, dir, ours, reference string, opts Options) (Report, error) {
	rep := Report{Suite: s, Reference: reference, Ours: ours}
	files, err := plan(s, dir, opts)
	if err != nil {
		return rep, err
	}
	rep.Files = len(files)

	dial, haveDialect := s.Syntax()
	// Asked once, before the runs: the dictionary is a property of the
	// reference binary and does not change between files.
	doc := SelfDocumentation(ctx, s, reference)
	results := make([]Result, len(files))
	moved := make([]string, len(files))
	sem := make(chan struct{}, opts.jobs())
	var wg sync.WaitGroup
	for i, f := range files {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i], moved[i] = grade(ctx, s, f.Dir, f.Name, ours, reference, dial, haveDialect, doc, opts)
		}()
	}
	wg.Wait()

	causes := map[string]int{}
	shared := map[string]int{}
	excused := make([]int, len(Catalog))
	statuses := map[[2]int]int{}
	var meanSum float64
	for i, res := range results {
		rep.Cases = append(rep.Cases, NamedResult{Name: s.attribute(files[i].Name), Result: res, Moved: moved[i]})
		switch {
		case res.Parsed:
			rep.Parsed++
		default:
			rep.Refused.Files++
			causes[res.Cause]++
			if !res.ReferenceRead {
				shared[res.Cause]++
				rep.ReferenceRefuses++
			}
		}
		switch {
		case res.OracleFailed:
			rep.OracleFailed++
		case res.DialectFailed:
			rep.DialectFailed++
		case res.OracleHung:
			rep.OracleHung++
		case res.DialectHung:
			rep.DialectHung++
		case res.Unstable:
			rep.Unstable++
		case res.Scored:
			rep.Scored++
			if res.Strict {
				rep.Strict++
			}
			rep.Common += int64(res.Common)
			rep.Longest += int64(res.Longest)
			if !res.Parsed {
				// The refused files, scored as a set of their own. They are
				// in the figures above as well, because they are evidence
				// like any other file; this is the size of what a refusal
				// costs, which is the thing the report used to assert.
				rep.Refused.Scored++
				if res.Strict {
					rep.Refused.Strict++
				}
				rep.Refused.Common += int64(res.Common)
				rep.Refused.Longest += int64(res.Longest)
			}
			rep.Prose += int64(res.Prose)
			rep.Reordered += int64(res.Reordered)
			for phrase, n := range res.Excuses {
				excused[phrase] += n
			}
			rep.Missing += int64(res.RefLines - res.Common)
			rep.Excess += int64(res.OurLines - res.Common)
			meanSum += ratio(res.Common, res.Longest)
			if res.LineCapped {
				rep.LineCapped++
			}
			if !res.Strict {
				statuses[[2]int{res.OurStatus, res.RefStatus}]++
			}
		}
	}
	rep.MeanFile = 0
	if rep.Scored > 0 {
		rep.MeanFile = meanSum / float64(rep.Scored)
	}
	rep.Causes = rank(causes, shared)
	rep.Excuses = RankExcuses(excused)
	rep.StatusPairs = rankStatuses(statuses)
	return rep, nil
}

// file is one runnable case: the directory it is run from and its name.
//
// The directory travels with the name because our own suite has two of them —
// the shared core/ and the dialect's own — and a run copies the directory it
// came from. A fetched suite has one and reaches here the same way.
type file struct {
	Dir, Name string
}

// plan is every file a column runs, in a stable order.
//
// A directory a column claims and does not have is an error rather than an
// omission: a column that quietly ran core/ alone would report a healthy
// number for half a suite, which is the same mistake as a silent skip one
// level down.
func plan(s Suite, dir string, opts Options) ([]file, error) {
	dirs := s.Dirs
	if len(dirs) == 0 {
		dirs = []string{s.TestDir}
	}
	var files []file
	for _, d := range dirs {
		tests := filepath.Join(dir, filepath.FromSlash(d))
		names, err := Files(tests, s.Ext)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if opts.Only != nil && !opts.Only[n] {
				continue
			}
			files = append(files, file{Dir: tests, Name: n})
		}
	}
	return files, nil
}

// grade is one file, both ways.
//
// The second return is how the reference disagreed with itself, and it is
// empty for every column but ours — see [difference], which is where that
// carve-out is made rather than here.
func grade(ctx context.Context, s Suite, tests, name, ours, reference string, dial syntax.Dialect, haveDialect bool, doc Doc, opts Options) (Result, string) {
	var res Result
	src, err := os.ReadFile(filepath.Join(tests, name))
	if err != nil {
		res.Cause = "unreadable"
		return res, ""
	}
	if !haveDialect {
		res.Cause = "no dialect"
	} else if _, perr := syntax.Parse(string(src), dial); perr != nil {
		// wild.Reason is our lexer's own words about an input with the
		// per-file detail taken out: the position goes, and so does an
		// ordinary word, while an operator or a reserved word stays because
		// those come from a vocabulary the grammar knows. That is what makes
		// a cause reportable here — it is our expression about the file, not
		// the file's.
		res.Cause = wild.Reason(perr)
	} else {
		res.Parsed = true
	}
	if haveDialect && !res.Parsed {
		// Only where this parser refused, which is the only place the answer
		// changes anything — and it is a read rather than a run, so it costs
		// a fraction of the two runs below.
		res.ReferenceRead = staticParse(ctx, reference, filepath.Join(tests, name), staticTimeout)
	}

	ref := runIn(ctx, s, tests, name, reference, opts)
	switch {
	case ref.Failed:
		res.OracleFailed = true
		return res, ""
	case ref.TimedOut:
		res.OracleHung = true
		return res, ""
	}
	own := runIn(ctx, s, tests, name, ours, opts)
	switch {
	case own.Failed:
		res.DialectFailed = true
		return res, ""
	case own.TimedOut:
		res.DialectHung = true
		return res, ""
	}

	res.OurStatus, res.RefStatus = own.Status, ref.Status
	identical := own.Output == ref.Output && own.Status == ref.Status
	theirs := normalize(ref.Output, reference, ref.Dir)
	mine := normalize(own.Output, ours, own.Dir)
	agreed := identical || (mine == theirs && own.Status == ref.Status)

	if !agreed || s.mustRepeat() {
		steady, moved := repeats(ctx, s, tests, name, reference, opts, theirs, ref.Status)
		if !steady {
			// The reference does not produce the same run twice, so the two
			// shells were never going to agree and this file is evidence
			// about neither. A process id, a clock reading, a scheduling
			// order: the reference disagrees with itself on these, and
			// counting them against us would put a number on the machine.
			//
			// In a suite of ours it means the opposite thing, and
			// [Suite.mustRepeat] is why the question was even asked here.
			res.Unstable = true
			return res, moved
		}
	}
	if agreed {
		// Identical before normalization means there was nothing for
		// normalization to decide: the two shells wrote the same bytes.
		text := mine
		if identical {
			text = own.Output
		}
		res.Scored, res.Strict = true, true
		l := lines(text)
		res.Common, res.Longest = len(l), len(l)
		res.OurLines, res.RefLines = len(l), len(l)
		return res, ""
	}
	res.Scored = true
	ourLines, refLines := lines(mine), lines(theirs)
	res.OurLines, res.RefLines = len(ourLines), len(refLines)
	res.Common, res.Longest, res.LineCapped = agreement(ourLines, refLines)
	res.Prose = min(doc.Attribute(ourLines, refLines), res.Longest-res.Common)
	res.Reordered = reordered(ourLines, refLines, res.Longest-res.Common)
	res.Excuses = excuses(ourLines, refLines)
	return res, ""
}

// placed is a run and the directory it was given, which normalization needs
// because the two runs get different ones.
type placed struct {
	Outcome
	Dir string
}

// runIn copies the suite into a directory of its own and runs one file there.
//
// A copy per run, because a suite file writes: it makes fixtures, it moves
// them, and several of them delete what they made only if they got that far.
// Running twice in one directory would have the second run reading the first
// one's leftovers, and the fetched tree would stop being what was unpacked.
func runIn(ctx context.Context, s Suite, tests, name, shell string, opts Options) placed {
	dir, err := os.MkdirTemp("", "suite")
	if err != nil {
		return placed{Outcome: Outcome{Output: err.Error(), Status: -1, Failed: true}}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	run := filepath.Join(dir, "t")
	if err := copyTree(tests, run); err != nil {
		return placed{Outcome: Outcome{Output: err.Error(), Status: -1, Failed: true}, Dir: run}
	}
	out := runFile(ctx, shell, run, name, append(environ(s, run, shell), opts.Extra...), opts.timeout())
	return placed{Outcome: out, Dir: run}
}

// repeats asks the reference for the same file again.
//
// For a fetched column, only where the two shells differed: that is the one
// place the answer changes anything. For a suite of ours, every file — see
// [Suite.mustRepeat].
func repeats(ctx context.Context, s Suite, tests, name, reference string, opts Options,
	want string, status int,
) (steady bool, moved string) {
	again := runIn(ctx, s, tests, name, reference, opts)
	if again.TimedOut {
		return false, "the second run of the reference was killed on the timeout"
	}
	got := normalize(again.Output, reference, again.Dir)
	// Steady up to an order neither run was asked for. One shell disagreeing
	// with *itself* about the sequence of an associative array's keys is not
	// a fact about either shell, and it is what made `assoc.tests` report
	// unstable — so the file scored nothing at all on about half of all runs,
	// over three lines out of some two hundred. See reorder.go for why the
	// same canonicalisation is sound here and is deliberately not applied to
	// the comparison against our own run.
	if again.Status == status && sameButForOrder(got, want) {
		return true, ""
	}
	return false, difference(s, want, status, got, again.Status)
}

// movedLines is how many of the differing lines are reported.
//
// A bound rather than the whole of it, because a file that loses its footing
// early disagrees with itself for the rest of its length and the first few
// lines are the ones that say where.
const movedLines = 6

// difference is what the reference did differently the second time, in words,
// and it is empty for every column but ours.
//
// The carve-out is exactly [Suite.attribute]'s and is made for the same
// reason. Another project's suite is its expression and a report may not
// quote it; ours are committed, Apache-2.0 and meant to be opened — and
// "jobs.tests is not deterministic" without the line is not a work list.
// #2291's assessment named that file off a CI log and nothing in the tree
// could say which of its two hundred lines had moved; the flake shows on
// about two runs in fourteen, on a machine nobody can log into, so it is only
// ever fixed by the run that catches it saying what it caught.
func difference(s Suite, want string, status int, got string, gotStatus int) string {
	if !s.Ours {
		return ""
	}
	var parts []string
	if status != gotStatus {
		parts = append(parts, fmt.Sprintf("status %d the first time and %d the second", status, gotStatus))
	}
	first, second := lines(want), lines(got)
	if gone := onlyIn(first, second); len(gone) > 0 {
		parts = append(parts, "first run only: "+strings.Join(atMost(gone), " · "))
	}
	if came := onlyIn(second, first); len(came) > 0 {
		parts = append(parts, "second run only: "+strings.Join(atMost(came), " · "))
	}
	if len(parts) == 0 {
		// The same lines in a different order, which a difference of
		// multisets cannot see and which is a defect of the same kind.
		parts = append(parts, "the same lines in a different order")
	}
	return strings.Join(parts, "; ")
}

// onlyIn is the lines of a that b does not have, counted rather than set-wise
// so that a line printed twice in one run and once in the other is reported.
func onlyIn(a, b []string) []string {
	left := map[string]int{}
	for _, l := range b {
		left[l]++
	}
	var out []string
	for _, l := range a {
		if left[l] > 0 {
			left[l]--
			continue
		}
		out = append(out, l)
	}
	return out
}

// atMost bounds a list for a report and says how much it left out.
func atMost(ls []string) []string {
	if len(ls) <= movedLines {
		return ls
	}
	return append(append([]string{}, ls[:movedLines]...),
		fmt.Sprintf("(and %d more)", len(ls)-movedLines))
}

// Files is the suite's runnable files, sorted.
//
// Only the ones carrying the suite's extension. The rest of the directory is
// fixtures and pieces the files source, which are needed on disk and are not
// runs of their own; running one would report a failure for a file nobody
// meant to start.
//
// The test directory's own files and not its subdirectories, for the same
// reason one step out: a subdirectory of a suite is a group with an
// arrangement of its own, and folding it into the top level would start files
// on terms that were never theirs. The count in the report is of what was
// run, so a group left out is visible as a smaller population rather than as
// a silent pass.
func Files(tests, ext string) ([]string, error) {
	entries, err := os.ReadDir(tests)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ext) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// copyTree copies a directory, files and subdirectories, and nothing else.
func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(p, target, d)
	})
}

func copyFile(src, dest string, d fs.DirEntry) error {
	info, err := d.Info()
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	// The executable bit is carried across, because the helpers were
	// compiled into this tree and a suite file calls them as programs.
	mode := fs.FileMode(0o600)
	if info.Mode().Perm()&0o100 != 0 {
		mode = 0o700
	}
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// rank orders causes by how many files each stopped, ties broken on the
// wording so that two runs on one machine print the same report.
//
// The ranking is the actionable output of this instrument, and what makes it
// actionable is the split rather than the count: a cause the reference's own
// `-n` refuses as well is not a gap in this parser, because no static read
// reaches it. Costing a cause by files is still the right unit for the static
// consumers of this parser — a formatter refuses the file, not the line — but
// it is a statement about that route and not about a run.
func rank(counts, shared map[string]int) []Cause {
	causes := make([]Cause, 0, len(counts))
	for reason, n := range counts {
		causes = append(causes, Cause{Reason: reason, Files: n, ReferenceRefuses: shared[reason]})
	}
	sort.Slice(causes, func(i, j int) bool {
		if causes[i].Files != causes[j].Files {
			return causes[i].Files > causes[j].Files
		}
		return causes[i].Reason < causes[j].Reason
	})
	return causes
}

func rankStatuses(counts map[[2]int]int) []StatusPair {
	pairs := make([]StatusPair, 0, len(counts))
	for k, n := range counts {
		pairs = append(pairs, StatusPair{Ours: k[0], Reference: k[1], Files: n})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Files != pairs[j].Files {
			return pairs[i].Files > pairs[j].Files
		}
		if pairs[i].Ours != pairs[j].Ours {
			return pairs[i].Ours < pairs[j].Ours
		}
		return pairs[i].Reference < pairs[j].Reference
	})
	return pairs
}

// Shell is the path a run may be handed a shell by, resolved once, where the
// person naming it is.
//
// Absolute, because [runIn] gives every run a directory of its own and runs
// the file from inside it — so a relative path to the binary under test is
// resolved against that directory and not against the caller's. It resolves
// to nothing, every run fails to start, and the column reports a score. The
// cost of that being caught late is on record: `make suite` passes absolute
// paths and worked, while the same command typed by hand with `-own-bin
// bash=build/own-bash` reported 0/10 strict for four dialects, which is a
// wrong answer rather than an error.
//
// Existence is checked here for the same reason. A missing binary is a fact
// about the invocation and belongs in the invocation's diagnostic, not spread
// across a per-file column of failures.
func Shell(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory, not a shell", abs)
	}
	return abs, nil
}

// Locate is the first of a suite's reference paths that exists.
func Locate(lookup []string) (string, bool) {
	for _, p := range lookup {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, true
		}
	}
	return "", false
}
