// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// `kill`, which was not a builtin either — and this one was not merely
// borrowed, it was the reason a corpus case could not be trusted.
//
// Every shell in the panel implements `kill` itself, and POSIX lists it as a
// regular builtin, for a reason that has nothing to do with convenience: a
// shell has to know about the signals it sends. `kill -INT $$` is the shell
// aiming a signal at itself, and when /bin/kill does the aiming the shell
// finds out the way any process does — asynchronously, whenever the runtime
// gets round to saying so.
//
// In Go that is a goroutine, and a goroutine is a race. os/signal catches the
// signal and forwards it to a channel from a goroutine of its own, so a drain
// that only reads the channel is asking a question whose answer depends on
// the scheduler. Measured on a busy machine: three runs in three hundred got
// it wrong, one running the handler a command late and two never running it
// at all, against zero failures in the same three hundred runs on an idle
// one. That is not a flaky test, it is a shell that can lose a trapped
// signal, and the corpus was right to keep noticing.
//
// A builtin closes it because kill(2) delivers an unblocked signal before it
// returns. The shell does not have to be told what it has just done, so the
// arrival is recorded at the point of sending — see selfSignaled — and the
// runtime's later copy is discarded. The signal is still sent for real, so
// nothing else that was watching for it misses out.
//
// Four things are deliberately left out, and none of them worked before
// either — /bin/kill could not do them, so nothing that worked has stopped:
//
//   - Job specs. `kill %1` needs job control rather than signaling.
//   - The bare `kill -l` listing. Four formats over a signal table that is
//     not the same on two operating systems, and this repository's corpus is
//     graded on Linux as well as macOS, so a case recording one machine's
//     table could not pass on both. The translating forms are here and are
//     what the panel agrees on.
//   - dash's refusal of SIG-prefixed names. `trap … SIGINT` is a bad trap to
//     dash exactly as `kill -SIGINT` is a bad option, so this belongs to
//     `trap` as much as to `kill`; an axis answering it for one and not the
//     other would be worse than the gap.
//   - dash's refusal of `-n`. The other three take it.

func init() {
	builtins["kill"] = biKill
}

// knownSignals is what `kill` can send, which is a longer list than what
// `trap` can catch: KILL and STOP are perfectly good things to send and
// cannot be caught by anyone.
//
// The numbers come from the host's own constants rather than a table written
// here, because they are not the same everywhere — 7 is EMT on a BSD and BUS
// on Linux. Only the signals both agree exist are listed, which is why SIGEMT
// and SIGINFO are missing on the machine that has them.
type signalEntry struct {
	Name string
	Sig  syscall.Signal
	// Fatal is whether the default action ends the process, which is what
	// the shell needs to know about a signal it has just sent itself and has
	// no handler for: the script stops there rather than running one more
	// command in a process the kernel has already been told to end.
	//
	// The stopping signals are deliberately not fatal and neither is IO,
	// whose default is to end the process on Linux and to be ignored on a
	// BSD. Getting that one wrong in the safe direction costs an ordering;
	// getting it wrong in the other direction would stop a script that was
	// not going to die.
	Fatal bool
}

var knownSignals = []signalEntry{
	{"HUP", syscall.SIGHUP, true},
	{"INT", syscall.SIGINT, true},
	{"QUIT", syscall.SIGQUIT, true},
	{"ILL", syscall.SIGILL, true},
	{"TRAP", syscall.SIGTRAP, true},
	{"ABRT", syscall.SIGABRT, true},
	{"FPE", syscall.SIGFPE, true},
	{"KILL", syscall.SIGKILL, true},
	{"BUS", syscall.SIGBUS, true},
	{"SEGV", syscall.SIGSEGV, true},
	{"SYS", syscall.SIGSYS, true},
	{"PIPE", syscall.SIGPIPE, true},
	{"ALRM", syscall.SIGALRM, true},
	{"TERM", syscall.SIGTERM, true},
	{"URG", syscall.SIGURG, false},
	{"STOP", syscall.SIGSTOP, false},
	{"TSTP", syscall.SIGTSTP, false},
	{"CONT", syscall.SIGCONT, false},
	{"CHLD", syscall.SIGCHLD, false},
	{"TTIN", syscall.SIGTTIN, false},
	{"TTOU", syscall.SIGTTOU, false},
	{"IO", syscall.SIGIO, false},
	{"XCPU", syscall.SIGXCPU, true},
	{"XFSZ", syscall.SIGXFSZ, true},
	{"VTALRM", syscall.SIGVTALRM, true},
	{"PROF", syscall.SIGPROF, true},
	{"WINCH", syscall.SIGWINCH, false},
	{"USR1", syscall.SIGUSR1, true},
	{"USR2", syscall.SIGUSR2, true},
}

// knownSignal reports whether a bare, uppercased name is one of the signals
// this shell can send.
func knownSignal(name string) bool {
	for _, k := range knownSignals {
		if k.Name == name {
			return true
		}
	}
	return false
}

func biKill(r *Runner, _ context.Context, args []string) int {
	if len(args) == 0 {
		return r.killReport(killUsage, "")
	}
	if args[0] == "-l" {
		return r.killList(args[1:])
	}
	name, sig, rest, err := r.killSignal(args)
	if r.unspecified {
		return r.status
	}
	if err != nil {
		return r.killFailed(err)
	}
	if len(rest) == 0 {
		return r.killReport(killUsage, "")
	}
	return r.killTargets(name, sig, rest)
}

// killSignal reads the leading options and returns the signal to send.
//
// Both spellings mean the same thing and neither is the odd one out: `-s INT`
// is the POSIX form and `-INT` is the one everybody types. `-n` takes a
// number and is the only option here dash does not have.
//
// The signal may also be written *onto* the option with no space — `kill -n9`
// — which is its own reading rather than a spelling of the one above: see
// joinedKillSignal, and Semantics.KillReadsASignalJoinedToItsOption for which
// dialects have it.
func (r *Runner) killSignal(args []string) (string, syscall.Signal, []string, error) {
	spec, form := "TERM", killSpecBare
	joined, isJoined := joinedKillSignal(args[0])
	switch a := args[0]; {
	case a == "--":
		args = args[1:]
	case a == "-s" || a == "-n":
		if len(args) < 2 {
			return "", 0, nil, &killError{kind: killMissingSignalArgument, operand: a}
		}
		spec, form, args = args[1], killSpecOption, args[2:]
	case isJoined && r.ask(r.sem().KillReadsASignalJoinedToItsOption,
		"a signal written onto `kill -n` or `kill -s` with no space"):
		spec, form, args = joined, killSpecOption, args[1:]
	case strings.HasPrefix(a, "-") && len(a) > 1:
		if r.unspecified {
			// The axis above went unanswered, and this word is exactly the
			// one it is about. Saying anything further about the signal
			// would be answering a question this shell has just refused.
			return "", 0, nil, nil
		}
		spec, form, args = a[1:], killSpecFlag, args[1:]
	}
	name, sig, err := r.signalSpec(spec, form)
	if err != nil {
		return "", 0, nil, err
	}
	return name, sig, args, nil
}

// joinedKillSignal is the signal written onto `-n` or `-s` with no space
// between, and whether the word is that at all.
//
// Which way round the digits go is the whole of the reading, and it is
// measured rather than assumed. `-n` joins a *number* and `-s` joins a
// *name*: `kill -n9` sends signal 9 and `kill -sKILL` sends SIGKILL, while
// `kill -nKILL` and `kill -s9` are neither — they fall through to the bare
// `-SPEC` form and are refused as the signals `nKILL` and `s9`, which is what
// bash 5.3.15 calls them. The two that *are* read report a bad spec without
// the letter, which is how the reading shows in a diagnostic: `kill -n99` is
// `kill: 99: invalid signal specification` there and `kill: n99:` in the
// build that does not read it.
//
// A number reaching `-n` may be 0, the existence probe, which is why this
// tests for digits rather than for a signal: `kill -n0` is status 0 and no
// signal, exactly as `kill -0` is. The emptiness allDigits answers `true` for
// cannot arrive — a word of two characters is not this shape.
func joinedKillSignal(word string) (string, bool) {
	if len(word) < 3 || word[0] != '-' {
		return "", false
	}
	rest := word[2:]
	switch word[1] {
	case 'n':
		return rest, allDigits(rest)
	case 's':
		return rest, !allDigits(rest)
	}
	return "", false
}

// killSpecForm is how the signal was spelled, which two dialects report
// differently: dash and ksh93 call an unrecognized `-Q` an unknown *option*
// and an unrecognized `-s Q` an unknown *signal*, where bash and zsh use one
// message for both and set the two wordings to the same string.
type killSpecForm int

const (
	killSpecBare killSpecForm = iota
	killSpecOption
	killSpecFlag
)

// signalSpec resolves a name or a number to the signal it names.
//
// Signal 0 is not a signal: it is the existence probe, and every shell in the
// panel takes `kill -0 pid` as "is this process there".
func (r *Runner) signalSpec(spec string, form killSpecForm) (string, syscall.Signal, error) {
	bad := func() (string, syscall.Signal, error) {
		kind := killInvalidSignal
		if form == killSpecFlag {
			kind = killIllegalOption
		}
		return "", 0, &killError{kind: kind, operand: spec}
	}
	if n, err := strconv.Atoi(spec); err == nil {
		if n == 0 {
			return "", 0, nil
		}
		for _, k := range knownSignals {
			if int(k.Sig) == n {
				return k.Name, k.Sig, nil
			}
		}
		return bad()
	}
	up := strings.ToUpper(spec)
	if trimmed, had := strings.CutPrefix(up, "SIG"); had && knownSignal(trimmed) {
		// Whether the prefix is part of a name is the dialect's answer, the
		// same one `trap` asks — dash reads `SIGCONT` as neither a signal nor
		// an option and says so twice over. Asked only where it decides
		// something: `SIGNOPE` names nothing either way.
		if !r.ask(r.sem().SIGPrefixAccepted, "the SIG prefix on a signal name") {
			return bad()
		}
		up = trimmed
	}
	for _, k := range knownSignals {
		if k.Name == up {
			return k.Name, k.Sig, nil
		}
	}
	return bad()
}

// killTargets signals each operand, reporting each failure as it happens.
//
// A word that is not a number is not a target at all, and every shell treats
// that as an error with the argument rather than a target that failed — so it
// stops here, where a process that has already exited does not.
func (r *Runner) killTargets(name string, sig syscall.Signal, targets []string) int {
	sent, failed := 0, 0
	for _, t := range targets {
		aims, bad := r.killTarget(t)
		switch bad {
		case jobSpecUnanswered:
			return r.status
		case jobSpecAmbiguous:
			r.diagf("%s\n", Wording(r.diag().AmbiguousJobSpec,
				"%[1]s: %[2]s: ambiguous job spec", "kill", strings.TrimPrefix(t, "%")))
			return orDefault(r.diag().KillArgumentStatus, 1)
		case jobMissing:
			// A `%` spec that resolves to nothing is its own complaint, not
			// a malformed pid.
			return r.killReport(killNoSuchJob, t)
		case killTargetNotAPid:
			return r.killReport(killNotAPid, t)
		}
		// One operand, however many processes it named: the operand is what
		// `kill` reports on, so a job whose pipeline has already lost a
		// member is a job that was signaled and not a target that failed.
		hit, miss := 0, error(nil)
		for _, aim := range aims {
			if aim.ownGroup {
				// The process leads a group, so the signal goes to all of it.
				// Through the same hook `fg` and `bg` use, so that "a job is
				// a group" is said in one place rather than two that could
				// drift.
				if err := r.signalGroupPid(aim.pid, sig); err != nil {
					miss = err
					continue
				}
				hit++
				continue
			}
			if r.stoppedBySignal {
				// This shell has just ended itself. Nothing after the signal
				// runs, including the rest of these targets, and the status
				// is the one signalDeath settled — returned rather than only
				// assigned, because the dispatcher takes what a builtin
				// returns as the command's status.
				return r.status
			}
			if err := r.sendSignal(aim.pid, name, sig); err != nil {
				miss = err
				continue
			}
			hit++
		}
		if r.stoppedBySignal {
			return r.status
		}
		if hit == 0 {
			failed++
			r.killReport(killFailureKind(miss), t)
			continue
		}
		sent++
	}
	if r.stoppedBySignal {
		return r.status
	}
	return r.killStatus(sent, failed)
}

// killFailureKind is what a send that failed is reported as: a target that is
// not there, or one that is there and is not ours.
//
// One function for both routes because a group and a process fail the same
// way, and because a gate's refusal arrives here as EPERM and has to land in
// the second — a refused target reads exactly as a target the kernel would
// not let us have. See signalgate.go for why that is the bargain.
func killFailureKind(err error) killErrorKind {
	if errors.Is(err, syscall.EPERM) {
		return killNotPermitted
	}
	return killNoSuchProcess
}

// killTargetNotAPid is killTarget's own failure code, past the job lookup's:
// an operand with no `%` that is not a number either.
const killTargetNotAPid = jobSpecUnanswered + 1

// killTarget reads what `kill` was pointed at: a pid, or a job.
//
// A pid is one target and a job is as many as it has processes — `%1` names
// the job, and a backgrounded pipeline is every element of it. Each target
// carries whether its number names a process group, because reaching a group
// is a different call from reaching a process and only one of them is this
// package's to make.
func (r *Runner) killTarget(t string) (targets []jobProcess, bad int) {
	if !strings.HasPrefix(t, "%") {
		n, err := strconv.Atoi(t)
		if err != nil {
			return nil, killTargetNotAPid
		}
		return []jobProcess{{pid: n}}, jobFound
	}
	j, code := r.findJobQuietly(t)
	if code != jobFound {
		return nil, code
	}
	// A group only where the process leads one. Started with the monitor off
	// it runs in this shell's group, so naming the group would name the shell
	// — and every other job it started, and on a terminal the whole foreground
	// group. The process is what `%1` means there (#1738). Job.processes
	// carries that answer per process.
	targets = j.processes()
	if len(targets) == 0 {
		// A job with no process of its own — nothing to signal, reported as
		// the missing job it behaves as.
		return nil, jobMissing
	}
	return targets, jobFound
}

// signalGroupPid sends to a job's process group.
//
// Nil hook means this shell has no way to reach a group, which is not the same
// as the group being gone — so it is reported as a process it could not find
// rather than silently succeeding. Asked before the gate is, because a hook
// that is not there is an action that will not happen, and an audit trail that
// recorded it would be recording a signal nothing sent.
//
// The gate is consulted here rather than inside the hook, and that is the same
// split as everywhere else: the hook is a capability the embedder supplied, in
// the embedder's own code, and the gate is a question about what the script
// asked for. It sees the target the kernel will be given — negative, because
// that is how a process group is named — so a policy reading a pid does not
// have to know which of two calls is about to be made.
func (r *Runner) signalGroupPid(pgid int, sig syscall.Signal) error {
	if r.SignalGroup == nil {
		return errNoJobProcess
	}
	if !r.signalAllowed(-pgid, sig) {
		return syscall.EPERM
	}
	return r.SignalGroup(pgid, sig)
}

// sendSignal sends one signal and settles what it means for this shell.
//
// The recording is the whole point of the builtin. kill(2) has delivered the
// signal by the time it returns, so a trap for it has already fired as far as
// the operating system is concerned, and waiting for os/signal's goroutine to
// confirm that is what made the arrival a matter of timing.
//
// A signal with no handler needs the opposite treatment and for the same
// reason. The kernel picks *some* thread to run the default action on, and a
// shell has several, so the thread that dies is rarely the one that called
// kill: measured, `kill -INT $$` with no trap printed the next command's
// output before the process went away. Nothing here can make that thread win
// the race, and nothing needs to — the script is over either way, so it stops
// here and lets the death arrive whenever it arrives.
//
// The split that decision produces is also where the gate goes. Two of the
// three cases never reach the kernel, so nothing leaves this process and there
// is nothing for a policy to refuse; the two that do call it go through
// killProcess, which asks first. Drawing the boundary at the system call
// rather than at the builtin is what keeps those two facts one fact.
func (r *Runner) sendSignal(pid int, name string, sig syscall.Signal) error {
	// Signal 0 is the existence probe and delivers nothing, and anything
	// aimed elsewhere is somebody else's process. A negative pid, and 0
	// meaning this shell's process group, are for the other members: which
	// group this shell is in is a job-control question rather than a
	// signaling one, so neither is treated as aimed here.
	if sig == 0 || pid != os.Getpid() {
		return r.killProcess(pid, sig)
	}
	s := r.sigs()
	s.mu.Lock()
	body, trapped := s.traps[name]
	s.mu.Unlock()
	switch {
	case trapped && body != "":
		// The shell is the only thing listening, so the kernel is not
		// involved: the handler runs before the next command because the
		// arrival was recorded here, not because a goroutine was quick.
		r.selfSignaled(name)
	case !trapped && fatalSignal(name) && r.untrappedSignalIgnored(name):
		// The shell has taken this signal's default action away from the
		// kernel and put nothing in its place, so the raise is not merely
		// survivable — it does not happen. Reported as sent, because the
		// builtin succeeded: what the signal then did is not `kill`'s answer.
		return nil
	case !trapped && fatalSignal(name):
		// Nothing is sent here either, and for a sharper reason. A signal
		// already on its way arrives on whichever thread the kernel picks,
		// which may be while the shell is still running an EXIT trap — that
		// is how a `bye` went missing from a case that expects one. The shell
		// knows it is dying; the dying is the driver's to do, once there is
		// nothing left to run.
		if r.inSubshell {
			// The process this pid names is the top-level shell, and a
			// subshell is a pretend child of it: the parent dies and the
			// sender does not — measured, `(kill -INT $$; echo s)` prints s
			// and the parent stops after the subshell. Note that the
			// *parent's* table decided trapped above, which is the same
			// boundary: a trap the subshell set for itself catches nothing
			// aimed at a pid it does not have.
			r.recordSharedDeath(name, sig)
			// And whether the subshell has anything left to do is the
			// dialect's. Five of the six run the rest of the body, because
			// five of the six gave the subshell a process of its own and the
			// signal was aimed at the parent. ksh93 runs a subshell in the
			// shell's own process, so the signal lands on the very thing
			// that was about to run the next command — and nothing here
			// forks either, which is what makes this a choice rather than a
			// consequence. See Semantics.SubshellRunsOnAfterSignalingTheShell.
			if !r.ask(r.sem().SubshellRunsOnAfterSignalingTheShell,
				"whether a subshell runs on after signaling the shell") {
				// Only the subshell stops here. The death is already
				// recorded, so the parent takes it at the next sequence
				// point and ends by the signal exactly as it would have.
				r.status = 128 + int(sig)
				r.stopTheShell()
			}
			return nil
		}
		r.signalDeath(name, sig)
	default:
		// An ignored signal, and the ones whose default action suspends or
		// resumes rather than ends. Those really do want the kernel: nothing
		// this package does could stop a process or start it again.
		return r.killProcess(pid, sig)
	}
	return nil
}

// untrappedSignalIgnored reports whether this shell has replaced a signal's
// default action with nothing, so that raising it at itself does neither what
// the kernel would do nor what a trap would.
//
// SIGQUIT is the only one the panel does this for, and the question is asked
// only where the panel disagrees about it. Interactive is unanimous — all five
// ignore an untrapped QUIT with `-i` — and so is every other fatal signal, so
// the axis is consulted for one signal in one mode and nowhere else.
//
// A second axis sits behind the first and is reached only through it. `trap -
// QUIT` gives the signal back its default action in zsh and leaves the ignore
// standing in bash and ash, so a shell that has been asked to restore the
// default is no longer ignoring anything. Asking it here rather than beside
// the first keeps it out of the shells the first has already answered no for:
// they are killed by an untrapped QUIT with or without the reset, so both
// readings run their scripts the same way and neither has to answer.
func (r *Runner) untrappedSignalIgnored(name string) bool {
	if name != "QUIT" {
		return false
	}
	if r.Interactive {
		return true
	}
	if !r.ask(r.sem().QuitIgnoredWhenNotInteractive,
		"whether an untrapped QUIT ends a shell that is not interactive") {
		return false
	}
	if !r.defaultRestored(name) {
		return true
	}
	restores := r.ask(r.sem().QuitResetRestoresTheDefault,
		"whether `trap -` on an ignored signal restores its default action")
	if r.unspecified {
		// Refused rather than answered. Saying "still ignored" here would
		// swallow the refusal and run the next command, which is the one
		// thing an unanswered axis must not do; saying "not ignored" hands
		// the caller back to the path that reads r.unspecified and reports
		// it, which is where the axis above already lands.
		return false
	}
	return !restores
}

// fatalSignal reports whether a signal with no handler ends the process.
func fatalSignal(name string) bool {
	for _, k := range knownSignals {
		if k.Name == name {
			return k.Fatal
		}
	}
	return false
}

// signalDeath stops the script because the shell has just killed itself.
//
// The status is the one a process killed by a signal reports, and it is set
// here rather than left to the driver because the driver will not get the
// chance if the kernel is quicker.
//
// Except where the ending is not a death at all. One shell in the panel ends
// on an untrapped SIGHUP the way `exit 1` ends it, and recording no death is
// the whole of that: nothing sets killedBy, so the driver raises nothing, the
// EXIT trap runs without ExitTrapRunsOnSignalDeath being asked, and an `exit`
// inside that trap still takes the status. Both routes here — the raise, and
// a subshell's raise collected by the parent — pass through this function,
// which is why the answer is read here rather than at either of them.
func (r *Runner) signalDeath(name string, sig syscall.Signal) {
	r.stoppedBySignal = true
	if name == "HUP" && r.ask(r.sem().HangupIsAnOrderlyExit,
		"whether an untrapped HUP exits the shell rather than killing it") {
		r.status = hangupExitStatus
		r.stopTheShell()
		return
	}
	r.killedBy, r.killedBySig = name, sig
	r.diedOfSig = sig
	r.status = 128 + int(sig)
	r.stopTheShell()
}

// hangupExitStatus is what a shell that treats SIGHUP as an exit exits with.
// Measured, and it is a constant rather than the signal's number: it stays 1
// whatever the previous command reported.
const hangupExitStatus = 1

// killStatus answers what the whole command reports, which is an axis with
// three answers rather than a majority and an exception.
//
// Only where it is actually a question. One target that failed is 1 in every
// shell in the panel, and it takes a second target for the three answers to
// become visible at all, so the common case needs no dialect.
func (r *Runner) killStatus(sent, failed int) int {
	if failed == 0 {
		return 0
	}
	if sent+failed == 1 {
		return 1
	}
	switch r.killStatusPolicy() {
	case KillStatusAnySuccess:
		if sent > 0 {
			return 0
		}
		return 1
	case KillStatusFailureCount:
		return failed
	default:
		return 1
	}
}

// killList is `kill -l`.
//
// The translating forms are the useful ones and the panel agrees on them:
// a number gives the name and a name gives the number. The bare listing is
// four different formats over a table that is not the same on two operating
// systems, so it is printed plainly here and left out of the corpus rather
// than recorded as a fact about a machine.
func (r *Runner) killList(args []string) int {
	if len(args) == 0 {
		byNumber := append([]signalEntry{}, knownSignals...)
		sort.Slice(byNumber, func(i, j int) bool { return byNumber[i].Sig < byNumber[j].Sig })
		names := make([]string, len(byNumber))
		for i, k := range byNumber {
			names[i] = k.Name
		}
		// Four shells, four shapes; the constant says whose this is.
		switch r.diag().KillListing {
		case KillListingNumbered:
			var b strings.Builder
			for i, k := range byNumber {
				fmt.Fprintf(&b, "%2d) SIG%s", int(k.Sig), k.Name)
				if (i+1)%5 == 0 || i == len(byNumber)-1 {
					b.WriteByte('\n')
				} else {
					b.WriteByte('\t')
				}
			}
			r.printf("%s", b.String())
		case KillListingSpaceJoined:
			r.printf("%s\n", strings.Join(names, " "))
		case KillListingZeroFirst:
			r.printf("0\n%s\n", strings.Join(names, "\n"))
		default:
			r.printf("%s\n", strings.Join(names, "\n"))
		}
		return 0
	}
	for _, a := range args {
		if n, err := strconv.Atoi(a); err == nil {
			name := ""
			for _, k := range knownSignals {
				if int(k.Sig) == n {
					name = k.Name
				}
			}
			if name == "" {
				return r.killReport(killInvalidSignal, a)
			}
			_, _ = fmt.Fprintln(r.stdout(), name)
			continue
		}
		switch r.killListAcceptsName() {
		case No:
			return r.killReport(killNotAPid, a)
		case Unspecified:
			return r.status
		}
		name, sig, err := r.signalSpec(a, killSpecOption)
		if err != nil || name == "" {
			return r.killReport(killInvalidSignal, a)
		}
		_, _ = fmt.Fprintln(r.stdout(), int(sig))
	}
	return 0
}

// killError is a `kill` that did not get as far as signaling anything.
type killError struct {
	kind    killErrorKind
	operand string
}

type killErrorKind int

const (
	// killUsage is nothing to signal: no operands at all.
	killUsage killErrorKind = iota
	// killMissingSignalArgument is `-s` with nothing after it.
	killMissingSignalArgument
	// killInvalidSignal is a name or number that names no signal.
	killInvalidSignal
	// killIllegalOption is the same thing spelled as a flag, which two
	// dialects report as an unknown option instead.
	killIllegalOption
	// killNotAPid is an operand that is not a number.
	killNotAPid
	// killNoSuchProcess is a target that is not there.
	killNoSuchProcess
	// killNotPermitted is a target that is there and is not ours.
	killNotPermitted
	// killNoSuchJob is a `%` spec that names no job.
	killNoSuchJob
)

func (e *killError) Error() string { return e.fallback() }

// verbs is what a wording may name: the operand as written, and its first
// character alone. One dialect wants the second for an illegal option and
// nothing else wants it at all.
func (e *killError) verbs() []any {
	first := e.operand
	if r := []rune(first); len(r) > 0 {
		first = string(r[0])
	}
	// Exactly one prefix, however many the operand arrived with: zsh names an
	// unknown signal `SIGQ` for `Q` and `SIGNOPE` for `SIGNOPE`, so passing
	// the operand as written to a format that adds one gave SIGSIGNOPE.
	prefixed := "SIG" + strings.TrimPrefix(strings.ToUpper(e.operand), "SIG")
	return []any{e.operand, first, prefixed}
}

func (e *killError) fallback() string {
	switch e.kind {
	case killMissingSignalArgument:
		return "kill: %[1]s: option requires an argument"
	case killInvalidSignal, killIllegalOption:
		return "kill: %[1]s: invalid signal specification"
	case killNotAPid:
		return "kill: %[1]s: not a pid"
	case killNoSuchJob:
		return "kill: %[1]s: no such job"
	case killNoSuchProcess:
		return "kill: (%[1]s) - No such process"
	case killNotPermitted:
		return "kill: (%[1]s) - Operation not permitted"
	}
	return "kill: usage: kill [-s sigspec | -n signum | -sigspec] pid ... or kill -l [sigspec]"
}

func (e *killError) format(d Diagnostics) string {
	switch e.kind {
	case killMissingSignalArgument:
		return d.KillMissingSignalArgument
	case killInvalidSignal:
		return d.KillInvalidSignal
	case killIllegalOption:
		return d.KillIllegalOption
	case killNotAPid:
		return d.KillNotAPid
	case killNoSuchJob:
		return d.KillNoSuchJob
	case killNoSuchProcess:
		return d.KillNoSuchProcess
	case killNotPermitted:
		return d.KillNotPermitted
	}
	return d.KillUsage
}

// status is what this class of failure reports, which is three questions
// rather than one: a dialect can be strict about arguments and relaxed about
// options, and one is strict about neither.
func (e *killError) status(d Diagnostics) int {
	switch e.kind {
	case killUsage:
		return orDefault(d.KillUsageStatus, 2)
	case killIllegalOption, killMissingSignalArgument:
		return orDefault(d.KillBadOptionStatus, 1)
	}
	return orDefault(d.KillArgumentStatus, 1)
}

func orDefault(v, fallback int) int {
	if v == 0 {
		return fallback
	}
	return v
}

// killReport prints one failure and returns the status it carries.
func (r *Runner) killReport(kind killErrorKind, operand string) int {
	return r.killFailed(&killError{kind: kind, operand: operand})
}

func (r *Runner) killFailed(err error) int {
	var ke *killError
	if !errors.As(err, &ke) {
		r.diagf("kill: %v\n", err)
		return 1
	}
	d := r.diag()
	target := ke.kind == killNoSuchProcess || ke.kind == killNotPermitted
	if (ke.kind == killUsage && d.KillUsageUnprefixed) || (target && d.KillTargetUnprefixed) {
		// One dialect prints these with no location and no shell name in
		// front, which is not how it prints a complaint about an argument.
		r.errf("%s\n", Wording(ke.format(d), ke.fallback(), ke.verbs()...))
	} else {
		r.diagf("%s\n", Wording(ke.format(d), ke.fallback(), ke.verbs()...))
	}
	if (ke.kind == killInvalidSignal || ke.kind == killIllegalOption) && d.KillUnknownSignalHint != "" {
		r.diagf("%s\n", d.KillUnknownSignalHint)
	}
	return ke.status(d)
}
