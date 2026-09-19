// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// prefixCommandKind is what an assignment prefix stands in front of.
//
// It exists because two shells in the panel answer a refused prefix by the
// *kind* of the command rather than by the prefix, and they draw the line in
// different places. Nothing else in the interpreter needs the distinction, so
// it is computed where the refusal is decided and nowhere else.
type prefixCommandKind int

const (
	// prefixBeforeExternal is a word this shell will start a process for,
	// including one it will not find: a name nothing answers for is an
	// external that fails to run, which is how the panel treats it.
	prefixBeforeExternal prefixCommandKind = iota
	// prefixBeforeRegularBuiltin is a builtin POSIX does not call special.
	prefixBeforeRegularBuiltin
	// prefixBeforeSpecialBuiltin is one POSIX does — the set that also keeps
	// the assignment where AssignmentPrefixPersistsOnSpecialBuiltin says so.
	prefixBeforeSpecialBuiltin
	// prefixBeforeFunction is a function defined in this shell.
	prefixBeforeFunction
)

// prefixCommand is how an assignment prefix's command reads, which is two
// facts rather than one: what the word *resolves to*, and whether it was
// written through `command`.
//
// Both are needed because the two shells that answer by kind read different
// ones. ksh93 reads what the word resolves to and `command` is transparent to
// it — measured 2026-09-11, `x=2 command true` says nothing at all and
// `x=2 command /bin/echo CE` refuses, never runs the echo and reports 1, which
// is exactly what the bare `true` and the bare `/bin/echo` do there. zsh reads
// the word that was written and `command` is not transparent: `x=2 true` ends
// the script and `x=2 command true` refuses, does not run it, reports 1 and
// carries on.
type prefixCommand struct {
	// kind is what the command word resolves to, looking through `command`.
	kind prefixCommandKind
	// throughCommand says `command` was the word written, whatever it named.
	throughCommand bool
}

// prefixCommandOf reads the command a prefix stands in front of.
//
// The lookup order is the dispatch's own — a function shadows a builtin and a
// builtin shadows an external — so this cannot disagree with what will
// actually run.
func (r *Runner) prefixCommandOf(argv []string) prefixCommand {
	p := prefixCommand{}
	for len(argv) > 0 {
		if _, ok := r.funcs[argv[0]]; ok {
			p.kind = prefixBeforeFunction
			return p
		}
		if _, ok := r.lookupBuiltin(argv[0]); ok {
			if argv[0] == "command" && len(argv) > 1 {
				// Look through it for the resolved kind and remember that it
				// was written, which is the whole of the disagreement.
				p.throughCommand = true
				argv = commandOperandOf(argv[1:])
				continue
			}
			if r.IsSpecialBuiltinHere(argv[0]) {
				p.kind = prefixBeforeSpecialBuiltin
				return p
			}
			p.kind = prefixBeforeRegularBuiltin
			return p
		}
		p.kind = prefixBeforeExternal
		return p
	}
	// `command` with nothing after it is the builtin itself.
	p.kind = prefixBeforeRegularBuiltin
	return p
}

// commandOperandOf drops `command`'s own option words so that what follows is
// the command it names. Nothing here validates them: a word that is no option
// of `command`'s is the command, and the builtin itself is what refuses one
// that is wrong.
func commandOperandOf(argv []string) []string {
	for len(argv) > 0 && len(argv[0]) > 1 && argv[0][0] == '-' {
		if argv[0] == "--" {
			return argv[1:]
		}
		argv = argv[1:]
	}
	return argv
}

// frozenPrefixNames is the names in an assignment prefix that are readonly,
// in the order they were written.
//
// One walk rather than two: the early check asks whether there is one at all
// and refusePrefixes names each of them, and a second copy of "which of these
// count" is how the two would come to disagree about a positional prefix.
//
// positionalAssignIndex and not prefixAssignsPositional, which *performs* the
// assignment: this is a question and not a step, and asking it through the
// acting spelling gave `1=X /bin/echo hi` the parameter the external route
// deliberately withholds.
func (r *Runner) frozenPrefixNames(assigns []*syntax.Assign) []string {
	var frozen []string
	for _, a := range assigns {
		if a.Operand {
			continue
		}
		if _, ok := positionalAssignIndex(a.Name); ok {
			continue
		}
		if r.readonly[a.Name] {
			frozen = append(frozen, a.Name)
		}
	}
	return frozen
}

// FrozenPrefixCheckOrder is when a frozen name in an assignment prefix is
// refused, against when the command's values are expanded and its
// redirections opened — see [Semantics.PrefixToAFrozenNameIsCheckedFirst].
type FrozenPrefixCheckOrder uint8

const (
	// FrozenPrefixCheckUnspecified is no answer, and is refused like any
	// other.
	FrozenPrefixCheckUnspecified FrozenPrefixCheckOrder = iota
	// FrozenPrefixCheckedWithTheCommand does whatever the command was going
	// to do first: a value that will not expand and a file that will not
	// open each report on their own and the frozen name is never mentioned.
	// zsh and dash.
	//
	// The zero value is the unspecified one rather than this, because a
	// dialect that has not been asked has not answered.
	FrozenPrefixCheckedWithTheCommand
	// FrozenPrefixCheckedFirst refuses the name ahead of everything the
	// command does, whatever the command is: bash and BusyBox ash.
	FrozenPrefixCheckedFirst
	// FrozenPrefixCheckedFirstWhereItPersists refuses it ahead of the
	// command only where the assignment is a real store — in front of a
	// function or a special builtin — and opens the redirections first for a
	// regular builtin and an external: ksh93.
	//
	// The same split [PrefixRedirectionOrder] draws, and the same reading of
	// it: what the word resolves to, looking through `command`. It is one
	// shell reading its own persistence rule twice rather than two rules
	// that happen to agree, which is why both read prefixCommandOf.
	FrozenPrefixCheckedFirstWhereItPersists
)

func (f FrozenPrefixCheckOrder) String() string {
	switch f {
	case FrozenPrefixCheckedWithTheCommand:
		return "the frozen name is checked with the command"
	case FrozenPrefixCheckedFirst:
		return "the frozen name is checked first"
	case FrozenPrefixCheckedFirstWhereItPersists:
		return "the frozen name is checked first where the prefix persists"
	}
	return "unspecified"
}

// refusePrefixesEarly is the readonly check on an assignment prefix, run
// before the command's values are expanded and before its redirections are
// opened. That order is Semantics.PrefixToAFrozenNameIsCheckedFirst.
//
// It returns whether the command is given up on, and it sets
// Runner.prefixCheckedFirst so the dispatch routes below skip the value of a
// frozen name and refusePrefixes does not report the same names twice.
//
// The axis is read only once a name in the prefix is actually frozen, which
// is where the other three prefix axes are asked and for the same reason: a
// command with a prefix is a minority of a script's lines, and one with a
// frozen name in it a minority of those.
func (r *Runner) refusePrefixesEarly(assigns []*syntax.Assign, argv []string) bool {
	if len(argv) == 0 || len(r.frozenPrefixNames(assigns)) == 0 {
		return false
	}
	switch r.sem().PrefixToAFrozenNameIsCheckedFirst {
	case FrozenPrefixCheckedWithTheCommand:
		return false
	case FrozenPrefixCheckedFirst:
	case FrozenPrefixCheckedFirstWhereItPersists:
		// Read before the refusal rather than inside it, because the
		// question here is the *order* and not whether the name is refused
		// at all: a regular builtin is both the kind this shell opens the
		// redirections for and the kind PrefixToARegularBuiltinIsRefused
		// says nothing about, and asking the order first keeps the two
		// answers from being told apart by which one happened to fire.
		if k := r.prefixCommandOf(argv).kind; k != prefixBeforeFunction &&
			k != prefixBeforeSpecialBuiltin {
			return false
		}
	default:
		r.diagf("%s\n", r.unanswered(
			"a frozen name in a prefix checked before the command's values and redirections"))
		r.status, r.unspecified = 2, true
		return false
	}
	r.prefixCheckedFirst = true
	_, stop := r.refusePrefixesNow(assigns, r.prefixCommandOf(argv), true)
	return stop
}

// prefixRefusalApplies reports whether a prefix to a frozen name is refused at
// all in front of this command.
//
// It is No in one shell and one position: ksh93 says nothing whatever about
// `readonly x=1; x=2 true`, runs the builtin and reports 0, and the same for
// `x=2 echo E`, for an alias naming one, and for `command` naming one. Every
// other column complains about all four. Measured 2026-09-11.
//
// Asked only in front of a regular builtin, which is the only position it
// parts the panel at: an external command, a special builtin and a function
// are refused in all six columns.
func (r *Runner) prefixRefusalApplies(p prefixCommand) bool {
	if p.kind != prefixBeforeRegularBuiltin {
		return true
	}
	return r.ask(r.sem().PrefixToARegularBuiltinIsRefused,
		"an assignment prefix to a readonly name being refused in front of a regular builtin")
}

// prefixRefusalCost decides what a refusal costs this command, without
// reporting anything: the two answers, and the name of an axis that has none.
//
// Three outcomes and not two, which is the shape the panel has: bash reports
// and runs the command anyway at status 0, ksh93 and zsh report and leave the
// command unrun at status 1, and dash ends the script.
//
// A fourth exists and has no value here: bash invoked as `sh` reports, gives
// up the rest of the command *list* and reaches the next line. No preset
// answers for that column — the `sh` invocation changes the grammar and not
// the semantics vector, so it takes bash's answer — and the corpus records it
// rather than the code modeling it. What made this issue is that the fourth
// was *ours*, in bash and in ksh, for the two of the four cells a `;` can
// see (#1219).
//
// Decided before the names are reported rather than after, because how many of
// them are named follows from this: a shell that gives the command up stops at
// the first refusal and a shell that carries on names them all.
func (r *Runner) prefixRefusalCost(p prefixCommand) (fatal, skip, giveUpTheLine bool, unanswered string) {
	if r.posixMode && r.ask(r.sem().PosixModeSharpensAPrefixRefusal,
		"POSIX mode sharpening what a refused assignment prefix costs") {
		// The mode answers both halves at once and the fatality and cost
		// axes are not consulted: see
		// Semantics.PosixModeSharpensAPrefixRefusal for the five rows.
		if p.kind == prefixBeforeSpecialBuiltin {
			return true, true, false, ""
		}
		return false, true, true, ""
	}
	switch r.sem().PrefixRefusalFatality {
	case PrefixRefusalNeverFatal:
	case PrefixRefusalAlwaysFatal:
		return true, true, false, ""
	case PrefixRefusalFatalOnASpecialBuiltinOrFunction:
		fatal = p.kind == prefixBeforeSpecialBuiltin || p.kind == prefixBeforeFunction
	case PrefixRefusalFatalOnACommandThisShellRuns:
		fatal = !p.throughCommand && p.kind != prefixBeforeExternal
	default:
		return false, true, false, "what a refused assignment prefix costs"
	}
	if fatal {
		return true, true, false, ""
	}
	switch r.sem().PrefixRefusalCostsTheCommand {
	case Yes:
		return false, true, false, ""
	case No:
		return false, false, false, ""
	}
	return false, true, false, "a refused assignment prefix costing the command it stood in front of"
}

// refusePrefixes reports a command's assignment prefixes to frozen names and
// says whether the command may still run.
//
// report is the caller's, because a value that failed to expand has already
// had a complaint of its own and which of the two a shell writes is a second
// split: bash names the frozen name and never evaluates the value, and dash,
// ksh93 and zsh name the expansion and never mention the name. The refusal
// itself stands either way — a value that would not expand is not a value the
// name may take.
//
// The form is assignedAnyhow and not a declaration's, which is what a prefix
// is: it is never `export x=2`, so the declaration wording and the builtin's
// name in the location are never this refusal's, whatever builtin happens to
// be running the command it is prefixed to.
func (r *Runner) refusePrefixes(assigns []*syntax.Assign, p prefixCommand, report bool) (refused, stop bool) {
	if r.prefixCheckedFirst {
		// Already reported, ahead of the values and the redirections, by the
		// dialect that checks the prefix before either. The caller still
		// needs "refused" so that the name keeps its value, and never
		// "stop": a command the early check gave up on never reached here.
		return true, false
	}
	return r.refusePrefixesNow(assigns, p, report)
}

// refusePrefixesNow is refusePrefixes without the already-reported guard, so
// that the early check can be the one report rather than a second one.
func (r *Runner) refusePrefixesNow(assigns []*syntax.Assign, p prefixCommand, report bool) (refused, stop bool) {
	frozen := r.frozenPrefixNames(assigns)
	if len(frozen) == 0 {
		// Nothing is frozen, so no axis is asked. The commands with a prefix
		// at all are a minority of a script's and the ones with a frozen name
		// in it are a minority of those, which is what keeps three axes off
		// the common path entirely.
		return false, false
	}
	if !r.prefixRefusalApplies(p) {
		// Not a refusal in this position. The name still keeps its value —
		// the caller leaves it unassigned on the strength of the first result
		// — and the command runs with nothing said.
		return true, false
	}
	fatal, skip, giveUpTheLine, unanswered := r.prefixRefusalCost(p)
	if report {
		// A shell that carries on names *every* frozen name in the prefix,
		// in written order; one that gives the command up stops at the first.
		// So the count is not an answer of its own — it follows from the cost
		// — and `readonly x=1 z=9; x=2 z=8 /bin/echo RAN` writes two
		// complaints in bash and one in ksh93, zsh and dash.
		named := frozen
		if skip {
			named = frozen[:1]
		}
		for _, name := range named {
			r.reportReadonlyRefusal(name, assignedAnyhow, false)
		}
	}
	switch {
	case unanswered != "":
		r.diagf("%s\n", r.unanswered(unanswered))
		r.status, r.unspecified = 2, true
	case fatal:
		r.fatalQuiet()
	case giveUpTheLine:
		// The command is not run and neither is the rest of the command
		// list, which is the whole of the difference from the case below:
		// measured, `v=3 true; echo pre=$?` writes no `pre=` and the next
		// line reports 1. Through the door the core's own give-ups use, so
		// a function body, an `if`, a `for` and a `||` unwind alike and a
		// command string ends the shell instead.
		r.GiveUpTheCommandAt(1)
	case skip:
		// The command is not run and the status is the refusal's own.
		// Measured: `readonly x=1; x=2 /bin/echo RAN; echo after` prints the
		// complaint, `after`, and never `RAN`, at status 1 — and a name
		// nothing can run reports 1 rather than the 127 it would have earned,
		// because the lookup never happens.
		r.status = 1
	}
	return true, skip
}

// prefixPersistsAtThisBuiltin reports whether a prefix in front of the
// builtin about to run is one the shell keeps.
//
// Two questions rather than one, and the second exists because `command` is a
// **precommand word**: the builtin whose name reaches `lookupBuiltin` is
// `command` itself, which is regular, while the thing the prefix is really in
// front of is whatever `command` names. prefixCommandOf has already looked
// through it — that is what `kind` holds — so the only thing left to decide is
// whether looking through is what this dialect does, and the panel splits on
// exactly that. See Semantics.CommandKeepsASpecialBuiltinsPrefix.
//
// Asked at the disagreement and nowhere else: a word that is itself a special
// builtin asks the first question alone, and a dialect that persists nothing
// never reaches the second.
func (r *Runner) prefixPersistsAtThisBuiltin(word string, kind prefixCommand) bool {
	throughCommand := false
	if !r.IsSpecialBuiltinHere(word) {
		if !kind.throughCommand || kind.kind != prefixBeforeSpecialBuiltin {
			return false
		}
		throughCommand = true
	}
	if !r.ask(r.sem().AssignmentPrefixPersistsOnSpecialBuiltin,
		"an assignment before a special builtin persisting") {
		return false
	}
	if !throughCommand {
		return true
	}
	return r.ask(r.sem().CommandKeepsASpecialBuiltinsPrefix,
		"`command` in front of a special builtin keeping its prefix")
}
