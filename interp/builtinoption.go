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
	rest, opts, _, code = r.builtinOptionsArg(name, args, known)
	return rest, opts, code
}

// builtinOptionsArg is builtinOptions for a builtin some of whose letters take
// an argument, marked by a `:` after the letter in known — the getopts
// convention, spelled the same way here so a builtin's letters read as its
// optstring.
//
// A word is a *bundle*: `-ra` means `-r -a` in every shell in the panel, so
// the letters are read one at a time. An argument-taking letter ends its
// bundle — the rest of the word is the argument when anything follows it,
// `-d:`, and the next word is when nothing does, `-d :`; both spellings are
// the same option everywhere. A letter whose argument never arrives is
// refused. See docs/spec/semantics.md, "A bundle of option letters is one
// word and several options".
func (r *Runner) builtinOptionsArg(name string, args []string, known string) (rest []string, opts string, optArg map[byte]string, code int) {
	for len(args) > 0 {
		a := args[0]
		if a == "-" {
			// A lone `-` is an operand in three of the four — a *name*,
			// which they then reject as an invalid identifier — and an
			// option in zsh, which eats it and leaves the builtin with one
			// operand fewer.
			if r.ask(r.sem().LoneDashIsAnOption, "a lone `-` given to a builtin") {
				args = args[1:]
				continue
			}
			if r.unspecified {
				return nil, opts, optArg, 2
			}
			break
		}
		if len(a) < 2 || a[0] != '-' {
			break
		}
		if a == "--" {
			return args[1:], opts, optArg, 0
		}
		// Where the letters start. One dialect skips every leading dash
		// before reading the bundle; see BadOptionNaming.
		start := 1
		if r.diag().BadOptionNaming == BadOptionFirstUnknownLetter {
			for start < len(a) && a[start] == '-' {
				start++
			}
		}
		for i := start; i < len(a); i++ {
			takesArg, ok := optionLetter(known, a[i])
			if !ok {
				return nil, opts, optArg, r.refuseOption(name, a, known)
			}
			opts += string(a[i])
			if !takesArg {
				continue
			}
			var value string
			switch {
			case i+1 < len(a):
				value = a[i+1:]
			case len(args) > 1:
				value, args = args[1], args[1:]
			default:
				return nil, opts, optArg, r.optionNeedsArgument(name, a[i])
			}
			if optArg == nil {
				optArg = map[byte]string{}
			}
			optArg[a[i]] = value
			break
		}
		args = args[1:]
	}
	return args, opts, optArg, 0
}

// optionLetter says whether c is one of the letters in known, and whether it
// takes an argument — a `:` after it in known, which is never a letter itself.
func optionLetter(known string, c byte) (takesArg, ok bool) {
	if c == ':' {
		return false, false
	}
	i := strings.IndexByte(known, c)
	if i < 0 {
		return false, false
	}
	return i+1 < len(known) && known[i+1] == ':', true
}

// optionNeedsArgument is an argument-taking letter whose bundle ended the
// argument list. Every shell refuses it — with a status of 2 except zsh — and
// the wording is the substrate's own, like the not-implemented one below: no
// letter a builtin here implements takes an argument yet, so no dialect has
// been measured saying it as itself.
func (r *Runner) optionNeedsArgument(builtin string, letter byte) int {
	r.diagf("%s: -%c: option requires an argument\n", builtin, letter)
	return 2
}

// badBuiltinOption reports it, and ends the script where the dialect says a
// special builtin's failure is fatal.
//
// Measured: bash and zsh report it and carry on — with 2 and 1 respectively —
// while dash and ksh93 stop the script there, which is the POSIX rule about a
// special builtin. It is a different question from BuiltinSyntaxErrorFatal,
// whose answers are yes for dash alone.
// refuseOption is what a builtin says about a leading `-` word it cannot use,
// given the letters it does understand.
//
// One place, because three things have to agree: which letter is the
// offending one, whether the dialect names that letter or the whole word, and
// whether the dialect *has* the option and this shell simply does not.
func (r *Runner) refuseOption(builtin, word, known string) int {
	letter, name := r.badOption(word, known)
	if has := r.diag().UnimplementedOptionLetters[builtin]; has != "" &&
		strings.IndexByte(has, letter) >= 0 {
		// An option the dialect really has. Saying it is unknown would be a
		// different and worse answer than saying it is missing — and it is
		// named the way the dialect names a bad one, the letter rather than
		// the bundle it rode in on: `read -ra` is about `-a`, because `-r`
		// is not the missing half.
		r.diagf("%s: %s is not implemented yet\n", builtin, name)
		return 2
	}
	return r.badBuiltinOption(builtin, name)
}

// badOption picks the offending letter and how to spell it. See
// Diagnostics.BadOptionNaming.
func (r *Runner) badOption(word, known string) (byte, string) {
	start := 1
	if r.diag().BadOptionNaming == BadOptionFirstUnknownLetter {
		for start < len(word) && word[start] == '-' {
			start++
		}
	}
	letter := byte('-')
	for i := start; i < len(word); i++ {
		if _, ok := optionLetter(known, word[i]); !ok {
			letter = word[i]
			break
		}
	}
	// The whole word survives only where it begins with `--`: measured with
	// `read -rx`, the dialect recorded as naming whole words names the
	// letter in a single-dash bundle — `-x`, like everyone else — and keeps
	// `--foo` intact where bash says `--` and zsh walks past the dashes.
	if r.diag().BadOptionNaming == BadOptionWholeWord && strings.HasPrefix(word, "--") {
		return letter, word
	}
	return letter, "-" + string(letter)
}

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
	// Only for a builtin POSIX marks special, which is what the rule is
	// about: `wait` is not one, and the two dialects that end a script over
	// `export -q` print the same complaint for `wait -x` and carry on. Every
	// caller of this was special until `wait` was not, so the check had
	// never been reached and was wrong the moment it was.
	if specialBuiltins[name] &&
		r.ask(r.sem().BadOptionToSpecialBuiltinFatal, "a special builtin's bad option ending the script") {
		r.status = status
		r.fatalQuiet()
	}
	return status
}
