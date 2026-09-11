// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
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
// that has not been called yet, and what stands there is **the letters the
// declaration was given, recorded**. Measured 2026-09-10 on zsh 5.9.2 with a
// file for `f1` on `$fpath` and the name never called:
//
//	autoload -Uz f1     f1 () {\n\t# undefined\n\tbuiltin autoload -XUz\n}
//	autoload -zU f1     the same — the order is canonical, not the one written
//	autoload -UU f1     builtin autoload -XU — and the letters are a set
//	autoload    f1      builtin autoload -X
//	autoload -r  f1     builtin autoload -X /the/dir/it/resolved/in
//	autoload -RUz f1    builtin autoload -XUz /the/dir/it/resolved/in
//
// so `-X` there is not decoration: it is the same builtin, called with no
// name, acting on the function it is running inside — which is exactly what
// autoloadResolveNow already does for a hand-written `-X`, and the directory
// operand is the `-r` half of that. The stub is therefore the real thing
// rather than a text arranged to look like it: it runs, it is what a script
// reads back, and the two cannot drift apart because there is only one of
// them.
//
// It was `builtin autoload +X NAME && NAME "$@"` — a re-exec written out
// longhand — which behaved correctly and read as something no zsh ever wrote.
// That is not only cosmetic. A completion dump names every autoloaded
// completion function, and the thing that reads a stub back is a *script*
// (#1697).
//
// Two more things the same measurement settles, and both are why the text
// here is not simply printed from the tree:
//
//   - `# undefined` is not a comment in the body. A function written by hand
//     with that line first lists without it, because a listing is printed
//     from the tree and the tree holds no comments. It is the shell saying
//     what the function is, which is why it arrives through
//     interp.Runner.SetUndefinedFunctions rather than through the body.
//   - `$functions[f1]` is a *third* text: `builtin autoload -XU`, with no
//     indentation, no `# undefined`, and neither the emulation letter nor the
//     directory. See zshFunctionsView.

// autoloadStubPrefix opens every stub: the builtin, and the letter that means
// "act on the function you are running inside".
const autoloadStubPrefix = "builtin autoload -X"

// autoloadUnimplemented are the letters zsh has and this shell does not. The
// ones it does have are the cases of autoloadOptions' switch.
//
// zsh's set, measured a letter at a time against all fifty-two: it has
// `d k m r R t T U w W X z` and refuses every other one as `bad option`. So
// the split here is between the five that say something this engine can
// answer and the seven that do not:
//
//   - `-U` suppresses alias expansion while the file is read and `-z` picks
//     zsh-style parsing. Neither changes what this shell *does*: an
//     autoloaded file is parsed with this shell's own grammar, which is
//     zsh's, and aliases are not expanded in it either way. Accepted rather
//     than refused because every real script writes `-Uz` and refusing it
//     would fail the line for a distinction with no effect here — and
//     **recorded**, because the stub is the letters and a script reads them
//     back off it.
//   - `-X` and `+X` resolve now rather than at the call.
//   - `-r` and `-R` fix the *path* now rather than at the call. See
//     autoloadFixPath for what the two of them are measured to do and how
//     they differ from each other.
//   - `-t`/`-T` (trace this function), `-d`/`-k`/`-m` (ksh-style and pattern
//     forms), `-w`/`-W` (read a compiled `.zwc` file) are named as missing.
//     Each is a thing this shell does not do, and a builtin that took the
//     letter and dropped it would read as one that did.
const autoloadUnimplemented = "dkmtTwW"

func registerAutoload(r *interp.Runner) {
	r.Register("autoload", autoloadBuiltin)
	// A listing is not the body printed back for a name still waiting to be
	// defined — see the note above autoloadStubPrefix, and
	// interp.Runner.SetUndefinedFunctions for the seam.
	r.SetUndefinedFunctions(func(name string) (string, bool) {
		line, ok := autoloadStubLine(r, name)
		if !ok {
			return "", false
		}
		indent := FunctionLayout().Indent
		return "{\n" + indent + "# undefined\n" + indent + line + "\n}", true
	})
}

// autoloadOpts is what the letters asked for.
type autoloadOpts struct {
	// keepAliases is `-U` and zshParse is `-z`. Neither changes what this
	// shell does — an autoloaded file is parsed with this grammar and no
	// alias is expanded in it either way — but both are *recorded*, because
	// the stub a declaration writes is the letters it was given and a script
	// reads them back. See autoloadStubPrefix.
	keepAliases bool
	zshParse    bool
	// now is `-X` or `+X`: resolve at once instead of at the call.
	now bool
	// plus records which sign `X` was written with, because they name
	// different things: `+X NAME` resolves the name given, and `-X` with no
	// name resolves the function it is running inside.
	plus bool
	// fixPath is `-r` or `-R`: search `$fpath` at the declaration and record
	// the path the name resolved to, rather than searching at the call.
	fixPath bool
	// strict is the `R` of the pair, which reports a name it cannot find at
	// once where `-r` says nothing and leaves the search for the call.
	strict bool
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
	status := 0
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
		// The path is fixed *before* the stub is written, because a fixed
		// path is part of the stub: `autoload -r f` lists as
		// `builtin autoload -X <dir>`, so the search has to have happened
		// for there to be a directory to write. A plain declaration says
		// "search at the call" instead, and a name declared with `-r` once
		// and plainly afterwards must not keep answering from the first
		// declaration's file.
		dir := ""
		if opts.fixPath {
			code, path := autoloadFixPath(r, name, opts.strict)
			if code != 0 {
				// Carried rather than returned, because `autoload -R a b`
				// has two names to answer for and stopping at the first
				// would leave the second undeclared. zsh ends the script
				// here instead; that is the same difference `autoloadBad`
				// records, and for the same reason.
				status = code
			}
			dir = path
		} else {
			autoloadForgetPath(r, name)
		}
		if !r.DefineFunction(name, autoloadStub(opts, dir)) {
			// The stub is this file's own text, so a name it cannot be
			// written around is a name that is not a name.
			r.Diagnosef("not valid in this context: %s\n", name)
			return 1
		}
		autoloadRecord(r, name)
	}
	return status
}

// autoloadDefined reports whether a name is already a function this builtin
// must leave alone — one with a body of its own rather than one of the stubs
// this builtin wrote.
//
// The exception is the whole of why this is not `FunctionText(name)`: a name
// waiting to be defined *is* a function already — the stub is the record —
// so a guard that did not know the difference would refuse every second
// declaration of the same name, and turn the stub a plugin manager rewrites
// on every reload into a function nothing can load.
func autoloadDefined(r *interp.Runner, name string) bool {
	if _, ok := r.FunctionText(name); !ok {
		return false
	}
	return !autoloadPending(r, name)
}

// autoloadStub is the body a name is given until it is called: the letters the
// declaration was given, in zsh's canonical order, and the directory a `-r`
// or `-R` settled on.
//
// The order is the shell's and not the one written — measured, `-zU` and `-Uz`
// both list as `-XUz` — and repeated letters collapse, because what is
// recorded is a set. Only the two letters this builtin implements can reach
// here; `-t` and the rest are refused as unimplemented before a stub is
// written, which is why there is no case for them.
func autoloadStub(opts autoloadOpts, dir string) string {
	body := autoloadStubPrefix
	if opts.keepAliases {
		body += "U"
	}
	if opts.zshParse {
		body += "z"
	}
	if dir != "" {
		body += " " + autoloadStubWord(dir)
	}
	return body
}

// autoloadStubWord is a directory as the stub writes it.
//
// zsh writes the recorded string with nothing done to it, and this does the
// same for every path that survives being read back — which is the whole of
// what a path on `$fpath` normally is. A path holding something the grammar
// would take apart is single-quoted instead, and that is a deliberate
// divergence in a corner the measurement does not reach: the stub is a body
// this shell runs, so a `$fpath` entry with a space in it must not become two
// operands and `autoload: -X: too many arguments` at the first call.
func autoloadStubWord(dir string) string {
	if dir != "" && strings.IndexFunc(dir, autoloadStubNeedsQuoting) < 0 {
		return dir
	}
	return "'" + strings.ReplaceAll(dir, "'", `'\''`) + "'"
}

// autoloadStubNeedsQuoting reports whether one character of a path would not
// survive being read back as a bare word.
func autoloadStubNeedsQuoting(c rune) bool {
	switch {
	case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		return false
	case c == '/', c == '.', c == '_', c == '-', c == '+', c == ':', c == '@',
		c == '%', c == ',':
		return false
	}
	return true
}

// autoloadStubLine is the stub a name currently holds, as one line and with
// the listing's indentation taken off — the text `# undefined` stands above
// and the text `$functions` reduces. A name that is not waiting to be defined
// has no stub.
func autoloadStubLine(r *interp.Runner, name string) (string, bool) {
	if !autoloadPending(r, name) {
		return "", false
	}
	body, ok := r.FunctionBodyText(name)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(body), true
}

// autoloadBodyValue is what `$functions` holds for a name still waiting to be
// defined, which is a third text again and measured rather than derived:
// `autoload -Uz f1` lists as `builtin autoload -XUz` and reads back through
// the association as `builtin autoload -XU`, with no indentation, no
// `# undefined`, and no directory however the declaration was written.
//
// So the emulation letter and the operand come off, and `-U` stays.
func autoloadBodyValue(r *interp.Runner, name string) (string, bool) {
	line, ok := autoloadStubLine(r, name)
	if !ok {
		return "", false
	}
	fields := strings.Fields(line)
	if len(fields) < 3 {
		// `builtin autoload -X` with nothing after it, which is a plain
		// declaration and already the value.
		return line, true
	}
	return fields[0] + " " + fields[1] + " " + strings.TrimSuffix(fields[2], "z"), true
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
			case letter == 'r' || letter == 'R':
				// Both fix the path; only `R` complains about a name it
				// cannot fix one for, so a word writing both — which no
				// real script does, but the letters permit — is the
				// stricter of the two.
				opts.fixPath = true
				opts.strict = opts.strict || letter == 'R'
			case letter == 'U':
				// Accepted and recorded rather than accepted and dropped:
				// this shell answers `-U` by parsing the way it already
				// parses, and the stub still has to say it was asked for.
				opts.keepAliases = true
			case letter == 'z':
				opts.zshParse = true
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
		// `-X` names no *function*: it acts on the one it is running inside.
		// What it may take is a single **directory** to look in, which is
		// what a `-r` declaration's stub carries — measured on zsh 5.9.2
		// with `other/gf` and `fns/gf` both readable and `fpath=(fns)`:
		//
		//	gf() { builtin autoload -XU other; print "AFTER $?" }
		//	gf a b        OTHER body a b / AFTER 0
		//
		// so the operand wins over `$fpath` outright, and a directory with
		// no such file there is `gf: function definition file not found`
		// rather than a search that carries on. Two operands are
		// `autoload: -X: too many arguments`, and any operand at all at the
		// top level is `bad autoload` — there is no function for it to be
		// about, which is the failure that comes first.
		if len(names) > 1 {
			r.Diagnosef("-X: too many arguments\n")
			return 1
		}
		name, ok := autoloadEnclosingFunction(r)
		if !ok {
			return autoloadBad(r)
		}
		if code := autoloadResolveIn(r, name, names); code != 0 {
			return code
		}
		return autoloadRunResolved(r, ctx, name)
	}
	if len(names) == 0 {
		// `+X` with nothing to resolve lists, the same as a bare
		// declaration — measured, `autoload +X` alone writes every stub out
		// exactly as `autoload` does.
		return autoloadListing(r)
	}
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
// An error from the call is a fatal one — a canceled context, not a status —
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
	return autoloadResolveIn(r, name, nil)
}

// autoloadResolveIn is that with the directory a `-X` was given, where it was
// given one: the operand replaces the search rather than joining it, so a
// name that is not in that one directory is not found however much of
// `$fpath` would have had it.
func autoloadResolveIn(r *interp.Runner, name string, dirs []string) int {
	if len(dirs) == 1 {
		body, err := r.ReadFileGated(filepath.Join(dirs[0], name))
		if err != nil {
			r.DiagnoseAsTheShellf("%s: function definition file not found\n", name)
			return 1
		}
		text := string(body)
		if inner, lone := autoloadLoneDefinition(name, text); lone {
			text = inner
		}
		if !r.DefineFunction(name, text) {
			r.DiagnoseAsTheShellf("%s: bad function definition\n", name)
			return 1
		}
		return 0
	}
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
	if inner, lone := autoloadLoneDefinition(name, body); lone {
		body = inner
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

// autoloadLoneDefinition reports a function file that holds **nothing but a
// definition of the function it is named after**, and gives back the text of
// that definition's own body.
//
// A function file may hold the body, or it may hold a `name() { … }` for the
// name it is called. Both are written in the wild and this shell ran only the
// first: the file's text became the body, so the first call ran a
// *definition*, redefined the function and returned 0 having done nothing.
// The second call then worked, which is what made it silent — nothing is
// missing afterwards, no diagnostic is written, and a completion or plugin
// function that returns 0 having done nothing is indistinguishable from one
// that declined (#1704).
//
// The shape is decided at **load** time and not at call time, which is
// measured rather than chosen: `autoload +X kfn; functions kfn` writes the
// *inner* body in zsh 5.9.2, so the file was read as a definition before
// anything called it. Defining from the inner body here therefore needs no
// second call and no re-entry — the first call runs the real body, with the
// arguments it was made with.
//
// It is the *whole file* that has to be the definition, which is the
// measurement that decides between the two readings the issue left open. With
// two files in one directory, 2026-09-10:
//
//	kfn   kfn() { print A }                     first call  A
//	tw2   helper() { … }; tw2() { print B }     first call  nothing, second B
//
// So it is not "the load defined this name" — `tw2` does, and is not called —
// and not "the last command was a definition" either. A definition wrapped in
// a group is not one command that is a definition, and is not called; a
// comment above the definition is, and is. A trailing `;` makes no
// difference. Every one of those is measured.
//
// The text handed back is what stands between the definition's braces. A
// listing is printed from the tree rather than from the file, so the
// whitespace does not survive either way and `functions kfn` writes the same
// body in both shells.
//
// Both routes into a function file ask: the search along `$fpath` and the one
// directory an `-X` was handed. A file read one way and not the other would
// be the same bug reachable by the other road.
func autoloadLoneDefinition(name, body string) (inner string, lone bool) {
	f, err := syntax.Parse(body, Dialect())
	if err != nil || len(f.Stmts) != 1 {
		return "", false
	}
	pipe, ok := f.Stmts[0].Expr.(*syntax.Pipeline)
	if !ok || len(pipe.Cmds) != 1 {
		return "", false
	}
	decl, ok := pipe.Cmds[0].(*syntax.FuncDecl)
	if !ok || decl.Name != name {
		return "", false
	}
	group, ok := decl.Body.(*syntax.Group)
	if !ok || len(group.List) == 0 {
		// A body that is not a brace group, or a definition with nothing in
		// it: there is no inner text to hand back, and the file is left to be
		// read the way it always was.
		return "", false
	}
	// Between the braces, taken from the *group* and not from the statements
	// inside it: a statement written without a separator before the closing
	// brace has no end position, and slicing to it produced an empty range
	// that quietly left this whole path unreached — which is the spelling
	// `kfn() { print x }` uses and the one a real function file is written
	// in. The group's own end is one past the brace.
	start, end := group.Start.Offset+1, group.Stop.Offset-1
	if start >= end || end > len(body) || body[end] != '}' {
		return "", false
	}
	return body[start:end], true
}

// autoloadFile reads the first file named `name` on `$fpath`.
func autoloadFile(r *interp.Runner, name string) (string, bool) {
	if path, ok := autoloadFixedPath(r, name); ok {
		// `-r` or `-R` already chose, and the choice holds: no fresh search
		// behind it, so a fixed file that has gone is not found rather than
		// found somewhere else. Measured — see autoloadFixPath.
		text, err := r.ReadFileGated(path)
		return string(text), err == nil
	}
	if strings.ContainsRune(name, filepath.Separator) {
		// A name with a directory in it is looked for where it says and
		// nowhere else, which is what `autoload /path/to/fn` means.
		text, err := r.ReadFileGated(name)
		return string(text), err == nil
	}
	dirs, _ := r.GetArray("fpath")
	for _, dir := range dirs {
		if dir == "" {
			dir = "."
		}
		text, err := r.ReadFileGated(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		return string(text), true
	}
	return "", false
}

// autoloadFixPath is the `-r` and `-R` half of a declaration: resolve the
// name on `$fpath` *now* and record the file it resolved to, so that the
// call reads that file rather than searching again.
//
// Measured on zsh 5.9 with `A/f` holding `print A` and `B/f` holding
// `print B`. The declaration is what searches:
//
//	fpath=(A); autoload -r f; fpath=(B); f    # A
//	fpath=(A); autoload    f; fpath=(B); f    # B
//
// so this is not a letter to accept and ignore — a plugin manager that
// rewrites `$fpath` between declaring a name and calling it gets a different
// file, and the one it asked for is the earlier one.
//
// Three more probes say what exactly is kept:
//
//   - The *path*, not the contents. Editing `A/f` in place after the
//     declaration makes the call print the edit, so the file is still read
//     at the call and only the choice of file is settled here.
//   - And kept for good. `fpath=(C); autoload -r g; rm C/g; fpath=(B)` with
//     a perfectly good `B/g` is `g: function definition file not found` —
//     a fixed path that has gone does not fall back to a fresh search.
//   - A name it cannot resolve is *not* fixed, and `-r` is silent about it:
//     `fpath=(); autoload -r f; fpath=(A); f` prints `A`, so the failure
//     leaves an ordinary deferred autoload behind. `-R` is the same
//     declaration with the failure reported —
//     `nosuchfn: function definition file not found`, status 1.
func autoloadFixPath(r *interp.Runner, name string, strict bool) (int, string) {
	path, ok := autoloadSearch(r, name)
	if !ok {
		if strict {
			// The function's own complaint, not the builtin's, which is what
			// autoloadResolve says it for the same words at the call.
			r.DiagnoseAsTheShellf("%s: function definition file not found\n", name)
			return 1, ""
		}
		return 0, ""
	}
	paths, _ := r.GetAssoc(autoloadPathStore)
	if paths == nil {
		paths = map[string]string{}
	}
	paths[name] = path
	r.SetAssoc(autoloadPathStore, paths)
	// The *directory*, which is what the stub carries and what a fixed path
	// is really about — measured, `autoload -r f` lists as
	// `builtin autoload -X <dir>` and never as the file.
	return 0, filepath.Dir(path)
}

// autoloadForgetPath drops a name's fixed path, so the next call searches.
func autoloadForgetPath(r *interp.Runner, name string) {
	paths, ok := r.GetAssoc(autoloadPathStore)
	if !ok {
		return
	}
	if _, ok := paths[name]; !ok {
		return
	}
	delete(paths, name)
	r.SetAssoc(autoloadPathStore, paths)
}

// autoloadFixedPath is the path a `-r` or `-R` declaration settled on.
func autoloadFixedPath(r *interp.Runner, name string) (string, bool) {
	paths, ok := r.GetAssoc(autoloadPathStore)
	if !ok {
		return "", false
	}
	path, ok := paths[name]
	return path, ok
}

// autoloadSearch is autoloadFile's search without the read: the path the
// name resolves to on `$fpath`, by the same rule that the first *readable*
// entry wins.
func autoloadSearch(r *interp.Runner, name string) (string, bool) {
	if strings.ContainsRune(name, filepath.Separator) {
		// A name that says where it is resolves to itself, and is still a
		// resolution that can fail: `-R` on an unreadable path is the same
		// complaint as `-R` on a name that is nowhere on `$fpath`.
		if _, err := r.ReadFileGated(name); err != nil {
			return "", false
		}
		return name, true
	}
	dirs, _ := r.GetArray("fpath")
	for _, dir := range dirs {
		if dir == "" {
			dir = "."
		}
		path := filepath.Join(dir, name)
		// Read rather than stat, so that "readable" here means what it means
		// in autoloadFile — an entry this shell could not open is one the
		// search goes past rather than one it stops on.
		if _, err := r.ReadFileGated(path); err != nil {
			continue
		}
		return path, true
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
	r.SetAssocElement(autoloadStore, name, "")
}

// autoloadPending reports whether a name is one this builtin marked and
// nothing has defined since.
//
// Both halves are needed and neither is enough. The set alone would still say
// yes after the file was read, since a name is not taken out of it; the body
// alone would say yes for a function somebody wrote by hand around a `-X`,
// which is a spelling real plugin loaders write (see autoloadRunResolved).
func autoloadPending(r *interp.Runner, name string) bool {
	if _, marked := r.AssocElement(autoloadStore, name); !marked {
		return false
	}
	body, ok := r.FunctionBodyText(name)
	if !ok {
		return false
	}
	body = strings.TrimSpace(body)
	return !strings.ContainsRune(body, '\n') && strings.HasPrefix(body, autoloadStubPrefix)
}

// autoloadStore is the set of names this builtin has marked, in the Runner's
// own table under a name no script can reach — the way `zstyle` and
// `zmodload` keep theirs, which is also what gives a subshell its own copy.
//
// Keyed, and a set is what it is: the values are empty and the key is the
// whole of the record. It was an indexed array, which made both halves of
// the set linear — a scan to ask whether a name was in it, and a rebuild to
// put one there — so a `compinit` marking fifteen hundred completions was
// quadratic twice over. Measured on a real startup that was 0.34s, a third
// of the time spent reading the rc file, and the sweep it forced through
// every subscript was most of what this shell did in interp/array.go.
const autoloadStore = ".zsh.autoload"

// autoloadPathStore maps a name declared with `-r` or `-R` to the file that
// declaration resolved it to. Kept beside autoloadStore and for the same
// reasons, a subshell's own copy among them.
const autoloadPathStore = ".zsh.autoload.path"
