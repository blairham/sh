// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"sort"
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
// `-m` reads the operands as patterns and answers for every name each one
// matches, in every table — see whencePattern, which carries the measurement.
// It was refused as missing until #5230, on the grounds that nothing here could
// walk PATH; [interp.Runner.PathEntries] is that walk.
//
// **`-s` was on that list and is not now** (#4446), and the sentence it was
// refused with is the specification it is implemented to: it "would be the
// same as the bare answer for every name that is not a symlink and silently
// wrong for one that is". The first half is what zsh writes, measured, and
// the second half is the arrow. `-S` is the same walk with every step named.
// See whenceLinks.

// The three names are one builtin with three option sets, which is what the
// shell itself does: `which` is `whence -c` and `where` is `whence -ca`, and
// each of them **stops offering the letters its preset already decided**.
// Measured letter by letter against zsh 5.9.2, 2026-09-05:
//
//	whence  -v -p -c -a -w -f -s -S    and -m -x
//	which      -p    -a -w    -s -S    and -m -x   (-c -v -f are bad options)
//	where      -p       -w    -s -S    and -m -x   (-a -c -v -f are bad options)
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
	whenceLetters = "vpcawfsSm"
	whichLetters  = "pawsSm"
	whereLetters  = "pwsSm"
	// typeLetters are `type`'s own letters, offered when it is answered here
	// rather than by the core — which it is only when `-m` is among them. See
	// typeBuiltin.
	typeLetters = "apwfsSm"
)

// The letters zsh has that this one does not used to be kept apart here, so
// they were refused as missing rather than as unknown — a script can tell a
// shell that lacks something from a typo. The list is empty now: `-m` was the
// last, built in #5230.
//
// **`-s` and `-S` left that list** (#4446). They are the two depths of
// one walk — see whenceLinks — and the argument for refusing `-s`, that it
// "would be the same as the bare answer for every name that is not a symlink
// and silently wrong for one that is", was the argument for implementing it:
// the bare answer is exactly what `-s` writes when there is no link, and the
// arrow is what it writes when there is.
// **`-x` has left this list** too, and for the reason `-s` did: the letter is
// implemented next door. `functions -x2 f` already wrote a body indented by
// two, and `whence`, `which` and `where` take the same letter with the same
// argument and the same complaint about a missing one — measured 2026-09-29,
// all four names answer `number expected after -x` for `-xa` and for a bare
// `-x`. So the reader is shared rather than written again; see
// interp.Runner.FunctionBodyIndentOption.

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
	if core, ok := r.Builtin("type"); ok {
		r.Register("type", typeBuiltin(core))
	}
}

// typeBuiltin is this shell's `type`, which is `whence -v` — measured, and
// the reason the sentences are shared rather than written twice. The core
// answers every spelling of it but one: `-m`, the pattern lookup, is a walk
// over every table and only `whence` makes it, so a `type` asked for it is
// answered by that walk with `-v` already on.
//
// Measured 2026-09-30 on zsh 5.9.2, `-f` on a script file under `env -i`:
// `type -m 'zba?'` writes exactly the lines `whence -vm 'zba?'` writes, and
// `type -wm`, `-fm`, `-pm` and `-am` write what `whence` writes with the same
// letters and `-v`. A letter `type` has not got is the refusal it always was —
// `type -cm zq` is `bad option: -c` at 1, as `type -c zq` is.
func typeBuiltin(core interp.Builtin) interp.Builtin {
	return func(r *interp.Runner, ctx context.Context, args []string) int {
		if !optionWordsHold(args, 'm') {
			return core(r, ctx, args)
		}
		names, m, code := whenceOptions(r, args, typeLetters, whenceMode{verbose: true})
		if code != 0 || len(names) == 0 {
			return code
		}
		return whenceNames(r, ctx, names, m)
	}
}

// optionWordsHold reports whether a letter is among the leading option words,
// which end at the first operand, at `--` and at a lone `-`.
func optionWordsHold(args []string, letter byte) bool {
	for _, word := range args {
		if word == "--" || word == "-" || !strings.HasPrefix(word, "-") {
			return false
		}
		if strings.IndexByte(word[1:], letter) >= 0 {
			return true
		}
	}
	return false
}

// whenceMode is what the letters asked for.
type whenceMode struct {
	verbose bool // -v: the sentence, which is this shell's `type`
	path    bool // -p: the PATH search alone
	csh     bool // -c: the csh-style listing, and what `where` is
	all     bool // -a: every resolution rather than the first
	kind    bool // -w: the bare kind word
	funcs   bool // -f: a function answers with its body
	link    bool // -s: where the path ends up
	chain   bool // -S: every link on the way there
	pattern bool // -m: each operand is a pattern over every table
}

func whenceBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	return whenceUnder(r, ctx, "whence", args, whenceLetters, whenceMode{})
}

// whichBuiltin is `whence -c` under its own name, with `-c` no longer on
// offer because it is already on.
func whichBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	return whenceUnder(r, ctx, "which", args, whichLetters, whenceMode{csh: true})
}

// whereBuiltin is `whence -ca` the same way.
func whereBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	return whenceUnder(r, ctx, "where", args, whereLetters, whenceMode{csh: true, all: true})
}

// whenceUnder is the one body the three names share: read the letters this
// name offers on top of the preset it was born with, then answer.
func whenceUnder(r *interp.Runner, ctx context.Context, name string, args []string, offered string, m whenceMode) int {
	// `-x` first, because its argument may be attached to the letter or be the
	// word after it, and a scan that has not taken it cannot tell an operand
	// from an argument — `which -x 2 f` is three words and two of them belong
	// to the option. The same reader `functions` uses, so the four names
	// cannot drift about what the letter takes or what it says when the number
	// is missing. See interp.Runner.FunctionBodyIndentOption.
	rest, restore, code := r.FunctionBodyIndentOption(name, args)
	if code != 0 {
		return code
	}
	defer restore()
	args = rest
	names, m, code := whenceOptions(r, args, offered, m)
	if code != 0 || len(names) == 0 {
		return code
	}
	return whenceNames(r, ctx, names, m)
}

func whenceNames(r *interp.Runner, ctx context.Context, names []string, m whenceMode) int {
	if m.pattern {
		return whencePatterns(r, names, m)
	}
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
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		word := rest[0]
		rest = rest[1:]
		if word == "--" {
			break
		}
		if word == "-" {
			// A lone `-`, eaten here and an operand elsewhere. It had been
			// hard-coded as an operand in the loop condition, so
			// `whence - echo` looked the dash up, found nothing, and
			// reported 1 where the reference writes `echo` at 0 (#5040).
			// See interp.Runner.ReadALoneDash.
			reading := r.ReadALoneDash()
			if reading == interp.LoneDashUnanswered {
				return nil, m, 2
			}
			if reading != interp.LoneDashEndsTheOptions {
				// An operand after all, so it goes back — the word was taken
				// off `rest` at the top of the loop before anything knew
				// which it was.
				rest = append([]string{word}, rest...)
			}
			// And the scan stops either way. A `break` inside a switch would
			// have left the switch and not the loop.
			break
		}
		for _, letter := range word[1:] {
			switch {
			case strings.ContainsRune(offered, letter):
				setWhenceLetter(&m, byte(letter))
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
	case 's':
		m.link = true
	case 'S':
		m.chain = true
	case 'm':
		m.pattern = true
	}
}

// whencePatterns is `-m`: every operand is a pattern, and the answer is every
// name it matches in every table, each in that table's own shape.
//
// Measured 2026-09-30 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f` on a script
// file under `env -i PATH=/usr/bin:/bin LC_ALL=C`), with PATH set to two
// scratch directories — `p2` holding zab, zbar, zbaz and `p1` holding zfoo,
// zbar, zqux, zz, a directory zdir and a file znoexec without the execute bit
// — an alias zbar and a function zbaz:
//
//   - **The tables in turn, and every match in each.** Aliases (regular and
//     global together, never suffix), then reserved words, then functions,
//     then builtins, then commands. `whence -m 't*'` with an alias `tal` and a
//     function `tfn` is `tal`'s value, then `then`, `time`, `typeset`, then
//     `tfn`, then the builtins from `test` on. A name in two tables is
//     answered twice: `zbaz` is the function and then `…/p2/zbaz`, and `local`
//     is the reserved word and then the builtin.
//   - **Sorted within a table**, not in definition order: functions defined
//     zq then za are listed za, zq; aliases zy, zb (global), zc are listed by
//     name across both kinds.
//   - **The command table is the directories' listing**, the first directory
//     winning: zdir and znoexec are answered, and `zbar` once, from p2. A name
//     `hash` put there is in it too — `hash zman=/bin/ls` makes `whence -m
//     'zm*'` write `/bin/ls` — and is not a PATH hit.
//   - **`-a` makes the command rows a search** instead: every PATH hit of
//     every matching name, so `zbar` twice and zdir, znoexec and the hashed
//     zman not at all. `-p` keeps only the command rows, with or without it.
//   - **`-s` and `-S` reach only the `-a` rows.** With `zlink -> /bin/ls` on
//     PATH, `whence -sm zlink`, `-Sm`, `-psm` and `type -sm` write the path
//     bare, where `whence -s zlink`, `whence -asm 'zl*'`, `-apsm` and `type
//     -sam` write the arrow — the table's rows are written as the table
//     holds them and only a search resolves.
//   - **A miss is silence in every shape**, `-v`, `-c` and `-w` included, and
//     the status is 1 only when no operand matched anything: `whence -m
//     'nos*' zfoo` and `whence -m zfoo 'nos*'` are both 0. A match is a name
//     in a table and not a line written — `whence -am qqdir` writes nothing
//     at 0, because the table holds the directory and the search finds no
//     file to run.
//   - **An operand matching in two patterns is written twice** — `whence -m
//     'zf*' 'zfo*'` repeats both lines.
//
// The pattern is the shell's own, so `-m 'z[ab]*'` and `-m '(zfoo|zz)'` match
// as `case` would and a plain name matches itself. A disabled builtin is not
// listed — `disable zle` takes it out of `whence -m 'zl*'` — because
// [interp.Runner.BuiltinNames] already leaves out what is switched off or
// withdrawn.
//
// What is not modeled: zsh fills its command table once and answers from it
// until `rehash`, so a file added to PATH later can be missing from its
// listing. This walks the directories each time.
func whencePatterns(r *interp.Runner, patterns []string, m whenceMode) int {
	found := false
	for _, pattern := range patterns {
		if whencePattern(r, pattern, m) {
			found = true
		}
	}
	if found {
		return 0
	}
	return 1
}

// whencePattern answers for one pattern and reports whether it matched.
func whencePattern(r *interp.Runner, pattern string, m whenceMode) bool {
	found := false
	matching := func(names []string) []string {
		var out []string
		for _, name := range names {
			if r.MatchPattern(pattern, name) {
				out = append(out, name)
			}
		}
		sort.Strings(out)
		return out
	}
	if !m.path {
		var aliases []string
		for name := range r.AliasTable() {
			aliases = append(aliases, name)
		}
		for name := range r.GlobalAliasTable() {
			aliases = append(aliases, name)
		}
		for _, name := range matching(aliases) {
			if display, value, akind, ok := r.AliasForName(name); ok {
				found = true
				writeLine(r, aliasAnswer(r, display, value, akind, m))
			}
		}
		tables := []struct {
			kind  interp.NameKind
			names []string
		}{
			{interp.NameReserved, zshReservedWords},
			{interp.NameFunction, r.ListedFuncNames()},
			{interp.NameBuiltin, r.BuiltinNames()},
		}
		for _, table := range tables {
			for _, name := range matching(table.names) {
				found = true
				writeLine(r, resolvedAnswer(r, name, table.kind, "", m))
			}
		}
	}
	commands := r.PathEntries()
	for _, name := range r.HashedCommandNames() {
		if path, ok := r.HashedCommandPath(name); ok {
			commands[name] = path
		}
	}
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	for _, name := range matching(names) {
		// A name the table holds is a match whether or not the search below
		// finds a file to write for it: measured, `whence -am qqdir` over a
		// directory on PATH and `whence -am 'qqm*'` over a name only `hash`
		// put there both write nothing and answer 0.
		found = true
		if m.all {
			for _, path := range r.LookPathAll(name) {
				writeLine(r, resolvedAnswer(r, name, interp.NameFile, path, m))
			}
			continue
		}
		// A row read from the table is written as the table holds it: `-s`
		// and `-S` draw no arrow here, and do on the `-a` rows above.
		table := m
		table.link, table.chain = false, false
		writeLine(r, resolvedAnswer(r, name, interp.NameFile, commands[name], table))
	}
	return found
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
	// Every resolution inside the shell rather than the first of them. One
	// name is a reserved word *and* a builtin here for all seven declaration
	// commands, and `whence -a export` in zsh 5.9.2 writes a line for each —
	// three of them with a function of that name defined. A file is listed by
	// the loop below, which shows every hit rather than the first (#3291).
	for _, kind := range r.NameKinds(name) {
		found = true
		writeLine(r, resolvedAnswer(r, name, kind, "", m))
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
//
// `-a` beside it widens the walk to **every** hit rather than the first, in
// either order: measured 2026-09-18 on zsh 5.9.2 with two copies of one name
// on PATH, `whence -ap` and `whence -pa` both write two lines where `whence
// -p` writes one. The two letters compose — `-a` says how many rows there
// are and `-p` says that a row is a PATH hit — which is the same composition
// the ksh dialect's `whence` was missing (#3198).
func whencePath(r *interp.Runner, name string, m whenceMode) int {
	if m.all {
		paths := r.LookPathAll(name)
		if len(paths) == 0 {
			return whenceMissing(r, name, m)
		}
		for _, path := range paths {
			writeLine(r, resolvedAnswer(r, name, interp.NameFile, path, m))
		}
		return 0
	}
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
	// The `-w` word is taken from the core rather than spelled again here,
	// for the reason the `-v` sentence below already is: `type` in this
	// shell *is* `whence -v`, so one question gets one answer. The `-c`
	// phrase stays local because only this letter has one.
	word := interp.AliasKindWord(kind)
	csh := "aliased to "
	switch kind {
	case interp.AliasGlobalKind:
		csh = "globally aliased to "
	case interp.AliasSuffixKind:
		csh = "suffix aliased to "
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
	// `-f` on a function is the body whatever else was asked for: measured
	// 2026-09-30 on zsh 5.9.2, `whence -vf zq`, `whence -fv zq`, `whence -wf
	// zq` and `whence -vfa zq` all write the definition and nothing else,
	// while `whence -vf echo` is still the builtin's sentence — the letter
	// is about functions and leaves every other kind to the shape. It is also
	// what `type -fm` needs, since `type` is `whence -v`.
	if kind == interp.NameFunction && m.funcs {
		if body, ok := r.FunctionText(name); ok {
			return body
		}
	}
	if m.kind {
		return name + ": " + whenceKindWord(kind)
	}
	if m.verbose {
		// The arrow goes **after** the sentence rather than inside the path
		// it names, and the difference shows only on a path with a space in
		// it: measured, `whence -sv sp` is `sp is '/…/w s/sp' -> /bin/ls`, so
		// the quoting the dialect puts round a path reaches the path and not
		// the resolution after it. Rendering the whole string first and
		// handing *that* to the sentence quoted the arrow too.
		if kind == interp.NameFile {
			return verboseSentence(r, name, kind, path) + whenceLinkArrow(r, path, m)
		}
		return verboseSentence(r, name, kind, path)
	}
	switch kind {
	case interp.NameFile:
		return whenceLinks(r, path, m)
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

// whenceKindWord is `-w`'s vocabulary, which this shell shares with its own
// `type -w` — the same letter on the two names for one builtin.
//
// One table, in interp beside the resolution it names, so that a kind added
// later cannot be worded here and forgotten there. See interp.NamedKindWord,
// which carries what was measured and how it differs from the other dialect's
// `-t`.
func whenceKindWord(kind interp.NameKind) string { return interp.NamedKindWord(kind) }

// verboseSentence is the `type` wording for a resolution, taken from the
// dialect's own Diagnostics so the two builtins cannot drift apart.
func verboseSentence(r *interp.Runner, name string, kind interp.NameKind, path string) string {
	dg := r.Diagnostics
	if dg == nil {
		dg = &interp.Diagnostics{}
	}
	switch kind {
	case interp.NameFunction:
		// The whole sentence from the core, autoload stub and origin alike —
		// `myfn is an autoload shell function` before the first call and
		// `myfn is a shell function from /…/myfn` after it. Written there
		// rather than here because `type` writes the identical line, and the
		// two spelling it separately is exactly how they came to disagree
		// about the origin (#1706).
		return r.FunctionSentence(name)
	case interp.NameBuiltin:
		// From the core, for the reason the function line is: `type` writes
		// the identical sentence, and a special builtin is worded differently
		// from an ordinary one in four of the seven columns. This shell is
		// not one of them — `whence -v .` is `. is a shell builtin` in zsh
		// 5.9.2, measured 2026-09-16 — and that answer is the dialect's
		// TypeDistinguishesSpecialBuiltins rather than a second wording
		// written here, so the two builtins cannot come to disagree.
		return r.BuiltinSentence(name)
	case interp.NameReserved:
		return interp.Wording(dg.TypeKeyword, "%[1]s is a shell keyword", name)
	case interp.NameFile:
		// From the core too, for the reason the two lines above are: `type`
		// writes the identical sentence and a second spelling of it is how
		// the two drift. This line had its own copy of the wording, which
		// was the same string until the path inside the sentence learned to
		// be quoted — see Diagnostics.TypeSentencePathQuoting, which
		// `whence -v 'a b'` answers and this copy did not (#3702).
		//
		// The path handed to it is already resolved when `-s` or `-S` asked
		// for that, measured: `whence -sv myls` is `myls is …/myls -> /bin/ls`,
		// so the arrow goes *inside* the sentence rather than beside it.
		return r.TypeExternalSentence(name, path)
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

// whenceLinks is `-s` and `-S`: what the path this shell found actually
// resolves to, written as an arrow after it.
//
// Measured 2026-09-26 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`), run `-f`, with
// `myls -> /bin/ls`, a chain `c -> b -> sub/a -> /bin/ls`, and `realecho` a
// real file, each found on PATH:
//
//	           -s                          -S
//	realecho   …/realecho                  …/realecho
//	myls       …/myls -> /bin/ls           …/myls -> /bin/ls
//	c          …/c -> /bin/ls              …/c -> …/b -> …/sub/a -> /bin/ls
//
// so the two letters are one walk read to two depths: `-s` names where the
// path ends up and `-S` names every path on the way. `realecho` is the
// control and it is load-bearing in both columns — pointed at a command that
// is a real file the two letters print exactly what the bare form prints, and
// a shell that had neither letter would agree on that row. The chain is what
// tells `-s` from `-S`, and the single link is what tells either from nothing.
//
// **`-w` is not this question and is answered before this is reached.**
// Measured — `whence -sw myls` and `whence -Sw myls` are both `myls: command`,
// the same as `whence -w myls` — so the kind word is never decorated. The
// other three shapes are: the bare form, `-c` (which is `which` and `where`)
// and `-v` (which is `type`) each write the arrow, and the `-v` one writes it
// inside its sentence.
//
// **`-S` wins when both are given**, in either order: `whence -sS` and
// `whence -Ss` both write the full chain.
//
// A name that is not a file never reaches here. A builtin, a function, a
// reserved word and an alias are what they are, and a name PATH does not hold
// is the ordinary silence at 1 — measured, `whence -s nosuchcmd` says nothing.
func whenceLinks(r *interp.Runner, path string, m whenceMode) string {
	return path + whenceLinkArrow(r, path, m)
}

// whenceLinkArrow is the part of that answer that follows the path — empty
// when neither letter was given and when nothing on the way is a link.
//
// Kept apart from the path so the `-v` sentence can put it outside the
// quoting that shape wraps a path in. See resolvedAnswer.
func whenceLinkArrow(r *interp.Runner, path string, m whenceMode) string {
	if !m.link && !m.chain {
		return ""
	}
	// The walk and the rendering are the core's, because `type -s` in this
	// same shell writes this identical tail and a second copy is how two
	// spellings of one question come to disagree. See interp.Runner.SymlinkArrow.
	return r.SymlinkArrow(path, m.chain)
}
