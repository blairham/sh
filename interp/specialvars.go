// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The parameters a shell provides without a script setting them.
//
// Five of them are unanimous across the panel and belong here; the rest are
// not, and belong to whichever dialects have them. Which *variables* a shell
// provides is the same kind of question as which builtins it has — neither
// grammar nor a conflict about meaning — so it is answered through the same
// seam, in each dialect's Apply.
//
// This was found by running a real script rather than by the corpus. A system
// script began `if [ $UID -ne 0 ]`, and with UID unset that is `[ -ne 0 ]` —
// which is not the same test and does not fail in the same way.
//
// Two of the five have to be produced when they are read rather than stored:
// LINENO is wherever execution has reached, and `$-` is whatever `set` has
// done by the time it is read — a stored copy of either would describe the
// line, and the options, the shell started with. That is what Dynamic is for.

// ensureSpecials gives the unanimous parameters their values.
//
// IFS is the one that matters most and the one that looked least urgent:
// splitting already used a default when it was unset, so everything *worked*
// while `echo "$IFS"` printed nothing and a script could neither read it nor
// tell it had been changed.
// Each producer here is registered only while nothing has ended it. The guard
// is not defensive: this runs at the head of every chunk RunPart is handed and
// a script is read incrementally, so without it a `LINENO` an `unset` and an
// assignment had turned into an ordinary name (see
// Semantics.AssignmentRestoresAnUnsetProducedParameter) was produced again by
// the next line of the same script.
func (r *Runner) ensureSpecials() {
	if r.Dynamic == nil {
		r.Dynamic = map[string]func(*Runner) string{}
	}
	if _, ok := r.Vars["IFS"]; !ok && !r.removed["IFS"] {
		r.setVarQuietly("IFS", " \t\n")
	}
	if _, ok := r.Dynamic["_"]; !ok && !r.endedProducers["_"] &&
		r.sem().UnderscoreIsAParameterAtAll != No {
		// Registered once, not every time: RunPart comes back through here
		// while a process substitution's goroutine may be reading the shared
		// table, and rewriting the same producer was a write all the same.
		//
		// And not registered at all where the shell has no such parameter,
		// which is the whole of Semantics.UnderscoreIsAParameterAtAll. A
		// registered producer *is* the name being set — the lookup answers
		// from it ahead of every table and `${_+x}` never calls it — so
		// there is no value a producer could return that reads as unset.
		// dash and BusyBox ash both stop a `set -u` script on `$_` and this
		// shell answered `[]` at 0 in their columns until the question was
		// asked (#3380).
		//
		// An `_` the environment brought still shows through, in those two
		// as in the others: with nothing registered the name is found the
		// way every inherited variable is, which is what their references do
		// with it — `_` is an ordinary name there, assignable, and the value
		// stays.
		r.Dynamic["_"] = func(r *Runner) string {
			// The tracked argument in the dialects that move `$_`. Which
			// record is read is a second question with a second answer —
			// one column moves it only between the commands the shell reads
			// — so the axis above decides *whether* and underscoreValue
			// decides *which*.
			if value, ok := r.underscoreWrittenValue(); ok {
				return value
			}
			if r.unspecified {
				return ""
			}
			if (r.lastArgSet || r.inputLastArgSet) &&
				r.ask(r.sem().UnderscoreTracksTheLastArgument, "`$_` following the last argument") {
				if value, ok := r.underscoreValue(); ok {
					return value
				}
			}
			return r.underscoreAtStartup()
		}
	}
	if _, ok := r.Vars["PPID"]; !ok && !r.removed["PPID"] {
		r.setVarQuietly("PPID", strconv.Itoa(os.Getppid()))
	}
	if _, ok := r.Vars["OPTIND"]; !ok && !r.removed["OPTIND"] {
		// The index the next `getopts` will read, which is 1 before the
		// builtin has ever run — unanimous across the panel, so it is a
		// starting value here rather than an axis. Writing it here and not
		// inside `getopts` is the whole point: a script that tests OPTIND
		// before entering its loop, or that is handed no options at all,
		// reads a number in every real shell and read nothing here.
		//
		// It is written into Vars rather than left to the environment
		// because the panel *overwrites* what the environment brought:
		// `OPTIND=7 sh -c 'echo $OPTIND'` prints 1 in all six, so an
		// inherited value must not show through.
		r.setVarQuietly("OPTIND", "1")
	}
	// OLDPWD is the one parameter here the panel disagrees about *inheriting*
	// rather than providing, so it is a policy rather than a starting value.
	r.settleInheritedOldpwd()
	// And the other parameter whose value is decided by what the shell was
	// handed rather than by what it is: the depth. Beside OLDPWD because it
	// is settled once per session for the same reason — see settleShellLevel.
	r.settleShellLevel()
	if _, ok := r.Dynamic["LINENO"]; !ok && !r.endedProducers["LINENO"] {
		r.Dynamic["LINENO"] = func(r *Runner) string {
			// One dialect numbers lines inside a function from the line the
			// function was written on; the rest count from the file, which
			// r.line already is. Asked only inside a function.
			//
			// Inside, meaning the line being run is one the function body
			// holds — the innermost frame decides, exactly as it does for a
			// diagnostic's location. A file the function sourced counts from
			// itself: measured on zsh 5.9.2, `$LINENO` on the first line of
			// such a file is 1 and not the offset into the function (#2037).
			//
			// lineNow rather than r.line, for the one construct that has
			// not advanced it yet: see
			// Semantics.CaseSubjectKeepsThePreviousLine.
			at := r.lineNow()
			if r.locationIsInsideAFunctionBody() && r.funcLine > 0 &&
				r.ask(r.sem().LinenoCountsFromTheFunction, "`$LINENO` inside a function counting from it") {
				return strconv.Itoa(at - r.funcLine)
			}
			return strconv.Itoa(at)
		}
	}
	if _, ok := r.Dynamic["-"]; !ok && !r.endedProducers["-"] {
		r.Dynamic["-"] = (*Runner).optionLetters
	}
}

// optionLetters is `$-`: the single-letter options currently in effect.
//
// Produced when it is read for the same reason LINENO is — it changes with
// every `set`, and a stored copy would be the options the shell started with.
// Scripts branch on it: `case $- in *e*)` is the standard errexit check, and
// while this expanded to nothing both arms of that test silently took the
// wrong branch.
//
// The string is the dialect's startup letters and a letter for each option
// this runner tracks and has on, put into the order the dialect publishes
// them in — Semantics.DollarDashLetterOrder, because no two shells in the
// panel order the merged string the same way and one of the four orders is
// its own option table's. Presence is still the contract a script can rely
// on: `case $- in *e*)` is what scripts write and equality against a whole
// string is what nothing realistic writes. pipefail earns no letter anywhere,
// which is unanimous and so needs no axis; the noglob letter is the one the
// shells disagree on.
func (r *Runner) optionLetters() string {
	var b strings.Builder
	b.WriteString(r.startupOptionLetters())
	if r.Interactive {
		// Measured unanimous and so no axis: every shell in the panel puts
		// `i` here for an interactive shell and none of them puts it there
		// for anything else — `-i` on every route including `-c` and a
		// script operand, and a terminal with nothing to run. It is not a
		// `set` option and no letter of `set` turns it on, which is why it
		// is read from the fact the front end carried in rather than from a
		// field the option table writes.
		b.WriteByte('i')
	}
	if r.LoginShell && r.sem().LoginShowsLInDollarDash == Yes {
		// A login shell, in the two shells that say so. No majority to
		// follow — see Semantics.LoginShowsLInDollarDash — and read from
		// the fact the front end carried in for the reason `i` above is:
		// login-ness is an invocation fact and no `set` letter writes it.
		b.WriteByte('l')
	}
	if r.Route == RouteCommandString && r.sem().CommandStringShowsCInDollarDash == Yes {
		// A command string, in the two shells that say so. No majority to
		// follow — see Semantics.CommandStringShowsCInDollarDash.
		b.WriteByte('c')
	}
	if r.showsS() {
		b.WriteByte('s')
	}
	if r.allexport {
		b.WriteByte('a')
	}
	if r.errexit {
		b.WriteByte('e')
	}
	if r.noexec {
		b.WriteByte('n')
	}
	if r.onecmd {
		// The state and not its effect: measured, `bash -c 'set -t; echo $-'`
		// shows `t` on the one route the option never stops, so the letter
		// reports what was asked for rather than whether it will be acted on.
		// Unanimous in the two shells that have the option, so no axis.
		b.WriteByte('t')
	}
	if r.restricted {
		// `r` for a restricted shell, in the one column that has the mode
		// built here. Measured: `set -r; echo $-` grows the letter and no
		// `set +r` ever takes it away, which is the same one-way shape
		// `noexec` has and for the same reason — the option cannot be
		// turned off, so the letter cannot be withdrawn.
		b.WriteByte('r')
	}
	if r.keywordAssignments {
		// `k` while `set -k` is on, in both shells that have the option and
		// in neither of the two that refuse the letter. zsh writes the same
		// letter for a different option entirely and reaches it through its
		// own table, so this is not the site that answers for zsh. Measured
		// 2026-09-16, `set -aefhkmuvxBC` from a script file: bash writes
		// `aefhkmuvxBC` and ksh93 `aefhkmuvxBC` — the letter sits behind `h`
		// in both, which is what DollarDashLetterOrder carries.
		b.WriteByte('k')
	}
	if r.monitor {
		b.WriteByte('m')
	}
	if r.verbose {
		b.WriteByte('v')
	}
	if r.errtrace {
		b.WriteByte('E')
	}
	if r.functrace {
		b.WriteByte('T')
	}
	if r.noglob {
		if r.ask(r.sem().NoglobLetterIsF, "which letter `$-` shows for noglob") {
			b.WriteByte('f')
		} else {
			b.WriteByte('F')
		}
	}
	if name := r.sem().SetFLetterOption; name != "" {
		// The `f` letter in a shell that does not spend it on globbing. It
		// reports the option's state and not that the letter was written,
		// which is the measurement: see Semantics.SetFLetterOption, where a
		// script that reaches the name the long way still gets the letter.
		//
		// Read through the namespace `[[ -o name ]]` asks in rather than
		// through the substrate's own `set -o` table, because the name
		// belongs to the dialect: the shell that has this letter keeps a
		// hundred and eighty names its `set -o` never lists, and this is one
		// of them. Measured: `[[ -o norcs ]]` and a bare `setopt` agree
		// there, and the substrate's table has never heard of the name.
		if on, _ := r.conditionOption(name); on {
			b.WriteByte('f')
		}
	}
	if r.nounset {
		b.WriteByte('u')
	}
	if r.tracing() {
		b.WriteByte('x')
	}
	if r.noclobber {
		b.WriteByte('C')
	}
	b.WriteString(r.dialectOptionLetters())
	return orderedOptionLetters(b.String(), r.sem().DollarDashLetterOrder)
}

// orderedOptionLetters puts the letters into the order the dialect publishes
// them in, which is a different order in every shell measured — see
// Semantics.DollarDashLetterOrder.
//
// A letter the order does not name keeps its produced place and follows the
// ones it does, rather than being dropped: the string is a claim about the
// letters a shell was measured writing, and a letter nobody could measure —
// `n`, whose option stops the `echo` that would read `$-` — must still come
// out somewhere.
func orderedOptionLetters(letters, order string) string {
	if order == "" {
		return letters
	}
	var b strings.Builder
	b.Grow(len(letters))
	for i := 0; i < len(order); i++ {
		if strings.IndexByte(letters, order[i]) >= 0 {
			b.WriteByte(order[i])
		}
	}
	for i := 0; i < len(letters); i++ {
		if strings.IndexByte(order, letters[i]) < 0 {
			b.WriteByte(letters[i])
		}
	}
	return b.String()
}

// startupOptionLetters is the letters `$-` begins with: the options this shell
// turned on for itself before the script had a chance to.
//
// Two strings and not one plus an addition, because one shell in the panel
// turns an option *off* when it is interactive — its `$-` goes from `hB` to
// `imBE`, dropping the command-tracking letter — so what an interactive shell
// starts with is a different set rather than a longer one. A dialect that has
// nothing separate to say leaves the second empty and gets the first for both,
// which is one of the four.
//
// The letters that describe the invocation rather than an option follow this
// and are not in either string: `i` for being interactive at all, `c` and `s`
// for the route, and `m` for a monitor that is really running.
func (r *Runner) startupOptionLetters() string {
	return withdrawnStartupLetters(r, r.declaredStartupLetters())
}

// declaredStartupLetters is startupOptionLetters before anything the script
// did is taken back out of it: the set this shell would have shown on its
// first line.
//
// Separate because it is also read from the other side. A startup letter is
// a claim that an option is *on* before a script has run, so it answers the
// question a state behind that letter would otherwise have to be told
// separately — see Runner.commandTracking, which reads it rather than take a
// second declaration of the same fact from the dialect. Reading the withdrawn
// string there would ask the state about itself.
func (r *Runner) declaredStartupLetters() string {
	if r.Interactive {
		if interactive := r.sem().InteractiveOptionLetters; interactive != "" {
			return interactive
		}
	}
	return r.sem().DefaultOptionLetters
}

// startsWithOptionLetter reports whether this shell turned the option behind
// a letter on for itself, before the script.
func (r *Runner) startsWithOptionLetter(letter byte) bool {
	return strings.IndexByte(r.declaredStartupLetters(), letter) >= 0
}

// startupLetterIsOn holds, per startup letter, whether the option behind it is
// still on. A letter is *only* here where this runner really holds the state:
// a letter we merely record would answer the same either way, and writing a
// predicate for it would be a claim we do not keep.
//
// It exists because a startup letter is the one kind `$-` cannot derive from
// a field — the option is on before any script runs, so the string says so
// rather than a state doing it — and a script that turns such an option off
// has to take the letter back out. Measured on bash 5.3.15: `set +B; echo $-`
// answers `hc`, the `B` gone (#1856).
//
// One table rather than a check beside each letter, so that a second letter
// earning a state is an entry here and not a second mechanism.
var startupLetterIsOn = map[byte]func(*Runner) bool{
	'B': func(r *Runner) bool { return !r.noBraceExpand },
	'h': func(r *Runner) bool {
		// Only where the letter means command tracking. One shell in the
		// panel spells a history option with `h`, and although it has no
		// `h` among its startup letters today, a letter withdrawn on the
		// strength of a state it does not name would be the wrong answer
		// the moment one did — see Semantics.SetHLetterTracksCommands,
		// which is the same question `set -h` asks from the writing side.
		if r.sem().SetHLetterTracksCommands != Yes {
			return true
		}
		return r.commandTracking()
	},
}

// withdrawnStartupLetters drops the startup letters whose option this script
// has since turned off.
func withdrawnStartupLetters(r *Runner, letters string) string {
	var b strings.Builder
	for i := 0; i < len(letters); i++ {
		if on, ok := startupLetterIsOn[letters[i]]; ok && !on(r) {
			continue
		}
		if !r.dialectLetterIsOn(letters[i]) {
			continue
		}
		b.WriteByte(letters[i])
	}
	return b.String()
}

// dialectLetterIsOn answers the same question startupLetterIsOn does, for a
// letter this shell spells its own way: is the option behind it still on.
//
// The table is the dialect's rather than this package's — see
// Runner.SetOptionLetterNames — so this is the entry that keeps one letter
// from needing two declarations. A shell whose `X` is a startup letter and
// whose `set +X` turns the option off has said both things once: `X` is in
// DefaultOptionLetters, and the map says which option `X` names.
//
// A letter the dialect has no name for is on as far as this is concerned,
// which is the same default the static table keeps: the string is a claim
// about what was measured, and a letter nothing here can answer for must not
// be withdrawn on the strength of a state nobody holds.
func (r *Runner) dialectLetterIsOn(letter byte) bool {
	name, ok := r.optionLetterNames[rune(letter)]
	if !ok || name == "" {
		return true
	}
	on, known := r.conditionOption(name)
	return !known || on
}

// dialectOptionLetters are the `$-` letters for the options this shell spells
// its own way, in byte order so that a map's iteration cannot move them.
//
// The other half of Runner.SetOptionLetterNames, and it is one half rather
// than two tables for the reason the paired letter tables next door are one:
// a letter `set` takes and `$-` never shows is a shell that cannot tell a
// script what it was asked for. Measured 2026-09-13 on zsh 5.9.2 —
// `set -T; echo $-` is `569TX` where the same shell with nothing set is
// `569X` — and every one of the thirty letters behaves that way.
//
// The startup letters are left to startupOptionLetters, which already carries
// them and already knows how to take one back: a letter written in both
// places would come out twice.
func (r *Runner) dialectOptionLetters() string {
	if len(r.optionLetterNames) == 0 {
		return ""
	}
	startup := r.declaredStartupLetters()
	letters := make([]byte, 0, len(r.optionLetterNames))
	for letter, name := range r.optionLetterNames {
		if name == "" || strings.IndexByte(startup, byte(letter)) >= 0 {
			continue
		}
		if on, _ := r.conditionOption(name); on {
			letters = append(letters, byte(letter))
		}
	}
	slices.Sort(letters)
	return string(letters)
}

// showsS reports whether `$-` carries the `s` of the standard-input route.
//
// Two of the three ways it gets there are unanimous and so need no axis,
// measured 2026-09-05 across bash 5.3, dash, ksh93 and zsh:
//
//   - The program arriving on standard input — `sh -s`, `sh` with no
//     operands reading a pipe or a file, and a shell at a prompt, which is
//     the same route with a person on the other end.
//   - `-s` written even where something else supplied the program: all four
//     run the command string for `sh -s -c cmd` and all four still show `s`.
//
// bash 3.2 is the dissent on the first, and it is a shell disagreeing with
// its own later build rather than a panel split: it shows `s` only where
// `-s` was written. Recorded in docs/spec/invocation.md, not modeled.
//
// The third way is the split: ksh93 alone shows `s` under `-c` as well.
//
// And the fourth is a script asking for it. One shell in the panel gives the
// route an option name — `stdin` — and lets a script move it, so the letter
// follows the option rather than the route once it has been written. Measured
// on dash: `set -o stdin; echo $-` under `-c` is `s`, and `set +o stdin` on
// the standard-input route leaves `$-` empty. The other three have no such
// name, so nothing there can reach this branch.
func (r *Runner) showsS() bool {
	if r.stdinOptionMoved {
		return r.stdinOption
	}
	if r.Route == RouteStandardInput || r.StandardInputOption {
		return true
	}
	return r.Route == RouteCommandString && r.sem().CommandStringShowsSInDollarDash == Yes
}

// ForgetLastArgument puts `$_` back to the value it has before any command
// has run, which is what a front end calls after sourcing text the script's
// author did not write.
//
// A dialect's prelude is shell, so it moves `$_` exactly as a script would:
// bash's ends in an assignment, which leaves the parameter empty everywhere
// that tracks it, and a script whose first line read `$_` got that instead of
// the startup value. It is the same reasoning that already holds the
// invocation's `set` options back until the prelude has run — tracing the
// dialect's own plumbing under `-x`, or stopping on it under `-e`, reports on
// machinery nobody wrote.
//
// Exported rather than done inside Run because only the caller knows which
// text was the script's: the interpreter is handed a parsed file either way
// and the two are indistinguishable to it.
// Both records, because a dialect whose `$_` moves only between the commands
// the shell reads has a prelude that is read exactly that way: ksh93's ends
// on `type=whence -v`, and without this that string is what the script's
// first `$_` answered with.
func (r *Runner) ForgetLastArgument() {
	r.lastArg, r.lastArgSet = "", false
	r.inputLastArg, r.inputLastArgSet = "", false
}

// underscoreAtStartup is `$_` before anything has moved it, and it is still
// the answer afterwards in the shells that never move it at all.
//
// Two questions in order, because that is the order the panel answers them
// in: an `_` the environment carried wins, and the invocation is written only
// where the environment said nothing. bash is the only member that writes
// one, and it writes argv[0] — so it is the invocation and not `$0`, which a
// `-c` shell takes from its first operand.
//
// A shell that *discards* the environment's `_` still reaches the second
// question, which the panel cannot decide either way — the one member that
// discards is also the one that writes nothing — so the two axes compose
// rather than the first swallowing the second.
//
// Neither is asked where the answer cannot matter. An axis a dialect has not
// answered reports itself, and a shell that was handed no `_` and has no
// invocation to write has nothing to disagree about.
func (r *Runner) underscoreAtStartup() string {
	if v, ok := r.inheritedValue("_"); ok {
		if r.ask(r.sem().UnderscoreInheritsFromTheEnvironment, "`$_` taking the value the environment brought") {
			return v
		}
		if r.sem().UnderscoreInheritsFromTheEnvironment != No {
			// Refused for want of an answer rather than answered no, and
			// the refusal has already been reported. Asking the second
			// question would report the same thing twice about one read.
			return ""
		}
	}
	if r.Invocation != "" &&
		r.ask(r.sem().UnderscoreStartsAtTheInvocation, "`$_` starting at the invocation") {
		return r.Invocation
	}
	return ""
}

// setVarQuietly assigns without going through the readonly check or the
// OPTIND bookkeeping, because none of that applies to the shell setting up
// its own parameters before a script has run.
func (r *Runner) setVarQuietly(name, value string) {
	if r.Vars == nil {
		r.Vars = map[string]string{}
	}
	r.Vars[name] = value
}

// SetDynamicArray registers an *array* whose elements are produced when they
// are read.
//
// The same seam as SetDynamic and for a stronger reason: a call stack is not
// a value a shell can store and keep correct. It changes with every function
// call and every sourced file, and an array set once would be right until the
// first `source` — which is the worst shape a bug can have, because the
// script that reads it is usually the one working out where it lives.
func (r *Runner) SetDynamicArray(name string, value func(*Runner) []string) {
	if r.DynamicArrays == nil {
		r.DynamicArrays = map[string]func(*Runner) []string{}
	}
	// **Cloned here, so that a read cannot be a write.** A producer answers
	// with the elements the name holds *now*, and the shortest way to write
	// one is to hand back the storage those elements already live in —
	// `argv` returns `r.Params`, `$words` returns the completion's own word
	// list. Expansion then walks the slice it was given and several of its
	// steps write into it as they go, because every other array reaching
	// them is a copy made at the read: `${(U)a}` upper-cases each element in
	// place and `${(@)a%%p}` trims each one.
	//
	// So `${(@)argv%%:*}` — a *read* of the positional parameters, with a
	// modifier — replaced them. Measured 2026-09-16 with `f() { :
	// ${(@)argv%%:*}; print -r -- "$argv" }; f a:1 b:2`: zsh 5.9.2 prints
	// `a:1 b:2` and this shell printed `a b`. `${argv%%:*}` without the flag
	// and `${@%%:*}` in the other spelling were both unaffected, and a
	// stored array is unaffected in every spelling, which is what says the
	// fault is the seam and not the operator.
	//
	// It is the seam rather than each of the half-dozen read sites, and
	// rather than each producer, because the rule is a property of the
	// contract: what a producer answers with belongs to the caller. A reader
	// that forgot to copy would be a bug nothing local to it could show, and
	// a producer that forgot would be a bug in a file that has never heard
	// of expansion.
	r.DynamicArrays[name] = func(rr *Runner) []string { return slices.Clone(value(rr)) }
}

// SetDynamic registers a parameter whose value is produced when it is read.
//
// This is the seam a dialect uses for the parameters it has and the others do
// not: `RANDOM` is a different number every time, `SECONDS` counts, and
// neither can be a stored string. A dialect calls this from Apply, the same
// place it registers a builtin.
func (r *Runner) SetDynamic(name string, value func(*Runner) string) {
	if r.Dynamic == nil {
		r.Dynamic = map[string]func(*Runner) string{}
	}
	r.Dynamic[name] = value
}

// SetDynamicPresence says when a produced parameter is *there* at all.
//
// A producer returns a string, so it can answer "empty" and cannot answer
// "unset" — and the two are a difference a script can see: `${p-word}` takes
// its default for one and not for the other, `${p+word}` is the mirror, and
// `set -u` stops on the first. The predicate is registered beside the
// producer and asked ahead of it; a name with no entry is present from the
// moment it is registered, which is what every other produced parameter is.
//
// It exists for a parameter that is *written* by something the shell does
// rather than computed from what the shell is. One dialect's call-depth
// parameter is the worked case: it is unset until the first function call has
// been made and reads `0` from then on, so the depth cannot answer it and no
// producer could — see Runner.HasEnteredAFunction and #3310.
func (r *Runner) SetDynamicPresence(name string, present func(*Runner) bool) {
	if r.dynamicPresence == nil {
		r.dynamicPresence = map[string]func(*Runner) bool{}
	}
	r.dynamicPresence[name] = present
}

// dynamicParameterIsThere asks the predicate a dialect registered beside a
// producer, and answers yes where there is none.
func (r *Runner) dynamicParameterIsThere(name string) bool {
	present, ok := r.dynamicPresence[name]
	return !ok || present(r)
}

// HasEnteredAFunction reports whether this shell has been inside a function
// call at some point — which is not the same question as being inside one
// now, and is not answerable from the call depth, since that is 0 both before
// the first call and after the last one returns.
func (r *Runner) HasEnteredAFunction() bool { return r.enteredAFunction }

// SetDynamicWriter says what happens when a script assigns to a produced
// scalar parameter.
//
// The write half of SetDynamic, and required rather than optional for any
// produced scalar a script is allowed to assign to. The failure it prevents is
// the one SetDynamicAssocWriter documents for a table: the producer answers
// ahead of the stored value, so an assignment with nowhere to go is accepted
// in silence and then read back as whatever the producer says. Nothing about
// the name changes and no diagnostic is written — the script simply finds that
// setting the parameter did nothing.
//
// A produced scalar a script must *not* assign to is marked readonly instead,
// which refuses with a sentence. And a producer that only wants to *know* what
// was last assigned needs neither: Assigned already holds it, which is how
// SECONDS counts from a value it was given.
func (r *Runner) SetDynamicWriter(name string, write func(r *Runner, value string)) {
	if r.dynamicWriters == nil {
		r.dynamicWriters = map[string]func(*Runner, string){}
	}
	r.dynamicWriters[name] = write
}

// SetAssignmentAction says what else happens when a script assigns to an
// **ordinary** variable — one the shell stores rather than produces.
//
// The sibling of SetDynamicWriter, and the difference is which half of the
// assignment the dialect owns. A produced parameter has no stored value, so
// its writer *is* the assignment; a name registered here keeps every ordinary
// property it had — `unset` takes it away, `declare -p` lists it, `${v-word}`
// asks whether it is there — and the action is a message that the value moved.
//
// It exists because one of them changes something outside the shell. Measured
// 2026-09-18 on bash 5.3.20 from a script file with no terminal: assigning
// HISTFILESIZE **truncates the history file where it stands**, so
// `HISTFILESIZE=1` over a three-line file leaves one line on disk before the
// next command runs, and the shell's ending is not the only moment the file
// is written. Nothing a producer could express: the value is an ordinary
// variable that reads back, and a lazy reading — truncating only when the
// file is next written — answers a three-line file to a script that looks
// (#3423).
//
// Registered per name rather than as one callback over every assignment,
// because a hook on every store is a cost every script pays for a question
// about two names.
func (r *Runner) SetAssignmentAction(name string, act func(r *Runner, value string)) {
	if r.assignmentActions == nil {
		r.assignmentActions = map[string]func(*Runner, string){}
	}
	r.assignmentActions[name] = act
}

// SetUnsetAction says what else happens when `unset` takes an **ordinary**
// variable away.
//
// The other half of SetAssignmentAction, and it is a seam of its own rather
// than the same callback told twice, because most names that want the first
// do not want the second: an action that truncates a file where the
// assignment stands has nothing to do when the name goes, and a dialect
// should not have to distinguish the two calls to say so.
//
// It exists for the names whose state **outside** the variable table outlives
// the variable. Measured 2026-09-21 on bash 5.3.20 from a script file with no
// terminal: the size the history list is capped at is not re-read from
// HISTSIZE on every entry — a value that is not a count leaves the last size
// that *was* one in force — so the cap is state beside the list, and `unset
// HISTSIZE` is what lifts it. Without hearing the removal, a dialect keeping
// that state cannot tell `HISTSIZE=2; unset HISTSIZE; HISTSIZE=abc`, which
// lifts the cap, from `HISTSIZE=2; HISTSIZE=abc`, which does not (#4054).
//
// The action runs after the name is gone, so it reads the variable back as
// unset; it runs for a name nothing had set, because `unset` of a name that
// was never there still says the state should be lifted; and it does not run
// where the removal was refused, or where the panel's one shell takes an
// enclosing local away instead of the name — nothing went away there.
func (r *Runner) SetUnsetAction(name string, act func(r *Runner)) {
	if r.unsetActions == nil {
		r.unsetActions = map[string]func(*Runner){}
	}
	r.unsetActions[name] = act
}

// RefuseUnset marks a name `unset` refuses without its being readonly.
//
// A third state beside readonly and writable, and measured as one rather
// than folded into either: bash 5.3.20 and 3.2.57 answer `unset BASH_SOURCE`
// with `unset: BASH_SOURCE: cannot unset` at 1 — the readonly sentence
// without its reason — for BASH_SOURCE, BASH_LINENO, BASH_ARGV and BASH_ARGC,
// while `declare -p` shows none of the four frozen and FUNCNAME, produced by
// the same stack, is unset in silence. Under `set -o posix` bash 5.3 ends the
// script there exactly as it does for a readonly name, so the refusal takes
// the readonly one's route and asks the same axis.
func (r *Runner) RefuseUnset(name string) {
	if r.unsetRefused == nil {
		r.unsetRefused = map[string]bool{}
	}
	r.unsetRefused[name] = true
}

// SetDynamicArrayWriter says what happens when a script assigns to a produced
// array — the whole of it, one element of it, an append to it, or an `unset`
// of it, all four arriving here as the elements the name is to hold.
//
// The write half of SetDynamicArray, and required rather than optional for
// any produced array a script is allowed to assign to, for the reason
// SetDynamicWriter gives for a scalar: the producer answers ahead of the
// stored array, so a write with nowhere to go is accepted in silence and then
// read back as whatever the producer says. An `unset` delivers no elements,
// which is the same message the shell being modeled sends — measured on
// zsh 5.9.2, `set -- a b c; unset argv` leaves `$#` at 0, exactly as
// `argv=()` does — so the two need no way to tell each other apart.
//
// A produced array a script must *not* assign to is marked readonly instead,
// which refuses with a sentence.
func (r *Runner) SetDynamicArrayWriter(name string, write func(r *Runner, values []string)) {
	if r.dynamicArrayWriters == nil {
		r.dynamicArrayWriters = map[string]func(*Runner, []string){}
	}
	r.dynamicArrayWriters[name] = write
}

// UnsetDynamic takes a produced parameter away again, writer and all.
//
// The counterpart of SetDynamic for a parameter that exists only while
// something is happening. A dialect whose parameters are those of a *call* —
// the line a line editor is holding while it runs an action of the shell's
// own, say — has to be able to end them, because a produced parameter answers
// every read and a script that finds one outside the call would be told a
// value where the shell being modeled says nothing at all.
//
// It leaves no trace: not the removal `unset` records, which would keep a
// later SetDynamic from answering, not the message Assigned holds, which
// belongs to a call that is over, and **not the readonly mark**, which is the
// one that had to be reached for rather than reasoned about. A produced
// parameter a script must not assign to is marked readonly rather than given a
// writer, and there is no other way to lift that mark; leaving it would make
// the name refuse an ordinary assignment for the rest of the session, long
// after the thing it belonged to had finished. `$WIDGET` in dialect/zsh is the
// case: read-only while a widget runs — measured, real zsh answers
// `read-only variable: WIDGET` — and an ordinary variable outside one.
//
// **It takes away a produced array as well, which it did not.** SetDynamic and
// SetDynamicArray are two tables and this reached only the first, so a caller
// ending an array's life called this, nothing happened, and the parameter
// outlived the call it belonged to. The keyed tables go the same way and for
// the same reason rather than because a caller has needed it: the rule is
// which name is being taken away, and a third shape nobody had registered yet
// is exactly how this one arrived. Driving zsh 5.9.2 and this shell through a
// pseudo-terminal, running a widget and asking at the next prompt:
// `${+region_highlight}` is 0 in zsh and was 1 here, where the scalars beside
// it were 0 in both — so the one caller that registered an array was the one
// caller whose close did nothing, and the tests could not see it because they
// asked the question of a shell that had never run a widget. The name is what
// this takes away, not the shape it was registered under.
func (r *Runner) UnsetDynamic(name string) {
	delete(r.Dynamic, name)
	delete(r.dynamicWriters, name)
	delete(r.DynamicArrays, name)
	delete(r.dynamicArrayWriters, name)
	delete(r.DynamicAssocs, name)
	delete(r.dynamicAssocElements, name)
	delete(r.dynamicAssocWriters, name)
	delete(r.dynamicAssocEmptied, name)
	delete(r.assigned, name)
	delete(r.readonly, name)
	delete(r.localMarked, name)
}

// SetSpecial gives a parameter a fixed value unless a script has already set
// one, which is how a dialect supplies something like UID.
func (r *Runner) SetSpecial(name, value string) {
	if _, ok := r.Vars[name]; ok || r.removed[name] {
		return
	}
	r.setVarQuietly(name, value)
}

// Assigned reports what a script last assigned to a produced parameter, so
// its producer can take that as a starting point — which is what `SECONDS`
// does with it.
func (r *Runner) Assigned(name string) (string, bool) {
	v, ok := r.assigned[name]
	return v, ok
}

// randomModulus is the range `RANDOM` draws in: 0 to 32767, which every shell
// in the panel that has the parameter uses.
const randomModulus = 32768

// Randoms is the source `RANDOM` draws from.
//
// Seeded or not, and that is the whole of what this holds. An unseeded shell
// draws from the process generator and is a different number every run, which
// is what the parameter is for. A script that has assigned to `RANDOM` gets a
// sequence that is a function of the seed alone — measured 2026-09-14,
// `RANDOM=42; echo "$RANDOM $RANDOM"` twice in one shell and then the whole
// script again:
//
//	bash 5.3.15   17772 26794    ksh93u+   22700 13681
//	zsh 5.9.2     17766 11151
//
// every pair repeated, in that run and in the next. So all three seed, and no
// two of them draw the same numbers — the reproducibility is the shared fact
// and the sequence is each shell's own, which is why this has a generator of
// its own rather than a table to match.
//
// A method rather than the package function it was, because the seed is the
// shell's: `interp.Randoms()` took none and kept no state a writer could
// reach, so the assignment was heard, stored for the producer to find, and
// never acted on. A seeded script was four unrelated numbers where bash gives
// two repeated ones — and a seeded generator is also the only way a *test* can
// say anything about a script that uses `RANDOM` (#2827).
func (r *Runner) Randoms() string {
	if !r.randomSeeded {
		return strconv.Itoa(rand.IntN(randomModulus))
	}
	// A fresh generator per draw, keyed on the seed and the count, so that the
	// whole of the state is two integers a subshell can carry away by value.
	r.randomDrawn++
	if r.seededRandoms != nil {
		return strconv.Itoa(r.seededRandoms(r.randomSeed, r.randomDrawn))
	}
	return strconv.Itoa(rand.New(rand.NewPCG(r.randomSeed, r.randomDrawn)).IntN(randomModulus))
}

// SetSeededRandoms says what sequence this shell's `RANDOM` answers once a
// script has seeded it.
//
// The seed and the number of draws since, and the number to print: a **pure
// function** rather than a running generator, which is what keeps the seam
// honest about subshells. The whole of a seeded generator's state stays the two
// integers this Runner holds, so `( )` carries on from where its parent had got
// to and its own draws leave the parent where it was (#2827) — a dialect that
// kept a state of its own would have to reproduce that by hand, and a dialect
// that kept a *shared* one would break it silently.
//
// It exists because reproducible is not the same as right. Before it, a seeded
// script here got the same numbers every run and they were nobody's: three
// shells in the panel have the parameter, all three seed, and **no two of them
// draw the same sequence** — so a generator chosen by the substrate is
// guaranteed to be wrong for all three. `docs/spec/random.md` records what each
// one answers and how it was measured; MinimalStandardState holds the
// recurrence they share, and the dialect holds the two answers that differ.
//
// A dialect that registers nothing keeps the substrate's own sequence, which is
// reproducible and unmeasured. That is the honest state for a shell nobody has
// put an oracle to, and it is not a placeholder for one that has.
func (r *Runner) SetSeededRandoms(draw func(seed, drawn uint64) int) {
	r.seededRandoms = draw
}

// SeedRandoms is what an assignment to `RANDOM` does.
//
// The text rather than a number, because the value is an **arithmetic
// expression** and not a numeral. Measured 2026-09-22 under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` on bash 5.3.20, zsh 5.9.2 and ksh93u+:
// `RANDOM=3+4` seeds 7 in all three, and `abc=5; RANDOM=abc` seeds **5**, not
// 0. So "a value that is not a number is a zero seed", which is how #2827
// recorded it, is the consequence of arithmetic rather than a rule of its own —
// `abc` is a bare name and a bare name is its value, which is zero only while
// nothing has set it. The two readings disagree about the whole sequence the
// moment the name has a value (#4240).
//
// An expression that will not evaluate is complained about and **changes
// nothing**: measured on bash 5.3.20, `RANDOM=nope; RANDOM=1/0; echo $RANDOM`
// writes the division complaint and then the *second* number of the zero-seed
// sequence, so the seed and the draw count both survived the failure. Note that
// this is gentler than an ordinary integer-attributed name, where `declare -i v;
// v=1/0` ends the script — measured on both, which is why the failure is not
// routed through ArithValue's fatal path.
//
// One row short, knowingly: zsh and ksh93 *end* the script over the same failing
// seed, so what is here is bash's answer given to all three. That is a
// disagreement between real shells over identical syntax and therefore wants a
// semantics axis rather than a conditional, which is not a thing to add in
// passing — `docs/spec/random.md` records the measurement and what the axis
// would owe. Before this, every shell here seeded zero in silence, so the row
// was wrong in both halves rather than one.
//
// The count starts again as well, which is the half that makes the sequence
// reproducible rather than merely derived: measured, a second `RANDOM=42`
// after two draws gives the same first number as the first `RANDOM=42` did.
func (r *Runner) SeedRandoms(value string) {
	n, ok := r.seedExpression(value)
	if !ok {
		return
	}
	r.randomSeed, r.randomDrawn, r.randomSeeded = uint64(n), 0, true
}

// seedExpression evaluates a seed, and answers false where the expression
// failed and the seed a script wrote must be left alone.
//
// ArithValue is the exported sibling and is not what this wants: it makes a
// failure fatal, which is right for a builtin holding an expression and wrong
// here — the shell being modeled carries on, and a produced parameter that
// ended the script over a bad seed would be a difference in the direction
// nobody could work around.
func (r *Runner) seedExpression(value string) (int, bool) {
	tree, text, err := r.arithTreeOver(nil, value, arithTextArrived)
	if err != nil {
		r.diagf("%s\n", r.diag().ParseFailure(err))
		return 0, false
	}
	n, err := r.evalNum(tree)
	if err != nil {
		r.diagf("%s\n", r.arithFailure(text, err))
		return 0, false
	}
	return n.asInt(), true
}

// Now is what this shell calls the current time: the Clock hook where one is
// set, and the wall clock otherwise. Every place in the engine that needs the
// time of day goes through it, so an embedder that must not be at the mercy
// of the clock has one place to say so.
func (r *Runner) Now() time.Time {
	if r.Clock != nil {
		return r.Clock()
	}
	return time.Now()
}

// StartedAt is when this runner began, which is what `SECONDS` counts from
// and what `printf '%(fmt)T' -2` writes. Before the first Run it is the
// current time, so the value is never the zero one.
func (r *Runner) StartedAt() time.Time {
	if r.started.IsZero() {
		return r.Now()
	}
	return r.started
}

// Uptime is what `SECONDS` counts, from when the runner was made.
func (r *Runner) Uptime() time.Duration {
	if r.started.IsZero() {
		return 0
	}
	return time.Since(r.started)
}

// FloatPlacesOf is how many decimal places a name's float attribute writes it
// in, and whether the name has one at all — `typeset -F 3 SECONDS=0`.
//
// Exported because a *produced* parameter cannot see the attribute tables:
// its value is counted on each read by a function the dialect registered, so
// the attribute that decides how that value is written back is the one thing
// the producer has to come here for. Every stored name meets the attribute in
// attributeFolded instead and needs nothing from this.
func (r *Runner) FloatPlacesOf(name string) (int, bool) {
	prec, ok := r.floatPrecision[name]
	if !ok {
		return 0, false
	}
	return floatPlaces(prec), true
}

// SecondsFrom is `SECONDS`: the time since the runner started, counted from
// whatever a script last assigned to it.
func (r *Runner) SecondsFrom() float64 {
	base := 0.0
	if v, ok := r.Assigned("SECONDS"); ok {
		if n, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			base = n
		}
	}
	return base + r.Uptime().Seconds()
}

// SetParameterTypeWord installs the wording `${(t)name}` answers with: what a
// name *is*, said the way this shell says it.
//
// The seam ParameterAttributes already exists for, reached from the other
// side. A dialect publishes the same facts as a table of every name — see the
// `$parameters` view in dialect/zsh — and this is the one-name spelling of
// it, so the two cannot come to different words about one parameter.
//
// Nil is the runner nobody told, and there the flag is refused by name. That
// is deliberate and is the sharpest case of the rule `(p)` and `(g)` already
// follow: `[[ ${(t)x} == *array* ]]` is what a function writes to check what
// it was handed, and a word invented here would answer it at status 0 in a
// shell that has no such vocabulary.
//
// The function is called only for a name the runner *has*; an unset one is
// the empty string and never reaches it, which is measured — `${(t)nosuch}`
// is empty and `${(t)nosuch-D}` is `D`, so the expansion is *unset* rather
// than an empty value.
func (r *Runner) SetParameterTypeWord(word func(ParameterAttributes) string) {
	r.parameterTypeWord = word
}
