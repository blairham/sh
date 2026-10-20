// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"os"
	"strconv"

	"github.com/blairham/sh/interp"
)

// Apply adds what bash has and the substrate does not.
//
// `source` is a synonym for `.`, and is a dialect's answer rather than the
// core's because dash does not have it at all — `command -v source` reports
// "not found" there. It is the same function under a second name rather than a
// second implementation, which is the only way the two cannot drift apart.
func Apply(r *interp.Runner) {
	// `eval` is in DeclarationUtilities above because the grammar has to let a
	// compound assignment follow the word, and it declares nothing: the
	// operand reaches it as one word holding the assignment with its elements
	// expanded. See interp/rejoinedoperand.go for the measurements.
	r.RejoinArrayOperand("eval")
	// `let` is the second word on that list and takes its operand the same
	// way: what reaches the builtin is one word holding the assignment with
	// its elements expanded, which `let` then evaluates as arithmetic — so
	// `let a=(1 2)` is `a=(1 2): missing `)'` at 1, where `let x=(2 + 3)` is
	// the parenthesized expression and leaves 5. Measured 2026-09-21 on bash
	// 5.3.20 with a shell function standing in for the builtin, which prints
	// the words it was given: `let a=(1 2)` is the single word `a=(1 2)`.
	r.RejoinArrayOperand("let")
	// The `set -o` names this shell has and the others do not all have,
	// measured by asking each of the four to turn every name off. This one
	// has the most, and five of them belong to it alone.
	r.AddSetOptions(
		"braceexpand",
		// `emacs` and `nolog` moved out of the substrate's common table when
		// the ash column was asked for them and had neither (#3366). Both
		// are in this shell's own `set -o` listing, measured on bash 5.3.20.
		"emacs",
		"errtrace",
		"nolog",
		"functrace",
		"hashall",
		"histexpand",
		"history",
		"interactive-comments",
		"keyword",
		"onecmd",
		"physical",
		"pipefail",
		"posix",
		"privileged",
	)
	// And the three of those names that `$-` spells with a letter this
	// package has to declare, because the substrate has no field behind them
	// — they are `set -o` names it records rather than behaviors it runs, and
	// a name this shell *grants* must show its letter or a script cannot read
	// back what it just asked for. See interp.Runner.SetOptionLetterNames,
	// whose whole point is that the letter and the name are one request.
	//
	// Measured 2026-09-23 on bash 5.3.20 from a script file, each name by
	// its long form and by its letter, in both signs:
	//
	//	set -o physical     hBP     set +o physical     hB
	//	set -o privileged   hpB     set +o privileged   hB
	//	set -o histexpand   hBH     set +o histexpand   hB
	//
	// — and the letter form gives the same four answers, which is what says
	// they are one request rather than two. This shell granted all three
	// names and showed none of the letters, so `set -o physical` was a
	// request it took and then denied having taken (#4163).
	r.SetOptionLetterNames(map[rune]string{
		'H': "histexpand",
		'P': "physical",
		'p': "privileged",
	})
	// And the one of those names that is also a parameter. `IGNOREEOF` and
	// `ignoreeof` are two spellings of one state here: an assignment turns
	// the option on whatever it assigns, `unset` turns it off, `set -o
	// ignoreeof` writes `10` over whatever the parameter held, and `set +o
	// ignoreeof` takes the name away. Measured 2026-09-21 on bash 5.3.20 —
	// see interp/tiedoption.go, which carries the eight rows. It is `set -o`
	// and not `shopt`: `shopt ignoreeof` is `invalid shell option name` in
	// that shell and in this one (#4047).
	r.TieOptionToParameter("ignoreeof", "IGNOREEOF", "10")
	// And the second of the two, which is the option this shell's own manual
	// names a variable for: assigning `POSIXLY_CORRECT` turns `posix` on and
	// unsetting it turns it off, `set -o posix` writes `y` into the name and
	// `set +o posix` takes the name away. Measured 2026-09-23 on bash 5.3.20 in
	// all four directions, and **any** value does it — `POSIXLY_CORRECT=` turns
	// the option on exactly as `=1` does, which is the tie's contract and not a
	// reading of the value (#4142).
	r.TieOptionToParameter("posix", "POSIXLY_CORRECT", "y")
	// And this one fires from the **environment** too, where `IGNOREEOF` does
	// not: `env POSIXLY_CORRECT=1 bash -c 'set -o'` reads `posix on` and `env
	// IGNOREEOF=1 bash -c 'set -o'` reads `ignoreeof off`. Two names, one tie
	// mechanism, and a measured disagreement about the startup route — see
	// interp.Runner.TieAlsoFiresFromTheEnvironment.
	r.TieAlsoFiresFromTheEnvironment("posix", "POSIXLY_CORRECT")
	// The statuses of the last pipeline's elements. The core keeps the
	// record and this names it; ksh93 and dash have no name for it at all.
	r.SetPipelineStatus("PIPESTATUS")
	// And the listing for it, on the same terms the call-stack arrays are on:
	// `declare -p PIPESTATUS` is `declare -a PIPESTATUS=([0]="0")` in bash
	// 5.3.20 and was `PIPESTATUS: not found` here (#3099).
	r.SetDynamicDeclaration("PIPESTATUS", interp.ProducedDeclaration{Array: true, ListsItsElements: true})
	// And an assignment to it is discarded, as the call-stack arrays' are:
	// measured 2026-10-05 on bash 5.3.20, `PIPESTATUS=(z); declare -p
	// PIPESTATUS` is `declare -a PIPESTATUS=([0]="0")` — the record the
	// assignment's own command left — and so is the same after `unset
	// PIPESTATUS`, which does not end the name in bash. This stored the
	// write and listed it back (#6053). The first command of a shell is the
	// recorded exception: `bash -c 'PIPESTATUS=5; declare -p PIPESTATUS'`
	// is `declare -- PIPESTATUS="5"`, a scalar, before any pipeline has run.
	r.SetDynamicArrayWriter("PIPESTATUS", func(*interp.Runner, []string) {})
	// What the last `=~` captured — the whole match, then the groups. The
	// core keeps the record and this names it; ksh93 and zsh keep their
	// captures under names and shapes of their own, never this one.
	r.SetRegexMatch("BASH_REMATCH")
	// And the parameter that moves the bound on function nesting. The bound is
	// the core's and the name is this shell's; ksh93 has the bound and no name
	// for it, and dash has neither. Measured 2026-09-23 on 5.3.20 — a positive
	// integer only, so `FUNCNEST=0`, an empty value and `FUNCNEST=abc` are all
	// "no bound of the script's own", which is the other answer to
	// interp.FunctionNesting.ZeroIsABound and is where zsh parts from this.
	// The refusal gives up the input line and the next one runs, which is
	// the other answer to RefusalEndsTheScript. See
	// interp.Runner.SetFunctionNesting.
	r.SetFunctionNesting(interp.FunctionNesting{Parameter: "FUNCNEST"})
	// The long names of the options that are on, as a readonly produced
	// variable bound to the option state in both directions. The core keeps
	// the state and this names it; the other three leave the name an ordinary
	// string and read nothing out of it at startup, which is measured.
	r.SetShellOptions("SHELLOPTS")
	// And the same binding for this shell's *other* option namespace, which
	// the core does not have: the `shopt` names that are on, produced and
	// readonly, read back out of the environment at startup. bash 3.2 has no
	// such variable, so this is bash 5's answer — see bashOptions.
	r.SetOptionList("BASHOPTS", bashOptions, applyInheritedBashOptions)
	// Parameters bash provides and the others do not all have. Which
	// variables a shell supplies is the same kind of question as which
	// builtins it has, so it is answered here rather than as an axis.
	//
	// A read of the process, in library code, on purpose. The purity rule
	// covers dialect/ as well as interp/ — an embedder links this and Apply
	// runs inside the Runner — but it is about state a Runner owns and two
	// Runners could disagree about. A uid is neither: nothing a script does
	// changes it, two shells in one program genuinely have the same one, and
	// $UID has no other source. Same class as $$, which AGENTS.md's forbidigo note has
	// blessed since it was written.
	r.SetSpecial("UID", strconv.Itoa(os.Getuid()))
	r.SetSpecial("EUID", strconv.Itoa(os.Geteuid()))
	// And the attributes this shell's own parameters carry, which were
	// missing from every one of them — see markOwnParameterAttributes, and
	// note that the readonly half changes an answer rather than a listing.
	markOwnParameterAttributes(r)
	// The login name for that same uid, for the `\u` prompt escape, and the
	// machine's name for `\h` and `\H`. Asked here for the reason the uid
	// above is, and asked through interp.LoginName rather than through a
	// closure of this package's own, because zsh's `%n` is the same question
	// under another spelling — the escape's *letter* is this dialect's and
	// resolving a login name is not.
	//
	// This is #1446. `%n` had an asker and `\u` had none, so the drawer fell
	// back to a lookup of its own that read `$USER` and `$LOGNAME`; with
	// neither set — `env -i`, `sudo -i`, a container, a cron job, a login
	// shell a daemon started — `\u` drew nothing, and the `\u@\h:\w\$ `
	// that most distributions ship rendered `@host:~$`. Every other escape in
	// that prompt was byte-identical to bash 5.3.15, which is what made it
	// look like a working prompt.
	//
	// Deferred rather than resolved here, which #1423 measured and this must
	// not undo: `user.Current` is 0.83-1.10 ms on darwin — Directory Services
	// — and Apply runs on every invocation, so asking eagerly would spend a
	// millisecond of every `bash -c ':'` on an escape that route cannot draw.
	// SetPromptUserFunc asks at the first draw and keeps the answer, which is
	// the right trade in both directions: a prompt is redrawn on every
	// keystroke that redraws the line, and a login name does not change while
	// a shell runs, so a resolver called per draw would pay that millisecond
	// per *character typed*.
	r.SetPromptUserFunc(interp.LoginName)
	r.SetPromptHostFunc(interp.MachineName)
	// And the table those two resolvers answer *for*, which is the half that
	// was missing: a runner told who the user is and which machine this is,
	// and never told that `\u` and `\h` are the letters that ask, draws the
	// letters as text. #1455.
	//
	// Installed here rather than only in the front end so that the escape
	// language travels with the dialect. `driver.Shell.PromptStyle` hands the
	// same value to the same runner on the way to the prompt drawer, and the
	// two agree because they are the same function's result — but an embedder
	// who applies this dialect to a Runner of their own never builds a Shell,
	// and before this they got a bash with no prompt language at all. zsh's
	// Apply had installed its table since #1090 and the other three had not,
	// which is the asymmetry #1455 reports.
	r.SetPromptStyle(PromptStyle())
	// $LINES and $COLUMNS follow the window, which this shell does by
	// default: `shopt checkwinsize` in bash 5.3.15 is `on`, and a session
	// there reports `COLUMNS=80 LINES=24` at its first prompt with nothing in
	// a startup file asking for it. bash 3.2.57 answers `off` to the same
	// question and is the one column of the panel this default is not right
	// for; one bash dialect answers for both, and it answers with the
	// version people are running.
	//
	// The default is here rather than in the option table because it is a
	// fact about *this shell*, in the same way the option names above are:
	// the capability itself belongs to the core, where zsh reaches it under
	// no name at all. See interp.Runner.TracksWindowSize.
	r.SetTracksWindowSize(true)
	// Where the script is, which is a stack rather than a value — see
	// callstack.go for why it cannot be stored.
	registerCallStack(r)
	// The same stack, one step up: see caller.go.
	registerCaller(r)
	// Rebinding a key, in readline's vocabulary. The editor performs the
	// actions and this names them; see bind.go, and repl/widgets.go for why
	// the two halves are apart.
	registerBind(r)
	// The history list and the two files it is kept in. A script has one —
	// bash maintains a list and writes a history file with no terminal
	// anywhere — so this is a builtin rather than something the prompt owns.
	// See history.go.
	registerHistory(r)
	// Stopping the shell itself, which is two halves: a refusal a script sees
	// and a stop only a binary that owns the process may make. See
	// suspend.go, and interp.Runner.StopThisProcess for the split.
	registerSuspend(r)
	// A function carried to a child through the environment, under the name
	// bash gives it. The other three do not carry functions at all.
	r.SetFunctionExport("BASH_FUNC_", "%%")
	// And how they are laid out, which is this shell's taste rather than
	// anything the printer should know.
	shown, exported := FunctionLayout(), ExportedFunctionLayout()
	// The one part of the arrangement that needs the runner: a `$'…'` is
	// written back as the characters it stands for, and what an escape comes
	// to is this shell's answer rather than the printer's (#3196).
	shown.AnsiCQuotedWordIsItsValue = r.AnsiCValue
	exported.AnsiCQuotedWordIsItsValue = r.AnsiCValue
	r.SetFunctionLayout(shown, exported)
	// And how a whole script is written back, which is that same arrangement
	// with a top level and one header spelling apart. The decoder is attached
	// here for the reason it is above: the escapes are this shell's answer.
	listed := ScriptListingLayout()
	listed.AnsiCQuotedWordIsItsValue = r.AnsiCValue
	r.SetScriptListingLayout(listed)
	// The command the shell is running, which a DEBUG action reads to find
	// out which one it fired for. See bashcommand.go.
	registerRunningCommand(r)
	registerTrapSignalNumber(r)
	r.SetDynamic("RANDOM", func(rr *interp.Runner) string { return rr.Randoms() })
	// And an assignment seeds it, which is what makes a script that uses
	// `RANDOM` reproducible: measured 2026-09-14, `RANDOM=42` twice in one
	// shell gives the same pair of numbers both times here, in ksh93u+ and
	// in zsh 5.9.2. Without the writer the assignment was heard and stored
	// for the producer to find, and the producer had no state to find it
	// with (#2827).
	r.SetDynamicWriter("RANDOM", func(rr *interp.Runner, value string) { rr.SeedRandoms(value) })
	// And the sequence a seeded `RANDOM` answers, which is this shell's own:
	// three shells in the panel have the parameter and no two of them draw the
	// same numbers. See random.go, and `docs/spec/random.md` (#4240).
	registerRandoms(r)
	// How the two of them list back, which a produced parameter has to be
	// told rather than carry: `declare -p RANDOM` is
	// `declare -i RANDOM="16735"` in bash 5.3 and was `RANDOM: not found`
	// here, from the name this shell had just expanded a number for (#2451).
	r.SetDynamicDeclaration("RANDOM", interp.ProducedDeclaration{Integer: true})
	r.SetDynamic("SECONDS", func(rr *interp.Runner) string {
		return strconv.Itoa(int(rr.SecondsFrom()))
	})
	// `-i`, but only once something has expanded it: the operand-less listing
	// writes `declare -- SECONDS` in a shell that has never read the
	// parameter and `declare -i SECONDS="0"` after a plain `: $SECONDS`. It
	// is the one name of the seven that moves, and the *named*
	// `typeset -p SECONDS` writes the letter in both states (#2722).
	r.SetDynamicDeclaration("SECONDS", interp.ProducedDeclaration{
		Integer: true, IntegerOnceRead: true,
	})
	// And the line, which this shell lists with *no* attribute where bash
	// 3.2 writes `-i` — `declare -- LINENO="1"`, measured on 5.3.15. It is
	// registered by the core rather than here, so this is a declaration for
	// a producer the dialect did not install, which the seam allows for
	// exactly this: the letters are the dialect's fact and the producer is
	// not (#2451).
	r.SetDynamicDeclaration("LINENO", interp.ProducedDeclaration{})
	// The four parameters of this shell's own that were simply absent, and
	// which are not the core's to supply — see shellparameters.go (#3098).
	registerShellParameters(r)
	// And the directory stack as a parameter, which is a view onto what the
	// prelude's `pushd` keeps rather than a store of its own.
	registerDirectoryStack(r)
	// The clock, as this shell reads it since 5.0. In a file of its own
	// because it is not the `zsh/datetime` registration under another name —
	// see epoch.go for the row-by-row measurement (#1158).
	registerEpochClock(r)
	// The alias table written as an association, readable and writable.
	registerBashAliases(r)
	registerBashCmds(r)
	if dot, ok := r.Builtin("."); ok {
		r.Register("source", dot)
	}
	// The builtin this shell alone answers to; the other three say "command
	// not found", so it is registered here rather than taken away there.
	r.Register("shopt", biShopt)
	registerLogout(r)
	// Where `set -x` writes, which this shell alone lets a script move. See
	// xtracefd.go for the three measurements that shape it.
	registerTraceDescriptor(r)
	// And the one name this shell's restricted mode freezes beyond the three
	// POSIX ones and the non-interactive startup variable. Measured by asking
	// `readonly -p` in a restricted shell: PATH, SHELL, ENV, BASH_ENV and
	// HISTFILE are frozen and nothing else is. ksh93's extra name is a
	// different one, which is why this is a list rather than an axis.
	r.FreezeInRestrictedMode("HISTFILE")
	// And the complaint a compatibility level out of range draws, which is
	// the assignment's and not the first row's to read it. See compat.go,
	// which also wires the `shopt compat44` spelling to the same state.
	registerCompatibilityLevel(r)
	// And the same table reached from the command line that started the
	// shell, where there is no builtin to run: `bash -O checkhash` moves one
	// of these names before the first line of the script. See shopt.go, and
	// Semantics.ShellOptionInvocationLetter for the letter the front end
	// reads (#3264).
	r.SetShellOptionNamespace(shoptMoveAtInvocation, shoptListAll, shoptState)
	// And the same namespace reached a third way: `compgen -A shopt` lists
	// these names, which is what bash's own shopt1.sub loops over. The action
	// is this dialect's because the names are — see
	// interp.Runner.SetCompgenAction, and "function" is where bash generates
	// it, measured rather than chosen (#4149).
	r.SetCompgenAction("shopt", "function", func(*interp.Runner, string) []string {
		return shoptNames()
	})
	// This shell's only self-documenting builtin, and there is no other name
	// for it — a script reaching for it used to get 127. See help.go, which
	// carries what is answered and what is deliberately refused.
	r.Register("help", biHelp)
	// `**` with nothing behind it crosses levels here too, so `d/**` reaches
	// every one of them. Set unconditionally rather than through `globstar`,
	// because the walk asks it only where `globstar` has already said `**`
	// crosses at all — and `globstar` is the only name this shell has for
	// either half. Measured 2026-09-07: `shopt -s globstar; echo **` in a
	// directory holding `ax`, `bx` and `cx/dx/ax` lists every level here and
	// answers `ax bx cx` in zsh (#1339).
	r.SetMatchOption(interp.StarStarAloneCrossesDirectories, true)
	// A symbolic link to a directory is one of the levels `**/` lists here,
	// even though the walk never goes inside one. Set for the same reason as
	// the line above: `globstar` is the only name this shell has, and the
	// walk asks this only where that option has already said `**` crosses.
	// Measured 2026-09-12 against bash 5.3.15 in a directory holding `r/x`
	// and a symlink `s` to `r`: `shopt -s globstar; echo **/` is `r/ s/`
	// here and `r/` in zsh, while `echo **/x` is `r/x` in both — the link is
	// named and not entered (#2360).
	r.SetMatchOption(interp.StarStarSeesLinkedDirectories, true)
	// And a run of `**` components is one component here, which is the third
	// question the same option name has to answer for. Without it the run is
	// a cross product and every name in it comes back once per way of
	// splitting it, because nothing anywhere takes duplicates out of a
	// pathname expansion. Measured 2026-09-13 in a tree of directories `a`
	// and `b` nested three deep: `shopt -s globstar; echo **/**/` is 14
	// names here and 48 in zsh, where `a/a/a/` appears four times (#2298).
	r.SetMatchOption(interp.RepeatedStarStarIsOneComponent, true)
	// A `**` that matched zero levels is the directory the walk stood in, so
	// `d/**` names `d/` ahead of what is under it. Set for the same reason as
	// the three above — `globstar` is the only name this shell has for any of
	// them — and it is the fourth question that one name has to answer.
	// Measured 2026-09-16 against ksh93u+, which is the only column that
	// reads it the other way: `printf '[%s]' d/**` is `[d/][d/e][d/e/f]`
	// here and `[d/e][d/e/f]` there (#3152).
	r.SetMatchOption(interp.StarStarZeroLevelIsTheDirectoryItStartsFrom, true)
	// And a `**` may begin inside a directory the pattern reached through a
	// symbolic link, which is where the walk starts rather than where it may
	// go: this shell never follows a link it meets on the way down. Measured
	// 2026-09-16 with `s` a symlink to `r` and `r/x` under it: `s/**` is
	// `[s/][s/x]` here and in zsh, and no match at all in ksh93 (#3152).
	r.SetMatchOption(interp.StarStarPatternsReadLinkedDirectories, true)
	// And a component behind a `**` looks inside a level the walk listed and
	// never entered, which the two other columns with the construct do not:
	// the walk stays bounded and only the set handed to the next component
	// is wider. Measured 2026-09-18 with `r/x`, a symlink `s` to `r` and a
	// second symlink `a/b/sl` to `r`: `./**/x` is `[./a/b/sl/x][./r/x][./s/x]`
	// here, `[./r/x]` in ksh93 and in zsh — and this shell names the linked
	// level for `**/` exactly as ksh93 does, which is what makes the two
	// separate questions. The walk that begins the word is this shell's own
	// exemption and the option records it: `**/x` is `[r/x]` here while the
	// same pattern with a `.` or an absolute root in front of it is not
	// (#3176).
	r.SetMatchOption(interp.ComponentBehindStarStarSeesLinkedLevels, true)
	// An `&` in a `${v/pat/rep}` replacement is the text the pattern
	// matched, which is this shell alone in the panel — bash 3.2, ksh93 and
	// zsh all answer `a[&]c` for `v=abc; ${v/b/[&]}` where this one answers
	// `a[b]c`. It is on with nothing said and `shopt -u patsub_replacement`
	// turns it off, so the default belongs here and the name belongs in
	// shopt.go (#1862).
	r.SetMatchOption(interp.ReplacementAmpersandIsTheMatch, true)
	// And the other default this preset holds the other way round from the
	// core: `shift` past the end is **silent** here, where dash, ksh93 and
	// zsh each write their own sentence. The wording is in this dialect's
	// Diagnostics all the same, because it is the wording
	// `shopt -s shift_verbose` asks for — so the option turns a capability
	// on rather than installing a sentence, and the listing reads the
	// capability back (#3465).
	r.SetReportsShiftPastTheEnd(false)
	// And the two history defaults this preset holds the other way round from
	// the core, which is the side every shell with no such option is on. A
	// command typed over several lines is **one joined entry** here, with a
	// `;` where each newline was, and `shopt -s lithist` is what keeps the
	// lines; and the history file is written from the session's list rather
	// than appended to where the two differ, which `shopt -s histappend` puts
	// back. Both measured on bash 5.3.20 through a pseudo-terminal — see
	// interp.Runner.HistoryJoinsATypedCommand and RewritesTheHistoryFile for
	// the tables, and shopt.go for why neither could stay a recorded state
	// (#4149).
	r.SetHistoryJoinsATypedCommand(true)
	r.SetRewritesTheHistoryFile(true)
	// `declare` is `typeset` under a second name rather than a second
	// implementation. ksh93 has only the older name and dash has neither, so
	// which names exist is a dialect's answer and not an axis.
	if typeset, ok := r.Builtin("typeset"); ok {
		r.Register("declare", typeset)
		// And the assignment rule follows the name: `declare x=*` stores the
		// character here, where in a shell without the name it would be an
		// ordinary command with an ordinary globbed argument.
		r.SetDeclaring("declare")
	}
}
