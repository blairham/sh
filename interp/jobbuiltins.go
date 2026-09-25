// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"slices"
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
// an exiting interactive shell would send, so its listing keeps the job. dash
// has no disown at all, and unregisters it. See
// Semantics.DisownRemovesTheJob.
//
// The shield is still nothing to do here, and the reason moved rather than
// went away. This engine does send that HUP since #4509 — see
// hangUpJobsIfAsked — but only where Runner.SendsHangupToJobsAtExit is on,
// and the one dialect whose disown *shields* rather than removes is ksh93,
// which has no option to turn it on with. So the shielding branch is reached
// only by a shell that was never going to signal anything.
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
	if r.ask(r.sem().DisownAlwaysFails, "`disown` answering 1 for every call and saying nothing") {
		// One column's whole answer, and it is not a report about the
		// lookup: the job it can list is still listed afterwards. Asked
		// after the option words so that a bad letter is still that shell's
		// `unknown option`. See Semantics.DisownAlwaysFails.
		return 1
	}
	if r.unspecified {
		return r.status
	}
	var jobs []*Job
	if len(args) == 0 {
		// The current job, and with none the complaint is the dialect's —
		// or, in one shell, a bare failing status. Empty means silence: the
		// engine measured saying nothing says nothing here on purpose.
		current := r.currentJob()
		if current == nil {
			if w := r.diag().DisownNoCurrentJob; w != "" {
				r.diagf("%s\n", w)
			}
			return 1
		}
		jobs = []*Job{current}
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
		// Shielded from a HUP this shell was not going to send anyway —
		// the only dialect that takes this branch has no option that turns
		// the sending on. Nothing to do, and saying so would be inventing
		// output.
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
	changedOnly, code := r.jobsChangedFilter(opts)
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
	if changedOnly {
		jobs = r.jobsThatChangedSinceTheyWereReported(jobs)
	}
	// Whatever a listing showed, it has now said it — which is what the next
	// `-n` listing is measured against. Over the jobs that were *looked at*
	// rather than the ones printed, the rule the forgetting below follows and
	// for the same reason: a filter the dialect applied is still the shell
	// having looked.
	r.markJobsReported(jobs)
	if code := r.printJobs(jobs, form, wanted, explicit, jobsShowsDirectory(opts)); code != 0 {
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
	// A listing that was not a listing of states does not count — in three
	// of the four. Measured: `jobs -p` and `jobs -r` in bash both leave the
	// finished job for the next bare `jobs` to report, where `jobs` and
	// `jobs -l` consume it. So only a form that showed the job's state, and
	// only with nothing filtered out, finishes with it.
	//
	// ksh93 is the fourth, and it finishes with the job whichever form asked:
	// `jobs -p` there prints the process id once and the next `jobs` shows
	// nothing, where dash and bash print the id and then still report the
	// job as Done. We followed the two that agree, which was defensible and
	// unrecorded — so it read as an accident rather than as a choice (#602).
	if wanted != anyJobState {
		return 0
	}
	finished := false
	for _, j := range jobs {
		if j.Finished() {
			finished = true
			break
		}
	}
	if !finished {
		// Nothing to finish with, so no dialect is questioned: a `jobs -p`
		// over running jobs answers the same way in every column.
		return 0
	}
	if form == jobsPidsAlone &&
		!r.ask(r.sem().PidListingFinishesWithAJob, "`jobs -p` finishing with a job the way a state listing does") {
		return 0
	}
	// Over a copy, because Forget compacts the table in place and a bare
	// listing was handed the table itself: dropping one element shifts every
	// element after it down, so the walk skipped the job that moved into the
	// slot it had just left. With three finished jobs the middle one survived
	// and was reported a second time by the next listing — under a number the
	// shell had told nobody about, once numbers stopped being positions.
	for _, j := range slices.Clone(jobs) {
		if j.Finished() {
			r.Forget(j)
		}
	}
	return 0
}

// jobsShowsDirectory reads `-d`, which adds a line naming the directory each
// listed job was started in.
//
// Read rather than asked, and the letter set is the whole of the dialect's
// answer. The other five columns do not have the letter at all — measured
// 2026-09-25, `sleep 3 & jobs -d` is `invalid option` in bash 5.3.15 and
// 3.2.57, `unknown option` in ksh93u+, `Illegal option -d` in dash, and
// `illegal option -d` in BusyBox ash 1.37.0 inside the pinned alpine image —
// so there is no second reading of the letter for an axis to switch between.
// That is the difference from `jobs -n`, which is an axis precisely because
// bash holds the same letter and means something else by it. See
// Semantics.JobsOptions, which is what lets the letter through at all.
func jobsShowsDirectory(opts string) bool {
	return strings.ContainsRune(opts, 'd')
}

// printJobs writes the listing itself, once the form, the filter and the set
// of jobs are settled.
//
// showDir is `-d`: a line of its own under each job's row naming where that
// job was started. Under the row rather than in it — the row is the ordinary
// state row and `-d` composes with `-l` and with the state filters rather
// than replacing anything.
func (r *Runner) printJobs(jobs []*Job, form jobsForm, wanted jobState, explicit, showDir bool) int {
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
			// Job.Ident rather than Job.PID, which is what makes this row
			// printable at all for a job with no process of its own. It used
			// to be left out: a real shell forks and has an id to print, and
			// this shell had 0 — a number `kill $(jobs -p)` spends as *every
			// process in the shell's group*. The invented number is out above
			// every id a kernel can issue and is read back here as the job it
			// names, so the listing can say what it always said. See
			// jobident.go.
			r.printf("%d\n", row.job.Ident())
		case jobsLongRow:
			r.printf("%s\n", r.jobLineLong(row.n, row.job, showBg))
		default:
			r.printf("%s\n", r.jobLine(row.n, row.job, showBg))
		}
		if showDir {
			r.printf("%s\n", r.jobDirectoryLine(row.job))
		}
	}
	return 0
}

// jobDirectoryLine is the line `jobs -d` adds under a job's row.
//
// The directory is written the way a prompt writes one — the home directory
// as `~` — and that shortening is read at *print* time rather than recorded
// with the job: measured 2026-09-25 on zsh 5.9.2, a job started under the
// home directory and then listed after `HOME` was assigned somewhere else is
// named by its full path, so what is stored is the path and what is decided
// here is how to spell it. abbreviateHome is the prompt's own, shared rather
// than written twice.
func (r *Runner) jobDirectoryLine(j *Job) string {
	dir := abbreviateHome(j.Dir, r.promptVar("HOME"))
	return Wording(r.diag().JobDirectoryLine, "(pwd : %[1]s)", dir)
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

// jobsChangedFilter reads `-n`, which is not a state filter but a filter on
// what the shell has already said.
//
// The letter reaches this only where the dialect has it — `JobsOptions` is
// what lets it through builtinOptions at all — and the axis is asked there
// rather than at the letter set, because a second dialect could take the
// letter and mean something else by it. bash does exactly that.
func (r *Runner) jobsChangedFilter(opts string) (bool, int) {
	if !strings.ContainsRune(opts, 'n') {
		return false, 0
	}
	if r.ask(r.sem().JobsListsWhatChangedSinceTheLastReport,
		"`jobs -n` listing only what changed since the shell last said so") {
		return true, 0
	}
	if r.unspecified {
		return false, 2
	}
	return false, 0
}

// jobsThatChangedSinceTheyWereReported keeps the jobs whose state has moved
// since the shell last said anything about them.
//
// The monitor is the gate and that is measured rather than tidy. A shell with
// nobody to tell does not notice a job end: ksh93's bare `jobs` in a script
// calls a job that has already exited `Running`, so by the time `jobs -n` is
// asked nothing has changed as far as that shell knows, and it prints nothing
// at status 0. This engine reaps on every listing and so would have the ending
// in hand, which is exactly why the noticing has to be modeled here instead of
// falling out of the table.
//
// A job that is still running is not a change however new it is. That is the
// same measurement from the other side: the first `jobs -n` after two jobs
// were started prints only the one that ended. bash's letter of the same name
// counts a job that has only just started, which is why the two readings are
// not one implementation.
func (r *Runner) jobsThatChangedSinceTheyWereReported(jobs []*Job) []*Job {
	if !r.monitor {
		return nil
	}
	changed := make([]*Job, 0, len(jobs))
	for _, j := range jobs {
		if j.reportedState != j.reportState() {
			changed = append(changed, j)
		}
	}
	return changed
}

// markJobsReported records what the shell has just said about each job, which
// is what the next `-n` listing is measured against.
func (r *Runner) markJobsReported(jobs []*Job) {
	for _, j := range jobs {
		j.reportedState = j.reportState()
	}
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
		// The job's own number, which is neither its place in this listing
		// nor its place in the table. `jobs %2` printed `[1]` before this,
		// because a listing of one job counted from the start of the slice it
		// had been handed.
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
	return r.jobLineLongAs(i, j, showBg, false)
}

// jobLineLongAs is that row with the same noticing question jobLineAs takes:
// `-l` is one way to reach it and Semantics.JobNoticeNamesThePID is the other,
// and the second of those renders a job that has *ended*, where the one
// dialect with a separate notice word for it wants that word.
func (r *Runner) jobLineLongAs(i int, j *Job, showBg, noticing bool) string {
	dg := r.diag()
	return Wording(dg.JobLineLong, "[%[1]d]%[2]s %[3]d %-24[4]s%[5]s",
		i, r.jobMarker(j), j.Ident(), r.jobState(j, noticing), r.jobCommand(j, showBg))
}

func (r *Runner) jobLineAs(i int, j *Job, showBg, noticing bool) string {
	return Wording(r.diag().JobLine, "[%[1]d]%[2]s  %-24[3]s%[4]s",
		i, r.jobMarker(j), r.jobState(j, noticing), r.jobCommand(j, showBg))
}

// jobNoticeLine is the row a *notice* is made of — what the shell says about
// a job on its own initiative, and what `fg` and `bg` say about the job they
// named.
//
// The long row where the dialect names the pid in a notice and the listing's
// own row where it does not. See Semantics.JobNoticeNamesThePID: one dialect
// has an option that moves it between one job and the next, so both renderings
// have to be reachable from one vector rather than chosen when it is built.
func (r *Runner) jobNoticeLine(i int, j *Job, showBg, noticing bool) string {
	if r.namesThePIDInANotice() {
		return r.jobLineLongAs(i, j, showBg, noticing)
	}
	return r.jobLineAs(i, j, showBg, noticing)
}

// namesThePIDInANotice reads the axis rather than asking it, which is the
// arrangement EndedJobIsListedAsRunningWithoutTheMonitor uses and for the same
// reason: a dialect that has not answered writes the short form the whole
// panel writes, and a complaint in the middle of a notice would land where
// nobody asked a question.
func (r *Runner) namesThePIDInANotice() bool {
	return r.sem().JobNoticeNamesThePID == Yes
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
		if !r.monitor && r.sem().EndedJobIsListedAsRunningWithoutTheMonitor == Yes {
			// One column reaps a `&` job only under the monitor, so with no
			// monitor its listing still says the job is running. Read
			// directly rather than asked, because a shell with no dialect at
			// all has no such blind spot to reproduce and a complaint here
			// would land in the middle of a listing.
			return Wording(dg.JobRunning, "Running")
		}
		state := Wording(dg.JobDone, "Done")
		if noticing && dg.JobDoneNotice != "" {
			state = dg.JobDoneNotice
		}
		if j.EndSig != 0 && dg.JobSignaled != "" {
			// A dialect that names the signal that ended the job. Asked
			// before the status below, because a signal death has a status
			// too and the two wordings would otherwise both apply — see
			// Diagnostics.JobSignaled, and Job.EndSig for why the signal is
			// carried rather than read back out of that status.
			sig := syscall.Signal(j.EndSig)
			return Wording(dg.JobSignaled, "Killed", r.signalDescription(sig), j.EndSig)
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

// jobNumber is the number a job is listed under and named by.
//
// The job's own, assigned when it entered the table — not its place in the
// table, and not its place in a listing that was asked for particular jobs.
// See Runner.addJob for why the difference is load-bearing.
func (r *Runner) jobNumber(j *Job) int { return j.num }

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
	current, previous := r.markedJobs()
	switch j {
	case current:
		return "+"
	case previous:
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
	r.printf("%s\n", r.resumeNotice(j, r.diag().JobResumedInForeground, j.Command, r.resumeState(j)))
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
	if !j.polled || j.PID == 0 {
		// A job somebody is already waiting on, which is every `&` job and
		// every job with no process of its own. Waiting on the pid a second
		// time here is not a second answer, it is an error: the job's own
		// goroutine has the child, so this reaped nothing and reported `fg:
		// no child processes` where the panel runs the job and reports its
		// status (#2720). Before `set -m` reached this builtin the only job
		// that could be resumed was one ^Z had left behind, which is polled
		// and has nobody waiting on it, so the shape never came up.
		return r.finishResumed(j)
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
		r.becomeCurrentJob(j)
		r.announceStopped(j)
		return status
	}
	r.Forget(j)
	return status
}

// finishResumed waits out a job `fg` put back in front that this shell is not
// the waiter for, and reports what it left.
//
// The channel and not the front end's wait hook, for the reason biFg gives at
// the call: the job's own goroutine holds the child. It is the same pair of
// channels `wait` selects over — see Runner.waitFor — minus the axis, because
// giving up on a stopped job is a question about `wait` and not about `fg`: a
// job that stops again under `fg` is announced and handed back in every shell
// that has the builtin at all.
//
// What it does not carry is Runner.diedOfSig. The signal that ended the job
// was seen on the job's own runner, which is a copy of this one and is gone by
// the time the channel closes, so a job killed while in front does not start
// the next prompt on a fresh line the way a foreground command does. The
// polled path above still does, which is why it is still there; recorded here
// rather than guessed at from the status, since 130 is also what `exit 130`
// leaves.
func (r *Runner) finishResumed(j *Job) int {
	note := j.stopNote
	if !r.monitor || j.PID == 0 {
		// Nothing that can stop, or no monitor to stop it with — the same
		// pair of conditions stoppedJobEndsAWait reads, and for the same
		// reason. A nil channel is never ready, so the select below becomes
		// a plain wait.
		note = nil
	}
	select {
	case <-j.done:
	case <-note:
		// Ended and stopped at once is ended, which is the rule awaitOrTrap
		// states for the same pair.
		select {
		case <-j.done:
		default:
			if r.noticeStoppedJob(j) {
				r.setLastJob(j)
				r.becomeCurrentJob(j)
				r.announceStopped(j)
				return r.stoppedWaitStatus(j)
			}
			<-j.done
		}
	}
	status := j.Status
	r.Forget(j)
	return status
}

// resumeNotice is what `fg` or `bg` says about the job it resumed.
//
// Four verbs — the number, the marker, the command and the state — and a
// fallback the caller supplies, because the two builtins differ in what a
// dialect that says nothing of its own prints: `fg` names the command alone
// and `bg` puts the `&` back after it.
func (r *Runner) resumeNotice(j *Job, wording, fallback, state string) string {
	if r.namesThePIDInANotice() {
		// The long row, with the resume's own state word in the column a
		// listing would have put `running` or `suspended` in. Built from
		// JobLineLong rather than from a second pair of resume wordings,
		// because that is what it is: measured, `fg` with the option on
		// prints `[1]  + 63526 running    sleep 3` and `bg` prints
		// `[1]  + 258 continued  sleep 5`, which is the `jobs -l` row and
		// the resume's verb (#4491).
		return Wording(r.diag().JobLineLong, "[%[1]d]%[2]s %[3]d %-24[4]s%[5]s",
			r.jobNumber(j), r.jobMarker(j), j.Ident(), state, j.Command)
	}
	if wording == "" {
		return fallback
	}
	return Wording(wording, "", r.jobNumber(j), r.jobMarker(j), j.Command, state)
}

// resumeState is the state word a resume notice carries, in the one dialect
// whose notice is a listing row rather than a sentence.
//
// `continued` where the job was actually continued, and the job's own state
// where it was already doing it — measured, zsh's `fg` on a running job
// prints the row `jobs` would print, `running` and all, and only a job it
// had to send a continue to is called continued. The state is read before
// either builtin clears Job.Stopped, because that flag is the whole of the
// question.
func (r *Runner) resumeState(j *Job) string {
	if j.Stopped {
		return Wording(r.diag().JobContinued, "continued")
	}
	return r.jobState(j, false)
}

func biBg(r *Runner, _ context.Context, args []string) int {
	j, code := r.resume(args, "bg")
	if code != 0 {
		return code
	}
	// A job that was never stopped is already doing what `bg` would ask of
	// it, and two of the panel say so rather than sending a continue to a
	// process that is running and announcing it as resumed. The other three
	// resume it and print the usual notice — see
	// Diagnostics.JobAlreadyInBackground, which is empty for those.
	if !j.Stopped {
		if w := r.diag().JobAlreadyInBackground; w != "" {
			r.diagf("%s\n", Wording(w, "", "bg", r.jobNumber(j)))
			return r.diag().JobAlreadyInBackgroundStatus
		}
	}
	// Read while Job.Stopped still says what the job was doing, which is what
	// the state word is about.
	state := r.resumeState(j)
	if err := r.signalJob(j, syscall.SIGCONT); err != nil {
		r.diagf("bg: %v\n", err)
		return 1
	}
	j.Stopped = false
	// `&` after it, which is what says the shell is not waiting.
	r.printf("%s\n", r.resumeNotice(j, r.diag().JobResumedInBackground, j.Command+" &", state))
	return 0
}

// resume finds the job `fg` or `bg` was asked about.
//
// Or refuses, which is what happens whenever this shell has no job control.
// Measured from a script with no terminal: every member of the panel refuses,
// so the divergence is in when that is said and what is said, and never in
// whether the job runs. This shell used to take it as a live request, print
// the job's command line on stdout the way an interactive `fg` does, and fail
// afterwards — which put a line of output in a script that had written
// `fg 2>/dev/null` precisely so there would be none (#2657).
func (r *Runner) resume(args []string, name string) (*Job, int) {
	first := false
	if !r.canResume() {
		first = r.ask(r.sem().JobControlAbsenceIsReportedFirst, "`bg` with no job control refusing before reading its operand")
	}
	if r.unspecified {
		return nil, 2
	}
	if first {
		// Three shells notice there is no job control before looking at
		// anything else, so the operand is never named — reporting it as a
		// missing job would claim job control exists. One of the three
		// refuses without a word; see Diagnostics.NoJobControl.
		if w := r.diag().NoJobControl; w != "" {
			r.diagf("%s\n", Wording(w, "", name))
		}
		return nil, 1
	}
	j, code := r.pickJob(args, name)
	if j == nil {
		return nil, code
	}
	// What the job's own goroutine already saw, taken before either builtin
	// reads Job.Stopped — because everything below turns on that flag, and
	// until something looks it says what the job was doing at the last
	// listing rather than what it is doing now.
	//
	// A job stopped from outside the shell — `kill -STOP %1`, another
	// terminal, a debugger — is the shape that showed it. `jobs` takes the
	// note as part of reaping and `fg` never did, so the same script with a
	// `jobs` line in it and without answered differently: with it, `fg`
	// continued the job and waited it out; without, it thought the job was
	// running, printed the wrong state word in the dialect whose notice
	// carries one, and then read the stale note in finishResumed and
	// reported 128+SIGSTOP without waiting at all (#2838).
	//
	// Idempotent, so this is not a second reaper: the note is taken once and
	// a job the script has since resumed is not put back to stopped.
	r.noticeStoppedJob(j)
	if !r.canResume() {
		// The operand read first and nothing wrong with it, which leaves the
		// refusal this shell had all along. dash names the spec here, and
		// names it `(null)` where there was none.
		d := r.diag()
		spec := d.AbsentJobSpec
		if len(args) > 0 {
			spec = args[0]
		}
		r.diagf("%s\n", Wording(d.JobNotUnderJobControl, "%[1]s: no job control", name, spec))
		return nil, orDefault(d.JobNotUnderJobControlStatus, 1)
	}
	if j.PID != 0 && !j.ownGroup {
		// A job started while the monitor was off runs in the shell's own
		// process group, so there is no group of its own to put in front of
		// the terminal. Every column that can reach the question refuses it
		// rather than resuming — see Diagnostics.JobStartedWithoutJobControl.
		//
		// After the job control check above and not before it: with the
		// monitor still off the answer is that there is no job control at
		// all, and naming the job here would claim there is. The state this
		// catches is the one in between — a job started with the monitor
		// off, in a shell that has since turned it on.
		//
		// Asked of the process and not of the job, because a job with no
		// process of its own has no group either way and is not refused:
		// `set -m; { sleep 4; } & fg` resumes in bash exactly as it does
		// here.
		//
		// This is #3020. Without it the job was resumed and waited out,
		// which is how a suite file bash finishes in 11 ms cost 30 seconds.
		d := r.diag()
		r.diagf("%s\n", Wording(d.JobStartedWithoutJobControl,
			"%[1]s: job %[2]d started without job control", name, r.jobNumber(j)))
		return nil, orDefault(d.JobStartedWithoutJobControlStatus, 1)
	}
	return j, 0
}

// canResume reports whether this shell will run a job for `fg` or `bg`.
//
// The monitor and having somebody to announce jobs to are different states,
// and four of the five columns resume on the first alone — see
// Semantics.MonitorAloneResumesAJob for the table and for the one that does
// not. This gate used to be Runner.JobControl in every dialect, which is a
// prompt and nothing else, so `set -m` in a script granted the monitor and
// `fg` still refused (#2720).
func (r *Runner) canResume() bool {
	if r.sem().MonitorAloneResumesAJob == Yes {
		return r.monitor
	}
	return r.JobControl
}

// pickJob is the operand half of resume: the job an argument names, or the
// current one where there is no argument.
func (r *Runner) pickJob(args []string, name string) (*Job, int) {
	if len(args) == 0 {
		current := r.currentJob()
		if current == nil {
			d := r.diag()
			r.diagf("%s\n", Wording(d.NoCurrentJob, "%[1]s: no current job", name))
			return nil, orDefault(d.NoCurrentJobStatus, 1)
		}
		return current, 0
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

// signalJob sends to every process the job is made of.
//
// Every one, because a job is not one process: a backgrounded pipeline is as
// many as it has elements, and `fg`, `bg` and `kill %1` all mean the job
// rather than whichever of its processes happened to settle its pid. See
// Job.took.
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
	procs := j.processes()
	if len(procs) == 0 {
		// A job with no process of its own — a builtin or a compound command
		// running on a cloned runner. There is nothing to signal, and saying
		// so is better than signaling something else.
		return errNoJobProcess
	}
	sent, last := 0, error(nil)
	for _, p := range procs {
		if err := r.signalJobProcess(p, sig); err != nil {
			last = err
			continue
		}
		sent++
	}
	if sent == 0 {
		return last
	}
	// One member already gone is not a failure to resume the job: what `fg`
	// and `bg` are asking for is that the job runs on, and it does.
	return nil
}

// signalJobProcess sends to one of the processes a job is made of.
func (r *Runner) signalJobProcess(p jobProcess, sig syscall.Signal) error {
	if !p.ownGroup {
		// The process runs in *this shell's* group — anything started with
		// the monitor off — so the group is not the job's to signal: aiming
		// at it would reach the shell, every other job it started and, on a
		// terminal, the whole foreground group. The process is the only
		// honest target (#1738).
		//
		// Through killProcess, so this signal passes the gate and reaches the
		// event stream. The group below does not, and the difference is
		// deliberate rather than an oversight: that is the embedder's own hook
		// being called, where this is the interpreter asking the kernel
		// itself, and every signal *this* package delivers is visible to the
		// boundary — see interp/signalgate.go.
		return r.killProcess(p.pid, sig)
	}
	if r.SignalGroup == nil {
		return errNoJobProcess
	}
	return r.SignalGroup(p.pid, sig)
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
		current := r.currentJob()
		if current == nil {
			return nil, jobMissing
		}
		return current, jobFound
	case "-":
		// The runner-up rather than the job before this one in the table,
		// which is the same choice the listing's `-` is written from — see
		// markedJobs. Measured, the two agree in every column: `jobs %-`
		// names exactly the job a listing puts `-` on.
		_, previous := r.markedJobs()
		if previous == nil {
			return nil, jobMissing
		}
		return previous, jobFound
	}
	n, ok := atoi(text)
	if !ok {
		return r.findJobByName(text)
	}
	for _, j := range r.jobs {
		if j.num == n {
			return j, jobFound
		}
	}
	return nil, jobMissing
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
