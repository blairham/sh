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
	// `-p` means "search a default PATH that finds the standard utilities",
	// which is the whole point of the letter: a script reaches for it when
	// PATH is the thing it cannot trust. It was read and then ignored here,
	// on the reasoning that ours is already the Runner's PATH rather than the
	// process's — true, and beside the point, since the case the letter is
	// written for is a PATH the *script* has cleared or mangled (#2933). See
	// interp/defaultpath.go. `-v` and `-p` are read by all four. What splits them is a leading `-`
	// word that is *not* one of those: bash, dash and ksh93 refuse it as an
	// option, and zsh alone stops reading options and takes it as the
	// command, so `command -q ls` is `command not found: -q` there. Probed
	// with a letter no panel shell owns — `-x` is a real ksh93 option, and
	// the first measurement read ksh93 off it, wrongly.
	verbose, sentence, defaultPath := false, false, false
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
			defaultPath = defaultPath || strings.ContainsRune(letters, 'p')
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
	if !verbose && !sentence && r.expandedCommandOnlyReports() {
		// The word `command` arrived through an expansion rather than being
		// written, and one dialect gives it no power to run anything there:
		// what it does instead is exactly what `-v` does. Asked after the
		// letters are read, because a written `-V` still wins — measured,
		// `c=command; $c -V echo` is the sentence at 0 while `$c echo hi`
		// names `echo` and runs nothing. See
		// Semantics.ExpandedCommandOnlyReports.
		verbose = true
	}
	if sentence || verbose {
		// `-p` asks the reporting letters the same question it asks the run:
		// `command -pv sed` names what `command -p sed` would start, and a
		// script that tested one and ran the other would be testing the
		// wrong PATH. Measured — all four columns answer `/usr/bin/sed` here
		// with a PATH that holds nothing.
		defer r.searchingTheDefaultPath(defaultPath)()
		return r.reportEveryOperand(args, sentence)
	}
	// What the command reports is the *command's*, not this builtin's. The
	// dialect that names a builtin in the location says
	// `sh:1: command not found: -x` and not `sh:command:1:` — the same rule
	// `.` and `eval` follow for the text they run, and for the same reason.
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()
	if args[0] == "local" {
		// Two columns run the word for its operand checks and put the
		// declaration nowhere — see
		// Semantics.LocalThroughCommandDeclaresNothing, which biLocal asks
		// once it is running. The flag says only that the prefix is there;
		// the axis is asked where the declaration would happen, so the two
		// columns that never find `local` through this builtin at all are
		// never asked a question their own shell cannot pose (#3370).
		//
		// Set only where the word itself is `local`, so `command eval
		// 'local a=1'` is the ordinary declaration it is in those shells:
		// this builtin has the name in front of it, and the one behind an
		// `eval` never reaches here.
		held := r.localUnderCommandPrefix
		r.localUnderCommandPrefix = true
		defer func() { r.localUnderCommandPrefix = held }()
	}
	return r.runWithoutFunctions(ctx, args, defaultPath)
}

// biBuiltin runs a builtin, and only a builtin.
//
// bash and zsh have it; dash has no such command, and ksh93 has one of the
// same name that does something else entirely — it *registers* builtins — so
// this is registered per dialect rather than being part of the substrate.
func biBuiltin(r *Runner, ctx context.Context, args []string) int {
	for len(args) > 0 && args[0] == "-" &&
		r.ask(r.sem().LoneDashIsAnOption, "a lone `-` given to `builtin` being eaten") {
		// A lone dash, which the dialect that eats one everywhere eats here
		// too. Before the dash-word reading below rather than inside it,
		// because that reading is off in exactly the shell this is on: `zsh`
		// answers BuiltinReadsOptions no and still consumes the dash.
		//
		// Measured 2026-09-18 on zsh 5.9.2 from a script file: `builtin -
		// echo x` prints `x` at 0, `builtin -` alone is silent at 0, and
		// `builtin - -` is silent at 0 too — so the eating repeats while the
		// word is a lone dash rather than happening once. `builtin -q` and
		// `builtin -- echo hi` are each `no such builtin:` the word, so
		// neither the bundle nor the terminator is read here. bash eats
		// nothing: `builtin -` there is `-: not a shell builtin` at 1, which
		// is the same answer this shell already gave and keeps (#3472).
		args = args[1:]
	}
	if r.unspecified {
		return r.status
	}
	if len(args) > 0 && len(args[0]) > 1 && args[0][0] == '-' {
		// A dash-word where a builtin's name goes, which only the dialect
		// can say is an option. See Semantics.BuiltinReadsOptions.
		if r.ask(r.sem().BuiltinReadsOptions, "`builtin` reading options") {
			rest, _, code := r.builtinOptions("builtin", args, "")
			if code != 0 {
				return code
			}
			args = rest
		} else if r.unspecified {
			return r.status
		}
	}
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

// reportEveryOperand is the reporting half of `command`, over however many
// names were given.
//
// **How many of them are read at all** is the dialect's: bash 5.3.20, zsh
// 5.9.2 and ksh93u+ answer for every operand, and dash 0.5.12 and BusyBox ash
// 1.37.0 answer for the first and stop. Measured 2026-09-18 with `command -v
// echo shift`, which is two lines in the first three and one in the last two,
// and again with `-V`. This engine had dash's answer in every column, so a
// portable `command -v a b` reported half of what three of the five shells
// report — silently, since the missing line is the second one.
//
// **What the status is when the answers are mixed** is the dialect's too, and
// it is a separate question: `command -v echo nosuch` is 0 in bash and 1 in
// zsh and ksh93, from the identical single line of output. See
// Semantics.CommandReportsEveryOperand and
// Semantics.CommandCountsAMissingOperand.
func (r *Runner) reportEveryOperand(names []string, sentence bool) int {
	if len(names) > 1 {
		// Asked where the columns disagree and nowhere else: one operand has
		// no second answer to differ about, and a shell that never writes
		// `command -v a b` should not have to answer for a shape it does not
		// use.
		if !r.ask(r.sem().CommandReportsEveryOperand, "`command -v` answering for more than one name") {
			if r.unspecified {
				return r.status
			}
			names = names[:1]
		}
	}
	found, missing, status := false, false, 0
	for _, name := range names {
		var st int
		if sentence {
			// `type`'s sentence with `command`'s name on the complaint: the
			// found wordings are shared and only the missing one is this
			// builtin's own — see Diagnostics.CommandVNotFound.
			st = r.describeName(name, typeKindNone, false,
				Wording(r.diag().CommandVNotFound, "command: %[1]s: not found",
					r.NameReportWord(name)))
		} else {
			st = r.reportWhatRuns(name)
		}
		if st == 0 {
			found = true
			continue
		}
		missing = true
		// The failing status is the one this dialect gives a name it could
		// not find — 1 in three columns and 127 in the two that stop at the
		// first operand — so it is kept from the answer rather than written
		// here.
		status = st
	}
	switch {
	case !missing:
		return 0
	case !found:
		return status
	}
	if r.ask(r.sem().CommandCountsAMissingOperand,
		"a name `command -v` could not find deciding the status beside one it could") {
		return status
	}
	if r.unspecified {
		return r.status
	}
	return 0
}

// expandedCommandOnlyReports reports whether this call reached `command`
// through an expansion in a dialect that gives such a call no power to run.
//
// Measured 2026-09-16 and again 2026-09-18 on ksh93u+ 2012-08-01, script
// files under `env -i PATH=/usr/bin:/bin LC_ALL=C`, with `c=command`:
//
//	command echo hi     hi, 0            — written, and quoting does not matter
//	"command" echo hi   hi, 0
//	\command echo hi    hi, 0
//	$c echo hi          echo, 1          — named, and `hi` was not found
//	${c} echo hi        echo, 1
//	"$c" echo hi        echo, 1
//	$(echo command) …   echo, 1
//	eval '$c echo hi'   echo, 1
//	$c -v echo          echo, 0
//	$c -V echo          echo is a shell builtin, 0
//	$c shift            shift, 0
//
// So it is the **word** and not the text: a quoted or backslashed `command`
// still runs, and one that arrived from a parameter, a substitution or an
// `eval` does not. The sharp row is the fourth, where a script that reaches
// `command` through a variable prints a word and runs nothing (#3369).
func (r *Runner) expandedCommandOnlyReports() bool {
	if r.commandWordWasWritten {
		return false
	}
	return r.ask(r.sem().ExpandedCommandOnlyReports,
		"an expanded `command` reporting rather than running")
}

// reportWhatRuns answers `command -v`.
//
// A builtin or a function is named as it was written; an external found on
// PATH is named by the path that would be run, because that is the part a
// script cannot work out for itself. An operand that was *already* written
// with a slash was never searched for, and what is written for it splits the
// panel — see Runner.reportedPath. Nothing found is a failure with no output
// at all — the silence is what makes `command -v x >/dev/null` the usual
// spelling.
func (r *Runner) reportWhatRuns(name string) int {
	// An alias first, and as a *definition* rather than as a bare name: the
	// three shells that print `alias a='echo hi'` here are printing a line
	// that would put the alias back, which is what makes `command -v`'s
	// output usable for more than a yes-or-no. ksh93 prints the body alone.
	if display, value, kind, ok := r.AliasForName(name); ok {
		r.printf("%s\n", r.commandVAliasLine(display, value, kind))
		return 0
	}
	if r.unspecified {
		return r.status
	}
	if _, ok := r.reportedFunc(name); ok {
		r.printf("%s\n", name)
		return 0
	}
	if _, ok := r.lookupBuiltin(name); ok {
		r.printf("%s\n", name)
		return 0
	}
	if r.reservedWord(name) {
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
		if path, err := r.lookPathReporting(name); err == nil {
			path = r.reportedPath(name, path)
			if r.unspecified {
				return r.status
			}
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

// reservedWord reports whether a name is part of *this dialect's* grammar
// rather than a command. `command -v if` answers `if` in every shell in the
// panel; `command -v select` answers the word in four of them and 127 in the
// one that has no `select` loop.
//
// **Asked of the grammar rather than written out here**, which is what #2918
// is. A list in this package is a claim about a language this package does
// not define, and it drifted the moment a dialect without the constructs
// arrived: our dash answered `[[`, `]]`, `select` and `function` as runnable
// words it then refused to run, and answered `time` with the keyword where
// dash has no such keyword and a script asking `command -v time` is looking
// for the path of `/usr/bin/time`. See [syntax.Dialect.Reserves] for the
// measurement and for the two spellings the lexer does not class as words.
func (r *Runner) reservedWord(name string) bool {
	if r.reservedWords != nil {
		return r.reservedWords(name)
	}
	return r.dialect().Reserves(name)
}

// runWithoutFunctions runs a command with the function table ignored, which
// is the whole of what `command name` means: a function may then wrap the
// thing it is named after without calling itself.
func (r *Runner) runWithoutFunctions(ctx context.Context, args []string, defaultPath bool) int {
	// Or with the builtin table ignored as well, in the dialect where the
	// word asks for an external program and nothing else. There the lookup
	// is skipped entirely rather than tried and discarded: `command set` has
	// to reach the PATH search and come back `command not found: set`, which
	// is the answer measured, and a builtin found here would never get
	// there. See Semantics.CommandReachesABuiltin.
	if fn, ok := r.lookupBuiltin(args[0]); ok &&
		r.ask(r.sem().CommandReachesABuiltin, "`command` in front of a builtin running that builtin") {
		outer := r.inBuiltin
		r.inBuiltin = args[0]
		// And the word takes this builtin's specialness away for the length
		// of the call, which is what makes its failure survivable. Saved and
		// put back rather than cleared, because a builtin can run another
		// one: `command eval 'command set -Z'` is two of these nested.
		outerCommand := r.throughCommandWord
		r.throughCommandWord = true
		// callBuiltin folds a failed write here as well as at the outer
		// dispatch, so it is blamed on the builtin that wrote:
		// `command echo hi >&-` names `echo`, not `command`. Measured —
		// bash words it identically with and without the wrapper.
		st := r.callBuiltin(ctx, args[0], fn, args[1:])
		r.takeSpecialBuiltinFailure()
		r.throughCommandWord = outerCommand
		r.inBuiltin = outer
		return st
	}
	// And the default path, if `-p` asked for one — around the lookup the
	// exec makes and around nothing else. A builtin does not come through
	// here, which is the line the measurement draws: `command -p eval 'ls'`
	// looks `ls` up on the caller's PATH in every column.
	defer r.searchingTheDefaultPath(defaultPath)()
	if err := r.exec(ctx, args, r.environ()); err != nil {
		r.diagf("command: %v\n", err)
		return 1
	}
	return r.status
}

// takeSpecialBuiltinFailure ends a special builtin's failure at the `command`
// that ran it, instead of letting it end the script.
//
// POSIX gives this as the reason the word exists, and it is the half of
// Semantics.CommandReachesABuiltin that was never written: reaching the
// builtin was implemented and surviving it was not, so `command set -Z` — the
// only spelling a script has for catching a fatal `set` refusal — ended the
// script in every dialect that calls such a refusal fatal (#2741).
//
// Measured 2026-09-13 from a script file under `env -i`, `command set -Z`
// followed by an `echo`, against the same two lines with the `command` taken
// off. The control is what makes the row about the word rather than about the
// refusal: dash, ksh93, BusyBox ash and bash invoked as `sh` all print the
// refusal and stop without it, and all four print the refusal, report 2 and
// carry on with it. bash under its own name never stops here either way, and
// zsh's `command` does not reach a builtin at all — so those two columns say
// nothing, and the four that speak are unanimous. Asked of no dialect for
// that reason.
//
// **Here rather than at each fatality**, because a special builtin has many
// ways to fail and they are decided in as many places: `set`'s refusal has an
// axis per spelling, an unknown option letter goes through
// badOptionEndsTheScript, a `shift` past the end and a `shift` whose count is
// not a number have two more, and `.` on a file that will not open has
// another. Every one of them is survivable in front of `command`, measured in
// the same run — `command shift 99`, `command shift -1`, `command shift abc`,
// `command readonly 1bad=x`, `command unset 1bad`, `command export 1bad=x`,
// `command . /nonexistent/file` and `command return abc` each report and
// carry on in bash 5.3, bash as `sh`, ksh93, dash and BusyBox ash. bash 3.2
// parts from them on exactly one, `command shift abc`, which it stops for;
// no dialect here claims that build, and it is the same column that answers
// BadOptionToSpecialBuiltinFatalInPosixMode per builtin. A check at each site
// would be eight copies of one rule, and the ninth failure added later would
// not have it.
//
// **It takes an error and never a request to stop**, which is the same line
// abandonKind draws for `.` and `eval`. Measured in the same run: `command
// exec /nonexistent/prog` ends the shell in every column but zsh — 127, and
// 126 in bash 3.2 — `command eval 'exit 5'` exits 5, and `set -e; command
// eval false` stops. None of them is a failure `command` is meant to swallow,
// and zsh is silent on all three for the reason it is silent above.
//
// **And only the builtin's own** in the narrow reading, which is what
// Runner.throughCommandWord still being set says: Runner.stmt clears it, so a
// flag that survived the call means nothing this builtin ran raised the error.
// That is where the panel parts — bash invoked as `sh` ends the script for
// `command eval 'export -q'`, while ksh93, dash and BusyBox ash end only the
// `eval`'s text. Those three put a **boundary** at the word and catch *any*
// fatal error raised inside it, an unset parameter under `set -u` and a
// readonly reassignment included, which is a second mechanism rather than a
// wider reading of this one. It is measured and asked as
// Semantics.FatalErrorEndsAtTheCommandWord (#2755).
//
// The wide reading needs no depth of its own: this function is called from
// the one place that *is* the boundary, so a pending error here was raised
// somewhere under this `command` whatever the flag says.
func (r *Runner) takeSpecialBuiltinFailure() {
	if !r.pendingFileError() {
		return
	}
	// The narrow half first, so a dialect answering the axis no still gets
	// the failure the named builtin raised before it ran anything — the two
	// readings are nested and not alternatives.
	if r.throughCommandWord ||
		r.ask(r.sem().FatalErrorEndsAtTheCommandWord, "`command` ending a fatal error raised inside the builtin it ran") {
		r.takeFileError()
	}
}
