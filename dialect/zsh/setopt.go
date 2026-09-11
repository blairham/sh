// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/blairham/sh/interp"
)

// `setopt` and `unsetopt` are how zsh scripts change options — far more often
// than `set -o`, which zsh also has. The two builtins are one machinery with
// the request inverted, and the names they take are zsh's own namespace:
// case-insensitive, underscores ignored, and a single `no` prefix negating
// whatever follows, so `No_Glob`, `NOGLOB` and `noglob` are one request.
// Measured: the prefix strips once and once only — `no_no_glob` is "no such
// option", not glob restored.
//
// # The name set
//
// zsh 5.9.2 has 185 options and twelve further spellings borrowed from sh and
// ksh, and all 197 are in the table below. The set was derived by probing the
// binary rather than read anywhere: `set -o` lists every option in the
// spelling that is off by default, `zmodload zsh/parameter` then exposes
// `$options` whose keys are the canonical names and whose values are the live
// states, and the twelve extras were each identified by flipping them one at
// a time and reading which canonical entry moved with it.
//
// The default recorded for each name is that same measured state in a
// non-interactive `zsh -c` with an empty HOME, with one correction that a
// bare `setopt` reports itself: `hashdirs` is the single option whose
// compiled-in default differs from its state in such a shell, which is why
// `nohashdirs` is the one line a bare `setopt` prints there.
//
// # Recognized, recorded, and implemented are three different claims
//
// Every entry is one of four kinds, and the difference is the honest part of
// this file:
//
//   - backed by the substrate's `set -o` machinery, so `setopt err_exit` and
//     `set -o errexit` are the same switch read and written through one seam;
//   - backed by a semantics axis or a match option — `shwordsplit`, `nomatch`,
//     `ksharrays` are zsh's own names for three axes the vector already
//     carries, and `nullglob`, `globdots`, `caseglob` and `extendedglob` are
//     zsh's names for four the pattern matcher carries. Flipping the first
//     three is what `emulate` does too (see emulate.go);
//   - fixed: a name zsh has whose state this shell cannot change. Asking for
//     the state it is already in succeeds, the same bargain setoptions.go
//     strikes for `set +o posix`; asking it to move is refused out loud with
//     zsh's own wording for an option that will not budge — measured on
//     `setopt monitor` in a zsh with no terminal: `can't change option`, 1.
//     Real zsh refuses exactly five of the 185 that way in a `-c` run on a
//     pipe, and they are the five about being interactive — `interactive`,
//     `monitor`, `shinstdin`, `singlecommand` and `zle`. Every other name it
//     takes, in both directions, which is measured and is what says the rest
//     belong in one of the other kinds rather than in a refusal.
//
//     `monitor` is not one of them here, because it is not one of them in
//     zsh either once the shell has a terminal: on a pseudo-terminal it moves
//     in both directions and at 0, interactively and inside a plain `-c`
//     alike. It is backed by the substrate's own `set -m`, which was already
//     running the job control it names — the entry was a constant wired off,
//     so an interactive shell answered `m` in `$-` and `off` in
//     `${options[monitor]}` at the same moment (#1720);
//   - backed by the store a recorded name uses, and read by the front end
//     rather than by anything in this package: `histignorespace` alone, whose
//     state the line editor asks for through this namespace before it records
//     a line. Written `storeBacked(…)`;
//   - **recorded**: a name this shell recognizes and remembers and does not
//     act on. `setopt auto_cd` succeeds, `setopt` then reports `autocd`, and
//     typing a directory name still does not change directory. 140 of the 185
//     are this, and they are marked `recorded(…)` below so the distinction can
//     be read off the table rather than taken on trust.
//
// Three names have moved out of "recorded". Two went when the front end
// learned to read them:
// `histignorespace` above, and `histignoredups`, which was already a `set -o`
// backed switch whose state nothing consulted. Both now decide what a session
// writes to its history file, so both are implemented rather than remembered.
// The third is `extendedglob`, which now moves the pattern matcher's
// [interp.ExtendedPatternOperators] — the closures, the exclusion, the
// negation and the `(#…)` flag groups all read a pattern differently while
// it is on, and reading it the same way either way was #1244.
//
// `multios`, `cshnullcmd` and `shnullcmd` left together: the first is this
// shell's name for [interp.Semantics.RedirectsUseEveryTarget], and the other
// two decide what a command that is only redirections runs, by pointing the
// null-command parameters at a name no script can write (#1779). See
// nullcommand.go.
//
// `typesetsilent` is the most recent, and it is the inverse of
// [interp.Semantics.ValuelessDeclarationOfAHeldNameListsIt]: the option on is
// that axis answering No. It moved because powerlevel10k sets it and then
// re-declares `local` names inside loops, so remembering-and-ignoring it wrote
// nine lines to stdout before every prompt (#2033).
//
// Nothing else about the split moved, and 140 is still most of the table.
//
// Recording is worth doing and is not the same as implementing. A real rc
// file opens with a dozen `setopt` lines about completion, correction and
// history — features this shell does not have — and a shell that answers each
// with `no such option` sprays complaints at every startup over nothing it was
// ever going to do. Recording stops the complaint and reports the request back
// faithfully; it promises nothing further, and docs/spec/semantics.md says so
// in the same words.
//
// A name outside the table is `no such option`, status 1, and the remaining
// operands are still acted on — measured: `setopt zzqq no_glob` complains and
// still turns globbing off, in either order.
//
// # The listings
//
// Measured, and simpler than it first looks. Each option has one printed
// spelling: the one that is off by default, so `noclobber` for an option that
// defaults on and `allexport` for one that defaults off. A bare `setopt`
// prints the spellings that are currently on and a bare `unsetopt` prints the
// ones that are currently off, both ordered by the canonical name and both
// naming the canonical option rather than the compat spelling that may have
// set it — measured, `setopt dotglob; setopt` answers `globdots`.
//
// So `setopt` is the list of deviations from zsh's defaults and `unsetopt` is
// its complement, which is why the first is two lines long and the second is
// 184 in a shell that has changed nothing.

// zshOption is one name in this dialect's option namespace.
type zshOption struct {
	// base is the canonical spelling: lower case, no underscores, no `no`.
	base string
	// def is the state a zsh default run has, which is what the listings
	// compare against.
	//
	// Four of the entries this file inherited hold this shell's own state
	// here instead of zsh's — `banghist`, `emacs`, `hashcmds` and
	// `interactivecomments` are all measured the other way round in real zsh
	// — which silences four deviations the listing exists to show. They are
	// left as they were found rather than corrected in a change about the
	// name set; see docs/spec/semantics.md.
	def bool
	// recorded marks a name that is remembered and not acted on. It is what
	// tells the listings and `emulate` that the state lives in the store
	// below rather than in anything the shell does.
	recorded bool
	// get reads the live state.
	get func(*interp.Runner) bool
	// set moves it, returning zero or a status after its own complaint. Nil
	// marks a fixed option: the current state can be asked for and granted,
	// and anything else is refused.
	set func(*interp.Runner, bool) int
	// atInvocation moves it on the command line that started the shell, for
	// a fixed name this shell takes there and refuses to a running script.
	// Nil for every other name, which is what makes the route irrelevant to
	// all of them.
	//
	// It hangs off the entry rather than off the builtin because both
	// invocation routes — `-t` and `-o singlecommand`, and `-o onecmd` under
	// the borrowed spelling — arrive here after the alias table has resolved
	// the name, and a `case` in the letter reader could only answer one of
	// the three.
	atInvocation func(*interp.Runner, bool) int
}

// zshOptions is the table, in listing order (sorted by base).
var zshOptions = []zshOption{
	// Alias expansion happens here, which is what the name asks about. Which
	// routes into the shell it happens on is the parser's question and this
	// is not it (syntax.Dialect.ExpandAliases); the state is that the shell
	// does the thing, and it is not this shell's to switch off.
	{
		// `no_aliases` stops alias expansion for as long as it is off, which
		// is what a prompt theme sets to protect its own code from a user's
		// aliases — so refusing it is not cosmetic, it changes how the rest
		// of that file is *parsed*.
		//
		// The option is not the same thing as whether this shell expands at
		// all. The route decides that — measured, `zsh -c 'alias hi=…; hi'`
		// does not expand and the same two lines in a file do — while the
		// option reads `on` under `-c` all the same: `zsh -c '[[ -o aliases
		// ]]'` is 0. So the state kept here is the option, and what reaches
		// the parser is the option *and* the route.
		base: "aliases", def: true,
		get: func(r *interp.Runner) bool { return !recordedDeviates(r, "aliases") },
		set: func(r *interp.Runner, on bool) int {
			setRecordedDeviation(r, "aliases", !on)
			r.SetAliasExpansion(on && r.AliasExpansionBase())
			return 0
		},
	},
	recorded("aliasfuncdef", false),
	setOptBacked("allexport", false, "allexport", false),
	recorded("alwayslastprompt", true),
	recorded("alwaystoend", false),
	recorded("appendcreate", false),
	recorded("appendhistory", true),
	// Implemented rather than recorded since #1445: the same core switch
	// bash's `shopt -s autocd` moves, because it is the same name for the
	// same behavior in two option namespaces. What differs between the two
	// shells is only whether the substitution is announced, and that is an
	// axis this dialect answers `No` — see zsh.go.
	switchBacked("autocd", false, (*interp.Runner).AutoCd, (*interp.Runner).SetAutoCd),
	recorded("autocontinue", false),
	recorded("autolist", true),
	recorded("automenu", true),
	recorded("autonamedirs", false),
	recorded("autoparamkeys", true),
	recorded("autoparamslash", true),
	recorded("autopushd", false),
	recorded("autoremoveslash", true),
	recorded("autoresume", false),
	recorded("badpattern", true),
	recorded("banghist", false),
	// BARE_GLOB_QUAL: whether a trailing `(…)` on a pattern is a glob
	// qualifier list or part of the pattern. On by default and implemented
	// rather than recorded since #1729 — see
	// interp.TrailingGroupIsPartOfThePattern, which the pattern matcher reads
	// and which is named for the state this one turns *off*.
	matchBacked("bareglobqual", true, interp.TrailingGroupIsPartOfThePattern, true),
	recorded("bashautolist", false),
	recorded("bashrematch", false),
	recorded("beep", true),
	recorded("bgnice", true),
	recorded("braceccl", false),
	recorded("bsdecho", false),
	matchBacked("caseglob", true, interp.GlobFoldsCase, true),
	recorded("casematch", true),
	recorded("casepaths", false),
	recorded("cbases", false),
	recorded("cdablevars", false),
	recorded("cdsilent", false),
	recorded("chasedots", false),
	recorded("chaselinks", false),
	// The two names zsh checks its job table with, and they are one question
	// asked twice: `checkjobs` is the master and `checkrunningjobs` narrows
	// what the master looks at. Measured through a pseudo-terminal against
	// zsh 5.9.2 on 2026-09-08, with the job started and then `exit` typed:
	//
	//   - both on (the defaults): a `sleep 40 &` is `you have running jobs.`
	//     and a ^Z'd job is `you have suspended jobs.`;
	//   - `unsetopt checkrunningjobs`: the running one leaves at once, the
	//     suspended one still holds;
	//   - `unsetopt checkjobs`: *both* leave at once, which is where this
	//     shell differs from bash — `shopt -u checkjobs` there still holds
	//     for a suspended job.
	//
	// So the master moves both core switches and the narrower name moves
	// only one, and each has to recompute the running switch from both bits
	// rather than from its own. See checkJobsOption.
	checkJobsOption(),
	checkRunningJobsOption(),
	setOptBacked("clobber", true, "noclobber", true),
	recorded("clobberempty", false),
	recorded("combiningchars", false),
	recorded("completealiases", false),
	recorded("completeinword", false),
	recorded("continueonerror", false),
	recorded("correct", false),
	recorded("correctall", false),
	recorded("cprecedences", false),
	recorded("cshjunkiehistory", false),
	recorded("cshjunkieloops", false),
	recorded("cshjunkiequotes", false),
	nullCommandOption("cshnullcmd"),
	recorded("cshnullglob", false),
	recorded("debugbeforecmd", true),
	recorded("dvorak", false),
	// Default off, which is real zsh's: measured 2026-09-11 at a terminal,
	// `[[ -o emacs ]]` answers 1 in an interactive session that has not
	// selected a mode, and a bare `setopt` says nothing about the name. It
	// was recorded as on here, which was one of four entries holding *this*
	// shell's state where zsh's differed — and the only one of the four the
	// state behind it could be corrected for (#1858).
	setOptBacked("emacs", false, "emacs", false),
	recorded("equals", true),
	setOptBacked("errexit", false, "errexit", false),
	recorded("errreturn", false),
	recorded("evallineno", true),
	setOptBacked("exec", true, "noexec", true),
	matchBacked("extendedglob", false, interp.ExtendedPatternOperators, false),
	recorded("extendedhistory", false),
	recorded("flowcontrol", true),
	recorded("forcefloat", false),
	// `$0` inside a function is the function's name here, which is what the
	// name asks for and what this shell already does — and what it goes on
	// doing whichever way the name is written, which is why it is recorded
	// rather than refused.
	recorded("functionargzero", true),
	setOptBacked("glob", true, "noglob", true),
	recorded("globalexport", true),
	recorded("globalrcs", true),
	recorded("globassign", false),
	recorded("globcomplete", false),
	matchBacked("globdots", false, interp.PatternsMatchHidden, false),
	recorded("globstarshort", false),
	{
		// GLOB_SUBST: the result of an expansion is read as a pattern rather
		// than as literal text. It is this shell's name for the axis the
		// vector already carries, which is what makes it one switch over
		// every path that takes a pattern operand — `[[ ]]`, a `case` arm, a
		// trim, a `:#` filter — rather than a bit each of them would have had
		// to consult.
		//
		// Recorded until #1734, which is the shape of the bug worth naming:
		// the per-expansion spelling of the same request, `${~x}`, was
		// carried and honored, so the machinery for "this result is a
		// pattern" existed and the *option* was what nothing asked. Measured
		// on zsh 5.9.2, 2026-09-10, under `setopt globsubst` with `u='a*b'`:
		// `[[ axb == $u ]]` and `case axb in ($u)` both match there and
		// neither matched here, `${w#$u}` trimmed there and not here, and
		// `${(M)${:-axb}:#$u}` kept the element there and dropped it here.
		// The quoting flags reach it too: `[[ $v == ${(q)v} ]]` and the `(b)`
		// form are true in that shell, the escape each writes being read as a
		// pattern rather than as text.
		//
		// Read off the axis rather than off a stored bit, which is what makes
		// `(setopt globsubst)` stay in the subshell — the same arrangement
		// `multios` and `shwordsplit` use.
		base: "globsubst", def: false,
		get: func(r *interp.Runner) bool {
			return r.Semantics.GlobExpansionResults == interp.Yes
		},
		set: func(r *interp.Runner, on bool) int {
			swapAxes(r, func(s *interp.Semantics) { s.GlobExpansionResults = answer(on) })
			return 0
		},
	},
	setOptBacked("hashcmds", false, "hashall", false),
	recordedOver("hashdirs", true, constantState(false)),
	recorded("hashexecutablesonly", false),
	recorded("hashlistall", true),
	recorded("histallowclobber", false),
	recorded("histbeep", true),
	recorded("histexpiredupsfirst", false),
	recorded("histfcntllock", false),
	recorded("histfindnodups", false),
	recorded("histignorealldups", false),
	// Not recorded, and no longer unread: this is the switch a session asks
	// before deciding whether a line repeating the one before it goes into the
	// history file. `set -o`-backed because zsh's `set -h` abbreviates it, so
	// the substrate carried the state already.
	setOptBacked("histignoredups", false, "histignoredups", false),
	storeBacked("histignorespace", false),
	recorded("histlexwords", false),
	recorded("histnofunctions", false),
	recorded("histnostore", false),
	recorded("histreduceblanks", false),
	recorded("histsavebycopy", true),
	recorded("histsavenodups", false),
	recorded("histsubstpattern", false),
	recorded("histverify", false),
	recorded("hup", true),
	// Implemented rather than recorded since #1856: the substrate has a
	// run-time switch beside its BraceExpansion axis, and this name is that
	// switch under zsh's spelling — inverted, because zsh names the state
	// that stops the expansion where the substrate names the expansion.
	setOptBacked("ignorebraces", false, "braceexpand", true),
	recorded("ignoreclosebraces", false),
	recorded("ignoreeof", false),
	recorded("incappendhistory", false),
	recorded("incappendhistorytime", false),
	// Whether this is an interactive shell — a fact the front end brought in
	// rather than a switch, which is why it is read and not set. The
	// third-party integration this machine's startup files load opens on
	// `[[ -o interactive ]]`, and ksh93 has the same name for the same fact,
	// measured in its own `set -o` listing.
	{
		base: "interactive", def: false,
		get: func(r *interp.Runner) bool { return r.Interactive },
	},
	// Comments are honored wherever they are written, and go on being, which
	// is why the name is recorded rather than refused: the shell keeps doing
	// the thing in either direction.
	recorded("interactivecomments", true),
	{
		base: "ksharrays", def: false,
		// Read off the base, which is the axis the name is about; the four
		// that move with it are in ksharrays.go and every one of them
		// answers the same way, so any of the five would report the same
		// state.
		get: func(r *interp.Runner) bool { return r.Semantics.ArrayBaseIsZero == interp.Yes },
		set: func(r *interp.Runner, on bool) int {
			swapAxes(r, func(s *interp.Semantics) { setKshArrays(s, on) })
			return 0
		},
	},
	recorded("kshautoload", false),
	recorded("kshglob", false),
	recorded("kshoptionprint", false),
	recorded("kshtypeset", false),
	recorded("kshzerosubscript", false),
	recorded("listambiguous", true),
	recorded("listbeep", true),
	recorded("listpacked", false),
	recorded("listrowsfirst", false),
	recorded("listtypes", true),
	recorded("localloops", false),
	recorded("localoptions", false),
	recorded("localpatterns", false),
	// LOCAL_TRAPS: a trap a function sets goes back at its return, which is
	// an axis rather than a record since #1731. Not the trap-side reading of
	// `localoptions` above — measured, that one leaves a function's trap
	// installed — so it moves a switch of its own. The rule is in
	// localtraps.go here and the store is interp's.
	{
		base: "localtraps", def: false,
		get: localTrapsOn,
		set: setLocalTraps,
	},
	// Whether this shell was started as a login shell. zsh puts the fact in
	// this namespace rather than in `$-` — Semantics.LoginShowsLInDollarDash
	// is `Yes` here for exactly that reason — so `-l` has to reach the
	// listing, `[[ -o login ]]` and `${options[login]}` alike.
	//
	// It is the invocation's answer and still movable, which is measured
	// rather than assumed: zsh 5.9.2 on 2026-09-10 takes `setopt login` in a
	// shell that is not one at 0 and reports `login` afterwards, and takes
	// `unsetopt login` in a login shell the same way. bash's answer for its
	// own spelling is the opposite — `shopt -s login_shell` is accepted and
	// never moves it — so the two shells were measured apart rather than one
	// read off the other (#1727).
	recordedOver("login", false, func(r *interp.Runner) bool { return r.LoginShell }),
	recorded("longlistjobs", false),
	recorded("magicequalsubst", false),
	recorded("mailwarning", false),
	recorded("markdirs", false),
	recorded("menucomplete", false),
	// The switch job control really is: the same one `set -m` moves, so the
	// two spellings are one state read and written through one seam. Granted
	// in both directions where the shell has a terminal and refused with
	// zsh's own wording where it has none — which is
	// Semantics.MonitorNeedsATerminal, answered `Yes` by this dialect, and
	// not a question about being interactive. See setoptions.go for the
	// panel, and TestSetoptMonitorNeedsATerminalAndNotAPrompt for the
	// measurement.
	setOptBacked("monitor", false, "monitor", false),
	recorded("multibyte", true),
	recorded("multifuncdef", true),
	{
		// zsh's MULTIOS, and one switch over both directions: a stream
		// redirected twice writes to both files and reads from both, and
		// turning this off leaves the last target alone in either. Read off
		// the axis rather than off a stored bit, which is what makes
		// `(unsetopt multios)` stay in the subshell.
		base: "multios", def: true,
		get: func(r *interp.Runner) bool {
			return r.Semantics.RedirectsUseEveryTarget == interp.Yes
		},
		set: func(r *interp.Runner, on bool) int {
			swapAxes(r, func(s *interp.Semantics) { s.RedirectsUseEveryTarget = answer(on) })
			return 0
		},
	},
	{
		base: "nomatch", def: true,
		get: func(r *interp.Runner) bool { return r.Semantics.GlobNoMatchIsError == interp.Yes },
		set: func(r *interp.Runner, on bool) int {
			swapAxes(r, func(s *interp.Semantics) { s.GlobNoMatchIsError = answer(on) })
			return 0
		},
	},
	// When a job notice is written, and this shell writes it before the next
	// prompt rather than the moment the job changes state. Recorded because
	// neither direction moves that: the name is remembered and reported, and
	// the notice keeps arriving where it arrives.
	recorded("notify", true),
	matchBacked("nullglob", false, interp.UnmatchedPatternIsEmpty, false),
	recorded("numericglobsort", false),
	recorded("octalzeroes", false),
	recorded("overstrike", false),
	recorded("pathdirs", false),
	recorded("pathscript", false),
	setOptBacked("pipefail", false, "pipefail", false),
	recorded("posixaliases", false),
	recorded("posixargzero", false),
	recorded("posixbuiltins", false),
	recorded("posixcd", false),
	recorded("posixidentifiers", false),
	recorded("posixjobs", false),
	recorded("posixstrings", false),
	recorded("posixtraps", false),
	recorded("printeightbit", false),
	recorded("printexitvalue", false),
	recorded("privileged", false),
	recorded("promptbang", false),
	recorded("promptcr", true),
	recorded("promptpercent", true),
	recorded("promptsp", true),
	recorded("promptsubst", false),
	recorded("pushdignoredups", false),
	recorded("pushdminus", false),
	recorded("pushdsilent", false),
	recorded("pushdtohome", false),
	recorded("rcexpandparam", false),
	recorded("rcquotes", false),
	// Whether this shell reads its startup files, which zsh initializes from
	// the invocation: `-f` and `--no-rcs` turn it off, and the name is the
	// only place the fact is published — `setopt` in a `-f` shell prints
	// `norcs` beside `nohashdirs`, where a shell that read its files prints
	// `nohashdirs` alone.
	//
	// The same shape `login` above has, and measured the same way on zsh
	// 5.9.2, 2026-09-11: the invocation decides the base and a running
	// script still moves it both ways. `zsh -f -c 'setopt rcs; [[ -o rcs ]]'`
	// answers on and its `setopt` drops the line again, and `zsh -c
	// 'unsetopt rcs'` answers off and gains one. So it is neither a constant
	// nor a name that refuses — it is `recordedOver` the fact the front end
	// carried in, Runner.StartupFilesSuppressed.
	//
	// Recorded and not implemented, which is the bargain the rest of that
	// set strikes: nothing here re-reads the option to decide whether a
	// *later* startup file is read. The files are `driver`'s, and it has the
	// invocation first-hand (#1864).
	recordedOver("rcs", true, func(r *interp.Runner) bool { return !r.StartupFilesSuppressed }),
	recorded("recexact", false),
	recorded("rematchpcre", false),
	recorded("restricted", false),
	recorded("rmstarsilent", false),
	recorded("rmstarwait", false),
	recorded("sharehistory", false),
	recorded("shfileexpansion", false),
	// sh-style globbing narrows the pattern language to the standard's. This
	// shell does not narrow it, which is the state, and zsh's default is the
	// same, so the listings do not move. The prompt theme this machine loads
	// reads the name at its third line.
	recorded("shglob", false),
	// Three of the four that refuse to move — `interactive` is the fourth,
	// above — and all of them are about being interactive, which is the whole
	// of what real zsh refuses in a `-c` run on a pipe. Every one is off here
	// and off in such a zsh, so turning it off is granted and turning it on
	// is the measured `can't change option`, 1.
	fixedConstant("shinstdin", false, false),
	nullCommandOption("shnullcmd"),
	recorded("shoptionletters", false),
	recorded("shortloops", true),
	recorded("shortrepeat", false),
	{
		base: "shwordsplit", def: false,
		get: func(r *interp.Runner) bool { return r.Semantics.SplitParamExpansion == interp.Yes },
		set: func(r *interp.Runner, on bool) int {
			swapAxes(r, func(s *interp.Semantics) { s.SplitParamExpansion = answer(on) })
			return 0
		},
	},
	singleCommandOption(),
	recorded("singlelinezle", false),
	recorded("sourcetrace", false),
	recorded("sunkeyboardhack", false),
	recorded("transientrprompt", false),
	recorded("trapsasync", false),
	{
		// zsh's TYPESET_SILENT, and the inverse of one axis: a valueless
		// declaration of a name its own scope already holds writes the name
		// back, and this option is how a script asks it not to. Read off the
		// axis rather than off a stored bit, for the reason `multios` is —
		// `(setopt typeset_silent)` stays in the subshell.
		//
		// The sense is inverted: the option being *on* is the axis answering
		// No, which is why this is not `matchBacked`'s shape with a flag.
		// Measured on zsh 5.9.2: `f(){ local s; s=1; local s; }` writes `s=1`
		// with the option off and nothing with it on, in both directions.
		//
		// powerlevel10k is what made this worth implementing rather than
		// remembering: it sets the option in its `emulate -L zsh` intro and
		// re-declares `local` inside loops, so a shell that ignores the name
		// narrates nine lines before every prompt (#2033).
		base: "typesetsilent", def: false,
		get: func(r *interp.Runner) bool {
			return r.Semantics.ValuelessDeclarationOfAHeldNameListsIt == interp.No
		},
		set: func(r *interp.Runner, on bool) int {
			swapAxes(r, func(s *interp.Semantics) {
				s.ValuelessDeclarationOfAHeldNameListsIt = answer(!on)
			})
			return 0
		},
	},
	recorded("typesettounset", false),
	setOptBacked("unset", true, "nounset", true),
	setOptBacked("verbose", false, "verbose", false),
	setOptBacked("vi", false, "vi", false),
	recorded("warncreateglobal", false),
	recorded("warnnestedvar", false),
	setOptBacked("xtrace", false, "xtrace", false),
	fixedConstant("zle", false, false),
}

// zshOptionAlias is one of the compat spellings: a second name for an option
// the table already holds, negated where the two names mean opposite states.
type zshOptionAlias struct {
	base string
	inv  bool
}

// zshOptionAliases are the twelve names zsh carries for sh and ksh
// compatibility. Each was identified by flipping it in a real zsh and reading
// which canonical option moved: `setopt dotglob` turns `globdots` on,
// `unsetopt braceexpand` turns `ignorebraces` on, and `setopt nolog` turns
// `histnofunctions` on. `hashall` and `trackall` are two names for one option,
// which is why both land on `hashcmds`.
//
// They resolve to the canonical entry before anything else happens, so they
// share its state exactly and are never what a listing prints — measured, and
// the reason they are a map here rather than entries of their own.
var zshOptionAliases = map[string]zshOptionAlias{
	"braceexpand": {"ignorebraces", true},
	"dotglob":     {"globdots", false},
	"hashall":     {"hashcmds", false},
	"histappend":  {"appendhistory", false},
	"histexpand":  {"banghist", false},
	"log":         {"histnofunctions", true},
	"mailwarn":    {"mailwarning", false},
	"onecmd":      {"singlecommand", false},
	"physical":    {"chaselinks", false},
	"promptvars":  {"promptsubst", false},
	"stdin":       {"shinstdin", false},
	"trackall":    {"hashcmds", false},
}

// zshOptionIndex is the table by canonical name, built once. A linear scan
// answered when the table held twenty-six names; at 185 it is walked twice
// per operand and a startup file writes dozens of them.
var zshOptionIndex = func() map[string]int {
	m := make(map[string]int, len(zshOptions))
	for i, o := range zshOptions {
		m[o.base] = i
	}
	return m
}()

// setOptBacked binds a zsh name to a `set -o` name this shell really applies,
// inverted where zsh's base is the substrate's negation — zsh's `clobber` is
// the same switch as `set -o noclobber`, read from the other end.
func setOptBacked(base string, def bool, opt string, inv bool) zshOption {
	return zshOption{
		base: base, def: def,
		get: func(r *interp.Runner) bool { on, _ := r.NamedOption(opt); return on != inv },
		set: func(r *interp.Runner, on bool) int { return r.ApplyNamedOption(opt, on != inv) },
	}
}

// fixedConstant is a name whose state here never moves and is not the
// substrate's to hold.
func fixedConstant(base string, def, state bool) zshOption {
	return zshOption{
		base: base, def: def,
		get: func(*interp.Runner) bool { return state },
	}
}

// matchBacked binds a zsh name to one of the pattern matcher's run-time
// options, inverted where zsh names the state the matcher's flag turns off:
// `caseglob` on is `GlobFoldsCase` off. These are implemented, not recorded —
// `setopt nullglob` really does delete a word that matched nothing.
func matchBacked(base string, def bool, opt interp.MatchOption, inv bool) zshOption {
	return zshOption{
		base: base, def: def,
		get: func(r *interp.Runner) bool { return r.MatchOption(opt) != inv },
		set: func(r *interp.Runner, on bool) int { r.SetMatchOption(opt, on != inv); return 0 },
	}
}

// switchBacked binds a zsh name to a capability the core holds under no
// option name of its own — the shape matchBacked has, for a switch that is
// not one of the pattern matcher's.
//
// The distinction recorded draws is the point of having this at all: a name
// that has stopped being remembered-and-ignored has to stop saying it is, or
// the listings and `emulate` go on describing a shell that no longer exists.
func switchBacked(base string, def bool, get func(*interp.Runner) bool, set func(*interp.Runner, bool)) zshOption {
	return zshOption{
		base: base, def: def,
		get: get,
		set: func(r *interp.Runner, on bool) int { set(r, on); return 0 },
	}
}

// checkJobsOption is zsh's `checkjobs`: the master over both halves of
// looking at the job table before leaving.
//
// It reads back the *stopped* switch, because that is the one it alone
// governs — the running switch is shared with `checkrunningjobs`, and reading
// it here would report `checkjobs` as off in a session that had only turned
// the narrower name off.
func checkJobsOption() zshOption {
	return zshOption{
		base: "checkjobs", def: true,
		get: (*interp.Runner).ChecksStoppedJobsAtExit,
		set: func(r *interp.Runner, on bool) int {
			r.SetChecksStoppedJobsAtExit(on)
			r.SetChecksRunningJobsAtExit(on && checkRunningJobsBit(r))
			return 0
		},
	}
}

// checkRunningJobsOption is zsh's `checkrunningjobs`, which decides whether
// the master looks at running jobs as well as suspended ones.
//
// Its own bit lives in the recorded store rather than in the core switch,
// which is the only arrangement that survives the master being turned off and
// on again: the core switch is false throughout that, so a shell reading it
// back would forget that this name had been left on. It is not `recorded` —
// something reads it, which is the distinction storeBacked exists to draw.
func checkRunningJobsOption() zshOption {
	o := storeBacked("checkrunningjobs", true)
	store := o.set
	o.set = func(r *interp.Runner, on bool) int {
		code := store(r, on)
		r.SetChecksRunningJobsAtExit(on && r.ChecksStoppedJobsAtExit())
		return code
	}
	return o
}

// checkRunningJobsBit reads that stored bit without going through the table,
// for the master's setter.
func checkRunningJobsBit(r *interp.Runner) bool {
	return !recordedDeviates(r, "checkrunningjobs")
}

// recorded is a name this shell remembers and does not act on. See the four
// kinds at the top of this file: the state is real, readable and reported,
// and nothing in the shell reads it.
func recorded(base string, def bool) zshOption {
	return zshOption{
		base: base, def: def, recorded: true,
		get: func(r *interp.Runner) bool { return def != recordedDeviates(r, base) },
		set: func(r *interp.Runner, on bool) int {
			setRecordedDeviation(r, base, on != def)
			return 0
		},
	}
}

// recordedOver is a recorded name whose state in a *fresh* shell here is not
// the table's default — either because this implementation sits somewhere
// else to begin with, or because the invocation decided it.
//
// The store holds deviations, so the base state has to be supplied rather
// than assumed: `hashdirs` is off here where zsh's compiled-in default is on,
// and `login` and `rcs` are whatever the front end was told. Everything else is
// `recorded`'s bargain unchanged — remembered, reported, and acted on by
// nothing.
func recordedOver(base string, def bool, state func(*interp.Runner) bool) zshOption {
	return zshOption{
		base: base, def: def, recorded: true,
		get: func(r *interp.Runner) bool { return state(r) != recordedDeviates(r, base) },
		set: func(r *interp.Runner, on bool) int {
			setRecordedDeviation(r, base, on != state(r))
			return 0
		},
	}
}

// constantState is the base state of a recorded name this shell holds still.
func constantState(state bool) func(*interp.Runner) bool {
	return func(*interp.Runner) bool { return state }
}

// singleCommandOption is zsh's `singlecommand`, and the one name in this
// table whose answer depends on *where the request came from*.
//
// Measured on zsh 5.9.2, 2026-09-10. `plain.sh` holds three `echo` lines:
//
//	zsh -t plain.sh                 P1 — the option is on, one line runs
//	zsh -o singlecommand plain.sh   P1
//	zsh -o onecmd plain.sh          P1, under the borrowed spelling
//	zsh -t -c $'echo A\necho B'      A and B — a command string is not stopped
//	zsh -t -c 'echo "$-"'           569Xt
//	set -t in a script              can't change option: -t, fatal
//	setopt singlecommand            can't change option: singlecommand
//	unsetopt singlecommand after -t can't change option: singlecommand
//
// So "fixed" means *a running script may not move it* rather than "this shell
// cannot have it on", and the invocation is the other side of that. The state
// is the substrate's `onecmd` — the same switch `set -t` writes in the two
// shells that let a script write it — so the two spellings are one state read
// and written through one seam, and `$-` carries `t` because the substrate
// puts it there.
//
// interp.Semantics.ImmovableOptionsSetAtInvocation is the axis that says this
// shell has the route at all; it is asked by the core for the `-t` letter and
// here for the two names, so a dialect that answers `No` refuses both.
func singleCommandOption() zshOption {
	return zshOption{
		base: "singlecommand", def: false,
		get: func(r *interp.Runner) bool { on, _ := r.NamedOption("onecmd"); return on },
		atInvocation: func(r *interp.Runner, on bool) int {
			return r.ApplyNamedOption("onecmd", on)
		},
	}
}

// storeBacked keeps its state in the same store a recorded name does and is
// not recorded, because something reads it.
//
// The one name in this table with that shape today is `histignorespace`. The
// state has nowhere better to live — the substrate has no `set -o` name for
// it, unlike `histignoredups`, which zsh's `set -h` abbreviates and which is
// therefore a switch this shell's own option machinery already carries — but
// the front end reads it through this dialect's namespace every time it
// accepts a line, so calling it recorded would be claiming less than it does.
// See HistoryStyle in this package.
//
// The distinction is the point: `recorded` means remembered and not acted on,
// and an option that has moved out of that set must stop saying it is in it.
func storeBacked(base string, def bool) zshOption {
	o := recorded(base, def)
	o.recorded = false
	return o
}

// zshRecordedStore is where the recorded options live: the canonical names
// whose state differs from the table's default, in an array under a name no
// script can reach — the shape `zstyle` and `emulate` already use, and for
// the same reason. A subshell deep-copies the Vars table, so `(setopt
// auto_cd)` stays in the subshell exactly as an axis-backed option does.
//
// Deviations rather than states, so that a runner that has never run `setopt`
// holds an empty array and every name reads back at its default.
const zshRecordedStore = ".zsh.setopt"

// recordedDeviates reports whether one recorded name has been moved off its
// default.
func recordedDeviates(r *interp.Runner, base string) bool {
	names, _ := r.GetArray(zshRecordedStore)
	for _, n := range names {
		if n == base {
			return true
		}
	}
	return false
}

// setRecordedDeviation records or clears one name's deviation, keeping the
// store sorted so the array is a function of the set and not of the order the
// rc file happened to write.
func setRecordedDeviation(r *interp.Runner, base string, dev bool) {
	names, _ := r.GetArray(zshRecordedStore)
	out := make([]string, 0, len(names)+1)
	for _, n := range names {
		if n != base {
			out = append(out, n)
		}
	}
	if dev {
		out = append(out, base)
		sort.Strings(out)
	}
	r.SetArray(zshRecordedStore, out)
}

// setRecordedOptions replaces the store wholesale, which is how an emulation
// resets every recorded name to its default at once and how a saved option
// table puts one back. The read half is not here: setRecordedDeviation builds
// a fresh slice on every write rather than sorting the one it found, so a
// saver can hold the store as it stands instead of copying it — see
// optionState.
func setRecordedOptions(r *interp.Runner, names []string) {
	r.SetArray(zshRecordedStore, names)
}

// swapAxes changes semantics copy-on-write: a subshell clone shares the
// vector by pointer, so the mutation goes on a fresh copy and the swap stays
// this runner's own — which is also what makes a subshell's `setopt` stay in
// the subshell.
func swapAxes(r *interp.Runner, change func(*interp.Semantics)) {
	s := *r.Semantics
	change(&s)
	r.Semantics = &s
}

func answer(on bool) interp.Answer {
	if on {
		return interp.Yes
	}
	return interp.No
}

// normalizeOption reduces a spelling to the namespace's canonical form.
func normalizeOption(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), "_", "")
}

// resolveOptionName takes one normalized name to its table entry and the
// direction the spelling asked for. The name is looked up whole first, which
// is what keeps `nomatch` an option rather than a negated `match` — it and
// `notify` are the only two option names that begin with `no`, measured — and
// only then is a single `no` stripped. A compat spelling resolves to its
// canonical entry at either step, so `nophysical` is `chaselinks` off.
func resolveOptionName(name string) (zshOption, bool, bool) {
	if o, inv, ok := exactOptionName(name); ok {
		return o, inv, true
	}
	rest, ok := strings.CutPrefix(name, "no")
	if !ok {
		return zshOption{}, false, false
	}
	o, inv, ok := exactOptionName(rest)
	return o, !inv, ok
}

// exactOptionName looks one spelling up without stripping anything, through
// the alias table first.
func exactOptionName(name string) (zshOption, bool, bool) {
	inv := false
	if a, ok := zshOptionAliases[name]; ok {
		name, inv = a.base, a.inv
	}
	if i, ok := zshOptionIndex[name]; ok {
		return zshOptions[i], inv, true
	}
	return zshOption{}, false, false
}

// conditionOption answers `[[ -o name ]]` out of this namespace rather than
// out of the `set -o` names, which is the whole of why the substrate takes a
// function for it: `[[ -o no_brace_expand ]]` is one question here and three
// unknown names anywhere else.
//
// It is the same lookup `setopt` does, read rather than written, so a name
// this dialect can speak about answers the same way through either — and a
// name it cannot is unknown to both, which is what lets the substrate's axis
// decide what to say about it.
func conditionOption(r *interp.Runner, name string) (on, known bool) {
	o, inverted, ok := resolveOptionName(normalizeOption(name))
	if !ok {
		return false, false
	}
	return o.get(r) != inverted, true
}

// registerSetopt installs the pair, and the namespace they share with the
// condition.
func registerSetopt(r *interp.Runner) {
	r.Register("setopt", setoptBuiltin(true))
	r.Register("unsetopt", setoptBuiltin(false))
	// Each of the three reads the runner it is handed rather than the one
	// this registration ran on: a subshell is a cloned runner that keeps
	// these fields, so closing over `r` here answered every `[[ -o ]]`,
	// listed every `set -o` row and performed every `set -o name` against
	// the shell that spawned the subshell (#1855).
	r.SetOptionNamespace(conditionOption)
	// And the same namespace by its other name. zsh's `set -o` is `setopt`
	// under a POSIX spelling rather than a table of its own — measured,
	// `set -o autocd` then `setopt` reports `autocd`, and `setopt autocd`
	// then `set +o` writes `set -o autocd` — so a `set -o` listing there is
	// 185 rows where every other shell writes a couple of dozen. This
	// dialect wrote the substrate's shared two dozen until #1080: a capture
	// surface recorded a zsh with 23 options, in a vocabulary that shell
	// does not use for output, and said nothing about the 170 that decide
	// what it does.
	r.SetOptionTable(listedOptions, moveOption)
}

// listedOptions is the `set -o` and `set +o` listing: every option in the
// table, in the same order and the same spelling a bare `setopt` uses.
//
// One printed spelling per option, the one that is off by default — measured,
// zsh writes `noaliases`, `allexport`, `noalwayslastprompt` in that order and
// nothing about the twelve borrowed sh and ksh spellings, which it accepts as
// input and never lists. So a row's state is its *deviation from zsh's
// default*, exactly as listZshOptions computes it: `noaliases off` in a shell
// where aliases work, and `autocd on` after `setopt autocd`.
func listedOptions(r *interp.Runner) []interp.ListedOption {
	rows := make([]interp.ListedOption, 0, len(zshOptions))
	for _, o := range zshOptions {
		rows = append(rows, interp.ListedOption{
			Name: spellOption(o.base, !o.def),
			On:   o.get(r) != o.def,
		})
	}
	return rows
}

// moveOption is `set -o name` and `set +o name` reaching this namespace, and
// it decides without speaking: the substrate words the refusal, because the
// same one is worded three ways depending on whether a script, an invocation
// or an inherited value asked. That is the whole difference between this and
// setOption below, which is `setopt`'s own and does speak.
//
// The name is resolved exactly as `setopt` resolves it — case folded,
// underscores ignored, one `no` prefix negating, the twelve compat spellings
// included — because it is the same namespace and a shell with two answers
// for `set -o Err_Exit` and `setopt Err_Exit` would have two namespaces.
func moveOption(r *interp.Runner, name string, on bool) (moved, known bool) {
	o, inverted, ok := resolveOptionName(normalizeOption(name))
	if !ok {
		return false, false
	}
	want := on != inverted
	if o.set != nil {
		return o.set(r, want) == 0, true
	}
	if mover := invocationMover(r, o); mover != nil {
		return mover(r, want) == 0, true
	}
	// Fixed: granted where it is already where it is being asked to be, and
	// refused otherwise — the same bargain setOption strikes, with the
	// sentence left to the caller.
	return o.get(r) == want, true
}

// setOption moves one option by name, reporting what `setopt` would. Shared
// with `emulate`, whose `-o name` and `+o name` are the same request written
// on another builtin's command line.
func setOption(r *interp.Runner, name string, on bool) int {
	o, inverted, ok := resolveOptionName(normalizeOption(name))
	if !ok {
		r.Diagnosef("no such option: %s\n", name)
		return 1
	}
	want := on != inverted
	if o.set != nil {
		return o.set(r, want)
	}
	if mover := invocationMover(r, o); mover != nil {
		return mover(r, want)
	}
	if o.get(r) == want {
		// Already where it was asked to be: granted, the same bargain the
		// substrate's own option table strikes.
		return 0
	}
	r.Diagnosef("can't change option: %s\n", name)
	return 1
}

// invocationMover is the applier a fixed name has for the command line that
// started the shell, and nil everywhere else — for a name without one, for a
// dialect whose axis says this shell has no such route, and for any request
// that did not come from an invocation.
//
// Both option builtins consult it rather than only `set -o`, because the
// front end reaches a `-o name` operand through the table and a `setopt` the
// invocation never runs cannot reach it at all: the flag is the runner's and
// is set for exactly the span of one invocation option.
func invocationMover(r *interp.Runner, o zshOption) func(*interp.Runner, bool) int {
	if o.atInvocation == nil || !r.AtInvocation() {
		return nil
	}
	if r.Semantics.ImmovableOptionsSetAtInvocation != interp.Yes {
		return nil
	}
	return o.atInvocation
}

// setoptBuiltin builds either half; they differ in the direction a bare base
// name means and in what an empty command lists.
func setoptBuiltin(setting bool) interp.Builtin {
	return func(r *interp.Runner, _ context.Context, args []string) int {
		if len(args) == 0 {
			listZshOptions(r, setting)
			return 0
		}
		status := 0
		for _, arg := range args {
			if code := setOption(r, arg, setting); code != 0 {
				status = code
			}
		}
		return status
	}
}

// listZshOptions is the bare command. Every option has one printed spelling —
// the one that is off by default — and `setopt` prints the spellings that are
// on where `unsetopt` prints the ones that are off. The table is already in
// the measured order, which is by canonical name.
func listZshOptions(r *interp.Runner, setting bool) {
	for _, o := range zshOptions {
		if o.get(r) != o.def == setting {
			_, _ = fmt.Fprintf(r.Out(), "%s\n", spellOption(o.base, !o.def))
		}
	}
}

// spellOption writes the name in the direction asked for: the base for the
// state a listing calls on, `no` and the base for the other.
func spellOption(base string, on bool) string {
	if on {
		return base
	}
	return "no" + base
}
