// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// This file implements `exec`, the third of the four builtins specialBuiltins
// called special before any of them existed.
//
// `exec` is two commands sharing a name, and they have almost nothing to do
// with each other:
//
//	exec > log        keep the redirections and carry on
//	exec cmd args     run cmd instead of this shell, and do not carry on
//
// The first is pure runner state. The second is the interesting one, and the
// interesting part is not shell semantics — it is that this package is a
// library.

func init() {
	// Registered here rather than in the builtins literal for the same reason
	// eval and `.` are: it reaches the dispatcher that reads that map.
	builtins["exec"] = biExec
}

// biExec replaces this shell with a command, or keeps its redirections.
//
// With no words at all it is the redirection form: the redirections were
// already applied by the caller, and the whole of the work is telling the
// caller not to put them back. Measured, and unanimous: `exec > f; echo one;
// echo two` leaves both lines in f, and a `trap … EXIT` still fires at the end
// because the shell is still there.
func biExec(r *Runner, ctx context.Context, args []string) int {
	if len(args) == 0 {
		// Not r.status: `false; exec; echo $?` reports 0 in every shell in
		// the panel, so the redirection form clears a failure the way an
		// empty eval does.
		r.keepRedirs = true
		return 0
	}
	return r.replaceSelf(ctx, args)
}

// replaceSelf runs a command in place of this shell.
//
// **This package is a library, and that decides the default.** A real shell
// calls execve and becomes the command: same pid, same signal dispositions,
// nothing of the shell left. A library cannot do that behind its embedder's
// back — a program using a Runner to interpret a script would have its own
// process replaced by whatever that script named, which is not a shell feature
// but a way to lose a program. The same reasoning the core's `cd` uses for not
// calling os.Chdir applies here with much larger consequences.
//
// So the default is to run the command as a child with this shell's streams
// and then stop the script with its status, and ReplaceProcess opts into the
// real thing. What differs between them is narrow and worth naming: the pid
// (`$$` survives a real exec and a child gets its own), the signal
// dispositions, and being the process the *parent* waits for. What does not
// differ is anything the conformance harness can observe — the output, the
// status, and that nothing after it runs.
func (r *Runner) replaceSelf(ctx context.Context, argv []string) int {
	// `exec -a name cmd` and friends: bash, ksh93 and zsh take options here
	// and dash takes none, so a leading `-a` is a command called "-a" there.
	// The axis is asked before the lookup, because which word is the command
	// depends on the answer.
	argv, flags, code := r.execOptions(argv)
	if code != 0 {
		return code
	}
	if len(argv) == 0 {
		// Every option and no command, which is the redirection form again.
		r.keepRedirs = true
		return 0
	}
	if r.restricted {
		// A restricted shell will not be replaced, and it says so before it
		// looks: measured, `exec /bin/sh`, `exec sh` and `exec nosuchcmd`
		// are one sentence at 1, so neither the slash nor whether the
		// command exists is reached. The builtin is named and the operand is
		// not — there is nothing to say about a word that was never looked
		// up.
		//
		// The redirection form above is untouched, which is the measurement
		// and is also the only reading that makes sense: `exec 3< f` in a
		// restricted shell is silent at 0, because the mode is about the
		// shell being replaced and not about `exec`. What the mode does stop
		// is `exec > f`, and it stops it in applyRedirs as an output
		// redirection like any other rather than here.
		return r.restrictedRefusal("exec")
	}

	path, lookErr := r.lookPath(argv[0])
	if lookErr != nil {
		path = argv[0]
	}
	action := r.act(Action{Kind: ActionExec, Path: path, Args: argv})
	if !r.allowed(ctx, action) {
		// A denied action is a command that failed rather than a broken
		// shell, and that is true here too: a refused `exec` leaves the shell
		// running, because the alternative is a gate that can end the script.
		return r.status
	}
	if lookErr != nil {
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: lookErr})
		return r.execFailed(lookErr)
	}

	// A subshell must never replace the *process*.
	//
	// `( exec echo hi ); echo after` prints both lines in every shell in the
	// panel, because there the subshell is a separate process and exec
	// replaces only that one. Here a subshell is a cloned Runner inside the
	// same process, so calling execve in one would replace the parent shell
	// too — the `echo after` disappeared, and so would everything after it in
	// a real script. The same holds for a pipeline element, which is a
	// subshell by another name.
	//
	// Running it as a child and ending only this Runner is what a separate
	// process would have looked like from outside, which is the whole of what
	// a subshell has to preserve.
	//
	// A stream that is not one descriptor takes the same road, for the same
	// reason read from the other end: a replacement is handed *numbers*, and
	// the dialect that writes to every target of a repeated redirection leaves
	// `exec >a >b` a writer over two files, which is no number at all. See
	// namedStreamsCanBePlaced.
	if r.ReplaceProcess != nil && !r.inSubshell && r.namedStreamsCanBePlaced() {
		// The embedder has said this process is a shell and may stop being
		// one. Nothing after this line runs if it succeeds.
		r.emit(ctx, Event{Kind: EventCommandStart, Action: action})
		// The descriptor table crosses here for the reason it crosses to an
		// external child: a replacement is what the script parked those
		// descriptors *for*. `exec 3>h; exec /bin/sh -c 'echo repl >&3'` said
		// "Bad file descriptor" where every shell in the panel but ksh93
		// writes, because Go opens everything close-on-exec and a Runner's
		// descriptor 3 is not the process's 3. Placing them is the hook's
		// half — it is the process's own table being rewritten, which a
		// library may not touch.
		//
		// The named streams cross in the same slice rather than separately,
		// which is the one thing this route does not share with a child: an
		// external command has its 0, 1 and 2 built by os/exec, and a
		// replacement has only the numbers this process is holding at the
		// moment of the execve. So `exec >log; exec /bin/echo hi` wrote to
		// the terminal, past a redirection the script had already made.
		// This shell's last act, so what it made for itself goes now:
		// nothing after a successful execve is this shell, and Finish is not
		// on this road. The pipes of the command carrying the `exec` are
		// unlinked with it, which is what removeProcSubs would have done at
		// the end of that command had there been an end — a name going away
		// under an open descriptor is the ordinary case here, not a race.
		r.cleanUpAtEnd()
		// And the mask goes onto the process for the same reason it goes on
		// around a fork: an execve keeps the mask the image had, and this
		// shell's is not the process's. Taken off again on the way back,
		// which is only reached when the replacement failed. See
		// umaskscope.go.
		releaseMask := r.holdMaskForFork()
		// replacementEnviron rather than execEnviron: two columns take this
		// shell back out of the depth count on the way over, so the program
		// that stands in its place is not one deeper than it was. See
		// Semantics.ShellLevelExec — the fallback below is a child and keeps
		// the count, which is measured and not an omission.
		err := r.ReplaceProcess(path, r.execArgv(argv, flags), r.replacementEnviron(flags), r.replacementFiles())
		releaseMask()
		// Only reached if the replacement failed, which is the one case where
		// there is still a shell to report it.
		//
		// A file the kernel will not start may still be a shell script — see
		// noexecscript.go, and execImageAsScript for why the same helper
		// answers both this door and a command word's.
		if st, ran := r.execImageAsScript(ctx, action, path, argv, r.execEnviron(flags), err); ran {
			return st
		}
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
		// A start that failed rather than a lookup that did: wrapped so the
		// shared reporter has a name and a resolved path to work from. The
		// resolved path is also what puts this on the pathname side of the
		// exit-trap question, which is right — the file was found.
		// The interpreter a `#!` line named, for the dialect that reads the
		// file to tell "this command is not there" from "the program its
		// first line names is not there" — see Runner.interpreterNamed. Both
		// doors ask, because `exec ./x` and `./x` word it identically.
		return r.execFailed(&pathError{
			name: argv[0], resolved: path,
			interpreter: r.interpreterNamed(ctx, path, err), err: err,
		})
	}

	r.emit(ctx, Event{Kind: EventCommandStart, Action: action})
	cmd := exec.CommandContext(ctx, path, argv[1:]...)
	// The name the command finds in argv[0]: `-a` where it was given, and
	// otherwise the word that was typed. os/exec puts the resolved path
	// there, which is the one thing it should never be.
	cmd.Args[0] = r.execArgv(argv, flags)[0]
	cmd.Dir = r.Dir
	cmd.Env = r.execEnviron(flags)
	cmd.Stdin = r.childStdin()
	cmd.Stdout = childOut(r.stdout())
	cmd.Stderr = childOut(r.stderr())
	// Standing in for a process replacement means standing in for what one
	// inherits, so the descriptor table crosses here as it does for any other
	// external command.
	cmd.ExtraFiles = r.childFiles()

	if err := r.startMasked(cmd); err != nil {
		if st, ran := r.execImageAsScript(ctx, action, path, argv, cmd.Env, err); ran {
			return st
		}
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
		// A start that failed rather than a lookup that did: wrapped so the
		// shared reporter has a name and a resolved path to work from. The
		// resolved path is also what puts this on the pathname side of the
		// exit-trap question, which is right — the file was found.
		// The interpreter a `#!` line named, for the dialect that reads the
		// file to tell "this command is not there" from "the program its
		// first line names is not there" — see Runner.interpreterNamed. Both
		// doors ask, because `exec ./x` and `./x` word it identically.
		return r.execFailed(&pathError{
			name: argv[0], resolved: path,
			interpreter: r.interpreterNamed(ctx, path, err), err: err,
		})
	}
	status := r.exitStatus(cmd.Wait())
	r.emit(ctx, Event{Kind: EventCommandEnd, Action: action, Status: status})

	// The script stops, and the EXIT trap does *not* run. That is unanimous
	// across the panel and it is not a special case there: the trap died with
	// the process the exec replaced. Standing in a child for that replacement
	// means saying so explicitly, or `trap 'echo T' EXIT; exec echo hi` prints
	// a T that no real shell prints.
	r.status = status
	r.exitTrap = nil
	r.stopTheShell()
	return status
}

// namedStreamsCanBePlaced reports whether 0, 1 and 2 can be handed to a
// replacement as descriptor numbers.
//
// A replacement has no renumbering step: it becomes the command in this
// process, so each stream has to *be* a descriptor before the execve. Almost
// everything a stream can be still crosses on the table's own rules — a file
// is placed, and anything that is not one arrives as a nil, which is a number
// that must not be open there. `exec >&-; exec cmd` leaves the command a
// closed descriptor in all five shells, and an embedder's buffer has no
// number to hand over either.
//
// One shape breaks that reading, and it is the shell's own doing. Under the
// dialect that writes to *every* target of a repeated redirection, `exec >a
// >b` leaves standard output a writer over two files. Read as "not a file,
// therefore nil, therefore closed", the command found standard output closed
// and failed — `echo: fflush: Bad file descriptor` — where that shell writes
// `hi` into both.
//
// So the replacement is declined for exactly that stream, and the shell
// stands in for it with the child route it already has for a subshell:
// os/exec gives a stream that is not a file a pipe and copies from it, so
// both files get the bytes. What that costs is written down where the child
// route is — the pid, the signal dispositions, and being the process the
// parent waits for. Measured, the shell whose behavior this reproduces spends
// a process on it too: it forks a copier and keeps its own pid for the
// command, where we keep the pid for the shell and give the command a new
// one. The number of processes agrees; which of them is the command does not.
//
// The test is the marker rather than the type, and that is the whole of why
// multiTarget exists: "not an *os.File" would sweep in an embedder's buffer,
// which is a different case whose answer — a closed number — is measured and
// deliberate, and which no child route could improve on anyway.
//
// Only the named streams are asked about. A *numbered* descriptor with
// several targets is not this question: it is not modeled here at all, in a
// replacement or out of one, and no route through os/exec would carry it
// either, because ExtraFiles is files.
func (r *Runner) namedStreamsCanBePlaced() bool {
	return !isMultiTarget(r.Stdin) &&
		!isMultiTarget(r.Stdout) &&
		!isMultiTarget(r.Stderr)
}

// isMultiTarget reports whether a stream is one the shell built out of
// several targets.
func isMultiTarget(v any) bool {
	_, ok := v.(multiTarget)
	return ok
}

// execCannotRun reports an exec that could not happen.
//
// The wording is shared with an ordinary command, because the panel words the
// two identically in every case but one: a bare name that was never found is
// "command not found" from a command word and "exec: name: not found" here.
func (r *Runner) execCannotRun(err error) int {
	// One dialect calls a command that could not be found the shell's failure
	// rather than the builtin's: `exec nosuchcmd` is `zsh:1: command not
	// found: nosuchcmd`, with no `exec` segment — exactly what a bare command
	// word reports. The message still names exec wherever the dialect's
	// wording does; it is the *location* that must not.
	//
	// A dialect's answer and not a rule, which the second shell to name a
	// builtin in its location settled: BusyBox ash writes `./e.sh: exec: line
	// 1: /nonexistent/x: not found` and keeps it. See
	// Diagnostics.ExecNotFoundIsTheShellsOwn (#2761).
	if r.diag().ExecNotFoundIsTheShellsOwn {
		outer := r.inBuiltin
		r.inBuiltin = ""
		defer func() { r.inBuiltin = outer }()
	}
	return r.cannotRun(err, naming{
		bare:          r.diag().ExecNotFound,
		fallback:      "exec: %[1]s: not found",
		absolute:      true,
		cannotExecute: r.diag().ExecCannotExecute,
	})
}

// execFailed reports an exec that could not happen and stops the script.
//
// The two halves are together here because the second reads the *error* the
// first was given: which EXIT-trap axis applies depends on whether a pathname
// was ever arrived at, and that is a property of the failure rather than of
// the call site. See execEnds.
func (r *Runner) execFailed(err error) int {
	return r.execEnds(err, r.execCannotRun(err))
}

// execEnds stops the script after an exec that could not happen.
//
// It always stops it — `exec nosuchcmd; echo REACHED` prints nothing in any
// shell in the panel, so unlike almost everything else here that is not an
// axis. Whether the EXIT trap runs on the way out is **two** axes, because
// bash answers the two halves of the failure differently: it runs the trap
// when the PATH search came up with nothing and drops it when a file was
// named and would not start. dash and BusyBox ash run it either way, ksh93
// and zsh drop it either way. See Semantics.ExecFailureRunsExitTrap and
// Semantics.ExecFailureOnAPathnameRunsExitTrap (#3983).
func (r *Runner) execEnds(err error, status int) int {
	axis, what := r.sem().ExecFailureRunsExitTrap,
		"an EXIT trap running after an `exec` whose name PATH did not have"
	if execNamedAFile(err) {
		axis, what = r.sem().ExecFailureOnAPathnameRunsExitTrap,
			"an EXIT trap running after an `exec` that could not start the file it named"
	}
	if !r.ask(axis, what) {
		r.exitTrap = nil
	}
	r.status = status
	r.stopTheShell()
	return status
}

// execNamedAFile reports whether a failed exec ever arrived at a pathname.
//
// **A resolved path is the whole of the question**, which is what lets one
// predicate stand for three different roads into the same failure: an operand
// with a slash in it, which never consults PATH; a search that kept a
// candidate it could not run; and a lookup that succeeded and an execve that
// did not. The remaining road — a bare name PATH had nothing for — is the one
// that leaves resolved empty, and it is the one the other axis answers.
//
// Not "the operand contains a slash", which is the reading #3983 was filed
// with. A non-executable file *found on PATH* has no slash in its operand and
// bash drops the trap for it; a directory found on PATH has no slash either
// and bash runs the trap, because bash reports that as nothing found at all.
// Both fall out of the resolved path and neither falls out of the slash.
func execNamedAFile(err error) bool {
	var pe *pathError
	return errors.As(err, &pe) && pe.resolved != ""
}

// orElse is the empty-means-fall-back rule these wording fields use, in one
// place rather than at each call site.
func orElse(preferred, fallback string) string {
	if preferred != "" {
		return preferred
	}
	return fallback
}

// execOptions takes the options `exec` accepts, which is a dialect question.
//
// bash, ksh93 and zsh take `-a name`, and dash takes nothing — measured:
// `exec -a myname sh -c 'echo $0'` prints myname in three of them and is
// "exec: -a: not found" in dash. So in dash a leading dash-word is the command
// and must not be eaten here.
//
// It returns the remaining words, the argv[0] override if one was given, and a
// status if the words could not be read at all.
func (r *Runner) execOptions(argv []string) (rest []string, flags execFlags, code int) {
	if len(argv) == 0 || !strings.HasPrefix(argv[0], "-") || argv[0] == "-" {
		return argv, flags, 0
	}
	if !r.ask(r.sem().ExecTakesOptions, "`exec` taking options of its own") {
		// Not an error: the word is the command, and looking it up will
		// produce the dialect's own "not found" for it.
		return argv, flags, 0
	}
	for len(argv) > 0 && strings.HasPrefix(argv[0], "-") && argv[0] != "-" {
		switch argv[0] {
		case "--":
			return argv[1:], flags, 0
		case "-a":
			if len(argv) < 2 {
				r.diagf("exec: -a: %s\n", "option requires an argument")
				return nil, flags, 2
			}
			flags.argv0, argv = argv[1], argv[2:]
			continue
		case "-l":
			if !r.ask(r.sem().ExecTakesTheLoginLetter, "`exec -l`") {
				return nil, flags, r.execBadOption(argv[0])
			}
			flags.login = true
		case "-c":
			if !r.ask(r.sem().ExecTakesTheEmptyEnvironmentLetter, "`exec -c`") {
				return nil, flags, r.execBadOption(argv[0])
			}
			flags.clearEnv = true
		default:
			return nil, flags, r.execBadOption(argv[0])
		}
		argv = argv[1:]
	}
	return argv, flags, 0
}

// execBadOption is a letter this dialect's `exec` has not got, reported the
// way every other builtin reports one — the dialect's wording, its usage
// line, and its rule about a special builtin's failure ending the script,
// which `exec` is in dash and ksh93.
//
// It replaced a sentence of this package's own that three dialects printed
// and none of them writes (#3056): ksh93 says `exec: -l: unknown option` and
// then its usage line, and the script stops there.
func (r *Runner) execBadOption(opt string) int {
	if r.unspecified {
		return r.status
	}
	return r.badBuiltinOption("exec", opt)
}

// execFlags is what `exec`'s own options asked for, gathered rather than
// passed one at a time: they arrive together and are spent together, at the
// two seams a replacement has — its argument vector and its environment.
type execFlags struct {
	// argv0 is `-a name`, the name the replacement finds in argv[0].
	argv0 string
	// login is `-l`, which marks the argv[0] as a login shell's.
	login bool
	// clearEnv is `-c`, which hands the replacement no environment.
	clearEnv bool
}

// argv builds the argument vector a replacement receives.
//
// The two letters meet here, and they do not compose the same way in the two
// shells that have both: bash puts the `-` on the name `-a` chose, and zsh
// lets `-a` win outright — in either order, so it is not a rule about which
// was written last. See Semantics.ExecLoginPrefixesTheGivenName.
//
// The prefix goes on the word as written, path and all, which is what the
// reference shells do: `exec -l /bin/sh` hands over `-/bin/sh`.
func (r *Runner) execArgv(argv []string, flags execFlags) []string {
	name := argv[0]
	switch {
	case flags.argv0 != "" && flags.login:
		if r.ask(r.sem().ExecLoginPrefixesTheGivenName, "`exec -l -a name`") {
			name = "-" + flags.argv0
		} else {
			name = flags.argv0
		}
	case flags.argv0 != "":
		name = flags.argv0
	case flags.login:
		name = "-" + name
	}
	if name == argv[0] {
		return argv
	}
	out := append([]string(nil), argv...)
	out[0] = name
	return out
}

// execEnviron is the environment a replacement is handed: this shell's,
// unless `-c` asked for none.
//
// An empty slice and not nil, which is the difference between "no variables"
// and "whatever the process happens to hold": os/exec reads a nil Env as the
// caller's own environment, so `exec -c` written that way would have handed
// over everything and looked like it worked.
func (r *Runner) execEnviron(flags execFlags) []string {
	if flags.clearEnv {
		return []string{}
	}
	return r.environ()
}
