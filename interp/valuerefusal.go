// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// A name whose stores refuse some **values**, the way a frozen name refuses
// every value.
//
// It exists for one dialect's OPTIND. Measured 2026-10-15 on dash 0.5.12,
// `-c` under `env -i PATH=/usr/bin:/bin`, every store into OPTIND checks that
// the value is a number and refuses it otherwise:
//
//	OPTIND=x                        `Illegal number: x`, the script ends at 2
//	export OPTIND=x / local / readonly
//	                                `export: Illegal number: x`, the same
//	OPTIND=x true / OPTIND=x env    `Illegal number: x`, the same, nothing run
//	for OPTIND in x; do :; done     the same
//	eval OPTIND=x                   `eval: Illegal number: x`, the same
//	: $((OPTIND=-1))                `Illegal number: -1`, the same
//	command export OPTIND=x         `export: Illegal number: x`, status 2,
//	                                and the script goes on
//	read OPTIND  (reading `x`)      `read: Illegal number: x`, status 2,
//	                                OPTIND untouched, the script goes on
//	getopts a OPTIND -a             `getopts: Illegal number: a`, status 2,
//	                                OPTIND left at the cursor's 2
//
// Every row is the route a **readonly** name's refusal takes in the same
// shell, sentence aside: `readonly y; read y` is `read: y: is read only` at 2
// and goes on, `readonly y; y=2 env` ends the script, `readonly y; command
// export y=2` goes on at 2. So the refusal is not a route of its own. It is
// the freeze's route with a different sentence, decided per value, and every
// door a freeze is checked at asks this too — the bare store in setVarAs,
// `read`'s and `getopts`'s own checks ahead of their stores, and a prefix
// handed to a program, which never reaches setVarAs. One table, asked from
// each of those, rather than a check per builtin: a door that forgets it
// stores the value, which is the bug this replaces (#6537).
//
// `unset OPTIND` is the same check meeting the empty string, and was already
// modeled as a refused unset — see Runner.RefuseUnset.

// RefuseValues says a store into name refuses the values refuse reports, with
// the sentence refuse returns. The refusal takes the route a readonly name's
// would: the same fatality, the same status, the same builtin named in front.
// The sentence is the bare complaint, `Illegal number: x`; a builtin that
// names itself for a freeze names itself in front of it.
func (r *Runner) RefuseValues(name string, refuse func(value string) (string, bool)) {
	if r.valueRefusals == nil {
		r.valueRefusals = map[string]func(string) (string, bool){}
	}
	r.valueRefusals[name] = refuse
	r.specialNames.add(name)
}

// refusedValue is the sentence a store of value into name earns, if any.
func (r *Runner) refusedValue(name, value string) (string, bool) {
	if !r.specialNames.mayHold(name) {
		return "", false
	}
	refuse, ok := r.valueRefusals[name]
	if !ok {
		return "", false
	}
	return refuse(value)
}

// refuseTheValue is refuseReadonly for a value rather than a freeze: it
// reports whether the store is refused, having said so and having settled
// what the refusal costs exactly as the freeze's would have.
func (r *Runner) refuseTheValue(name, value string, form assignForm) bool {
	sentence, refused := r.refusedValue(name, value)
	if !refused {
		return false
	}
	return r.refuseAsAFreezeWould(name, form, sentence)
}

// prefixValueRefused is the same question for a prefix handed to a program,
// which reaches the program's environment without passing through setVarAs:
// a value the name does not take is reported and costs what a frozen name's
// prefix costs in front of a program. Measured 2026-10-15 on dash 0.5.12,
// `OPTIND=x env; echo after` and `OPTIND=x /usr/bin/true` are `Illegal
// number: x`, nothing run and the script ended at 2, which is what `readonly
// y; y=1 env` does there. See RefuseValues and prefixRefusalCost.
func (r *Runner) prefixValueRefused(prefixEnv []string, p prefixCommand) bool {
	if len(r.valueRefusals) == 0 {
		return false
	}
	for _, kv := range prefixEnv {
		name, value, _ := strings.Cut(kv, "=")
		sentence, refused := r.refusedValue(name, value)
		if !refused {
			continue
		}
		fatal, _, giveUpTheLine, unanswered := r.prefixRefusalCost(p)
		r.reportFreezeRefusal(name, assignedAnyhow, false, sentence)
		switch {
		case unanswered != "":
			r.diagf("%s\n", r.unanswered(unanswered))
			r.status, r.unspecified = 2, true
		case fatal:
			r.fatalQuiet()
		case giveUpTheLine:
			r.GiveUpTheCommandAt(1)
		default:
			r.status = 1
		}
		return true
	}
	return false
}
