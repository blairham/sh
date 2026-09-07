// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// builtins are commands the shell runs itself.
//
// They exist because they must: `set` and `shift` change the shell's own
// state, and a child process cannot. That is also why the gate does not see
// them — nothing leaves this process — while it does see every external
// command and every file opened for a redirection.
var builtins = map[string]Builtin{
	":":        biTrue,
	"true":     biTrue,
	"false":    biFalse,
	"set":      biSet,
	"export":   biExport,
	"echo":     biEcho,
	"cd":       biCd,
	"pwd":      biPwd,
	"read":     biRead,
	"wait":     biWait,
	"exit":     biExit,
	"trap":     biTrap,
	"local":    biLocal,
	"typeset":  biDeclare,
	"readonly": biReadonly,
	"break":    biBreak,
	"continue": biContinue,
	"return":   biReturn,
	// `eval` and `.` are added in source.go's init rather than here — they
	// run arbitrary shell, so they reach the dispatcher that reads this map,
	// and Go calls a literal that closes that loop an initialization cycle.
	// `unset` joined them, for the reason its init below gives.
}

// `unset a[i+1]` evaluates its subscript, and evaluating an expression
// substitutes into it first — so a command substitution written in one reaches
// the dispatcher that reads the map above, exactly as `eval` does. That is a
// real capability rather than an accident of layering: the subscript is an
// arithmetic expression, and every way of writing one is open to it.
func init() {
	builtins["unset"] = biUnset
}

// biBreak and biContinue transfer control out of a loop. They are recorded on
// the runner rather than returned as errors, because leaving a loop is
// ordinary control flow and modeling it as a failure would make every caller
// check for something that is not one.
func biBreak(r *Runner, _ context.Context, args []string) int {
	r.ctl, r.ctlDepth = controlBreak, loopDepth(args)
	return 0
}

func biContinue(r *Runner, _ context.Context, args []string) int {
	r.ctl, r.ctlDepth = controlContinue, loopDepth(args)
	return 0
}

func biReturn(r *Runner, _ context.Context, args []string) int {
	if r.inFunc == "" && r.sourceDepth == 0 {
		// Nothing to return from. Three of the panel end the script here
		// with the status given; bash refuses and carries on, which is a
		// difference in *where the script stops* rather than in wording.
		if r.ask(r.sem().ReturnOutsideAFunctionIsRefused, "a `return` with nothing to return from") {
			r.diagf("%s\n", Wording(r.diag().ReturnOutsideAFunction,
				"return: can only `return' from a function or sourced script"))
			// Reported and not obeyed: no control flow is set, so the next
			// statement runs.
			return 2
		}
		if r.unspecified {
			return 2
		}
	}
	// What `$?` was as `return` began, which is what the RETURN trap's
	// action sees — the argument below is for the caller, not the trap.
	r.returnSeenStatus = r.status
	r.ctl = controlReturn
	if len(args) > 0 {
		if n, ok := atoi(args[0]); ok {
			return n
		}
	}
	return r.status
}

func loopDepth(args []string) int {
	if len(args) > 0 {
		if n, ok := atoi(args[0]); ok && n > 0 {
			return n
		}
	}
	return 1
}

// specialBuiltins are the ones POSIX marks special. Two consequences follow
// from the same list — an assignment prefixed to one persists, and a failure
// in one is fatal to a non-interactive shell — so it is one concept rather
// than two lists that could drift.
var specialBuiltins = map[string]bool{
	"break": true, ":": true, "continue": true, ".": true, "eval": true,
	"exec": true, "exit": true, "export": true, "readonly": true,
	"return": true, "set": true, "shift": true, "times": true,
	"trap": true, "unset": true,
}

func biTrue(*Runner, context.Context, []string) int  { return 0 }
func biFalse(*Runner, context.Context, []string) int { return 1 }

// biSet implements the part of `set` this slice needs: replacing the
// positional parameters.
//
// `set --` with nothing after it clears them, which is different from `set`
// with no arguments at all — that lists variables, in the dialect's shape.
// See setlisting.go.
func biSet(r *Runner, _ context.Context, args []string) int {
	if len(args) == 0 {
		return r.setListing()
	}
	// Options come before `--`, and each is a letter that may be turned on
	// with `-` or off with `+`. Only the ones with implemented behavior are
	// accepted; the rest are refused rather than silently ignored, which
	// would let a script believe it had asked for something.
	i := 0
	// Set by the array letter, which turns the operands from positional
	// parameters into an array's elements. See setarray.go.
	arrayName := ""
	arrayFront, haveArray := false, false
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if len(a) < 2 || (a[0] != '-' && a[0] != '+') {
			break
		}
		on := a[0] == '-'
		// The long spelling, whose name is the next word. Every option this
		// shell has can be written either way, and every shell in the panel
		// uses the same names for them — which is what makes the long form
		// the one that needs no dialect.
		//
		// `o` is read where it sits rather than only as a word of its own,
		// because it is nearly always the last letter of a bundle:
		// `set -euo pipefail` is the line at the top of a great many scripts,
		// and it was refused outright. The letters before it are ordinary
		// letters and still apply.
		if letters, ok := strings.CutSuffix(a[1:], "o"); ok {
			if !r.setLetters(letters, on) {
				return r.setOptionFailure()
			}
			if i+1 >= len(args) {
				// With no name to set, `-o` lists the options and `+o`
				// writes them back as input — four shapes, each the
				// dialect's own.
				return r.listOptions(!on)
			}
			i++
			// The namespace the *script* means by a `set -o` name, which is
			// the dialect's own where it has one (#1080).
			if !r.setNamedOption(args[i], on) {
				return r.setOptionFailure()
			}
			continue
		}
		// The array letter, which takes the *next word* as a name — the
		// same shape `-o` has, and the only other one in this builtin. The
		// letters in front of it still apply: `set -eA nn 1 2` is errexit as
		// well as an assignment, measured in both shells that have it.
		if letters, ok := strings.CutSuffix(a[1:], "A"); ok && r.setArrayLetter() {
			if !r.setLetters(letters, on) {
				return r.setOptionFailure()
			}
			if i+1 >= len(args) {
				return r.setArrayWithoutAName(on)
			}
			i++
			arrayName, arrayFront, haveArray = args[i], !on, true
			cont := r.ask(r.sem().SetArrayOptionsContinuePastTheName,
				"the words after `set -A name` read as options rather than as values")
			if r.unspecified {
				return r.status
			}
			if cont {
				// Options carry on past the name, so the values are whatever
				// the option parse does not claim — which is exactly the
				// words that would have become the positional parameters,
				// and a `--` among them still ends the options.
				continue
			}
			// Or the name ends the options and every word behind it is a
			// value, `--` and dash words included.
			i++
			break
		}
		if !r.setLetters(a[1:], on) {
			// 2 unless the refusal recorded a status of its own, which a
			// denied `set -m` does in one dialect.
			return r.setOptionFailure()
		}
	}
	if haveArray {
		// The positional parameters are left alone: measured, `set -- one two
		// three; set -A a x y` keeps all three of them in both shells. So
		// this returns rather than falling into the replacement below.
		return r.setArrayOperands(arrayName, arrayFront, args[i:])
	}
	// `set -C` alone sets an option and leaves the parameters alone; only an
	// explicit `--`, or operands after the options, replaces them.
	if i == 0 || (i <= len(args) && args[min(i-1, len(args)-1)] == "--") || i < len(args) {
		r.Params = append([]string(nil), args[i:]...)
	}
	return 0
}

// setLetterNames maps the letters every shell in the panel spells the same
// way to the long names they abbreviate, so both spellings of an option run
// through the one table and cannot drift apart. The letters missing from it
// are the ones some shell reads differently — f, h, E and T — which stay in
// the switch below, each behind the axis that says whose reading applies.
var setLetterNames = map[rune]string{
	'C': "noclobber",
	// The letter for allexport, which POSIX gives and all four have.
	'a': "allexport",
	'e': "errexit",
	'm': "monitor",
	// One-way: all four shells ignore `set +n` once it is on — and with it
	// on, the `set +n` never runs anyway. The table entry says so.
	'n': "noexec",
	'u': "nounset",
	// POSIX, all four: write input back as it is read.
	'v': "verbose",
	'x': "xtrace",
}

// setLetters applies the short spelling, reporting whether every letter was
// one this shell has.
//
// Split out of the loop above so a bundle ending in `o` can apply the letters
// before it and then read the name that follows — `set -euo pipefail` is
// errexit and nounset as well as pipefail, and dropping either half of that
// would be worse than refusing the whole line.
func (r *Runner) setLetters(letters string, on bool) bool {
	for _, opt := range letters {
		if name, ok := setLetterNames[opt]; ok {
			o := commonSetOptions[name]
			if o.try != nil {
				sign := "-"
				if !on {
					sign = "+"
				}
				if !o.try(r, on, sign+string(opt)) {
					return false
				}
				continue
			}
			o.apply(r, on)
			continue
		}
		switch opt {
		case 'h':
			// A letter three shells have and no two mean identically: bash
			// and ksh93 abbreviate command tracking with it (hashall,
			// trackall), zsh a history option (histignoredups), and dash
			// refuses it outright.
			if !r.ask(r.sem().SetHasTheHLetter, "`set -h` being an option letter at all") {
				if r.unspecified {
					return false
				}
				return r.badSetOptionLetter(opt, on)
			}
			if r.ask(r.sem().SetHLetterTracksCommands, "which option `set -h` abbreviates") {
				// The same state the hashall and trackall table entries
				// write, so the letter and the names cannot disagree.
				r.tracksCommands = on
			} else if r.unspecified {
				return false
			} else {
				r.histIgnoreDups = on
			}
		case 'E', 'T':
			// bash's trap-carriage letters. zsh spells different options
			// with the same letters and dash and ksh93 have neither, so a
			// wrong guess here would quietly mean something else.
			if !r.ask(r.sem().SetHasTraceLetters, "`set -E` and `set -T` carrying traps into functions") {
				if r.unspecified {
					return false
				}
				return r.badSetOptionLetter(opt, on)
			}
			if opt == 'E' {
				r.errtrace = on
			} else {
				r.functrace = on
			}
		case 'f':
			// Not universal: one shell spells this option the long way only
			// and uses `-f` for something else, which does not touch
			// globbing. Asked rather than assumed, and only here — `set -o
			// noglob` needs no dialect.
			if r.ask(r.sem().SetFTurnsOffGlobbing, "`set -f` turning off pathname expansion") {
				r.noglob = on
			}
		default:
			return r.badSetOptionLetter(opt, on)
		}
	}
	return true
}

// setOption applies `set -o name`, reporting whether the name is one we have.
//
// The names are the same in every shell in the panel, which is what makes the
// long form the one that needs no dialect: `set -o noglob` means the same
// thing in all four where `set -f` does not.
// badSetOptionName reports a long option name this shell does not have.
//
// The same shape as a bad option letter, and for the same reason: one of the
// panel follows it with a usage line, one puts the builtin's name in the
// location rather than in the sentence, and one answers 1 where the rest
// answer 2.
// badSetOptionLetter reports an option letter this shell does not have.
//
// The letter and the name are one question with one answer, which is why this
// reads the same dialect status badSetOptionName reads and asks the same
// dialect whether a refusal ends the script. Measured 2026-09-05 on `-q`,
// `-j`, `-z` and `-A` — the letters all six of bash 5.3, bash 3.2,
// bash-as-`sh`, dash, ksh93 and zsh refuse: `set -q` reports exactly what
// `set -o nosuchoption` reports in each of them, and dies or does not in the
// same way. The letter asked neither question before: it reported 2 for
// everybody and never ended a script, so `zsh -q` exited 2 where zsh exits 1
// and `set -q` in a dash script carried on where dash stops (#483).
//
// The words are the dialect's, in Diagnostics.SetInvalidOptionLetter, because
// the panel spells this four ways — `-q: invalid option`, `Illegal option -q`,
// `-q: unknown option`, `bad option: -q` (#598). The sign rides in the wording
// rather than being one: bash and ksh93 echo back the `+` of `set +q` and dash
// and zsh write `-q` whichever was asked, so the format takes the spelling as
// written and the bare letter and each dialect uses the one it says.
//
// A letter the dialect really has and this shell has not implemented is said
// to be missing instead, the way every other builtin's is — see
// Diagnostics.UnimplementedOptionLetters. Telling a script that `set -b` is
// invalid in a bash that has it would be a worse answer than telling it the
// truth.
func (r *Runner) badSetOptionLetter(opt rune, on bool) bool {
	d := r.diag()
	sign := "+"
	if on {
		sign = "-"
	}
	spelled, bare := sign+string(opt), string(opt)
	msg := Wording(d.SetInvalidOptionLetter, "set: %[1]s: invalid option", spelled, bare)
	usage := true
	if has := d.UnimplementedOptionLetters["set"]; strings.ContainsRune(has, opt) {
		msg, usage = "set: "+spelled+" is not implemented yet", false
	}
	r.saySetRefusal(msg, usage, false)
	return r.setRefusalStatus("a refused `set` option letter ending the script")
}

func (r *Runner) badSetOptionName(name string) bool {
	d := r.diag()
	msg := Wording(d.SetInvalidOptionName, "set: %[1]s: invalid option name", name)
	r.saySetRefusal(msg, d.SetInvalidOptionNameUsage, true)
	return r.setRefusalStatus("an unknown `set -o` name ending the script")
}

// saySetRefusal writes one refused `set` option, and takes the whole of the
// difference between the two routes it can arrive by.
//
// Inside a script it is the builtin speaking: the dialect's location, `set`
// named where the dialect names it, and the builtin's own usage line under it
// where the dialect prints one.
//
// At an invocation nothing has been read, and the panel says so — measured
// 2026-09-05 on `-q` and on `-o zzznosuch`. Three of the four drop the
// builtin's name and its location alike and print their own name and the
// sentence, which is `invocationPrefix` and the same wording with the leading
// `set: ` taken off. The usage block is the *shell's* there rather than
// `set`'s, so it comes from a field of its own. bash is the exception on the
// long spelling only, and reports it exactly as the builtin would with its own
// name standing where `set` would — location and all, `bash: line 0: bash:
// zzznosuch: invalid option name`.
func (r *Runner) saySetRefusal(msg string, usage, isName bool) {
	d := r.diag()
	if r.fromEnvironment {
		// A third shape, and the plainest of them: an option name that came
		// out of the environment rather than out of an argument vector or a
		// script. The location is the shell's own, at line 0 like the
		// invocation's, and nothing stands where `set` would — measured,
		// `SHELLOPTS=nosuchoption` draws `<shell>: line 0: nosuchoption:
		// invalid option name` from the one shell that reads the variable,
		// against `<shell>: line 0: <shell>: …` for the same bad name written
		// as `-o`. No usage block either: nobody was given a usage to get
		// wrong.
		r.diagf("%s\n", strings.TrimPrefix(msg, "set: "))
		return
	}
	if !r.atInvocation {
		r.diagf("%s\n", msg)
		if usage {
			r.sayBuiltinUsage(d.BuiltinUsage["set"])
		}
		return
	}
	if rest, ok := strings.CutPrefix(msg, "set: "); ok && isName && d.InvocationNameRefusalNamesTheShell {
		r.diagf("%s: %s\n", r.name(), rest)
		return
	}
	r.errf("%s%s\n", d.invocationPrefix(r.name()), strings.TrimPrefix(msg, "set: "))
	if u := d.InvocationUsage; u != "" {
		r.errf("%s\n", Wording(u, u, r.name(), filepath.Base(r.name())))
	}
}

// sayBuiltinUsage writes a usage line the way the dialect writes one: after
// its own location, or on a line of its own where the dialect prints no
// prefix in front of it.
func (r *Runner) sayBuiltinUsage(usage string) {
	if usage == "" {
		return
	}
	if r.diag().BuiltinUsageUnprefixed {
		r.errf("%s\n", usage)
	} else {
		r.diagf("%s\n", usage)
	}
}

// setRefusalStatus records what a refused `set` option reports and ends the
// script where the dialect says such a refusal is fatal. One place for both
// spellings, because the panel answers them identically (#483).
func (r *Runner) setRefusalStatus(why string) bool {
	status := orDefault(r.diag().SetInvalidOptionStatus, 2)
	r.setOptionStatus = status
	if r.ask(r.sem().BadSetOptionNameFatal, why) {
		r.status = status
		r.fatalQuiet()
	}
	return false
}

func (r *Runner) setOption(name string, on bool) bool {
	if name == "pipefail" {
		// The one name with an axis of its own, because whether the shell
		// has it was settled before this table existed and the answer is
		// the same question in a different shape.
		if r.ask(r.sem().PipefailOption, "`set -o pipefail`") {
			r.pipefail = on
			return true
		}
		if r.unspecified {
			// The refusal is already reported. A second complaint about the
			// same word would only obscure it.
			return true
		}
		return r.badSetOptionName(name)
	}
	o, ok := r.lookupSetOption(name)
	if !ok {
		return r.badSetOptionName(name)
	}
	if o.try != nil {
		// A request the dialect may refuse, handed the spelling it was asked
		// with — the refusal echoes it back.
		return o.try(r, on, name)
	}
	if o.apply != nil {
		o.apply(r, on)
		return true
	}
	if on == o.on {
		// Already where it is being asked to be, so the request has been
		// granted. This is what makes `set +o posix` work in a shell with no
		// posix mode, rather than stopping a script over a state it already
		// had.
		return true
	}
	// A name this shell has and does not do. Refused out loud rather than
	// accepted quietly, because accepting would be promising to behave
	// differently afterwards.
	r.diagf("set: %s: not implemented\n", name)
	return false
}

// unsetFunction removes one function, and reports what the dialect reports.
//
// Two questions, and the panel answers them independently — which is what
// makes them two fields. ksh93 judges the *name*: `1x` could never be a
// function name, and it says so whether or not a function exists. zsh
// reports the *table*: it complains about any name it does not hold,
// including a perfectly well formed one. bash and dash say nothing about
// either, and unsetting a function that is there is quiet in all four.
func (r *Runner) unsetFunction(name string) int {
	d := r.diag()
	if !isPlainName(name) && r.ask(r.sem().UnsetFunctionChecksTheName, "`unset -f` judging the name it was given") {
		r.diagf("%s\n", Wording(d.UnsetBadFunctionName, "unset: %[1]s: invalid function name", name))
		return 1
	}
	fn := r.funcs[name]
	if r.speaksForTheShell(fn) {
		// The shell's own, so the script has no function of that name to
		// remove and this is the "not defined" case rather than a removal
		// (#1082). Every shell in the panel has `dirs`, `popd` and `pushd`
		// as builtins, and in every one of them the name still works after
		// `unset -f`: measured, bash is silent at 0 and zsh writes the same
		// `no such hash table element` it writes for any name it does not
		// hold, at 1. That is the field below and not a new one — a name
		// this shell provides is exactly a name the script never defined.
		fn = nil
	}
	if fn == nil {
		if r.ask(r.sem().UnsetFunctionReportsMissing, "`unset -f` naming a function that is not defined") {
			r.diagf("%s\n", Wording(d.UnsetFunctionNotFound, "unset: %[1]s: not found", name))
			return 1
		}
		return 0
	}
	r.removeFunction(name)
	return 0
}

// removeFunction takes a function out, and gives the name back to the
// prelude's where there was one.
//
// One mechanism for what is two spellings: `unset -f pushd` and — in the
// dialect that has it — `unset "functions[pushd]"` are the same operation
// asked for two ways, and a shell where they left the name in two different
// states would be two shells. A redefinition took the voice away and removing
// it gives the name back: the shells with the builtin uncover it when the
// function shadowing it goes, so `pushd() { echo mine; }; unset -f pushd;
// pushd /tmp` pushes in both of them. Reconstructing the namespace boundary
// by hand, because a dialect written as shell has one table where they have
// two.
func (r *Runner) removeFunction(name string) {
	delete(r.funcs, name)
	delete(r.funcFiles, name)
	delete(r.exportedFuncs, name)
	if prelude := r.preludeFuncs[name]; prelude != nil {
		r.funcs[name] = prelude
	}
}

// badSubscriptToUnset reports an `unset` operand whose subscript would not
// evaluate, and answers with the status the builtin carries.
//
// Two answers, and they are not a wording difference. bash ends the script
// where a bad expression always ends it, so nothing after the `unset` runs;
// ksh93 and zsh leave a failed builtin behind and go on to the next command,
// which is the shape a script can test. ksh93 also names the builtin in front
// of the sentence, where it words the identical failure in an expansion
// without one.
func (r *Runner) badSubscriptToUnset(sub string, err error) int {
	sentence := r.subscriptFailure(sub, err)
	// The complaint is the shell's rather than the builtin's — it is the same
	// sentence the same shell writes about the same text inside `$(( ))` — so
	// the location must not name `unset`. The one dialect that does name it
	// puts it in the message, where it puts every other builtin's name.
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()
	if r.ask(r.sem().BadSubscriptToUnsetFatal, "a bad subscript ending the script") {
		r.fatal("%s\n", sentence)
		return r.status
	}
	if r.unspecified {
		return 2
	}
	r.diagf("%s\n", Wording(r.diag().UnsetBadSubscript, "%[1]s", sentence))
	return 1
}

// unsetReadonly refuses to remove a readonly name, reporting 0 when there was
// nothing to refuse.
//
// Every shell in the panel refuses, says so, and leaves the value standing —
// `readonly x=1; unset x; echo "${x-gone}"` prints 1 in all six — so the
// refusal is the core answer and only what follows it splits. dash and zsh end
// the script; bash and ksh93 report and carry on, and go on to the *rest* of
// the operands: `readonly x=1; y=2; unset x y` leaves x standing and removes
// y, which is why this reports per name rather than giving up the builtin.
//
// The status is 1 wherever it can be seen, which is the three that carry on;
// the shell that stops carries FatalErrorStatusIsOne's, as every fatal error
// does, so there is no status of this error's own.
func (r *Runner) unsetReadonly(name string) int {
	if !r.readonly[name] {
		return 0
	}
	// The builtin has been named in the sentence by three of the four
	// dialects, so it must not be named in the *location* by the one that
	// puts every other builtin's name there: zsh writes `zsh:1: read-only
	// variable: x` here and `zsh:unset:1: 1x: invalid parameter name` for a
	// bad name, from the same builtin. It is the same care setVarAs takes for
	// the assignment this refusal is the twin of.
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()
	msg := Wording(r.diag().UnsetReadonly, "unset: %s: cannot unset: readonly variable", name)
	if r.ask(r.sem().UnsetReadonlyFatal, "unsetting a readonly name ending the script") {
		// fatal carries FatalErrorStatusIsOne's number, which is the whole of
		// the status question here — dash exits 2 and zsh 1, and neither is
		// this error's own.
		r.fatal("%s\n", msg)
		return r.status
	}
	if r.unspecified {
		return 2
	}
	r.diagf("%s\n", msg)
	return 1
}

// unsetMatching is `unset -m`: each operand is a pattern, and every parameter
// whose name it matches goes.
//
// The names are collected before anything is removed, because the tables are
// what is being walked; and they are sorted so that a diagnostic from one
// removal — a readonly name — arrives in the same order every run.
func (r *Runner) unsetMatching(patterns []string) int {
	if len(patterns) == 0 {
		// Measured: zsh refuses the letter with nothing to match rather
		// than treating it as `unset` with no operands, which is silent.
		r.diagf("%s\n", Wording(r.diag().UnsetPatternUsage, "%[1]s: not enough arguments", "unset"))
		return 1
	}
	status := 0
	for _, pattern := range patterns {
		// Collected before anything is removed, because what is being
		// walked is the tables themselves; and sorted, so that a refusal
		// from one removal arrives in the same order every run.
		o := r.patternOpts(pattern)
		for _, name := range r.parameterNames() {
			if !matchPattern(pattern, name, o) {
				continue
			}
			if code := r.unsetReadonly(name); code != 0 {
				status = code
				if r.ctl == controlExit {
					return status
				}
				continue
			}
			r.unsetName(name)
		}
	}
	return status
}

// parameterNames is every parameter this shell can see, once each and in
// order. The same three sources [Runner.namesWithPrefix] reads.
//
// The array tables are deliberately not among them, and that is a fact about
// this shell rather than an omission: an array's name is in Vars as well —
// `a=(p q)` writes both — so walking Arrays and AssocArrays here added
// nothing at all. It was written, and a mutant that deleted it survived every
// test including the one about arrays, which is how the duplication was
// found. Should the two tables ever come apart, this is one of the places
// that has to be told.
func (r *Runner) parameterNames() []string {
	seen := map[string]bool{}
	var out []string
	// The `removed` half of the guard is the one no test can see, and it is
	// here because a name `unset` has already taken away is not a parameter
	// — not because anything would go wrong without it. Removing a name
	// twice is removing it once, so a mutant that drops this clause passes
	// everything, the same standing this codebase gives an escaped ordinary
	// character. It is the *rule* that is being written down.
	add := func(name string) {
		if seen[name] || r.removed[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for name := range r.Vars {
		add(name)
	}
	for name := range r.Dynamic {
		add(name)
	}
	for name := range r.inheritedEnv {
		add(name)
	}
	sort.Strings(out)
	return out
}

func biUnset(r *Runner, _ context.Context, args []string) int {
	letters := r.sem().UnsetOptions
	if letters == "" {
		letters = "vf"
	}
	args, opts, code := r.builtinOptions("unset", args, letters)
	if code != 0 {
		return code
	}
	if strings.ContainsRune(opts, 'm') {
		// `unset -m` reads its operands as patterns and unsets every
		// parameter whose *name* matches one. Ahead of `-f`, because the
		// two are the same question asked of two namespaces and only the
		// variable one is measured here; and ahead of the name check,
		// because a pattern is not a name and would not survive it.
		return r.unsetMatching(args)
	}
	if strings.ContainsRune(opts, 'f') {
		// `unset -f` is about functions and not about variables, unanimously
		// — and the option was read and then ignored, so a function survived
		// being unset and went on answering to its name. The exported set
		// goes with it: what is not a function cannot be carried as one.
		status := 0
		for _, name := range args {
			if code := r.unsetFunction(name); code != 0 {
				status = code
			}
		}
		return status
	}
	// After `-f`, so that a function name keeps its own laxer rule: bash
	// takes `unset -f 1x` without a word where it refuses `unset 1x`.
	args, status := r.builtinNames("unset", args, strings.ContainsRune(opts, 'v'))
	if r.ctl == controlExit {
		return status
	}
	for _, name := range args {
		base, sub, subscripted := r.subscriptOperand(name)
		if !subscripted {
			base = name
		}
		// Before the subscript is read, and named by the *base*: `unset a[0]`
		// against a readonly `a` is refused by the variable the subscript
		// indexes, and the two shells with arrays that refuse it say `a`
		// rather than `a[0]`. The element is never reached, so the subscript
		// is not evaluated either — which is why this stands ahead of the
		// whole subscripted branch rather than inside it.
		if code := r.unsetReadonly(base); code != 0 {
			status = code
			if r.ctl == controlExit {
				return status
			}
			continue
		}
		if subscripted {
			// `unset a[1]` is about one element and not about the array.
			// The subscript was read as part of the name, so the whole thing
			// was deleted from a table it was never in and nothing happened
			// at all.
			if r.assocDeclared(base) {
				// `[@]` is a key like any other where the attribute is on:
				// no shell measured clears a keyed array through it, so the
				// whole-array reading below is the indexed array's alone.
				r.unsetAssocElem(base, sub)
				continue
			}
			if sub == "@" || sub == "*" {
				// Every element rather than one, in two of the three shells
				// with arrays. The third reads the brackets as an expression
				// here as everywhere else, and falls through to it.
				if handled, code := r.unsetWholeArray(base); handled {
					if code != 0 {
						status = code
					}
					continue
				}
			}
			if handled, code := r.unsetSubscriptRange(base, sub); handled {
				// A subscript written as a pair, where the dialect reads the
				// comma as a range rather than as the arithmetic operator
				// whose value is its right operand. Ahead of the single
				// reading rather than inside it, because a range names a
				// span and a span is not a subscript.
				if code != 0 {
					status = code
				}
				if r.ctl == controlExit {
					return status
				}
				continue
			}
			if r.unspecified {
				return r.status
			}
			idx, err := r.subscriptValue(sub)
			if err != nil {
				// Reported by every shell in the panel, and silent here: the
				// error came back and nothing read it, so `unset a[b c]` was
				// a no-op at status 0. What follows differs — bash gives up
				// on the script and the other two leave a failed builtin
				// behind — but nobody says nothing.
				status = r.badSubscriptToUnset(sub, err)
				if r.ctl == controlExit {
					return status
				}
				continue
			}
			if code := r.unsetArrayElem(base, idx, sub); code != 0 {
				status = code
			}
			continue
		}
		r.unsetName(name)
	}
	return status
}

// unsetName removes one whole parameter, whatever kind it is.
//
// Extracted so that the pattern form and the name form remove alike: the two
// entered the builtin by different doors and would otherwise have been two
// copies of this, which is how one of them ends up forgetting a table.
func (r *Runner) unsetName(name string) {
	if t, tied := r.tieOf(name); tied {
		// Half a tie is not a state this shell has: `unset SCA` leaves `sca`
		// with no elements *and* unset, and `unset sca` leaves `$SCA`
		// unset. Measured, and the tie itself goes too — a later `SCA=a:b`
		// is a plain scalar and `${#sca}` stays 0.
		//
		// Untied first, so the two calls below do not mirror into each
		// other, and only the *other* name is recursed into.
		r.untie(name)
		other := t.scalar
		if name == t.scalar {
			other = t.array
		}
		r.unsetName(other)
	}
	delete(r.Vars, name)
	delete(r.exported, name)
	delete(r.Arrays, name)
	delete(r.AssocArrays, name)
	// Recorded as well as deleted: a name that came from the environment is
	// not in Vars to begin with, and deleting nothing left it visible to
	// every lookup — `unset PATH` did not clear PATH.
	if r.removed == nil {
		r.removed = map[string]bool{}
	}
	r.removed[name] = true
}

// biExport marks a name for the environment, and assigns when given a value.
func biExport(r *Runner, _ context.Context, args []string) int {
	// `-f` and `-n` are offered only where the dialect has them. Where it
	// does not, each goes through the ordinary unknown-option path and gets
	// that shell's own refusal, which in two of them ends the script.
	letters := "p"
	// Asked only where there is an `-n` to decide about, for the reason `-f`
	// is below: `export A=1` is the same in all four, and refusing it over a
	// question nothing turned on would be refusing to export anything.
	if hasOption(args, 'n') {
		takes := r.ask(r.sem().ExportTakesTheAttributeOff, "`export -n`")
		if r.unspecified {
			return 2
		}
		if takes {
			letters += "n"
		}
	}
	if hasOption(args, 'f') {
		carries := r.ask(r.sem().ExportCarriesFunctions, "`export -f`")
		if r.unspecified {
			return 2
		}
		switch {
		case carries:
			letters += "f"
		case r.diag().ExportFunctionOptionRefused != "":
			// A dialect that knows the letter and will not do it, which is
			// not the same as one that has never heard of it — and says so
			// in different words.
			r.diagf("%s\n", r.diag().ExportFunctionOptionRefused)
			return 1
		}
	}
	args, opts, code := r.builtinOptions("export", args, letters)
	if code != 0 {
		return code
	}
	if r.exported == nil {
		r.exported = map[string]bool{}
	}
	if strings.ContainsRune(opts, 'f') {
		return r.exportFuncs(args)
	}
	if strings.ContainsRune(opts, 'p') || (opts == "" && len(args) == 0) {
		// The listing: exported names alone, in the dialect's shape. An
		// operand narrows it to that name's declaration.
		//
		// `export` with nothing at all is the same listing. POSIX says so and
		// every shell in the panel does it — measured with one name exported
		// under a scrubbed environment, dash, bash 5.3, bash 3.2, ksh93 and
		// zsh all write the exported names out. This printed nothing, which
		// is why its failed write had nothing to fail: `export >&-` answered
		// 0 in silence where dash and bash report the write.
		//
		// The *shape* of the bare listing is not always `-p`'s: dash and both
		// bash builds write the same thing either way, and ksh93 and zsh drop
		// the leading `export` word for the bare form alone. That split is
		// recorded in the corpus and left for a dialect to answer; the
		// listing every shell has is worth more than the silence it replaces.
		return r.declarePrintForm(args, r.bareOrDashP(opts, r.sem().ExportListing),
			func(d declaration) bool { return d.exported })
	}
	args, status := r.builtinNames("export", args, false)
	if r.ctl == controlExit {
		return status
	}
	for _, a := range args {
		name, value, hasValue := strings.Cut(a, "=")
		if hasValue {
			r.setVarAs(name, value, assignedByDeclaration)
			if r.ctl == controlExit {
				// The assignment ended the script, so the builtin has
				// nothing left to report — and returning its own status
				// would put back the one the failure set.
				return r.status
			}
		}
		// Recorded either way rather than deleted for `-n`: a name that came
		// in through the environment is exported by having done so, and only
		// an explicit "no" can take that off. Deleting the record put the
		// question back to the environment, which answers yes.
		r.exported[name] = !strings.ContainsRune(opts, 'n')
	}
	if r.assignFailed && status == 0 {
		// A name it refused to assign is the builtin's failure, not just a
		// remark: the dialect that reports a readonly reassignment and
		// carries on leaves 1 in `$?` for `export x=2` as much as for `x=2`.
		return 1
	}
	return status
}

// Registered here rather than in the table above: `shift` reads its count as
// an expression in two dialects, which reaches the evaluator, which reaches
// the runner, which reaches the table — a cycle Go refuses to order. Two
// other builtins are registered this way for the same kind of reason.
func init() { builtins["shift"] = biShift }

// firstOptionLetter is the letter a bundle of single-letter options is
// refused by: the dashes are stripped and the first rune after them taken, so
// `--help` is `h`.
func firstOptionLetter(operand string) string {
	letters := strings.TrimLeft(operand, "-")
	if letters == "" {
		return operand
	}
	return string([]rune(letters)[0])
}

// biShift drops the first n positional parameters.
//
// Shifting past the end is where the panel splits: fatal in dash and ksh93,
// survivable in bash and zsh. The dialect answers it rather than this taking
// a side.
// subscriptOperand splits `a[1]` into the name and the subscript.
//
// Only for a name that really ends in one: `unset a` is the whole array and
// `unset a[1]` is an element of it, and the two arrive as the same kind of
// word. The subscript comes back as text because its reading is the base
// name's to decide: an expression for an indexed array, any string at all for
// a declared associative one — so `unset m[k]` is an element of `m` when `m`
// carries the attribute and an element numbered by whatever `k` evaluates to
// when it does not.
//
// The split is where it stops. It used to reject anything that was not a
// numeral, which made `unset a[i+1]` a bad *name* rather than a bad
// subscript — a whole different complaint about a subscript that is fine.
// Whether the text evaluates is the caller's question, asked where the
// element is reached and not here, which is also what keeps a builtin out of
// the arithmetic evaluator's reach until it means to use it.
func (r *Runner) subscriptOperand(operand string) (string, string, bool) {
	open := strings.IndexByte(operand, '[')
	if open <= 0 || !strings.HasSuffix(operand, "]") {
		return "", "", false
	}
	base := operand[:open]
	sub := strings.TrimSpace(operand[open+1 : len(operand)-1])
	return base, sub, true
}

// hasOption reports whether the letter appears in the option words before the
// first operand.
func hasOption(args []string, letter byte) bool {
	for _, a := range args {
		if a == "--" || !strings.HasPrefix(a, "-") || a == "-" {
			return false
		}
		if strings.IndexByte(a[1:], letter) >= 0 {
			return true
		}
	}
	return false
}

func biShift(r *Runner, _ context.Context, args []string) int {
	n := 1
	// What the complaint names differs from what the shift does: one dialect
	// reports the operand *as written*, and an operand that was never given
	// is reported as `(null)` rather than as the default it stood in for.
	operand := "(null)"
	// Whether the marker was taken, which decides more than where the count
	// starts: past it a dash word is an operand rather than an option, so
	// ksh93 reads `shift -- -1` as a count below zero where it refuses a
	// bare `-1` as an option it does not have.
	marked := false
	if len(args) > 0 && args[0] == "--" {
		// The end-of-options marker, and only the first one: what follows is
		// the count however it is spelled, so `shift -- -1` is a negative
		// count in the three that take the marker and `shift -- --`
		// complains about the second `--` as a count. Asked here rather than
		// alongside the option words because bash answers the two
		// differently — no dash word is an option there and the marker still
		// works.
		switch {
		case r.ask(r.sem().ShiftDoubleDashEndsOptions, "`shift --` read as the end of options"):
			args = args[1:]
			marked = true
		case r.unspecified:
			return r.status
		}
	}
	if len(args) > 0 {
		operand = args[0]
		if st, done := r.shiftCount(args[0], marked, &n); done {
			return st
		}
	}
	if n < 0 {
		// The other end of the range. Same fatality rule and the same
		// untouched `$#`; only the wording differs, and dash never arrives
		// here at all — a negative count is a word that is not a number
		// there, which shiftCount has already said.
		return r.shiftOutOfRange(r.diag().ShiftNegativeCount, n, operand)
	}
	if n > len(r.Params) {
		return r.shiftOutOfRange(r.diag().ShiftTooMany, n, operand)
	}
	r.Params = r.Params[n:]
	return 0
}

// shiftOutOfRange reports a count outside `0..$#`, in whichever direction.
//
// One rule for both ends: fatal in dash and ksh93, survivable in bash and
// zsh, `$#` untouched either way, and status 1 where it is survived. The
// wording is the caller's because the two directions are worded separately —
// bash is silent above `$#` and complains below it, and zsh has a sentence
// for each.
func (r *Runner) shiftOutOfRange(wording string, n int, operand string) int {
	if r.ask(r.sem().ShiftPastEndFatal, "shift past the end being fatal") {
		// controlReturn only unwound a function, so at the top level the
		// script carried on past an error the shell calls fatal.
		r.fatal("%s\n", Wording(wording, "shift: can't shift that many", n, operand))
		return r.status
	}
	// Survivable, and still worth saying where the dialect says it: zsh
	// prints its complaint and carries on, and bash prints nothing at all
	// for a count above `$#`. No fallback here for that reason — an empty
	// wording is a dialect that has nothing to say rather than one that has
	// not been asked.
	if wording != "" {
		r.diagf("%s\n", Wording(wording, wording, n, operand))
	}
	return 1
}

// shiftCount reads the operand, reporting whether the builtin is finished.
//
// A leading `-` splits the panel three ways, which is ShiftOptionWords: ksh93
// reads every dash word as an *option* and refuses it as one, zsh reads only
// the ones that are not all digits that way, and bash and dash read them all
// as the count and complain about the number. Same input, two different kinds
// of complaint, and which input gets which differs again.
//
// Both refusals end the script in the two dialects where a special builtin's
// failure is fatal, and `shift` is a special builtin, so that rule is the one
// already in place rather than a new one.
//
// A lone `-` is not a dash word in any of the three readings and reaches the
// count, which is bash's and dash's answer for it. The marker `--` never gets
// here: biShift has taken it off already, or the dialect does not have one
// and the word is the count. marked says which — past a marker that was taken
// there are no options left to read, so `shift -- -1` is a count in all three
// dialects that have one.
func (r *Runner) shiftCount(operand string, marked bool, n *int) (int, bool) {
	if !marked && len(operand) > 1 && operand[0] == '-' {
		p := r.shiftOptionWords()
		if r.unspecified {
			return r.status, true
		}
		if p == ShiftOptionWordsAny || (p == ShiftOptionWordsNonNumeric && !allDigits(operand[1:])) {
			// The *first letter*, not the whole word: a leading `-` word is
			// a bundle of single-letter options, so `shift --help` is
			// refused as `-h` — the dashes are stripped and the first
			// letter after them is the one named. printf's options already
			// follow the same rule, measured the same way.
			return r.badBuiltinOption("shift", "-"+firstOptionLetter(operand)), true
		}
	}
	if v, ok := atoiSigned(operand); ok {
		// A plain number, which every reading agrees on. Nothing is asked
		// for a count of zero or more: `shift 2` is two everywhere, and so
		// is `shift +2` — the sign is unanimous.
		return r.shiftTakeCount(v, operand, n)
	}
	if r.ask(r.sem().ShiftCountIsArithmetic, "`shift n` reading its count as an expression") {
		// An expression rather than a number: `shift 1+1` moves two and
		// `shift n` moves whatever n holds. An unset name is zero there, so
		// `shift abc` shifts nothing and succeeds, where the dialects that
		// want a number call it one they cannot read.
		tree, perr := r.arithTree(nil, operand)
		if perr != nil {
			r.diagf("shift: %v\n", perr)
			return 2, true
		}
		v, err := r.evalArith(tree)
		if err != nil {
			return r.status, true
		}
		return r.shiftTakeCount(v, operand, n)
	}
	if r.unspecified {
		return r.status, true
	}
	return r.shiftBadNumber(operand)
}

// shiftTakeCount accepts a count that was read, asking about a negative one.
//
// Below zero is where the reading of the word itself is still in question:
// bash, ksh93 and zsh have a count that is out of range, and dash has a word
// that is not a number — the same complaint it makes about `-x`. Above or at
// zero nothing is asked, which is why the ordinary `shift 2` needs no
// dialect.
func (r *Runner) shiftTakeCount(v int, operand string, n *int) (int, bool) {
	if v < 0 && !r.ask(r.sem().ShiftNegativeIsOutOfRange, "a negative `shift` count read as a count out of range") {
		if r.unspecified {
			return r.status, true
		}
		return r.shiftBadNumber(operand)
	}
	*n = v
	return 0, false
}

// shiftBadNumber is the complaint about an operand that is not a count, and
// the end of the script where a special builtin's bad operand ends one.
func (r *Runner) shiftBadNumber(operand string) (int, bool) {
	d := r.diag()
	r.diagf("%s\n", Wording(d.ShiftBadNumber, "shift: %[1]s: numeric argument required", operand))
	status := orDefault(d.BuiltinBadOptionStatus, 2)
	if r.ask(r.sem().BadOptionToSpecialBuiltinFatal, "a special builtin's bad operand ending the script") {
		r.status = status
		r.fatalQuiet()
		return r.status, true
	}
	return status, true
}

// biEcho writes its arguments separated by spaces.
//
// Whether backslash escapes expand without -e is the EchoInterpretsEscapes
// axis — dash and zsh expand them, bash and ksh93 do not — asked below, and
// only when a backslash appears, so `echo hi` needs no dialect. Which option
// letters exist, which of `-e -E` wins, and the \x and \e set extensions are
// each their own axis, read the same way.
func biEcho(r *Runner, _ context.Context, args []string) int {
	newline := true
	letters := r.sem().EchoOptions
	if letters == "" {
		letters = "n"
	}
	// -1 is -E, +1 is -e, 0 is the dialect's default. A word carrying any
	// letter outside the dialect's set is not an option at all — the whole
	// word becomes an operand, which is unanimous: `echo -nq hi` prints
	// `-nq hi` in all four shells.
	forced := 0
	for len(args) > 0 {
		a := args[0]
		if len(a) < 2 || a[0] != '-' || strings.ContainsFunc(a[1:], func(c rune) bool {
			return !strings.ContainsRune(letters, c)
		}) {
			break
		}
		for i := 1; i < len(a); i++ {
			switch a[i] {
			case 'n':
				newline = false
			case 'e':
				forced = 1
			case 'E':
				// `-E -e` expands everywhere; only `-e -E` splits the
				// panel, so the question waits for that order.
				if forced != 1 ||
					r.ask(r.sem().EchoLastEscapeFlagWins, "which of `echo -e -E` decides") {
					forced = -1
				}
			}
		}
		args = args[1:]
	}
	out := strings.Join(args, " ")
	// dash and zsh expand backslash escapes without -e; bash and ksh93 do
	// not. A grouping no other axis produces.
	// Asked only when the text could differ either way, so `echo hi` needs no
	// dialect and `echo 'a\tb'` does.
	expand := forced == 1
	if forced == 0 {
		expand = strings.ContainsRune(out, '\\') &&
			r.ask(r.sem().EchoInterpretsEscapes, "echo interpreting backslash escapes")
	}
	if expand && strings.ContainsRune(out, '\\') {
		// The two set extensions are asked only when their escapes appear.
		hex := strings.Contains(out, `\x`) &&
			r.ask(r.sem().EchoExpandsHexEscapes, "echo expanding \\xHH")
		// The two spellings of the escape character are two questions, and
		// each is asked only where its own letter appears: ksh93 has `\E`
		// and not `\e`, zsh has `\e` and not `\E`, so one answer for both
		// was wrong for half the panel (#908).
		esc := strings.Contains(out, `\e`) &&
			r.ask(r.sem().EchoExpandsEscEscape, "echo expanding \\e")
		capEsc := strings.Contains(out, `\E`) &&
			r.ask(r.sem().EchoExpandsCapitalEscEscape, "echo expanding \\E")
		var stopped bool
		out, stopped = expandEchoEscapes(out, hex, esc, capEsc)
		if stopped {
			// `\c` ends the output, newline included.
			newline = false
		}
	}
	if newline {
		out += "\n"
	}
	if out == "" {
		// `echo -n` has nothing to write, and nothing cannot fail to be
		// written: it succeeds even on a closed descriptor, everywhere
		// measured.
		return 0
	}
	// Recorded, not returned: whether a write that went nowhere fails the
	// command is the dispatcher's question, answered once for every builtin.
	if _, err := r.stdout().Write([]byte(out)); err != nil {
		r.writeFailed = err
	}
	return 0
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func hexValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}

// xsiEscape is the escape table POSIX's XSI `echo` defines with one letter,
// and it is the one part of the subject nothing in the panel argues about: an
// `echo` argument, a `printf` format and a `%b` argument all read these eight
// the same way, in all six shells measured.
//
// It is written once because the three sites that read escapes disagree about
// everything *else* — `\c`, `\e`, `\x` and the octal forms each split the
// panel differently, and each split falls in a different place per site — so
// the eight that never move are the thing to hold still, rather than the
// thing to copy three times.
//
// ok is false for a character no entry claims, which leaves what to do with
// the backslash to the caller: the sites do not agree about that either.
func xsiEscape(c byte) (byte, bool) {
	switch c {
	case 'a':
		return '\a', true
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	case 'n':
		return '\n', true
	case 'r':
		return '\r', true
	case 't':
		return '\t', true
	case 'v':
		return '\v', true
	case '\\':
		return '\\', true
	}
	return 0, false
}

// expandEchoEscapes interprets the escapes `echo` expands where the dialect
// says it does: the XSI set, with `\xHH`, `\e` and `\E` admitted per
// dialect — the last two separately, because ksh93 and zsh have one each and
// not the other.
// stopped reports a `\c`, which discards the rest of the output and the
// closing newline with it.
func expandEchoEscapes(s string, hex, esc, capEsc bool) (expanded string, stopped bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		if e, ok := xsiEscape(s[i]); ok {
			b.WriteByte(e)
			continue
		}
		switch s[i] {
		case 'c':
			return b.String(), true
		case 'e', 'E':
			admitted := esc
			if s[i] == 'E' {
				admitted = capEsc
			}
			if !admitted {
				b.WriteByte('\\')
				b.WriteByte(s[i])
				break
			}
			b.WriteByte(0x1b)
		case '0':
			// `\0` and up to three octal digits after it.
			n, j := 0, i+1
			for j < len(s) && j <= i+3 && s[j] >= '0' && s[j] <= '7' {
				n = n*8 + int(s[j]-'0')
				j++
			}
			b.WriteByte(byte(n))
			i = j - 1
		case 'x':
			if !hex {
				b.WriteByte('\\')
				b.WriteByte(s[i])
				break
			}
			n, j := 0, i+1
			for j < len(s) && j <= i+2 && isHexDigit(s[j]) {
				n = n*16 + hexValue(s[j])
				j++
			}
			if j == i+1 {
				// `\x` with no digits stays as written.
				b.WriteString(`\x`)
				break
			}
			b.WriteByte(byte(n))
			i = j - 1
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String(), false
}

// biCd changes the shell's working directory.
//
// This is the definition of a core primitive: it changes the runner's own
// state, every dialect needs it, and no shell function can say it. It lived
// in cmd/bash while that binary was demonstrating Register, which was the
// right place for a demonstration and the wrong one to leave it.
// cdOptions reads `cd`'s leading options.
//
// `-L` and `-P` are the two, and they are unanimous: the default and `-L`
// keep the name the directory was reached by, and `-P` resolves it. Measured
// through a symlink, where all four print the link's path for the first two
// and the real one for the third.
//
// A lone `-` is not an option — it is the previous directory — which the
// length test leaves alone.
func (r *Runner) cdOptions(args []string) (rest []string, physical bool, code int) {
	sawLogical, sawPhysical := false, false
	done := func(rest []string, code int) ([]string, bool, int) {
		// Which of the two decides is a question only when both were given,
		// and it is asked only then: with one of them the two rules agree,
		// and a shell that refused an unambiguous `cd -P` would be refusing
		// over a disagreement that is not in front of it.
		if sawLogical && sawPhysical && !r.ask(r.sem().CdLastPathOptionWins, "which of `cd -L` and `cd -P` decides") {
			return rest, sawPhysical, code
		}
		return rest, physical, code
	}
	for len(args) > 0 {
		a := args[0]
		if len(a) < 2 || a[0] != '-' {
			break
		}
		if a == "--" {
			return done(args[1:], 0)
		}
		for i := 1; i < len(a); i++ {
			switch a[i] {
			case 'L':
				physical, sawLogical = false, true
			case 'P':
				physical, sawPhysical = true, true
			default:
				// The one place the panel splits: three of them refuse a
				// letter `cd` does not have, and zsh reads the word as
				// somewhere to go instead — `cd -Q` looks for a directory
				// called `-Q` there.
				if !r.ask(r.sem().CdRefusesUnknownOption, "an option `cd` does not have") {
					return done(args, 0)
				}
				return nil, physical, r.badBuiltinOption("cd", "-"+string(a[i]))
			}
		}
		args = args[1:]
	}
	return done(args, 0)
}

func biCd(r *Runner, _ context.Context, args []string) int {
	args, physical, code := r.cdOptions(args)
	if code != 0 {
		return code
	}
	dir := ""
	if len(args) > 0 {
		dir = args[0]
	}
	dash := false
	switch dir {
	case "":
		dir, _ = r.getVar("HOME")
		if dir == "" {
			code, _ := r.cdNowhere(r.diag().CdHomeNotSet, "cd: HOME not set")
			// With no HOME there is nowhere to go even for the dialects that
			// do not call it an error, so this stops either way.
			return code
		}
	case "-":
		// The previous directory, which is why cd records one.
		dash = true
		dir, _ = r.getVar("OLDPWD")
		if dir == "" {
			if code, stop := r.cdNowhere(r.diag().CdOldpwdNotSet, "cd: OLDPWD not set"); stop {
				return code
			}
			// Not an error here, and not nothing either: the dialects that
			// survive this go to where they already are, which still prints
			// for the ones that print.
			dir = r.workDir()
		}
	}

	old := r.workDir()
	announced := false
	if !filepath.IsAbs(dir) && !dash {
		// CDPATH, searched for an operand that is not absolute and does not
		// lead with a dot — `cd ./x` names a place, not a search. The entry
		// that wins decides the announcement: a plain `.` moves quietly, and
		// any other winner is printed — in three of the four; zsh moves in
		// silence either way.
		if found, viaPath := r.searchCdpath(dir); viaPath != "" {
			if viaPath != "." &&
				r.ask(r.sem().CdpathAnnouncesTheDirectory, "`cd` printing where CDPATH sent it") {
				announced = true
			}
			if r.unspecified {
				return 2
			}
			dir = found
		}
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(old, dir)
	}
	if physical {
		// `-P` is where the directory *is*, rather than the name it was
		// reached by. Every shell in the panel resolves the whole path and
		// reports the resolved one from `pwd` afterwards, so this replaces
		// the name rather than only checking it.
		//
		// A path that cannot be resolved is left as written: what to say
		// about a directory that is not there is the question below, and it
		// answers with what the operating system said rather than with
		// anything this step could add. A symlink cycle arrives here that
		// way and reads as ELOOP from the stat, which is what the panel
		// says a cycle is — under `-P` and under `-L` alike, since the
		// chdir hits it either way.
		//
		// Through the gate, one component at a time: resolving is a walk
		// over the filesystem and a script chose the path, so a policy has
		// to see each step of it. See physicalpath.go.
		if resolved, err := r.physicalPath(dir); err == nil {
			dir = resolved
		}
	}
	// Through the gate. A denied stat surfaces as the missing-directory
	// error below, so `cd` into a path the policy hides fails the way `cd`
	// into a path that is not there does — same sentence, same status.
	info, err := r.stat(dir)
	if err == nil && !info.IsDir() {
		err = &os.PathError{Op: "cd", Path: dir, Err: syscall.ENOTDIR}
	}
	if err != nil {
		// The reason the operating system gave, rather than one made up
		// here: three of the four report it, and two of those distinguish a
		// path that is not there from one that is not a directory. Saying
		// "no such directory" for both was a sentence no shell prints and an
		// answer one of them can tell is wrong.
		r.diagf("%s\n", Wording(r.diag().CdCannotChange, "cd: %[1]s: %[2]s",
			args[0], r.diag().reasonText(reason(err))))
		return orDefault(r.diag().CdStatus, 1)
	}
	// Only the runner's own directory moves. Calling os.Chdir would move the
	// whole process, which is wrong for an embedded interpreter and would be
	// shared by every Runner in it.
	r.Dir = dir
	r.setVar("OLDPWD", old)
	r.setVar("PWD", dir)
	if announced {
		r.printf("%s\n", dir)
	}
	if dash && r.ask(r.sem().CdDashPrintsTheDirectory, "`cd -` printing where it went") {
		// Asked only for `cd -`, which is the only form any of them prints.
		r.printf("%s\n", dir)
	}
	return 0
}

// searchCdpath walks CDPATH for a relative operand that does not lead with
// a dot, returning the joined path of the first entry holding a directory of
// that name and the entry that held it. All four shells search; who prints
// afterwards is the axis at the call.
func (r *Runner) searchCdpath(operand string) (found, via string) {
	if strings.HasPrefix(operand, "./") || strings.HasPrefix(operand, "../") {
		return "", ""
	}
	cdpath, ok := r.getVar("CDPATH")
	if !ok || cdpath == "" {
		return "", ""
	}
	for _, entry := range strings.Split(cdpath, string(filepath.ListSeparator)) {
		if entry == "" {
			entry = "."
		}
		candidate := entry + string(filepath.Separator) + operand
		abs := candidate
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(r.workDir(), abs)
		}
		// Through the gate; an entry the policy hides is walked past the
		// way an entry with no such directory is.
		if st, err := r.stat(abs); err == nil && st.IsDir() {
			return abs, entry
		}
	}
	return "", ""
}

// cdNowhere is `cd` with nothing to go to: no HOME, or no OLDPWD.
//
// Two dialects call that an error and two stay where they are and report
// success — quietly, which is the surprising half: a script that relies on
// `cd` moving has already carried on by the time it notices.
func (r *Runner) cdNowhere(wording, fallback string) (int, bool) {
	if !r.ask(r.sem().CdWithoutHomeIsAnError, "`cd` with nowhere to go being an error") {
		return 0, false
	}
	r.diagf("%s\n", Wording(wording, fallback))
	return orDefault(r.diag().CdStatus, 1), true
}

func biPwd(r *Runner, _ context.Context, args []string) int {
	// `-L` and `-P` with the last one deciding, which is unanimous —
	// `pwd -L -P` is physical in all four shells. Operands are ignored by
	// three of the four, which is what leaving them unread does.
	_, opts, code := r.builtinOptions("pwd", args, "LP")
	if code != 0 {
		return code
	}
	physical := false
	for _, o := range opts {
		physical = o == 'P'
	}
	dir := r.workDir()
	if physical {
		// Where the directory *is*, not the name it was reached by. A path
		// that cannot be resolved is printed as held, the same answer `cd
		// -P` gives for one — and through the gate a component at a time,
		// because `pwd -P` is a question about the filesystem and a policy
		// that hides part of it must be able to refuse the answer.
		if resolved, err := r.physicalPath(dir); err == nil {
			dir = resolved
		}
	}
	// Through printf, which records a failed write for the dispatcher to fold
	// into the status. Writing the stream directly discarded the error, so
	// `pwd >&-` answered 0 in silence where dash says `pwd: pwd: I/O error`
	// and bash `pwd: write error: Bad file descriptor`, both with status 1.
	r.printf("%s\n", dir)
	return 0
}

// biRead reads a line into variables.
//
// Also a primitive by the same test: it has to set a variable in the *calling*
// shell, which a child process cannot reach.
//
// Without -r a backslash escapes the character after it, including a newline,
// which is why -r is what scripts should use and rarely do.
func biRead(r *Runner, ctx context.Context, args []string) int {
	// Every leading `-` word was skipped here, whatever it was — so
	// `read -s v` echoed what was meant to be hidden and `read -n 1 v` read
	// a whole line, both without a word said. All four refuse an option they
	// do not have, and the ones they *do* have and this shell does not are
	// named as missing rather than as unknown.
	//
	// Through the shared reader, because options bundle: `read -ra arr` is
	// `-r -a arr` in every shell, and reading whole words here refused the
	// bundle wholesale — `-ra is not implemented yet` — with the letter this
	// builtin does implement inside it (#347). The letters are the
	// dialect's: `n` takes a count in the two shells that count and is a
	// flag in the one where it decorates completion, so even the shape of
	// the optstring is an answer rather than a constant.
	letters := r.sem().ReadOptions
	if letters == "" {
		letters = "r"
	}
	args, opts, optArg, code := r.builtinOptionsArg("read", args, letters)
	if code != 0 {
		return code
	}
	raw := strings.Contains(opts, "r")
	// -s is parsed and deliberately does nothing more: silence is about a
	// terminal's echo, and this runner never echoes what it reads. Parsing
	// it is the point — measured, `printf x | read -s v` reads x and prints
	// nothing in every shell that has the letter, terminal or none, and
	// skipping the word instead once let a password echo (#321).

	// -i is parsed and does nothing, which is not the same as ignoring it.
	// It is the text a *line editor* opens with, so it has an effect only
	// where there is a terminal and an editor on it — and this runner's
	// `read` never opens one, because the letter that would (`-e`) is
	// refused by name as unimplemented. bash answers the same way wherever
	// its own input is not a terminal: `printf x | read -i pre -r l` sets l
	// to `x`, seed and all. The reading that would be wrong is treating the
	// seed as a *default* for an empty line — `printf '\n' | read -i pre l`
	// leaves l empty in bash, not `pre`, and in no shell in the panel does
	// it do otherwise (#761). The letter still consumes its argument, which
	// is the half that has to be right either way: without it `read -i pre
	// -r l` would read `pre` as the variable name.

	// -p rides the optstring's shape the way -n does: `p:` takes a prompt,
	// handled once the stream is known, and a bare `p` names the coprocess
	// as the source. With one running the letter reads from its near end;
	// with none it is the measured refusal — the dialect's words, status 1,
	// and the variables untouched, which is the part that separates this
	// from a read that reached its input and failed.
	coprocSource := -1
	if _, ok := optArg['p']; !ok && strings.Contains(opts, "p") {
		fd, running := r.CoprocRead()
		if !running {
			r.diagf("%s\n", Wording(r.diag().ReadNoCoprocess, "read: -p: no coprocess"))
			return 1
		}
		coprocSource = fd
	}

	// The stream: standard input, or the descriptor -u names — resolved
	// against the shell's own table, where `exec 5<file` put it.
	in := r.In()
	if coprocSource >= 0 {
		if rd, open := r.readerForFd(coprocSource); open {
			in = rd
		}
	}
	if word, ok := optArg['u']; ok {
		fd, numeric := atoi(word)
		if !numeric || fd < 0 {
			return r.readBadNumber(word)
		}
		rd, open := r.readerForFd(fd)
		if !open {
			// Three answers with one status: bash and ksh93 complain in
			// their own words and zsh says nothing, so an empty wording is
			// the dialect's answer rather than a gap to paper over.
			if w := r.diag().ReadBadFileDescriptor; w != "" {
				// Through Wording for the format handling alone — the
				// fallback is never reached, because an empty wording is an
				// answer here: one shell reports this failure in silence.
				r.diagf("%s\n", Wording(w, w, word))
			}
			return 1
		}
		in = rd
	}

	// The prompt: written to standard error, no newline, and only when the
	// stream being read is a terminal — both measured in both shells whose
	// -p takes an argument, and the second half from both sides: a pipe gets
	// no prompt, and neither does a -u descriptor on a file while the
	// terminal sits untouched on standard input. Raw rather than through the
	// diagnostic path, because a prompt carries no location in any shell.
	if prompt, ok := optArg['p']; ok && inputIsTerminal(in) {
		r.errf("%s", prompt)
	}

	// The delimiter: a newline unless -d renamed it. The argument's first
	// character speaks — `read -d xy` stops at the x in every shell with
	// the letter — and an empty argument means NUL in the two measured
	// saying so. (ksh93 reads through a NUL instead; seeing that difference
	// takes a NUL in the input, which is also what it takes to care.)
	delim := byte('\n')
	if word, ok := optArg['d']; ok {
		delim = 0
		if word != "" {
			delim = word[0]
		}
	}

	// The counts: -n reads at most N characters, still stopping at the
	// delimiter; -N reads exactly N, the delimiter ordinary, backslash
	// ordinary, and the text handed over whole rather than split.
	count, exact := -1, false
	if word, ok := optArg['n']; ok {
		n, numeric := atoi(word)
		if !numeric || n < 0 {
			return r.readBadNumber(word)
		}
		count = n
	}
	if word, ok := optArg['N']; ok {
		n, numeric := atoi(word)
		if !numeric || n < 0 {
			return r.readBadNumber(word)
		}
		count, exact = n, true
	}

	// The array: bash's -a names it in the option's argument and ignores
	// any operands after it; ksh93 and zsh spell it -A and take the name as
	// the first operand, clearing the names that follow. The letters
	// differ, so the behaviors can ride them without an axis.
	array := ""
	if name, ok := optArg['a']; ok {
		array = name
	}
	if strings.Contains(opts, "A") {
		array = "REPLY"
		if len(args) > 0 {
			array, args = args[0], args[1:]
		}
	}

	// The timeout, if the dialect has the letter: seconds, fractions
	// allowed. Zero and expiry land in the same place — nothing read in the
	// time given — and what that reports is the dialect's number.
	timeout, timed := time.Duration(-1), false
	if word, ok := optArg['t']; ok {
		secs, err := strconv.ParseFloat(word, 64)
		if err != nil || secs < 0 {
			return r.readBadNumber(word)
		}
		timeout, timed = time.Duration(secs*float64(time.Second)), true
	}

	next := directByteSource(in)
	switch {
	case timed && timeout == 0:
		// A timeout of zero is a question about the stream rather than a
		// deadline that has already passed, and the panel answers it three
		// ways — see ReadZeroTimeoutStyle.
		if src, reads := r.zeroTimeoutSource(in); reads {
			next = src
			break
		}
		if r.sem().ReadZeroTimeout != ReadZeroTimeoutPolls {
			r.diagf("%s\n", r.unanswered("what `read -t 0` asks of the stream"))
			r.status = 2
			r.unspecified = true
			return 2
		}
		// The status is the whole answer: nothing is read and no name is
		// touched, in either direction.
		if inputWaiting(in) {
			return 0
		}
		return 1
	case timed:
		var stop func()
		next, stop = r.timedByteSource(ctx, in, timeout)
		defer stop()
	}
	text, lits, end := readSegment(next, raw, delim, count, exact)

	// A `read` that fails still assigns. All four shells clear the variables
	// at end of input rather than leaving what was there, and the reason is
	// the loop everyone writes: `while read -r l` leaves `l` behind, and a
	// stale value after the loop reads as the last line rather than as
	// nothing. Assigning happens before the status is decided, not instead
	// of it.
	status := 0
	switch end {
	case endTimeout:
		status = orDefault(r.diag().ReadTimeoutStatus, 1)
		// A deadline is not an end of input, and two of the three shells with
		// the letter treat it as no read at all: no name is touched, not even
		// cleared, so whatever the variable held survives. The third assigns
		// the short read — which looks like clearing only because the usual
		// way to reach a timeout is with nothing having arrived.
		if !r.ask(r.sem().ReadTimeoutKeepsWhatArrived,
			"an expired `read -t` assigning what did arrive") {
			if r.unspecified {
				return 2
			}
			return status
		}
	case endEOF:
		status = 1
		switch {
		case exact:
			// Both shells with the letter report the short read; whether
			// the partial text survives into the variable splits them.
			if text != "" && !r.ask(r.sem().ReadExactCountKeepsPartial,
				"a short `read -N` keeping what did arrive") {
				text = ""
			}
		case count >= 0 && text != "":
			// A full count and a wholly empty input answer the same way
			// everywhere; only the partial fill asks.
			if r.ask(r.sem().ReadPartialCountSucceeds,
				"a short `read -n` counting as a success") {
				status = 0
			}
		}
	}

	if len(args) == 0 && array == "" {
		// Three shells set REPLY; dash wants a variable name and says so.
		if r.ask(r.sem().ReadRequiresAVariableName, "a bare `read` with no variable") {
			r.diagf("%s\n", Wording(r.diag().ReadArgCount, "read: arg count"))
			return 2
		}
		if r.unspecified {
			return 2
		}
		args = []string{"REPLY"}
	}
	// Only the -A spelling touches the operands after the array: it took its
	// name from among them and clears the rest, where bash's -a leaves the
	// names after its argument exactly as they were — both measured with
	// `x=keep`.
	clearRest := strings.Contains(opts, "A")
	if exact {
		// -N hands the text over whole: `read -N 5 x y` on `a b c` puts all
		// five characters in x and nothing in y, measured in both shells
		// with the letter.
		if array != "" {
			r.setArray(array, exactElems(text))
			if clearRest {
				for _, name := range args {
					r.setVar(name, "")
				}
			}
			return status
		}
		for i, name := range args {
			v := ""
			if i == 0 {
				v = text
			}
			r.setVar(name, v)
		}
		return status
	}
	// Splitting sees the escapes: an escaped separator is data and does not
	// split, which is why the mask rides along rather than the processing
	// being a pre-pass over the string.
	ifs, set := r.ifs()
	fields := splitFieldsLiteral(text, lits, ifs, set)
	if array != "" {
		// An array target takes the fields *as* fields, so the tail of the
		// splitting rule is live here: `IFS=:; read -A a` on `a:b:` fills
		// three elements in the shell where a trailing separator delimits and
		// two in the rest of the panel. The mask goes with it, because an
		// escaped separator is data — `a\:` is one element `a:` in both
		// readings.
		fields = r.trailingSeparatorField(fields, text, lits, ifs, set, false)
		r.setArray(array, fields)
		if clearRest {
			for _, name := range args {
				r.setVar(name, "")
			}
		}
		return status
	}
	// The last variable takes the whole remainder, which is what makes
	// `read a b` put "c d" in b for input "a c d".
	//
	// The remainder is text and not a field, so the tail of the splitting
	// rule is not asked here: the shell where a trailing separator delimits
	// puts `b:` in `b` for `IFS=:; read a b` on `a:b:`, which the extra
	// *field* does not produce. That the remainder is rejoined on a hard
	// space at all is a divergence of its own and unanimous against us — all
	// six shells keep the separators the input had, so `IFS=:; read a b` on
	// `a:b:c` gives `b:c` where this gives `b c` — and it is filed rather
	// than fixed under a question about the tail.
	for i, name := range args {
		switch {
		case i >= len(fields):
			r.setVar(name, "")
		case i == len(args)-1:
			r.setVar(name, strings.Join(fields[i:], " "))
		default:
			r.setVar(name, fields[i])
		}
	}
	return status
}

// readBadNumber is a count, timeout or descriptor argument that is not a
// number. The panel words this per shell per letter; one substrate wording
// carries the fact until a dialect measures its own.
func (r *Runner) readBadNumber(word string) int {
	r.diagf("%s\n", Wording(r.diag().ReadBadNumber, "read: %[1]s: invalid number", word))
	return 1
}

// readerForFd is the stream `read -u` names: standard input by its number,
// anything past the named three from the shell's own table — which holds
// what a redirection resolved to, exactly the "copied as it is now" a real
// descriptor duplication means. A number nothing is open at, or one held by
// something that cannot be read, is not a stream.
func (r *Runner) readerForFd(fd int) (io.Reader, bool) {
	if fd == 0 {
		return r.stdin(), true
	}
	if v, held := r.fds[fd]; held {
		rd, ok := v.(io.Reader)
		return rd, ok
	}
	return nil, false
}

// exactElems is what `read -N` gives an array: the raw text as one element,
// or none at all when nothing survived.
func exactElems(text string) []string {
	if text == "" {
		return nil
	}
	return []string{text}
}

// How a readSegment ended: at the delimiter, at a satisfied count, at the
// end of the input, or out of time. Only the last two mark a failure, and
// they mark different ones.
const (
	endDelim = iota
	endCount
	endEOF
	endTimeout
)

// A byte and how it arrived. evEOF and evTimeout carry no byte.
const (
	evByte = iota
	evEOF
	evTimeout
)

// directByteSource reads the stream a byte at a time, in the calling
// goroutine — the path every read without a deadline takes.
func directByteSource(in io.Reader) func() (byte, int) {
	var ch [1]byte
	return func() (byte, int) {
		n, err := in.Read(ch[:])
		if n == 0 || err != nil {
			return 0, evEOF
		}
		return ch[0], evByte
	}
}

// timedByteSource reads the stream a byte at a time until the deadline. The
// reads happen on their own goroutine — an io.Reader cannot be told to stop
// waiting — and each is made only when asked for, so a satisfied read never
// reads ahead into input a later `read` should get. A read still in flight
// when the deadline passes is abandoned; if its byte ever arrives it is
// lost, which is the cost of a timeout over a plain pipe and is confined to
// the stream the timeout was used on. stop releases the goroutine and must
// be called once the segment is read.
func (r *Runner) timedByteSource(ctx context.Context, in io.Reader, timeout time.Duration) (next func() (byte, int), stop func()) {
	if timeout <= 0 {
		// No time at all, which is out of time before the first byte. A
		// timeout that is *written* as zero never reaches here — it is a
		// question about the stream rather than a deadline, and is answered
		// by ReadZeroTimeout — so this is the guard and not the behavior.
		return func() (byte, int) { return 0, evTimeout }, func() {}
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	type event struct {
		b   byte
		eof bool
	}
	req := make(chan struct{})
	// Buffered by one, so a byte or an end-of-file arriving after the
	// deadline parks in the channel instead of parking the goroutine: the
	// sender loops back to a closed req and exits.
	resp := make(chan event, 1)
	go func() {
		var ch [1]byte
		for range req {
			n, err := in.Read(ch[:])
			if n == 0 || err != nil {
				resp <- event{eof: true}
				return
			}
			resp <- event{b: ch[0]}
		}
	}()
	done := false
	next = func() (byte, int) {
		if done {
			return 0, evEOF
		}
		req <- struct{}{}
		select {
		case ev := <-resp:
			if ev.eof {
				done = true
				return 0, evEOF
			}
			return ev.b, evByte
		case <-tctx.Done():
			done = true
			return 0, evTimeout
		}
	}
	stop = func() {
		cancel()
		close(req)
	}
	return next, stop
}

// readSegment reads until the delimiter, the count, or the end of the input,
// honoring the backslash unless raw: it removes the special meaning of the
// character after it and is itself removed — so `a\tb` (a literal backslash,
// then a t) delivers `atb` — and an escaped newline vanishes whole, the line
// continuation, whatever the delimiter is. An escaped delimiter is data with
// the backslash dropped, and a backslash the input ends on escapes nothing
// and is dropped too; both measured, unanimous (docs/spec/semantics.md,
// "read's options are the dialect's letters"). With exact set the count is
// the only stop: the delimiter and the backslash are ordinary bytes, which
// is what makes `read -N` the way to take input exactly as it came.
//
// literal marks the delivered bytes a backslash escaped, aligned with text,
// nil when there were none. The caller's field splitter needs it because an
// escaped separator does not split — by the time the escapes are gone, an
// escaped space and a separating one are the same byte, so a mask has to
// carry what the string no longer can.
//
// count limits the characters as delivered, after an escape or a
// continuation has folded its backslash away; negative means unlimited.
func readSegment(next func() (byte, int), raw bool, delim byte, count int, exact bool) (text string, literal []bool, end int) {
	var b strings.Builder
	var escapedAt []int // offsets in b whose byte arrived behind a backslash
	pending := false    // a backslash read, its character not yet
	for {
		if count >= 0 && b.Len() >= count {
			return b.String(), literalMask(escapedAt, b.Len()), endCount
		}
		c, ev := next()
		switch ev {
		case evEOF:
			// A pending backslash had nothing to escape and is dropped.
			return b.String(), literalMask(escapedAt, b.Len()), endEOF
		case evTimeout:
			return b.String(), literalMask(escapedAt, b.Len()), endTimeout
		}
		if exact {
			b.WriteByte(c)
			continue
		}
		if pending {
			pending = false
			if c == '\n' {
				// The continuation: backslash and newline vanish whole,
				// under -d too, where the newline is no longer the
				// delimiter. An escaped *other* delimiter is the branch
				// below — data, not a join.
				continue
			}
			// Escaped, so literal: the delimiter does not end the read
			// here, and a separator marked this way must not split.
			escapedAt = append(escapedAt, b.Len())
			b.WriteByte(c)
			continue
		}
		if !raw && c == '\\' {
			pending = true
			continue
		}
		if c == delim {
			return b.String(), literalMask(escapedAt, b.Len()), endDelim
		}
		b.WriteByte(c)
	}
}

// literalMask spreads escaped offsets into a mask aligned with the text, nil
// when nothing was escaped — the common case, kept allocation-free.
func literalMask(at []int, n int) []bool {
	if len(at) == 0 {
		return nil
	}
	m := make([]bool, n)
	for _, i := range at {
		m[i] = true
	}
	return m
}

// readLine reads one line, honoring the backslash unless raw, and says
// whether the input ended.
//
// The two are separate answers because a final line with no newline is both:
// there is a line, and there will not be another. All four shells assign it
// and report failure, which is what stops `while read -r l` from running a
// last unterminated line twice — once as the line, once as the empty read
// after it.
func (r *Runner) readLine(raw bool) (line string, atEOF bool) {
	text, _, end := readSegment(directByteSource(r.In()), raw, '\n', -1, false)
	return text, end == endEOF
}

// biLocal makes variables local to the running function.
//
// Shell scoping is *dynamic*, not lexical: a local is visible to everything
// the function calls, and stops existing when the function returns. That is
// why this saves the outer value on a stack rather than creating a new
// environment — there is only ever one set of variables, and `local` says
// which of them to put back.
func biLocal(r *Runner, _ context.Context, args []string) int {
	if len(r.scopes) == 0 {
		if st, stop := r.localOutsideAFunction(); stop {
			return st
		}
	}
	// The letters are the dialect's own, and may be none at all: dash gives
	// `local` no options, so `local -r x` declares a variable named `-r`
	// there — and then refuses it as the bad name it is. The parse is
	// `typeset`'s, because in both shells that read letters here they are
	// the same letters meaning the same attributes.
	var f declareFlags
	if known := r.sem().LocalOptions; known != "" {
		rest, flags, code := r.parseDeclareFlags("local", args, known)
		if code != 0 {
			return code
		}
		args, f = rest, flags
	}
	if len(r.scopes) > 0 && (len(args) == 0 || f.print) {
		// Bare `local` is a listing, and the shells do not agree what of —
		// see BareLocalListingForm. `local -p` is the same listing spelled
		// as a letter, except when operands narrow it to named declarations.
		if len(args) > 0 {
			return r.declarePrint(args)
		}
		return r.bareLocalListing()
	}
	args, status := r.builtinNames("local", args, false)
	if r.ctl == controlExit {
		return status
	}
	for _, a := range args {
		name, value, hasValue := strings.Cut(a, "=")
		// Before the attributes, for the reason biTypeset gives: `-x` here
		// must not answer for the name this declaration shadows.
		wasExported := r.isExported(name)
		r.applyAttributes(name, f)
		// shadow does nothing when there is no scope to save into, which is
		// the dialect that took this as a global: there is nothing to put
		// back, and it becomes a plain assignment.
		fresh := r.shadow(name)
		r.localExportAttribute(name, f.export)
		if r.unspecified {
			// The declaration is not made at all: reporting the unanswered
			// axis and then assigning anyway is the silent wrong answer.
			return r.status
		}
		r.shadowedExport(name, wasExported)
		if f.assoc && !f.remove {
			// After the shadow, the same order `typeset -A` keeps: the
			// caller's absence comes back when the function returns.
			r.markAssoc(name)
		}
		if hasValue {
			r.setVar(name, value)
			if r.ctl == controlExit {
				return r.status
			}
		} else {
			r.declareEmpty(name, fresh)
		}
		if f.readonly && !f.remove {
			r.markReadonly(name)
		}
	}
	if r.assignFailed && status == 0 {
		// See biExport.
		return 1
	}
	return status
}

// bareOrDashP picks the listing shape for `export` and `readonly`: the one
// `-p` writes when `-p` was written, and BareDeclarationListing when nothing
// was. Two shells answer the two differently — see that field.
func (r *Runner) bareOrDashP(opts string, dashP DeclarationListingForm) DeclarationListingForm {
	if strings.ContainsRune(opts, 'p') {
		return dashP
	}
	return r.sem().BareDeclarationListing
}

// biReadonly marks variables immutable.
func biReadonly(r *Runner, _ context.Context, args []string) int {
	args, opts, code := r.builtinOptions("readonly", args, "paAf")
	if code != 0 {
		return code
	}
	if (strings.ContainsRune(opts, 'p') || opts == "") && len(args) == 0 {
		// The listing: readonly names alone, in the dialect's shape. `-p` and
		// nothing at all list alike, which is the same rule `export` follows
		// and is measured the same way.
		return r.declarePrintForm(nil, r.bareOrDashP(opts, r.sem().ReadonlyListing),
			func(d declaration) bool { return d.readonly })
	}
	args, status := r.builtinNames("readonly", args, false)
	if r.ctl == controlExit {
		return status
	}
	for _, a := range args {
		name, value, hasValue := strings.Cut(a, "=")
		if hasValue {
			r.setVarAs(name, value, assignedByDeclaration)
			if r.ctl == controlExit {
				// See biExport.
				return r.status
			}
			// `readonly` is one shell's `typeset -r` and behaves like it
			// here: an assignment through it resets the export attribute
			// where that shell's `typeset` does. See
			// declarationAssignmentExport.
			r.declarationAssignmentExport(name, false)
			if r.unspecified {
				return r.status
			}
		}
		r.markReadonly(name)
	}
	if r.assignFailed && status == 0 {
		// See biExport.
		return 1
	}
	return status
}

// biExit ends the shell.
//
// A bare `exit` reports what the last command did, and a status is taken
// modulo 256 because that is all a process can carry — `exit 300` is 44 in
// every shell measured.
func biExit(r *Runner, _ context.Context, args []string) int {
	// Asked before the operand is read, because the shell is not going
	// anywhere: `exit 3` with a job stopped stays, and the 3 is never used.
	// The status is the dialect's, and the two that stay disagree about it —
	// bash reports a builtin that failed, zsh reports nothing of the kind.
	if r.HoldsExitForStoppedJobs() {
		return r.diag().StoppedJobsAtExitStatus
	}
	if len(args) > 0 {
		// strconv rather than the local atoi, which is for file descriptors
		// and rejects a sign — `exit -1` has to parse before it can be
		// judged.
		// strconv rather than the local atoi, which is for file descriptors
		// and rejects a sign — `exit -1` has to parse before it can be
		// judged.
		n, err := strconv.Atoi(strings.TrimSpace(args[0]))
		switch {
		case err == nil && n >= 0:
			r.status = n % 256
		default:
			switch r.exitArgument() {
			case ExitArgStrict:
				return r.badExitArg(args[0])
			case ExitArgNumeric:
				if err != nil {
					return r.badExitArg(args[0])
				}
				r.status = ((n % 256) + 256) % 256
			case ExitArgLenient:
				// Text reads as zero there, which is what `exit abc` gives.
				r.status = ((n % 256) + 256) % 256
			default:
				// No dialect answered; exitArgument has already said so,
				// and the script stops rather than exiting with a status it
				// just refused to choose.
				r.ctl = controlExit
				return r.status
			}
		}
	}
	if len(args) == 0 && r.inExitTrap &&
		r.ask(r.sem().ExitInTrapReportsEarlierStatus, "a bare `exit` in an EXIT trap") {
		// The status the trap was entered with, not the one its own commands
		// left behind: `trap "false; exit" 0; true` is 0 in three of the
		// four.
		r.status = r.exitTrapEntryStatus
	}
	r.ctl = controlExit
	return r.status
}

// biTrap sets what runs when a condition arises: EXIT, a signal, or one of
// the pseudo-conditions a dialect has (ERR, DEBUG, RETURN — pseudotrap.go).
//
// A signal nobody can catch is refused rather than accepted and never fired,
// because the silent wrong answer is the one this package exists to avoid.
func biTrap(r *Runner, _ context.Context, args []string) int {
	args, done := r.trapOptions(args)
	if done != trapKeepGoing {
		return int(done)
	}
	if len(args) == 0 {
		return r.printTraps(nil, false)
	}
	body, conds := args[0], args[1:]
	if len(conds) == 0 {
		return r.trapSingleArgument(body)
	}
	// Before the conditions, because the dialect that reads the action now
	// says nothing about a bad condition when the action will not parse.
	if st, refused := r.trapActionRefused(body); refused {
		return st
	}
	// Every condition is checked before any is acted on, so a bad one does
	// not leave half the request applied.
	type target struct {
		name   string
		pseudo string
		sig    syscall.Signal
		exit   bool
	}
	targets := make([]target, 0, len(conds))
	for _, c := range conds {
		if strings.EqualFold(c, "EXIT") || c == "0" {
			targets = append(targets, target{exit: true})
			continue
		}
		// The pseudo-conditions come before the signal table because they
		// are not in it: a dialect that has ERR takes the word here, and one
		// that does not falls through and refuses it as the unknown name it
		// is there.
		if name, ok := r.pseudoCondition(c); ok {
			targets = append(targets, target{pseudo: name})
			continue
		}
		if r.unspecified {
			return r.status
		}
		name, sig, kind := r.canonicalSignal(c)
		switch kind {
		case signalUncatchable:
			// A real signal that nobody can catch. Every shell in the panel
			// takes `trap … KILL` and then never fires it; this refuses
			// instead, which is a deliberate divergence and keeps its own
			// wording, because it is not the dialect's complaint to word.
			r.diagf("trap: %s: not a signal this shell can catch\n", c)
			return 2
		case signalUnknown:
			msg := Wording(r.diag().TrapBadSignal, "trap: %[1]s: bad trap", c)
			if r.diag().TrapBadSignalUnprefixed {
				r.errf("%s\n", msg)
			} else {
				r.diagf("%s\n", msg)
			}
			// 1 in all four, and the one thing they agree on here.
			return 1
		}
		targets = append(targets, target{name: name, sig: sig})
	}
	// Setting, resetting or ignoring anything makes the trap state this
	// runner's own: a listing it inherited from the shell around it is
	// dropped rather than shown alongside — see trapsModified.
	r.trapsModified()
	for _, tg := range targets {
		switch {
		case tg.pseudo != "":
			r.setPseudoTrap(tg.pseudo, body)
		case tg.exit:
			if body == "-" {
				r.exitTrap = nil
				continue
			}
			// A second trap replaces the first rather than adding to it.
			b := body
			r.exitTrap = &b
			r.trapDepth = r.depth
		case body == "-":
			r.trapSignal(tg.name, tg.sig, nil)
		default:
			b := body
			r.trapSignal(tg.name, tg.sig, &b)
		}
	}
	return 0
}

// sortedKeys lists a map's keys in a stable order, so `trap` prints the same
// thing twice running.
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// singleQuote renders text the way `trap` lists it, so the output could be
// fed back in.
func singleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// badExitArg reports an argument `exit` will not take.
//
// The status is 2 in both shells that refuse, and it is not the fatal-error
// status: bash exits 1 for a fatal error and 2 for this. A usage error is its
// own thing, which is why it is written here rather than routed through fatal.
func (r *Runner) badExitArg(arg string) int {
	r.diagf("%s\n", Wording(r.diag().NumericArgument, "%[1]s: invalid number: %[2]s", "exit", arg))
	r.status = 2
	r.ctl = controlExit
	return r.status
}
