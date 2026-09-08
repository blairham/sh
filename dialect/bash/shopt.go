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
// prompt expands parameters, `.` searches PATH, and the three history names
// below describe the history this shell already keeps.
//
// Which side a name falls on is measured, never assumed, and the three
// history names were measured through a terminal rather than through `-c`:
// `-i -c` runs one command and exits without entering the prompt loop, so it
// never reaches the editor that keeps the history at all. The observables,
// against bash 5.3.15 driven the same way:
//
//   - histappend. Ours appends and never rewrites: repl/history.go opens the
//     file O_APPEND|O_CREATE|O_WRONLY and writes only the lines this session
//     added. Measured — a line another process appended to HISTFILE while a
//     session was live was still there after that session exited, with our
//     lines after it. A writer that rewrote the file from its own list would
//     have dropped it.
//   - cmdhist. A command typed over four lines is one entry here, not four:
//     typing a `for` loop and pressing Up recalls the whole loop. bash with
//     `shopt -u cmdhist` recalls only `done`.
//   - lithist. That one entry keeps its newlines rather than being joined
//     with semicolons: ours recalls `for q in ZZ`, `do`, `echo MARK$q`,
//     `done` across four lines, byte-for-byte what bash under
//     `shopt -s lithist` recalls, where bash's default recalls the
//     semicolon-joined `for q in ZZ; do echo MARK$q; done`.
//
// So turning those three *on* is a request that has been granted, and
// turning one *off* is refused: this shell cannot promise to rewrite the
// history file, to split a construct into a line each, or to join one with
// semicolons.
//
// checkwinsize is the one the issue that prompted this expected to be true
// and it is not, which is why it is measured here and not reasoned about.
// The name asks for LINES and COLUMNS to be updated, and this shell never
// assigns either: under a terminal resized from 80x24 to 132x40, bash moves
// `$COLUMNS` and `$LINES` with it and ours reports both unset throughout.
// The editor's own width is always current — repl/winsize_unix.go asks the
// terminal on every draw — but that is not the variable the name promises,
// and granting it would tell a startup file that `$COLUMNS` tracks the
// window when nothing here makes it.
//
// The other five refusals are refusals for the same reason, and each is
// written down because a bare `false` in the table below reads like a TODO
// and the wrong repair for a TODO is to flip it. Every one of these is the
// state this shell is genuinely in, so the refusal is the correct answer and
// not a gap left in the table:
//
//   - no_empty_cmd_completion is the one most likely to be misread, because
//     the name is a negative and the honest answer looks like the lazy one.
//     It asks that completion on an *empty* line not search PATH. Ours
//     searches it — repl's completer answers an empty command word with every
//     builtin, function, reserved word and executable it can reach — so this
//     shell is in the option's *off* state and `shopt -s` is a request to
//     change behavior, not to confirm it. Flipping this to true would claim a
//     quieter completion than this shell has.
//   - cdspell and dirspell ask for a misspelled path to be corrected, in `cd`
//     and in completion. Neither retries: `cd` goes from a failed stat
//     straight to the diagnostic, and the completer matches an exact prefix.
//   - autocd asks that a bare directory name be a `cd`. Command lookup ends
//     at "is a directory", which is the opposite of a fallback.
//   - checkjobs asks that exiting warn about *running* jobs, not only stopped
//     ones. The stopped-job hold exists; nothing counts the running ones.
//
// Four of those five are interactive-only in bash as well, so a `-c` probe
// cannot tell an implementation from an absence — it shows both shells doing
// nothing. They were measured through a terminal, which is the only place the
// question is askable. #1445 carries what each of the six would need.
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
	"cmdhist":                 true,
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
	"histappend":              true,
	"histreedit":              false,
	"histverify":              false,
	"hostcomplete":            false,
	"huponexit":               false,
	"inherit_errexit":         false,
	"interactive_comments":    true,
	"lastpipe":                false,
	"lithist":                 true,
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

// The two column widths this builtin writes in, and they really are two.
//
// A `shopt` row is padded to shoptListingWidth, whether the name came from
// this builtin's own table or was named after `-o`. A listing `-o` produces
// with *nothing* named is `set -o`'s listing at `set -o`'s width, because
// that is what it is: measured in bash 5.3.15, `shopt -o` is byte-for-byte
// `set -o` and `shopt -o -s` is that listing with the off rows dropped.
//
// setOptionListingWidth is the same number Diagnostics.OptionListingWidth
// carries, named once and read in both places so the filtered listing and
// the unfiltered one cannot drift apart.
const (
	shoptListingWidth     = 20
	setOptionListingWidth = 15
)

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
	_, _ = fmt.Fprintf(r.Out(), "%-*s\t%s\n", shoptListingWidth, name, state)
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
		return shoptSetO(r, ctx, names, set, unset, quiet, reissue)
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
// what prevents it.
//
// Reading is the same principle with the same seam. A named option's state
// comes from interp.Runner.NamedOption, which is the live answer `set -o`
// itself prints, and the two whole-table listings are handed to `set` for
// the reason the writes are: measured, `shopt -o` is byte-for-byte `set -o`
// and `shopt -o -p` is byte-for-byte `set +o` in bash 5.3.15, so printing
// them here again would be a second copy of a format that is already the
// dialect's Diagnostics answer.
//
// What `-o` does *not* share with the rest of this builtin is the name
// space: `shopt -o cdspell` is an error in bash even though `shopt cdspell`
// is fine, and the wording differs by a word — `invalid option name` here
// against `invalid shell option name` there. Both are measured.
func shoptSetO(r *interp.Runner, ctx context.Context, names []string, set, unset, quiet, reissue bool) int {
	if set || unset {
		if len(names) == 0 {
			// `shopt -o -s` alone lists the `set -o` options that are on and
			// `-o -u` those that are off. Narrowing is why interp exposes
			// the rows rather than only the formatted listing: `set` prints
			// them all, and filtering its output would mean parsing text.
			if !quiet {
				for _, row := range r.ListedOptions() {
					if row.On == set {
						printSetO(r, row.Name, row.On, reissue, setOptionListingWidth)
					}
				}
			}
			return 0
		}
		return shoptMoveO(r, ctx, names, unset)
	}
	if len(names) == 0 {
		if quiet {
			// Nothing named and nothing to print, and bash still succeeds.
			return 0
		}
		// The whole table, in `set`'s own words. `-p` is `set +o`.
		return runSet(r, ctx, reissue)
	}
	// Named options, read one at a time. The status is whether every one of
	// them is on, which is the whole point of the spelling: it is how a
	// script tests one option without parsing `$SHELLOPTS`.
	status := 0
	for _, name := range names {
		on, known := r.NamedOption(name)
		if !known {
			r.Diagnosef("shopt: %s: invalid option name\n", name)
			status = 1
			continue
		}
		if !on {
			status = 1
		}
		if !quiet {
			printSetO(r, name, on, reissue, shoptListingWidth)
		}
	}
	return status
}

// shoptMoveO is the writing half, handed to `set` a name at a time.
func shoptMoveO(r *interp.Runner, ctx context.Context, names []string, unset bool) int {
	status := 0
	for _, name := range names {
		if code := runSet(r, ctx, unset, name); code != 0 {
			status = code
		}
	}
	return status
}

// runSet hands one `set -o` or `set +o` to the `set` builtin itself.
//
// One function rather than a line at each of the three call sites — the two
// writing ones and the whole-table listing — because they are the same
// delegation for the same reason, and this file has three of them only
// because `-o` reads and writes through the same namespace. A second copy
// is how the plus-and-minus choice, or the missing-builtin case, ends up
// answered one way in one branch and another way in the next.
func runSet(r *interp.Runner, ctx context.Context, plus bool, names ...string) int {
	setBuiltin, ok := r.Builtin("set")
	if !ok {
		r.Diagnosef("shopt: -o: set builtin is not available\n")
		return 2
	}
	flag := "-o"
	if plus {
		flag = "+o"
	}
	return setBuiltin(r, ctx, append([]string{flag}, names...))
}

// printSetO writes one `set -o` name the way `-o` asks for it, which is not
// quite how the same builtin writes one of its own names.
//
// Measured in bash 5.3.15, and the two widths really are two: a named row
// under `-o` is padded to shoptListingWidth like every other row this
// builtin prints, while `shopt -o` with nothing named is `set -o`'s listing
// at `set -o`'s narrower width. Under `-p` it is a re-inputtable `set` line
// rather than a `shopt` one, since `shopt -s vi` would not put it back.
func printSetO(r *interp.Runner, name string, on, reissuable bool, width int) {
	if reissuable {
		sign := "+"
		if on {
			sign = "-"
		}
		_, _ = fmt.Fprintf(r.Out(), "set %so %s\n", sign, name)
		return
	}
	state := "off"
	if on {
		state = "on"
	}
	_, _ = fmt.Fprintf(r.Out(), "%-*s\t%s\n", width, name, state)
}
