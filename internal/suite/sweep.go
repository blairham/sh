// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
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
	// Unstable says the reference did not produce the same run twice, so the
	// file says nothing about either shell.
	Unstable bool
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
	Suite            Suite
	Reference        string
	ReferenceVersion string
	Ours             string
	Helpers          []string

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

	OracleHung  int
	DialectHung int
	Unstable    int

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
}

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
	tests := filepath.Join(dir, filepath.FromSlash(s.TestDir))
	names, err := Files(tests, s.Ext)
	if err != nil {
		return rep, err
	}
	if opts.Only != nil {
		var kept []string
		for _, n := range names {
			if opts.Only[n] {
				kept = append(kept, n)
			}
		}
		names = kept
	}
	rep.Files = len(names)

	dial, haveDialect := s.Syntax()
	results := make([]Result, len(names))
	sem := make(chan struct{}, opts.jobs())
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = grade(ctx, s, tests, name, ours, reference, dial, haveDialect, opts.timeout())
		}()
	}
	wg.Wait()

	causes := map[string]int{}
	shared := map[string]int{}
	statuses := map[[2]int]int{}
	var meanSum float64
	for _, res := range results {
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
	rep.StatusPairs = rankStatuses(statuses)
	return rep, nil
}

// grade is one file, both ways.
func grade(ctx context.Context, s Suite, tests, name, ours, reference string, dial syntax.Dialect, haveDialect bool, timeout time.Duration) Result {
	var res Result
	src, err := os.ReadFile(filepath.Join(tests, name))
	if err != nil {
		res.Cause = "unreadable"
		return res
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

	ref := runIn(ctx, s, tests, name, reference, timeout)
	if ref.TimedOut {
		res.OracleHung = true
		return res
	}
	own := runIn(ctx, s, tests, name, ours, timeout)
	if own.TimedOut {
		res.DialectHung = true
		return res
	}

	res.OurStatus, res.RefStatus = own.Status, ref.Status
	if own.Output == ref.Output && own.Status == ref.Status {
		// Identical before normalization, so there is nothing for
		// normalization to decide and nothing to re-run: the two shells wrote
		// the same bytes.
		res.Scored, res.Strict = true, true
		l := lines(own.Output)
		res.Common, res.Longest = len(l), len(l)
		return res
	}

	theirs := normalize(ref.Output, reference, ref.Dir)
	mine := normalize(own.Output, ours, own.Dir)
	if mine == theirs && own.Status == ref.Status {
		res.Scored, res.Strict = true, true
		l := lines(mine)
		res.Common, res.Longest = len(l), len(l)
		return res
	}
	if !repeats(ctx, s, tests, name, reference, timeout, theirs, ref.Status) {
		// The reference does not produce the same run twice, so the two
		// shells were never going to agree and this file is evidence about
		// neither. A process id, a clock reading, a scheduling order: the
		// reference disagrees with itself on these, and counting them against
		// us would put a number on the machine.
		res.Unstable = true
		return res
	}
	res.Scored = true
	res.Common, res.Longest, res.LineCapped = agreement(lines(mine), lines(theirs))
	return res
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
func runIn(ctx context.Context, s Suite, tests, name, shell string, timeout time.Duration) placed {
	dir, err := os.MkdirTemp("", "suite")
	if err != nil {
		return placed{Outcome: Outcome{Output: err.Error(), Status: -1}}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	run := filepath.Join(dir, "t")
	if err := copyTree(tests, run); err != nil {
		return placed{Outcome: Outcome{Output: err.Error(), Status: -1}, Dir: run}
	}
	out := runFile(ctx, shell, run, name, environ(s, run, shell), timeout)
	return placed{Outcome: out, Dir: run}
}

// repeats asks the reference for the same file again, and only where the two
// shells differed — the one place the answer changes anything.
func repeats(ctx context.Context, s Suite, tests, name, reference string, timeout time.Duration, want string, status int) bool {
	again := runIn(ctx, s, tests, name, reference, timeout)
	if again.TimedOut {
		return false
	}
	return normalize(again.Output, reference, again.Dir) == want && again.Status == status
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

// Locate is the first of a suite's reference paths that exists.
func Locate(lookup []string) (string, bool) {
	for _, p := range lookup {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, true
		}
	}
	return "", false
}

// Version is the build string a reference shell reports, for the report's
// header. The suite is pinned and the shell on the machine is not, so a
// reader needs both to know what was compared.
func Version(ctx context.Context, shell string) string {
	out, err := runVersion(ctx, shell)
	if err != nil {
		return "unknown"
	}
	line, _, _ := strings.Cut(out, "\n")
	return strings.TrimSpace(line)
}
