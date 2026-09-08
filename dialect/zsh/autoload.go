// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blairham/sh/interp"
)

// `autoload` marks a name to be defined from `$fpath` the first time it is
// called.
//
// Measured 2026-09-06 against zsh 5.9.2 under `env -i` with a scratch HOME
// and no startup files. It was `no such builtin: autoload`, and the line a
// real plugin manager writes is
//
//	builtin autoload -Uz is-at-least
//	is-at-least 5.1 && ZI[NEW_AUTOLOAD]=1
//
// so the refusal cost two lines: the declaration and then the call.
//
// **The name becomes a function immediately**, before anything is read.
// Measured: `autoload -Uz myfunc` is silent and 0, and `whence -w myfunc`
// answers `myfunc: function` at once — the stub *is* the record, which is
// why nothing here keeps a separate list of pending names. Calling it is
// what searches `$fpath`, and a name whose file is not there fails **at the
// call**, not at the declaration:
//
//	autoload -Uz nosuchfn      # 0, silent
//	nosuchfn                   # nosuchfn: function definition file not found, 1
//
// The file's contents become the function's *body*, so its `$*` is the
// call's arguments and a second call runs the loaded body rather than
// searching again.
//
// The one place this shows its own workings is `functions NAME` on a name
// that has not been called yet. zsh prints a stub of its own —
// `builtin autoload -XUz`, where `-X` means "replace the function I am
// running in" and the shell then re-enters it — and that re-entry is
// interpreter machinery rather than anything a body can say. The stub here
// says the same thing in the language it has: load, and then call what was
// loaded. The *behavior* is the same and the text is not, which is a
// difference a corpus row records rather than one to paper over.

// autoloadStubBody is what an undefined autoloaded name runs when it is
// called: resolve it, and then call what the resolution defined.
//
// `+X` replaces this very function, so the call that follows reaches the
// loaded body rather than this one — a lookup happens per call, not per
// definition. It cannot recurse: `+X` either replaces the definition or
// fails, and `&&` stops on the failure. A file that defines nothing leaves
// an empty body, which is a call that does nothing rather than a loop.
const autoloadStubBody = "builtin autoload +X %s && %s \"$@\""

// autoloadLetters are the letters this builtin implements, and
// autoloadUnimplemented the ones zsh has and this shell does not.
//
// zsh's set, measured a letter at a time against all fifty-two: it has
// `d k m r R t T U w W X z` and refuses every other one as `bad option`. So
// the split here is between the three that say something this engine can
// answer and the nine that do not:
//
//   - `-U` suppresses alias expansion while the file is read and `-z` picks
//     zsh-style parsing. Both are accepted and neither changes anything: an
//     autoloaded file is parsed with this shell's own grammar, which is
//     zsh's, and aliases are not expanded in it either way. Accepted rather
//     than refused because every real script writes `-Uz` and refusing it
//     would fail the line for a distinction with no effect here.
//   - `-X` and `+X` resolve now rather than at the call.
//   - `-t`/`-T` (trace this function), `-d`/`-k`/`-m` (ksh-style and pattern
//     forms), `-r`/`-R` (resolve the path now, `-R` fatally), `-w`/`-W`
//     (read a compiled `.zwc` file) are named as missing. Each is a thing
//     this shell does not do, and a builtin that took the letter and dropped
//     it would read as one that did.
const (
	autoloadLetters       = "UzX"
	autoloadUnimplemented = "dkmrRtTwW"
)

func registerAutoload(r *interp.Runner) {
	r.Register("autoload", autoloadBuiltin)
}

// autoloadOpts is what the letters asked for.
type autoloadOpts struct {
	// now is `-X` or `+X`: resolve at once instead of at the call.
	now bool
	// plus records which sign `X` was written with, because they name
	// different things: `+X NAME` resolves the name given, and `-X` with no
	// name resolves the function it is running inside.
	plus bool
}

func autoloadBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	opts, rest, code := autoloadOptions(r, args)
	if code != 0 {
		return code
	}
	if opts.now {
		return autoloadResolveNow(r, ctx, opts, rest)
	}
	if len(rest) == 0 {
		return autoloadListing(r)
	}
	for _, name := range rest {
		if autoloadDefined(r, name) {
			// Already a function, so there is nothing to mark: measured,
			// `myfn() { echo body; }; autoload -Uz myfn; myfn` runs the body
			// and is 0, with a file for `myfn` on `$fpath` or without one,
			// and the name does not appear in the bare listing afterwards.
			//
			// It matters more than the corner suggests. A real startup file
			// declares a name it may already have — a plugin manager's
			// `builtin autoload -Uz is-at-least` runs again on every reload —
			// and a declaration that replaced the definition with a stub
			// turned a working function into `function definition file not
			// found` at the next call. The stub is the record, so writing one
			// over a real body is not a note about the name, it is losing it.
			continue
		}
		if !r.DefineFunction(name, autoloadStub(name)) {
			// The stub is this file's own text, so a name it cannot be
			// written around is a name that is not a name.
			r.Diagnosef("not valid in this context: %s\n", name)
			return 1
		}
		autoloadRecord(r, name)
	}
	return 0
}

// autoloadDefined reports whether a name is already a function this builtin
// must leave alone — one with a body of its own rather than one of the stubs
// this builtin wrote.
//
// The exception is the whole of why this is not `FunctionText(name)`: the
// generated stub resolves itself with `+X NAME`, and the name it names is a
// function at that moment — itself. A guard that did not know the difference
// would refuse every autoload the moment it was called.
func autoloadDefined(r *interp.Runner, name string) bool {
	if _, ok := r.FunctionText(name); !ok {
		return false
	}
	return !autoloadPending(r, name)
}

// autoloadStub is the body a name is given until it is called.
func autoloadStub(name string) string {
	return strings.ReplaceAll(autoloadStubBody, "%s", name)
}

// autoloadOptions reads the leading option words. The plus sign is an option
// here as much as the minus, which is the shape `typeset` has and no other
// builtin outside the declarations.
func autoloadOptions(r *interp.Runner, args []string) (opts autoloadOpts, rest []string, code int) {
	rest = args
	for len(rest) > 0 && len(rest[0]) > 1 &&
		(strings.HasPrefix(rest[0], "-") || strings.HasPrefix(rest[0], "+")) {
		word := rest[0]
		rest = rest[1:]
		if word == "--" {
			break
		}
		plus := word[0] == '+'
		for i := 1; i < len(word); i++ {
			letter := word[i]
			switch {
			case letter == 'X':
				opts.now, opts.plus = true, plus
			case strings.IndexByte(autoloadLetters, letter) >= 0:
				// `-U` and `-z`, which this shell answers by parsing the way
				// it already parses.
			case strings.IndexByte(autoloadUnimplemented, letter) >= 0:
				r.Diagnosef("-%c is not implemented yet\n", letter)
				return opts, nil, 1
			default:
				r.Diagnosef("bad option: -%c\n", letter)
				return opts, nil, 1
			}
		}
	}
	return opts, rest, 0
}

// autoloadResolveNow is `-X` and `+X`.
//
// The two signs name different things, measured: `+X NAME` resolves the name
// it is given, and `-X` with no name resolves the function it is running
// inside — which is what the stub uses, and which is `bad autoload` at the
// top level where there is no function to be inside of.
func autoloadResolveNow(r *interp.Runner, ctx context.Context, opts autoloadOpts, names []string) int {
	if !opts.plus {
		// `-X` names no function: it acts on the one it is running inside,
		// and an operand is `bad autoload` rather than a name to resolve —
		// measured, `autoload -X foo` says it at the top level as much as
		// `autoload -X` alone does.
		if len(names) > 0 {
			return autoloadBad(r)
		}
		name, ok := autoloadEnclosingFunction(r)
		if !ok {
			return autoloadBad(r)
		}
		if code := autoloadResolve(r, name); code != 0 {
			return code
		}
		return autoloadRunResolved(r, ctx, name)
	}
	// `+X` with nothing to resolve is silence and 0 — measured, and not the
	// same answer as the minus sign with nothing, which is the tell that the
	// two signs are two commands rather than one with a flag.
	status := 0
	for _, name := range names {
		if autoloadDefined(r, name) {
			// A function that is already there is not resolved over, and the
			// refusal is silent: measured, `myfn() { echo body; }; autoload
			// -Uz +X myfn` is status 1 with nothing on either stream, and
			// `myfn` still runs its own body afterwards. A status rather than
			// silence because `+X` was asked to do something and did not.
			//
			// Only the plus sign asks this. `-X` is the *opposite* case by
			// construction — it replaces the function it is running inside,
			// which is always a function with a body — so a guard shared
			// between the two signs would refuse the only thing `-X` does.
			status = 1
			continue
		}
		if code := autoloadResolve(r, name); code != 0 {
			status = code
		}
	}
	return status
}

// autoloadRunResolved is the second half of `-X`: run what the resolution
// just defined, here, with the arguments the function it replaced was called
// with.
//
// Resolving alone is not what `-X` does, and the difference is a *silent* one
// — which is why it stood. Measured on zsh 5.9 with a file holding
// `print -r -- "BODY args=<$*>"; return 7` and the stub zsh's own plugin
// loaders write,
//
//	myf() { local -a fpath; fpath=( DIR ); builtin autoload -X -Uz; print "AFTER $?" }
//	myf a b
//
// zsh prints `BODY args=<a b>` and then `AFTER 7`, where this shell printed
// `AFTER 0` and nothing else: the name was redefined and the body never ran,
// so every function loaded this way was a no-op that reported success. That
// is what a real startup runs into — `~/.zi/bin/zi.zsh` substitutes its own
// `autoload` and writes exactly this stub for every function a plugin
// autoloads (`ZI[NEW_AUTOLOAD]=1`, taken on every zsh since 5.1), so the
// annexes' hooks, the meta-plugin expander among them, all returned 0 having
// done nothing. The loader then read "the annex declined" and went looking
// for a plugin named after each ice word it had been asked to expand.
//
// Three things the same measurement settles:
//
//   - The arguments are the replaced function's own. `$*` inside the loaded
//     body is `a b`, so this passes r.Params rather than the builtin's
//     operands, which `-X` refuses to have any of.
//   - The call is *nested*, not a replacement of the frame: the body sees the
//     stub's locals — a `local secret=hidden` set before the `-X` is readable
//     from the file's text — and `${#funcstack}` counts three where the stub
//     is one. So this is an ordinary call from inside the builtin and not an
//     unwinding of the caller.
//   - The stub carries on afterwards, with `$?` holding what the body
//     returned. `AFTER 7` says both halves of that, so no control flow is
//     requested here and the status is simply returned.
//
// An error from the call is a fatal one — a cancelled context, not a status —
// and a builtin has an int to answer with and no way to pass it on. It is
// reported and answered 1 rather than dropped, which at least leaves a
// failing status where the shell would have stopped.
func autoloadRunResolved(r *interp.Runner, ctx context.Context, name string) int {
	// A copy, because the call replaces r.Params for the length of the body
	// and restores the slice header afterwards.
	args := append([]string(nil), r.Params...)
	ran, err := r.CallFunction(ctx, name, args...)
	if err != nil {
		r.Diagnosef("%s: %v\n", name, err)
		return 1
	}
	if !ran {
		// The resolution reported success, so the name is a function. This
		// is unreachable rather than a case, and it answers 1 rather than 0
		// so that a future change which makes it reachable is visible.
		return 1
	}
	return r.ExitStatus()
}

// autoloadBad is `-X` where there is no function for it to be about.
//
// zsh ends the script here and this shell reports and runs on, which is a
// difference the corpus records rather than one to hide: a dialect builtin
// has no way to say "and stop" that this engine offers, and inventing one
// for a spelling only the shell's own generated stub ever writes would be
// more surface than the corner earns.
func autoloadBad(r *interp.Runner) int {
	r.Diagnosef("bad autoload\n")
	return 1
}

// autoloadEnclosingFunction is the name of the function this builtin is
// running inside, which is what a bare `-X` acts on.
func autoloadEnclosingFunction(r *interp.Runner) (string, bool) {
	// CallStack is **innermost first**, with the script's own frame last, so
	// this walks forwards. It walked backwards once and found the outermost
	// function instead: `inner` called from `outer` resolved `outer`, which
	// is the caller being replaced by the callee's file. Nothing caught it
	// until a mutant asked, because the generated stub uses `+X NAME` and
	// never comes through here — only a hand-written `-X` does.
	for _, f := range r.CallStack() {
		if f.Name != "" {
			return f.Name, true
		}
	}
	return "", false
}

// autoloadResolve finds a name's file on `$fpath` and makes its contents the
// name's body.
//
// The search is `$fpath` in order and the first *readable* entry wins — not
// the first that exists, because a directory on `$fpath` that cannot be read
// is a search that goes on rather than a failure, which is what makes a
// stale entry harmless.
func autoloadResolve(r *interp.Runner, name string) int {
	body, ok := autoloadFile(r, name)
	if !ok {
		// Not the builtin speaking: measured, this message never carries
		// `autoload` in the location — `nosuchfn:10: nosuchfn: function
		// definition file not found` from inside the stub and
		// `<file>:14: myfunc2: …` from the top level. It is about the
		// function, and the location says so.
		r.DiagnoseAsTheShellf("%s: function definition file not found\n", name)
		return 1
	}
	if !r.DefineFunction(name, body) {
		// The file is not something this shell can read as a body. Its own
		// complaint rather than "not found", because the file *was* found
		// and saying otherwise would send somebody looking for it.
		r.DiagnoseAsTheShellf("%s: bad function definition\n", name)
		return 1
	}
	return 0
}

// autoloadFile reads the first file named `name` on `$fpath`.
func autoloadFile(r *interp.Runner, name string) (string, bool) {
	if strings.ContainsRune(name, filepath.Separator) {
		// A name with a directory in it is looked for where it says and
		// nowhere else, which is what `autoload /path/to/fn` means.
		text, err := os.ReadFile(name)
		return string(text), err == nil
	}
	dirs, _ := r.GetArray("fpath")
	for _, dir := range dirs {
		if dir == "" {
			dir = "."
		}
		text, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		return string(text), true
	}
	return "", false
}

// autoloadListing is the bare form: the names still waiting to be defined,
// each written as the function it currently is.
//
// Only the undefined ones — measured, a name that has been called is gone
// from the listing, because it is an ordinary function now and this builtin
// has nothing left to say about it.
func autoloadListing(r *interp.Runner) int {
	var pending []string
	for _, name := range r.FuncNames() {
		if autoloadPending(r, name) {
			pending = append(pending, name)
		}
	}
	sort.Strings(pending)
	for _, name := range pending {
		if text, ok := r.FunctionText(name); ok {
			_, _ = fmt.Fprintf(r.Out(), "%s\n", strings.TrimRight(text, "\n"))
		}
	}
	return 0
}

// autoloadRecord notes that a name was autoloaded. The stub is the record,
// so this only keeps the *set* of names for the listing to filter by — a
// function whose body happens to look like a stub was still not autoloaded.
func autoloadRecord(r *interp.Runner, name string) {
	names, _ := r.GetArray(autoloadStore)
	if !containsWord(names, name) {
		names = append(names, name)
	}
	r.SetArray(autoloadStore, names)
}

// autoloadPending reports whether a name is one this builtin marked and
// nothing has defined since.
func autoloadPending(r *interp.Runner, name string) bool {
	names, _ := r.GetArray(autoloadStore)
	if !containsWord(names, name) {
		return false
	}
	text, ok := r.FunctionText(name)
	return ok && strings.Contains(text, "builtin autoload +X "+name)
}

// autoloadStore is the set of names this builtin has marked, in the Runner's
// own table under a name no script can reach — the way `zstyle` and
// `zmodload` keep theirs, which is also what gives a subshell its own copy.
const autoloadStore = ".zsh.autoload"
