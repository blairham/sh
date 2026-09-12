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
	// The one name in this table that is **on** with nothing said, which is
	// why Configure sets it and the rest do not: an `&` in a
	// `${v/pat/rep}` replacement is the text the pattern matched here, and
	// `shopt -u patsub_replacement` is what turns that back off. It sat in
	// shoptStates reporting off until the reading existed to gate — #1712
	// left it there deliberately rather than flip a flag over behavior
	// nothing provided, and this is the other half of that (#1862).
	"patsub_replacement": interp.ReplacementAmpersandIsTheMatch,
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
// The seven below were in shoptStates until #1445 and #1562, refused there for
// the only reason a refusal is allowed here: nothing implemented them. Each
// names a capability the core now holds, and in every case the *capability*
// is in interp or repl and only the *spelling* is here — `autocd` and
// `checkjobs` are zsh's names for two of the same behaviors as well, and the
// two dialects wire one switch apiece rather than each carrying a copy.
//
// All seven are interactive-only in bash too — one needs a window, one needs
// a prompt loop, three need a Tab, one needs a `cd` somebody typed and one
// needs an `exit` there is somebody to refuse — so none of this was askable
// on the `-c` route; the observables below were measured through a
// pseudo-terminal against bash 5.3.15 on 2026-09-08.
//
//   - checkwinsize. $LINES and $COLUMNS follow the window. Measured: bash
//     reports `COLUMNS=80 LINES=24` at its first prompt on an 80x24 terminal
//     and `132`/`40` at the next prompt after a resize; ours reported both
//     unset throughout, which is what #1429 refused it on. repl asks the
//     terminal once per prompt and assigns them — see repl.Shell's
//     trackWindowSize. Not exported, which is measured too.
//
//   - autocd. A bare directory name is a `cd`. Measured: `subdir` moves
//     there and writes `cd -- subdir`, a *directory* named `echo` does not
//     shadow the `echo` builtin, and `nosuchdir` is still `command not
//     found`. See interp.Runner.autoCdInstead, which is where all three of
//     those conditions live.
//
//   - cdspell. An interactive `cd` corrects a misspelled operand instead of
//     refusing it. Measured: bash corrects one edit per component — a
//     transposition, a dropped letter, an extra one or a wrong one — prints
//     the corrected operand on **stdout** and then moves, and refuses two
//     edits even in a nine-character name, so the threshold is flat rather
//     than scaled by length. See interp.Runner.correctPath, which is written
//     once so that `dirspell` reaches the same corrector rather than growing
//     a second.
//
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
//
//   - dirspell and direxpand, which are one row and not two. `dirspell` asks
//     the completer for `cdspell`'s correction, and the measurement that
//     matters is what it takes to see the correction *at all*. In a directory
//     holding `documents/target-file.txt`, with the options set from an rc
//     file so readline had them at initialization:
//
//     shopt -s …          ls documnets/tar<TAB>
//     (neither)           the bell, and the line as typed
//     dirspell            the bell, and the line as typed
//     direxpand           the bell, and the line as typed
//     dirspell direxpand  /…/documents/target-file.txt
//
//     `ls documents/tar<TAB>` completes in every one of those sessions, so
//     the first three rows are a correction with nowhere to go rather than a
//     correction that did not happen. That is what the two names divide
//     between them: `dirspell` decides which directory is *read* and
//     `direxpand` decides whether the directory that was read is what gets
//     *written*, and this shell keeps the typed text unless asked. So the
//     names are wired as a pair, and neither is a promise the other has to
//     keep. The corrector is interp.Runner.correctPath, reached here through
//     CorrectedDirectory, and the retry is in repl — see
//     shellCompleter.corrected, which carries the rest of the panel.
//
//     Two further measurements, because both bound what was built. The
//     correction is of the *directory* portion only: `documnets<TAB>` with
//     no trailing slash rings the bell in every configuration. And what
//     `direxpand` writes back for a corrected directory is an **absolute**
//     path with `..` collapsed — `documents/../documnets/` and `documnets/`
//     complete to the same `/…/documents/target-file.txt` — where `cdspell`
//     shows a person the operand in the shape it was typed. One corrector,
//     two shapes of answer.
//
//     What this shell still does not do is the *other* half of `direxpand`,
//     and it is not a half of either name: bash reads the directory portion
//     after parameter expansion whether the option is on or off — with it
//     off `${HOME}/docum<TAB>` completes to `${HOME}/documents/` and with it
//     on to the expanded path — and this completer does not expand
//     parameters at all, so it offers nothing for that word in either state.
//     That is an absence with no option over it, present with both names off,
//     and it is #1574 rather than anything these two promise.
//
//   - no_empty_cmd_completion. The one most likely to be misread, because the
//     name is a negative and the honest answer used to look like the lazy
//     one. It asks that completion on an empty command word *not* search
//     PATH. This shell searched it, so the shell sat in the option's off
//     state and `shopt -s` was a request to change behavior rather than to
//     confirm it — which is why it could not be granted by writing `true`
//     into a table. The core switch is positive
//     (interp.Runner.CompletesEmptyCommandWord) and the inversion happens
//     here, in the one place the name's sense is decided.
//
// `lastpipe` is the ninth entry and belongs to neither group above. It is not
// interactive-only — it is the one name here a *script* sets and immediately
// depends on — and what it moves is a semantics axis rather than a capability.
// Its own comment on the entry carries the measurement; #2361 is the issue.
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
	"dirspell": {
		get: (*interp.Runner).CorrectsCompletionSpelling,
		set: (*interp.Runner).SetCorrectsCompletionSpelling,
	},
	"direxpand": {
		get: (*interp.Runner).ExpandsCompletedDirectory,
		set: (*interp.Runner).SetExpandsCompletedDirectory,
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
	// The one name in this table that moves a *semantics axis* rather than a
	// capability, and the reason it is a switch at all: where the last element
	// of a pipeline runs is
	// interp.Semantics.LastPipelineElementInCurrentShell, answered `No` by
	// this preset and by dash and `Yes` by ksh93 and zsh — and this is the
	// only shell in the panel that lets a script move it. So the axis says
	// where the shell stands and the switch says whether the script asked for
	// the other side; nothing here is a second implementation of the pipeline.
	//
	// It sat in shoptStates refusing the write until #2361, which is the one
	// refusal in that table that could actively mislead: a script sets the
	// option precisely so that `cmd | read v` and `cmd | while read; do …;
	// done` leave something behind, and a quiet `shopt` over an unmoved
	// pipeline would have read an empty variable with nothing on standard
	// error. That is why the name could not be closed by widening a table.
	"lastpipe": {
		get: (*interp.Runner).KeepsLastPipelineElement,
		set: (*interp.Runner).SetKeepsLastPipelineElement,
	},
}

// shoptReadOnly are the two names that are indicators rather than switches:
// bash lists them, answers what the shell *is*, and takes a request to change
// either one without doing anything about it.
//
// Measured 2026-09-10 across all sixty names bash 5.3.15 lists — `shopt -s`,
// query, `shopt -u`, query, a name at a time — and again against bash 3.2.57.
// Every other name moves. These two report 0 with nothing on standard error
// and stay exactly where they were: `shopt -s login_shell` in a shell that is
// not one leaves it off, and `shopt -u login_shell` in a login shell leaves it
// on. The two bashes agree, so it is not an axis.
//
// A category of their own rather than an entry in either table beside them.
// shoptStates refuses a write it cannot honor, out loud and at 1, which is
// right for a behavior this shell has not built and wrong here — bash promises
// nothing either. shoptSwitches is a name wired to a real switch, and a
// "switch" whose setter is an empty function reads to the next person as an
// oversight rather than as the measurement it is. What these two share is that
// the *request* is taken and ignored, which is what this table says.
//
// It matters because both spellings come back out of `shopt -p` for an agent
// harness to source: a login shell dumps `shopt -s login_shell`, and the shell
// that wrote the line has to be able to read it back (#1709). `login_shell`
// arrived here from shoptStates, where a static `false` made it wrong twice
// over — it read `off` inside a login shell as well as refusing the write.
// `restricted_shell` was still in that table, refused at 1 where bash says
// nothing and answers 0.
//
// The state is read live rather than stored, because one of them is a fact
// about this invocation that the front end carried in — the same
// interp.Runner.LoginShell that answers the `$-` question next door, where
// bash says *no* (Semantics.LoginShowsLInDollarDash) precisely because it
// keeps the answer here instead.
var shoptReadOnly = map[string]func(*interp.Runner) bool{
	"login_shell": func(r *interp.Runner) bool { return r.LoginShell },
	// Nothing here starts a restricted shell, so the indicator tells the truth
	// by reading off — the same honesty shoptStates keeps, without the
	// refusal, because bash refuses nothing here either.
	"restricted_shell": func(*interp.Runner) bool { return false },
}

// shoptRecorded are the names bash lists whose whole effect is on a line
// somebody is completing, in ways this shell's completer does not provide.
// They are reported at **bash's own default**, remembered when moved, and
// acted on by nothing.
//
// It is the same bargain the zsh dialect's `recorded(…)` strikes over 141 of
// its 185 names, arrived at here for the same reason and from the other end.
// `shopt -p` is a **capture surface**: an agent harness snapshots a shell
// with it and sources the result back ahead of every later command, so a
// state this shell reports wrong is re-applied to every command it runs — and
// a state *bash* reported and this shell will not read back is a complaint on
// standard error ahead of each of them. Measured 2026-09-10: a real bash
// 5.3.15's own `shopt -p`, sourced into this shell, wrote seven
// `not implemented` lines (#1712).
//
// Three names rather than the whole table, because recording is a cost and
// not a free win — a recorded name reports a state nothing keeps. These three
// earn it on one test: the option decides what a *completer* offers, and
// there is nothing else a script can ask it.
//
//   - force_fignore. bash's on state is that a word FIGNORE names is left out
//     even when it is the only completion. This shell has no FIGNORE, so no
//     word is ever named and there is no last resort to offer; the difference
//     the option describes has nowhere to appear.
//   - hostcomplete. Completing a word containing `@` against a host list.
//     This completer reads no host list.
//   - progcomp. Using the specifications `complete` registers. This shell
//     keeps every spec verbatim and prints it back — see
//     interp/completebuiltin.go, which exists because a bash_completion.d
//     file dies at 127 otherwise — and the completer consults none of them.
//
// `complete_fullquote` is *not* here, and the difference is the point of
// having the category at all: this shell really does backslash every shell
// metacharacter in a completed name — see escapeName in repl/completeword.go
// — so bash's on state is this implementation's state and it belongs in
// shoptStates below with the rest of what is true.
var shoptRecorded = map[string]bool{
	"force_fignore": true,
	"hostcomplete":  true,
	"progcomp":      true,
}

// shoptRecordedStore is where a moved recorded name is kept: an array under a
// name no script can reach, which is the shape the zsh dialect's own recorded
// store uses and for the same reasons — a subshell deep-copies the variable
// table, so `(shopt -u progcomp)` stays in the subshell.
//
// Deviations rather than states, so a shell that has never run `shopt` on one
// of these holds an empty array and every name reads back at bash's default.
const shoptRecordedStore = ".bash.shopt"

// shoptRecordedState reads one recorded name.
func shoptRecordedState(r *interp.Runner, name string, def bool) bool {
	names, _ := r.GetArray(shoptRecordedStore)
	for _, n := range names {
		if n == name {
			return !def
		}
	}
	return def
}

// shoptSetRecorded records or clears one name's deviation, keeping the store
// sorted so it is a function of the set rather than of the order an rc file
// happened to write.
func shoptSetRecorded(r *interp.Runner, name string, on, def bool) {
	names, _ := r.GetArray(shoptRecordedStore)
	out := make([]string, 0, len(names)+1)
	for _, n := range names {
		if n != name {
			out = append(out, n)
		}
	}
	if on != def {
		out = append(out, name)
		sort.Strings(out)
	}
	r.SetArray(shoptRecordedStore, out)
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
// Every name #1429 and #1445 collected is now wired; nothing in the table
// below is a behavior somebody asked for and did not get. The last two,
// `dirspell` and `direxpand`, are in shoptSwitches above.
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
	"complete_fullquote":   true,
	"execfail":             false,
	"extdebug":             false,
	"extquote":             true,
	"failglob":             false,
	"globasciiranges":      true,
	"globskipdots":         true,
	"gnu_errfmt":           false,
	"histappend":           true,
	"histreedit":           false,
	"histverify":           false,
	"huponexit":            false,
	"inherit_errexit":      false,
	"interactive_comments": true,
	"lithist":              true,
	"localvar_inherit":     false,
	"localvar_unset":       false,
	"mailwarn":             false,
	"noexpand_translation": false,
	"progcomp_alias":       false,
	"promptvars":           true,
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
	if ro, ok := shoptReadOnly[name]; ok {
		return ro(r), true
	}
	if def, ok := shoptRecorded[name]; ok {
		return shoptRecordedState(r, name, def), true
	}
	on, known = shoptStates[name]
	return on, known
}

// shoptNames is every name, sorted, for the listings.
func shoptNames() []string {
	names := make([]string, 0,
		len(shoptModes)+len(shoptSwitches)+len(shoptReadOnly)+len(shoptRecorded)+len(shoptStates))
	for n := range shoptModes {
		names = append(names, n)
	}
	for n := range shoptSwitches {
		names = append(names, n)
	}
	for n := range shoptReadOnly {
		names = append(names, n)
	}
	for n := range shoptRecorded {
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
		if _, ok := shoptReadOnly[name]; ok {
			// An indicator: the request is taken, nothing moves, and nothing
			// is said. Measured, and it is bash's answer whichever way the
			// name already reads — see shoptReadOnly.
			continue
		}
		if def, ok := shoptRecorded[name]; ok {
			shoptSetRecorded(r, name, on, def)
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
