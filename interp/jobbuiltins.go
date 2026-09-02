// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"strings"
	"syscall"
)

// `jobs`, `fg` and `bg` — three of the names reserved in #117, and the three
// that could not be written until something could tell a stopped command from
// a finished one.
//
// They are registered here rather than in a dialect because all four shells
// have them and disagree only about how the listing is worded, which is what
// Diagnostics is for.

func init() {
	builtins["jobs"] = biJobs
	builtins["fg"] = biFg
	builtins["bg"] = biBg
}

func biJobs(r *Runner, _ context.Context, args []string) int {
	args, _, code := r.builtinOptions("jobs", args, "lpnrs")
	if code != 0 {
		return code
	}
	jobs := r.jobs
	if len(args) > 0 {
		j, code := r.findJob(args[0], "jobs")
		if code != 0 {
			return code
		}
		jobs = []*Job{j}
	}
	rows, code := r.jobRows(jobs)
	if code != 0 {
		return code
	}
	showBg, code := r.showsBackgroundCommand(rows)
	if code != 0 {
		return code
	}
	for _, row := range rows {
		r.printf("%s\n", r.jobLine(row.n, row.job, showBg))
	}
	// The listing that reports a finished job is the listing that forgets
	// it: every shell in the panel mentions one at most once, so a second
	// `jobs` shows nothing. Not an axis and not optional — a shell that kept
	// them would grow a listing for the length of the session.
	//
	// Over what was *looked at* rather than over what was printed, which is
	// not the same set: the dialect that leaves a finished job out of the
	// listing has still finished with it, and forgetting only the printed
	// ones would keep them forever in exactly that dialect.
	for _, j := range jobs {
		if j.Finished() {
			r.Forget(j)
		}
	}
	return 0
}

// jobRow is a job together with the number it is listed under, which is its
// own and not its place in the listing — the two differ wherever the newest
// is printed first.
type jobRow struct {
	n   int
	job *Job
}

// jobRows is what a `jobs` listing contains and in what order, both of which
// the dialect answers.
func (r *Runner) jobRows(jobs []*Job) ([]jobRow, int) {
	rows := make([]jobRow, 0, len(jobs))
	for i, j := range jobs {
		// Asked only where there is a finished job to leave out. A listing
		// of running ones is the same in every shell, and refusing it
		// because of a question nothing turned on would be refusing to work.
		if j.Finished() {
			if !r.ask(r.sem().JobsListFinishedJobs, "a finished job appearing in a `jobs` listing") {
				if r.unspecified {
					return nil, 2
				}
				continue
			}
		}
		rows = append(rows, jobRow{n: i, job: j})
	}
	// Likewise: one job is in the same place either way.
	if len(rows) > 1 {
		if r.ask(r.sem().JobsListNewestFirst, "a `jobs` listing starting with the most recent") {
			for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
				rows[i], rows[j] = rows[j], rows[i]
			}
		} else if r.unspecified {
			return nil, 2
		}
	}
	return rows, 0
}

// jobLine is one row of a `jobs` listing.
//
// Four shells, four shapes — the number and the marker are common and
// everything else is not, so the wording carries three verbs: the marker, the
// state and the command.
func (r *Runner) jobLine(i int, j *Job, showBg bool) string {
	dg := r.diag()
	state := Wording(dg.JobRunning, "Running")
	switch {
	case j.Stopped:
		// One verb, the signal that stopped it, which only one dialect names.
		state = Wording(dg.JobStopped, "Stopped", j.StopSig)
	case j.Finished():
		state = Wording(dg.JobDone, "Done")
		if j.Status != 0 && dg.JobExited != "" {
			// A dialect that says something else for a job that failed. One
			// verb, the status, and no dialect words it without one.
			state = Wording(dg.JobExited, "Exit %[1]d", j.Status)
		}
	}
	return Wording(dg.JobLine, "[%[1]d]%[2]s  %-24[3]s%[4]s",
		i+1, r.jobMarker(j), state, r.jobCommand(j, showBg))
}

// jobCommand is the command column of a listing.
//
// Three things it is not simply j.Command. A shell that kept no text has a
// placeholder to print in its place; a job still running carries its `&` back
// in one dialect and not in the others; and a job that has ended never does,
// even there.
func (r *Runner) jobCommand(j *Job, showBg bool) string {
	if j.Command == "" || (!j.Stopped && !showBg) {
		return r.diag().JobUnknownCommand
	}
	if r.diag().JobRunningShowsAmpersand && !j.Stopped && !j.Finished() {
		return j.Command + " &"
	}
	return j.Command
}

// showsBackgroundCommand asks whether a `&` job's command belongs in the
// listing, once, and only where there is one to leave out.
//
// A stopped job's command is printed by every shell in the panel, and a job
// whose text was never kept has nothing to decide about — so a listing of
// those alone is the same in all four and must not be refused.
func (r *Runner) showsBackgroundCommand(rows []jobRow) (bool, int) {
	decides := false
	for _, row := range rows {
		if !row.job.Stopped && row.job.Command != "" {
			decides = true
			break
		}
	}
	if !decides {
		return false, 0
	}
	show := r.ask(r.sem().JobsShowBackgroundCommand, "the command of a `&` job appearing in a `jobs` listing")
	if r.unspecified {
		return false, 2
	}
	return show, 0
}

// jobMarker is the `+` on the job `fg` would pick and the `-` on the one after
// it, which is how a listing says what `%%` and `%-` mean without spelling
// them out.
func (r *Runner) jobMarker(j *Job) string {
	switch {
	case j == r.lastJob:
		return "+"
	case len(r.jobs) > 1 && j == r.jobs[len(r.jobs)-2]:
		return "-"
	}
	return " "
}

func biFg(r *Runner, _ context.Context, args []string) int {
	j, code := r.resume(args, "fg")
	if code != 0 {
		return code
	}
	// Named on the way in, which is how a shell says which job it just put
	// back in front of you when you did not say.
	r.printf("%s\n", j.Command)
	if err := r.signalJob(j, syscall.SIGCONT); err != nil {
		r.diagf("fg: %v\n", err)
		return 1
	}
	j.Stopped = false
	if r.WaitForCommand == nil {
		// Nothing here can wait for it, so saying it was resumed is the most
		// this can honestly claim.
		return 0
	}
	// The terminal goes with it, for the same reason it does when a command
	// starts: without it ^C and ^Z would reach this shell instead, and the
	// job would run on unreachable while the wait below never returned.
	if r.Foreground != nil {
		if err := r.Foreground(j.PID); err == nil {
			defer func() { _ = r.Foreground(0) }()
		}
	}
	w, err := r.WaitForCommand(j.PID)
	if err != nil {
		r.diagf("fg: %v\n", err)
		return 1
	}
	status, stopped := r.waitResult(w)
	if stopped {
		// Stopped again, so it stays a job rather than being forgotten.
		j.Stopped = true
		return status
	}
	r.Forget(j)
	return status
}

func biBg(r *Runner, _ context.Context, args []string) int {
	j, code := r.resume(args, "bg")
	if code != 0 {
		return code
	}
	if err := r.signalJob(j, syscall.SIGCONT); err != nil {
		r.diagf("bg: %v\n", err)
		return 1
	}
	j.Stopped = false
	// `&` after it, which is what says the shell is not waiting.
	r.printf("%s &\n", j.Command)
	return 0
}

// resume finds the job `fg` or `bg` was asked about.
func (r *Runner) resume(args []string, name string) (*Job, int) {
	if len(args) == 0 {
		if r.lastJob == nil {
			r.diagf("%s\n", Wording(r.diag().NoSuchJob, "%[1]s: no current job", name))
			return nil, 1
		}
		return r.lastJob, 0
	}
	return r.findJob(args[0], name)
}

// findJob reads a job spec.
//
// `%1` by number, `%%` and `%+` for the current one, `%-` for the one before
// it, and a bare number for the same. Unanimous across the panel, which is why
// none of it is a dialect question.
func (r *Runner) findJob(spec, name string) (*Job, int) {
	j, code := r.findJobQuietly(spec)
	if code != 0 {
		r.diagf("%s\n", Wording(r.diag().NoSuchJob, "%[1]s: %[2]s: no such job", name, spec))
		return nil, 1
	}
	return j, 0
}

// signalJob sends to the job's process group rather than to the one process.
//
// The group is the point: a job is a pipeline as often as a command, and
// signaling only the first of three would resume one and leave the rest
// stopped. The negative pid is how the kernel is told to mean the group.
func (r *Runner) signalJob(j *Job, sig syscall.Signal) error {
	if j.PID == 0 {
		// A job with no process of its own — a builtin or a compound command
		// running on a cloned runner. There is nothing to signal, and saying
		// so is better than signaling something else.
		return errNoJobProcess
	}
	if r.SignalGroup == nil {
		return errNoJobProcess
	}
	return r.SignalGroup(j.PID, sig)
}

// errNoJobProcess is a job there is nothing to signal for: one that never had
// a process, or a shell with no way to send to a group.
var errNoJobProcess = errors.New("this job has no process to resume")

// findJobQuietly is findJob without the complaint, for a caller that words its
// own — `kill %9` is `kill`'s error to report, not this one's.
func (r *Runner) findJobQuietly(spec string) (*Job, int) {
	text := strings.TrimPrefix(spec, "%")
	switch text {
	case "", "%", "+":
		if r.lastJob == nil {
			return nil, 1
		}
		return r.lastJob, 0
	case "-":
		if len(r.jobs) < 2 {
			return nil, 1
		}
		return r.jobs[len(r.jobs)-2], 0
	}
	n, ok := atoi(text)
	if !ok || n < 1 || n > len(r.jobs) {
		return nil, 1
	}
	return r.jobs[n-1], 0
}
