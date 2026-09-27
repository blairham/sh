// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strconv"
	"strings"
	"syscall"

	"github.com/blairham/sh/interp"
)

// `$jobstates`, `$jobtexts` and `$jobdirs`: the shell's job table, presented
// as three associations keyed by the number `%N` names.
//
// They were three of the module's absent names until #4760, and the refusal
// was fatal, so a script that read one stopped there. Nothing about them was
// missing from the shell: `jobs -d` already names the directory a job started
// in, and the state and the command line are what `jobs` itself prints. What
// was missing was the publication.
//
// Measured 2026-09-26 against zsh 5.9.2 (aarch64-apple-darwin25.4.0) run `-f`
// with `env -u FPATH` over a script file, `zmodload zsh/parameter` first:
//
//	sleep 5 &            jobstates[1]  running:+:P=running
//	                     jobtexts[1]   sleep 5
//	                     jobdirs[1]    the directory the job started in
//	sleep 9 | cat &      jobstates[1]  running:+:P1=running:P2=running
//	                     jobtexts[1]   sleep 9 | cat
//	four jobs            jobstates     1 running::…  2 running::…
//	                                   3 running:-:…  4 running:+:…
//	kill -STOP on one    jobstates[1]  suspended:+:P=suspended (signal)
//
// So a value is the job's state, the marker `%+` and `%-` put on it, and then
// one `pid=state` per process of the job, all parted by colons — and the
// marker's field is *empty* for a job carrying neither, which is what makes
// the four-job row the one worth measuring: two jobs with markers cannot show
// that the separator stays.
//
// `${(t)…}` is `association-readonly-hide-hideval-special` for all three, so
// each carries the readonly-and-hidden pair `$builtins` needs and for the same
// two reasons: zsh refuses an assignment to them, and a produced table with
// neither would take one into a stored table that then shadows the producer.
//
// **Views and not snapshots**, as everything else in this module is: the
// producers run at the moment the parameter is read, so a job started between
// two reads is in the second of them. The tests prove it that way round, since
// a snapshot taken late enough passes a test that only asks whether a value
// looks right.
//
// A *finished* job is not in the table at all — measured, `/bin/sh -c 'exit 3'
// &` is gone by the time the next line reads `${jobstates[1]}`, and the job
// started after it is numbered 1 — so nothing here has to word a job that
// ended. That is the shell's own reaping and not this file's.

// registerJobParameters installs the three as views over the job table.
func registerJobParameters(r *interp.Runner) {
	for _, p := range []struct {
		name  string
		view  func(*interp.Runner) interp.AssocArray
		value func(*interp.Runner, string) (string, bool)
	}{
		{"jobstates", jobStatesView, jobStateValue},
		{"jobtexts", jobTextsView, jobTextValue},
		{"jobdirs", jobDirsView, jobDirValue},
	} {
		r.SetDynamicAssoc(p.name, p.view)
		// And the same table read one key at a time, which is what nearly
		// every read of one is: `${jobstates[1]}` must not have to render
		// every job to reach the first.
		r.SetDynamicAssocElement(p.name, p.value)
		r.MarkReadonly(p.name)
		hideModuleParameter(r, p.name)
	}
}

// listedJobs is the jobs these three report on: the table as it stands, less
// anything that has already ended.
//
// The reaping is the shell's, so this is a guard rather than a policy — a job
// the table has not yet compacted away is one the reference would not show,
// and a view that showed it would be publishing this shell's bookkeeping
// rather than its job table.
func listedJobs(r *interp.Runner) []*interp.Job {
	all := r.Jobs()
	out := make([]*interp.Job, 0, len(all))
	for _, j := range all {
		if j.Finished() {
			continue
		}
		out = append(out, j)
	}
	return out
}

// jobKey is the key a job is listed under: the number `%N` names it by,
// written out.
func jobKey(j *interp.Job) string { return strconv.Itoa(j.Number()) }

// jobByKey finds the job a key names, so that reading one element never
// builds the whole table.
//
// **The key is a job specification and not a number**, which is the half a
// table keyed by `1` and `2` hides: measured 2026-09-26 against zsh 5.9.2,
// `${jobstates[%+]}`, `${jobtexts[%?lee]}` and `${jobstates[sleep]}` all
// resolve, so this goes through the same resolver `fg` and `kill` use rather
// than reading the key as an integer.
//
// A miss is worded two ways there and the split is the reason
// [interp.Runner.FindJobBySpec] tells them apart: a *number* naming no job is
// silent — `${jobstates[9]}` and `${jobstates[%9]}` are empty at status 0 —
// and *text* matching no command is `job not found: nosuch`, with the `%`
// taken off the name in the sentence. The value is empty and the status is 0
// either way, so the complaint is the only thing that separates them and a
// row asserting the value alone would prove nothing.
func jobByKey(r *interp.Runner, key string) (*interp.Job, bool) {
	j, code := r.FindJobBySpec(key)
	switch code {
	case interp.JobSpecFound:
		if j.Finished() {
			// The table these three publish is the one that has not ended —
			// see listedJobs.
			return nil, false
		}
		return j, true
	case interp.JobSpecNoSuchName, interp.JobSpecAmbiguousName:
		// The sentence is this shell's own and is written here rather than
		// carried on Diagnostics, for the reason refuseEmptyParameterWrite's
		// is: nothing outside this dialect has the parameter to complain
		// about. Measured 2026-09-26: reading a key that matches no job's
		// command is `<file>:N: job not found: nosuch`.
		r.Diagnosef("job not found: %s\n", strings.TrimPrefix(key, "%"))
	}
	return nil, false
}

// jobStatesView is `$jobstates`: each job's state, its marker, and one
// `pid=state` for every process it is made of.
func jobStatesView(r *interp.Runner) interp.AssocArray {
	jobs := listedJobs(r)
	out := make(interp.AssocArray, len(jobs))
	for _, j := range jobs {
		out[jobKey(j)] = interp.Scalar(jobStateText(r, j))
	}
	return out
}

// jobStateValue is `${jobstates[N]}`, the one key.
func jobStateValue(r *interp.Runner, key string) (string, bool) {
	j, ok := jobByKey(r, key)
	if !ok {
		return "", false
	}
	return jobStateText(r, j), true
}

// jobStateText builds one job's value.
//
// The marker comes from [interp.Runner.MarkedJobs] rather than from a rule of
// this file's own, which is what keeps it from disagreeing with the `+` and
// `-` a `jobs` listing prints: they are one fact about one job and the choice
// between a stopped job and the newest one is an axis the core already holds.
func jobStateText(r *interp.Runner, j *interp.Job) string {
	state := jobStateWord(j.Stopped, syscall.Signal(j.StopSig))
	var b strings.Builder
	b.WriteString(state)
	b.WriteByte(':')
	current, previous := r.MarkedJobs()
	switch j {
	case current:
		b.WriteByte('+')
	case previous:
		b.WriteByte('-')
	}
	// A job carrying neither marker leaves the field empty and keeps both
	// colons, which is the shape the four-job measurement is about.
	b.WriteByte(':')
	for i, pid := range j.Processes() {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(strconv.Itoa(pid))
		b.WriteByte('=')
		b.WriteString(state)
	}
	return b.String()
}

// jobTextsView is `$jobtexts`: the command line each job was started from.
func jobTextsView(r *interp.Runner) interp.AssocArray {
	jobs := listedJobs(r)
	out := make(interp.AssocArray, len(jobs))
	for _, j := range jobs {
		out[jobKey(j)] = interp.Scalar(j.Command)
	}
	return out
}

// jobTextValue is `${jobtexts[N]}`, the one key.
func jobTextValue(r *interp.Runner, key string) (string, bool) {
	j, ok := jobByKey(r, key)
	if !ok {
		return "", false
	}
	return j.Command, true
}

// jobDirsView is `$jobdirs`: the directory each job was started in.
//
// The job's and not the shell's, which is the whole discriminating case: a
// `cd` between starting a job and reading this does not move the answer, and
// it is the same field `jobs -d` names under a row (#4507).
func jobDirsView(r *interp.Runner) interp.AssocArray {
	jobs := listedJobs(r)
	out := make(interp.AssocArray, len(jobs))
	for _, j := range jobs {
		out[jobKey(j)] = interp.Scalar(j.Dir)
	}
	return out
}

// jobDirValue is `${jobdirs[N]}`, the one key.
func jobDirValue(r *interp.Runner, key string) (string, bool) {
	j, ok := jobByKey(r, key)
	if !ok {
		return "", false
	}
	return j.Dir, true
}
