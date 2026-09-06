// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"context"
	"fmt"
	"sort"

	"github.com/blairham/sh/interp"
)

// `shopt` is this dialect's builtin alone: the other three panel shells
// answer it with "command not found", so it is registered here and nowhere
// near the core. What it switches, though, lives in the core as
// interp.MatchOption values, because pathname expansion and the matcher are
// the core's — this file only maps the names to them.
//
// Everything below is measured against bash 5.3 (see
// docs/spec/grammar/patterns.md and the shopt/ corpus cases): the listing
// format, the statuses, which flags conflict, and what each wired option
// changes.

// shoptModes are the names wired to real behavior.
var shoptModes = map[string]interp.MatchOption{
	"nullglob":    interp.UnmatchedPatternIsEmpty,
	"dotglob":     interp.PatternsMatchHidden,
	"nocaseglob":  interp.GlobFoldsCase,
	"nocasematch": interp.MatchFoldsCase,
	"globstar":    interp.StarStarCrossesDirectories,
	"extglob":     interp.QuantifiedGroupsEverywhere,
}

// shoptSwitches are the names wired to a switch the core holds rather than to
// the matcher — the second kind of "really implemented", and today one name.
//
// `expand_aliases` is the option a bash script has to set before an alias
// means anything, which is why an alias case could not be written for bash at
// all while this was refused (#632). It is implemented rather than recorded:
// the alias is expanded afterwards and not expanded before, and `shopt -u`
// mid-script stops it again — measured in bash 5.3 and bash 3.2 alike, on all
// three non-interactive routes.
var shoptSwitches = map[string]struct {
	get func(*interp.Runner) bool
	set func(*interp.Runner, bool)
}{
	"expand_aliases": {
		get: (*interp.Runner).AliasExpansion,
		set: (*interp.Runner).SetAliasExpansion,
	},
}

// shoptStates are the rest of the names bash 5.3 lists, with the state this
// implementation is in — not the state bash defaults to, the same rule
// interp's set-option table follows. Asking for the state we already hold is
// a request that has been granted; asking to move is refused out loud,
// because granting it would promise behavior nothing here provides.
//
// The handful that are on are on because the behavior they name is simply
// how this shell works: ranges match by byte, `*` never yields `.` or `..`,
// comments are honored everywhere, `$'…'` is decoded inside `${…}`, the
// prompt expands parameters, and `.` searches PATH.
var shoptStates = map[string]bool{
	"array_expand_once":       false,
	"assoc_expand_once":       false,
	"autocd":                  false,
	"bash_source_fullpath":    false,
	"cdable_vars":             false,
	"cdspell":                 false,
	"checkhash":               false,
	"checkjobs":               false,
	"checkwinsize":            false,
	"cmdhist":                 false,
	"compat31":                false,
	"compat32":                false,
	"compat40":                false,
	"compat41":                false,
	"compat42":                false,
	"compat43":                false,
	"compat44":                false,
	"complete_fullquote":      false,
	"direxpand":               false,
	"dirspell":                false,
	"execfail":                false,
	"extdebug":                false,
	"extquote":                true,
	"failglob":                false,
	"force_fignore":           false,
	"globasciiranges":         true,
	"globskipdots":            true,
	"gnu_errfmt":              false,
	"histappend":              false,
	"histreedit":              false,
	"histverify":              false,
	"hostcomplete":            false,
	"huponexit":               false,
	"inherit_errexit":         false,
	"interactive_comments":    true,
	"lastpipe":                false,
	"lithist":                 false,
	"localvar_inherit":        false,
	"localvar_unset":          false,
	"login_shell":             false,
	"mailwarn":                false,
	"no_empty_cmd_completion": false,
	"noexpand_translation":    false,
	"patsub_replacement":      false,
	"progcomp":                false,
	"progcomp_alias":          false,
	"promptvars":              true,
	"restricted_shell":        false,
	"shift_verbose":           false,
	"sourcepath":              true,
	"varredir_close":          false,
	"xpg_echo":                false,
}

const shoptUsage = "shopt: usage: shopt [-pqsu] [-o] [optname ...]"

// shoptState answers whether a name is on, and whether it is a name at all.
func shoptState(r *interp.Runner, name string) (on, known bool) {
	if mode, ok := shoptModes[name]; ok {
		return r.MatchOption(mode), true
	}
	if sw, ok := shoptSwitches[name]; ok {
		return sw.get(r), true
	}
	on, known = shoptStates[name]
	return on, known
}

// shoptNames is every name, sorted, for the listings.
func shoptNames() []string {
	names := make([]string, 0, len(shoptModes)+len(shoptSwitches)+len(shoptStates))
	for n := range shoptModes {
		names = append(names, n)
	}
	for n := range shoptSwitches {
		names = append(names, n)
	}
	for n := range shoptStates {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// printShopt writes one name in the format asked for: the reissuable
// `shopt -s name` under -p, and otherwise the two-column form the listing
// uses — the name padded to the width of the longest, then a tab.
func printShopt(r *interp.Runner, name string, on, reissuable bool) {
	if reissuable {
		flag := "-u"
		if on {
			flag = "-s"
		}
		_, _ = fmt.Fprintf(r.Out(), "shopt %s %s\n", flag, name)
		return
	}
	state := "off"
	if on {
		state = "on"
	}
	_, _ = fmt.Fprintf(r.Out(), "%-20s\t%s\n", name, state)
}

func biShopt(r *interp.Runner, ctx context.Context, args []string) int {
	var set, unset, quiet, reissue, setO bool
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if len(a) < 2 || a[0] != '-' {
			break
		}
		for _, c := range a[1:] {
			switch c {
			case 's':
				set = true
			case 'u':
				unset = true
			case 'q':
				quiet = true
			case 'p':
				reissue = true
			case 'o':
				setO = true
			default:
				r.Diagnosef("shopt: -%c: invalid option\n", c)
				_, _ = fmt.Fprintln(r.Err(), shoptUsage)
				return 2
			}
		}
	}
	names := args[i:]
	if set && unset {
		r.Diagnosef("shopt: cannot set and unset shell options simultaneously\n")
		return 1
	}
	if setO {
		return shoptSetO(r, ctx, names, set, unset)
	}
	if set || unset {
		if len(names) == 0 {
			// `shopt -s` alone lists what is on; `-u` what is off.
			if !quiet {
				for _, n := range shoptNames() {
					if on, _ := shoptState(r, n); on == set {
						printShopt(r, n, on, reissue)
					}
				}
			}
			return 0
		}
		return shoptApply(r, names, set)
	}
	// A query. With no names it is the full listing, and always succeeds;
	// with names the status says whether every one of them is on.
	if len(names) == 0 {
		if !quiet {
			for _, n := range shoptNames() {
				on, _ := shoptState(r, n)
				printShopt(r, n, on, reissue)
			}
		}
		return 0
	}
	status := 0
	for _, name := range names {
		on, known := shoptState(r, name)
		if !known {
			r.Diagnosef("shopt: %s: invalid shell option name\n", name)
			status = 1
			continue
		}
		if !on {
			status = 1
		}
		if !quiet {
			printShopt(r, name, on, reissue)
		}
	}
	return status
}

// shoptApply sets or unsets each name, carrying on past a failure the way the
// measured shell does: the names it knows are switched even when one it does
// not sits between them.
func shoptApply(r *interp.Runner, names []string, on bool) int {
	status := 0
	for _, name := range names {
		if mode, ok := shoptModes[name]; ok {
			r.SetMatchOption(mode, on)
			continue
		}
		if sw, ok := shoptSwitches[name]; ok {
			sw.set(r, on)
			continue
		}
		if held, ok := shoptStates[name]; ok {
			if held == on {
				// Already in the state being asked for: granted.
				continue
			}
			r.Diagnosef("shopt: %s: not implemented\n", name)
			status = 1
			continue
		}
		r.Diagnosef("shopt: %s: invalid shell option name\n", name)
		status = 1
	}
	return status
}

// shoptSetO is the `-o` face, which acts on the `set -o` names rather than
// on this builtin's own. Setting and unsetting are handed to the `set`
// builtin itself — the two tables must never drift apart, and delegation is
// what prevents it. Listing those options through here is not implemented
// yet; the core keeps no readable state for the ones it declines, so there
// is nothing honest to print.
func shoptSetO(r *interp.Runner, ctx context.Context, names []string, set, unset bool) int {
	if !set && !unset || len(names) == 0 {
		r.Diagnosef("shopt: -o without -s or -u and a name is not implemented yet\n")
		return 2
	}
	setBuiltin, ok := r.Builtin("set")
	if !ok {
		r.Diagnosef("shopt: -o: set builtin is not available\n")
		return 2
	}
	flag := "-o"
	if unset {
		flag = "+o"
	}
	status := 0
	for _, name := range names {
		if code := setBuiltin(r, ctx, []string{flag, name}); code != 0 {
			status = code
		}
	}
	return status
}
