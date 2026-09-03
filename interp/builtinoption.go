// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// An option a builtin does not have.
//
// `export -Q x` used to export `x` and drop the `-Q` without a word, which
// hides a typo and hides a shell whose options are not the ones the script was
// written for. Every shell in the panel refuses it, and two of them stop the
// script there.
//
// The shape is the same in all four — the builtin, the option, and a
// complaint — so one wording with two verbs covers it, which is why this is a
// shared refusal rather than one per builtin. The *usage* line that follows it
// in two of the four is per builtin, and is a map for that reason.

// builtinOptions reads the leading `-` words a builtin was given, returning
// what is left.
//
// A word that is exactly `-` is an operand in three of the four, and `--` ends
// the options in all of them. zsh is the exception and eats the `-`: `export -`
// lists the environment there and `unset -` is "not enough arguments", which
// only became visible once operand validation gave the kept `-` something to
// fail. `known` says which letters this builtin takes; anything else is
// refused.
func (r *Runner) builtinOptions(name string, args []string, known string) (rest []string, opts string, code int) {
	for len(args) > 0 {
		a := args[0]
		if len(a) < 2 || a[0] != '-' {
			// A lone `-` is an operand — a *name*, which three of the four
			// then reject as an invalid identifier. Now that operand
			// validation exists the rule is observable from outside, and
			// `unset -` naming the dash is what holds it.
			break
		}
		if a == "--" {
			return args[1:], opts, 0
		}
		for i := 1; i < len(a); i++ {
			if !strings.ContainsRune(known, rune(a[i])) {
				return nil, opts, r.badBuiltinOption(name, "-"+string(a[i]))
			}
			opts += string(a[i])
		}
		args = args[1:]
	}
	return args, opts, 0
}

// badBuiltinOption reports it, and ends the script where the dialect says a
// special builtin's failure is fatal.
//
// Measured: bash and zsh report it and carry on — with 2 and 1 respectively —
// while dash and ksh93 stop the script there, which is the POSIX rule about a
// special builtin. It is a different question from BuiltinSyntaxErrorFatal,
// whose answers are yes for dash alone.
func (r *Runner) badBuiltinOption(name, opt string) int {
	d := r.diag()
	r.diagf("%s\n", Wording(d.BuiltinBadOption, "%[1]s: %[2]s: invalid option", name, opt))
	if usage := d.BuiltinUsage[name]; usage != "" {
		if d.BuiltinUsageUnprefixed {
			r.errf("%s\n", usage)
		} else {
			r.diagf("%s\n", usage)
		}
	}
	status := orDefault(d.BuiltinBadOptionStatus, 2)
	if r.ask(r.sem().BadOptionToSpecialBuiltinFatal, "a special builtin's bad option ending the script") {
		r.status = status
		r.fatalQuiet()
	}
	return status
}
