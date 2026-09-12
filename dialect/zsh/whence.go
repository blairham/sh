// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"strings"

	"github.com/blairham/sh/interp"
)

// `whence` and `where` are this shell's questions about a name, and they are
// not ksh93's builtin under the same spelling. Measured 2026-09-05 against
// zsh 5.9.2 in the oracle environment, mode by mode, and the two shells
// differ in every part that could differ:
//
//   - The stream. `whence -v nope` writes `nope not found` to **standard
//     output** here and to standard error in ksh93, so a script redirecting
//     one of them sees a different thing in each.
//   - The status of a usage error. An unknown letter is `bad option: -z` at
//     **1** here, where ksh93 prints a usage line at 2. `whence` with no
//     operand at all is a silent 1 here and a usage error there.
//   - The letters. This shell has `-c`, `-m`, `-w`, `-f`, `-s` and `-x`,
//     which ksh93 has not, and has no `-q`, which ksh93 does.
//   - The wordings, in all four shapes below.
//
// The four output shapes, each measured on an alias, a function, a reserved
// word, a builtin, a file and a name that is nothing:
//
//	shape   alias              function       reserved                builtin
//	bare    the value          the name       the name                the name
//	-v      N is an alias …    the sentence `type` writes, which is this
//	                           shell's `type` exactly — `type` here *is*
//	                           `whence -v`, measured
//	-c      N: aliased to V    the body       N: shell reserved word  N: shell built-in command
//	-w      N: alias           N: function    N: reserved             N: builtin
//
// A file is its path in the first three and `N: command` in `-w`; a name that
// resolves to nothing is silence in the bare shape, `N not found` in `-v` and
// `-c`, and `N: none` in `-w`. Every one of them is a status of 1, and a
// command with several operands reports 1 if any of them missed while still
// answering for the rest.
//
// Resolution order is alias, then whatever this shell would run — and the
// alias part is the one thing the core cannot answer, because whether a word
// expands as an alias is the parser's fact rather than the runner's.
//
// `where` is `whence -ca` and takes **no options of its own**: `where -v echo`
// is `bad option: -v`, measured. So it is not a synonym with a flag set, it is
// a second name whose options are refused.
//
// `-m`, `-s` and `-x` are this shell's and are not implemented, and they are
// refused out loud rather than ignored. `-m` reads the operands as patterns
// and matches them against every name the shell could run, PATH included —
// the answer on the measuring machine was sixty-four lines of /usr/bin — and
// nothing here can walk PATH; `-s` resolves a symlink, which would be the
// same as the bare answer for every name that is not one and silently wrong
// for one that is; `-x` sets the tab width of a printed body. The same rule
// `compgen` follows: an answer that cannot be generated is refused rather
// than guessed.

// The three names are one builtin with three option sets, which is what the
// shell itself does: `which` is `whence -c` and `where` is `whence -ca`, and
// each of them **stops offering the letters its preset already decided**.
// Measured letter by letter against zsh 5.9.2, 2026-09-05:
//
//	whence  -v -p -c -a -w -f      and -m -s -x -S
//	which      -p    -a -w         and -m -s -x -S   (-c -v -f are bad options)
//	where      -p       -w         and -m -s -x -S   (-a -c -v -f are bad options)
//
// That is the rule and not a coincidence: `-c` is already on in both, so it
// cannot be asked for; `-a` is already on in `where`; and `-v` and `-f` are
// the two shapes `-c` displaces, so they go with it. Reading the three sets
// off one table is what keeps them from drifting — `where` used to refuse
// *every* dash word, which was right for `-v` and wrong for `-p` and `-w`,
// both of which this shell answers.
//
// whenceLetters are the letters each name implements.
const (
	whenceLetters = "vpcawf"
	whichLetters  = "paw"
	whereLetters  = "pw"
)

// whenceUnimplemented are the letters zsh has that this one does not, kept
// apart so they are refused as missing rather than as unknown — a script can
// tell a shell that lacks something from a typo. All three names take them,
// and all three refuse them the same way.
//
// `-S` joined the list from the same measurement: it is `-s` that resolves
// every step of a symlink chain rather than the last, and this shell answered
// `bad option` where zsh takes it.
const whenceUnimplemented = "msxS"

// registerWhence installs all three names.
//
// `which` is registered even though /usr/bin/which exists, because a builtin
// shadowing a PATH command is what zsh itself does — #572 left it out on the
// grounds that shadowing was not what it was asked for, and #633 is the
// decision going the other way: the dialect is this shell or it is not.
func registerWhence(r *interp.Runner) {
	r.Register("whence", whenceBuiltin)
	r.Register("which", whichBuiltin)
	r.Register("where", whereBuiltin)
}

// whenceMode is what the letters asked for.
type whenceMode struct {
	verbose bool // -v: the sentence, which is this shell's `type`
	path    bool // -p: the PATH search alone
	csh     bool // -c: the csh-style listing, and what `where` is
	all     bool // -a: every resolution rather than the first
	kind    bool // -w: the bare kind word
	funcs   bool // -f: a function answers with its body
}

func whenceBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	return whenceUnder(r, ctx, args, whenceLetters, whenceMode{})
}

// whichBuiltin is `whence -c` under its own name, with `-c` no longer on
// offer because it is already on.
func whichBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	return whenceUnder(r, ctx, args, whichLetters, whenceMode{csh: true})
}

// whereBuiltin is `whence -ca` the same way.
func whereBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	return whenceUnder(r, ctx, args, whereLetters, whenceMode{csh: true, all: true})
}

// whenceUnder is the one body the three names share: read the letters this
// name offers on top of the preset it was born with, then answer.
func whenceUnder(r *interp.Runner, ctx context.Context, args []string, offered string, m whenceMode) int {
	names, m, code := whenceOptions(r, args, offered, m)
	if code != 0 || len(names) == 0 {
		return code
	}
	return whenceNames(r, ctx, names, m)
}

func whenceNames(r *interp.Runner, ctx context.Context, names []string, m whenceMode) int {
	status := 0
	for _, name := range names {
		if st := whenceOne(r, ctx, name, m); st != 0 {
			status = st
		}
	}
	return status
}

// whenceOptions reads the leading option words.
//
// An unknown letter is `bad option: -z` and 1, and nothing is answered after
// it — measured, the operands are not reached. A letter this shell has and
// this one does not is refused with its own wording, so the two cases stay
// distinguishable.
func whenceOptions(r *interp.Runner, args []string, offered string, m whenceMode) (names []string, mode whenceMode, code int) {
	rest := args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && rest[0] != "-" {
		word := rest[0]
		rest = rest[1:]
		if word == "--" {
			break
		}
		for _, letter := range word[1:] {
			switch {
			case strings.ContainsRune(offered, letter):
				setWhenceLetter(&m, byte(letter))
			case strings.ContainsRune(whenceUnimplemented, letter):
				r.Diagnosef("-%c is not implemented yet\n", letter)
				return nil, m, 1
			default:
				r.Diagnosef("bad option: -%c\n", letter)
				return nil, m, 1
			}
		}
	}
	if len(rest) == 0 {
		// A `whence` with nothing to ask about says nothing and reports 1 —
		// no usage line, which is where ksh93's answer goes the other way.
		return nil, m, 1
	}
	return rest, m, 0
}

func setWhenceLetter(m *whenceMode, letter byte) {
	switch letter {
	case 'v':
		m.verbose = true
	case 'p':
		m.path = true
	case 'c':
		m.csh = true
	case 'a':
		m.all = true
	case 'w':
		m.kind = true
	case 'f':
		m.funcs = true
	}
}

// whenceOne answers for one name.
func whenceOne(r *interp.Runner, ctx context.Context, name string, m whenceMode) int {
	if m.path {
		return whencePath(r, name, m)
	}
	if m.all {
		return whenceAll(r, ctx, name, m)
	}
	if display, value, akind, ok := r.AliasForName(name); ok {
		writeLine(r, aliasAnswer(r, display, value, akind, m))
		return 0
	}
	kind, path := r.ResolveName(name)
	if kind == interp.NameNotFound {
		return whenceMissing(r, name, m)
	}
	writeLine(r, resolvedAnswer(r, name, kind, path, m))
	return 0
}

// whenceAll is `-a`: the alias, then what the shell would run, then every
// PATH hit — measured `x`, `ls`, `/bin/ls` for a name that is all three.
func whenceAll(r *interp.Runner, ctx context.Context, name string, m whenceMode) int {
	found := false
	if display, value, akind, ok := r.AliasForName(name); ok {
		found = true
		writeLine(r, aliasAnswer(r, display, value, akind, m))
	}
	switch kind, _ := r.ResolveName(name); kind {
	case interp.NameFunction, interp.NameBuiltin, interp.NameReserved:
		found = true
		writeLine(r, resolvedAnswer(r, name, kind, "", m))
	case interp.NameNotFound, interp.NameFile:
		// A file is listed by the loop below, which shows every hit rather
		// than the first; nothing found so far leaves it to say so.
	}
	for _, path := range r.LookPathAll(name) {
		found = true
		writeLine(r, resolvedAnswer(r, name, interp.NameFile, path, m))
	}
	if found {
		return 0
	}
	return whenceMissing(r, name, m)
}

// whencePath is `-p`: the PATH search with everything else invisible. A
// function, a builtin and a reserved word are all nobody here, and so is a
// name PATH does not hold — silence and 1, except under `-v`, which says so.
func whencePath(r *interp.Runner, name string, m whenceMode) int {
	path, ok := r.LookPath(name)
	if !ok {
		return whenceMissing(r, name, m)
	}
	writeLine(r, resolvedAnswer(r, name, interp.NameFile, path, m))
	return 0
}

// aliasAnswer words an alias in whichever shape the letters asked for, and
// for whichever of the three kinds the table found it under.
//
// Measured 2026-09-12 with a global `UP` and a suffix `txt`:
//
//	         regular              global                    suffix
//	plain    echo hi              | tr a-z A-Z              cat
//	-w       a: alias             UP: global alias          txt: suffix alias
//	-c       a: aliased to …      UP: globally aliased to … txt: suffix aliased to …
//	-v       a is an alias for …  UP is a global alias …    txt is a suffix alias …
//
// The `-v` sentence is the one the dialect's Diagnostics already carry, taken
// through the core rather than written again here: `type` in this shell *is*
// `whence -v`, so a second spelling would be a second answer to one question.
func aliasAnswer(r *interp.Runner, name, value string, kind interp.AliasKind, m whenceMode) string {
	word, csh := "alias", "aliased to "
	switch kind {
	case interp.AliasGlobalKind:
		word, csh = "global alias", "globally aliased to "
	case interp.AliasSuffixKind:
		word, csh = "suffix alias", "suffix aliased to "
	case interp.AliasAnyKind, interp.AliasRegularKind:
	}
	switch {
	case m.kind:
		return name + ": " + word
	case m.csh:
		return name + ": " + csh + value
	case m.verbose:
		return r.AliasSentence(name, value, kind)
	}
	return value
}

// resolvedAnswer words what the shell would run.
//
// The `-v` sentences are not written here: `type` in this shell *is*
// `whence -v`, measured, so the wordings are the ones the dialect's
// Diagnostics already carry and asking them twice is how two answers to one
// question come to disagree.
func resolvedAnswer(r *interp.Runner, name string, kind interp.NameKind, path string, m whenceMode) string {
	if m.kind {
		return name + ": " + whenceKindWord(kind)
	}
	if m.verbose {
		return verboseSentence(r, name, kind, path)
	}
	switch kind {
	case interp.NameFile:
		return path
	case interp.NameFunction:
		if m.csh || m.funcs {
			if body, ok := r.FunctionText(name); ok {
				return body
			}
		}
	case interp.NameBuiltin:
		if m.csh {
			return name + ": shell built-in command"
		}
	case interp.NameReserved:
		if m.csh {
			return name + ": shell reserved word"
		}
	case interp.NameNotFound:
	}
	return name
}

// whenceKindWord is `-w`'s vocabulary, which is its own: a file is `command`
// here and `file` in the shell whose `type -t` names kinds.
func whenceKindWord(kind interp.NameKind) string {
	switch kind {
	case interp.NameFunction:
		return "function"
	case interp.NameBuiltin:
		return "builtin"
	case interp.NameReserved:
		return "reserved"
	case interp.NameFile:
		return "command"
	case interp.NameNotFound:
	}
	return "none"
}

// verboseSentence is the `type` wording for a resolution, taken from the
// dialect's own Diagnostics so the two builtins cannot drift apart.
func verboseSentence(r *interp.Runner, name string, kind interp.NameKind, path string) string {
	dg := r.Diagnostics
	if dg == nil {
		dg = &interp.Diagnostics{}
	}
	switch kind {
	case interp.NameFunction:
		if autoloadPending(r, name) {
			// A name declared with `autoload` and not yet called is not an
			// ordinary function and does not say it is — measured,
			// `whence -v myfn` is `myfn is an autoload shell function` and
			// becomes `myfn is a shell function from …` after one call. The
			// question is the dialect's own bookkeeping rather than
			// interp's, which is why it is asked here; the wording is still
			// the table's, so this and `type` cannot say it differently.
			return interp.Wording(dg.TypeUndefinedFunction,
				"%[1]s is an undefined function", name)
		}
		return interp.Wording(dg.TypeFunction, "%[1]s is a function", name)
	case interp.NameBuiltin:
		return interp.Wording(dg.TypeBuiltin, "%[1]s is a shell builtin", name)
	case interp.NameReserved:
		return interp.Wording(dg.TypeKeyword, "%[1]s is a shell keyword", name)
	case interp.NameFile:
		return interp.Wording(dg.TypeExternal, "%[1]s is %[2]s", name, path)
	case interp.NameNotFound:
	}
	return name
}

// whenceMissing is a name that resolved to nothing: silence in the bare shape
// and a line in the rest, always on standard output and always 1.
func whenceMissing(r *interp.Runner, name string, m whenceMode) int {
	switch {
	case m.kind:
		writeLine(r, name+": none")
	case m.verbose || m.csh:
		writeLine(r, interp.Wording(r.Diagnostics.TypeNotFound, "%[1]s not found", name))
	}
	return 1
}

// writeLine puts an answer where this shell puts them, which is standard
// output for every one of them — the not-found line included, and that is the
// difference from ksh93 the corpus records.
func writeLine(r *interp.Runner, s string) {
	_, _ = fmt.Fprintf(r.Out(), "%s\n", s)
}
