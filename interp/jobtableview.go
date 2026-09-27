// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// The job table read from outside this package, for a dialect that publishes
// it as parameters a script can read.
//
// The three facts below are the ones a listing already uses and nothing
// outside could reach: the number a job is *listed* under, the processes it is
// made of, and which two jobs carry the markers. Everything else such a view
// needs — what was typed, the directory, whether the job is stopped, what
// stopped it — is already a field or a method of [Job].
//
// **Facts and not a rendering.** The state words, the separators and the
// order a parameter's value puts them in belong to the shell publishing them,
// exactly as a listing's columns belong to the dialect that prints one: this
// package holds the table and never says how a shell spells it. See
// dialect/zsh/jobparam.go for one such spelling.

// Number is the number this job is listed under and named by: `%2` is the job
// whose Number is 2, for as long as the job is in the table.
//
// Not [Job.Ident], which is the number `$!` answers and `wait <n>` takes — a
// process id, or one this shell invented for a job that has no process. The
// two are different numbers for the same job and a caller that confused them
// would key a table by a pid.
func (j *Job) Number() int { return j.num }

// Processes is every process this job is made of just now, oldest first.
//
// The same list [Job.processes] hands a signal, so a view built from it names
// what `kill %N` would reach rather than a second opinion about it. That also
// means it is *pruned*: a process the shell has already waited for is not one
// of them, and a job between two commands is made of the pid it settled with.
func (j *Job) Processes() []int {
	live := j.processes()
	out := make([]int, 0, len(live))
	for _, p := range live {
		out = append(out, p.pid)
	}
	return out
}

// MarkedJobs is the job `%+` names and the job `%-` names, either of which is
// nil where this shell has no such job.
//
// The same two [Runner.markedJobs] hands a listing, which is the point: a
// parameter naming the current job and a `jobs` listing marking one must not
// be able to disagree. The choice itself is an axis — see
// Semantics.StoppedJobTakesTheCurrentJobMarker.
func (r *Runner) MarkedJobs() (current, previous *Job) { return r.markedJobs() }

// JobSpecResult is how a job specification handed to [Runner.FindJobBySpec]
// was resolved. The misses are told apart because a caller words them
// differently: a number naming no job is nothing to say anything about, and
// text matching no job's command is a name the caller can quote back.
type JobSpecResult int

const (
	// JobSpecFound is a job.
	JobSpecFound JobSpecResult = iota
	// JobSpecNoSuchNumber is a number, or a marker, naming no job in the
	// table as it stands.
	JobSpecNoSuchNumber
	// JobSpecNoSuchName is text matching no job's command.
	JobSpecNoSuchName
	// JobSpecAmbiguousName is text matching more than one.
	JobSpecAmbiguousName
	// JobSpecUnanswered is a dialect that has not said whether text is a job
	// specification at all — see Semantics.JobSpecsByName. The refusal has
	// already been written where this is returned.
	JobSpecUnanswered
)

// FindJobBySpec is the job a specification names, for a caller outside this
// package that publishes the table — a parameter keyed by job, where a script
// writes `${jobstates[1]}`, `${jobstates[%+]}` or `${jobstates[sleep]}` and
// expects the same three answers `fg` and `kill` give.
//
// The same resolver the builtins use, so a parameter and a `kill` cannot come
// to disagree about what `%?text` means, and it complains about nothing: the
// caller words the miss, which is the whole reason the misses are separate
// results.
//
// **A leading `%` is what makes the markers markers.** With it the whole
// grammar applies — `%%`, `%+`, `%-`, `%N`, `%text`, `%?text`. Without it only
// a number is special and everything else is text to match against a command,
// so a bare `-` is a name and not the previous job. Measured 2026-09-26
// against zsh 5.9.2 through the parameters this exists for: `${jobstates[%-]}`
// is the previous job and `${jobstates[-]}` is `job not found: -`.
func (r *Runner) FindJobBySpec(spec string) (*Job, JobSpecResult) {
	if marked := strings.HasPrefix(spec, "%"); !marked {
		if _, ok := atoi(spec); !ok {
			j, code := r.findJobByName(spec)
			return j, nameLookupResult(code)
		}
	}
	j, code := r.findJobQuietly(spec)
	switch code {
	case jobFound:
		return j, JobSpecFound
	case jobSpecAmbiguous:
		return nil, JobSpecAmbiguousName
	case jobSpecUnanswered:
		return nil, JobSpecUnanswered
	}
	if text := strings.TrimPrefix(spec, "%"); text != "" && text != "%" &&
		text != "+" && text != "-" {
		if _, ok := atoi(text); !ok {
			return nil, JobSpecNoSuchName
		}
	}
	return nil, JobSpecNoSuchNumber
}

// nameLookupResult maps the lookup codes onto the exported ones for a spec
// that was resolved as text in the first place.
func nameLookupResult(code int) JobSpecResult {
	switch code {
	case jobFound:
		return JobSpecFound
	case jobSpecAmbiguous:
		return JobSpecAmbiguousName
	case jobSpecUnanswered:
		return JobSpecUnanswered
	}
	return JobSpecNoSuchName
}
