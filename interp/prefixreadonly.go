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
			if specialBuiltins[argv[0]] {
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
func (r *Runner) prefixRefusalCost(p prefixCommand) (fatal, skip bool, unanswered string) {
	switch r.sem().PrefixRefusalFatality {
	case PrefixRefusalNeverFatal:
	case PrefixRefusalAlwaysFatal:
		return true, true, ""
	case PrefixRefusalFatalOnASpecialBuiltinOrFunction:
		fatal = p.kind == prefixBeforeSpecialBuiltin || p.kind == prefixBeforeFunction
	case PrefixRefusalFatalOnACommandThisShellRuns:
		fatal = !p.throughCommand && p.kind != prefixBeforeExternal
	default:
		return false, true, "what a refused assignment prefix costs"
	}
	if fatal {
		return true, true, ""
	}
	switch r.sem().PrefixRefusalCostsTheCommand {
	case Yes:
		return false, true, ""
	case No:
		return false, false, ""
	}
	return false, true, "a refused assignment prefix costing the command it stood in front of"
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
	var frozen []string
	for _, a := range assigns {
		if a.Operand {
			continue
		}
		// positionalAssignIndex and not prefixAssignsPositional, which
		// *performs* the assignment: this is a question and not a step, and
		// asking it through the acting spelling gave `1=X /bin/echo hi` the
		// parameter the external route deliberately withholds.
		if _, ok := positionalAssignIndex(a.Name); ok {
			continue
		}
		if r.readonly[a.Name] {
			frozen = append(frozen, a.Name)
		}
	}
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
	fatal, skip, unanswered := r.prefixRefusalCost(p)
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
