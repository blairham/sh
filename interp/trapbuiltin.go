// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"sort"
	"strings"
	"syscall"
)

// trapOutcome is what reading trap's options decided: either a status to
// return, or that the remaining words are trap's ordinary arguments.
type trapOutcome int

// trapKeepGoing is not a status. It is outside the range a builtin can
// return, so it cannot be confused with one.
const trapKeepGoing trapOutcome = -1

// trapOptions reads the leading options, the way printfOptions does.
//
// `trap` took none at all before this: `trap -p` set a trap whose action was
// the word `-p`, and the shell then failed to run it when the trap fired —
// a wrong answer that only surfaced later, and somewhere else. That is still
// zsh's answer, and is now zsh's alone.
//
// Ending them at `--` is unanimous, and a lone `-` is trap's own word for
// "put this condition back", never an option. The rest is which letters the
// dialect knows: bash has `-p`, `-P` and `-l`, ksh93 only `-p`, dash none at
// all, and zsh does not read options here in the first place.
func (r *Runner) trapOptions(args []string) ([]string, trapOutcome) {
	// One step rather than a loop: every option here is the whole command.
	// `-p`, `-P` and `-l` print and stop, `--` hands back what follows it,
	// and an unknown one is refused — so nothing is ever read twice.
	if len(args) > 0 {
		a := args[0]
		if len(a) < 2 || a[0] != '-' {
			return args, trapKeepGoing
		}
		if a == "--" {
			return args[1:], trapKeepGoing
		}
		// Whether options are read at all comes first, because it is asked
		// of the letters this shell knows as much as of the ones it does
		// not: zsh takes `-p` as the action just as it takes `-Q`.
		if !r.ask(r.sem().TrapParsesOptions, "trap reading a leading `-` word as an option") {
			return args, trapKeepGoing
		}
		if r.unspecified {
			return nil, 2
		}
		switch a {
		case "-p":
			if r.ask(r.sem().TrapPrintsWithP, "`trap -p`") {
				if r.unspecified {
					return nil, trapOutcome(r.status)
				}
				return nil, trapOutcome(r.printTraps(args[1:], r.printsBareWithConditions(args[1:])))
			}
		case "-P":
			if r.ask(r.sem().TrapPrintsBareWithP, "`trap -P condition`") {
				if r.unspecified {
					return nil, trapOutcome(r.status)
				}
				if len(args) == 1 {
					// The one option that insists on an operand: printing
					// every trap without saying which is `-p`'s job.
					r.diagf("%s\n", Wording(r.diag().TrapBarePrintNeedsCondition,
						"trap: -P requires at least one signal name"))
					return nil, 2
				}
				return nil, trapOutcome(r.printTraps(args[1:], true))
			}
		case "-l":
			if r.ask(r.sem().TrapListsSignalsWithL, "`trap -l`") {
				if r.unspecified {
					return nil, trapOutcome(r.status)
				}
				return nil, trapOutcome(r.listSignals())
			}
		}
		if r.unspecified {
			return nil, 2
		}
		return nil, trapOutcome(r.badBuiltinOption("trap", a))
	}
	return args, trapKeepGoing
}

// printsBareWithConditions answers whether naming conditions to `-p` changes
// what it prints.
//
// ksh93 is the dialect that says yes: `trap -p` writes the whole `trap -- :
// EXIT` line and `trap -p EXIT` writes just the action, which is what bash
// spells `-P`. Asked only when there are conditions, so the dialects that do
// not have the question are never marked as failing to answer it.
func (r *Runner) printsBareWithConditions(conds []string) bool {
	if len(conds) == 0 {
		return false
	}
	return r.ask(r.sem().TrapPrintsBareWithConditions, "`trap -p condition` writing the action alone")
}

// printTraps writes what is currently trapped, either every condition or the
// named ones. Bare is `-P`: the action alone, with nothing around it.
//
// A condition that is not trapped prints nothing and is not an error, which
// is how `trap -p EXIT` reports "nothing there" in both dialects that have it.
func (r *Runner) printTraps(conds []string, bare bool) int {
	if len(conds) == 0 {
		if bare {
			return 0
		}
		if r.exitTrap != nil {
			r.printf("trap -- %s EXIT\n", r.quotedTrapAction(*r.exitTrap))
		}
		for _, name := range sortedKeys(r.sigs().traps) {
			r.printf("trap -- %s %s\n", r.quotedTrapAction(r.sigs().traps[name]), r.printedSignalName(name))
		}
		return 0
	}
	for _, c := range conds {
		action, name, _, ok := r.trapFor(c)
		if !ok {
			msg := Wording(r.diag().TrapBadSignal, "trap: %[1]s: bad trap", c)
			if r.diag().TrapBadSignalUnprefixed {
				r.errf("%s\n", msg)
			} else {
				r.diagf("%s\n", msg)
			}
			return 1
		}
		if action == nil {
			continue
		}
		if bare {
			r.printf("%s\n", *action)
			continue
		}
		r.printf("trap -- %s %s\n", r.quotedTrapAction(*action), r.printedSignalName(name))
	}
	return 0
}

// trapFor is what a condition is currently trapped to, and whether the
// condition is one this shell knows at all.
func (r *Runner) trapFor(cond string) (*string, string, syscall.Signal, bool) {
	if strings.EqualFold(cond, "EXIT") || cond == "0" {
		return r.exitTrap, "EXIT", 0, true
	}
	name, sig, kind := r.canonicalSignal(cond)
	if kind == signalUnknown {
		return nil, "", 0, false
	}
	if action, ok := r.sigs().traps[name]; ok {
		return &action, name, sig, true
	}
	return nil, name, sig, true
}

// trapSingleArgument is `trap condition` — one word, with nothing to run.
//
// Three of the four read the word as a condition to put back, which is what
// `trap - condition` spells the long way. ksh93 refuses the form outright and
// the refusal ends the script, so `trap EXIT` is a working reset in three
// dialects and a fatal error in the fourth.
func (r *Runner) trapSingleArgument(cond string) int {
	if !r.ask(r.sem().TrapOneArgumentIsACondition, "`trap condition` putting that condition back") {
		if r.unspecified {
			return r.status
		}
		r.diagf("%s\n", Wording(r.diag().TrapConditionRequired, "trap: condition(s) required"))
		r.fatalQuiet()
		return r.status
	}
	if r.unspecified {
		return r.status
	}
	_, name, sig, ok := r.trapFor(cond)
	if !ok {
		return r.trapUnknownSingleCondition(cond)
	}
	if name == "EXIT" {
		r.exitTrap = nil
		return 0
	}
	r.trapSignal(name, sig, nil)
	return 0
}

// trapUnknownSingleCondition is the one word that turned out not to name a
// condition, which the three dialects that read it as one answer three ways.
//
// bash prints its usage rather than naming the word, because with one word it
// cannot tell a misspelled condition from an action someone forgot to give a
// condition to. dash names the word the same way it does anywhere else. zsh
// says nothing at all — and does say something for `trap : foo`, so this is
// the single-word form's own answer and not zsh declining to check.
func (r *Runner) trapUnknownSingleCondition(cond string) int {
	if !r.ask(r.sem().TrapReportsAnUnknownSingleCondition, "`trap notacondition` being complained about") {
		if r.unspecified {
			return r.status
		}
		return 0
	}
	if r.unspecified {
		return r.status
	}
	if r.ask(r.sem().TrapSingleUnknownConditionIsUsage, "`trap notacondition` printing the usage rather than the word") {
		if r.unspecified {
			return r.status
		}
		d := r.diag()
		// The same usage line a bad option prints, from the same table.
		if usage := d.BuiltinUsage["trap"]; usage != "" {
			if d.BuiltinUsageUnprefixed {
				r.errf("%s\n", usage)
			} else {
				r.diagf("%s\n", usage)
			}
		}
		return orDefault(d.BuiltinBadOptionStatus, 2)
	}
	if r.unspecified {
		return r.status
	}
	msg := Wording(r.diag().TrapBadSignal, "trap: %[1]s: bad trap", cond)
	if r.diag().TrapBadSignalUnprefixed {
		r.errf("%s\n", msg)
	} else {
		r.diagf("%s\n", msg)
	}
	return 1
}

// quotedTrapAction spells an action the way this dialect's `trap` lists it,
// which is not always the way its `alias` does: zsh writes a tab as `$'a\tb'`
// in an alias and as a plainly quoted `'a<tab>b'` in a trap.
func (r *Runner) quotedTrapAction(action string) string {
	return r.quoteListedValue(r.sem().TrapQuoting, "`trap`", action)
}

// printedSignalName is how this dialect spells a signal when printing what
// is trapped: bash writes SIGINT where the other three write INT. EXIT is
// not a signal and never takes the prefix in any of them.
func (r *Runner) printedSignalName(name string) string {
	if name == "EXIT" {
		return name
	}
	return r.diag().TrapPrintsSignalPrefix + name
}

// listSignals is `trap -l`.
//
// The same plain listing `kill -l` prints, and for the same reason: the real
// shells number the table and lay it out in columns, and the table is not the
// same on two operating systems. Printed plainly here and kept out of the
// corpus rather than recorded as a fact about a machine.
func (r *Runner) listSignals() int {
	byNumber := append([]signalEntry{}, knownSignals...)
	sort.Slice(byNumber, func(i, j int) bool { return byNumber[i].Sig < byNumber[j].Sig })
	for _, k := range byNumber {
		r.printf("%s\n", k.Name)
	}
	return 0
}
