// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
)

// `type` says what a name would run, in a sentence rather than as a path.
//
// The same question `command -v` answers and the same lookup, which is why
// they share one: a function first, then a builtin, then a keyword, then
// PATH. What differs is that this one is for a person to read, so every part
// of it is worded — and the four shells word all four parts differently, down
// to whether a missing name gets the shell's name in front of it.
func init() { builtins["type"] = biType }

func biType(r *Runner, _ context.Context, args []string) int {
	// `--` ends the options in three of the four. dash has no options at
	// all and reads it as a name, which is why this is asked rather than
	// assumed: `type -- ls` prints `--: not found` there and then goes on to
	// answer about `ls`.
	names, m, code := r.typeOperands(args)
	if code != 0 {
		return code
	}
	status := 0
	for _, name := range names {
		if bad := r.typeOneMode(name, m); bad != 0 {
			status = bad
		}
	}
	return status
}

// typeMode is what the option letters asked for.
type typeMode struct {
	// kind is `-t`: the bare kind word instead of the sentence.
	kind bool
	// all is `-a`: every resolution the name has, PATH hits included.
	all bool
	// path is `-p`: the path alone — with two axes inside it, see
	// TypePSearchesPathPastTheShell and TypePathAnswerIsASentence.
	path bool
	// pathSearch is `-P`: the PATH search whatever the shell would say,
	// which one dialect alone spells.
	pathSearch bool
	// noFuncs is `-f`: functions left out of the search — or, in the
	// dialect where TypeFSaysTheFunctionBack answers yes, printed whole.
	noFuncs bool
}

// typeOperands separates the options from the names.
func (r *Runner) typeOperands(args []string) (names []string, m typeMode, code int) {
	// Asked only where there is a `--` to decide about. `type ls` is the
	// same in all four, and refusing it over a question nothing turned on
	// would be refusing to answer.
	if len(args) == 0 || len(args[0]) < 2 || args[0][0] != '-' {
		return args, m, 0
	}
	if !r.ask(r.sem().TypeEndsOptionsWithDashDash, "`type --` ending the options") {
		if r.unspecified {
			return nil, m, 2
		}
		// No options anywhere, so every operand is a name — `--` included.
		return args, m, 0
	}
	if args[0] == "--" {
		return args[1:], m, 0
	}
	// The letters are the dialect's — TypeOptions, plus `-t` where the axis
	// that predates the optstring answers for it. The `t` question is asked
	// only where a `t` rides in the option words: any other letter is
	// refused identically whichever way the answer goes — the ones the
	// dialect really has are named as missing and the rest as unknown — so
	// the question would decide nothing there.
	known := r.sem().TypeOptions
	if !strings.ContainsRune(known, 't') && typeOptionWordsCarryT(args) {
		if r.ask(r.sem().TypeNamesTheKindWithDashT, "`type -t` naming the bare kind") {
			known += "t"
		}
		if r.unspecified {
			return nil, m, 2
		}
	}
	rest, opts, code := r.builtinOptions("type", args, known)
	if code != 0 {
		return nil, m, code
	}
	m = typeMode{
		kind:       strings.ContainsRune(opts, 't'),
		all:        strings.ContainsRune(opts, 'a'),
		path:       strings.ContainsRune(opts, 'p'),
		pathSearch: strings.ContainsRune(opts, 'P'),
		noFuncs:    strings.ContainsRune(opts, 'f'),
	}
	return rest, m, 0
}

// typeOptionWordsCarryT says whether a `t` rides in the leading option words —
// the words builtinOptions would read before the first operand.
func typeOptionWordsCarryT(args []string) bool {
	for _, a := range args {
		if len(a) < 2 || a[0] != '-' || a == "--" {
			return false
		}
		if strings.ContainsRune(a[1:], 't') {
			return true
		}
	}
	return false
}

// typeOneMode dispatches one name to the shape its letters asked for.
func (r *Runner) typeOneMode(name string, m typeMode) int {
	if m.all {
		return r.typeAll(name, m)
	}
	if m.pathSearch {
		return r.typeBarePath(name, m.kind)
	}
	if m.path {
		return r.typePath(name, m)
	}
	if m.noFuncs {
		if fn, ok := r.funcs[name]; ok {
			// `-f` splits: two shells use it to leave functions out of the
			// search, and one turns it around and *prints* the function —
			// the definition alone, no sentence in front of it.
			says := r.ask(r.sem().TypeFSaysTheFunctionBack, "`type -f` printing the function whole")
			if r.unspecified {
				return 2
			}
			if says {
				r.printf("%s\n", r.listedFunction(name, fn))
				return 0
			}
		}
		return r.describeName(name, m.kind, true, r.typeNotFoundWording(name))
	}
	return r.typeOne(name, m.kind)
}

// typeOne accounts for one name — as a sentence, or as `-t`'s bare kind —
// and reports a status if it could not.
//
// The kinds are one dialect's words and every dialect's words at once: only
// one shell in the panel has `-t` at all, so there is no second wording to
// hold a field for. The measured shell also answers `alias`, which is out of
// reach here for the same reason plain `type` never names one: whether
// aliases expand is the parser's fact — see syntax.Dialect.ExpandAliases —
// and the runner holds only the table.
func (r *Runner) typeOne(name string, kind bool) int {
	return r.describeName(name, kind, false, r.typeNotFoundWording(name))
}

func (r *Runner) typeNotFoundWording(name string) string {
	return Wording(r.diag().TypeNotFound, "type: %[1]s: not found", name)
}

// typePath is `-p`: the path alone. Which names it answers for, and in what
// shape, are the two axes measured inside the letter.
func (r *Runner) typePath(name string, m typeMode) int {
	past := r.ask(r.sem().TypePSearchesPathPastTheShell, "`type -p` searching PATH past the shell's own answer")
	if r.unspecified {
		return 2
	}
	if !past {
		// This engine's `-p` speaks only where the plain answer would have
		// been a file: a function, builtin or keyword is silence and 0.
		if _, ok := r.funcs[name]; ok && !m.noFuncs {
			return 0
		}
		if _, ok := r.lookupBuiltin(name); ok {
			return 0
		}
		if reservedWord(name) {
			return 0
		}
	}
	sentence := r.ask(r.sem().TypePathAnswerIsASentence, "`type -p` answering with a sentence")
	if r.unspecified {
		return 2
	}
	path, err := r.lookPath(name)
	if err != nil {
		if sentence {
			return r.typeNotFound(m.kind, r.typeNotFoundWording(name))
		}
		// A miss is silence and the failing status in the bare-path shells.
		return orDefault(r.diag().TypeNotFoundStatus, 1)
	}
	switch {
	case m.kind:
		r.printf("file\n")
	case sentence:
		r.printf("%s\n", Wording(r.diag().TypeExternal, "%[1]s is %[2]s", name, path))
	default:
		r.printf("%s\n", path)
	}
	return 0
}

// typeBarePath is `-P`: the PATH search whatever the shell would say, the
// bare path or silence — one dialect's letter, so there is no second shape
// to ask about.
func (r *Runner) typeBarePath(name string, kind bool) int {
	path, err := r.lookPath(name)
	if err != nil {
		return orDefault(r.diag().TypeNotFoundStatus, 1)
	}
	if kind {
		r.printf("file\n")
		return 0
	}
	r.printf("%s\n", path)
	return 0
}

// typeFunctionLine is the sentence `type` and `command -V` write for a name
// that is a function: the shell's ordinary wording, or its wording for one
// whose body has not been read yet.
//
// One place, because both callers ask the identical question and a second
// copy is how the two would come to answer it differently — which is the
// mistake this tree has made often enough to have a rule about. A shell with
// no such notion has no second wording either, and falls straight through.
// FunctionSentence is that line for a dialect's own name-reporting builtin,
// which asks the identical question: `type` in one shell *is* `whence -v`,
// and the two writing the sentence separately is how they came to disagree
// about the origin — one of them naming a file and the other a constant.
func (r *Runner) FunctionSentence(name string) string {
	return r.typeFunctionLine(r.diag(), name)
}

func (r *Runner) typeFunctionLine(dg Diagnostics, name string) string {
	if _, undefined := r.undefinedFunction(name); undefined && dg.TypeUndefinedFunction != "" {
		return Wording(dg.TypeUndefinedFunction, "%[1]s is an undefined function", name)
	}
	if origin, ok := r.functionOrigin(name); ok && dg.TypeFunctionFrom != "" {
		return Wording(dg.TypeFunctionFrom, "%[1]s is a function from %[2]s", name, origin)
	}
	return Wording(dg.TypeFunction, "%[1]s is a function", name)
}

// functionOrigin is where a function was defined, for the dialect whose
// sentence names it.
//
// [Runner.funcFiles] already held the answer for a definition the parser read
// — it is what a frame reports and what a function's own trace is built from
// — and holds it for one a builtin defined from text as well, which is what
// makes an autoloaded function name its file rather than the shell.
//
// The fallback is the shell's own name, and it is the shell's name in a
// diagnostic rather than `$0`: measured 2026-09-12 through a symlink,
// `./xyzzy -c 'g(){ :; }; whence -v g'` still says `from zsh`.
//
// There is one route with no origin at all — a program on standard input,
// where the same definition is `g is a shell function` with no clause after
// it. Reported as false rather than as an empty string, so the caller writes
// the other sentence instead of a clause naming nothing.
func (r *Runner) functionOrigin(name string) (string, bool) {
	if file := r.funcFiles[name]; file != "" {
		return file, true
	}
	if r.Route == RouteStandardInput {
		return "", false
	}
	return r.name(), true
}

// typeAll is `-a`: every resolution the name has — the shell's own answer
// and then every PATH hit, in PATH order, duplicates and all.
func (r *Runner) typeAll(name string, m typeMode) int {
	dg := r.diag()
	found := false
	if fn, ok := r.funcs[name]; ok && !m.noFuncs {
		found = true
		if m.kind {
			r.printf("function\n")
		} else {
			shows := r.ask(r.sem().TypePrintsFunctionBody, "`type` printing a function's body")
			if r.unspecified {
				return 2
			}
			r.printf("%s\n", r.typeFunctionLine(dg, name))
			if shows {
				r.printf("%s\n", r.listedFunction(name, fn))
			}
		}
	}
	switch _, ok := r.lookupBuiltin(name); {
	case ok:
		found = true
		if m.kind {
			r.printf("builtin\n")
		} else {
			r.printf("%s\n", Wording(dg.TypeBuiltin, "%[1]s is a shell builtin", name))
		}
	case reservedWord(name):
		found = true
		if m.kind {
			r.printf("keyword\n")
		} else {
			r.printf("%s\n", Wording(dg.TypeKeyword, "%[1]s is a shell keyword", name))
		}
	}
	// The reserved-name guard the plain answer has, for the same reason;
	// past it, the listing's file lines are worded the same way by every
	// shell that has the letter — measured, and *not* this dialect's
	// TypeExternal: the engine that calls a plain answer a tracked alias
	// writes `ls is /bin/ls` here like the others.
	if !r.reservedBuiltin(name) {
		for _, path := range r.lookPathAll(name) {
			found = true
			if m.kind {
				r.printf("file\n")
			} else {
				r.printf("%s is %s\n", name, path)
			}
		}
	}
	if found {
		return 0
	}
	return r.typeNotFound(m.kind, r.typeNotFoundWording(name))
}

// typeNotFound is the tail every mode shares: the complaint — or `-t`'s
// silence — and the dialect's status.
func (r *Runner) typeNotFound(kind bool, notFound string) int {
	if kind {
		return orDefault(r.diag().TypeNotFoundStatus, 1)
	}
	r.reportNameNotFound(notFound)
	return orDefault(r.diag().TypeNotFoundStatus, 1)
}

// reportNameNotFound writes the line `type` and `command -V` share for a name
// that is nothing, on the stream the dialect reports it on.
//
// Two questions, asked in order and independent of each other: whether the
// line carries the shell's name and location — TypeNotFoundUnprefixed — and
// which stream it goes to — TypeNotFoundOnStdout. Half the panel calls this an
// answer and writes it where the answers go, and half calls it a complaint;
// getting the stream wrong hides the line from `type nope 2>/dev/null` or
// leaks it into `$(type -p nope)`, neither of which the wording would show.
func (r *Runner) reportNameNotFound(msg string) {
	dg := r.diag()
	line := r.diagLine("%s\n", msg)
	if dg.TypeNotFoundUnprefixed {
		// Nothing in front of it, so there is no prefix to work out.
		line = msg + "\n"
	}
	if dg.TypeNotFoundOnStdout {
		r.printf("%s", line)
		return
	}
	r.errf("%s", line)
}

// describeName is the sentence itself, shared with `command -V`, which asks
// `type`'s question with a complaint of its own for a name that is nothing —
// the one line the two spell differently, so it arrives already worded.
func (r *Runner) describeName(name string, kind, skipFuncs bool, notFound string) int {
	dg := r.diag()
	// The tables come first, as they do in every shell in the panel and as
	// the parser does when it reads a line: an alias beats a function of the
	// same name. What "the tables hold it" means is not the plain lookup —
	// the kind, the suffix keying and one dialect's expansion gate are all
	// in AliasForName.
	if display, value, akind, ok := r.AliasForName(name); ok {
		if kind {
			r.printf("alias\n")
			return 0
		}
		r.printf("%s\n", r.AliasSentence(display, value, akind))
		return 0
	}
	if r.unspecified {
		return 2
	}
	if fn, ok := r.funcs[name]; ok && !skipFuncs {
		if kind {
			r.printf("function\n")
			return 0
		}
		shows := r.ask(r.sem().TypePrintsFunctionBody, "`type` printing a function's body")
		if r.unspecified {
			return 2
		}
		r.printf("%s\n", r.typeFunctionLine(dg, name))
		if shows {
			// The function itself, laid out rather than quoted: the tree is
			// what this shell has, and the spelling it was written with is
			// gone by now. Which is why there is a printer — see
			// syntax.PrintWith.
			r.printf("%s\n", r.listedFunction(name, fn))
		}
		return 0
	}
	if _, ok := r.lookupBuiltin(name); ok {
		if kind {
			r.printf("builtin\n")
			return 0
		}
		r.printf("%s\n", Wording(dg.TypeBuiltin, "%[1]s is a shell builtin", name))
		return 0
	}
	if reservedWord(name) {
		if kind {
			r.printf("keyword\n")
			return 0
		}
		r.printf("%s\n", Wording(dg.TypeKeyword, "%[1]s is a shell keyword", name))
		return 0
	}
	// The same guard `command -v` has, and for the same reason: there is an
	// executable called /usr/bin/umask and this shell will not run it, so
	// saying where it is would be answering about the wrong thing.
	if !r.reservedBuiltin(name) {
		if path, err := r.lookPath(name); err == nil {
			if kind {
				// The kind and never the path, which is what keeps the word
				// comparable on any machine.
				r.printf("file\n")
				return 0
			}
			r.printf("%s\n", Wording(dg.TypeExternal, "%[1]s is %[2]s", name, path))
			return 0
		}
	}
	if kind {
		// Nothing at all for a name that is nothing — no line and no
		// diagnostic, measured. The status is the whole of the answer,
		// which is what makes `-t` scriptable in the first place.
		return orDefault(dg.TypeNotFoundStatus, 1)
	}
	// Two of the four write this one with nothing in front of it, where every
	// other message they print carries the shell's name — and the same two
	// write it to standard output. reportNameNotFound holds both facts.
	r.reportNameNotFound(notFound)
	return orDefault(dg.TypeNotFoundStatus, 1)
}
