// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
//
// A name carries a *list* of core options rather than one, because this
// dialect's spelling is not always one behavior: `nocasematch` folds the
// pattern operators of `[[ ]]` and `case` **and** the regular expression one,
// which the core keeps apart because zsh's option of the same name turns only
// the second on. Measured 2026-09-13 — see interp.RegexFoldsCase for the
// grid. The first entry is the one the listing reads, which is exact while
// this builtin is the only thing that moves either bit.
var shoptModes = map[string][]interp.MatchOption{
	"nullglob":    {interp.UnmatchedPatternIsEmpty},
	"dotglob":     {interp.PatternsMatchHidden},
	"nocaseglob":  {interp.GlobFoldsCase},
	"nocasematch": {interp.MatchFoldsCase, interp.RegexFoldsCase},
	"globstar":    {interp.StarStarCrossesDirectories},
	"extglob":     {interp.QuantifiedGroupsEverywhere},
	// The one name in this table that is **on** with nothing said, which is
	// why Configure sets it and the rest do not: an `&` in a
	// `${v/pat/rep}` replacement is the text the pattern matched here, and
	// `shopt -u patsub_replacement` is what turns that back off. It sat in
	// shoptStates reporting off until the reading existed to gate — #1712
	// left it there deliberately rather than flip a flag over behavior
	// nothing provided, and this is the other half of that (#1862).
	"patsub_replacement": {interp.ReplacementAmpersandIsTheMatch},
	// The third answer a script can ask for about a pattern that matched
	// nothing, beside leaving it in place and `nullglob` deleting it.
	// Measured 2026-09-13 on bash 5.3.15, in a directory holding `a1` and
	// `a2`:
	//
	//	shopt -s failglob; echo nosuch*     `no match: nosuch*`, and the
	//	                                    command does not run
	//	shopt -s failglob; echo a*          `a1 a2`
	//	shopt -s nullglob failglob; …       the complaint, in either order
	//
	// The last of those is why it is a match option of its own rather than a
	// second state of `nullglob`'s: bash refuses with both set where zsh
	// deletes the word with `nullglob` beside `nomatch`, so one rule cannot
	// serve both. interp.UnmatchedPatternIsError carries the measurement.
	//
	// How far the refusal reaches is the ordinary failed-expansion question
	// and is answered by this preset's FailedExpansionAbandonsTheLine, which
	// is `Yes` — so the statement is given up and the shell goes on to the
	// next one. Measured on the same binary: a script whose second line is
	// `echo nosuch*` prints the complaint and still runs its third line,
	// where a `-c` string is one statement and loses everything after the
	// semicolon.
	"failglob": {interp.UnmatchedPatternIsError},
}

// shoptSwitch is one name's two halves: what it reads now, and what setting
// or unsetting it does. Named rather than written inline at the map, so that
// an entry can be built by a helper — which is what the compatibility letters
// need, being seven spellings of one state (see compat.go).
type shoptSwitch struct {
	get func(*interp.Runner) bool
	set func(*interp.Runner, bool)
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
// `lastpipe`, `inherit_errexit` and `extdebug` are the last three entries and
// belong to neither group above. None of them is interactive-only — they are
// the names here a *script* sets and immediately depends on — and what they
// move is a semantics axis apiece and a pair of trap-carriage options rather
// than a capability. Their own comments on the entries carry the
// measurements; #2361, #3001 and #2426 are the issues.
var shoptSwitches = map[string]shoptSwitch{
	// The seven compatibility letters, which are one state under a second
	// spelling rather than seven switches: each reads and writes the level
	// `BASH_COMPAT` carries, so the two doors move together in both
	// directions. See compat.go, which holds the measurements and the reason
	// the letters stop at 44.
	"compat31": compatSwitch(31),
	"compat32": compatSwitch(32),
	"compat40": compatSwitch(40),
	"compat41": compatSwitch(41),
	"compat42": compatSwitch(42),
	"compat43": compatSwitch(43),
	"compat44": compatSwitch(44),
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
	// The one switch here that turns a *dialect answer* down rather than a
	// capability up. bash believes what its command hash holds, which is
	// Semantics.CommandHashIsTrusted; `checkhash` makes it look first, and
	// then a hashed path that has gone sends it back to PATH exactly as it
	// sends the other three. Measured 2026-09-13 with two copies of one name
	// on PATH: with the option off bash reports the remembered path at 127,
	// and with it on it runs the second copy.
	"checkhash": {
		get: (*interp.Runner).ChecksHashedCommand,
		set: (*interp.Runner).SetChecksHashedCommand,
	},
	"cdspell": {
		get: (*interp.Runner).CorrectsCdSpelling,
		set: (*interp.Runner).SetCorrectsCdSpelling,
	},
	// The other switch here that turns a *capability* down rather than up,
	// and the only name any shell on the panel has for the question: with
	// `sourcepath` off, a `.` operand with no slash in it is a path relative
	// to the shell's directory and is not looked for along $PATH. On by
	// default, which is the zero value the Runner stores (#3058).
	"sourcepath": {
		get: (*interp.Runner).SearchesPathForSource,
		set: (*interp.Runner).SetSearchesPathForSource,
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
	// extdebug, and it is two states under one name. bash's extended
	// debugging turns **both** trap-carriage options on with it — measured
	// in bash 5.3.15, `shopt -s extdebug` leaves `set -o` reporting
	// `errtrace on` and `functrace on` and `$-` carrying `E` and `T`, and
	// `shopt -u extdebug` puts both back off — which is what makes it the
	// other half of #2426: a DEBUG trap set beside it is exactly the trap
	// that has to enter the calls it is watching.
	//
	// The indicator bit is stored here rather than derived from the two
	// options because the three come apart, measured on the same binary:
	// `shopt -s extdebug; set +T` leaves `functrace off` with
	// `shopt -p extdebug` still writing `shopt -s extdebug`, and `set -T`
	// alone never turns the indicator on. So the options are the behavior
	// and the bit is bash's own answer about itself, which is the one thing
	// here the core has no state for.
	//
	// bash 3.2.57 moves the indicator alone and leaves both options off — a
	// change within bash rather than a difference between shells, so the
	// corpus's `bash32` column disagrees with the other two on purpose and
	// this is 5.3's answer.
	//
	// `declare -F` reporting a definition's line and file joined the two in
	// #2476, and it is the one of the five that is not a `set` option under
	// another name — see interp.Runner.LocatesFunctions. A DEBUG action's
	// status deciding what runs next joined them after it, and the
	// BASH_ARGC/BASH_ARGV record closes #2476's list: the arguments of each
	// call, kept while the option is on and read back under two names by
	// registerCallStack. See interp.Runner.SetRecordsCallArguments, which is
	// where turning the record on records the frame it was turned on in.
	//
	// What is still not here is what a *sourced file* does to that record,
	// and it is two rules of its own rather than a smaller version of this
	// one: measured 2026-09-14, `. ./s.sh x y` pushes a frame of two even
	// with extended debugging **off**, and `. ./s.sh` with no operands
	// pushes a frame of one holding the file's own name. Neither follows
	// from the option, so neither is modeled here.
	//
	// The entry keeps the same partial honesty `set -o posix` does — it
	// moves what was measured to move with it and promises nothing else —
	// rather than refusing the name outright, which is what left a debugging
	// script with the option off, `$?` at 1 and a trap that saw only the
	// call (#2426).
	"extdebug": {
		get: func(r *interp.Runner) bool { return shoptStoredState(r, "extdebug", false) },
		set: func(r *interp.Runner, on bool) {
			shoptSetStored(r, "extdebug", on, false)
			r.SetErrorTracing(on)
			r.SetFunctionTracing(on)
			r.SetLocatesFunctions(on)
			r.SetDebugActionDecides(on)
			r.SetRecordsCallArguments(on)
		},
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
	// A failed `exec` is an ordinary failed command here rather than the end
	// of the script, which is the one thing about `exec` this shell lets a
	// script move. Measured 2026-09-23 on bash 5.3.15:
	//
	//	shopt -s execfail; exec nosuchcmd42; echo "st=$?"   complaint, st=127
	//	shopt -s execfail; exec ./notexec;   echo "st=$?"   complaint, st=126
	//
	// The status is the one the exit would have carried and the wording does
	// not change, so this moves the *ending* and nothing else — see
	// interp.Runner.ExecFailureLeavesTheShellRunning, which is where the
	// EXIT-trap consequence is written down.
	//
	// It sat in shoptStates refusing the write, and that refusal was the
	// misleading kind: a script sets the option precisely so that a missing
	// command does not take the shell down with it, and a quiet refusal left
	// it taken down anyway with the line after the `exec` never reached
	// (#4149).
	"execfail": {
		get: (*interp.Runner).ExecFailureLeavesTheShellRunning,
		set: (*interp.Runner).SetExecFailureLeavesTheShellRunning,
	},
	// SIGHUP to the jobs this shell still has when it ends, for an
	// interactive login shell. Off with nothing said, and **both** conditions
	// are required — measured 2026-09-23 on bash 5.3.15 with a backgrounded
	// subshell holding a `HUP` trap:
	//
	//	-l -i, on      the EXIT trap's line, then the job's
	//	-l -i, off     the EXIT trap alone
	//	-i not login   the EXIT trap alone
	//	-l ./s.sh      the EXIT trap alone
	//
	// The last two are the rows that pin the conditions, and they had to be
	// run without a terminal to mean anything: `script -qec 'bash -l'` gives
	// the shell a pty and so makes it interactive by inference, which read as
	// "interactive is not required" until `$-` was asked and said otherwise.
	// See interp.Runner.SendsHangupToJobsAtExit.
	//
	// It sat in shoptStates refusing the write, and the refusal was the kind a
	// person notices later: the option is set in an rc file so that a session's
	// background work does not outlive the session, and a refused shell left
	// every job of every login session running (#4149).
	"huponexit": {
		get: (*interp.Runner).SendsHangupToJobsAtExit,
		set: (*interp.Runner).SetSendsHangupToJobsAtExit,
	},
	// Whether a command typed over several physical lines is **one** history
	// entry. On with nothing said, and the count question rather than the
	// separator one — `lithist` beside it decides what goes between the lines
	// of a single entry and is moot once this is off. Measured 2026-09-23 on
	// bash 5.3.15 through a pty, reading `history` rather than the file:
	//
	//	cmdhist on,  lithist off   one entry, `for i in 1 2; do   echo $i; done`
	//	cmdhist on,  lithist on    one entry, holding the newlines
	//	cmdhist off, either        four entries
	//
	// The file cannot tell the middle row from the last: one entry holding
	// newlines and four entries are the same bytes there, which is what the
	// first instrument read and why it reported the option doing nothing. See
	// interp.Runner.HistoryKeepsATypedCommandWhole.
	//
	// It sat in shoptStates reading **on** — this shell does keep one entry —
	// so `shopt -s` was a silent grant and `shopt -u` the refusal, leaving a
	// script that asked for a line at a time with a construct it could only
	// recall whole (#4149).
	"cmdhist": {
		get: (*interp.Runner).HistoryKeepsATypedCommandWhole,
		set: (*interp.Runner).SetHistoryKeepsATypedCommandWhole,
	},
	// A history expansion that **failed** is handed back to be edited rather
	// than thrown away. Off with nothing said, and it is `histverify` one case
	// over — that one is an expansion that changed the line, this one a
	// reference the list did not hold — so it reaches the same road.
	//
	// Measured 2026-09-23 through a pty on bash 5.3.15, typing
	// `!nosuchprefix` and then ` ZMARK` at whatever prompt followed:
	//
	//	off   the complaint, then ` ZMARK` runs on its own
	//	on    the complaint, then the prompt reads `!nosuchprefix`, so
	//	      ` ZMARK` makes `!nosuchprefix ZMARK` and fails again
	//
	// The complaint prints either way, so this does not replace the
	// diagnostic. See interp.Runner.HistoryExpansionReedits (#4149).
	"histreedit": {
		get: (*interp.Runner).HistoryExpansionReedits,
		set: (*interp.Runner).SetHistoryExpansionReedits,
	},
	// A `cd` operand that named no directory is looked up as a *variable*
	// holding one — the last resort, after the relative lookup and after
	// CDPATH, both of which win. Measured 2026-09-23 on bash 5.3.15, each row
	// in a fresh directory:
	//
	//	d=$PWD/target; cd d    moves, printing `$PWD/target`
	//	d=target;      cd d    moves, printing `target`
	//	a real `d` exists      the directory wins, nothing printed
	//	d=$PWD/afile;  cd d    `cd: d: Not a directory` — the *operand*
	//
	// See interp.Runner.BareCdOperandCanNameAVariable for the whole table and
	// for the two details an implementation gets wrong: the announcement is
	// the value as written rather than where it arrived, and the failure
	// still names the operand.
	//
	// It sat in shoptStates refusing the write, and the refusal was the
	// quiet kind: a script that sets this has written `cd d` meaning the
	// variable, and a shell that refused the option read the word as a
	// directory name and said it was not there (#4149).
	"cdable_vars": {
		get: (*interp.Runner).BareCdOperandCanNameAVariable,
		set: (*interp.Runner).SetBareCdOperandCanNameAVariable,
	},
	// Whether a prompt's value is expanded each time it is drawn. On with
	// nothing said, and it gates the *expansion* pass only — the backslash
	// language runs either way, measured 2026-09-23 through a pty on bash
	// 5.3.15:
	//
	//	PS1='[$x]> ' with x=XVAL   on `[XVAL]> `   off `[$x]> `
	//	PS1='[\u]> '               on `[root]> `   off `[root]> `
	//
	// It reaches the drawing through interp.PromptStyle.Expand, which is
	// asked at every draw — see promptVarsIsOn, and zsh's promptsubst one
	// dialect over for the same shape.
	//
	// It sat in shoptStates reading **on**, which made `shopt -s` a silent
	// grant and `shopt -u` the refusal — so the one state a script sets the
	// name for was the one it could not have, over a prompt this shell was
	// expanding unconditionally (#4149).
	promptVarsName: {
		get: promptVarsIsOn,
		set: func(r *interp.Runner, on bool) { shoptSetStored(r, promptVarsName, on, promptVarsDefault) },
	},
	// The fourth name here that moves a semantics axis, and the one whose
	// axis holds a *name* rather than an answer:
	// interp.Semantics.PromptCommentsNeedTheOption is this option's own
	// spelling, so the front end asks this entry per accepted line and a `#`
	// typed at the prompt stops opening a comment the moment a script turns
	// it off.
	//
	// Measured 2026-09-23 on bash 5.3.15, into `bash --norc --noprofile -i`
	// on a pipe with a scratch HOME:
	//
	//	echo a #b                                 a
	//	shopt -u interactive_comments; echo a #b   a #b
	//	…then shopt -s again; echo c #d            c
	//
	// It sat in shoptStates reading **on**, which was honest about the
	// default and wrong about everything the name exists for: `shopt -s` was
	// a silent grant and `shopt -u` was refused at 1, so the one state a
	// script asks for was the one it could not have. The reading did not
	// change — the axis this shell already had is now given bash's name for
	// it, which is what the option was missing.
	//
	// Non-interactive input is not this option's business in bash either:
	// `bash -c 'shopt -u interactive_comments; echo a #b'` is `a`, measured
	// the same day, and the axis is read by the prompt alone (#4149).
	"interactive_comments": {
		get: func(r *interp.Runner) bool { return shoptStoredState(r, "interactive_comments", true) },
		set: func(r *interp.Runner, on bool) { shoptSetStored(r, "interactive_comments", on, true) },
	},
	// The third name here that moves a semantics axis, and the one whose
	// sense is inverted: what it asks for is the answer
	// interp.Semantics.UnsetRemovesAnEnclosingLocal calls **No**. This shell
	// removes a caller's local outright — `v=GLOBAL; g(){ unset v; };
	// f(){ local v=L; g; echo "${v-UNSET}"; }; f` prints GLOBAL here and
	// UNSET in zsh, dash and BusyBox ash — and `shopt -s localvar_unset` is
	// what makes it answer the way the rest of the panel already does.
	//
	// So the option and the default are one axis and not a feature beside a
	// bug, which is why this could not be closed by writing `true` into
	// shoptStates: the name sat there reporting off over a shell that was
	// giving the option's answer with the option unset, so a script setting
	// it was refused for asking to keep behavior it already had, and one
	// that never mentioned it read the wrong value in silence (#3435).
	"localvar_unset": {
		get: func(r *interp.Runner) bool { return !r.UnsetRemovesAnEnclosingLocal() },
		set: func(r *interp.Runner, on bool) { r.SetUnsetRemovesAnEnclosingLocal(!on) },
	},
	// The second name here that moves a semantics axis, and the one that
	// moves the same axis `set -o posix` does. Whether a `$(…)` body's own
	// shell holds `set -e` is
	// interp.Semantics.ErrExitEntersACommandSubstitution, answered `No` by
	// this preset and by BusyBox ash and `Yes` by dash, ksh93 and zsh — and
	// bash is the only column with a name for moving it.
	//
	// The option and the mode are one piece of state rather than two,
	// because that is what was measured: `set -o posix` leaves `shopt
	// inherit_errexit` reporting `on`, `set +o posix` leaves it reporting
	// `on` with the behavior still there, and `shopt -u inherit_errexit` is
	// the way back from either door. A bit of its own beside the axis would
	// have had to agree with the mode in both directions and would have
	// disagreed in the second.
	//
	// It sat in shoptStates below, refusing the write, and that refusal was
	// the actively misleading kind #2361 names: a script sets this option
	// precisely so that a failure inside `$(…)` stops the substitution, and
	// the table said `off` while the shell behaved as though it were on. So
	// the name reported the opposite of what the shell did, in the one
	// direction that hides work — the value comes back short and nothing is
	// printed to say so (#3001).
	"inherit_errexit": {
		get: (*interp.Runner).ErrExitEntersACommandSubstitution,
		set: (*interp.Runner).SetErrExitEntersACommandSubstitution,
	},
	// The second inverted entry here, and it is inverted for the same reason
	// `no_empty_cmd_completion` is: bash names the *suppression*, so the
	// option being on is the capability being off. `shopt -u globskipdots`
	// is what asks for `.` and `..` back in a `.*` expansion, and the core
	// bit names the listing rather than the skipping — which is what keeps
	// the zero value of a Runner the state every shell without the option is
	// in.
	//
	// It sat in shoptStates reading **on**, which made `shopt -s` a silent
	// grant and `shopt -u` the refusal, so there was no way to ask for the
	// two names at all (#3386).
	//
	// Measured 2026-09-17 on bash 5.3.20 under `LC_ALL=C`, in a directory
	// holding `.a`, `.b`, `vis` and `sub/`, with the option off:
	//
	//	.*                     . .. .a .b
	//	*                      sub vis
	//	shopt -s dotglob; *    .a .b sub vis
	//	*/.*                   sub/. sub/.. sub/.x
	//	.*/                    ../ ./
	//	shopt -s globstar; **  sub sub/y vis
	//
	// The third row is what makes this a question of its own rather than the
	// hidden-name rule again: with the leading-period rule lifted **and** the
	// two names asked for, `*` still does not see them. So the option is
	// about the *listing* a component with a written period may match, which
	// is interp.PeriodPatternListsDotAndDotDot, and not about
	// interp.PatternsMatchHidden.
	//
	// bash 5.2 is where the name arrived and its default is on, so the
	// default behavior here was already right; what was missing was the way
	// back.
	// The name that turns a *diagnostic* on, and the only one here whose
	// whole effect is that the shell starts saying something it was already
	// doing. `shift` past the end is 1 with `$#` untouched in this shell
	// either way; what the option buys is the sentence, which is why the
	// wording sits in this dialect's Diagnostics and the *withholding* is
	// the capability — interp.Runner.ReportsShiftPastTheEnd, turned off by
	// Configure because this shell's default is the quiet one.
	//
	// Measured 2026-09-17 on bash 5.3.20 and 3.2.57 alike, from a script
	// file, with the option on:
	//
	//	set -- a; shift 3         shift: 3: shift count out of range, 1
	//	set -- a; shift; shift    shift: shift count out of range, 1
	//	shift 0                   0, silent
	//	set -- a b; shift 2       0, silent
	//	shift -1                  named with the option off as well as on
	//
	// The second row is why there are two wordings rather than a
	// placeholder, and the last is why the option governs one end of the
	// range and not both. It was in shoptStates refusing the write, which
	// was honest while nothing carried the sentence and stopped being so the
	// moment the sentence existed (#3465).
	// The name a script written for a System V `echo` sets, and the third in
	// this table that moves the shell to the other side of an answer rather
	// than turning a capability up: whether `echo` interprets its escapes
	// with no `-e` is interp.Semantics.EchoInterpretsEscapes, answered `Yes`
	// by dash and zsh and `No` by this preset and by ksh93 — and this is the
	// only shell in the panel that lets a script move it.
	//
	// It is set in an rc file rather than per call, which is why refusing it
	// was worse than it looks: every later `echo` in the file answered the
	// other way, so one diagnostic was followed by any number of wrong lines
	// (#3059).
	//
	// Measured 2026-09-17 on bash 5.3.20 with the option on, `$(…)` around
	// each call so the bytes are visible:
	//
	//	echo 'a\tb'       a<TAB>b
	//	echo -E 'a\tb'    a\tb        — the letter still wins, for one call
	//	echo -e 'a\tb'    a<TAB>b
	//	echo -n x         no newline  — the option is not about the letters
	//	echo 'a\x41b'     aAb
	//	echo 'a\0101b'    aAb
	//	echo 'a\cb'       a           — output stops, newline included
	//
	// So it moves the default and nothing else: which escapes exist, which
	// letters are read and which of `-e -E` wins are each their own axis and
	// each answers the same with the option on. bash 3.2 agrees on every row
	// but `\e`, which is that build's own age rather than this option's.
	"xpg_echo": {
		get: (*interp.Runner).EchoExpandsEscapes,
		set: (*interp.Runner).SetEchoExpandsEscapes,
	},
	// One switch under two names, which is measured and not a convenience:
	// `shopt -s array_expand_once` turns `assoc_expand_once` on too and the
	// reverse, on bash 5.3.20. Two entries reading and writing the same bit
	// is the whole of that — nothing has to copy one name's state to the
	// other, and no listing can show them disagreeing.
	//
	// The sense inverts here and nowhere else, for the reason
	// `no_empty_cmd_completion` above gives: what the option asks for is the
	// round to *stop*, and the core holds the capability the positive way
	// round. See interp.Runner.ExpandsAnOperandsSubscriptAgain, which also
	// carries which surfaces move and which two deliberately do not.
	"array_expand_once": {
		get: func(r *interp.Runner) bool { return !r.ExpandsAnOperandsSubscriptAgain() },
		set: func(r *interp.Runner, on bool) { r.SetExpandsAnOperandsSubscriptAgain(!on) },
	},
	"assoc_expand_once": {
		get: func(r *interp.Runner) bool { return !r.ExpandsAnOperandsSubscriptAgain() },
		set: func(r *interp.Runner, on bool) { r.SetExpandsAnOperandsSubscriptAgain(!on) },
	},
	// The descriptor a `{name}>file` redirection picked is taken back when
	// the command ends rather than left open. The sense inverts here, for
	// the reason `no_empty_cmd_completion` above gives: the option names the
	// *closing* and the core holds the capability the positive way round.
	// See interp.Runner.FdVariableDescriptorOutlivesTheCommand, which also
	// carries why `exec` is outside it.
	//
	// It sat in shoptStates refusing the write, and the refusal was the
	// honest kind while nothing implemented it — a shell that granted the
	// name and went on leaving descriptors open would be the silent wrong
	// answer, since the whole observable is whether a later `<&$fd` finds
	// anything.
	"varredir_close": {
		get: func(r *interp.Runner) bool { return !r.FdVariableDescriptorOutlivesTheCommand() },
		set: func(r *interp.Runner, on bool) { r.SetFdVariableDescriptorOutlivesTheCommand(!on) },
	},
	"shift_verbose": {
		get: (*interp.Runner).ReportsShiftPastTheEnd,
		set: (*interp.Runner).SetReportsShiftPastTheEnd,
	},
	"globskipdots": {
		get: func(r *interp.Runner) bool {
			return !r.MatchOption(interp.PeriodPatternListsDotAndDotDot)
		},
		set: func(r *interp.Runner, on bool) {
			r.SetMatchOption(interp.PeriodPatternListsDotAndDotDot, !on)
		},
	},
	// A valueless local declaration takes the value **and the attributes** of
	// the name at the enclosing scope instead of starting empty. The
	// wholesale spelling of what the `-I` letter on one declaration asks
	// for, and the two are one request with one answer — measured, `local -I
	// v` with this off and `local v` with it on produce the same binding
	// down to the letters.
	//
	// It sat in shoptStates refusing the write, and the refusal was the
	// honest kind while nothing implemented it — a shell that granted the
	// name and went on making empty bindings would be the silent wrong
	// answer, since the whole observable is what the local holds. See
	// interp/localinherit.go for the measured table and the edges that
	// bound it (#3434).
	"localvar_inherit": {
		get: (*interp.Runner).LocalInheritsTheOuterValue,
		set: (*interp.Runner).SetLocalInheritsTheOuterValue,
	},
	// A line history expansion changed goes back on the editing line rather
	// than running. The only name in this table whose whole observable is at
	// a prompt, which is why it sat in shoptStates refusing the write until
	// there was somewhere to put the text: an editor that cannot be handed a
	// line has no way to keep the promise, and a shell reporting `on` while
	// the expansion ran unverified would be the exact failure the option
	// exists to prevent — a mistyped `!string` running something nobody read.
	//
	// What made it buildable is the seam `vared` left behind (#2914):
	// repl.lineStart is a read that begins from text somebody supplied, and
	// this is its second caller. See interp.Runner.HistoryExpansionVerifies
	// for the measured rows and docs/spec/history.md for the prose (#3203).
	"histverify": {
		get: (*interp.Runner).HistoryExpansionVerifies,
		set: (*interp.Runner).SetHistoryExpansionVerifies,
	},
	// The two history names this table gained last, and the third and fourth
	// entries here whose sense is inverted: what the core holds is the state
	// every shell *without* the option is in, so the option is its negative.
	// See interp.Runner.HistoryJoinsATypedCommand and RewritesTheHistoryFile,
	// which carry the measurements; what follows is why each could not stay a
	// recorded state.
	//
	// `lithist` asks that a command typed over several lines keep those lines
	// in the history rather than be joined into one entry. This shell kept
	// them, so it sat on the option's **on** side with the option reported
	// on, and `shopt -u lithist` — which a script runs to put a known shell
	// in front of itself — was refused for asking for bash's own default. The
	// join is internal/histjoin's, which a script's history gate and `fc`'s
	// editor road already reached; a prompt is its third reader (#4149).
	"lithist": {
		get: func(r *interp.Runner) bool { return !r.HistoryJoinsATypedCommand() },
		set: func(r *interp.Runner, on bool) { r.SetHistoryJoinsATypedCommand(!on) },
	},
	// `histappend` is the one of the pair whose refusal was argued at length
	// and was wrong for a reason only a measurement could show. The table
	// recorded it **on** because this shell appends to HISTFILE and never
	// rewrites it — which is true — and read bash's `off` as a promise to
	// rewrite. Measured 2026-09-22, bash 5.3.20 with the option off appends
	// too, in every case but one: it rewrites the file from its list where
	// the list no longer holds every line the session added. So the two
	// shells differed in exactly that corner and the listing was describing
	// the wrong half of the option.
	"histappend": {
		get: func(r *interp.Runner) bool { return !r.RewritesTheHistoryFile() },
		set: func(r *interp.Runner, on bool) { r.SetRewritesTheHistoryFile(!on) },
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
// Recording is a cost and not a free win — a recorded name reports a state
// nothing keeps — so a name earns its place here by a stated test and never by
// being inconvenient to implement. There are two grounds, and the second was
// added by #4149 after measuring the reference rather than this shell.
//
// **The option decides what a completer offers, and there is nothing else a
// script can ask it.** The first three below.
//
// **The reference does not exhibit the difference either.** Not "this shell
// cannot tell the two states apart" — bash cannot. Measured 2026-09-23 on bash
// 5.3.15 in the pinned image, every row run twice on the same binary, once in
// each state, and compared byte for byte:
//
//   - extquote. Its documented effect is that `$'…'` and `$"…"` are decoded
//     inside a `${…}` enclosed in double quotes. Eight shapes — a default
//     value, an alternate value, a pattern removal, a substitution, `$"…"`,
//     a nested expansion, an assignment, and a bare `$'…'` word — are
//     **identical** with the option set and unset. The name is vestigial in
//     5.3: the decoding happens either way. So there is no behavior to build,
//     and a switch here would promise one.
//   - noexpand_translation. Its effect is on how a *translated* `$"…"` is
//     quoted, and bash's own manual says an untranslated string is
//     unaffected. Five shapes — plain, with a parameter, with a command
//     substitution, in a double-quoted argument, and with `TEXTDOMAIN` and
//     `TEXTDOMAINDIR` exported — are identical in both states. Reaching the
//     difference needs a message catalog, which is neither this shell's
//     business nor the suite's environment.
//   - globasciiranges. Range expressions compare as if in the C locale, and
//     the suite runs every file under `LC_ALL=C` and `LANG=C`. There the two
//     states *are* one state, so no observation — including the suite's own —
//     could tell an implementation from a no-op. This one differs from the two
//     above in that bash would show it outside the C locale; what makes it
//     unfalsifiable is the environment rather than the reference, and that is
//     recorded here rather than left as a gap to fill because an
//     implementation nobody can falsify is worse than an absence with a
//     reason. See #4230 for what measuring against a locale the machine does
//     not have costs.
//
// A name here moves **out** the moment somebody finds an observable difference.
// That is the whole guarantee: the table claims no behavior, so the only way it
// can be wrong is by holding a name that does have some, and the reason stated
// for each is the thing to falsify.
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
// `complete_fullquote` was *not* here, on the ground that this shell really
// does backslash every shell metacharacter in a completed name — see
// escapeName in repl/completeword.go — so bash's on state is this
// implementation's state. That reasoning was about the **listing** and it
// still holds; what it cannot do is satisfy `shopt -u`, and #4149 is where
// that started to matter: shopt1.sub sets every name bash lists and then
// unsets every one of them, so a table that grants one direction and refuses
// the other can never answer it. The name is below now, on the third ground,
// with the shapes I could not tell apart named so that somebody can.
//
// **Not "the reference does not exhibit it" and not "the completer offers
// nothing" — I could not construct an observation.** The third ground, and
// the weakest of the three, so it says exactly what was tried. bash's off
// state removes `$` and a backquote from the characters it quotes in a
// completed filename *when they appear in a shell variable reference in the
// word being completed*. Measured 2026-09-23 through a pty on bash 5.3.15,
// paced keystrokes so readline actually saw the Tab, with a file named
// `has$dollar` and a directory named `dir$x` present:
//
//	echo has<Tab>                     `echo has\$dollar ` in both states
//	ls $V<Tab>, V unset               unchanged in both
//	ls $V<Tab>, V=dir                 unchanged in both
//	ls $V<Tab> with `dir$x` matching  unchanged in both
//
// Four shapes, no difference. Unlike extquote I am **not** claiming the
// reference is vestigial here: the documented condition is narrow and I may
// simply not have hit it. What I am claiming is that I have no measurement to
// implement against, and an implementation nobody can falsify is worse than a
// recorded state with its attempts written down. The off state does have
// somewhere it *could* appear in this shell — escapeName could quote less —
// which is why this is its own ground and not the completer one above.
var shoptRecorded = map[string]bool{
	"force_fignore": true,
	"hostcomplete":  true,
	"progcomp":      true,
	// The second ground, with bash's own defaults kept so the listing does not
	// move: see the measurements in the comment above.
	"extquote":             true,
	"globasciiranges":      true,
	"noexpand_translation": false,
	// The first ground again — a completer option this completer has nothing
	// behind. `progcomp` above is the same option one step out: it decides
	// whether the specifications `complete` registers are used at all, and
	// this one decides what happens when one of them yields nothing and the
	// command name is an alias. A completer that consults no specification
	// never reaches the question. Measured through a pty the same day:
	// `myali pl<Tab>` after `alias myali=ls` completes `plainfile` in bash
	// with the option on and off alike, because ordinary filename completion
	// answers first either way.
	"progcomp_alias": false,
	// And the third ground, alone: see the comment above for the four shapes.
	"complete_fullquote": true,
	// The first ground once more, and the plainest instance of it: the option
	// decides what a **mail check** reports, and this shell has no mail check.
	// Nothing reads MAIL, MAILPATH or MAILCHECK anywhere under interp or repl,
	// so there is no file whose access time could be noticed and no prompt
	// cycle that would notice it. bash's message — `The mail in … has been
	// read` — has nothing here to be said about.
	//
	// Its own entry rather than folded in with force_fignore and hostcomplete
	// because it is not a completer's: the shape of the ground is the same —
	// an option over a facility this shell does not have — and the facility is
	// different, which is worth being able to read off the table.
	"mailwarn": false,
}

// shoptStateStore is where a moved name this dialect keeps for itself is
// kept: an array under a name no script can reach, which is the shape the zsh
// dialect's own recorded store uses and for the same reasons — a subshell
// deep-copies the variable table, so `(shopt -u progcomp)` stays in the
// subshell.
//
// Deviations rather than states, so a shell that has never run `shopt` on one
// of these holds an empty array and every name reads back at bash's default.
//
// Two kinds of name live here and they are not the same kind. The recorded
// ones above are remembered and acted on by nothing. `extdebug` is remembered
// *and* acted on: the behavior it carries is the core's two trap-carriage
// options, and what is stored here is only the indicator bit bash writes back
// through `shopt -p`, which no core state holds because the option and the
// indicator come apart — see the entry in shoptSwitches.
const shoptStateStore = ".bash.shopt"

// shoptStoredState reads one stored name.
func shoptStoredState(r *interp.Runner, name string, def bool) bool {
	names, _ := r.GetArray(shoptStateStore)
	for _, n := range names {
		if n == name {
			return !def
		}
	}
	return def
}

// shoptSetStored records or clears one name's deviation from its default,
// keeping the store sorted so it is a function of the set rather than of the
// order an rc file happened to write.
func shoptSetStored(r *interp.Runner, name string, on, def bool) {
	names, _ := r.GetArray(shoptStateStore)
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
	r.SetArray(shoptStateStore, out)
}

// shoptStates are the rest of the names bash 5.3 lists, with the state this
// implementation is in — not the state bash defaults to, the same rule
// interp's set-option table follows. Asking for the state we already hold is
// a request that has been granted; asking to move is refused out loud,
// because granting it would promise behavior nothing here provides.
//
// The handful that are on are on because the behavior they name is simply
// how this shell works: ranges match by byte, `*` never yields `.` or `..`,
// `$'…'` is decoded inside `${…}`, the prompt expands parameters, `.`
// searches PATH, and `cmdhist` describes the history this shell already
// keeps.
//
// `interactive_comments` was in that list and is in shoptSwitches above now.
// It is the same shape `compat31`…`compat44` were: a state recorded on the
// side this shell happened to be on, over a switch the shell already had —
// interp.Semantics.PromptCommentsNeedTheOption, which this preset was leaving
// empty. Reading `on` was honest about the default and wrong about the one
// state a script asks the name for (#4149).
//
// Which side a name falls on is measured, never assumed, and the history
// names have to be measured through a terminal rather than through `-c`:
// `-i -c` runs one command and exits without entering the prompt loop, so it
// never reaches the editor that keeps the history at all.
//
//   - cmdhist. A command typed over four lines is one entry here, not four:
//     typing a `for` loop and pressing Up recalls the whole loop. bash with
//     `shopt -u cmdhist` recalls only `done`. So turning it *on* is a request
//     that has been granted, and turning it off is refused: this shell cannot
//     promise to split a construct into a line each.
//
// `histappend` and `lithist` were here beside it, recorded **on** because
// this shell appended to the file and kept a construct's newlines, and both
// are in shoptSwitches above now — the two names the suite file `shopt.tests`
// was refused on (#4149). Neither moved because the honest reading changed:
// they moved because the behavior each names was built, so the name is a
// switch with bash's own default instead of a state nothing could leave.
//
// Every name #1429 and #1445 collected is now wired; nothing in the table
// below is a behavior somebody asked for and did not get. The last two,
// `dirspell` and `direxpand`, are in shoptSwitches above.
// `array_expand_once` and `assoc_expand_once` were the two names in this
// table recorded on the side this shell was *not* on, and they are in
// shoptSwitches above now. What they name is the suppression of a second
// expansion of a subscript that reached a builtin as text, and the observable
// that separates the two states is a subscript the shell never expanded to
// begin with:
//
//	declare -A a; k='x y'; a[$k]=hello
//	unset -v 'a[$k]'          # quoted, so unset receives the dollar sign
//	echo "${a[$k]-UNSET}"
//
// bash with the names off — its default — prints UNSET, because `unset`
// expands the subscript itself and finds the key. With them on it prints
// hello, and this shell printed hello either way, so the state the names
// described was the state it was already in and `shopt -s assoc_expand_once`
// was refused for a request that had in fact been granted.
//
// Recording them on would have made that refusal go away, and it was
// measured rather than assumed: bash's own quotearray file drops four lines
// and its shopt file gains exactly four, because a listing that reads `on`
// where bash's default reads `off` is a difference of its own and the file
// that lists every name finds it twice. A wash, so the honest half of the
// answer was not bought with the dishonest half; what closed it was the
// second expansion itself (#3298).
//
// `compat31` through `compat44` were seven more entries here and are in
// shoptSwitches above now, for the same reason those two were: they are not
// seven states at all — they are the compatibility level under a second
// spelling — so a `shopt -s compat44` that did not move `BASH_COMPAT`
// reported a level the rest of the shell could not see, which is the refusal
// that misleads rather than the one that is heard (#4262).
// The two names left here are **decided** rather than pending, and the
// decision is to refuse both. They are what `shopt.tests` has instead of
// agreement, and #2298 classifies them as recorded decisions with their
// reasoning rather than as work outstanding (#4149).
//
// `gnu_errfmt` makes every diagnostic take GNU's `prog:file:line: message`
// shape. Implementing it means reproducing this shell's wording for **every
// diagnostic it can emit**, and that surface is unbounded: it cannot be
// enumerated from a manual, and CLEANROOM forbids reading the source that
// defines it. A fit to the eighteen rows one suite file happens to exercise
// would be right on those eighteen and wrong on a set nobody can bound, with
// no way to state how wrong — which is this table's own principle, that an
// implementation nobody can falsify is worse than a documented absence.
//
// `bash_source_fullpath` does not describe a rule a reader could predict.
// Measured: the reference keeps **two copies** of the path — the frame
// `BASH_SOURCE` reports is fixed at the `.` and never revisited, and the
// value comes out absolute if the option was on at *either* the `.` or the
// call. The same file, sourced once and called twice with the option moved
// in between, answers differently for reasons that are about that shell's
// bookkeeping. An implementation artifact, not a semantics axis, and refused
// on the ground #4225 was decided on.
//
// So a name arriving here now needs an argument that it is neither of those
// shapes. The table is no longer a queue.
var shoptStates = map[string]bool{
	"bash_source_fullpath": false,
	"gnu_errfmt":           false,
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
	if modes, ok := shoptModes[name]; ok {
		return r.MatchOption(modes[0]), true
	}
	if sw, ok := shoptSwitches[name]; ok {
		return sw.get(r), true
	}
	if ro, ok := shoptReadOnly[name]; ok {
		return ro(r), true
	}
	if def, ok := shoptRecorded[name]; ok {
		return shoptStoredState(r, name, def), true
	}
	on, known = shoptStates[name]
	return on, known
}

// bashOptions is `$BASHOPTS`: every `shopt` name this shell has on, sorted
// and colon-separated.
//
// The same design as `$SHELLOPTS` one namespace along — produced rather than
// stored, so `shopt -s cdspell; echo $BASHOPTS` says what is true now, and
// readonly, so no assignment can make it lie. The two are half of how a shell
// hands its option state to a child, and a harness that read this one back
// used to see a shell with nothing set at all (#2475).
//
// The names are this shell's own state and not a claim about bash's. Several
// that bash lists on by default are recorded here as off because nothing
// implements them, and reporting them the other way round to match a listing
// would be exactly the lie the produced value exists to prevent.
//
// bash 3.2 has no such variable — a `-c` there writes nothing for it, and an
// inherited value turns nothing on — so this is bash 5's answer and the corpus
// row says which is being copied.
func bashOptions(r *interp.Runner) string {
	names := shoptNames()
	on := make([]string, 0, len(names))
	for _, n := range names {
		if state, known := shoptState(r, n); known && state {
			on = append(on, n)
		}
	}
	// shoptNames is already sorted, so this is a re-assertion rather than
	// work — kept because the ordering is the contract and a later change to
	// how the names are gathered must not quietly drop it.
	sort.Strings(on)
	return strings.Join(on, ":")
}

// applyInheritedBashOptions is the write direction: every name in the value
// this shell inherited is turned on.
//
// **A name it does not know is skipped in silence**, which is measured and is
// the opposite of what `$SHELLOPTS` does with one: `BASHOPTS=nosuchopt:cdspell
// bash -c 'shopt -p cdspell'` answers `shopt -s cdspell` at status 0 with
// nothing on standard error, and so does a value with a leading or trailing
// colon. A `set -o` name in it is ignored the same way — the two namespaces
// are separate here as they are everywhere else in this builtin.
//
// Nothing is turned off. The value says what is on, and a shell whose defaults
// differ from what it was handed does not lose them: a value naming only
// `cdspell` leaves `sourcepath` on in bash, measured.
//
// The names it does know go through shoptApply, which is the same door
// `shopt -s` uses. So a name this shell cannot honor draws that builtin's own
// sentence rather than a second one written for startup, and `$BASHOPTS`
// afterwards still reports only what is really on.
func applyInheritedBashOptions(r *interp.Runner, value string) {
	var known []string
	for _, name := range strings.Split(value, ":") {
		if _, ok := shoptState(r, name); ok {
			known = append(known, name)
		}
	}
	if len(known) > 0 {
		shoptApply(r, known, true)
	}
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
			shoptListAll(r, reissue)
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
		if modes, ok := shoptModes[name]; ok {
			for _, mode := range modes {
				r.SetMatchOption(mode, on)
			}
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
			shoptSetStored(r, name, on, def)
			continue
		}
		if held, ok := shoptStates[name]; ok {
			if held == on {
				// Already in the state being asked for: granted.
				continue
			}
			shoptComplaint(r, "%s: not implemented\n", name)
			status = 1
			continue
		}
		shoptComplaint(r, "%s: invalid shell option name\n", name)
		status = 1
	}
	return status
}

// shoptComplaint writes one of shoptApply's sentences, naming the builtin
// only where a builtin was written.
//
// The same table is reached from two routes and bash words them differently.
// Measured 2026-09-16 on bash 5.3.20 with standard input on /dev/null:
//
//	bash -O nosuchopt -c :
//	  bash: line 0: nosuchopt: invalid shell option name
//	bash -c 'shopt -s nosuchopt'
//	  bash: line 1: shopt: nosuchopt: invalid shell option name
//
// One sentence, two routes, and the difference is a word — the name of a
// builtin that was never typed. interp.Runner.AtInvocation is what tells the
// two apart, and it is true only for the length of one
// interp.Runner.SetShellOption call.
func shoptComplaint(r *interp.Runner, format string, args ...any) {
	if r.AtInvocation() {
		r.Diagnosef(format, args...)
		return
	}
	r.Diagnosef("shopt: "+format, args...)
}

// shoptMoveAtInvocation moves one name from the words the shell was started
// with — `bash -O checkhash`, `bash +O extglob` — and answers the status the
// invocation ends with.
//
// Through shoptApply, which is the same door `shopt -s` and an inherited
// `$BASHOPTS` already use, so a name this shell knows and cannot honor draws
// the one sentence written for it rather than a second copy.
//
// The status is **not** this builtin's. Measured on bash 5.3.20: `shopt -s
// nosuchopt` inside a script is status 1 and the script carries on, while
// `bash -O nosuchopt -c 'echo hi'` is status 2, prints nothing on standard
// output and never runs the command — and never opens the script operand
// either, since `bash -O nosuchopt /nope/x.sh` complains about the name and
// not about the file. So the two numbers are mapped here rather than shared.
func shoptMoveAtInvocation(r *interp.Runner, name string, on bool) int {
	if shoptApply(r, []string{name}, on) != 0 {
		return 2
	}
	return 0
}

// shoptListAll writes every name in the table, which is both what `shopt`
// with no operands does and what the invocation letter does with no name
// after it.
//
// One function for the two because they are one listing: measured on bash
// 5.3.20, `bash -O` is byte-for-byte `shopt` and `bash +O` is byte-for-byte
// `shopt -p`, and both leave the shell to go on and run whatever it was
// given at status 0.
func shoptListAll(r *interp.Runner, reissuable bool) {
	for _, n := range shoptNames() {
		on, _ := shoptState(r, n)
		printShopt(r, n, on, reissuable)
	}
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
//
// A name `set -o` does not know is refused here, in this builtin's words, and
// the refusal **leaves the status alone**. Measured 2026-09-16 on bash 5.3.20:
// `shopt -o -s nosuch` and `shopt -o -u nosuch` are `shopt: nosuch: invalid
// option name` at 0, and `shopt -o -s errexit nosuch` sets errexit and is 0 —
// where handing the name on made the complaint `set`'s, at 2, and ended a
// script under errexit. bash 3.2.57 answers the same sentence at 1, which is
// an age this dialect, being 5.3's, does not hold.
func shoptMoveO(r *interp.Runner, ctx context.Context, names []string, unset bool) int {
	status := 0
	for _, name := range names {
		if _, known := r.NamedOption(name); !known {
			r.Diagnosef("shopt: %s: invalid option name\n", name)
			continue
		}
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
