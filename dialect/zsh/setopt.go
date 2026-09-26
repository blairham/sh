// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
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
// The default recorded for each name is zsh's compiled-in default, which is
// its measured state in a non-interactive `zsh -c` with an empty HOME for
// every name but one: `hashdirs` is on by default and off in such a shell,
// which is why `nohashdirs` is the one line a bare `setopt` prints there and
// why it is not a line an *interactive* zsh prints at all (#2351).
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
//     rather than by anything in this package: `histignorespace`, whose state
//     the line editor asks for through this namespace before it records a
//     line; `interactivecomments`, which it asks for before it *parses* one;
//     and `promptsp` and `promptcr`, which it asks for before it draws a
//     prompt. Written `storeBacked(…)`;
//   - **recorded**: a name this shell recognizes and remembers and does not
//     act on. `setopt auto_cd` succeeds, `setopt` then reports `autocd`, and
//     typing a directory name still does not change directory. 127 of the 185
//     are this, and they are marked `recorded(…)` below so the distinction can
//     be read off the table rather than taken on trust.
//
// Names have moved out of "recorded" as this shell learned to read them.
// `histignorespace` above and `histignoredups`, which was already a `set -o`
// backed switch whose state nothing consulted, were the first two: both now
// decide what a session writes to its history file, so both are implemented
// rather than remembered. `interactivecomments` is the most recent and is read
// earlier in the same loop — before the line is parsed rather than after it is
// run — and it decides whether a `#` typed at the prompt opens a comment or is
// a character of the word it stands in (#2537).
// Then `extendedglob`, which now moves the pattern matcher's
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
// `typesetsilent` is the inverse of
// [interp.Semantics.ValuelessDeclarationOfAHeldNameListsIt]: the option on is
// that axis answering No. It moved because powerlevel10k sets it and then
// re-declares `local` names inside loops, so remembering-and-ignoring it wrote
// nine lines to stdout before every prompt (#2033).
//
// `debugbeforecmd` is the one that shows what
// recording costs when the name is wired to something the shell really does.
// It is [interp.Semantics.DebugTrapRunsBeforeTheCommand] — where the DEBUG
// trap fires, ahead of each command or behind it — and remembering it meant
// `setopt debugbeforecmd` and `unsetopt debugbeforecmd` produced
// byte-identical output while the listings went on reporting the difference
// back faithfully (#4473).
//
// `longlistjobs` is [interp.Semantics.JobNoticeNamesThePID]: whether a job
// notice names the job's pid. Recording it meant `setopt longlistjobs`
// reported itself on every surface and the notice went on being written
// without one, which is a disagreement zsh's own `W02jobs.ztst` asks about
// directly (#4491).
//
// `cbases` is [interp.Semantics.IntegerBaseMarkIsCSpelled], which is
// whether `$(( [#16] 108 ))` writes `0x6C` or `16#6C`, and this shell wrote
// the second in both states of it (#4502).
//
// `kshoptionprint` is the one that was not a *behavior* at all but the shape
// of a listing: on, both bare listings become 185 `name<pad>on|off` rows, and
// this shell recorded it and went on writing the deviating names — so
// `emulate ksh`, which turns it on by ksh's own default, printed 11 lines
// where the reference prints 185 (#4529).
//
// `notify` is the one where recording cost a
// *hang* rather than a wrong line. It is
// [interp.Semantics.FinishedJobNoticeArrivesAtOnce] — when a finished job's
// notice is written, the moment the job ends or before the next prompt — and
// with it remembered this shell behaved as `nonotify` in every session, so
// the notice arrived one command late and a driver whose only readable line
// was that notice waited for it for ever (#4524).
//
// `posixtraps` is one of the three most recent, and it is the one whose
// *moment* had to be measured before it could be wired. It is
// [interp.Semantics.ExitTrapIsFunctionLocal] read backwards — the option on
// is that axis answering No — and the axis was asked at the function's
// return, where this option is read when the `trap` command runs. The two
// moments disagree in both directions, so reading it at the return would have
// been a second wrong answer rather than the missing one (#4547).
//
// `magicequalsubst` is the second: the first unquoted `=` in a command
// argument splitting the word, so `--prefix=~/opt` reaches the program as a
// path. It names an expansion this interpreter already performs for an
// assignment's value, which is what made recording it the wrong bargain
// (#4548). Its moment was measured the same way #4547's was and it is the
// **expansion**, not the parse: a function defined while the option was off
// and called while it is on expands, and one defined on and called off does
// not — see TestMagicEqualSubstIsReadWhenTheWordIsExpanded.
//
// `rcexpandparam` is the third, and it is the one whose recording changed how
// many arguments a command received. It is
// [interp.Semantics.ParamExpansionDistributesOverTheWord] — whether `x${a}y`
// on a two-element array is one word or two — and the distribution it asks
// for was already built and already reachable, as `${^a}`, so remembering the
// option meant the machinery sat there and the switch in front of it did
// nothing (#4549).
//
// Its moment was measured before it was wired, for the reason the two above
// it give: the option is read **when the word is expanded** and not when it
// is parsed. A function defined while it was off distributes when it is
// called with it on, and one defined while it was on does not when it is
// called with it off — four rows, in both directions, plus an `eval` of a
// string built while it was off. The `${^a}` half of the same mechanism is
// the opposite: that one is state on the node and is settled at the parse.
//
//
// `errreturn` is the fourth, and its moment was asked in the same breath as
// the three above it: it now takes an implicit `return` at a command that
// failed —
// [interp.Semantics.FailureTakesAnImplicitReturn] — and the deciding moment
// is when the failing statement is *judged*, neither the definition of the
// body nor the entry to the call. It is the function-scoped sibling of
// `errexit`, which was substrate-backed here all along, so the machinery was
// next door and the option simply did not reach it: a script that set it drew
// no diagnostic and ran straight past the command it was asking the shell to
// stop at (#4546).
//
// `autopushd` is the fifth, and it is the one where the moment and the *noun*
// both had to be asked. It now makes every `cd` a `pushd` —
// [interp.Semantics.CdPushesTheDirectoryItLeaves] — and the state that
// decides is the one the option is in **when `cd` runs**, not when it
// finishes: `cd` ends by calling `chpwd`, and a hook that turns the option
// off there does not take the push back, while one that turns it on causes
// none. The noun is the `cd` and not "a directory change", which is the
// reading that breaks here and nowhere in real zsh: `pushd` and `popd` are
// builtins there and prelude functions that call `cd` here, so the option
// would have made `pushd` push twice. The prelude reads the stack into the
// positional parameters ahead of its own `cd` for that reason. And
// `$dirstack` came off the absent roster in the same change, because a stack
// `cd` grows and no parameter to read it from is half a feature (#4592).
//
// Nothing else about the split moved, and 127 is still most of the table.
// The prose said 137 for three conversions after the table said otherwise;
// TestTheOptionsSomethingReadsAreNotRecordedOnly counts the table and is
// what these two numbers have to match.
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
// So `setopt` is the list of deviations from the defaults and `unsetopt` is
// its complement, which is why the first is two lines long and the second is
// 183 in a shell that has changed nothing.
//
// **Whose defaults is the whole of it, and the answer is the mode's rather
// than zsh's.** Both halves of a listing are keyed on the same number — the
// state an option would have in a shell emulating what this one is emulating
// — so an `emulate` moves the baseline under both the membership and the
// spelling, and moves nothing else. That is [listingBase], and #4517 was
// this file reading `o.def` in its place: after `emulate sh` this shell
// printed the 40 names that differ from **zsh's** defaults, which is most of
// what the emulation had just written, where real zsh prints the 8 still away
// from **sh's**.

// zshOption is one name in this dialect's option namespace.
type zshOption struct {
	// base is the canonical spelling: lower case, no underscores, no `no`.
	base string
	// def is the state a zsh default run has, which is what the listings
	// compare against.
	//
	// **Every entry holds zsh's default and not this shell's**, which was
	// not true of this field until #4533. `hashcmds` was the last one that
	// held ours: it is backed by the substrate's `hashall`, no startup letter
	// of this dialect spells that, and `Runner.commandTracking` had no third
	// source to fall through to — so it read off in a shell that hashes, and
	// `def: false` recorded the wrong answer here to keep the listing from
	// showing the deviation it exists to show.
	//
	// Correcting `def` alone would indeed have moved the row rather than
	// removed it, which is what the note here used to say and is why it stood
	// for as long as it did. What closed it was the backing state's own
	// default: `interp.Semantics.CommandTrackingStartsOn` is this dialect
	// answering Yes, so `hashall` reads on in a fresh shell and `def: true`
	// then agrees with it. `interactivecomments` was corrected in #2516,
	// `emacs` in #1858 and `banghist` in #2542; those three had no state
	// behind them, and this one did, which is the whole of the difference.
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
	// immovable reports that this name cannot be moved in *this* shell,
	// whatever set says — so the request is granted where it asks for the
	// state the option already has and refused otherwise, which is the
	// bargain a nil set strikes. Nil for every name whose movability does
	// not depend on the shell.
	//
	// It is a function and not a second bool because the names that need it
	// are movable in one kind of shell and fixed in another: `zle` is freely
	// movable at an interactive prompt and refused to a `-c` script, and one
	// entry has to say both.
	immovable func(*interp.Runner) bool
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
	// speaksItsOwnRefusal marks the entry whose `set` has already complained
	// by the time it answers, so the substrate words nothing further. See
	// interp.OptionRefusedAndSaid, and monitorOption for the one name in the
	// table that is in this state and why it cannot be got out of it.
	speaksItsOwnRefusal bool
	// over is the base state a [recordedOver] name deviates *from*, and nil
	// for every other entry. It is the constructor's own `state` argument,
	// published on the value because two callers need to tell the two kinds
	// of recorded name apart and the closure alone cannot say which is which.
	//
	// applyEmulation is the caller that made it necessary. The store holds
	// deviations, so an emulation resets an ordinary `recorded` name by
	// *dropping* it — which lands the name on `def`. For a name whose base is
	// not `def` that same drop lands it on the base instead, which is the
	// invocation's state rather than the emulation's default: `zsh -f` then
	// `emulate -R zsh` left `rcs` and `hashdirs` off where real zsh turns
	// both back on (#4506).
	over func(*interp.Runner) bool
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
	// AUTO_LIST is read by the line editor on every completion key — see
	// repl.EditorStyle.ListMatchesWithoutASecondKeyOption, which names it —
	// so it is not `recorded`, which means remembered and acted on by
	// nothing. Measured 2026-09-19 through a pseudo-terminal: with it on, a
	// word whose matches agree on nothing past what is typed answers one Tab
	// with the bell and the listing, and with it off the same key writes the
	// bell alone and the listing waits for a second (#3714).
	storeBacked("autolist", true),
	recorded("automenu", true),
	recorded("autonamedirs", false),
	recorded("autoparamkeys", true),
	recorded("autoparamslash", true),
	{
		// AUTO_PUSHD: every `cd` is a `pushd`, so the directory stack grows as
		// the shell moves. Off by default, measured on zsh 5.9.2
		// (aarch64-apple-darwin25.4.0), 2026-09-26, `-f`.
		//
		// It was `recorded` until #4592 — accepted, reported back correctly by
		// `[[ -o autopushd ]]`, `$options[autopushd]` and the `setopt`
		// listing, its letter `-N` wired, and nothing pushed. `pushd`, `popd`
		// and `dirs` maintained a stack the whole time, which is what made the
		// gap a routing one rather than a missing feature.
		//
		// **The noun is the `cd`, and the moment is when it runs** — see
		// [interp.Semantics.CdPushesTheDirectoryItLeaves]. Not "a directory
		// change", which is the reading that breaks: `pushd` and `popd` are
		// builtins in the shell being modeled and prelude *functions* that
		// call `cd` in this one, and `pushd /tmp` under the option pushes one
		// entry there and not two. And not the state when `cd` finishes: a
		// `chpwd` that turns the option off during the `cd` does not take the
		// push back, and one that turns it on does not cause a push.
		base: "autopushd", def: false,
		get: func(r *interp.Runner) bool {
			return r.Semantics.CdPushesTheDirectoryItLeaves == interp.Yes
		},
		set: func(r *interp.Runner, on bool) int {
			setAxis(r, func(s *interp.Semantics) *interp.Answer {
				return &s.CdPushesTheDirectoryItLeaves
			}, answer(on))
			return 0
		},
	},
	recorded("autoremoveslash", true),
	recorded("autoresume", false),
	recorded("badpattern", true),
	// On in a fresh zsh, measured 2026-09-13 across all four surfaces it
	// shows on: `[[ -o banghist ]]`, `[[ -o histexpand ]]`, the `unsetopt`
	// listing and `${options[banghist]}`. parameter.go has needed it to be on
	// since #1527 — its `unset "options[name]"` measurement moves `equals`
	// and `banghist` *off* to show that an unset is a move rather than a
	// reset, which an option already off could not have demonstrated (#2542).
	{
		// BANG_HIST, and since #3093 moving it moves something: it is the
		// switch the expander reads. On by default,
		// measured 2026-09-13 across all four surfaces it shows on — `[[ -o
		// banghist ]]`, `[[ -o histexpand ]]`, the `unsetopt` listing and
		// `${options[banghist]}` — and measured again 2026-09-15 through a
		// pseudo-terminal, where `echo !!` at a fresh zsh prompt expands with
		// nothing configured.
		//
		// The option and the expander are two states here, which is measured
		// rather than a convenience: `[[ -o banghist ]]` in `zsh -c` reports
		// **on** while that same shell expands nothing, because zsh gates the
		// expander on being interactive and leaves the option where it is. So
		// the reading stays the recorded one — on unless something moved it —
		// and moving it also moves the switch the front end reads, so that
		// `unsetopt banghist` at a prompt stops the expansion rather than
		// only being remembered.
		base: "banghist", def: true,
		get: func(r *interp.Runner) bool { return !recordedDeviates(r, "banghist") },
		set: func(r *interp.Runner, on bool) int {
			setRecordedDeviation(r, "banghist", !on)
			r.SetHistoryExpansion(on)
			return 0
		},
	},
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
	// `casematch` is the `=~` operator's and **nothing else**, which is the
	// whole reason it is not the same wire as bash's option of the same
	// name. Measured on zsh 5.9.2, 2026-09-13, with `setopt nocasematch`:
	// `[[ ABC =~ ^abc$ ]]` matches, while `[[ ABC == abc ]]`, `case A in a)`
	// and `v=ABC; ${v//b/X}` are every one of them unmoved. bash's
	// `nocasematch` moves all four (#2622).
	matchBacked("casematch", true, interp.RegexFoldsCase, true),
	recorded("casepaths", false),
	{
		// C_BASES: whether a value written in an output base carries C's
		// spelling of that base — `0x6C` — or zsh's own `16#6C`. Off by
		// default, and implemented rather than recorded since #4502: it was
		// accepted and remembered and the arithmetic output side never read
		// it, so `setopt cbases` changed nothing.
		//
		// Measured on zsh 5.9.2 under `-f`, 2026-09-25, with the option on
		// and off: `$(( [#16] 108 ))` is `0x6C` on and `16#6C` off, and it
		// reaches `typeset -i16 a=108` by the same amount. **Base eight is
		// the control** — `$(( [#8] 8 ))` is `8#10` in both states, which is
		// what says the option is about the spellings C has rather than
		// about bases in general. It moves too once `octal_zeroes` is on as
		// well, to `010`, and that conjunction is where it lives: see
		// [interp.Semantics.IntegerBaseMarkIsCSpelled]. `[##16]` writes no
		// mark at all, so both states agree there.
		//
		// Read off the axis rather than off a stored bit, the arrangement
		// `octalzeroes` and `debugbeforecmd` use — so `(setopt cbases)`
		// stays in the subshell and `emulate -R` puts it back with the rest
		// of the vector.
		base: "cbases", def: false,
		get: func(r *interp.Runner) bool {
			return r.Semantics.IntegerBaseMarkIsCSpelled == interp.Yes
		},
		set: func(r *interp.Runner, on bool) int {
			setAxis(r, func(s *interp.Semantics) *interp.Answer {
				return &s.IntegerBaseMarkIsCSpelled
			}, answer(on))
			return 0
		},
	},
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
	// Implemented rather than recorded since #2883, and it could not be
	// implemented before: while this dialect bound the operators C's way by
	// default, the option had nothing to switch *to* and moving it either
	// direction answered the same numbers. See
	// syntax.ArithPrecedenceShiftsAndBitwiseBindTighter for the two ladders
	// and zsh.go for the measurement that put this shell on the other one.
	switchBacked("cprecedences", false, cPrecedences, setCPrecedences),
	recorded("cshjunkiehistory", false),
	recorded("cshjunkieloops", false),
	recorded("cshjunkiequotes", false),
	nullCommandOption("cshnullcmd"),
	recorded("cshnullglob", false),
	{
		// DEBUG_BEFORE_CMD: whether the DEBUG trap runs ahead of each
		// command or behind it. On by default, and implemented rather than
		// recorded since #4473 — it was accepted and then ignored, so both
		// states of the option produced byte-identical output and the
		// *before* reading was the only one this shell had.
		//
		// Measured on zsh 5.9.2 under `-f`, 2026-09-25, an action printing
		// `$LINENO` over a `trap` on line 2 and `print A` on line 3: `3 A`
		// with it set, and `2 A 3` with it unset — the firings keep their
		// lines and their order and only move around the command. A
		// compound's head goes behind the whole construct, which is the same
		// rule and not a second: `if` on line 3 with a body on line 5 writes
		// `3 3 5 C` set, and `3 C 5 3` unset.
		//
		// Read off the axis rather than off a stored bit, the arrangement
		// `globsubst`, `multios` and `shwordsplit` use — so `(unsetopt
		// debugbeforecmd)` stays in the subshell and `emulate -L` puts it
		// back with the rest of the vector.
		base: "debugbeforecmd", def: true,
		get: func(r *interp.Runner) bool {
			return r.Semantics.DebugTrapRunsBeforeTheCommand != interp.No
		},
		set: func(r *interp.Runner, on bool) int {
			setAxis(r, func(s *interp.Semantics) *interp.Answer {
				return &s.DebugTrapRunsBeforeTheCommand
			}, answer(on))
			return 0
		},
	},
	recorded("dvorak", false),
	// Default off, which is real zsh's: measured 2026-09-11 at a terminal,
	// `[[ -o emacs ]]` answers 1 in an interactive session that has not
	// selected a mode, and a bare `setopt` says nothing about the name. It
	// was recorded as on here, which was one of four entries holding *this*
	// shell's state where zsh's differed — and the only one of the four the
	// state behind it could be corrected for (#1858).
	editingOption("emacs", "emacs"),
	recorded("equals", true),
	setOptBacked("errexit", false, "errexit", false),
	{
		// ERR_RETURN: a failing command executes an implicit `return` where
		// `ERR_EXIT` would execute an `exit`. Off by default, measured on
		// zsh 5.9.2 (aarch64-apple-darwin25.4.0), 2026-09-25, `-f`.
		//
		// It was `recorded` until #4546 — accepted, reported back correctly
		// by `[[ -o errreturn ]]`, `$options[errreturn]` and the `setopt`
		// listing, and acted on by nothing. That is the shape that reads as
		// working from every surface but the one the option is for: a script
		// that sets it gets no diagnostic and runs straight past the command
		// that failed.
		//
		// **The noun is `return` and not "the enclosing function"** — see
		// [interp.Semantics.FailureTakesAnImplicitReturn], which holds the
		// measured table. The manual's own example is a function, and three
		// of the rows that decide the rule are not: the top level of a
		// script, a subshell and a sourced file each end the way a written
		// `return` ends them.
		base: "errreturn", def: false,
		get: func(r *interp.Runner) bool {
			return r.Semantics.FailureTakesAnImplicitReturn == interp.Yes
		},
		set: func(r *interp.Runner, on bool) int {
			setAxis(r, func(s *interp.Semantics) *interp.Answer {
				return &s.FailureTakesAnImplicitReturn
			}, answer(on))
			return 0
		},
	},
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
			setAxis(r, func(s *interp.Semantics) *interp.Answer { return &s.GlobExpansionResults }, answer(on))
			return 0
		},
	},
	// HASH_CMDS: permission to remember where a command was found. **On by
	// default**, measured on zsh 5.9.2, 2026-09-25, in `zsh -f -c` across
	// every surface that names it — `[[ -o hashcmds ]]` is 0,
	// `${options[hashcmds]}` is `on`, a bare `unsetopt` writes `nohashcmds`,
	// and `set -o` writes `nohashcmds            off`.
	//
	// The state and the hashing are two questions and only the first was ever
	// wrong here (#4533). Both shells hash in every state of the option, so a
	// probe that ran a command and read `hash` back agrees under either
	// default; and the option is not inert either, since `unsetopt hashcmds;
	// ls >/dev/null; hash` is empty in both. What discriminates is holding
	// the hashing fixed and reading the report, which parted on all four
	// surfaces above while `hash` listed the same entry in both shells.
	//
	// Backed by `hashall`, whose default in this dialect is
	// [interp.Semantics.CommandTrackingStartsOn] — so the report and the
	// switch behind it cannot disagree again, which is what `def: false`
	// beside a `false` backing state had been papering over.
	setOptBacked("hashcmds", true, "hashall", false),
	// `hashdirs` follows **interactive**, the same shape `zle` has and
	// measured the same way on zsh 5.9.2, 2026-09-12: `zsh -f script` and
	// `zsh -f -c …` report it off, `zsh -f -i script` reports it on with
	// or without a terminal. Its compiled-in default is on, so the state
	// being off is what puts `nohashdirs` in a `-c` shell's listing and
	// what takes the line back out of an interactive one's (#2351).
	//
	// Recorded, and it stays recorded: this shell remembers no directory
	// it found a command in, so neither state promises anything. What was
	// wrong was the report.
	recordedOver("hashdirs", true, func(r *interp.Runner) bool { return r.Interactive }),
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
	// HUP: whether a session that is leaving sends SIGHUP to the jobs it is
	// about to abandon, and says how many. Not recorded since #4509 — the
	// name is a real switch now, and it is the one bash already reaches
	// under `shopt -s huponexit` rather than a second of its own. See
	// interp.Runner.SendsHangupToJobsAtExit for the switch and the three
	// HangupAtExit axes for where this shell parts from that one: no login
	// shell required, stopped jobs left alone, and the whole of it before
	// the EXIT trap.
	switchBacked("hup", true,
		(*interp.Runner).SendsHangupToJobsAtExit,
		(*interp.Runner).SetSendsHangupToJobsAtExit),
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
		// And a fact the command line may state, which is the other side of
		// "read and not set": real zsh refuses `setopt interactive` to a
		// running script and **takes** `zsh -o interactive`. Measured
		// 2026-09-16 on zsh 5.9.2: `zsh -f -o interactive -c 'echo $-'` is
		// `569XZfi` against `569Xf` with nothing asked, so `i` arrives and
		// `Z` arrives with it — `zle` is read over this same state, so
		// turning the shell interactive turns the editor on without this
		// saying so (#3154).
		atInvocation: func(r *interp.Runner, on bool) int {
			r.Interactive = on
			return 0
		},
	},
	// Off in a default zsh, and recorded on here until #2516. Measured
	// 2026-09-12 on zsh 5.9.2 with an empty HOME: `zsh -f -c '[[ -o
	// interactivecomments ]]'` exits 1, `set -o` prints
	// `interactivecomments   off`, and a bare `unsetopt` names it. So the
	// default the listings compare against is off.
	//
	// The old value was this shell's own state rather than zsh's — the `#`
	// was honored wherever it was written — and that is not what a recorded
	// default is for.
	//
	// Claiming otherwise was not cosmetic. A syntax highlighter reads this
	// option to pick a tokenizer; told it is on, it splits the line
	// comment-aware and classifies a bare `ls` as a comment, so every command
	// typed came out in the comment style (#2516).
	//
	// The state is now *read* as well as remembered, which is what took this
	// out of `recorded` and into `storeBacked` beside `histignorespace`: the
	// line editor asks for it before each line it has been typed and reads a
	// `#` as an ordinary character while it is off, which is zsh's own answer
	// and was #2537. The store is still where the state lives — nothing in
	// this package acts on it — and the front end is what acts.
	storeBacked("interactivecomments", false),
	{
		base: "ksharrays", def: false,
		// Read off the base, which is the axis the name is about; the four
		// that move with it are in ksharrays.go and every one of them
		// answers the same way, so any of the five would report the same
		// state.
		get: func(r *interp.Runner) bool { return r.Semantics.ArrayBaseIsZero == interp.Yes },
		set: func(r *interp.Runner, on bool) int {
			// The five move together and setKshArrays is their only
			// writer once the shell is running — the dialect's own
			// Semantics places them once at construction and nothing
			// else assigns them — so the base answers for all five and
			// a vector already holding this value has nothing to swap.
			// See setAxis for why that matters on a real rc.
			if r.Semantics.ArrayBaseIsZero == answer(on) {
				return 0
			}
			swapAxes(r, func(s *interp.Semantics) { setKshArrays(s, on) })
			return 0
		},
	},
	recorded("kshautoload", false),
	recorded("kshglob", false),
	// KSH_OPTION_PRINT: **the shape of the two bare listings**, and nothing
	// else. On, `setopt` and `unsetopt` each write every option in the table
	// as `name<pad>on|off` instead of writing the deviating names and the
	// rest; the two then print the same 185 rows as each other and as
	// `set -o`, byte for byte. Measured on zsh 5.9.2 under `-f`, 2026-09-25:
	// `setopt kshoptionprint; setopt` and `… set -o` and `… unsetopt` are
	// three identical 185-line listings.
	//
	// **The state of this one option decides the shape** — that is the noun,
	// and it is not the emulation. Two cases hold it fixed while the mode
	// moves and the shape does not: `emulate sh; setopt kshoptionprint;
	// setopt` is 185 long rows in a mode that is not ksh, and `emulate ksh;
	// unsetopt kshoptionprint; setopt` is the 12 bare names in a mode that
	// is. Reading the *mode* would have agreed with reading the option on
	// every row where nobody touches it, because ksh's default is the only
	// one that turns it on.
	//
	// It composes with [emulationDefault] rather than replacing it: the
	// shape is this option's and the baseline every row is measured against
	// is still the mode's, which is why the long form under `emulate sh`
	// writes `promptpercent on` where the zsh-mode one writes
	// `nopromptpercent off` for the same unmoved state (#4517, #4529).
	//
	// Named for ksh93 and not shaped like it: `set -o` in AT&T ksh 93u+ 2012
	// opens with a `Current option settings` header, pads to 25 and lists 33
	// names. What this option produces is zsh's own `set -o` table.
	//
	// `storeBacked` because the state lives in the recorded store and
	// listZshOptions reads it; it was `recorded` until #4529, which is why
	// `emulate ksh` printed 11 names where the reference prints 185 rows.
	storeBacked("kshoptionprint", false),
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
	{
		// LONG_LIST_JOBS: whether a job *notice* names the job's pid — the
		// long row `jobs -l` writes, rather than the short one `jobs`
		// writes. Off by default, and implemented rather than recorded since
		// #4491: it was accepted and then ignored, so both states of it
		// wrote `[1]  + done       :` where the reference writes the pid in
		// one of them.
		//
		// Measured on zsh 5.9.2 under `-fiV +Z` on a pseudo-terminal,
		// 2026-09-25. The noun is the notice: with the option on, a job that
		// ended is `[1]  + 96801 done       :`, one a signal killed is
		// `[1]  + 97103 terminated  sleep 30`, a ^Z is
		// `[1]  + 98875 suspended  sleep 5` where the short form is the
		// sentence `zsh: suspended  sleep 5`, `bg` writes
		// `[1]  + 258 continued  sleep 5` and `fg` writes
		// `[1]  + 63526 running    sleep 3`. A `jobs` listing of the very
		// same suspended job is `[1]  + suspended  sleep 5` in **both**
		// states, and `jobs -l` is long in both — so the option is keyed on
		// the surface and not on the job, the state or the letter.
		//
		// Read off the axis rather than off a stored bit, the arrangement
		// `globsubst`, `multios` and `shwordsplit` use — so
		// `(setopt longlistjobs)` stays in the subshell and `emulate -L`
		// puts it back with the rest of the vector.
		base: "longlistjobs", def: false,
		get: func(r *interp.Runner) bool {
			return r.Semantics.JobNoticeNamesThePID == interp.Yes
		},
		set: func(r *interp.Runner, on bool) int {
			setAxis(r, func(s *interp.Semantics) *interp.Answer {
				return &s.JobNoticeNamesThePID
			}, answer(on))
			return 0
		},
	},
	{
		// MAGIC_EQUAL_SUBST: **the first unquoted `=` in a word**, and not
		// the shape of what stands in front of it. On, every argument of the
		// form `something=value` gets the tilde treatment an assignment's
		// value already gets, so `configure --prefix=~/opt` reaches the
		// program as a path. Off — which is zsh's default and this one's —
		// the two characters go through as written.
		//
		// The noun is that one `=`, and the two obvious misreadings agree
		// with it nearly everywhere. It is not "a word shaped like an
		// assignment": `--opt=~` and `1abc=~` expand under the option and
		// neither is a name. And it is not the *last* `=`: `a=b=~` is left
		// alone, because the split is at offset 1 and the `~` then opens
		// neither the value nor one of its colon segments, while `a=b=~:~`
		// moves only the second tilde. Measured on zsh 5.9.2
		// (aarch64-apple-darwin25.4.0) under `-f`, 2026-09-25, with
		// `HOME=/Users/testhome`.
		//
		// What the value gets is the assignment's own treatment and not a
		// copy of it — head, colons, `~+`, `~-`, `~user`, `=cmd` — which is
		// why this is one axis on the word pipeline rather than a second
		// expansion. Over 24 written values the command word and the
		// assignment statement come to the same string in the reference on
		// every row.
		//
		// [interp.Semantics.TheFirstUnquotedEqualsInAWordOpensATildeContext]
		// holds the panel, the quoting rows and the three roads that are
		// written down rather than answered. It was `recorded` until #4548,
		// which is why `print -r -- a=~` under the option wrote a tilde.
		base: "magicequalsubst", def: false,
		get: func(r *interp.Runner) bool {
			return r.Semantics.TheFirstUnquotedEqualsInAWordOpensATildeContext == interp.Yes
		},
		set: func(r *interp.Runner, on bool) int {
			setAxis(r, func(s *interp.Semantics) *interp.Answer {
				return &s.TheFirstUnquotedEqualsInAWordOpensATildeContext
			}, answer(on))
			return 0
		},
	},
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
	monitorOption(),
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
			setAxis(r, func(s *interp.Semantics) *interp.Answer { return &s.RedirectsUseEveryTarget }, answer(on))
			return 0
		},
	},
	{
		base: "nomatch", def: true,
		get: func(r *interp.Runner) bool { return r.Semantics.GlobNoMatchIsError == interp.Yes },
		set: func(r *interp.Runner, on bool) int {
			setAxis(r, func(s *interp.Semantics) *interp.Answer { return &s.GlobNoMatchIsError }, answer(on))
			return 0
		},
	},
	{
		// NOTIFY: **when** a finished job's notice is written — the moment
		// the job ends, or held for the next prompt. On by default here and
		// off in the rest of the panel, and zsh's own manual puts it as
		// "report the status of background jobs immediately, rather than
		// waiting until just before printing a prompt".
		//
		// Implemented rather than recorded since #4524. It was accepted and
		// ignored, so this shell behaved as `nonotify` in every session: a
		// finished job's notice arrived one command late, and the two states
		// of the option produced byte-identical output.
		//
		// The noun is the option and not the job. Measured 2026-09-25 on a
		// pseudo-terminal against zsh 5.9.2, `sleep 0.4 &` and no signal
		// anywhere — with it on the row lands with nothing typed, and lands
		// *inside* a `sleep 1.5; print FGDONE` ahead of `FGDONE`; with it off
		// both cases hold the row until after the next command's output.
		// Neither half is about what the job was or how it ended.
		//
		// Read off the axis rather than off a stored bit, the arrangement
		// `globsubst`, `octalzeroes` and `debugbeforecmd` use — so
		// `(unsetopt notify)` stays in the subshell and `emulate -L` puts it
		// back with the rest of the vector.
		base: "notify", def: true,
		get: func(r *interp.Runner) bool {
			return r.Semantics.FinishedJobNoticeArrivesAtOnce != interp.No
		},
		set: func(r *interp.Runner, on bool) int {
			setAxis(r, func(s *interp.Semantics) *interp.Answer {
				return &s.FinishedJobNoticeArrivesAtOnce
			}, answer(on))
			return 0
		},
	},
	matchBacked("nullglob", false, interp.UnmatchedPatternIsEmpty, false),
	recorded("numericglobsort", false),
	{
		// zsh's OCTAL_ZEROES, and the one name in this table that turns the
		// *rest of the panel's* arithmetic back on. A leading zero is not a
		// base here — `$(( 010 ))` is ten, alone among the six columns —
		// and this option is how a script written for bash, ksh93 or dash
		// asks for the reading those shells have.
		//
		// Semantics.ArithLeadingZeroIsOctal is that disagreement and this is
		// the only switch in the panel that moves it, which is why the
		// option reads off the axis rather than off a stored bit: a
		// `(setopt octal_zeroes)` stays in the subshell exactly as every
		// other axis-backed name here does.
		//
		// Measured 2026-09-17 on zsh 5.9.2, from a script file under
		// `env -i PATH=/usr/bin:/bin LC_ALL=C`, with the option set and
		// again without it:
		//
		//	                        set      unset
		//	$(( 010 ))               8        10
		//	$(( 0100 ))             64       100
		//	$(( 0_10 ))              8        10
		//	k=010; $(( k ))          8        10
		//	typeset -i d=010      8#10        10
		//	let "x=010"; $x       8#10        10
		//	$(( 0x10 ))             16        16
		//	$(( 08 ))            refused       8
		//
		// Four of those rows are other axes agreeing with this one rather
		// than second questions, and every one of the four is already
		// answered the way the column reads: the value out of a name goes
		// through the same reader as the literal
		// (ArithStoredValueReadsALeadingZeroAsDecimal, No), so does an
		// assignment to an integer name
		// (IntegerAssignmentReadsALeadingZeroAsDecimal, No) and so does
		// `let` (LetReadsALeadingZeroAsDecimal, No), and a digit the base
		// cannot hold is refused (ArithInvalidOctalDigitIsError, Yes). The
		// `8#10` in the two integer rows is this shell's own base-carrying
		// display and not a different number — the value is eight, written
		// in the base it was read in.
		//
		// A prefix is untouched in both directions: `0x10` is sixteen with
		// the option set and without it, so this is the bare leading zero
		// and not radix reading generally.
		//
		// It was `recorded` until #2884 — accepted, remembered and acted on
		// by nothing, which is the shape that reads as working: a script
		// that sets the option draws no diagnostic and gets the other
		// shells' number wrong, silently, in a place where both answers are
		// plausible and file modes are written this way.
		base: "octalzeroes", def: false,
		get: func(r *interp.Runner) bool {
			return r.Semantics.ArithLeadingZeroIsOctal == interp.Yes
		},
		set: func(r *interp.Runner, on bool) int {
			setAxis(r, func(s *interp.Semantics) *interp.Answer { return &s.ArithLeadingZeroIsOctal }, answer(on))
			return 0
		},
	},
	recorded("overstrike", false),
	recorded("pathdirs", false),
	recorded("pathscript", false),
	setOptBacked("pipefail", false, "pipefail", false),
	recorded("posixaliases", false),
	recorded("posixargzero", false),
	{
		// zsh's POSIX_BUILTINS, and the three things it does that this shell
		// can speak about.
		//
		// The first: with it on, `command name` reaches the builtin of that
		// name, which is what `command` means in POSIX and in the other four
		// dialects. With it off — a plain zsh — the word asks for an
		// external program alone, so `command set -o globstar` is `command
		// not found: set` at 127 and the option is never set.
		//
		// The second: an assignment written in front of a special builtin is
		// still set on the next line. Measured 2026-09-13 —
		// `zsh -o posixbuiltins -c 'FOO=1 : ; echo "[$FOO]"'` is `[1]` where
		// a plain zsh prints `[]`, and `unsetopt posixbuiltins` puts the
		// transient reading back. This option is the *only* door to it in
		// zsh: a zsh invoked as `sh` has POSIX_BUILTINS on, and turning it
		// off there makes the prefix transient again, so the name is a
		// starting position for this option rather than a second mechanism
		// beside it. `FOO=1 true` is the control and stays transient either
		// way, which is what keeps this the special-builtin rule and not the
		// prefix rule as a whole (#2659).
		//
		// The third: `getopts` gives up the rest of a clustered word when it
		// refuses a letter in it, and counts the word there and then.
		// Measured 2026-09-25 on zsh 5.9.2, `getopts ab o` called until it
		// fails over `set -- -axb -b`, the letter and OPTIND after each:
		//
		//	no_posix_builtins   a 1   ? 1   b 1   b 2
		//	posix_builtins      a 1   ? 2   b 2
		//
		// The `b` inside `-axb` is reported in the first row and never in
		// the second, so this is a letter the script stops seeing and not
		// only a number that moves. A missing argument is the same question
		// at a word that is already spent — `getopts ba: o` over
		// `set -- -ba` leaves OPTIND at 1 with the option off and 2 with it
		// on — while a letter that was *accepted* still lags either way,
		// which is what keeps this apart from the counting axis it sits
		// beside. `emulate sh` produces the second row through this entry
		// and `bash --posix` produces the first, so it is this option rather
		// than POSIX mode at large. See Semantics.GetoptsErrorEndsTheWord
		// and zsh's own B10getopts test (#4474).
		//
		// All three read off their axis rather than off a stored bit, which
		// is what makes `(setopt posixbuiltins)` stay in the subshell — the
		// same arrangement `shwordsplit` and `globsubst` use. The state is
		// reported from the first of the two because a shell invoked as `sh`
		// reaches the second through interp.Runner.SetPosixMode without
		// passing through here, and an option reporting itself on from a
		// field somebody else wrote would say `setopt` had run when it had
		// not.
		base: "posixbuiltins", def: false,
		get: func(r *interp.Runner) bool {
			return r.Semantics.CommandReachesABuiltin == interp.Yes
		},
		set: func(r *interp.Runner, on bool) int {
			setAxis(r, func(s *interp.Semantics) *interp.Answer { return &s.CommandReachesABuiltin }, answer(on))
			setAxis(r, func(s *interp.Semantics) *interp.Answer {
				return &s.AssignmentPrefixPersistsOnSpecialBuiltin
			}, answer(on))
			setAxis(r, func(s *interp.Semantics) *interp.Answer {
				return &s.GetoptsErrorEndsTheWord
			}, answer(on))
			return 0
		},
	},
	recorded("posixcd", false),
	recorded("posixidentifiers", false),
	recorded("posixjobs", false),
	recorded("posixstrings", false),
	{
		// POSIX_TRAPS: whether an EXIT trap set inside a function is the
		// function's, firing at its return, or the shell's, firing when the
		// shell exits. Off by default here, which is zsh's own answer and
		// the one nobody else in the panel gives; on is what POSIX says and
		// what the other four do without being asked.
		//
		// It is [interp.Semantics.ExitTrapIsFunctionLocal] read backwards —
		// the option on is that axis answering No — and it was recorded and
		// inert until #4547: `setopt posixtraps` succeeded, `[[ -o
		// posixtraps ]] `agreed, and the trap went on firing at the return
		// in both states.
		//
		// What lifts it above an opt-in curiosity is that nobody has to type
		// it. `posixtraps` is in emulationAlwaysReset and sh's and ksh's
		// defaults for it are on, so `emulate sh` — a common opening line in
		// a zsh function library — asks for POSIX trap timing, and a cleanup
		// handler that ran before the rest of the script instead of at the
		// end is an ordering bug rather than a cosmetic one.
		//
		// Measured on zsh 5.9.2 (`/opt/homebrew/bin/zsh`, `-f`), 2026-09-25,
		// over `f() { trap 'print EXITTRAP' EXIT; print in-f }; f; print
		// after-f`: `in-f after-f EXITTRAP` with the option on and `in-f
		// EXITTRAP after-f` with it off, and `emulate sh` in place of the
		// `setopt` gives the first.
		//
		// Only EXIT. Measured the same day, a USR1 trap set in a function is
		// still the function's to set and the shell's to keep in both states,
		// and a ZERR trap set in a function still fires for a failure after
		// the return in both — so this name moves one condition and not the
		// idea of a trap being function-scoped, which is `localtraps`.
		//
		// Read off the axis rather than off a stored bit, the arrangement
		// `globsubst`, `multios` and `posixbuiltins` use — so `(setopt
		// posixtraps)` stays in the subshell and `emulate -L` puts it back
		// with the rest of the vector.
		base: "posixtraps", def: false,
		get: func(r *interp.Runner) bool {
			return r.Semantics.ExitTrapIsFunctionLocal == interp.No
		},
		set: func(r *interp.Runner, on bool) int {
			setAxis(r, func(s *interp.Semantics) *interp.Answer {
				return &s.ExitTrapIsFunctionLocal
			}, answer(!on))
			return 0
		},
	},
	recorded("printeightbit", false),
	recorded("printexitvalue", false),
	recorded("privileged", false),
	recorded("promptbang", false),
	// PROMPT_CR and PROMPT_SP are read by the line editor every time it
	// draws a prompt — `repl.EditorStyle` names them and `Shell.dialectOption`
	// asks this namespace for their state — so neither is `recorded`, which
	// means remembered and acted on by nothing.
	//
	// They were left as `recorded` during #2502 as a workaround rather than
	// an answer: `emulate` reset every non-recorded name, so calling them
	// what they are would have made `setopt nopromptsp; emulate sh` turn the
	// mark back on. #2515 took the reset off that distinction — both names
	// are measured as ones a bare `emulate` leaves alone — so the workaround
	// is no longer buying anything and the entries can say what is true.
	storeBacked("promptcr", true),
	recorded("promptpercent", true),
	storeBacked("promptsp", true),
	recorded("promptsubst", false),
	recorded("pushdignoredups", false),
	recorded("pushdminus", false),
	recorded("pushdsilent", false),
	recorded("pushdtohome", false),
	{
		// RC_EXPAND_PARAM: whether a parameter expansion that writes no `^`
		// of its own distributes over the word it stands in. `a=(1 2);
		// x${a}y` is the two words `x1y x2y` on and the one word `x1 2y`
		// off, so what it moves is the **argv word count**. Off by default,
		// and implemented rather than recorded since #4549: it was accepted
		// and remembered and the word builder never read it.
		//
		// The per-expansion spelling — `${^a}`, and `${^^a}` back off — has
		// worked since #1517, so this option is the *default* that spelling
		// overrides rather than a second mechanism. Measured on zsh 5.9.2
		// under `-f`, 2026-09-25, and the pair is the discriminating one:
		// `x${^a}y` is `x1y x2y` with the option **off**, and `x${^^a}z${a}`
		// is `x1 2z1 2z2` with it **on** — the doubled caret lays its own
		// span in while the plain one beside it distributes. One mechanism,
		// read parity-first; see interp/rcexpandflag.go, which holds the
		// only implementation of the distribution itself.
		//
		// **The subject is the parameter expansion, not the fields.** Same
		// command, same field count, same word shape: `x$(echo p q)y` is
		// `xp qy` in both states, while `x${(f)"$(printf 'p\nq\n')"}y` is
		// `xp qy` off and `xpy xqy` on. Only the one a `${…}` wraps moves.
		//
		// Read off the axis rather than off a stored bit, the arrangement
		// `octalzeroes`, `debugbeforecmd` and `cbases` use — so `(setopt
		// rcexpandparam)` stays in the subshell and `emulate -R` puts it
		// back with the rest of the vector.
		base: "rcexpandparam", def: false,
		get: func(r *interp.Runner) bool {
			return r.Semantics.ParamExpansionDistributesOverTheWord == interp.Yes
		},
		set: func(r *interp.Runner, on bool) int {
			setAxis(r, func(s *interp.Semantics) *interp.Answer {
				return &s.ParamExpansionDistributesOverTheWord
			}, answer(on))
			return 0
		},
	},
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
	shinStdinOption(),
	nullCommandOption("shnullcmd"),
	recorded("shoptionletters", false),
	recorded("shortloops", true),
	recorded("shortrepeat", false),
	{
		base: "shwordsplit", def: false,
		get: func(r *interp.Runner) bool { return r.Semantics.SplitParamExpansion == interp.Yes },
		set: func(r *interp.Runner, on bool) int {
			setAxis(r, func(s *interp.Semantics) *interp.Answer { return &s.SplitParamExpansion }, answer(on))
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
			setAxis(r, func(s *interp.Semantics) *interp.Answer { return &s.ValuelessDeclarationOfAHeldNameListsIt }, answer(!on))
			return 0
		},
	},
	recorded("typesettounset", false),
	setOptBacked("unset", true, "nounset", true),
	setOptBacked("verbose", false, "verbose", false),
	editingOption("vi", "viins"),
	recorded("warncreateglobal", false),
	recorded("warnnestedvar", false),
	setOptBacked("xtrace", false, "xtrace", false),
	zleOption(),
}

// zleOption is zsh's `zle`: whether the shell has its line editor.
//
// It follows **interactive** and not the terminal, which is the measurement
// that matters and is not what the manual's wording suggests: `zsh -i -c …`
// with no terminal anywhere reports it *on*, and `zsh -c …` on a pseudo-
// terminal reports it *off*. Both real builds on this machine agree — 5.9 and
// 5.9.2 — so what the name is about is the kind of shell, not the fds.
//
// So it is the shape `monitor` has and not the shape `shinstdin` has: on by
// default at an interactive prompt and freely movable there, off and refused
// to a script. Measured in an interactive zsh, where `setopt zle` and
// `unsetopt zle` are both granted at 0 and the option really moves; and in a
// `-c` shell, where `setopt zle` is `can't change option: zle` at 1 — which
// is the half this table already had, and the half that made the whole name a
// constant.
//
// That constant was the entire reason powerlevel10k's instant prompt never
// fired here. Its cached prompt is guarded by
// `[[ -t 0 && -t 1 && -t 2 && -o interactive && -o zle && -o no_xtrace ]]`,
// every clause of which we answered as real zsh does except this one, so the
// file returned at its first line and a real configuration took 400ms to a
// prompt where real zsh takes 33ms — not because anything was slow, but
// because real zsh was printing a prompt it had already rendered (#2121).
//
// The state lives in the recorded store over a base of "is this shell
// interactive", so a subshell keeps its own answer exactly as an axis-backed
// option does. It is not `recorded`: something reads this one.
func zleOption() zshOption {
	o := recordedOver("zle", false, func(r *interp.Runner) bool { return r.Interactive })
	o.recorded = false
	o.immovable = func(r *interp.Runner) bool { return !r.Interactive }
	move := o.set
	// Turning it **on** needs a terminal as well as a prompt, and turning it
	// off does not. That pair is only reachable in a shell that is
	// interactive with nothing to edit — which is exactly what
	// `zsh -o interactive` on a pipe makes, so the state arrived with #3154
	// and the answer is measured rather than inherited. zsh 5.9.2,
	// 2026-09-16: `zsh -f -o interactive -c 'setopt zle'` is
	// `can't change option: zle` at 1 while `unsetopt zle` in the same shell
	// is a silent 0 that really takes the `Z` out of `$-`.
	//
	// Written on the entry's own set rather than through `immovable`,
	// because that answers one way for both directions and this splits on
	// the direction. The invocation route never reaches here: its hook is
	// consulted first, and `zsh -o zle` is granted with no terminal in
	// sight.
	o.set = func(r *interp.Runner, on bool) int {
		if on && !r.Terminal {
			r.Diagnosef("can't change option: zle\n")
			return 1
		}
		return move(r, on)
	}
	// And the command line takes the name whatever the shell is, which is
	// not the same as moving it. Measured 2026-09-16 on zsh 5.9.2:
	// `zsh -f -o zle -c '[[ -o zle ]]'` is status 0 for the request and **1**
	// for the read — the option is granted and stays off — while
	// `zsh -f -o interactive +o zle -c 'echo $-'` is `569Xfi`, the `Z` that
	// interactive brought taken away again. So the request is refused
	// nowhere on this route and honored only where the editor could run: the
	// state is "interactive, unless a request has said otherwise", and a
	// shell with no prompt has no editor for `-o zle` to turn on (#3154).
	o.atInvocation = func(r *interp.Runner, on bool) int {
		if r.Interactive {
			return move(r, on)
		}
		return 0
	}
	return o
}

// monitorOption is zsh's `monitor`, which is job control and is the same
// switch `set -m` moves — so the two spellings are one state read and written
// through one seam. Granted in both directions where the shell has a terminal
// and refused with zsh's own wording where it has none, which is
// Semantics.MonitorNeedsATerminal and not a question about being interactive.
// See interp/setoptions.go for the panel and
// TestSetoptMonitorNeedsATerminalAndNotAPrompt for the measurement.
//
// The command line is the third answer and the one #3154 was about: real zsh
// **takes** `zsh -o monitor` at 0 with nothing on standard error, and
// `[[ -o monitor ]]` in that shell is still 1. Measured 2026-09-16 on zsh
// 5.9.2, with and without `-o interactive` beside it, in both directions.
// Granted and inert, which is neither the refusal a script gets nor a move —
// the shape interp.Runner.AddInertSetOptions gives a whole name, here for one
// route of one name.
// It is also the one entry in this table that **speaks**, and it cannot be
// made not to: the state behind the name is the substrate's job control, and
// the substrate refuses job control with no terminal in this dialect's own
// wording (Diagnostics.MonitorDenied) on the way through `ApplyNamedOption`.
// So it says so rather than letting the caller write a second sentence about
// the same request — before #3190, `set -o monitor` in a shell with no
// terminal wrote `can't change option: monitor` twice, where `setopt monitor`
// beside it, which does not pass through that seam, said it once. Every other
// name in the table decides in silence and the substrate speaks, which is
// what lets one refusal be worded three ways by route.
func monitorOption() zshOption {
	o := setOptBacked("monitor", false, "monitor", false)
	o.atInvocation = func(*interp.Runner, bool) int { return 0 }
	o.speaksItsOwnRefusal = true
	return o
}

// shinStdinOption is zsh's `shinstdin`: the shell is reading its program from
// standard input.
//
// A fact about the invocation rather than a switch, and the substrate already
// holds it — it is the state behind the `s` in `$-`, which this shell was
// already writing on the route that has it. It was a constant `off` here, so
// one shell answered `[[ -o shinstdin ]]` false while its own `$-` said `s`
// in the same run.
//
// Measured 2026-09-16 on zsh 5.9.2. `echo 'cmd' | zsh -f` and
// `echo 'cmd' | zsh -f -s` both read the option **on**; `zsh -f -c cmd` reads
// it off; and `zsh -f -o shinstdin -c cmd` is granted at 0 and reads it on,
// with `s` in `$-` — while `setopt shinstdin` in a running script is
// `can't change option: shinstdin` at 1. So the command line may state it and
// a script may not, which is the same split `interactive` keeps one row up
// (#3154).
func shinStdinOption() zshOption {
	return zshOption{
		base: "shinstdin", def: false,
		get: func(r *interp.Runner) bool { on, _ := r.NamedOption("stdin"); return on },
		atInvocation: func(r *interp.Runner, on bool) int {
			return r.ApplyNamedOption("stdin", on)
		},
	}
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

// cPrecedences reads zsh's `c_precedences`: on when the arithmetic operators
// bind the way they do in C and in the rest of the panel.
//
// The state is the parser's order rather than a flag of its own, so the
// listings cannot drift from what an expression actually answers.
func cPrecedences(r *interp.Runner) bool {
	return r.ArithPrecedence() == syntax.ArithPrecedenceAsInC
}

// setCPrecedences moves it. Off is this shell's native order, which is the
// dialect's own answer and not a third state — see Dialect.
func setCPrecedences(r *interp.Runner, on bool) {
	if on {
		r.SetArithPrecedence(syntax.ArithPrecedenceAsInC)
		return
	}
	r.SetArithPrecedence(syntax.ArithPrecedenceShiftsAndBitwiseBindTighter)
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
// than assumed, and the base need not be a constant: `hashdirs` and `zle`
// follow whether the shell is interactive, and `login` and `rcs` are whatever
// the front end was told. Everything else is `recorded`'s bargain unchanged —
// remembered, reported, and acted on by nothing.
func recordedOver(base string, def bool, state func(*interp.Runner) bool) zshOption {
	return zshOption{
		base: base, def: def, recorded: true, over: state,
		get: func(r *interp.Runner) bool { return state(r) != recordedDeviates(r, base) },
		set: func(r *interp.Runner, on bool) int {
			setRecordedDeviation(r, base, on != state(r))
			return 0
		},
	}
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
	// Asked rather than listed. This is a membership test on a set that
	// happens to be stored as an array, and reading it with GetArray built
	// the whole list to answer one question — measured on a real ~/.zshrc,
	// eleven thousand lists in a startup, because an `emulate -L zsh` at the
	// top of a zsh function reaches every option that names one. See
	// interp.Runner.ArrayHolds.
	return r.ArrayHolds(zshRecordedStore, base)
}

// setRecordedDeviation records or clears one name's deviation, keeping the
// store sorted so the array is a function of the set and not of the order the
// rc file happened to write.
func setRecordedDeviation(r *interp.Runner, base string, dev bool) {
	if recordedDeviates(r, base) == dev {
		// Already where it is being put, so the store as it stands is the
		// store this would write: the rebuild below would allocate a fresh
		// slice, sort it and hand back the same set. The same reasoning as
		// setAxis, on the other half of this dialect's option state.
		return
	}
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
//
// The copy is a real cost and not a notional one: [interp.Semantics] is 976
// axes and 3296 bytes — counted off the struct on 2026-09-25, where the two
// numbers here had stood at 962 and 3280 through a dozen axes — and because
// the fresh copy is what the runner keeps,
// it is a heap allocation every time. A caller that knows the axis is
// already where it is being asked to go should not call this at all — see
// setAxis, which is the guarded form and is what the option table uses.
func swapAxes(r *interp.Runner, change func(*interp.Semantics)) {
	s := *r.Semantics
	change(&s)
	r.Semantics = &s
}

// setAxis is swapAxes for the shape nearly every option has: one axis, moved
// to a value the caller already holds. When the axis is already there it does
// nothing — no copy, no allocation, and the vector stays shared with whoever
// else is pointing at it.
//
// That case is not an edge. A zsh function idiomatically opens with
// `emulate -L zsh`, which walks the option table and sets every name this
// emulation has a default for, whether or not it differs from what the shell
// already had. Measured on the maintainer's real ~/.zshrc — powerlevel10k and
// zi, 31 plugins — a single startup made **35,597** swap calls, of which
// **30,026 changed nothing**: 98MB of the 494MB that startup allocated, spent
// copying a struct onto an identical one.
//
// The axis is named once, as a pointer into whichever copy is being read or
// written, so the guard and the change cannot drift apart into asking about
// one field and writing another. Handing over two expressions — a condition
// and a mutation — is how that drift gets written, and it would be silent:
// the shell would still be correct, and the copy would come back.
func setAxis[T comparable](r *interp.Runner, of func(*interp.Semantics) *T, v T) {
	if *of(r.Semantics) == v {
		return
	}
	s := *r.Semantics
	*of(&s) = v
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
	// The third face of the same namespace: this shell's `set` gives a
	// single letter to most of its options, and the letters are its own
	// rather than the panel's. See setLetterOptions.
	r.SetOptionLetterNames(setLetterOptions)
}

// setLetterOptions are this shell's `set` option letters, each mapped to the
// name in the table above that it abbreviates.
//
// Measured 2026-09-13 on zsh 5.9.2, one letter at a time from `env -i zsh -c`
// and read back with `[[ -o name ]]` in **both** directions: `set -X` makes
// the option on and `set +X` makes it off. A bulk answer would have hidden
// the two exceptions at the bottom of this comment, which is the whole reason
// the sweep is a letter at a time.
//
// Every one of these thirty was refused here as `-X is not implemented yet`
// while `set -o <name>` already took the same option and moved it — the
// substrate's letter table holds the letters the panel spells alike, and this
// shell's are simply not in it. That is what #2578 found from the `set -T`
// end: a script writing `set -T` was told the letter did not exist, where the
// real shell turns `cdablevars` on.
//
// The names are the spelling the option is *listed* under when it is on,
// which is the direction this shell gives the letter: `-F` is `noglob` and
// `-C` is `noclobber` because the letter turns the negative on, while `-X` is
// `listtypes` because that one is positive. Both readings are the same rule —
// the letter names one entry of the table above, and `set -L` is `setopt` of
// that entry — and the table is where the negation lives, not here.
//
// The letters deliberately **not** here are the ones the panel shares, which
// stay in the substrate's own table so that `-e`, `-u` and `-x` cannot come
// to mean two things: a letter belongs in this map exactly when this shell
// disagrees with the rest of the panel about it. `-T` is the worked example —
// trap carriage in one shell, `cdablevars` here — and it is why a shared
// reading behind an axis could not answer it.
//
// Two measurements that are not a mapping, and both are recorded rather than
// smoothed over:
//
//   - `-s` is taken at 0 and moves nothing at all, in either direction. It
//     sorts no positional parameters (`set -- c a b; set -s` leaves `c a b`),
//     it changes no row of `set -o`, and it puts no letter in `$-`. The
//     option the letter belongs to is one the shell will not change after
//     startup, and the letter path is silent about that where the name path
//     is not: `set -o shinstdin` is `can't change option: shinstdin` at 1 in
//     the same shell. That inconsistency is the reference's, so what is
//     modeled is the observable — taken, and nothing happens.
//   - `-i` and `-Z` are the two letters that refuse rather than move, and
//     they refuse in the *option* wording rather than the bad-letter one:
//     `can't change option: -i`. Mapping them to `interactive` and `zle`
//     is what produces that, because those two names already refuse the same
//     way when `set -o` asks for them.
var setLetterOptions = map[rune]string{
	'd': "noglobalrcs",
	'g': "histignorespace",
	// Two letters the substrate's own table already reached, and they are
	// here so that they reach the *same place through the same seam*. Both
	// end at the name they always ended at — `-h` writes the substrate's
	// `histignoredups` and `-p` its `privileged`, which this dialect's table
	// is written in terms of — so the state does not move. What changes is
	// that `$-` shows the letter, which it could not while the letter was
	// answered by a branch with no entry in any letter table: measured,
	// `set -h; echo $-` is `569Xh` and `set -o histignoredups; echo $-` is
	// the same string, so the letter reports the option rather than the
	// spelling that moved it.
	'h': "histignoredups",
	'p': "privileged",
	// Taken at 0, and it moves nothing. Not a gap — see the comment above.
	's': "",
	'i': "interactive",
	'k': "interactivecomments",
	'l': "login",
	'r': "restricted",
	'w': "chaselinks",
	'y': "shwordsplit",
	'B': "nobeep",
	'D': "pushdtohome",
	'E': "pushdsilent",
	'F': "noglob",
	'G': "nullglob",
	'H': "rmstarsilent",
	'I': "ignorebraces",
	'J': "autocd",
	'K': "nobanghist",
	'L': "sunkeyboardhack",
	'M': "singlelinezle",
	'N': "autopushd",
	'O': "correctall",
	'P': "rcexpandparam",
	'Q': "pathdirs",
	'R': "longlistjobs",
	'S': "recexact",
	'T': "cdablevars",
	'U': "mailwarning",
	'V': "nopromptcr",
	'W': "autoresume",
	'X': "listtypes",
	'Y': "menucomplete",
	'Z': "zle",
}

// optionListingWidth pads the name column of every listing that writes a
// state beside a name: `set -o`, where it is the number
// Diagnostics.OptionListingWidth carries, and the long form `setopt` and
// `unsetopt` take under `ksh_option_print`. Named once and read in both
// places so the two cannot drift apart, which is the arrangement
// dialect/bash's setOptionListingWidth already uses for `shopt -o`.
//
// Measured on zsh 5.9.2: the state word starts at column 23 on all 185 rows,
// and the longest name a listing writes is 21 characters.
const optionListingWidth = 22

// listedOptions is the `set -o` and `set +o` listing: every option in the
// table, in the same order and the same spelling a bare `setopt` uses.
//
// One printed spelling per option — the one that is off under the current
// mode's default, which is [emulationDefault] and not always `o.def`.
// Measured, zsh writes `noaliases`, `allexport`, `noalwayslastprompt` in that
// order and nothing about the twelve borrowed sh and ksh spellings, which it
// accepts as input and never lists. So a row's state is its deviation from
// that baseline, exactly as listZshOptions computes it: `noaliases off` in a
// shell where aliases work, and `autocd on` after `setopt autocd`.
func listedOptions(r *interp.Runner) []interp.ListedOption {
	mode := currentEmulation(r)
	rows := make([]interp.ListedOption, 0, len(zshOptions))
	for _, o := range zshOptions {
		base := emulationDefault(o, mode)
		rows = append(rows, interp.ListedOption{
			Name: spellOption(o.base, !base),
			On:   o.get(r) != base,
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
func moveOption(r *interp.Runner, name string, on bool) interp.OptionMove {
	o, inverted, ok := resolveOptionName(normalizeOption(name))
	if !ok {
		return interp.OptionNotFound
	}
	want := on != inverted
	if mover := invocationMover(r, o); mover != nil {
		// Asked first, ahead of both the fixed bargain and the entry's own
		// set. A name with one of these has been measured on the command
		// line that started the shell and answers there for itself — which
		// is the whole point of the hook, since four of the five names that
		// carry one are **refused** to a running script and granted at an
		// invocation. See Semantics.ImmovableOptionsSetAtInvocation.
		return moved(mover(r, want) == 0, o)
	}
	if o.immovable != nil && o.immovable(r) {
		return moved(o.get(r) == want, o)
	}
	if o.set != nil {
		return moved(o.set(r, want) == 0, o)
	}
	// Fixed: granted where it is already where it is being asked to be, and
	// refused otherwise — the same bargain setOption strikes, with the
	// sentence left to the caller.
	return moved(o.get(r) == want, o)
}

// moved turns one entry's answer into the substrate's, which is where the
// contract at the top of moveOption is kept: an entry that has already
// complained says so, and every other refusal is left for the substrate to
// word.
func moved(ok bool, o zshOption) interp.OptionMove {
	switch {
	case ok:
		return interp.OptionMoved
	case o.speaksItsOwnRefusal:
		return interp.OptionRefusedAndSaid
	}
	return interp.OptionRefused
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
	if mover := invocationMover(r, o); mover != nil {
		// The command line that started the shell, which is a route of its
		// own and is asked before either answer below. See moveOption.
		return mover(r, want)
	}
	if o.immovable != nil && o.immovable(r) {
		// Held still in this shell, so the fixed bargain below applies even
		// though the entry has a set — asked for where it already is, it is
		// granted, and anything else is zsh's own sentence.
		if o.get(r) == want {
			return 0
		}
		r.Diagnosef("can't change option: %s\n", name)
		return 1
	}
	if o.set != nil {
		return o.set(r, want)
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
// the one that is off under [emulationDefault] — and `setopt` prints the spellings
// that are on where `unsetopt` prints the ones that are off. The table is
// already in the measured order, which is by canonical name, and that order
// is the *canonical* name's rather than the printed spelling's: a baseline
// that moves changes which of `aliasfuncdef` and `noaliasfuncdef` is written
// and never where the row stands.
func listZshOptions(r *interp.Runner, setting bool) {
	if kshOptionPrintOn(r) {
		// The long shape, which is `set -o`'s and is the same rows: the
		// direction a bare name means is what `setting` carries, and this
		// form has no direction, so both builtins write the whole table
		// here. See the kshoptionprint entry for the measurement.
		for _, row := range listedOptions(r) {
			state := "off"
			if row.On {
				state = "on"
			}
			_, _ = fmt.Fprintf(r.Out(), "%-*s%s\n", optionListingWidth, row.Name, state)
		}
		return
	}
	mode := currentEmulation(r)
	for _, o := range zshOptions {
		base := emulationDefault(o, mode)
		if o.get(r) != base == setting {
			_, _ = fmt.Fprintf(r.Out(), "%s\n", spellOption(o.base, !base))
		}
	}
}

// kshOptionPrintIndex is where `kshoptionprint` sits in the table, resolved
// once — the arrangement localOptionsIndex uses, and for the same reason: a
// name this package holds as a constant is looked up at build time rather
// than on every listing.
var kshOptionPrintIndex = zshOptionIndex["kshoptionprint"]

func kshOptionPrintOn(r *interp.Runner) bool { return zshOptions[kshOptionPrintIndex].get(r) }

// spellOption writes the name in the direction asked for: the base for the
// state a listing calls on, `no` and the base for the other.
func spellOption(base string, on bool) string {
	if on {
		return base
	}
	return "no" + base
}

// editingOption is `vi` and `emacs`: the two option names that are also the
// two ways to choose a keymap.
//
// **Turning one of these on selects a keymap, and that is the whole of what
// this adds to setOptBacked.** In zsh the option and `bindkey` write the same
// piece of state from two directions, measured on zsh 5.9.2 by reading
// `bindkey -lL main` back after each:
//
//	setopt vi                 -> bindkey -A viins main
//	set -o vi                 -> bindkey -A viins main
//	zsh -o vi                 -> bindkey -A viins main
//	setopt emacs              -> bindkey -A emacs main
//	bindkey -v                -> bindkey -A viins main
//
// Without this the option moved the substrate's editing mode and nothing
// else, so `setopt vi` reported itself on — `[[ -o vi ]]` was true — while
// the editor went on reading the emacs keymap and the Escape a person pressed
// was typed into the line. An option that reports itself on and does nothing
// is worse than one this shell does not have, because nothing says so (#3140).
//
// **Only turning one *on* moves the keymap**, which is measured rather than
// assumed and is the half that is easy to get wrong: `setopt vi; unsetopt vi`
// leaves `main` aliased to `viins` in zsh, and `bindkey -v; set +o vi` leaves
// it there too. Turning the option off is not a request to go back.
//
// The keymap name is spelled here and not in the substrate for the reason
// interp.EditingMode gives: the core holds *which* mode, and the two dialects
// with a line editor spell it — `vi-insert` in one and `viins` in the other.
func editingOption(base, keymap string) zshOption {
	return zshOption{
		base: base, def: false,
		get: func(r *interp.Runner) bool { on, _ := r.NamedOption(base); return on },
		set: func(r *interp.Runner, on bool) int {
			code := r.ApplyNamedOption(base, on)
			if code == 0 && on {
				selectKeymap(r, keymap)
			}
			return code
		},
	}
}
