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
// the matcher — the second kind of "really implemented".
//
// `expand_aliases` is the option a bash script has to set before an alias
// means anything, which is why an alias case could not be written for bash at
// all while this was refused (#632). It is implemented rather than recorded:
// the alias is expanded afterwards and not expanded before, and `shopt -u`
// mid-script stops it again — measured in bash 5.3 and bash 3.2 alike, on all
// three non-interactive routes.
//
// The five below were in shoptStates until #1445, refused there for the only
// reason a refusal is allowed here: nothing implemented them. Each names a
// capability the core now holds, and in every case the *capability* is in
// interp or repl and only the *spelling* is here — `autocd` and `checkjobs`
// are zsh's names for two of the same behaviors as well, and the two dialects
// wire one switch apiece rather than each carrying a copy.
//
// All five are interactive-only in bash too — one needs a window, one needs a
// prompt loop, one needs a Tab, one needs a `cd` somebody typed and one needs
// an `exit` there is somebody to refuse — so none of this was askable on the
// `-c` route; the observables below were measured through a pseudo-terminal
// against bash 5.3.15 on 2026-09-08.
//
//   - checkwinsize. $LINES and $COLUMNS follow the window. Measured: bash
//     reports `COLUMNS=80 LINES=24` at its first prompt on an 80x24 terminal
//     and `132`/`40` at the next prompt after a resize; ours reported both
//     unset throughout, which is what #1429 refused it on. repl asks the
//     terminal once per prompt and assigns them — see repl.Shell's
//     trackWindowSize. Not exported, which is measured too.
//   - autocd. A bare directory name is a `cd`. Measured: `subdir` moves
//     there and writes `cd -- subdir`, a *directory* named `echo` does not
//     shadow the `echo` builtin, and `nosuchdir` is still `command not
//     found`. See interp.Runner.autoCdInstead, which is where all three of
//     those conditions live.
//   - cdspell. An interactive `cd` corrects a misspelled operand instead of
//     refusing it. Measured: bash corrects one edit per component — a
//     transposition, a dropped letter, an extra one or a wrong one — prints
//     the corrected operand on **stdout** and then moves, and refuses two
//     edits even in a nine-character name, so the threshold is flat rather
//     than scaled by length. See interp.Runner.correctPath, which is written
//     once so that `dirspell` reaches the same corrector rather than growing
//     a second.
//   - checkjobs. Exiting warns about jobs that are still *running*, and
//     follows the sentence with the job table. Measured through a
//     pseudo-terminal: with the option on, `sleep 40 &` then `exit` writes
//     `There are running jobs.`, then `[1]+  Running   sleep 40 &`, then a
//     fresh prompt, and a second `exit` leaves; with it off the first `exit`
//     leaves. The stopped-job warning this shell already gave is *not* the
//     option's — `shopt -u checkjobs` still says `There are stopped jobs.` —
//     and a job of each kind is reported as stopped. See
//     interp.Runner.ChecksRunningJobsAtExit, which zsh reaches under the same
//     name from the other option namespace.
//   - no_empty_cmd_completion. The one most likely to be misread, because the
//     name is a negative and the honest answer used to look like the lazy
//     one. It asks that completion on an empty command word *not* search
//     PATH. This shell searched it, so the shell sat in the option's off
//     state and `shopt -s` was a request to change behavior rather than to
//     confirm it — which is why it could not be granted by writing `true`
//     into a table. The core switch is positive
//     (interp.Runner.CompletesEmptyCommandWord) and the inversion happens
//     here, in the one place the name's sense is decided.
var shoptSwitches = map[string]struct {
	get func(*interp.Runner) bool
	set func(*interp.Runner, bool)
}{
	"expand_aliases": {
		get: (*interp.Runner).AliasExpansion,
		set: (*interp.Runner).SetAliasExpansion,
	},
	"checkwinsize": {
		get: (*interp.Runner).TracksWindowSize,
		set: (*interp.Runner).SetTracksWindowSize,
	},
	"autocd": {
		get: (*interp.Runner).AutoCd,
		set: (*interp.Runner).SetAutoCd,
	},
	"cdspell": {
		get: (*interp.Runner).CorrectsCdSpelling,
		set: (*interp.Runner).SetCorrectsCdSpelling,
	},
	"checkjobs": {
		get: (*interp.Runner).ChecksRunningJobsAtExit,
		set: (*interp.Runner).SetChecksRunningJobsAtExit,
	},
	// The inverted one, and the only entry in this file that is not a
	// straight pair. bash names the *suppression*, so the option being on is
	// the capability being off; writing it the other way round would have
	// `shopt -s no_empty_cmd_completion` ask for more completion rather than
	// less.
	"no_empty_cmd_completion": {
		get: func(r *interp.Runner) bool { return !r.CompletesEmptyCommandWord() },
		set: func(r *interp.Runner, on bool) { r.SetCompletesEmptyCommandWord(!on) },
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
// One of the names #1445 collected is still refused, and it is refused for
// the only reason a refusal is allowed here: nothing implements the behavior.
// It is written down because a bare `false` in the table below reads like a
// TODO and the wrong repair for a TODO is to flip it into a lie.
//
//   - dirspell asks for `cdspell`'s correction during *completion*, and the
//     measurement that matters is what it takes to see it happen at all.
//     Driven through a pseudo-terminal against bash 5.3.15 on 2026-09-08,
//     against a directory really called `documents`: with `dirspell` alone,
//     `ls documnets/tar<TAB>` rings the bell and leaves the line as typed,
//     and so do `ls documnets/<TAB>` and `cd documnets/<TAB>`. With
//     `direxpand` alone, the same three do nothing. With **both** on, the
//     same keystrokes rewrite the line to the completed
//     `…/documents/target-file.txt`. The control is in every one of those
//     sessions: `ls documents/tar<TAB>` completes correctly, so the tree and
//     the completer were right and only the correction was missing.
//
//     So this name cannot be granted on its own — it is visible only beside
//     `direxpand`, which this shell does not do either, and which is a
//     second behavior rather than a second spelling of this one. Correcting
//     here without it would be correcting where bash does not. The corrector
//     itself is already written and already shared:
//     interp.Runner.correctPath, which `cdspell` reaches. What is missing is
//     the completer asking it, and `direxpand` beside it. See #1562, which
//     carries the table above and what each half would take.
//
// It is interactive-only in bash as well, so a `-c` probe cannot tell an
// implementation from an absence — it shows both shells doing nothing. It was
// measured through a terminal, which is the only place the question is
// askable.
var shoptStates = map[string]bool{
	"array_expand_once":    false,
	"assoc_expand_once":    false,
	"bash_source_fullpath": false,
	"cdable_vars":          false,
	"checkhash":            false,
	"cmdhist":              true,
	"compat31":             false,
	"compat32":             false,
	"compat40":             false,
	"compat41":             false,
	"compat42":             false,
	"compat43":             false,
	"compat44":             false,
	"complete_fullquote":   false,
	"direxpand":            false,
	"dirspell":             false,
	"execfail":             false,
	"extdebug":             false,
	"extquote":             true,
	"failglob":             false,
	"force_fignore":        false,
	"globasciiranges":      true,
	"globskipdots":         true,
	"gnu_errfmt":           false,
	"histappend":           true,
	"histreedit":           false,
	"histverify":           false,
	"hostcomplete":         false,
	"huponexit":            false,
	"inherit_errexit":      false,
	"interactive_comments": true,
	"lastpipe":             false,
	"lithist":              true,
	"localvar_inherit":     false,
	"localvar_unset":       false,
	"login_shell":          false,
	"mailwarn":             false,
	"noexpand_translation": false,
	"patsub_replacement":   false,
	"progcomp":             false,
	"progcomp_alias":       false,
	"promptvars":           true,
	"restricted_shell":     false,
	"shift_verbose":        false,
	"sourcepath":           true,
	"varredir_close":       false,
	"xpg_echo":             false,
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
