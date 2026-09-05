// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
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
	argv, argv0, code := r.execOptions(argv)
	if code != 0 {
		return code
	}
	if len(argv) == 0 {
		// Every option and no command, which is the redirection form again.
		r.keepRedirs = true
		return 0
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
		return r.execEnds(r.execCannotRun(lookErr))
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
	if r.ReplaceProcess != nil && !r.inSubshell {
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
		err := r.ReplaceProcess(path, withArgv0(argv, argv0), r.environ(), r.replacementFiles())
		// Only reached if the replacement failed, which is the one case where
		// there is still a shell to report it.
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
		// A start that failed rather than a lookup that did: wrapped so the
		// shared reporter has a name and a resolved path to work from.
		return r.execEnds(r.execCannotRun(
			&pathError{name: argv[0], resolved: path, err: err}))
	}

	r.emit(ctx, Event{Kind: EventCommandStart, Action: action})
	cmd := exec.CommandContext(ctx, path, argv[1:]...)
	// The name the command finds in argv[0]: `-a` where it was given, and
	// otherwise the word that was typed. os/exec puts the resolved path
	// there, which is the one thing it should never be.
	cmd.Args[0] = argv[0]
	if argv0 != "" {
		cmd.Args[0] = argv0
	}
	cmd.Dir = r.Dir
	cmd.Env = r.environ()
	cmd.Stdin = r.Stdin
	cmd.Stdout = r.stdout()
	cmd.Stderr = r.stderr()
	// Standing in for a process replacement means standing in for what one
	// inherits, so the descriptor table crosses here as it does for any other
	// external command.
	cmd.ExtraFiles = r.childFiles()

	if err := cmd.Start(); err != nil {
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
		// A start that failed rather than a lookup that did: wrapped so the
		// shared reporter has a name and a resolved path to work from.
		return r.execEnds(r.execCannotRun(
			&pathError{name: argv[0], resolved: path, err: err}))
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
	r.ctl = controlExit
	return status
}

// execCannotRun reports an exec that could not happen.
//
// The wording is shared with an ordinary command, because the panel words the
// two identically in every case but one: a bare name that was never found is
// "command not found" from a command word and "exec: name: not found" here.
func (r *Runner) execCannotRun(err error) int {
	// A command that could not be found is the shell's failure rather than
	// the builtin's, and the one dialect that names a builtin in the location
	// agrees: `exec nosuchcmd` is `zsh:1: command not found: nosuchcmd`, with
	// no `exec` segment — exactly what a bare command word reports. The
	// message still names exec wherever the dialect's wording does; it is the
	// *location* that must not, which is the rule Diagnostics already states
	// and this was the one place not following it.
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()
	return r.cannotRun(err, naming{
		bare:          r.diag().ExecNotFound,
		fallback:      "exec: %[1]s: not found",
		absolute:      true,
		cannotExecute: r.diag().ExecCannotExecute,
	})
}

// execEnds stops the script after an exec that could not happen.
//
// It always stops it — `exec nosuchcmd; echo REACHED` prints nothing in any
// shell in the panel, so unlike almost everything else here that is not an
// axis. Whether the EXIT trap runs on the way out *is* one: dash and bash run
// it, ksh93 and zsh drop it.
func (r *Runner) execEnds(status int) int {
	if !r.ask(r.sem().ExecFailureRunsExitTrap, "an EXIT trap running after a failed exec") {
		r.exitTrap = nil
	}
	r.status = status
	r.ctl = controlExit
	return status
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
func (r *Runner) execOptions(argv []string) (rest []string, argv0 string, code int) {
	if len(argv) == 0 || !strings.HasPrefix(argv[0], "-") || argv[0] == "-" {
		return argv, "", 0
	}
	if !r.ask(r.sem().ExecTakesOptions, "`exec` taking options of its own") {
		// Not an error: the word is the command, and looking it up will
		// produce the dialect's own "not found" for it.
		return argv, "", 0
	}
	for len(argv) > 0 && strings.HasPrefix(argv[0], "-") && argv[0] != "-" {
		switch argv[0] {
		case "--":
			return argv[1:], argv0, 0
		case "-a":
			if len(argv) < 2 {
				r.diagf("exec: -a: %s\n", "option requires an argument")
				return nil, "", 2
			}
			argv0, argv = argv[1], argv[2:]
		case "-c", "-l":
			// Accepted and not implemented, which is a real answer rather
			// than a pretense: -c would need an empty environment and -l a
			// login argv[0], and neither is measured by anything here yet.
			// Silently ignoring them would be the pretense.
			r.diagf("exec: %s is not implemented\n", argv[0])
			return nil, "", 2
		default:
			r.diagf("exec: %s: invalid option\n", argv[0])
			return nil, "", 2
		}
	}
	return argv, argv0, 0
}

// withArgv0 builds the argument vector a replacement receives, with argv[0]
// overridden if `-a` asked for it.
func withArgv0(argv []string, argv0 string) []string {
	if argv0 == "" {
		return argv
	}
	out := append([]string(nil), argv...)
	out[0] = argv0
	return out
}
