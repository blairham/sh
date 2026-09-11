// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
)

// The two ways to ask for a command without asking a function.
//
// `command name` runs the builtin or the external and never the function of
// that name, which is what lets a function wrap the thing it is named after —
// `ls() { command ls --color "$@"; }` is the whole reason it exists.
//
// `command -v name` asks *what* would run rather than running it, and is how a
// portable script tests whether it has a tool. It is unanimous across the
// panel except for the status when the answer is nothing.

// Registered here rather than in the table beside the others: `command` and
// `builtin` both reach the dispatcher that reads that table, and Go calls a
// literal closing that loop an initialization cycle. `eval` and `.` are added
// the same way and for the same reason.
func init() {
	builtins["command"] = biCommand
	builtins["builtin"] = biBuiltin
}

// biCommand runs a command with functions bypassed, or reports what one is.
// commandOptionLetters reports the letters in a bundle when every one of
// them is an option `command` has, so a word is either wholly understood or
// wholly a question for the dialect.
func commandOptionLetters(word string) (string, bool) {
	letters := word[1:]
	for i := range len(letters) {
		if letters[i] != 'v' && letters[i] != 'V' && letters[i] != 'p' {
			return "", false
		}
	}
	return letters, letters != ""
}

func biCommand(r *Runner, ctx context.Context, args []string) int {
	// Through the shared reader rather than a loop of its own, which is what
	// this had. That loop named the whole word — `command --version` came
	// back as `--version: invalid option` where every shell names `--`,
	// because a leading `-` word is a bundle and only its first letter is
	// refused — and it used neither the dialect's wording nor its status.
	//
	// `-p` means "use a default PATH". Ours is already the Runner's rather
	// than the process's, and inventing a second would be a guess about this
	// machine, so it is accepted and changes nothing — which is what it
	// means for a shell that never had the developer's PATH to begin with.
	// `-v` and `-p` are read by all four. What splits them is a leading `-`
	// word that is *not* one of those: bash, dash and ksh93 refuse it as an
	// option, and zsh alone stops reading options and takes it as the
	// command, so `command -q ls` is `command not found: -q` there. Probed
	// with a letter no panel shell owns — `-x` is a real ksh93 option, and
	// the first measurement read ksh93 off it, wrongly.
	verbose, sentence := false, false
	for len(args) > 0 {
		a := args[0]
		if len(a) < 2 || a[0] != '-' {
			break
		}
		if a == "--" {
			args = args[1:]
			break
		}
		if letters, ok := commandOptionLetters(a); ok {
			verbose = verbose || strings.ContainsRune(letters, 'v')
			// `-V` answers the same question as a sentence — POSIX gives
			// both letters, and all four shells word it the way their
			// `type` does.
			sentence = sentence || strings.ContainsRune(letters, 'V')
			args = args[1:]
			continue
		}
		if !r.ask(r.sem().CommandRejectsUnknownOption, "`command -q` refused as an option rather than run as the command") {
			break
		}
		if r.unspecified {
			return r.status
		}
		return r.refuseOption("command", a, "vp")
	}
	if len(args) == 0 {
		return 0
	}
	if sentence {
		// `type`'s sentence with `command`'s name on the complaint: the
		// found wordings are shared and only the missing one is this
		// builtin's own — see Diagnostics.CommandVNotFound.
		return r.describeName(args[0], false, false,
			Wording(r.diag().CommandVNotFound, "command: %[1]s: not found", args[0]))
	}
	if verbose {
		return r.reportWhatRuns(args[0])
	}
	// What the command reports is the *command's*, not this builtin's. The
	// dialect that names a builtin in the location says
	// `sh:1: command not found: -x` and not `sh:command:1:` — the same rule
	// `.` and `eval` follow for the text they run, and for the same reason.
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()
	return r.runWithoutFunctions(ctx, args)
}

// biBuiltin runs a builtin, and only a builtin.
//
// bash and zsh have it; dash has no such command, and ksh93 has one of the
// same name that does something else entirely — it *registers* builtins — so
// this is registered per dialect rather than being part of the substrate.
func biBuiltin(r *Runner, ctx context.Context, args []string) int {
	if len(args) == 0 {
		return 0
	}
	fn, ok := r.lookupBuiltin(args[0])
	if !ok {
		// The location does not name the builtin here, in the dialect that
		// names it everywhere else: this message is *about* a name that is
		// not one, so there is no builtin speaking. Measured — `zsh:cd:1:`
		// and `zsh:shift:1:` against a plain `zsh:1: no such builtin`.
		// Cleared and not put back: the caller that dispatched this builtin
		// saved the name and restores it either way, so restoring it here
		// would be a line nothing could ever observe.
		r.inBuiltin = ""
		r.diagf("%s\n", Wording(r.diag().NotABuiltin, "builtin: %s: not a shell builtin", args[0]))
		return 1
	}
	// And the *name* is the one that ran, not this wrapper's. The dialect
	// that puts a builtin in the location says `zsh:cd:1: no such file or
	// directory: /nope` and `zsh:shift:1: shift count must be <= $#` for
	// `builtin cd /nope` and `builtin shift 5` alike, where this shell said
	// `zsh:builtin:1:` for both — the word the script wrote to *reach* the
	// builtin standing in for the one it reached. It is on zsh's own autoload
	// path, because the stub a declaration writes is `builtin autoload -X`
	// and every complaint from a resolution came out named after `builtin`
	// (#1968). Saved and put back for the reason the dispatch above does it:
	// a builtin can run another one.
	//
	// The fold names the builtin that wrote, not this wrapper — the same
	// reason runWithoutFunctions folds for itself.
	outer := r.inBuiltin
	r.inBuiltin = args[0]
	defer func() { r.inBuiltin = outer }()
	return r.callBuiltin(ctx, args[0], fn, args[1:])
}

// reportWhatRuns answers `command -v`.
//
// A builtin or a function is named as it was written; an external is named by
// the path that would be run, because that is the part a script cannot work
// out for itself. Nothing found is a failure with no output at all — the
// silence is what makes `command -v x >/dev/null` the usual spelling.
func (r *Runner) reportWhatRuns(name string) int {
	if _, ok := r.funcs[name]; ok {
		r.printf("%s\n", name)
		return 0
	}
	if _, ok := r.lookupBuiltin(name); ok {
		r.printf("%s\n", name)
		return 0
	}
	if reservedWord(name) {
		r.printf("%s\n", name)
		return 0
	}
	// Not a PATH hit for a name this shell reserves. There is an executable
	// called /usr/bin/umask and we refuse to run it, so reporting it here
	// would defeat the guard a careful script writes:
	//
	//	if command -v umask >/dev/null; then umask 077; fi
	//
	// The guard exists to avoid exactly the failure that follows. It has to
	// answer for what will actually run, not for what is on the disk.
	if !r.reservedBuiltin(name) {
		if path, err := r.lookPath(name); err == nil {
			r.printf("%s\n", path)
			return 0
		}
	}
	if r.ask(r.sem().CommandNotFoundStatusIsNotFound, "`command -v` reporting a missing name as not found") {
		// One shell answers with the status a missing *command* has — 127 —
		// rather than a plain failure, which matters to a script that tests
		// the number rather than just the truth of it.
		return 127
	}
	return 1
}

// reservedWord reports whether a name is part of the grammar rather than a
// command. `command -v if` answers `if` in every shell in the panel.
func reservedWord(name string) bool {
	switch name {
	case "if", "then", "else", "elif", "fi", "for", "while", "until", "do",
		"done", "case", "esac", "in", "function", "select", "time", "{", "}",
		"[[", "]]", "!":
		return true
	}
	return false
}

// runWithoutFunctions runs a command with the function table ignored, which
// is the whole of what `command name` means: a function may then wrap the
// thing it is named after without calling itself.
func (r *Runner) runWithoutFunctions(ctx context.Context, args []string) int {
	if fn, ok := r.lookupBuiltin(args[0]); ok {
		outer := r.inBuiltin
		r.inBuiltin = args[0]
		// callBuiltin folds a failed write here as well as at the outer
		// dispatch, so it is blamed on the builtin that wrote:
		// `command echo hi >&-` names `echo`, not `command`. Measured —
		// bash words it identically with and without the wrapper.
		st := r.callBuiltin(ctx, args[0], fn, args[1:])
		r.inBuiltin = outer
		return st
	}
	if err := r.exec(ctx, args, r.environ()); err != nil {
		r.diagf("command: %v\n", err)
		return 1
	}
	return r.status
}
