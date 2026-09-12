// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/blairham/sh/interp"
)

// `zsystem`, the one builtin of `zsh/system` that is neither a read nor a
// write: a file lock, and a way to ask what this shell's copy of the module
// can actually do.
//
// Measured 2026-09-10 against zsh 5.9.2 (Homebrew, aarch64) with `zsh -f` and
// no startup files. #1737 implemented the three that move bytes and left this
// one refused by name; this is the rest of it (#1749).
//
// # Why a lock and not a flag
//
// Four call sites in the tree this shell starts with, and two of them are
// `|| return` guards:
//
//	zsystem flock -t 1 -f ___fd -e $___fle
//	zsystem flock -f lock_fd $file_prefix.lock || return
//	zsystem flock -u $lock_fd || return
//	zsystem flock -- $file_prefix.lock
//
// A plugin manager writes the first and a prompt theme's git daemon the other
// three, so a `zsystem` that refused stopped that daemon before it started.
// **And a `zsystem` that answered 0 without taking a lock would be worse**:
// the daemon would start, twice, with two copies writing one status file. That
// is the failure this whole module was filed about (#1737) wearing a new hat,
// which is why the tests for this run two processes and not one.
//
// # `supports` has to be true or it is the same bug again
//
// `zsystem supports flock` is the question a script asks before it commits to
// the lock, and it is answered here from whether this build has the system
// call rather than from a constant — see systemLockSupported, which is the
// platform's answer and not this file's opinion. Measured: zsh answers 0 for
// `flock` and for `supports`, and **1 for everything else including its own
// builtins' names**, so `zsystem supports subshell`, `zsystem supports
// zsystem` and `zsystem supports sysread` are all 1. The subcommand vocabulary
// is those two words and no more.
//
// # It is fcntl's lock and not flock's, and that is measurable
//
// Two facts settle it, both measured against zsh rather than assumed. Locking
// one file twice in one shell **succeeds both times** — a per-open-file-
// description lock would have the second wait for the first — and a lock does
// **not** survive into a forked child, which a per-description one would.
// Both are what POSIX record locks do and neither is what flock(2) does.
//
// The consequence here is one this shell gets for free and would have had to
// work for: a subshell is a cloned Runner in this process rather than a fork,
// and a per-process lock cannot exclude itself, so `( zsystem flock -t 0 f )`
// inside a shell already holding `f` is 0 in both shells for the same reason.
// The half that does *not* carry over is written down in the spec: zsh's fork
// gives a subshell its own copy of the lock table, and here it is a clone of
// one array, so a subshell can unlock what its parent took.
//
// # The three statuses of a failed lock, and only one of them speaks
//
// Measured, and a caller reads all three:
//
//	written                    zsh 5.9.2
//	zsystem flock f            blocks until it has it
//	zsystem flock -t 0 f       1, `failed to lock file f: …` on a held file
//	zsystem flock -t 0.5 f     2 and **nothing said**, after half a second
//
// So `-t 0` is "ask" and a positive `-t` is "wait", and the silent 2 is the
// one a script written as `zsystem flock -t 1 … || fallback` is counting on:
// a shell that printed a diagnostic there would put a line on a person's
// terminal every time a lock was busy.
//
// `-t` is in **seconds** here and a fraction of one is allowed, which is worth
// saying beside `zselect`, whose `-t` is in hundredths. The two builtins are in
// two modules and do not share a unit.

// systemLockStore is the descriptors this shell is holding locks on, kept as
// an indexed array under a name no script can reach — the idiom `zstyle`,
// `emulate` and `zmodload` all use here, and it gives a subshell its own copy
// for the same reason they want one.
//
// The numbers are the only state: the open file itself is in the runner's
// descriptor table, which is where `zsystem flock -u` reaches it from, so
// there is no second table to be kept in step with the first.
const systemLockStore = ".zsh.flock"

// systemLockPollDefault is how often a `-t` wait re-asks for a lock.
//
// The wait has to be a poll rather than a blocking call, because the blocking
// form of this system call has no deadline in it: a caller asking for one
// second either gets the lock or comes back, and there is no way to cancel a
// call already made. `-i` names the interval and this is what it is without
// one.
//
// Ten milliseconds rather than zsh's own, which is not a documented number and
// is measurably coarser — a lock released mid-wait is picked up here inside a
// hundredth of a second and there in about three tenths. A caller cannot ask
// for a *slower* answer than it asked for, so being early is not a difference
// a script can act on; being late by a third of a second on a prompt's git
// daemon is.
const systemLockPollDefault = 10 * time.Millisecond

// zsystemBuiltin is `zsystem <subcommand> [args]`.
func zsystemBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	if len(args) == 0 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	switch args[0] {
	case "supports":
		return zsystemSupports(r, args[1:])
	case "flock":
		return zsystemFlock(r, ctx, args[1:])
	}
	r.Diagnosef("unknown subcommand: %s\n", args[0])
	return 1
}

// zsystemSupports is `zsystem supports <name>`: the status is the whole
// answer and nothing is printed.
//
// **255, not 1, for the wrong number of operands.** Measured, and it is the
// only status like it in the module — every other refusal here is 1 — which is
// why it is pinned rather than rounded to the neighbors. A script writing
// `zsystem supports` with the name it meant to pass left empty gets a status
// no ordinary failure produces.
func zsystemSupports(r *interp.Runner, args []string) int {
	switch {
	case len(args) == 0:
		r.Diagnosef("supports: not enough arguments\n")
		return 255
	case len(args) > 1:
		r.Diagnosef("supports: too many arguments\n")
		return 255
	}
	// `supports` is one of the two, which reads as a curiosity and is the
	// honest answer: it is the subcommand vocabulary being reported, not a
	// list of features, and a script may reasonably ask whether the question
	// can be asked at all.
	if args[0] == "supports" || (args[0] == "flock" && systemLockSupported) {
		return 0
	}
	return 1
}

// zsystemFlockOpts is what `flock`'s letters asked for.
type zsystemFlockOpts struct {
	unlock   bool          // -u
	read     bool          // -r
	variable string        // -f
	timeout  time.Duration // -t
	timed    bool
	interval time.Duration // -i
}

// zsystemFlock is `zsystem flock [-r] [-t timeout] [-i interval] [-f var]
// file` and `zsystem flock -u fd`.
func zsystemFlock(r *interp.Runner, ctx context.Context, args []string) int {
	opts, rest, code := zsystemFlockOptions(r, args)
	if code != 0 {
		return code
	}
	if len(rest) == 0 {
		r.Diagnosef("flock: not enough arguments\n")
		return 1
	}
	if len(rest) > 1 {
		r.Diagnosef("flock: too many arguments\n")
		return 1
	}
	if opts.unlock {
		return zsystemUnlock(r, rest[0])
	}
	return zsystemLock(r, ctx, opts, rest[0])
}

// zsystemFlockOptions reads `flock`'s option words.
//
// Not systemOptions, and the reason is in the wordings rather than in the
// grammar: every refusal this subcommand makes is prefixed with the
// subcommand's own name — `flock: unknown option: x` where `sysopen` says
// `bad option: -q` — and `-f` and `-i` each have a sentence of their own.
// Measured one at a time; a shared reader would have to carry three wording
// sets to say them.
func zsystemFlockOptions(r *interp.Runner, args []string) (zsystemFlockOpts, []string, int) {
	opts := zsystemFlockOpts{interval: systemLockPollDefault}
	rest := args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && len(rest[0]) > 1 {
		word := rest[0]
		rest = rest[1:]
		if word == "--" {
			break
		}
		for i := 1; i < len(word); i++ {
			switch letter := word[i]; letter {
			case 'u':
				opts.unlock = true
			case 'r':
				opts.read = true
			case 'e':
				// The default, and written by one of the four real call
				// sites, so it is taken rather than refused.
				opts.read = false
			case 'f':
				value, ok := systemLetterValue(word, &rest, i)
				// A value that begins with a dash is not one, measured:
				// `zsystem flock -f -- file` and `-f -x file` are each
				// `option f requires a variable name`, where `-f 5 file` is
				// taken and puts the descriptor on a parameter named `5`. So
				// what is refused is a word that looks like the *next option*
				// rather than a word that is not an identifier.
				if !ok || strings.HasPrefix(value, "-") {
					r.Diagnosef("flock: option f requires a variable name\n")
					return opts, nil, 1
				}
				opts.variable = value
				i = len(word)
			case 't', 'i':
				value, ok := systemLetterValue(word, &rest, i)
				if !ok {
					r.Diagnosef("flock: option %c requires a numeric value\n", letter)
					return opts, nil, 1
				}
				if code := zsystemFlockSeconds(r, &opts, letter, value); code != 0 {
					return opts, nil, code
				}
				i = len(word)
			default:
				r.Diagnosef("flock: unknown option: %c\n", letter)
				return opts, nil, 1
			}
		}
	}
	return opts, rest, 0
}

// zsystemFlockSeconds reads `-t` or `-i`'s value, which is an amount of
// **seconds** and is an arithmetic expression rather than a literal.
//
// Measured, and the expression half is what explains the rest of it: `zsystem
// flock -t /tmp/f` is `bad math expression: operand expected`, `-t 1+1` is two
// seconds, and `-t n` is whatever `n` holds — so `-t bogus` is *zero* rather
// than a refusal, because a bare name in arithmetic is an unset variable and
// an unset variable is 0. A plain number is read as one first, since this
// shell's arithmetic is integer-valued where zsh's is not and `-t 0.5` is a
// real half-second wait in both.
//
// The two letters part on what they will accept. An interval must be
// **positive**: `-i 0` and `-i -1` are each `invalid interval value` with the
// word quoted as it was written, which is also what `-i bogus` gets, by the
// same route — the expression is zero and zero is not an interval. A timeout
// has no such rule, and zero is its most useful value: it is what "ask, do not
// wait" is spelled as.
func zsystemFlockSeconds(r *interp.Runner, opts *zsystemFlockOpts, letter byte, text string) int {
	seconds, err := strconv.ParseFloat(text, 64)
	if err != nil {
		n, ok := r.ArithValue(text)
		if !ok {
			// The expression itself failed, and ArithValue has already said
			// so as the shell rather than as this builtin — which is what
			// zsh does with it too: the diagnostic names no subcommand.
			return 1
		}
		seconds = float64(n)
	}
	d := time.Duration(seconds * float64(time.Second))
	if letter == 'i' {
		if d <= 0 {
			r.Diagnosef("flock: invalid interval value: '%s'\n", text)
			return 1
		}
		opts.interval = d
		return 0
	}
	opts.timeout, opts.timed = d, true
	return 0
}

// zsystemLock opens the file and takes the lock, and is where the three
// statuses of a failure are decided. See the note at the top of this file.
func zsystemLock(r *interp.Runner, ctx context.Context, opts zsystemFlockOpts, name string) int {
	if !systemLockSupported {
		// A platform with no record locking. Nothing is opened and nothing is
		// claimed, which is the answer `zsystem supports flock` has already
		// given a script that asked first.
		r.Diagnosef("flock: not supported on this system\n")
		return 1
	}
	direction, verb := "writing", false
	if opts.read {
		direction, verb = "reading", true
	}
	path := shellPath(r, name)
	// A lock is taken by opening the file, so it is an open and goes through
	// the gate like one (#1805). Recorded as a write unless `-r` asked for a
	// read lock, which is the access it actually makes.
	action, ok := r.AllowOpen(ctx, path, !verb)
	if !ok {
		return 1
	}
	f, err := systemLockOpen(path, verb)
	if err != nil {
		// No `flock:` on this one, measured: the sentence names the file and
		// what it was to be opened for, which is already more than the
		// subcommand's name would add.
		r.Diagnosef("failed to open %s for %s: %s\n", name, direction, sysErrnoText(r, err))
		return 1
	}
	if !r.VerifyOpened(ctx, &action, f) {
		_ = f.Close()
		return 1
	}
	held, err := systemLockTake(int(f.Fd()), opts.read, !opts.timed, opts.timeout, opts.interval)
	if !held {
		_ = f.Close()
		if opts.timed && opts.timeout > 0 {
			// The wait ran out. **Nothing is said**, which is what lets a
			// guard written `zsystem flock -t 1 … || fallback` run in a
			// prompt without putting a line on the terminal every time the
			// lock is busy.
			return 2
		}
		r.Diagnosef("failed to lock file %s: %s\n", name, sysErrnoText(r, err))
		return 1
	}
	fd := r.OpenDescriptor(f)
	// The shell's plumbing rather than the script's: a child inheriting it
	// would hold the file open past the shell that locked it, and the lock is
	// given up when the last descriptor on it in this process goes.
	r.KeepDescriptorFromChildren(fd)
	r.SetArray(systemLockStore, append(zsystemLockHeld(r), strconv.Itoa(fd)))
	if opts.variable != "" {
		r.SetDescriptorVariable(opts.variable, fd)
	}
	return 0
}

// zsystemUnlock is `zsystem flock -u fd`: release the lock and give the
// descriptor up.
//
// The refusal is measured and so is the number in it: `zsystem flock -u bogus`
// is `file descriptor 0 not in use for locking`, because the operand is read
// as a number and a word that is not one reads as zero. That is the same rule
// `-t` follows two functions up, and it is reproduced for the same reason.
func zsystemUnlock(r *interp.Runner, text string) int {
	fd, err := strconv.Atoi(text)
	if err != nil {
		fd = 0
	}
	held := zsystemLockHeld(r)
	at := -1
	for i, entry := range held {
		if entry == text || entry == strconv.Itoa(fd) {
			at = i
			break
		}
	}
	if at < 0 {
		r.Diagnosef("flock: file descriptor %d not in use for locking\n", fd)
		return 1
	}
	if sys, ok := r.SystemDescriptor(fd); ok {
		// Released explicitly and then closed. The close alone would do it —
		// a record lock goes when the last descriptor on the file in this
		// process does — but this shell's subshells are clones in one process
		// rather than forks, so "the last descriptor" is a claim about a
		// table this function does not own. Unlocking first is the half that
		// does not depend on it.
		systemLockRelease(sys)
	}
	r.CloseDescriptor(fd)
	r.SetArray(systemLockStore, append(append([]string(nil), held[:at]...), held[at+1:]...))
	return 0
}

// zsystemLockHeld is the descriptors this shell has locks on.
func zsystemLockHeld(r *interp.Runner) []string {
	held, _ := r.GetArray(systemLockStore)
	return held
}
