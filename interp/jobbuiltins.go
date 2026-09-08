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
	builtins["disown"] = biDisown
}

// biDisown is the shell letting go of a job.
//
// What letting go *means* is the axis: two shells take the job out of the
// table, so `jobs` no longer lists it, and one only shields it from the HUP
// an exiting interactive shell would send — a signal this engine never
// forwards — so its listing keeps the job. dash has no disown at all, and
// unregisters it. See Semantics.DisownRemovesTheJob.
func biDisown(r *Runner, _ context.Context, args []string) int {
	// The letters the dialects have — bash's -a, -h, -r — are not
	// implemented; they ride UnimplementedOptionLetters and are refused by
	// name.
	if len(args) > 0 && len(args[0]) > 1 && args[0][0] == '-' {
		if args[0] == "--" {
			args = args[1:]
		} else {
			return r.refuseOption("disown", args[0], "")
		}
	}
	var jobs []*Job
	if len(args) == 0 {
		// The current job, and with none the complaint is the dialect's —
		// or, in one shell, a bare failing status. Empty means silence: the
		// engine measured saying nothing says nothing here on purpose.
		if r.lastJob == nil {
			if w := r.diag().DisownNoCurrentJob; w != "" {
				r.diagf("%s\n", w)
			}
			return 1
		}
		jobs = []*Job{r.lastJob}
	}
	for _, a := range args {
		j, code := r.findJob(a, "disown")
		if code != 0 {
			return code
		}
		jobs = append(jobs, j)
	}
	if !r.ask(r.sem().DisownRemovesTheJob, "`disown` taking the job out of the `jobs` table") {
		if r.unspecified {
			return r.status
		}
		// Shielded from a HUP this engine never sends: nothing to do, and
		// saying so would be inventing output.
		return 0
	}
	for _, j := range jobs {
		r.Forget(j)
	}
	return 0
}

// jobsForm is what a `jobs` listing prints per job: the state row, the same
// row with the process id in it, or the process id alone.
//
// The three are chosen by `-l` and `-p`, and the *last* letter given decides
// in every shell that has both — `jobs -pl` is the long listing and
// `jobs -lp` the process ids, in dash, bash and ksh93 alike. Unanimous, so
// not an axis.
type jobsForm int

const (
	jobsStateRow jobsForm = iota
	jobsLongRow
	jobsPidsAlone
)

func biJobs(r *Runner, _ context.Context, args []string) int {
	// The letters are the dialect's. A shared set would have this engine
	// accept `jobs -r` in dash, which refuses it — the failure mode #469
	// was: an option nobody reads is an option silently ignored.
	letters := r.sem().JobsOptions
	if letters == "" {
		letters = "lp"
	}
	args, opts, code := r.builtinOptions("jobs", args, letters)
	if code != 0 {
		return code
	}
	// What `bg` let go of may have ended since the last prompt, and a listing
	// that did not ask would report it as still running.
	r.reapJobs()
	// A listing is the shell showing the person their stopped jobs, which is
	// the whole of what the warning at exit is for — measured, `exit` after a
	// `jobs` exits at once in bash and in zsh, where `exit` after any other
	// command warns first. Set on the way in, and set for `jobs -p` too: the
	// form that prints only process ids clears it just the same. This chunk's
	// flag rather than the one `exit` reads: a listing suppresses the warning
	// for the line after it, and not for a line after that.
	r.tellingOfJobsAtExit = true
	form, code := r.jobsForm(opts)
	if code != 0 {
		return code
	}
	wanted, code := r.jobStateFilter(opts)
	if code != 0 {
		return code
	}

	// Operands name jobs, and every shell in the panel lists them in the
	// order they were written rather than in the listing's own order — so
	// `jobs %1 %2` and `jobs %2 %1` differ, in the two dialects whose bare
	// listing starts from the newest as much as in the other two.
	//
	// A spec that names nothing is reported *after* the jobs written before
	// it have been listed, unanimously, which is why the complaint is held
	// back rather than made where the lookup failed.
	jobs := r.jobs
	explicit := len(args) > 0
	badSpec, badLookup := "", jobFound
	if explicit {
		jobs = make([]*Job, 0, len(args))
		for _, a := range args {
			j, found := r.findJobQuietly(a)
			if found != jobFound {
				badSpec, badLookup = a, found
				break
			}
			jobs = append(jobs, j)
		}
	}
	if code := r.printJobs(jobs, form, wanted, explicit); code != 0 {
		return code
	}
	if badLookup != jobFound {
		return r.reportJobLookup(badSpec, badLookup, "jobs")
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
	//
	// A listing that was not a listing of states does not count. Measured:
	// `jobs -p` and `jobs -r` in bash both leave the finished job for the
	// next bare `jobs` to report, where `jobs` and `jobs -l` consume it. So
	// only a form that showed the job's state, and only with nothing
	// filtered out, finishes with it.
	if form == jobsPidsAlone || wanted != anyJobState {
		return 0
	}
	for _, j := range jobs {
		if j.Finished() {
			r.Forget(j)
		}
	}
	return 0
}

// printJobs writes the listing itself, once the form, the filter and the set
// of jobs are settled.
func (r *Runner) printJobs(jobs []*Job, form jobsForm, wanted jobState, explicit bool) int {
	rows, code := r.jobRows(jobs, explicit)
	if code != 0 {
		return code
	}
	showBg, code := r.showsBackgroundCommand(rows)
	if code != 0 {
		return code
	}
	for _, row := range rows {
		if !wanted.holds(row.job) {
			continue
		}
		switch form {
		case jobsPidsAlone:
			if row.job.PID == 0 {
				// A job with no process of its own — a builtin or a
				// compound command on a cloned runner, where a real shell
				// would have forked and had an id to print. Left out
				// rather than printed as 0, because this listing is
				// written to be *used*: `kill $(jobs -p)` with a 0 in it
				// signals the whole process group.
				continue
			}
			r.printf("%d\n", row.job.PID)
		case jobsLongRow:
			r.printf("%s\n", r.jobLineLong(row.n, row.job, showBg))
		default:
			r.printf("%s\n", r.jobLine(row.n, row.job, showBg))
		}
	}
	return 0
}

// jobsForm reads `-l` and `-p` out of the letters that were given.
func (r *Runner) jobsForm(opts string) (jobsForm, int) {
	form := jobsStateRow
	for i := len(opts) - 1; i >= 0; i-- {
		switch opts[i] {
		case 'l':
			return jobsLongRow, 0
		case 'p':
			// zsh spells the process *group* with the same letter and
			// prints its ordinary rows, which is why `kill $(jobs -p)`
			// is a bash idiom and not a portable one.
			if r.ask(r.sem().JobsPidsOnlyOption, "`jobs -p` printing process ids and nothing else") {
				return jobsPidsAlone, 0
			}
			if r.unspecified {
				return form, 2
			}
			return jobsLongRow, 0
		}
	}
	return form, 0
}

// jobState is which jobs a listing was asked for: `-r` running, `-s` stopped,
// or — with neither letter, and with both where the dialect adds them up —
// whatever is there.
type jobState int

const (
	anyJobState jobState = iota
	runningJobs
	stoppedJobs
)

// holds reports whether a job is one the filter asked for.
//
// A finished job is neither running nor stopped, and is left out by both
// letters: measured in bash, where `jobs -r` says nothing about a job that
// has ended and the next bare `jobs` still reports it.
func (w jobState) holds(j *Job) bool {
	switch w {
	case runningJobs:
		return !j.Stopped && !j.Finished()
	case stoppedJobs:
		return j.Stopped
	}
	return true
}

// jobStateFilter reads `-r` and `-s`.
func (r *Runner) jobStateFilter(opts string) (jobState, int) {
	last := anyJobState
	both := false
	for i := 0; i < len(opts); i++ {
		switch opts[i] {
		case 'r':
			both = both || last == stoppedJobs
			last = runningJobs
		case 's':
			both = both || last == runningJobs
			last = stoppedJobs
		}
	}
	if !both {
		return last, 0
	}
	// Both letters at once, which is the only place the two shells that
	// have them disagree.
	if r.ask(r.sem().JobsStateFiltersAccumulate, "`jobs -r -s` listing a job in either state") {
		return anyJobState, 0
	}
	if r.unspecified {
		return last, 2
	}
	return last, 0
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
//
// explicit says the jobs were named by operands, which settles the order
// itself: `jobs %1 %2` and `jobs %2 %1` list them the way they were written
// in every shell in the panel, including the two whose bare listing starts
// from the newest.
func (r *Runner) jobRows(jobs []*Job, explicit bool) ([]jobRow, int) {
	rows := make([]jobRow, 0, len(jobs))
	for _, j := range jobs {
		// The job's own number, not its place in this listing. `jobs %2`
		// printed `[1]` before this, because a listing of one job counted
		// from the start of the slice it had been handed.
		i := r.jobNumber(j)
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
	// Likewise: one job is in the same place either way. And a listing whose
	// jobs were named by operands is already in the order it was asked for,
	// in every shell in the panel — `jobs %1 %2` lists 1 then 2 in dash and
	// ksh93 too, whose bare listing starts from the other end.
	if len(rows) > 1 && !explicit {
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
//
// jobLineAs renders one row of a listing. noticing says the shell is reporting
// that the job ended rather than listing one, which one dialect words
// differently.
func (r *Runner) jobLine(i int, j *Job, showBg bool) string {
	return r.jobLineAs(i, j, showBg, false)
}

// jobLineLong is the same row with the process id in it, which is what `-l`
// adds. Every shell in the panel has the letter and every one of them puts
// the id somewhere different, so the whole row is the dialect's format.
func (r *Runner) jobLineLong(i int, j *Job, showBg bool) string {
	dg := r.diag()
	return Wording(dg.JobLineLong, "[%[1]d]%[2]s %[3]d %-24[4]s%[5]s",
		i+1, r.jobMarker(j), j.PID, r.jobState(j, false), r.jobCommand(j, showBg))
}

func (r *Runner) jobLineAs(i int, j *Job, showBg, noticing bool) string {
	return Wording(r.diag().JobLine, "[%[1]d]%[2]s  %-24[3]s%[4]s",
		i+1, r.jobMarker(j), r.jobState(j, noticing), r.jobCommand(j, showBg))
}

// jobState is the state column: what a job is doing, worded the dialect's
// way. noticing says the shell is reporting that the job ended rather than
// listing one that has, which one dialect words differently.
func (r *Runner) jobState(j *Job, noticing bool) string {
	dg := r.diag()
	switch {
	case j.Stopped:
		// One verb, the signal that stopped it, which only one dialect names.
		return Wording(dg.JobStopped, "Stopped", j.StopSig)
	case j.Finished():
		state := Wording(dg.JobDone, "Done")
		if noticing && dg.JobDoneNotice != "" {
			state = dg.JobDoneNotice
		}
		if j.Status != 0 && dg.JobExited != "" {
			// A dialect that says something else for a job that failed. One
			// verb, the status, and no dialect words it without one.
			state = Wording(dg.JobExited, "Exit %[1]d", j.Status)
		}
		return state
	}
	return Wording(dg.JobRunning, "Running")
}

// jobNumber is the number a job is listed under: its place in the table,
// which is not its place in a listing that was asked for particular jobs.
func (r *Runner) jobNumber(j *Job) int {
	for i, other := range r.jobs {
		if other == j {
			return i
		}
	}
	return 0
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
	if r.diag().JobNoticeShowsAmpersand && j.Finished() {
		// A different shell and a different moment: one puts the `&` back
		// while the job runs, the other when it reports that it ended.
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
	r.printf("%s\n", r.resumeNotice(j, r.diag().JobResumedInForeground, j.Command))
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
	if w.Killed {
		// What ended it, for the same two readers `runWatched` tells: the
		// prompt starts a fresh line after the `^C` the terminal echoed, and
		// an interrupt gives up the line. A job put back in front is a
		// foreground command again, so it answers both the same way.
		r.diedOfSig = w.Signal
		if w.Signal == syscall.SIGINT && r.Interactive {
			defer r.abandonForInterrupt()
		}
	}
	if stopped {
		// Stopped again, so it stays a job rather than being forgotten — and
		// says so, exactly as the first ^Z did. It is the current job again
		// too: the one `fg` with no operand would pick is the one that just
		// stopped.
		j.Stopped, j.StopSig = true, int(w.Signal)
		r.setLastJob(j)
		r.announceStopped(j)
		return status
	}
	r.Forget(j)
	return status
}

// resumeNotice is what `fg` or `bg` says about the job it resumed.
//
// Three verbs — the number, the marker and the command — and a fallback the
// caller supplies, because the two builtins differ in what a dialect that
// says nothing of its own prints: `fg` names the command alone and `bg` puts
// the `&` back after it.
func (r *Runner) resumeNotice(j *Job, wording, fallback string) string {
	if wording == "" {
		return fallback
	}
	return Wording(wording, "", r.jobNumber(j)+1, r.jobMarker(j), j.Command)
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
	r.printf("%s\n", r.resumeNotice(j, r.diag().JobResumedInBackground, j.Command+" &"))
	return 0
}

// resume finds the job `fg` or `bg` was asked about.
func (r *Runner) resume(args []string, name string) (*Job, int) {
	if !r.JobControl &&
		r.ask(r.sem().JobControlAbsenceIsReportedFirst, "`bg` with no job control refusing before reading its operand") {
		// Two shells notice there is no job control before looking at
		// anything else, so the operand is never named — reporting it as a
		// missing job would claim job control exists.
		r.diagf("%s\n", Wording(r.diag().NoJobControl, "%[1]s: no job control", name))
		return nil, 1
	}
	if r.unspecified {
		return nil, 2
	}
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
// none of it is a dialect question. `%name` and `%?text` resolve by the
// command's text, and are questions — see JobSpecsByName.
func (r *Runner) findJob(spec, name string) (*Job, int) {
	j, code := r.findJobQuietly(spec)
	if code == jobFound {
		return j, 0
	}
	return nil, r.reportJobLookup(spec, code, name)
}

// reportJobLookup is findJob's second half, for a caller that has to say
// something else before the complaint — `jobs %1 %9` lists job 1 and reports
// the bad spec after it, in every shell in the panel.
func (r *Runner) reportJobLookup(spec string, code int, name string) int {
	switch code {
	case jobSpecAmbiguous:
		r.diagf("%s\n", Wording(r.diag().AmbiguousJobSpec,
			"%[1]s: %[2]s: ambiguous job spec", name, strings.TrimPrefix(spec, "%")))
		return 1
	case jobSpecUnanswered:
		return r.status
	}
	d := r.diag()
	r.diagf("%s\n", Wording(d.NoSuchJob, "%[1]s: %[2]s: no such job", name, spec))
	return orDefault(d.NoSuchJobStatus, 1)
}

// signalJob sends to the job's process group rather than to the one process.
//
// The group is the point: a job is a pipeline as often as a command, and
// signaling only the first of three would resume one and leave the rest
// stopped. The negative pid is how the kernel is told to mean the group.
//
// Not gated, unlike the signal `kill` sends, and the difference is what the
// script chose rather than what reaches the kernel. This is job control: a
// fixed SIGCONT, to a job this shell started and therefore already passed the
// exec gate on its way to existing, named by a `%` spec that can name nothing
// else. A policy that does not want that process resumed did not want it
// started, and refusing it here would leave a stopped job with nothing able
// to reach it. `kill -CONT %1` is the script choosing, and that one is gated.
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

// What a job lookup came back with. Codes rather than a boolean because an
// ambiguous name and a missing one are different complaints, and an
// unanswered axis is neither.
const (
	jobFound = iota
	jobMissing
	jobSpecAmbiguous
	jobSpecUnanswered
)

// findJobQuietly is findJob without the complaint, for a caller that words its
// own — `kill %9` is `kill`'s error to report, not this one's.
func (r *Runner) findJobQuietly(spec string) (*Job, int) {
	text := strings.TrimPrefix(spec, "%")
	switch text {
	case "", "%", "+":
		if r.lastJob == nil {
			return nil, jobMissing
		}
		return r.lastJob, jobFound
	case "-":
		if len(r.jobs) < 2 {
			return nil, jobMissing
		}
		return r.jobs[len(r.jobs)-2], jobFound
	}
	n, ok := atoi(text)
	if !ok {
		return r.findJobByName(text)
	}
	if n < 1 || n > len(r.jobs) {
		return nil, jobMissing
	}
	return r.jobs[n-1], jobFound
}

// findJobByName resolves `%name` — the job whose command begins with the
// text — and `%?text`, the one whose command contains it.
func (r *Runner) findJobByName(text string) (*Job, int) {
	if !r.ask(r.sem().JobSpecsByName, "a job named by its command — `%name`") {
		if r.unspecified {
			return nil, jobSpecUnanswered
		}
		// Every such spec is a job that is not there, which is the measured
		// answer of the shell that resolves only numbers here.
		return nil, jobMissing
	}
	contains := strings.HasPrefix(text, "?")
	pattern := strings.TrimPrefix(text, "?")
	var matches []*Job
	for _, j := range r.jobs {
		if (contains && strings.Contains(j.Command, pattern)) ||
			(!contains && strings.HasPrefix(j.Command, pattern)) {
			matches = append(matches, j)
		}
	}
	switch len(matches) {
	case 0:
		return nil, jobMissing
	case 1:
		return matches[0], jobFound
	}
	// A second match is the disagreement: refused as ambiguous, or the most
	// recent match taken.
	if r.ask(r.sem().AmbiguousJobNameIsRefused, "`%name` matching more than one job being refused") {
		return nil, jobSpecAmbiguous
	}
	if r.unspecified {
		return nil, jobSpecUnanswered
	}
	return matches[len(matches)-1], jobFound
}
