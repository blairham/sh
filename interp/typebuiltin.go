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
// typeKind is how a letter that answers with a *word* wants it written.
//
// Two letters do, and they disagree about every word and about the shape of
// the line, which is why this is a vocabulary rather than a bool. Measured
// 2026-09-12 against the shells that have them:
//
//	                -t (one dialect)   -w (another)
//	an alias        alias              NAME: alias
//	a function      function           NAME: function
//	a builtin       builtin            NAME: builtin
//	a reserved word keyword            NAME: reserved
//	a file on PATH  file               NAME: command
//	nothing         (silence)          NAME: none
//
// Four of the six words differ and the sixth is the sharpest: `-t` says
// nothing at all for a name that is nothing — the status is the whole answer —
// where `-w` names it `none`. A syntax highlighter reads that line for every
// word on the line, so silence there would make every unknown command
// indistinguishable from a failure to ask (#2512).
type typeKind int

const (
	// typeKindNone is the plain sentence — `ls is /bin/ls`.
	typeKindNone typeKind = iota
	// typeKindBare is the kind alone, on a line of its own.
	typeKindBare
	// typeKindNamed is the name, a colon, and the kind.
	typeKindNamed
)

// sayKind writes the kind where a letter asked for one, and reports whether it
// wrote anything — false is the plain sentence, which the caller goes on to
// produce.
//
// One resolution serves all three shapes. The alternative is a second walk of
// the tables per letter, which is how two spellings of the same question come
// to disagree about which of an alias and a function wins.
func (r *Runner) sayKind(k typeKind, name, bare, named string) bool {
	switch k {
	case typeKindBare:
		r.printf("%s\n", bare)
	case typeKindNamed:
		r.printf("%s: %s\n", name, named)
	default:
		return false
	}
	return true
}

// BuiltinSentence is the line `type` and `command -V` write for a name that
// resolved to a builtin — and the line zsh's `whence -v` writes for it, which
// is why this is exported rather than private to this file.
//
// Two wordings, one axis and one membership. Four of the seven columns call a
// POSIX special builtin *special* and three call every builtin the same
// thing; see [Semantics.TypeDistinguishesSpecialBuiltins] for the panel. The
// axis is asked only for a name that *is* special, so `type echo` — the
// control that says the four columns are drawing a distinction rather than
// using a longer phrase — stays a question no dialect has to answer, and a
// runner with no dialect can still answer it.
//
// One function for all three spellings, because this is a sentence written in
// three places and a second copy of it is how `type` and `command -V` come to
// disagree about one builtin.
func (r *Runner) BuiltinSentence(name string) string {
	dg := r.diag()
	plain := Wording(dg.TypeBuiltin, "%[1]s is a shell builtin", name)
	if !r.IsSpecialBuiltinHere(name) {
		return plain
	}
	if !r.ask(r.sem().TypeDistinguishesSpecialBuiltins, "`type` calling a special builtin special") {
		return plain
	}
	return Wording(dg.TypeSpecialBuiltin, "%[1]s is a special shell builtin", name)
}

type typeMode struct {
	// kind is `-t`: the bare kind word instead of the sentence.
	kind bool
	// word is `-w`: the name, a colon and the kind. See typeKind, where the
	// two are measured against each other.
	word bool
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
	// kindWins says the `-t` was written *after* the last of `-p` and `-P`,
	// which is what decides between them. See typeLetterOrder.
	kindWins bool
}

// typeLetterOrder reports whether the kind letter was written after the last
// path letter, which is how the one column that has both settles them.
//
// There is no precedence between `-t` and `-p`: the later letter decides, in
// the same bundle or in separate words. Measured 2026-09-16 on bash 5.3.20 and
// bash 3.2.57 with `cd`, a name that is both a builtin and a file, since
// nothing with one resolution can tell the readings apart:
//
//	type -apt cd    builtin / file      the kind, of every resolution
//	type -atp cd    /usr/bin/cd         the path
//	type -pt cd     builtin             the kind, with no -a
//	type -tp cd     (nothing)           the path, which a builtin has none of
//	type -atP cd    /usr/bin/cd
//	type -aPt cd    file                the kind, of the file rows alone
//	type -Pt cd     file
//	type -tP cd     /usr/bin/cd
//
// The `-P` rows are what say this is a *shape* and not a switch between two
// modes: that letter forces the PATH search however the shape comes out, so a
// `-t` after it writes the kind of the rows the search left rather than of
// every resolution the name has. `-p` has no such half — `type -apt` is the
// whole listing — which is why the narrowing below asks for `pathSearch`
// separately.
//
// The letters arrive in the order they were written because builtinOptions
// appends them as it reads; nothing else in the tree needs that and this is
// the first thing that does.
func typeLetterOrder(opts string) bool {
	return strings.LastIndexByte(opts, 't') > strings.LastIndexAny(opts, "pP")
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
	// The letters are the dialect's, and all of them: `-t` is in TypeOptions
	// like every other letter since #2180. The axis that used to add it
	// duplicated the optstring — bash's already carried the `t`, so the
	// branch that added it could never run — which is the same split zsh's
	// `-w` had before it moved.
	rest, opts, code := r.builtinOptions("type", args, r.sem().TypeOptions)
	if code != 0 {
		return nil, m, code
	}
	m = typeMode{
		kind:       strings.ContainsRune(opts, 't'),
		word:       strings.ContainsRune(opts, 'w'),
		all:        strings.ContainsRune(opts, 'a'),
		path:       strings.ContainsRune(opts, 'p'),
		pathSearch: strings.ContainsRune(opts, 'P'),
		noFuncs:    strings.ContainsRune(opts, 'f'),
		kindWins:   typeLetterOrder(opts),
	}
	return rest, m, 0
}

// kindDecides reports whether a letter asking for the kind was written, and
// written after the last of the path letters — which is the whole of what
// parts `type -apt` from `type -atp`.
func (m typeMode) kindDecides() bool { return m.kind && m.kindWins }

// asked is the shape this mode's letters want a kind written in.
func (m typeMode) asked() typeKind {
	switch {
	case m.word:
		return typeKindNamed
	case m.kind:
		return typeKindBare
	}
	return typeKindNone
}

// typeOneMode dispatches one name to the shape its letters asked for.
func (r *Runner) typeOneMode(name string, m typeMode) int {
	if m.all {
		return r.typeAll(name, m)
	}
	if m.pathSearch {
		// The kind is written only where the letter asking for it came
		// last; `type -tP` is the path and `type -Pt` is `file`.
		return r.typeBarePath(name, m.kindDecides())
	}
	if m.path && !m.kindDecides() {
		return r.typePath(name, m)
	}
	if m.noFuncs {
		if fn, ok := r.reportedFunc(name); ok {
			// `-f` splits: two shells use it to leave functions out of the
			// search, and one turns it around and *prints* the function —
			// the definition alone, no sentence in front of it.
			says := r.ask(r.sem().TypeFSaysTheFunctionBack, "`type -f` printing the function whole")
			if r.unspecified {
				return 2
			}
			if says {
				r.printf("%s", r.listedFunctionLine(name, fn))
				return 0
			}
		}
		return r.describeName(name, m.asked(), true, r.typeNotFoundWording(name))
	}
	return r.typeOne(name, m.asked())
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
func (r *Runner) typeOne(name string, kind typeKind) int {
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
		// been a file: an alias, function, builtin or keyword is silence
		// and 0. The alias is first because the plain answer puts it first —
		// `type -p ls` is silent in bash 5.3.20 with `alias ls=...` set, and
		// names the file without it.
		if _, _, _, ok := r.AliasForName(name); ok {
			return 0
		}
		if r.unspecified {
			return 2
		}
		if _, ok := r.reportedFunc(name); ok && !m.noFuncs {
			return 0
		}
		if _, ok := r.lookupBuiltin(name); ok {
			return 0
		}
		if r.reservedWord(name) {
			return 0
		}
	}
	sentence := r.ask(r.sem().TypePathAnswerIsASentence, "`type -p` answering with a sentence")
	if r.unspecified {
		return 2
	}
	path, err := r.lookPathReporting(name)
	if err != nil {
		if sentence {
			return r.typeNotFound(m.kind, r.typeNotFoundWording(name))
		}
		// A miss is silence and the failing status in the bare-path shells.
		return orDefault(r.diag().TypeNotFoundStatus, 1)
	}
	if m.kind {
		// The kind and never the path, so the operand question is not asked:
		// `type -pt ./x` is `file` in every column.
		r.printf("file\n")
		return 0
	}
	path = r.reportedPath(name, path)
	if r.unspecified {
		return r.status
	}
	if sentence {
		r.printf("%s\n", r.TypeExternalSentence(name, path))
	} else {
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
	// Written the dialect's way, and only here: `-P` with `-t` answers
	// `file` above and never reaches a path, so the axis is not asked where
	// nothing would show it — see Runner.reportedPath.
	path = r.reportedPath(name, path)
	if r.unspecified {
		return r.status
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
// [Runner.funcOrigins] already held the answer for a definition the parser read
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
	if file := r.functionFile(name); file != "" {
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
	// `-p` and `-P` narrow the listing to its file rows, and a letter asking
	// for the *kind* takes it back — but only when it was written last, and
	// only `-p` gives it back whole. `type -apt cd` is `builtin` then `file`
	// and `type -aPt cd` is `file` alone, because the capital forces the
	// PATH search whatever shape the answer comes out in. See
	// typeLetterOrder, which carries the panel.
	if (m.path || m.pathSearch) && !m.word && (m.pathSearch || !m.kindDecides()) {
		return r.typeAllPaths(name, m)
	}
	dg := r.diag()
	found := false
	// The tables come first, as they do in the plain answer and in every
	// column that has the letter: bash 5.3.20, zsh 5.9.2 and ksh93u+ all
	// write the alias row ahead of the function, the builtin and the files,
	// and all three then carry on rather than stopping there. Measured
	// 2026-09-16. This listing had no alias row at all, so a name that was
	// only an alias was `not found` under `-a` while plain `type` named it.
	if display, value, akind, ok := r.AliasForName(name); ok {
		found = true
		if !r.sayKind(m.asked(), name, "alias", "alias") {
			r.printf("%s\n", r.AliasSentence(display, value, akind))
		}
	}
	if r.unspecified {
		return 2
	}
	// The reserved word first, and *every* resolution the name has after it.
	// `type -a export` in zsh 5.9.2 writes `export is a reserved word`, then
	// the function if one is defined, then `export is a shell builtin` — the
	// whole list, in that order, after the alias. A `switch` on the builtin
	// wrote one line or the other, which is right in the four dialects where
	// no name is ever both and short by a line in the one where seven are
	// (#3291). Measured with a function of that name defined, which is what
	// puts the three in an order rather than a pair.
	if r.reservedWord(name) {
		found = true
		if !r.sayKind(m.asked(), name, "keyword", NamedKindWord(NameReserved)) {
			r.printf("%s\n", Wording(dg.TypeKeyword, "%[1]s is a shell keyword", name))
		}
	}
	if fn, ok := r.reportedFunc(name); ok && !m.noFuncs {
		found = true
		if !r.sayKind(m.asked(), name, "function", NamedKindWord(NameFunction)) {
			shows := r.ask(r.sem().TypePrintsFunctionBody, "`type` printing a function's body")
			if r.unspecified {
				return 2
			}
			r.printf("%s\n", r.typeFunctionLine(dg, name))
			if shows {
				r.printf("%s", r.listedFunctionLine(name, fn))
			}
		}
	}
	if _, ok := r.lookupBuiltin(name); ok {
		found = true
		if !r.sayKind(m.asked(), name, "builtin", NamedKindWord(NameBuiltin)) {
			line := r.BuiltinSentence(name)
			if r.unspecified {
				return 2
			}
			r.printf("%s\n", line)
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
			if r.sayKind(m.asked(), name, "file", NamedKindWord(NameFile)) {
				continue
			}
			path = r.reportedPath(name, path)
			if r.unspecified {
				return r.status
			}
			r.printf("%s is %s\n", name, path)
		}
	}
	if found {
		return 0
	}
	return r.typeNotFound(m.kind, r.typeNotFoundWording(name))
}

// typeAllPaths is `-a` with `-p` or `-P`: the same walk, printed as paths.
//
// The letters compose rather than one canceling the other — `-a` decides how
// many rows there are and `-p` decides what a row says — which is why this
// shares the found/not-found tail with [Runner.typePath] instead of restating
// it. Measured 2026-09-16 over a function, a builtin-and-file, a name twice on
// PATH and a name that is nothing:
//
//	              bash 5.3.20        ksh93u+            zsh 5.9.2
//	-ap f         silence, 0         silence, 1         `f not found`, 1
//	-ap echo      /bin/echo, 0       /bin/echo, 0       `echo is /bin/echo`, 0
//	-ap dup       both paths, 0      both paths, 0      both sentences, 0
//	-ap nosuch    silence, 1         silence, 1         `nosuch not found`, 1
//
// The first row is TypePSearchesPathPastTheShell and nothing new: where the
// shell's own answer counts, a name it can answer for is *found* even though
// this letter prints none of it. What `-a` changes is that the shell's answer
// no longer stops the PATH walk — bash prints nothing for `type -p echo` and
// the file for `type -ap echo` — so the two are asked separately here.
func (r *Runner) typeAllPaths(name string, m typeMode) int {
	past := r.ask(r.sem().TypePSearchesPathPastTheShell, "`type -p` searching PATH past the shell's own answer")
	if r.unspecified {
		return 2
	}
	found := false
	if !past && !m.pathSearch {
		// Counted, never printed: `-P` is the letter that ignores the
		// shell's own answer in every column, so it asks nothing here.
		if _, _, _, ok := r.AliasForName(name); ok {
			found = true
		}
		if r.unspecified {
			return 2
		}
		if _, ok := r.reportedFunc(name); ok && !m.noFuncs {
			found = true
		}
		if _, ok := r.lookupBuiltin(name); ok {
			found = true
		}
		if r.reservedWord(name) {
			found = true
		}
	}
	sentence := r.ask(r.sem().TypePathAnswerIsASentence, "`type -p` answering with a sentence")
	if r.unspecified {
		return 2
	}
	if !r.reservedBuiltin(name) {
		for _, path := range r.lookPathAll(name) {
			found = true
			path = r.reportedPath(name, path)
			if r.unspecified {
				return r.status
			}
			switch {
			case m.kindDecides():
				// A `-t` written after the path letters asks for the kind
				// of the rows the search left, not for their paths.
				r.printf("file\n")
			case sentence:
				r.printf("%s\n", r.TypeExternalSentence(name, path))
			default:
				r.printf("%s\n", path)
			}
		}
	}
	if found {
		return 0
	}
	if sentence {
		return r.typeNotFound(m.kind, r.typeNotFoundWording(name))
	}
	// A miss is silence and the failing status in the bare-path shells, the
	// same tail the plain letter has.
	return orDefault(r.diag().TypeNotFoundStatus, 1)
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
func (r *Runner) describeName(name string, kind typeKind, skipFuncs bool, notFound string) int {
	dg := r.diag()
	// The tables come first, as they do in every shell in the panel and as
	// the parser does when it reads a line: an alias beats a function of the
	// same name. What "the tables hold it" means is not the plain lookup —
	// the kind, the suffix keying and one dialect's expansion gate are all
	// in AliasForName.
	if display, value, akind, ok := r.AliasForName(name); ok {
		if r.sayKind(kind, name, "alias", "alias") {
			return 0
		}
		r.printf("%s\n", r.AliasSentence(display, value, akind))
		return 0
	}
	if r.unspecified {
		return 2
	}
	if fn, ok := r.reportedFunc(name); ok && !skipFuncs {
		if r.sayKind(kind, name, "function", NamedKindWord(NameFunction)) {
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
			r.printf("%s", r.listedFunctionLine(name, fn))
		}
		return 0
	}
	// The reserved word first, which is the resolution order and not a
	// preference: a plain answer names what the shell would reach, and a
	// word this grammar owns is reached before any table is consulted. It
	// only ever *shows* in the one dialect where a name is both — zsh's
	// seven declaration commands — because nowhere else does a builtin share
	// a name with a word of the grammar (#3291).
	if r.reservedWord(name) {
		if r.sayKind(kind, name, "keyword", NamedKindWord(NameReserved)) {
			return 0
		}
		r.printf("%s\n", Wording(dg.TypeKeyword, "%[1]s is a shell keyword", name))
		return 0
	}
	if _, ok := r.lookupBuiltin(name); ok {
		if r.sayKind(kind, name, "builtin", NamedKindWord(NameBuiltin)) {
			return 0
		}
		line := r.BuiltinSentence(name)
		if r.unspecified {
			return 2
		}
		r.printf("%s\n", line)
		return 0
	}
	// The same guard `command -v` has, and for the same reason: there is an
	// executable called /usr/bin/umask and this shell will not run it, so
	// saying where it is would be answering about the wrong thing.
	if !r.reservedBuiltin(name) {
		if path, err := r.lookPathReporting(name); err == nil {
			if r.sayKind(kind, name, "file", NamedKindWord(NameFile)) {
				// The kind and never the path, which is what keeps the word
				// comparable on any machine — and is why the operand
				// question is asked below rather than here.
				return 0
			}
			path = r.reportedPath(name, path)
			if r.unspecified {
				return r.status
			}
			r.printf("%s\n", r.TypeExternalSentence(name, path))
			return 0
		}
	}
	if kind != typeKindNone {
		// Nothing at all for a name that is nothing under `-t` — no line and
		// no diagnostic, measured. The status is the whole of the answer,
		// which is what makes it scriptable in the first place. `-w` names it
		// `none` instead, which is the one place the two shapes differ by
		// more than a word: a highlighter reads this line for every word on
		// the line, and silence would be indistinguishable from not asking.
		if kind == typeKindNamed {
			r.printf("%s: %s\n", name, NamedKindWord(NameNotFound))
		}
		return orDefault(dg.TypeNotFoundStatus, 1)
	}
	// Two of the four write this one with nothing in front of it, where every
	// other message they print carries the shell's name — and the same two
	// write it to standard output. reportNameNotFound holds both facts.
	r.reportNameNotFound(notFound)
	return orDefault(dg.TypeNotFoundStatus, 1)
}
