// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
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
// on Linux. sharedSignals below is the part every platform has; the rest of
// the table is the platform's own, in platformsignals_<goos>.go, which is
// what makes SIGEMT and SIGINFO present on the machine that has them and
// SIGSTKFLT and SIGPWR present on the machine that has those.
//
// That file also carries platformSignalMax, and the two are not the same
// question. The table is what the shell can *name*; the bound is what the
// kernel will *take*, and on Linux the gap between them is thirty-three
// signals — a script that says `kill -40` there is asking for a real-time
// signal that every reference sends and that this shell refused in every
// dialect (#3168, #3287).
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

var sharedSignals = []signalEntry{
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

var knownSignals = append(append([]signalEntry{}, sharedSignals...), platformSignals...)

// signalInPlatformRange reports whether a number is a signal on this kernel,
// whether or not the table above has a name for it.
//
// The two questions come apart only where a platform has more signals than
// names — Linux's real-time range — and that is exactly where every reference
// sends and this shell refused. A bound of zero is a platform nobody
// measured, where the table stays the whole answer.
func signalInPlatformRange(n int) bool {
	return n >= 1 && n <= platformSignalMax
}

// signalTable is the table this dialect reads names out of, which is the
// platform's minus the names this shell has never heard of.
//
// Filtered per Runner rather than per process because it is the *shell's*
// table and not the machine's: ksh93 on this machine names EMT and has no
// name for signal 29, which every other column calls INFO. See
// Semantics.SignalNamesTheShellLacks.
func (r *Runner) signalTable() []signalEntry {
	lacks := r.sem().SignalNamesTheShellLacks
	if lacks == "" {
		return knownSignals
	}
	table := make([]signalEntry, 0, len(knownSignals))
	for _, k := range knownSignals {
		if !slices.Contains(strings.Fields(lacks), k.Name) {
			table = append(table, k)
		}
	}
	return table
}

// knownSignal reports whether a bare, uppercased name is one of the signals
// this shell can send.
func (r *Runner) knownSignal(name string) bool {
	_, ok := r.signalNamed(name)
	return ok
}

// signalNamed is the table entry a bare, uppercased word names, an older
// spelling of a name included.
//
// One lookup for every route a name arrives by — `kill -s`, `kill -SPEC`,
// `kill -l NAME` and `trap` — because the alias is the *table's* and not one
// builtin's: measured 2026-09-17, ksh93u+ and zsh 5.9.2 answer all four with
// IOT and bash 5.3.20 answers none of them. See
// Semantics.SignalNamesTheShellAlsoReads.
func (r *Runner) signalNamed(name string) (signalEntry, bool) {
	if table, ok := r.signalSpelledAs(name); ok {
		// A word this shell spells its own way, back to what the table
		// carries it under — so the name the same shell *writes* for a
		// number reads back to that number, which is what makes it a name
		// and not a rendering.
		name = table
	}
	if canonical, ok := r.signalAlias(name); ok {
		name = canonical
	}
	for _, k := range r.signalTable() {
		if k.Name == name {
			return k, true
		}
	}
	return signalEntry{}, false
}

// signalAlias turns an older name this shell also answers to into the name
// its own table carries.
func (r *Runner) signalAlias(name string) (string, bool) {
	for _, pair := range strings.Fields(r.sem().SignalNamesTheShellAlsoReads) {
		if alias, canonical, ok := strings.Cut(pair, "="); ok && alias == name {
			return canonical, true
		}
	}
	return "", false
}

// signalSpelling is the word this shell writes for a table entry wherever a
// number is turned back into a name — the entry's own name in every column
// but one. See Semantics.SignalNamesTheShellSpellsItsOwnWay.
func (r *Runner) signalSpelling(name string) string {
	for _, pair := range strings.Fields(r.sem().SignalNamesTheShellSpellsItsOwnWay) {
		if table, shell, ok := strings.Cut(pair, "="); ok && table == name {
			return shell
		}
	}
	return name
}

// signalSpelledAs is that field read backwards: the table's name for a word
// this shell spells its own way, so the word is read as well as written.
func (r *Runner) signalSpelledAs(word string) (string, bool) {
	for _, pair := range strings.Fields(r.sem().SignalNamesTheShellSpellsItsOwnWay) {
		if table, shell, ok := strings.Cut(pair, "="); ok && shell == word {
			return table, true
		}
	}
	return "", false
}

// signalListingName is the word the bare listing writes for a table entry,
// which is the entry's own name in every column but one.
func (r *Runner) signalListingName(name string) string {
	if !r.diag().SignalListingWritesTheAlias {
		return name
	}
	for _, pair := range strings.Fields(r.sem().SignalNamesTheShellAlsoReads) {
		if alias, canonical, ok := strings.Cut(pair, "="); ok && canonical == name {
			return alias
		}
	}
	return name
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
	case a == "-s" || (a == "-n" && r.ask(r.sem().KillReadsTheNumberOption, "`kill -n signum`")):
		if len(args) < 2 {
			if r.ask(r.sem().KillOptionWithNoArgumentIsASignalName,
				"`kill -s` with nothing after it read as the signal `s`") {
				// Not a missing argument at all in that reading: the letter
				// is the spec, and it meets the same refusal any other word
				// that names no signal meets.
				spec, form, args = a[1:], killSpecOption, args[1:]
				break
			}
			if r.unspecified {
				return "", 0, nil, nil
			}
			return "", 0, nil, &killError{kind: killMissingSignalArgument, operand: a}
		}
		spec, form, args = args[1], killSpecOptionFor(a), args[2:]
	case a == "-n" && r.unspecified:
		// The axis above went unanswered on the very word it is about, so
		// this shell has already said so.
		return "", 0, nil, nil
	case isJoined && r.ask(r.sem().KillReadsASignalJoinedToItsOption,
		"a signal written onto `kill -n` or `kill -s` with no space"):
		spec, form, args = joined, killSpecOptionFor(args[0][:2]), args[1:]
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

// startsWithDigit reports whether a signal spec is number-shaped — which is a
// different question from being a number, and is what separates `-9x` from
// `-NOPE` in the one column that words the two differently.
func startsWithDigit(spec string) bool {
	return spec != "" && spec[0] >= '0' && spec[0] <= '9'
}

// killSpecForm is how the signal was spelled, which two dialects report
// differently: dash and ksh93 call an unrecognized `-Q` an unknown *option*
// and an unrecognized `-s Q` an unknown *signal*, where bash and zsh use one
// message for both and set the two wordings to the same string.
type killSpecForm int

const (
	killSpecBare killSpecForm = iota
	// killSpecOption is `-s`, which takes a signal *name*.
	killSpecOption
	// killSpecNumberOption is `-n`, which takes a signal *number*. Its own
	// form because two dialects treat a number differently from a name:
	// `kill -n 99` reaches `kill(2)` in ksh93 and zsh where `kill -s 99` is
	// refused by both. See Semantics.KillSendsASignalNumberItCannotName.
	killSpecNumberOption
	killSpecFlag
)

// killSpecOptionFor is which of the two option forms a word is, given the
// option itself — `-n` or `-s`, joined spelling included.
func killSpecOptionFor(option string) killSpecForm {
	if option == "-n" {
		return killSpecNumberOption
	}
	return killSpecOption
}

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
		return "", 0, &killError{kind: kind, operand: spec, fromFlag: form == killSpecFlag}
	}
	// A word written where a *number* goes and not one. `-n 9x` is that by
	// position; `-9x` is that by shape, because a dash-word starting with a
	// digit is a number to the shells that read one there where `-NOPE` is
	// not. One column has a third wording for it — measured 2026-09-17 on
	// zsh 5.9.2, where `kill -9x` is `invalid signal number: -9x`, `kill -n
	// 9x` is `invalid signal number: 9x` and `kill -s 9x` is `unknown
	// signal: SIG9X` with the listing hint after it. Note which of the three
	// carries the dash: the flag form's operand is the whole word (#3167).
	//
	// Every other column falls back to the wording it already has for the
	// form the word arrived in, which is why fromFlag is carried rather than
	// the kind being chosen by whether a dialect has the new string: dash
	// answers `-9x` with `Illegal option -9` at 2 and `-n 9x` with its
	// argument complaint, and those are two different statuses.
	badNumber := func() (string, syscall.Signal, error) {
		return "", 0, &killError{
			kind:     killInvalidSignalNumber,
			operand:  spec,
			fromFlag: form == killSpecFlag,
		}
	}
	if n, err := strconv.Atoi(spec); err == nil {
		if n == 0 {
			// Not a signal at all but the existence probe, and it is asked
			// before anything else because every column takes it after `-s`:
			// `kill -s 0 $$` is 0 in all five, zsh included.
			return "", 0, nil
		}
		if form == killSpecOption &&
			!r.ask(r.sem().KillNameOptionReadsANumber, "a number written after `kill -s`") {
			if r.unspecified {
				return "", 0, nil
			}
			// `-s` takes a name, and a word of digits is not one in the
			// column that holds to it — which reports the digits as the name
			// they are not, hint and all, rather than as a bad number.
			return bad()
		}
		for _, k := range knownSignals {
			if int(k.Sig) == n {
				// The name is the *platform's* here rather than the
				// dialect's, and deliberately: a shell with no name for a
				// number still sends it. ksh93 refuses `kill -INFO` and
				// sends `kill -29`, which is the same split every column
				// draws between a word it will not read and a number the
				// kernel understands.
				return k.Name, k.Sig, nil
			}
		}
		if form != killSpecOption && signalInPlatformRange(n) {
			// A signal this kernel has and this table cannot name — Linux's
			// real-time range, where every reference sends and this engine
			// refused in every dialect. Core rather than an axis: the shells
			// that check are checking the range and not a list of names, so
			// there is nothing here for a dialect to disagree about. `-s`
			// stays out for the reason it stays out below: it takes a name,
			// and a number there is already the wrong kind of word.
			return "", syscall.Signal(n), nil
		}
		// A number this shell has no name for. Two dialects hand it to the
		// kernel anyway and let `kill(2)` be the one to refuse it, which is
		// the difference between a word the shell would not take and a send
		// that failed — see the axis for the measurement, and note it is
		// asked only where the signal was written as a number in a position
		// that takes one, since `-s 99` is a name-shaped operand that
		// happens to be digits.
		//
		// Read rather than `ask`ed, for the reason Runner.EditingMode is:
		// an unanswered axis here would replace one refusal with another
		// rather than stop a shell from guessing. Not sending is a real
		// answer — four of the five columns give it — and it is the one the
		// standard implies, so a vector that says nothing gets a `kill` that
		// keeps its own counsel rather than one that puts a number it has
		// never heard of into a system call.
		if form != killSpecOption && n > 0 && r.sem().KillSendsASignalNumberItCannotName == Yes {
			return "", syscall.Signal(n), nil
		}
		return bad()
	}
	if form == killSpecNumberOption && r.diag().KillBadSignumIsAUsageError {
		// One column reads a word that is not a number after `-n` as a
		// misuse of the option rather than as a signal nobody has, and
		// writes the same block a bare `kill` writes, at the same status.
		return "", 0, &killError{kind: killUsage}
	}
	if form == killSpecNumberOption || (form == killSpecFlag && startsWithDigit(spec)) {
		// Number-shaped and not a number, so it never reaches the name
		// table: `SIG9X` names nothing in any column, and the one shell with
		// a wording for this says so without the prefix.
		//
		// `-s` is excluded and that is measured rather than symmetry: it
		// takes a *name*, so `kill -s 9x` is `unknown signal: SIG9X` with
		// the listing hint in the very column that writes `invalid signal
		// number: -9x` for the flag form.
		return badNumber()
	}
	up := strings.ToUpper(spec)
	if trimmed, had := strings.CutPrefix(up, "SIG"); had && r.knownSignal(trimmed) {
		// Whether the prefix is part of a name is the dialect's answer, the
		// same one `trap` asks — dash reads `SIGCONT` as neither a signal nor
		// an option and says so twice over. Asked only where it decides
		// something: `SIGNOPE` names nothing either way.
		if !r.ask(r.sem().SIGPrefixAccepted, "the SIG prefix on a signal name") {
			return bad()
		}
		up = trimmed
	}
	if k, ok := r.signalNamed(up); ok {
		return k.Name, k.Sig, nil
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
		aims, fromJob, bad := r.killTarget(t)
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
		if fromJob && r.aimsAJobSpecAtItsGroup(aims) {
			// dash names the job's *process group* rather than its process.
			// With the monitor off — which is every script — the job's
			// process leads no group, so there is no group of that number
			// and the send is ESRCH. Measured 2026-09-16 on Apple's dash-16
			// and on Debian's and Alpine's dash 0.5.12, on a job blocked
			// opening a fifo so that it cannot have finished: `kill -0 %1`,
			// `%%` and `%+` are all `kill: No such process` at 1, where
			// `kill -0 "$!"` on the same job is 0 and `kill -0 -"$!"` is the
			// same refusal. bash, zsh and ksh93 aim at the process and
			// succeed.
			failed++
			r.killFailed(&killError{kind: killNoSuchProcess, operand: t, errno: syscall.ESRCH})
			continue
		}
		if len(aims) == 0 {
			// A target that is there with nothing to reach, which is a job
			// either way: one that has ended and that the script has not
			// waited for — a child a real shell has not reaped yet — or one
			// still running that this shell has no process for. `kill`
			// succeeds at both and nothing receives anything; see
			// Runner.jobProcesses for why the panel says the target is
			// there in each. Every other route out of killTarget names at
			// least one process, so an empty list is one of those two and
			// not a list from anywhere else.
			sent++
			continue
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
			r.killFailed(&killError{kind: killFailureKind(miss), operand: t, errno: miss})
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
// killFailureKind is which complaint a failed send draws, which is three
// cases and not two once a shell can send a signal number it cannot name.
//
// EINVAL is what `kill(2)` answers for a signal the platform does not have,
// and the two shells that get that far word it differently: zsh prints the
// errno — `kill <pid> failed: invalid argument` — and ksh93 gives every
// failed send its one sentence, `kill: <pid>: no such process`, errno
// regardless. Measured 2026-09-16 with `kill -99 $$` on macOS arm64, where
// the same two shells answer `no such process` for a pid that is really
// absent, so the wordings are separable rather than coincidental.
func killFailureKind(err error) killErrorKind {
	switch {
	case errors.Is(err, syscall.EPERM):
		return killNotPermitted
	case errors.Is(err, syscall.ESRCH):
		return killNoSuchProcess
	}
	return killSendFailed
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
func (r *Runner) killTarget(t string) (targets []jobProcess, fromJob bool, bad int) {
	if !strings.HasPrefix(t, "%") {
		n, err := strconv.Atoi(t)
		if err != nil {
			return nil, false, killTargetNotAPid
		}
		if j := r.jobByIdent(n); j != nil {
			// A number this shell invented for a job that has no process of
			// its own — what `$!` gives for `( : ) &` — so it is read as the
			// job it names rather than handed to the kernel. Without this
			// the number would reach `kill` as a process id, which is the
			// whole reason it is out above every id the kernel can issue:
			// it would find nothing. Reading it here is what makes
			// `p=$!; kill "$p"` mean the job the script started. See
			// jobident.go.
			// A number, so it is a pid as far as `kill` is concerned even
			// here: the job spec this shell reads it back as is an
			// arrangement of ours, not a `%` word the script wrote, and the
			// panel aims a number at the process in every column.
			targets, code := r.jobProcesses(j)
			return targets, false, code
		}
		return []jobProcess{{pid: n}}, false, jobFound
	}
	j, code := r.findJobQuietly(t)
	if code != jobFound {
		return nil, true, code
	}
	targets, found := r.jobProcesses(j)
	return targets, true, found
}

// aimsAJobSpecAtItsGroup reports whether this dialect points a `%` spec at
// the job's process group, and the group is one that cannot be there.
//
// The pair is one question because only the second half is reachable in a
// script: a group id is its leader's pid, and a process that leads no group
// has no group of its number for anything to receive. So where the dialect
// aims at the group and the job's first process leads none, the send is
// ESRCH by construction and no call has to be made to find that out — which
// also keeps this shell from ever handing a negative number to the kernel on
// a job that shares its own group. Where the monitor did put the job in a
// group of its own, the ordinary group send below is already the right call.
func (r *Runner) aimsAJobSpecAtItsGroup(aims []jobProcess) bool {
	// dash names the first process of the job, so that is the one whose
	// group is in question. Where the monitor did put it in a group of its
	// own the two readings agree — the group is there, and the ordinary
	// group send below reaches it whichever the dialect aims at — so the
	// axis is put only where it decides something, which is the discipline
	// every other conditional axis in this package follows.
	if len(aims) > 0 && aims[0].ownGroup {
		return false
	}
	return r.ask(r.sem().KillJobSpecAimsAtTheGroup, "`kill` aiming a job spec at the job's process group")
}

// jobProcesses is what signaling a job aims at, whether the script named the
// job by a `%` spec or by the number `$!` gave for it.
//
// A group only where the process leads one. Started with the monitor off it
// runs in this shell's group, so naming the group would name the shell — and
// every other job it started, and on a terminal the whole foreground group.
// The process is what `%1` means there (#1738). Job.processes carries that
// answer per process.
//
// One function rather than the same three lines in two places, because the two
// routes are one decision: the number `$!` reports for a job with no process
// is read back as that job, so `kill "$!"` and `kill %1` must reach the same
// thing or the shell has two answers for one question.
func (r *Runner) jobProcesses(j *Job) (targets []jobProcess, bad int) {
	targets = j.processes()
	if len(targets) == 0 {
		if j.Finished() {
			// The job is over and the script has not waited for it, which on
			// every reference is a child that has exited and not been reaped
			// — still a process, and still something `kill` succeeds at
			// aiming a signal nothing will receive. `cmd & p=$!;
			// kill -0 "$p"` is how a script asks whether a job it started is
			// still its own to wait for, and this shell answered no for
			// every job made of builtins or of a compound command, where a
			// real shell has forked and has a body to leave behind (#2938).
			//
			// Nothing is signaled, which is the same as the reference: a
			// signal sent to a child that has already exited reaches nothing
			// either. What changes is only that the target is *there*, which
			// is what it is on every shell until `wait` collects it — and
			// what stops being true here the moment `wait` does, because
			// that takes the job out of the table and the lookups above
			// never reach this.
			return nil, jobFound
		}
		// A job still running with no process of its own, which is two
		// shapes and not one: a background builtin or compound command that
		// will never have a process, and an ordinary external command whose
		// *redirection* is still blocked on an open — `cat < fifo &` — so
		// the process is a moment away rather than never coming. This shell
		// settles a pid of zero at a blocking open so that `&` returns at
		// all, and the job stands here in between; see
		// Runner.settleBackgroundJobBeforeABlockingOpen.
		//
		// It is a target that is *there*, for the reason the finished job
		// above is: every reference forks before it opens anything, so the
		// child exists from the moment `&` returns and answers `kill -0` for
		// the whole of the job's life. Measured 2026-09-15 on bash 5.3.15,
		// zsh 5.9.2, ksh93u+ 2012-08-01 and dash, on a job blocked opening a
		// fifo as a redirection, as an operand, and made only of builtins:
		// `kill -0 "$!"` is 0 in all four for every shape (#2994). This
		// shell answered no to all of them, so the question a script asks
		// before it signals a job read as "that job is gone" while the job
		// was running.
		//
		// What is still not right, and is the same gap #2938 named rather
		// than a new one: nothing is signaled. A job this shell has no
		// process for cannot receive one, so `kill -TERM "$!"` reports the 0
		// the panel reports and the job runs on, where a real shell's fork
		// would have taken it. The number is not spent on anything else —
		// it is read back as the job here rather than handed to the kernel,
		// which is what jobident.go puts it above every issuable pid for —
		// so the failure is a signal that reaches nothing, never a signal
		// that reaches something the script did not name.
		return nil, jobFound
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
	if !catchableSignal(sig) {
		// `trap` takes KILL and STOP because every shell on the panel does,
		// and the entry it leaves is a listing rather than a handler: nothing
		// can catch either signal, so what follows is what would have
		// followed with no trap at all. Measured — `trap 'echo T' KILL; kill
		// -KILL $$` prints nothing and exits 137 in all six — which is
		// exactly the treatment below for a signal nobody trapped, so the
		// table is stepped over rather than answered a second way (#2919).
		//
		// This shell would otherwise be the one place the promise came true:
		// a signal aimed at `$$` never reaches the kernel here — see
		// selfSignaled — so a KILL handler would have fired, and a script
		// that traps KILL defensively would have survived `kill -9` here and
		// nowhere else.
		//
		// The ignore is stepped over too, and that half is about this being a
		// library. `trap '' KILL` leaves an entry with an empty body, which
		// is not a handler and so falls to the bottom of the switch — where a
		// real kill(2) is made at our own pid, and an embedder's process dies
		// on a line of script. An untrapped KILL has never taken that route:
		// it goes through signalDeath, which stops the script and leaves the
		// dying to Runner.DieBySignal. An ignore that cannot be honored must
		// land in the same place.
		trapped = false
	}
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

// signalCell is one position of the bare listing: the number the host gives
// it, and the word this shell writes there.
type signalCell struct {
	sig  int
	text string
}

// signalListing is the bare listing's positions, in the host's numbering.
//
// It walks the *platform's* table rather than this dialect's, because a
// position the dialect cannot name is not always a position it leaves out.
// Measured 2026-09-17: ksh93u+ on macOS writes `SIG29` where every other
// column writes `INFO`, and dash on Linux writes a bare `16` for the STKFLT
// it is short of — two shells with the same gap and two renderings of it, and
// this engine dropped the row in both. See
// Diagnostics.KillListingUnnamedPosition, whose empty value is the third
// answer: leave the position out, which is what a shell whose table is the
// platform's never has to decide.
func (r *Runner) signalListing() []signalCell {
	byNumber := append([]signalEntry{}, knownSignals...)
	sort.Slice(byNumber, func(i, j int) bool { return byNumber[i].Sig < byNumber[j].Sig })
	unnamed := r.diag().KillListingUnnamedPosition
	cells := make([]signalCell, 0, len(byNumber))
	for _, k := range byNumber {
		if r.knownSignal(k.Name) {
			cells = append(cells, signalCell{int(k.Sig), r.signalSpelling(r.signalListingName(k.Name))})
			continue
		}
		if unnamed != "" {
			cells = append(cells, signalCell{int(k.Sig), fmt.Sprintf(unnamed, int(k.Sig))})
		}
	}
	return cells
}

// listSignalTable writes the whole signal table, which is what `kill -l`
// with no operands prints and — in the one dialect whose `trap` has the
// letter — what `trap -l` prints too.
//
// One function for both because they are one listing: measured 2026-09-17,
// bash 5.3.20's `trap -l` is its `kill -l` byte for byte, and the table is
// the shell's rather than either builtin's (#3474). This wrote one bare name
// per line under `trap` while `kill` already had all four shapes, so the
// same shell answered the same question two ways.
//
// The order is the host's numbering and not the order the table is written
// in, which is what makes the listing a fact about the machine rather than
// about this file.
func (r *Runner) listSignalTable() int {
	cells := r.signalListing()
	names := make([]string, len(cells))
	for i, c := range cells {
		names[i] = c.text
	}
	// Four shells, four shapes; the constant says whose this is.
	switch r.diag().KillListing {
	case KillListingNumbered:
		var b strings.Builder
		for i, k := range cells {
			fmt.Fprintf(&b, "%2d) SIG%s", k.sig, k.text)
			// A tab after every entry and a newline instead of it at the end
			// of a row, so a short last row carries the tab it was separated
			// by and then a newline of its own. Measured: bash's last line is
			// `31) SIGUSR2<tab>` on a 31-signal machine and
			// `64) SIGRTMAX<tab>` on a 64-signal one, and the full rows above
			// them end with no tab at all.
			if (i+1)%5 == 0 {
				b.WriteByte('\n')
				continue
			}
			b.WriteByte('\t')
			if i == len(cells)-1 {
				b.WriteByte('\n')
			}
		}
		r.printf("%s", b.String())
	case KillListingNumberedPerLine:
		var b strings.Builder
		for _, k := range cells {
			// The number in a two-wide right-aligned column and the name
			// bare — ` 1) HUP` and `10) USR1`. No `SIG` and no packing, so
			// this shares only the number column with the shape above it.
			fmt.Fprintf(&b, "%2d) %s\n", k.sig, k.text)
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

// killList is `kill -l`.
//
// The translating forms are the useful ones and the panel agrees on them:
// a number gives the name and a name gives the number. The bare listing is
// four different formats over the platform's table, which is why it took a
// platform table to write: it was one machine's shortest column before, and
// a case recording it could not pass on both.
func (r *Runner) killList(args []string) int {
	if len(args) == 0 {
		return r.listSignalTable()
	}
	for _, a := range args {
		if n, err := strconv.Atoi(a); err == nil {
			name, ok := r.killListName(n)
			if r.unspecified {
				return r.status
			}
			if !ok {
				return r.killReport(killListNumberNotASignal, a)
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
			// The listing form's own complaint rather than the signal
			// spec's, which is measured: BusyBox ash says `unknown signal
			// 'nope'` for `kill -l nope` and `bad signal name 'nope'` for
			// `kill -s nope`, where the other columns word both alike and
			// so are unmoved by the kind (#3165).
			return r.killReport(killListNumberNotASignal, a)
		}
		_, _ = fmt.Fprintln(r.stdout(), int(sig))
	}
	return 0
}

// killListName turns a number into what `kill -l` writes for it.
//
// The number is not a signal number. It is an *exit status*: `$?` is 128 plus
// the signal for a child the kernel ended, so `kill -l "$?"` is the ordinary
// way a script turns a death back into a name, and every shell on the panel
// answers `kill -l 129` with `HUP`. This engine refused all four columns
// until #3053, because the lookup was against the signal table alone.
//
// Three axes, because the agreement stops at the common case:
//
//   - how far the 128 comes off. bash, zsh and dash take it off once and keep
//     the result only when it names something; ksh93 and BusyBox ash subtract
//     while the number is still 128 or more, which is what makes `kill -l
//     257` HUP there and a refusal in the other three.
//   - what is left when nothing matches: printed back in zsh, ksh93 and ash,
//     refused in bash and dash.
//   - whether 0 is `EXIT`, which is the trap table's pseudo-signal rather
//     than the kernel's.
//
// The first subtraction is not one of them. It is unanimous, so it is core
// and asks nobody; the axes begin where the references stop agreeing.
//
// The number printed back is the reduced one, which is the same thing as the
// number as written in every column that reaches here: the two shells that
// reduce repeatedly print the reduction, and zsh — the one that prints a
// number it did not reduce — only ever reaches this with the original,
// because the one subtraction it makes is kept only when it names a signal.
func (r *Runner) killListName(n int) (string, bool) {
	if name, ok := r.killSignalName(n); ok {
		return name, true
	}
	if name, ok, answered := r.killListUnnamedInRange(n); answered {
		return name, ok
	}
	// One 128 off, kept when what is left names a signal. Unanimous across
	// the seven columns — bash 5.3, bash-as-`sh`, bash 3.2, zsh, ksh93, dash
	// and BusyBox ash all answer `kill -l 129` with `HUP` and `kill -l 159`
	// with the last signal on the table — so it is core and asks nobody.
	//
	// The subtraction has to be followed by the same unnamed-in-range answer
	// the whole number got, and that is measured rather than symmetry: dash
	// on Linux answers `kill -l 160` with `32`, which is 160 less 128 landing
	// on a signal it has no name for.
	if n >= 128 {
		if name, ok := r.killSignalName(n - 128); ok {
			return name, true
		}
		if name, ok, answered := r.killListUnnamedInRange(n - 128); answered {
			return name, ok
		}
	}
	// Past here the panel parts, and each question is asked only where the
	// numbers reach it.
	if n == 0 {
		if r.ask(r.sem().KillListNamesZeroAsExit, "`kill -l 0` naming the EXIT trap") {
			return "EXIT", true
		}
		if r.unspecified {
			return "", false
		}
	}
	if n >= 128 {
		if r.ask(r.sem().KillListReducesRepeatedly, "`kill -l 257` reduced by 128 more than once") {
			for n >= 128 {
				n -= 128
			}
			return r.killListName(n)
		}
		if r.unspecified {
			return "", false
		}
	}
	if r.ask(r.sem().KillListPrintsANumberItCannotName, "`kill -l 160` answered with the number") {
		return strconv.Itoa(n), true
	}
	return "", false
}

// killListUnnamedInRange answers `kill -l N` for a number this kernel really
// has and this shell has no name for, and reports whether that was the
// question at all.
//
// The panel writes the number back at status 0 — dash included, which refuses
// a number *outside* the range at 2. Those are two questions and running them
// together is what made dash's row read as a dialect answer (#3287). bash is
// the one column that answers with an empty line, which is the axis.
//
// Nothing on a platform whose whole range the table names can reach this
// through a number alone; on macOS it is reached through a shell whose own
// table is short of the platform's, which is ksh93 and signal 29.
func (r *Runner) killListUnnamedInRange(n int) (name string, ok, answered bool) {
	if !signalInPlatformRange(n) {
		return "", false, false
	}
	if r.ask(r.sem().KillListLeavesAnUnnamedSignalBlank,
		"`kill -l` writing nothing for a signal it has no name for") {
		return "", true, true
	}
	if r.unspecified {
		return "", false, true
	}
	return strconv.Itoa(n), true, true
}

// killSignalName is the signal table read backwards, and the only lookup
// `kill -l` had before #3053.
//
// This dialect's table rather than the platform's, which is what makes
// ksh93's `kill -l 29` the number 29 where every other column here writes
// INFO: the signal is there and the shell has no name for it.
func (r *Runner) killSignalName(n int) (string, bool) {
	for _, k := range r.signalTable() {
		if int(k.Sig) == n {
			// The shell's own word where it has one, which is the same word
			// its listing writes: a column that renames a signal renames it
			// everywhere a number comes back as a name. Not the listing's
			// alias preference, which one column holds for its listing
			// alone — see signalListingName.
			return r.signalSpelling(k.Name), true
		}
	}
	return "", false
}

// killError is a `kill` that did not get as far as signaling anything.
type killError struct {
	kind    killErrorKind
	operand string
	// errno is what `kill(2)` said, for the one wording that prints it. Nil
	// for every failure the shell decided on its own.
	errno error
	// fromFlag says the spec was written as `-SPEC` rather than after `-s`
	// or `-n`, which two things read: the status a number-shaped refusal
	// carries, and whether the operand is printed with its dash.
	fromFlag bool
}

type killErrorKind int

const (
	// killUsage is nothing to signal: no operands at all.
	killUsage killErrorKind = iota
	// killMissingSignalArgument is `-s` with nothing after it.
	killMissingSignalArgument
	// killInvalidSignal is a name or number that names no signal.
	killInvalidSignal
	// killListNumberNotASignal is a number `kill -l` could not turn into a
	// name. Its own kind because one dialect calls the operand an exit
	// status there and a signal name everywhere else, which is the whole
	// point of the listing form: the number it is given is a `$?`.
	killListNumberNotASignal
	// killInvalidSignalNumber is a word written where a *number* goes and
	// not one: `-n 9x`, and `-9x`, which is number-shaped by starting with a
	// digit. One dialect has a third wording for it, distinct from both the
	// unknown-signal one and the unknown-option one; every other column
	// falls back to whichever of those two the form it arrived in already
	// draws, which is what killError.fromFlag decides.
	killInvalidSignalNumber
	// killIllegalOption is the same thing spelled as a flag, which two
	// dialects report as an unknown option instead.
	killIllegalOption
	// killNotAPid is an operand that is not a number.
	killNotAPid
	// killNoSuchProcess is a target that is not there.
	killNoSuchProcess
	// killSendFailed is a send the kernel refused for any other reason,
	// which is reachable only where the shell sends a signal number it has
	// no name for — see Semantics.KillSendsASignalNumberItCannotName. One
	// dialect prints the errno here and the rest fall back on the sentence
	// they use for a target that is not there, which is measured: ksh93
	// really does call an EINVAL `no such process`.
	killSendFailed
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
	// And the kernel's own words, for the dialect that prints them. Go
	// spells an errno the way strerror does and in lower case already —
	// `invalid argument` — which is how zsh prints it.
	reason := ""
	if e.errno != nil {
		reason = e.errno.Error()
	}
	// And the operand as the script wrote it, dash and all, which is the one
	// verb the flag form needs: zsh prints `invalid signal number: -9x` for
	// the flag and `: 9x` for `-n 9x`, so the dash belongs to the *form* and
	// not to the wording.
	written := e.operand
	if e.fromFlag {
		written = "-" + written
	}
	return []any{e.operand, first, prefixed, reason, written}
}

func (e *killError) fallback() string {
	switch e.kind {
	case killMissingSignalArgument:
		return "kill: %[1]s: option requires an argument"
	case killInvalidSignal, killIllegalOption, killListNumberNotASignal, killInvalidSignalNumber:
		return "kill: %[1]s: invalid signal specification"
	case killNotAPid:
		return "kill: %[1]s: not a pid"
	case killNoSuchJob:
		return "kill: %[1]s: no such job"
	case killNoSuchProcess, killSendFailed:
		return "kill: (%[1]s) - No such process"
	case killNotPermitted:
		return "kill: (%[1]s) - Operation not permitted"
	}
	return "kill: usage: kill [-s sigspec | -n signum | -sigspec] pid ... or kill -l [sigspec]"
}

func (e *killError) format(d Diagnostics) string {
	switch e.kind {
	case killMissingSignalArgument:
		// The operand is the option itself, which is the only thing that
		// tells the two sentences apart: one column calls what `-s` wants a
		// signame and what `-n` wants a numeric signum.
		if e.operand == "-n" {
			return orElse(d.KillMissingNumberArgument, d.KillMissingSignalArgument)
		}
		return d.KillMissingSignalArgument
	case killInvalidSignal:
		return d.KillInvalidSignal
	case killListNumberNotASignal:
		return orElse(d.KillListBadNumber, d.KillInvalidSignal)
	case killInvalidSignalNumber:
		// The form the word arrived in picks the fallback, so a dialect with
		// no wording of its own answers exactly as it did before there was
		// one: an unknown option for `-9x`, an unknown signal for `-n 9x`.
		if e.fromFlag {
			return orElse(d.KillInvalidSignalNumber, d.KillIllegalOption)
		}
		return orElse(d.KillInvalidSignalNumber, d.KillInvalidSignal)
	case killIllegalOption:
		return d.KillIllegalOption
	case killNotAPid:
		return d.KillNotAPid
	case killNoSuchJob:
		return d.KillNoSuchJob
	case killNoSuchProcess:
		return d.KillNoSuchProcess
	case killSendFailed:
		return orElse(d.KillSendFailed, d.KillNoSuchProcess)
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
	case killInvalidSignalNumber:
		// The same split the wording takes, and for the same reason: dash
		// answers 2 for an illegal option and its argument complaints
		// elsewhere, so a shared status here would move one of them.
		if e.fromFlag {
			return orDefault(d.KillBadOptionStatus, 1)
		}
		return orDefault(d.KillArgumentStatus, 1)
	}
	return orDefault(d.KillArgumentStatus, 1)
}

func orDefault(v, fallback int) int {
	if v == 0 {
		return fallback
	}
	return v
}

// usesTheOptionWording reports whether this failure is written with the
// unknown-option wording, which two kinds reach: the option complaint itself,
// and a number-shaped word in the flag form, whose fallback is that wording.
func usesTheOptionWording(e *killError, d Diagnostics) bool {
	if e.kind == killIllegalOption {
		return true
	}
	return e.kind == killInvalidSignalNumber && e.fromFlag && d.KillInvalidSignalNumber == ""
}

// lines is the complaint, which is one line in every dialect but one.
//
// ksh93 splits a dash-word into its characters and says `kill: -N: unknown
// option` for each, in order and repeats included — its option parser's doing
// rather than `kill`'s, and measured as such: `kill -NOPE` is four lines there
// and `kill -Q` is one. The word itself is never printed.
func (e *killError) lines(d Diagnostics) []string {
	w, fallback := e.format(d), e.fallback()
	if !usesTheOptionWording(e, d) || !d.KillIllegalOptionPerLetter {
		return []string{Wording(w, fallback, e.verbs()...)}
	}
	out := make([]string, 0, len(e.operand))
	for _, c := range e.operand {
		per := &killError{kind: e.kind, operand: string(c), fromFlag: e.fromFlag}
		out = append(out, Wording(w, fallback, per.verbs()...))
	}
	return out
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
	target := ke.kind == killNoSuchProcess || ke.kind == killNotPermitted || ke.kind == killSendFailed
	// One dialect prints the usage and a failed target with no location and
	// no shell name at all. The one that writes its *own name* in front of
	// every `kill` message says so through Diagnostics.BuiltinNamesTheShellAlone
	// instead, which the ordinary prefix path already reads.
	bare := (ke.kind == killUsage && d.KillUsageUnprefixed) ||
		(target && d.KillTargetUnprefixed)
	say := r.diagf
	if bare {
		say = r.errf
	}
	for _, line := range ke.lines(d) {
		say("%s\n", line)
	}
	if usesTheOptionWording(ke, d) && d.KillIllegalOptionUsage != "" {
		// Printed once however many complaints came before it, which is what
		// took it out of the wording: one shell splits a dash-word into its
		// letters and complains about each, and the block follows the lot.
		//
		// It is the same block a bare `kill` writes, so it is prefixed the
		// same way — which in the one shell that has it is not at all.
		usage := r.diagf
		if d.KillUsageUnprefixed {
			usage = r.errf
		}
		usage("%s\n", d.KillIllegalOptionUsage)
	}
	if (ke.kind == killInvalidSignal || ke.kind == killIllegalOption) && d.KillUnknownSignalHint != "" {
		r.diagf("%s\n", d.KillUnknownSignalHint)
	}
	return ke.status(d)
}
