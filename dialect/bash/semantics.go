// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"github.com/blairham/sh/interp"
)

// Semantics is what bash 5 means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	// An unquoted list is joined on the first character of IFS and split
	// back, which is what makes `IFS=:; set -- x "" y; printf "[%s]" $@`
	// three fields here and two in ksh93 and dash. The join is also what
	// loses a *trailing* empty element — `set -- x y ""` is `x:y:`, and a
	// trailing separator makes no field — so this one answer is what the
	// panel's two shapes of disagreement both come from.
	s.UnquotedListJoinsOnIFS = interp.Yes
	// And with the elements joined there is no boundary left to be a
	// delimiter, which is the other reading of the same gap. Measured
	// 2026-09-26 on bash 5.3.20 and 3.2.57 alike, and exhibited rather than
	// implied by the join: `IFS=:; set -- 'a b' ':'; w x$@y` is
	// `[xa b] [] [y]` here, a row where the join and the field-break reading
	// agree and only this axis could move it.
	s.UnquotedListBoundaryIsIFSWhitespace = interp.No
	// A here-document whose delimiter never arrived gains the newline its
	// last line never had: `printf 'cat <<X\nbody'` writes five bytes here
	// and four in dash, ksh93 and zsh. Both bash builds measured agree
	// (#1020).
	s.UnterminatedHeredocGainsATrailingNewline = interp.Yes
	// A substitution body that does not parse is a failed *expansion* here
	// rather than a failed script: the refusal is reported, the word expands
	// to the empty string, and the statement and the script carry on at 0.
	// dash, ksh93 and zsh all abandon the input instead, which is what this
	// shell's answer was until #2703 — so `echo A; echo "`+"`echo \`echo n`"+`"; echo B`
	// stopped at `A` where bash prints `A`, an empty line and `B`.
	s.SubstitutionParseErrorIsFatal = interp.No
	// But where it *is* fatal — the `$( … )` spelling, which this axis is not
	// asked for — a subshell does not contain it: bash 5.3.20 ends the script
	// from inside `( … )`, a pipeline element and an enclosing `$( … )`
	// alike. bash 3.2.57 is the third answer and does not even end the
	// subshell, which is the row above rather than this one (#3274).
	s.SubstitutionParseErrorEscapesASubshell = interp.Yes
	// And a **here-document body** is the one redirection half it does not
	// end the script from. Measured 2026-09-17 on bash 5.3.20: `cat <<END`
	// over a body holding `$(echo hi; for)` reports the refusal, does not run
	// `cat`, and runs the rest of the script with 1 left behind — where the
	// *target* of the same redirection, `cat < "$(echo hi; for)"`, ends the
	// script at 2. The two halves are measured apart because this column is
	// the one that splits them (#3318).
	s.SubstitutionParseFailureInAHeredocBodyEndsTheShell = interp.No
	// The 1 that carries on is a fatal error's number and not a syntax
	// error's, and the borrowed-text rows are what say so rather than the
	// here-document row alone: measured 2026-09-17, `v=$(echo hi; for)` at
	// the top level of a script exits 2 here, where the same failure inside a
	// file `.` read or inside an `eval` argument exits **1**. A plain syntax
	// error in that sourced file is the control and is a different shape
	// entirely — `.` reports 2 and the script runs on (#3319).
	s.SubstitutionParseFailureCarriesTheFatalStatus = interp.Yes
	// An associative array's subscript is a quoting context here: the key is
	// the text inside its quotes, so `m["k"]=W` stores under `k` and
	// `${m["k"]}` reads it back. zsh takes the subscript as written and
	// stores under the three characters.
	s.SubscriptIsAQuotingContext = interp.Yes
	// An indexed subscript keeps its quoting for the arithmetic: `${a['2']}` is `'2': arithmetic syntax error` (#5562).
	s.IndexedSubscriptKeepsItsQuoting = interp.Yes
	// `unset` of a name a *calling* function made local takes the binding
	// away here, so the next scope out answers for the rest of the script:
	// the panel's other three leave the name unset until the call that
	// declared it returns. Measured on bash 5.3.20 and bash 3.2.57 alike —
	// see interp.Semantics.UnsetRemovesAnEnclosingLocal for the rows and for
	// the four that say the binding is gone rather than hidden. This is the
	// one column a script can move, with `shopt -s localvar_unset`.
	s.UnsetRemovesAnEnclosingLocal = interp.Yes
	// The other join, and the opposite answer: where an unquoted `@` list
	// reaches a context that keeps no fields, this shell rejoins it on a
	// hard space rather than on IFS. `IFS=-; a=(x y z); v=${a[@]}` is
	// `x y z` here and `x-y-z` in zsh, and the same in a `case` subject, a
	// `[[ ]]` operand and a here-document body. The `*` spelling is core and
	// does not come through here — `v=${a[*]}` is `x-y-z` in this shell.
	s.UnsplitAtListJoinsOnIFS = interp.No
	// bash 5.3.20 and bash 3.2.57 both print `e1: a b c` for `${e1?$*}`
	// under `IFS=:`: the word is expanded as an argument list would be and
	// the sentence made of the fields. The only column in the panel that
	// reads it this way.
	s.DiagnosticWordIsFields = interp.Yes
	// The panel's holdout on the login profile, measured on all four
	// non-interactive routes and in both bash 5.3 and the 3.2 macOS ships:
	// `exec -a -bash bash script.sh` reads neither ~/.bash_profile nor
	// /etc/profile, where dash, ksh93 and zsh all read theirs. Being a login
	// shell is not the part it declines — `shopt login_shell` is on — it is
	// that a non-interactive shell reads no startup file unless `--login`
	// was written out, and this front end has no `--login` to write.
	s.LoginProfileWhenNonInteractive = false
	// And the file it reads instead, on those same non-interactive routes.
	// Measured 2026-09-05 with a scratch HOME: `BASH_ENV=f bash script.sh`,
	// `bash -c` and a program piped in all source f, in bash 5.3 and in the
	// 3.2 macOS ships, where dash, ksh93 and zsh do nothing with the name.
	// It is not read at a prompt, which is where `$ENV` and a file of bash's
	// own name take over, and not read in POSIX mode — `bash --posix` and
	// bash invoked as `sh` read neither this nor `$ENV`.
	s.NonInteractiveStartupVariable = "BASH_ENV"
	// The parameter whose patterns take names back out of a pathname
	// expansion, and the hidden-name switch an assignment to it writes. Both
	// measured on bash 5.3.15 and bash 3.2.57, 2026-09-13 — the run is in
	// Semantics.IgnoredNamesVariable, which also records why this follows
	// the assignment rather than the value. ksh93's `FIGNORE` is the same
	// facility under another name and does not answer alike; it is filed
	// rather than guessed at.
	s.IgnoredNamesVariable = "GLOBIGNORE"
	// The compatibility level, which selects an older release's reading of a
	// behavior. Only #4256's row is answered against it so far; see
	// Semantics.CompatibilityLevelVariable, and compat.go for the range this
	// shell complains outside of and the `shopt` letters that spell the same
	// state.
	s.CompatibilityLevelVariable = compatVariable
	// And the order the expansion comes back in, which is 5.3's and is this
	// column's alone: no other shell in the panel has a parameter for it.
	// See interp.Semantics.SortOrderVariable.
	s.SortOrderVariable = "GLOBSORT"
	s.IgnoredNamesRevealHiddenNames = true
	// A colon-separated list, matched against the *word* the expansion
	// produced, and a state an assignment latches rather than the
	// parameter's own: an inherited value does nothing until something
	// assigns it. ksh93's parameter of the same kind answers the other way
	// to all three.
	s.IgnoredNamesValueIsOnePattern = interp.No
	s.IgnoredNamesMatchTheLastComponent = interp.No
	s.IgnoredNamesFollowTheParameter = interp.No
	// `.` and `..` are not in what a pattern may match. bash 3.2 lists them
	// and 5.3 does not, so this is the version's answer and not bash's; no
	// preset here is bash 3.2.
	s.GlobListsDotAndDotDot = interp.No
	// And the files it reads by name. Measured 2026-09-05 through a
	// pseudo-terminal with a scratch home directory holding a marker for
	// every name any shell in the panel reads.
	//
	// The profile is a chain and exactly one link of it runs: with all three
	// present bash reads `~/.bash_profile`, with that one absent it reads
	// `~/.bash_login`, and with both absent `~/.profile`. bash 3.2 agrees.
	s.LoginStartupFiles = ".bash_profile .bash_login .profile"
	// And the file a login shell reads on the way *out*, which this shell
	// alone has. Measured 2026-09-22 with a marker in `~/.bash_logout`:
	// `bash -lc 'exit 3'` and `bash -l ./s.sh` whose own line exits both read
	// it; `bash -lc 'echo x'`, which runs out of input rather than exiting,
	// does not; and neither does a shell that is not a login shell. See
	// Semantics.LogoutFile for the rest of the row.
	s.LogoutFile = ".bash_logout"
	// And a script operand written without a slash is looked for along PATH,
	// the way a command word is — `bash ls` runs into `/bin/ls` rather than
	// reporting a file that is not there. See the field for the panel's row.
	s.ScriptOperandSearchedOnPath = true
	// `$0` can arrive in the environment here, which nothing else in the
	// panel does. See Semantics.DollarZeroFromEnvironment.
	s.DollarZeroFromEnvironment = "BASH_ARGV0"
	// `bash -i` reads `~/.bashrc` and nothing else, and does not read `$ENV`
	// — that is the same file under POSIX mode, where `~/.bashrc` goes
	// unread; the front end asks the mode rather than the dialect.
	s.InteractiveStartupFile = ".bashrc"
	// The system-wide file, and this shell has one only in the login slot.
	// Measured 2026-09-12: a login bash's `~/.bash_profile` reports
	// `path_helper`'s `${PATH%%:*}` and not the inherited one, so
	// `/etc/profile` ran in front of it — in 5.3, in 3.2 and under an
	// argv[0] of `sh` alike.
	//
	// **No system-wide run-commands file**, which is measured rather than
	// assumed and is the interesting half. `/etc/bashrc` exists on this
	// machine and sets `PS1` and `checkwinsize`; a `~/.bashrc` that reports
	// `$PS1` sees bash's own `\s-\v\$ ` default, and `shopt checkwinsize`
	// answers `off` in 3.2. So bash reaches `/etc/bashrc` only through
	// `/etc/profile`, which sources it by hand for a login shell, and a
	// shell that named it here would read it twice.
	s.SystemStartupFiles = interp.SystemStartupFiles{Login: "profile"}
	// The panel's holdout on ordering, and the reason every bash tutorial
	// tells a person to source `~/.bashrc` from their `~/.bash_profile` by
	// hand: `bash -l -i` reads the profile and stops. zsh reads both.
	s.InteractiveStartupFileWhenLogin = interp.No
	// The escape hatches, all three spelled long. `--rcfile` and
	// `--init-file` are the same option, measured to behave identically, and
	// both are carried because a person's muscle memory has one of them.
	s.StartupFileOptions = interp.StartupFileOptions{
		Login: "-l --login",
		// `--noprofile` names the *slot* and not the file: measured, `bash
		// --noprofile -l -c 'echo ${PATH%%:*}'` answers the inherited head,
		// so it suppressed `/etc/profile` along with `~/.bash_profile`.
		SuppressLogin:       "--noprofile",
		SuppressInteractive: "--norc",
		NameInteractive:     "--rcfile --init-file",
	}
	// What `bash --version` writes. Measured 2026-09-11: the real shell
	// answers on standard output at status 0, and the first line is the one
	// scripts read — see version, in prelude.go, for why the tag is there.
	s.VersionOption = interp.VersionOption{Spellings: "--version", Text: versionLine()}
	// And `--help`, which is the same usage block Diagnostics already writes
	// under a refused option with a version line over it and six lines under
	// it. Measured 2026-09-18 on bash 5.3.20 with standard input on
	// /dev/null: `bash --help` and `bash --badopt` were diffed line by line
	// and the twenty-two lines between them are byte-identical, which is why
	// the block is drawn from the one place rather than copied here.
	//
	// On standard output and at 0, where the refusal is standard error at 2 —
	// the word is a request this shell has rather than one it does not, and
	// before #3268 it was answered as `--help: invalid option` with the right
	// block under the wrong sentence.
	//
	// The trailer names the shell twice, as the block above it does. The last
	// two lines are addresses and the blank line before them is part of the
	// output, measured.
	s.HelpOption = interp.HelpOption{
		Spellings: "--help",
		Text:      helpVersionLine(),
		Trailer: "Type `%[1]s -c \"help set\"' for more information about shell options.\n" +
			"Type `%[1]s -c help' for more information about shell builtin commands.\n" +
			"Use the `bashbug' command to report bugs.\n" +
			"\nbash home page: <http://www.gnu.org/software/bash>\n" +
			"General help using GNU software: <http://www.gnu.org/gethelp/>",
	}
	// The option that writes the program back instead of running it, which
	// is this shell's alone in the panel. What it writes is the *listing*
	// layout applied to a file and not a formatter's output — every comment
	// and the shebang with it are gone — and the status a parse failure
	// exits is 1 where a run of the same script exits 2, both measured.
	// See interp.ScriptListingOption and ScriptListingLayout.
	s.ScriptListingOption = interp.ScriptListingOption{
		Spellings:          "--pretty-print",
		ParseFailureStatus: 1,
	}
	// The options that ask for the strings the program marked for
	// translation — `$"…"` — rather than a run of it. This shell's alone in
	// the panel, and the only one of the five that reads the mark at all.
	// Nothing here translates anything: see interp.StringCatalogOption.
	s.StringCatalogOption = interp.StringCatalogOption{
		Spellings:               "-D --dump-strings",
		PortableObjectSpellings: "--dump-po-strings",
	}
	// `-O shopt_option`, which is this shell's alone: the letter whose next
	// word is a name in the `shopt` table rather than a `set` option. The
	// usage block above already advertises it — `-ilrsD or -c command or -O
	// shopt_option (invocation only)` is bash's own line, printed here under
	// a refused option — and until #3264 it was advertising an option the
	// front end did not have, so `bash -O checkhash -c 'shopt checkhash'`
	// took `checkhash` as the script and exited 127 looking for a file
	// nobody named. The measurement across the panel is on the axis; the
	// names the letter reaches are in shopt.go.
	//
	// bash 3.2.57 has it too, measured, so it is not gated on the version.
	s.ShellOptionInvocationLetter = "O"
	// `-c` and `-s` together: the command string names the operands here,
	// so `sh -sc CMD name a` has `$0` of `name` and one parameter — the
	// same answer in the 3.2 macOS ships. ksh93 and zsh let `-s` name them.
	s.StdinOptionNamesTheOperands = interp.No
	s.CommandNotFoundStatusIsNotFound = interp.No
	// A pathname operand comes back unchanged from `command -v`, `command -V`,
	// `type`, `type -p`, `type -P` and `type -a` alike — `./bb/tool` stays
	// `./bb/tool` — against POSIX's reading, which only ksh93 follows.
	// Measured 2026-09-14 in 5.3.15 and in the 3.2.57 macOS ships.
	s.APathnameOperandIsReportedAbsolute = interp.No
	s.SetFTurnsOffGlobbing = interp.Yes
	// The one shell in the panel that selects an editing mode for itself.
	// Measured 2026-09-11 on 5.3.15 and on the 3.2 macOS ships, and under an
	// `argv[0]` of `sh` as well: `set -o` reports `emacs off` in a script and
	// `emacs on` under `-i`, with or without a terminal.
	s.InteractiveSelectsEmacs = interp.Yes
	// And starts with neither selected inside Emacs. See
	// Semantics.InsideEmacsTurnsEditingOff (#6310).
	s.InsideEmacsTurnsEditingOff = true
	// A `#` typed at this shell's prompt opens a comment only while
	// `interactive_comments` is on, and this shell has it on with nothing
	// said. Measured 2026-09-23 on bash 5.3.15, `printf … | bash --norc
	// --noprofile -i` on a pipe with a scratch HOME:
	//
	//	echo a #b                                a
	//	shopt -u interactive_comments; echo a #b  a #b
	//	…then shopt -s again; echo c #d           c
	//
	// The third line is why the axis holds a *name* rather than a bool: the
	// state moves mid-session and the answer has to be asked for per line.
	// The option is not a `set -o` name — `[[ -o interactive_comments ]]` is 1
	// on the same shell — so this reaches the prompt through the `shopt`
	// namespace's reader rather than the condition operator's lookup.
	//
	// The axis was the empty string here, which reads as "nothing has to be
	// on": right at the default and wrong the moment a script turns the name
	// off, and that is the whole of what `shopt -u interactive_comments` is
	// for (#4149).
	s.PromptCommentsNeedTheOption = "interactive_comments"
	// `set +B` stops `{a,b}` expanding and `set -B` puts it back — measured
	// 2026-09-11 on bash 5.3.15, both directions, and the letter leaves `$-`
	// while it is off. So it is not one-way the way `noexec` is (#1856).
	s.SetBTurnsOffBraceExpansion = interp.Yes
	// Measured: `echo $-` reports `hB` — hashall and braceexpand — under
	// -c, a script file and standard input alike, before the letters that
	// describe the route.
	s.DefaultOptionLetters = "hB"
	// And `hiBH` for `-i script.sh`, so the interactive set adds the
	// history-expansion letter and keeps everything else — measured
	// 2026-09-05 on bash 5.3.15, where `set -o` reports `histexpand on`
	// and `history on` there and both off for a script. `i` is not written
	// here: it is unanimous and comes from the runner.
	//
	// bash 3.2 answers `hiB` for the same invocation and `hiBHc` for `-i
	// -c`, so it disagrees with itself by route as well as with its later
	// build. 5.3 is the panel member that counts, as it is everywhere else.
	s.InteractiveOptionLetters = "hBH"
	// And the feature that letter names. bash has a history expander and
	// starts an interactive session with it on — measured 2026-09-15 through
	// a pseudo-terminal with a two-row prompt, where `echo one two three`
	// followed by `echo !!` echoes `echo echo one two three` to standard
	// error and prints `echo one two three`, with nothing configured and no
	// `set -H` typed. bash 3.2 answers identically, and so does the same
	// binary invoked as `sh`.
	s.HistoryExpansion = interp.Yes
	// `set -k` and `set -o keyword`, and this is the column that reaches
	// *past* a declaration utility with them: measured 2026-09-16, `set -k;
	// export E1=e1` leaves `export` listing the environment and `E1` unset.
	// See Semantics.KeywordAssignments and keywordassign.go.
	s.KeywordAssignments = interp.Yes
	s.KeywordPromotesADeclarationsOperand = interp.Yes
	// A declaration utility is recognized from the command word as written,
	// and only from an unquoted literal one: `cmd=export; $cmd v=$b`,
	// `\export v=$b`, `'export' v=$b`, `"export" v=$b`, `expor't' v=$b` and
	// `e=; $e export v=$b` all split the value where `export v=$b` keeps it.
	// Measured 2026-09-16 on bash 5.3.20 with `b='x y'`, for export, readonly,
	// local, typeset and declare alike. See #3339.
	s.DeclarationCommandWord = interp.DeclarationByUnquotedLiteralWord
	// And `command` in front of one does not keep the rule: `command export
	// v=$b` splits, in every spelling of the prefix. Measured the same day.
	s.CommandPrefixKeepsADeclaration = interp.No
	s.HistoryExpansionAtAPrompt = interp.Yes
	// And bash is the one column that uses the expander where nobody is
	// typing. Measured 2026-09-16 from a script file, from `-c` and from
	// standard input alike: after `set -o history` and `set -H`, `echo !!`
	// writes `echo echo one two three` to standard error and runs it, while
	// zsh with `setopt banghist` and ksh93 with `set -H` both print the two
	// characters. See Semantics.HistoryExpansionInAScript.
	s.HistoryExpansionInAScript = interp.Yes
	// And POSIX mode takes double-quoted text out of its reach. See
	// Semantics.HistoryExpansionSparesDoubleQuotesInPosixMode.
	s.HistoryExpansionSparesDoubleQuotesInPosixMode = interp.Yes
	// And an event's words are the shell's own. See
	// Semantics.HistoryWords.
	s.HistoryWords = interp.HistoryWordsShell
	// And a comment is left as written. See
	// Semantics.HistoryCommentStopsExpansion.
	s.HistoryCommentStopsExpansion = interp.Yes
	// `enable` has a `-d`, so a refusal names the operand rather than the
	// letter: measured 2026-09-18, `enable -d notbuiltin` is `enable:
	// notbuiltin: not a shell builtin` at 1 in 5.3.20 and in 3.2.57 alike,
	// where zsh answers `bad option: -d`. See
	// Semantics.EnableUnloadsABuiltin.
	s.EnableUnloadsABuiltin = interp.Yes
	// And a `G` in front of an `s` substitutes once per word. Measured
	// 2026-09-18 after `echo foo boo`, `!!:Gs/o/0/` is `ech0 f0o b0o`,
	// which zsh and ksh93 both refuse as a modifier they do not know. See
	// Semantics.HistoryWordwiseSubstitutionModifier.
	s.HistoryWordwiseSubstitutionModifier = interp.Yes
	// A `"` ends an event's name where it closes a string that is open there
	// and is a letter of the name where nothing is open — measured, `echo
	// "!zz"` blames `!zz` and `echo "$( echo "!zz" )"` blames `!zz"` (#4187).
	// See Semantics.HistoryClosingQuoteEndsAnEventName.
	s.HistoryClosingQuoteEndsAnEventName = interp.Yes
	// `bash -c 'echo $-'` reports `hBc`; ksh93 agrees and dash and zsh do
	// not. The `s` of the standard-input route is not added under `-c`
	// here — ksh93 alone does that.
	// `+i` takes the prompt back. Measured 2026-09-16 on bash 5.3.20 and
	// bash 3.2.57, the program on a pipe: `bash -i +i -c 'echo $-'` and
	// `bash +i -c 'echo $-'` both report `hBc`, with no `i` and no job-control
	// notice, where `-i` alone reports `hiBHc` and announces one.
	s.PlusSignedInteractiveLetterStillPrompts = interp.No
	s.CommandStringShowsCInDollarDash = interp.Yes
	s.LoginShowsLInDollarDash = interp.No
	s.CommandStringShowsSInDollarDash = interp.No
	// And the order it publishes them in: the lowercase letters sorted, then
	// the uppercase ones sorted, then the letter naming the route it was
	// invoked by. Measured 2026-09-12 on bash 5.3.15, and every row of it is
	// a shell disagreeing with the order the script wrote:
	//
	//	set -f; set -u; set -e   efhuBc
	//	set -C                   hBCc
	//	set -C -e                ehBCc
	//	set -a                   ahBc
	//	-i -c, at a terminal     himBHc
	//	set -e -C, on stdin      ehBCs
	//
	// The last two are what say the trailing letter is the route's and not
	// `c` in particular: `i` and `m` sort in with the rest where `s` does
	// not. Which of `c` and `s` comes first is not measurable here — this
	// shell never shows both, being the one that answers `No` to
	// CommandStringShowsSInDollarDash — so the pair is written in the order
	// the substrate produces them.
	// `r` sits between the interactive letters and `t`, which is measured
	// rather than guessed: `set -r` then `set -aefhkuvxBCEHT` is
	// `aefhkruvxBCEHTc`, a shell started `-i` reads `hirBHc`, and `set -r`
	// followed by `set -t` is `hrtBc`. So `k` is in front of it, `i` is in
	// front of it, and `t` and `u` are behind it.
	s.DollarDashLetterOrder = "aefhkilmnprtuvxBCEHPTcs"
	s.ArrayScalarIsTheWholeArray = interp.No
	// And the one element a plain `$m` on a keyed table gives is the one
	// keyed `0`, which is nothing at all where no such key was written.
	s.KeyedTableScalarIsTheFirstValue = interp.No
	s.ArrayNameWithoutSubscriptIsTheList = interp.No
	// No, and there is no way to ask for anything else: this shell has
	// neither zsh's `${^spec}` flag nor `RC_EXPAND_PARAM`, so a parameter
	// expansion is always laid into the word it stands in. Measured
	// 2026-09-25 on bash 5.3.20, `v='p q'; x${v}y` is the two words `xp`
	// and `qy` rather than `xpy` and `xqy`, and `set -- A B; x$@y` is `xA`
	// and `By` the same way (#4549).
	s.ParamExpansionDistributesOverTheWord = interp.No
	// A plain assignment's right-hand side is text and not a pattern, so
	// `a=*.txt` stores the six characters. Measured 2026-09-26 on bash
	// 5.3.20 in a directory holding `a.txt b.txt c.txt`: `a=*.txt; echo
	// "[$a]"` is `[*.txt]` where `echo *.txt` on the next line is the three
	// names, which is the control that says the directory and the pattern
	// were both real. Only zsh has a switch for the other reading (#4638).
	s.ScalarAssignmentValueIsGlobbed = interp.No
	// A keyed literal's bare elements are each one field: `typeset -A m=($k
	// $v)` with `k='1 2'` is the single key `1 2`, and `typeset -A m=(a *)`
	// keeps the star. See
	// Semantics.BareElementsInATableLiteralAreEachOneValue.
	s.BareElementsInATableLiteralAreEachOneValue = interp.Yes
	// And an odd number of them is taken rather than refused: the last field
	// becomes a key with nothing under it. Measured 2026-09-26 on 5.3.20,
	// `declare -A h=(a 1 b); declare -p h` is `declare -A h=([b]="" [a]="1" )`
	// at status 0, with the line after it run. See
	// Semantics.BareElementsInATableLiteralMustPairOff.
	s.BareElementsInATableLiteralMustPairOff = interp.No
	// A subscript inside a literal is an expression: `a=([1+1]=c)` lands at 2.
	s.ArrayLiteralSubscriptIsAKey = interp.No
	// A `[k]+=` element of a replacing keyed literal joins the value the
	// *name* held, which `m=(…)` is in the middle of discarding:
	// `typeset -A m; m[k]=v; m=([k]+=x)` is `vx` here and `x` in the other
	// two — see Semantics.KeyedLiteralAppendJoinsTheReplacedValue.
	s.KeyedLiteralAppendJoinsTheReplacedValue = interp.Yes
	// `typeset -A m[k]=v` — see Semantics.TableLetterReachesItsOwnOperandsSubscript.
	s.TableLetterReachesItsOwnOperandsSubscript = interp.Yes
	// bash reads a group built out of an expansion — see
	// Semantics.ExpansionResultSuppliesGroupSyntax.
	s.ExpansionResultSuppliesGroupSyntax = interp.Yes
	// `typeset -r a[1]=v` freezes the array and then loses the element write
	// to the freeze — see Semantics.ReadonlyElement. `readonly a[1]=v` never
	// reaches it here: that spelling is refused as a bad name first.
	s.ReadonlyElement = interp.ReadonlyElementFrozenFirst
	// The `@` family's letter is checked against the value rather than
	// against the spelling: `${u@QQ}` on an unset name is empty at status 0
	// and the same word on a set one is a bad substitution. Measured on
	// four spellings and on an empty array, which counts as no value.
	s.TransformLetterCheckedOnlyWhenValued = interp.Yes
	s.AssignmentUpdatesPipelineStatus = interp.Yes
	// Every bare assignment counts, so the array literal's question is not
	// reached; answered the same. See Semantics.ArrayAssignmentUpdatesPipelineStatus.
	s.ArrayAssignmentUpdatesPipelineStatus = interp.Yes
	s.TestAndArithmeticUpdatePipelineStatus = interp.Yes
	// And the record for one of those two is written *after* a `!` in front
	// of it: `false | true; ! [[ a = a ]]` leaves 1 here and 0 in zsh.
	// Measured 2026-09-11 on 5.3.15 and 3.2.57 alike. An ordinary command is
	// not affected — `! false` records 1 in both — so this is a rule about
	// the construct rather than about negation (#1513).
	s.NegatedTestRecordsThePostNegationStatus = interp.Yes
	// And a compound writes the record for nobody: what stands after one is
	// whatever the last pipeline that actually *ran* inside it left, so
	// `if false; then :; fi` holds the condition's 1 and `case a in b) :;;
	// esac` leaves the record from before it untouched, having run nothing.
	// A redirection on the compound does not change that, which is the
	// opposite of the rule the neighboring axes follow. Measured 2026-09-11
	// on 5.3.15 and 3.2.57 alike (#2016).
	s.CompoundPipelineStatusRecord = interp.CompoundPipelineStatusFromWhatRan
	s.UnsetEndsTheProducedPipelineStatus = interp.No
	// The lines of `eval`'s text continue the line the `eval` is written on,
	// and `$LINENO` moves with them — measured with the whole `eval` on one
	// physical line, which is the only arrangement that can tell this from
	// the physical reading (#2462).
	s.EvalTextContinuesTheCallersLines = interp.Yes
	// And once the text is done the line is the `eval`'s again: an ERR trap
	// the `eval` itself fires reads its own line, not the text's last.
	// Measured 2026-10-30 on 5.3.20, a three-line text held in `x` and
	// ending `false`, `eval "$x"` on line 10: the `false` inside fires at
	// 12 and the `eval` at 10, where this said 12 twice; bash 3.2.57 says
	// 10 for the `eval` as well (#6955).
	s.EvalPutsTheCallersLineBack = interp.Yes
	s.SelectLayout = interp.SelectMenuVerticalThenColumns
	s.SelectPromptNeedsTerminal = interp.No
	s.AliasParsesOptions = interp.Yes
	s.AliasHasPrintOption = interp.Yes
	// `alias -x` is `alias: -x: invalid option` here, at 2.
	s.AliasHasExportOption = interp.No
	// Neither of the other two kinds: `alias -g` and `alias -s` are both
	// `invalid option` here, measured on 5.3 and 3.2 alike.
	s.GlobalAliases = interp.No
	s.SuffixAliases = interp.No
	s.AliasListsAsDefinitions = interp.No
	s.AliasRestrictsToRegularKind = interp.No
	s.AliasOperandsCanBePatterns = interp.No
	s.AliasPlusPrintsNamesOnly = interp.No
	// The command hash, which bash keeps more of than the rest of the panel
	// and reads more strictly. It trusts what it remembered: a hashed path
	// that has gone is reported as that *path* at 127, where the other three
	// walk PATH again and run the next copy. The four letters past `-r`
	// follow from keeping a table worth addressing — `-l` writes it back as
	// commands, `-p` puts an entry in by hand, `-d` takes one out and `-t`
	// reports one. Measured 2026-09-13 against 5.3.15.
	s.CommandHashIsTrusted = interp.Yes
	s.HashListsAsCommands = interp.Yes
	s.HashTakesAPathToRemember = interp.Yes
	s.HashForgetsOneName = interp.Yes
	s.HashReportsThePath = interp.Yes
	// Not sorted: bash walks its own table's buckets, which is not an order
	// this shell can or should reproduce. See interp.Runner.hashedCommandNames.
	s.HashListingIsSorted = interp.No
	// And `set +h` really stops it: nothing is remembered until the option
	// comes back. zsh reads its own `hashcmds` the same way; ksh93 keeps
	// hashing with `trackall` off, and zsh's *letter* `-h` is a history
	// option that never reaches the table.
	s.HashObeysCommandTracking = interp.Yes
	// And the builtin itself closes with it, which zsh's reading of the same
	// option does not: every spelling answers one sentence at 1 here.
	s.HashRefusesWhileTrackingIsOff = interp.Yes
	// And a lookup that only *reports* where a command is leaves the table
	// alone, alone in the panel: `type ls >/dev/null; hash` is an empty
	// table here and holds `ls` in zsh, ksh93 and dash.
	s.ALookupRemembersThePath = interp.No
	s.TypeNamesAnAliasOnlyWhenExpanded = interp.Yes
	s.AliasReportsNotFound = interp.Yes
	s.UnaliasReportsNotFound = interp.Yes
	s.AliasNotFoundStatusCounts = interp.No
	// A removed alias leaves nothing behind: `alias h=1; unalias h;
	// unalias h` is 0 then 1 here, as it is in zsh, dash and BusyBox ash.
	s.AliasRemembersTheNamesItNames = interp.No
	s.AliasOptionEndsTheLookup = interp.No
	// `-p` is the whole listing and stops reading: every operand behind it
	// is discarded, so `alias a=1 b=2; alias -p a` writes both lines,
	// `alias -p nosuch` is silent at 0 where `alias nosuch` is
	// `alias: nosuch: not found` at 1, and `alias -p z=1` defines nothing.
	// The letter and not any option word — `alias -- a` lists `a` alone and
	// `alias -- nosuch` reports. Measured 2026-09-19 (#3701).
	s.AliasPrintOptionIgnoresItsOperands = interp.Yes
	// A command word that is exactly `-` is a command name here and is
	// reported as one: `- echo hi` is `command not found` at 127 and the
	// script carries on. zsh is the column that throws the word away (#3236).
	s.UnaliasAllRefusesOperands = interp.No
	s.AliasQuoting = interp.ListingQuoteAlwaysEscaped
	s.AliasListingQuotesTheName = interp.No
	s.TrapQuoting = interp.ListingQuoteAlwaysEscaped
	// `declare -p` writes `declare -- v="1"` — double quotes, unlike the
	// single-quoting listings above.
	s.DeclareListing = interp.DeclareListingClustered
	// And a listing with no operands writes the produced parameters with no
	// reading beside them: `declare -i RANDOM`, `declare -- SECONDS`,
	// `declare -- LINENO`, `declare -- EPOCHSECONDS`. Measured 2026-09-13,
	// bash 5.3.15, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over a
	// script file — and the same rows from the same binary under argv[0] of
	// `sh`, so the shape is bash's and not the invocation's.
	//
	// Not what the *named* `declare -p RANDOM` writes in that shell, which is
	// `declare -i RANDOM="16735"` — see interp.ProducedDeclaration, which is
	// where the letters come from and which this deliberately does not
	// re-derive. The two forms disagreeing in one shell is why the axis
	// exists (#2518).
	//
	// bash writes the reading once the parameter has been read, and then
	// writes the *same* one again — two listings a line apart both hold
	// `declare -i RANDOM="22963"`, where ksh93 and zsh draw again. So the
	// gate and the cache are one behavior: a parameter nothing has expanded
	// has no last reading to write. Neither half is modeled here, and both
	// are #2722.
	//
	// One letter moves with that gate and is left as the after state on
	// purpose. `SECONDS` lists as `declare -- SECONDS` until something reads
	// it and `declare -i SECONDS="0"` afterwards — measured 2026-09-13, and
	// it is the only one of the seven that moves; `RANDOM`, `SRANDOM` and
	// `BASHPID` carry `-i` from the start and `LINENO`, `EPOCHSECONDS` and
	// `EPOCHREALTIME` carry none either side. This engine writes `-i` in
	// both states, which is the letter the named `declare -p SECONDS` writes
	// in bash whatever has been read (#2451) and is the after state here.
	s.ProducedParameterListing = interp.ProducedListingLastReading
	s.DeclareValueQuoting = interp.ListingQuoteAlwaysDouble
	s.ListingControlEscape = interp.ControlEscapeOctal
	// A `#` in a listed value is quoted only where a comment could begin —
	// as the value's first byte — and nowhere else. Measured 2026-09-12 with
	// a bare `set`: `ab#cd`, `a#b#c`, `1#b` and `tail#` all list unquoted and
	// `#abcd` does not. It shows in `set` and not in `declare -p`, whose
	// values are double-quoted whatever is in them (#2299).
	s.ListedHashIsBareUnlessItOpensTheValue = interp.Yes
	// And a `~` by the same rule and in the same listings: `a~b` and `b~`
	// are bare, `~b` and `~` are quoted, and a key follows its value —
	// `[a~b]="1"` against `["~b"]="1"`. Measured 2026-09-17 on 5.3.20 and on
	// the 3.2.57 macOS ships, which agree; zsh and ksh93 quote every one.
	// The two position rules compose: `a#~b` and `a~b#c` are bare here and
	// `~a#b` is quoted (#2298).
	s.ListedTildeIsBareWhereItCannotExpand = interp.Yes
	// And the other side of the same character: the leading unquoted `~` of
	// an associative subscript is expanded before the text becomes a key, on
	// a store and on a read alike — `m[~/k]` is the element `$HOME/k` names.
	// Measured 2026-09-17 on 5.3.20; ksh93 agrees and zsh takes the
	// characters (#2298).
	s.SubscriptKeyExpandsALeadingTilde = interp.Yes
	// A bare `~` reads `HOME` as it stands, which is the substrate's answer
	// and is left alone here. GNU bash 5.3.20 answers from a *copy* instead —
	// see the note on Runner.homeForAWrittenTilde for the table and for why
	// that is a recorded refusal rather than a dialect's answer: bash 5.3.15,
	// this shell's own 3.2, zsh, ksh93, dash, ash and POSIX all read the
	// variable, so the cache is one patch range of one build.
	//
	// And with no home to read — `HOME` never set — the word becomes the home of
	// the user the process runs as, out of the same password database
	// `~user` reads. Measured 2026-09-23 on 5.3.20 and 3.2.57, which agree:
	// `env -i PATH=… bash -c 'printf "<%s>" ~'` is a path while `$HOME` is
	// empty, and `v=a:~:b` takes the same answer after the colon. A `HOME=`
	// that is *set* and empty is not this question and stays empty. See
	// interp.TildeWithNoHomePolicy (#4179).
	s.TildeWithNoHome = interp.TildeWithNoHomeReadsThePasswordEntry
	// And a word that merely *looks* like an assignment is a tilde context
	// here, which is this shell's alone: `echo make -k FOO=~/mumble` prints
	// the home directory, as does `foo=~:~` after each colon, while dash,
	// zsh, ksh93, BusyBox ash and this same binary under the `sh` name all
	// keep the two characters. Measured 2026-09-22 on 5.3.20 and 3.2.57,
	// which agree, over seven shapes and six boundary cases — the left side
	// has to be a name, so `--opt=~/m` and `1abc=~/m` are left alone
	// everywhere. POSIX mode takes it away and the axis is read with
	// Runner.PosixMode; see
	// Semantics.AnAssignmentShapedArgumentIsATildeContextOutsidePosixMode
	// (#4213).
	s.AnAssignmentShapedArgumentIsATildeContextOutsidePosixMode = interp.Yes
	// The three bytes the panel does not agree about in a listed word.
	// Measured 2026-09-14 over `set` and over a keyed `typeset -p`, which
	// agree: `=` is ordinary here — `v=a=b` and the keys `[a=b]`, `[=x]`,
	// `[1=2]`, `[x=y=z]` all bare — and so is a byte above ASCII, `v=é` and
	// `[é]`, which stays itself inside a `$'...'` a control byte opened:
	// `$'a\téb'`. `^` is not: `'^'` and `'a^b'`, where ksh93 leaves both
	// bare. This engine had the ksh93 answer to all three (#2820).
	s.ListedBangIsOrdinary = interp.No
	s.ListedCaretIsOrdinary = interp.No
	s.ListedEqualsIsOrdinary = interp.Yes
	// The character and not the escape, which is bash's answer in every
	// locale but `C` — measured 2026-09-15 on one binary, `LC_ALL=C`
	// writing `$'\303\251'` and every other setting, including none at
	// all, writing `é`. This is the character reading; the `C` cell is
	// ListedNonAsciiFollowsTheLocale's, which this dialect answers Yes.
	s.ListedNonAsciiIsOrdinary = interp.Yes
	// And the character is written as **itself** inside a `$'...'` a control
	// byte put this shell in, rather than as a code point, and no text in
	// front of it reaches for the form. Measured 2026-09-27 on GNU bash
	// 5.3.20 under `LC_ALL=en_US.UTF-8`: `v=$'a\téb'` lists as `$'a\téb'`
	// and `v="a-é"` lists bare (#4807).
	s.ListedNonAsciiIsSpelledAsACodePoint = interp.No
	s.ListedNonAsciiTakesTheDollarFormAfterANonName = interp.No
	// And nothing about the character itself narrows it either: measured
	// 2026-09-27 on 5.3.20 under `LC_ALL=en_US.UTF-8`, `°`, `€`, `×`, a
	// non-breaking space, `Ⓐ` and an emoji all list bare, where ksh93 takes
	// every one but `Ⓐ` into `$'...'` (#4829).
	s.ListedNonAsciiIsBareOnlyWhenAlphabetic = interp.No
	// And no bare assignment head: the `=` rule here is the character, not
	// a prefix, so `x=y=z` is bare rather than `x='y=z'`.
	s.ListedAssignmentPrefixIsBare = interp.No
	s.ExportListing = interp.DeclareListingClustered
	s.ReadonlyListing = interp.DeclareListingClustered
	// The bare form is this shell's `-p` form exactly, in both builds and in
	// the POSIX mode: `export` writes `declare -x V="a b"` however it is asked.
	s.BareDeclarationListing = interp.DeclareListingClustered
	// Two attribute letters on a listing with no names: the kind letters
	// narrow and the rest join. Measured 2026-09-10 over a table holding an
	// array, an integer array, an exported array, an association, an
	// integer, a readonly and an exported name — `declare -ir` writes the
	// integers *and* the read-only names, where `declare -ai` writes only
	// the array that is also an integer and `declare -aA` writes nothing.
	s.DeclarationListingFilter = interp.DeclarationFilterKindNarrowsAny
	s.DeclarePrintReportsAMissingName = interp.Yes
	s.DeclarePrintReportsAMissingFunctionName = interp.Yes
	// `declare -p e=(1 2)` lists the array it has just made and `declare -p
	// s=5` is `s=5: not found` at 1 with nothing stored — measured
	// 2026-09-20 on bash 5.3.20. See interp/declareprintoperand.go.
	s.DeclarePrintPerformsItsOperand = interp.DeclarePrintOperandIsDeclaredWhereItIsALiteral
	// And the `-p` letter on `export` and `readonly` is inert once operands
	// are written: measured 2026-09-20 on bash 5.3.20 and on bash 3.2.57,
	// `export -p w=8` writes nothing and leaves `w` at 8 and exported, and
	// `readonly -p u=9` writes nothing and freezes `u` at 9. The `n` letter
	// beside it still works, so it is this one letter that does nothing and
	// not the option word. See interp/exportprintoperand.go.
	s.ExportOrReadonlyPrintWithOperands = interp.ExportPrintLetterIsInert
	// The standard's listing writes every kind here: measured 2026-10-02 on
	// bash 5.3.20 under --posix, `export -p` writes `export -a a=(…)` and
	// `export -A h=(…)` beside `export s="1"`, and `readonly -p` the frozen
	// array. See Semantics.CommandWordListingNeedsAKindLetter.
	s.CommandWordListingNeedsAKindLetter = interp.No
	// unanswered NameBeyondASCIIStaysOutOfTheEnvironment: a name here is
	// ASCII under every locale, so there is no such name to export.
	s.TrapActionIsParsedWhenSet = interp.No
	s.TrapParseFailureNamesWhereItFired = interp.No
	s.SymbolicMaskTakesMoreThanOneOperator = interp.Yes
	s.SymbolicMaskWhoAloneSetsIt = interp.No
	s.SymbolicMaskOmittedWhoHonorsTheMask = interp.No
	s.SymbolicMaskTakesTheSetuidLetter = interp.Yes
	// Both are 5.x additions: bash 3.2 calls the `u` of `g=u` and the `X`
	// of `u=X` invalid symbolic mode characters, and this preset is 5.3.
	s.SymbolicMaskTakesAPermissionCopy = interp.Yes
	// And a copy written beside permission letters replaces what they
	// accumulated rather than joining them: from `umask 222`, `umask -S
	// g=uw` is `g=rwx` and `g=wu` is `g=rx`, which is the spelling that
	// parts this from dash (#3074). Measured 2026-09-18.
	s.UmaskPermissionCopyBesideLetters = interp.UmaskPermissionCopyReplaces
	s.SymbolicMaskTakesTheConditionalExecuteLetter = interp.Yes
	s.SymbolicMaskTakesTheStickyLetter = interp.Yes
	// No dash word is an option: `shift -x` complains about a number and
	// `shift -1` is a count out of range. The marker is honored all the same.
	s.ShiftOptionWords = interp.ShiftOptionWordsNone
	s.ShiftDoubleDashEndsOptions = interp.Yes
	// `--` in front of a numeric operand is taken and the number behind it
	// read: `break -- 1` ends the loop, `exit -- 3` exits 3. Identical in
	// 3.2 and in the same binary called as `sh`.
	s.NumericOperandDoubleDashEndsOptions = interp.Yes
	// A second operand is one too many, and the refusal costs the rest of
	// the statement — the loop stops, the `; echo` behind it never runs,
	// and the next line does. Measured 2026-09-16 on 5.3.20 in a script
	// file; 3.2.57 says the same sentence at status 1.
	s.ExtraNumericOperand = interp.ExtraNumericOperandGivesUpTheStatement
	// unanswered ShiftFromTheEndLetter: this shell has no `-p` under
	// `shift`, and does not read a dash word as an option there at all —
	// ShiftOptionWords above is the statement of that — so `shift -p 2` is
	// a *count* it cannot make a number of, and the complaint it already
	// makes is this shell's own. Measured 2026-09-26 on bash 5.3.
	s.ShiftNamesAreArrays = interp.No
	s.ShiftNegativeIsOutOfRange = interp.Yes
	s.ShiftCountTakesASign = interp.Yes
	s.WaitReadsOptions = interp.Yes
	// Job specs by command text, with a second match refused as ambiguous;
	// `wait` complains about a spec that names nothing, has -n, and
	// `disown` takes the job out of the table.
	s.JobSpecsByName = interp.Yes
	s.AmbiguousJobNameIsRefused = interp.Yes
	s.WaitReportsAMissingJob = interp.Yes
	// A bare `wait` reports a job a loud signal killed that it reaped itself,
	// with the row a named one writes. See
	// Semantics.BareWaitReportsASignalDeath (#5699).
	s.BareWaitReportsASignalDeath = interp.BareWaitReportsASignalDeathItReaps
	// Measured 2026-10-01: a finished job stays in the table until something reports it: `(exit 4) & sleep 0.3; wait %%` is 4. See Semantics.FinishedJobLeavesTheTable.
	s.FinishedJobLeavesTheTable = interp.No
	// Whether a job builtin writes a finished job's notice first, with the
	// monitor on and no prompt (#6064). See
	// Semantics.FinishedJobIsReportedByAJobBuiltin.
	s.FinishedJobIsReportedByAJobBuiltin = interp.No
	// A file `.` reads from a startup file is numbered from its own first line (#6013). See
	// Semantics.SourcedFileContinuesTheStartupLineCount.
	s.SourcedFileContinuesTheStartupLineCount = interp.No
	// A startup file given up over a fatal error: bash leaves 1, the fatal status of a file, whatever ran before (#6046).
	// See Semantics.StartupFileGivenUpLeavesTheStatusBefore.
	s.StartupFileGivenUpLeavesTheStatusBefore = interp.No
	// A failed expansion in a command's words: bash sets 1 (#6067).
	// See Semantics.FailedExpansionInACommandKeepsAFailingStatus.
	s.FailedExpansionInACommandKeepsAFailingStatus = interp.No
	// A fatal error in an interactive shell's program: bash gives up the line of a `-c` string or a script under `-i` (#6073).
	// See Semantics.InteractiveProgramErrorWhenInteractive.
	s.InteractiveProgramErrorWhenInteractive = interp.InteractiveProgramErrorCostsTheLine
	// At a prompt, bash numbers the session (#6074). See Semantics.PromptNumbersEachInputFromOne.
	s.PromptNumbersEachInputFromOne = interp.No
	// A fatal error in `.` or `eval` text: an interactive bash gives up the line of `.` or `eval` text a fatal error was on and reads on (#6057).
	// See Semantics.BorrowedTextErrorWhenInteractiveCostsTheLine.
	s.BorrowedTextErrorWhenInteractiveCostsTheLine = interp.Yes
	// A fatal error in an interactive shell's startup file: bash gives up the line and runs the rest of its `.bashrc` (#6009).
	// See Semantics.StartupFileErrorWhenInteractive.
	s.StartupFileErrorWhenInteractive = interp.StartupErrorCostsTheLine
	// A job started before `set -m`, under a terminal: bash refuses it: `fg: job 1 started without job control` (#5927). See
	// Semantics.JobStartedWithoutTheMonitorIsRefused.
	s.JobStartedWithoutTheMonitorIsRefused = interp.Yes
	// And a job it has already reported stays waitable by its process id
	// once the job has left the table — measured on both routes into it,
	// `wait %1; wait "$p"` and `wait "$p"` twice, in 5.3.15 and 3.2.57
	// alike. See the axis for the narrower answer this same binary gives
	// when it is invoked as `sh`.
	s.WaitRemembersAReapedJob = interp.Yes
	// A named `wait` says which signal ended the job it reaped, in the
	// foreground's sentence, and keeps quiet for a TERM. Measured
	// 2026-10-02 on bash 5.3.20 with a job killed after it started: `sleep 1
	// & kill -USR1 $!; wait $!` writes `line 1: PID User defined signal 1:
	// 30  sleep 1`. The 2026-09-18 row that said nothing was a TERM, which
	// is the one signal this route leaves unsaid — see
	// Diagnostics.WaitIsQuietForTerminate (#5389).
	s.WaitReportsTheSignalThatEndedTheJob = interp.Yes
	s.WaitNextJob = interp.WaitNextJobFirstToFinish
	// And `-p var` beside it, which names the job the status came from.
	// bash 5's letter alone: the 3.2 build answers `wait: -p: invalid
	// option`. Measured 2026-09-13.
	s.WaitPNamesTheFinishedJob = interp.Yes
	// A trapped signal cuts a `wait` short with 128 plus the signal, and the
	// form that names a job answers the same as the bare one.
	s.WaitForAJobFailsWhenInterrupted = interp.No
	s.DisownRemovesTheJob = interp.Yes
	// And it reports what it did: 0 for a job it disowned, 1 with `no such
	// job` for one it could not find (#3187).
	s.DisownAlwaysFails = interp.No
	s.CommandRejectsUnknownOption = interp.Yes
	// Whether `command -v` answers for every name it was given, and what
	// decides the status when it found some of them. See
	// interp.Semantics.CommandReportsEveryOperand for the split.
	s.CommandReportsEveryOperand = interp.Yes
	s.CommandCountsAMissingOperand = interp.No
	// And whether a `command` reached through an expansion keeps the power
	// to run what it names (#3369).
	s.ExpandedCommandOnlyReports = interp.No
	// The word takes the named builtin's specialness away and draws no
	// boundary of its own: a fatal error raised *inside* it is still fatal.
	// Measured 2026-09-13 in subshells under `env -i`, three producers that
	// this shell does end a script for — `command eval 'echo "${NOPE?bad}"'`,
	// `readonly rv=1; command eval 'rv=2'` and `set -u; command eval 'echo
	// "${NOPE}"'` — each ends the subshell at 1 with the word and without it,
	// under this shell's own name and under `sh` alike. Both builds agree.
	s.FatalErrorEndsAtTheCommandWord = interp.No
	// A declaration's value refused by a freeze is never fatal behind the
	// word, by either name or build — `f() { command readonly x=5; echo in;
	// }` prints `in` under `sh` as well (#6962). See
	// interp.Semantics.FrozenOperandRefusalOutlivesTheCommandWord.
	s.FrozenOperandRefusalOutlivesTheCommandWord = interp.No
	s.GetoptsRejectsUnknownOption = interp.Yes
	s.ShiftCountIsArithmetic = interp.No
	s.TrapBodyRunsWhatParsed = interp.Yes
	// A signal body, an EXIT body and a RETURN body count from their own
	// first line — the zero value, and what TrapBodyLine leaves alone —
	// while a DEBUG or ERR body counts from the line the condition fired
	// on. Measured with a two-line body: fired before a command on line
	// 4, `echo $LINENO` on the body's first line prints 4 and a failure
	// on its second is located at line 5, where the same body on a signal
	// trap prints 1 and locates the failure at line 2. bash 5.3,
	// bash-as-sh and bash 3.2 all answer this way.
	s.CommandTrapBodyLine = interp.TrapBodyLineOffsetFromWhereItFired
	// And an EXIT body, which has no line of its own, counts as having
	// fired on the first rather than past the script's last. Reachable here
	// through the clause above: a DEBUG trap fires for the commands of an
	// EXIT body, and that body's line base is the EXIT trap's firing line.
	// Measured 2026-09-22 on bash 5.3.20 from a five-line script file with
	// a two-line DEBUG body and an EXIT trap, `echo $LINENO` on each body
	// line: inside the EXIT trap the pair is `1`/`2`, not the `6`/`7` that
	// counting past the end would give (#4193).
	s.ExitTrapFiresPastTheEnd = interp.No
	s.ExitTrapFiresWhereTheScriptStopped = interp.No
	// A `kill -CHLD` naming the shell runs nothing: the condition is a child
	// of this shell changing state, and a signal the script sent is not that.
	// Measured 2026-09-26 over a script file, `trap 'echo C' CHLD` /
	// `kill -CHLD $$; echo a` / `echo b` reads `a b` (#4756).
	s.SelfAimedChildSignalRunsTheTrap = interp.No
	// Every child this shell reaped raises the condition, a foreground
	// external command included, rather than a background job alone.
	// Measured 2026-09-27 over script files, `trap 'echo C' CHLD` and then
	// `/bin/echo x`, a pipeline, `v=$(/bin/echo x)` or `( /bin/echo x )`,
	// each followed by `echo a`: every one of them runs the handler (#4780).
	s.ChildConditionCountsEveryReapedChild = interp.Yes
	// And a `WINCH` aimed the same way runs between commands like every other
	// signal. Measured 2026-09-26 over a script file, `trap 'echo W' WINCH` /
	// `kill -WINCH $$; echo a` / `echo b` reads `W a b` (#4755).
	s.SelfAimedWindowChangeWaitsForInputOrAChild = interp.No
	s.ReportsAKilledCommandInACommandSubstitution = interp.No
	// `set -e` is the one option a `$(…)` body does not simply inherit
	// here. Measured 2026-09-15, `set -e; echo "end[$(false; echo no)]"` is
	// `end[no]` in bash 5.3.20 and in bash 3.2.57, and the option is
	// genuinely off in the body rather than unenforced — `$-` inside it
	// carries no `e` and `set -o` reports `errexit off`. The starting value
	// only: this is the `inherit_errexit` switch below, and `set -o posix`
	// turns it on by the other door.
	s.ErrExitEntersACommandSubstitution = interp.No
	// A `${ … ;}` body is a variable scope here as well as a frame: a
	// declaration inside one is local to the body and the shell's own name
	// comes back after it. Measured 2026-09-16 from a script file —
	// `x=outer; v=${ local x=in; printf %s "$x"; }` is `[in]` with `x` still
	// `outer`, and `declare` and `typeset` answer the same — where ksh93u+,
	// the only other column with the construct, leaves `x` as `in`. Nested
	// in a call the scopes nest: with `local x=fn` around it the name comes
	// back as `fn`.
	//
	// A plain assignment is not this and still writes the shell's own name,
	// which is what keeps the spelling different from the forked one. And
	// `local` being legal inside a body follows from the scope rather than
	// being a second decision — in `$( … )` it is refused here exactly as it
	// is at the top level.
	s.CurrentShellSubstitutionBodyIsAScope = interp.Yes
	s.SelectAssumesUnboundedWidth = interp.No
	s.SelectEofEndsPromptLine = interp.No
	s.SelectEofIsSuccess = interp.No
	s.SelectTakesUnterminatedReply = interp.No
	s.SelectEofPrintsNewline = interp.Yes
	s.AssignmentPrefixPersistsOnSpecialBuiltin = interp.No
	// And `command` in front of one takes the persistence away where the
	// option puts it there. Reachable only under `set -o posix`, which is
	// where the row above turns yes, and measured there: 2026-09-18,
	// `set -o posix; s=base; s=C command :` leaves `base` while the bare
	// `s=P :` leaves `P` (#3448).
	s.CommandKeepsASpecialBuiltinsPrefix = interp.No
	// A prefix to a *function* is the command's environment for the length
	// of the call and nothing afterwards: `f(){ echo "[$v]"; }; v=1; v=9 f`
	// prints `[9]`, a command the body starts is told `v=9`, and `$v` is `1`
	// on the next line. Measured 2026-09-12 in 5.3.15 and in 3.2.57, and the
	// same either way round the `posix` switch in 5.3 — 3.2 is the build
	// that moves under it, which is what macOS `/bin/sh` is (#2407).
	s.AssignmentPrefixPersistsAfterAFunction = interp.No
	s.PrefixToAFunctionIsExported = interp.Yes
	// A prefix in front of a `function`-form function writes this shell's
	// own cell, exactly as it does in front of a POSIX-form one: measured
	// 2026-09-18 on 5.3.20, `s=base; function kf { echo "[$s]"; }; s+=5 kf`
	// shows the body `base5`, which is the append reading the shell's value
	// (#3161).
	s.PrefixToAKeywordFunctionIsScopedToTheCall = interp.No
	// An array literal written as a prefix is a scalar holding its text, the
	// elements expanded and joined: measured 2026-10-15 on 5.3.20, `x=(a
	// $(echo b)) printenv x` is `(a b)` and `f(){ typeset -p x; }; x=(a b) f`
	// lists `declare -x x="(a b)"` (#6547).
	s.PrefixArrayLiteralIsAnArray = interp.No
	// The cell that prefix writes is the prefix's own, though, and not the
	// binding the name already had: `foo=(asdf fdsa); ff() { declare -p foo;
	// }; foo=bar ff` lists `declare -x foo="bar"` — a plain exported scalar,
	// with the array, the table and every letter gone for the length of the
	// call. Measured 2026-09-21 in 5.3.20 and in 3.2.57 on every row 3.2 can
	// be asked, so the two builds do not split. The body's readings move with
	// it: `${#foo[@]}` is 1, and `declare -i foo=7; foo=bar ff` hands the body
	// `bar` rather than reading the word as arithmetic (#4087).
	s.AssignmentPrefixMakesAFreshCell = interp.Yes
	// And a prefix to a *builtin* is exported too, which is the reading that
	// makes it the command's environment rather than a value this shell holds
	// for one line: `v=1; v=9 eval 'env | grep "^v="'` hands the child `v=9`
	// and `c=1; c=2 declare -p c` reads `declare -x c="2"` on a name nobody
	// exported. Measured 2026-09-16 in 5.3.20 and in 3.2.57, and bash is the
	// only column that does either (#3437).
	// The prefix is worked through **before** the redirections are opened, so
	// a substitution in a value still runs when one fails: measured
	// 2026-09-18, `f() { :; }; w=$(echo S >&2) f > /nope/x` writes `S` and
	// then the file complaint, where zsh, dash and BusyBox ash write the
	// complaint alone (#3449).
	s.PrefixExpandedBeforeTheRedirections = interp.PrefixExpandedBeforeRedirectionsAlways
	// Whether a command's trace line waits for its redirections. See
	// interp.Semantics.TraceLineFollowsTheRedirections (#5547).
	s.TraceLineFollowsTheRedirections = interp.No
	// See interp.Semantics.TraceRedirectionsOnALineOfTheirOwn (#5558).
	s.TraceRedirectionsOnALineOfTheirOwn = interp.No
	// bash writes an assignment's line before its store (#5546).
	// See interp.Semantics.TraceLineFollowsTheStore.
	s.TraceLineFollowsTheStore = interp.No
	// And a declaration utility's operand is reached **before** that prefix,
	// which is the opposite half of the same sequence: measured 2026-09-19,
	// `PRE=$(echo PRE >&2) export s=$(echo OP >&2)` writes `OP` and then
	// `PRE` here and in bash 3.2, where ksh93 and zsh write `PRE` first
	// (#3814).
	s.PrefixExpandedBeforeADeclarationsOperand = interp.No
	// And the entries are put back before a failure ends the shell, under
	// `-o posix` too: measured 2026-10-30, `trap 'echo $v' EXIT; v=old;
	// v=new w=${q?boom} :` writes `old` (#7004).
	s.PrefixEntriesStayWhenAFailureEndsTheShell = interp.No
	s.PrefixExportAtABuiltin = interp.PrefixExportAtABuiltinOn
	// And a listing with **no operand** does not see that prefix at all: it
	// answers from the shell's own variables, so `export k=1; k=9 export -p`
	// writes `declare -x k="1"` on the same line whose `k=9 typeset -p k`
	// writes `declare -x k="9"`. A name the prefix created is in no
	// whole-table listing here. Measured 2026-09-18 (#3446).
	s.PrefixInAWholeTableListing = interp.PrefixInAWholeTableListingIsNotThere
	// A declaration naming `-x` or `-r` over the name its own prefix is
	// standing in front of keeps the prefix's value: `b=7; b=8 readonly b`
	// leaves `declare -rx b="8"`, and so do `export` and `typeset -r`. Not
	// the special-builtin rule above — `b=8 :` leaves `7` here — and not
	// every declaration: `x=2 declare x`, `i=2 declare -i i` and `t=2
	// declare +x t` all leave the shell's own value standing (#3437).
	s.DeclarationPromotesThePrefixEntry = interp.Yes
	// A subscripted name in a prefix is refused by name and the command runs
	// anyway: `a[1]=v f` writes `` `a[1]': not a valid identifier ``, calls
	// `f` with the element untouched, and reports what `f` reported. The
	// other entries of the same prefix are applied — `w=5 a[1]=1 f` shows
	// the body `w` — so the refusal is of the word and not of the prefix.
	// Measured 2026-09-16 on bash 5.3.20 (#3433).
	//
	// unanswered SubscriptedPrefixIsTakenBack: nothing is stored to take
	// back. The axis is asked only where SubscriptedAssignmentPrefix writes
	// the element, and this dialect refuses it instead.
	s.SubscriptedAssignmentPrefix = interp.SubscriptedPrefixIsRefused
	// echo reads -n, -e and -E, the last of -e/-E deciding, with the hex
	// and ESC escapes on top of the XSI set.
	s.EchoOptions = "neE"
	s.EchoLastEscapeFlagWins = interp.Yes
	s.EchoExpandsHexEscapes = interp.Yes
	// `echo -e '\u0041'` and `echo -e '\U00000041'` are both `A` in 5.3.15,
	// and a run with no digit at all is left as written with a complaint —
	// the leaving is what the second axis records here.
	s.EchoExpandsUnicodeEscapes = interp.Yes
	// A code point the locale cannot hold leaves the escape standing,
	// normalized to four or eight upper-case digits, and the command carries
	// on. Measured 2026-09-11 under `LC_ALL=C` (#1851).
	s.UnicodeEscapeOutsideTheLocale = interp.OutsideLocaleEscapeWritten
	// And the radix character, which this shell takes from the locale and reads
	// a number *only* at: measured on macOS 15 with the host's own
	// `de_DE.UTF-8`, `printf '%.4f' 1` is `1,0000` and `printf '%.2f' 1.5` is
	// `printf: 1.5: invalid number` at 1. bash 3.2.57 answers both the same
	// way, so this is not a dated reading. See interp.Semantics.NumberRadix.
	s.NumberRadix = interp.RadixIsTheLocalesOwn
	// And a value the six-byte form cannot hold writes **nothing at all** for
	// the escape, with the rest of the word carried on to the stream.
	// Measured 2026-09-22 under both `LC_ALL=C` and `LC_ALL=en_US.UTF-8`,
	// `printf '%s' $'a\UFFFFFFFFb'` is `a b` in 5.3.20 where `$'a\U7FFFFFFFb'`
	// — the last value the form has room for — writes all six bytes between
	// them. zsh carries the arithmetic past the ceiling instead.
	s.CodePointPastSixBytesIsEncoded = interp.No
	// An *unset* locale is UTF-8-capable here, which is this shell alone in
	// the panel and is measured on three operators at once: under `env -i`,
	// 5.3.15 answers 5 for `s=héllo; echo ${#s}`, uppercases `café` to
	// `CAFÉ`, and writes `61 c3 a9 5a` for `echo -e 'a\u00e9Z'`. ksh93u+ and
	// zsh 5.9.2 answer 6 and `CAFé` there, and 3.2.57 answers 6 — so this is
	// the modern build's reading rather than the family's (#2020).
	s.UnsetLocaleIsUnicodeAware = interp.Yes
	// A pattern the encoding cannot decode drops the whole match to bytes
	// here, so a byte that begins no character finds itself inside one.
	// Measured 2026-09-22 under `LC_ALL=en_US.UTF-8` on 5.3.20: with the euro
	// sign in `e` and its middle byte alone in `b`, `[[ $e == *$b* ]]` matches
	// and `${e#*$b}` is the single trailing byte, where zsh and ksh93 leave
	// both alone. A pattern of ASCII still counts characters — `[[ $e == ?? ]]`
	// finds nothing in all three — which is what makes this the pattern's
	// reading rather than a standing preference for bytes.
	s.UndecodablePatternComparesBytes = interp.Yes
	s.EchoEmptyHexDigitRunIsNul = interp.No
	// Both spellings of the escape character, which is this shell alone in
	// the panel: ksh93 has only `\E` and zsh only `\e`.
	s.EchoExpandsEscEscape = interp.Yes
	s.EchoExpandsCapitalEscEscape = interp.Yes
	// read takes -r, -s, -e and -E plus the argument letters: -a names the
	// array in the option's argument, -d a delimiter, -i the text a line
	// editor would be seeded with, -n and -N the two counts, -p a prompt for
	// when the input is a terminal, -t a timeout and -u a descriptor. A
	// short -n keeps its text and reports 1; a short -N keeps its text too.
	//
	// -e and -E are the line editor's two letters and they are read and
	// dropped, which is the measured whole of them wherever the input is not
	// a terminal — and the input `read` is given is not one in any script.
	// Measured 2026-09-22 against a pipe: `read -e x`, `read -E x y`,
	// `read -e -i seed x` and `read -e -n 2 x` each answer exactly as the
	// same line without the letter does, seed and all, and a bare `read -e`
	// at end of input reports 1 with the name cleared. Refusing them by name
	// instead cost `read.tests` five lines and two statuses (#4170).
	// unanswered ReadArrayDefault: the array letter here is the lowercase
	// `-a`, and its name is the option's own argument rather than an
	// operand — so a line that names no array is refused by the option
	// parser and never reaches the question. Measured 2026-09-26 on bash
	// 5.3.20: `read -a <<<'a b'` is `read: -a: option requires an argument`
	// at 2, with no `REPLY` afterwards.
	s.ReadOptions = "rseEa:d:i:n:N:p:t:u:"
	// Measured 2026-10-01: a NUL is dropped from what `read` and a substitution take in (the substitution says so, in a warning). See Semantics.NulInAValue.
	s.NulInAValue = interp.NulDropped
	// And `-e` and `-E` are the **line editor** here, not zsh's echo: `-e`
	// reads the line through readline and `-E` reads it with the shell's
	// default completion bound, and off a terminal both do nothing at all.
	// Measured 2026-09-28, `bash --norc -c 'printf "  a   b   c  \n" |
	// { read -E x y; declare -p x y; }'` writes not one byte before the
	// declarations, where zsh 5.9.2 writes `a` and `b   c`. So the letters
	// stay in the set above and this is the No that keeps them doing
	// nothing (#4963).
	s.ReadEchoLettersWriteTheValues = interp.No
	// `-n` unsets through a name reference. bash 3.2 does not have it —
	// `unset -n x` is an invalid option there and under `--posix` — so
	// this is 5.3's set, which is the binary the panel measures.
	s.UnsetOptions = "vfn"
	// `readonly` takes the kind letters and the function letter as well as
	// POSIX's `-p`. Measured 2026-09-12: `readonly -f zz` is `zz: not a
	// function` at 1, so the letter is read; `-i`, `-x`, `-r`, `-g`, `-l`,
	// `-u` and `-t` are all `invalid option` at 2. bash 3.2 has no `-A` —
	// its usage line is `readonly [-af]` — which is a change within bash
	// rather than a difference between shells, and this is 5.3's set,
	// the binary the panel measures.
	s.ReadonlyOptions = "paAfn"
	// And what the `n` of that set does, which is not what the same letter
	// does on `export`: nothing is taken off. Measured 2026-09-18 from a
	// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch
	// HOME — `v=1; readonly -n r=v` is 0 and `declare -p r` is `declare --
	// r="v"`, which `r=5` then writes; `readonly q=1; readonly -n q` leaves
	// `q` frozen; and `readonly -n z=2` over a frozen `z` is the ordinary
	// `z: readonly variable` at 1. So the letter suppresses the freeze this
	// call would have made and does nothing else (#3464).
	s.ReadonlyReferenceLetter = interp.ReadonlyReferenceLetterDeclaresAnUnfrozenName
	// Measured 2026-09-12: `x=1; unset -n x` leaves `x` at 1 and reports 0,
	// where ksh93 removes it. `-n` names the reference and this shell reads a
	// name that is not one as naming nothing at all — far enough that
	// `unset -n 1x` is silent at 0 while plain `unset 1x` refuses the
	// identifier, though a readonly name is still refused (#932).
	s.UnsetReferenceLetterRemovesANonReference = interp.No
	// A name reference that reaches itself through *another* reference is
	// made here rather than refused, and the complaint arrives when
	// something reads through it. Measured 2026-09-15 on 5.3.15: `declare -n
	// a=b; declare -n b=a` is a silent 0, and `echo "$a"` then writes
	// `warning: a: circular name reference` followed by an empty line. The
	// direct `declare -n r=r` is refused in both shells and is the core's
	// answer rather than this axis — see interp/nameref.go.
	s.NamerefCycleIsRefused = interp.No
	// A `-n` declaration over a name carrying an array is refused **after**
	// the two refusals about the name itself, and the array *attribute* is
	// enough: measured 2026-09-16 on 5.3.20, `typeset -a r; typeset -n r=v`
	// is `r: reference variable cannot be an array` at 1 even though `r`
	// holds nothing, while `r=(a b); typeset -n r=r` is the self-reference
	// sentence instead. ksh93 answers both halves the other way round
	// (#3103).
	s.NamerefArrayRefusal = interp.NamerefArrayCheckedLastOnTheAttribute
	// A declaration operand carrying a **subscript** takes the reference
	// attribute off and writes the element on the name itself: measured
	// 2026-09-23 on 5.3.20 and 5.3.15 alike, `typeset -n xref; typeset -a
	// xref[1]=one` is `warning: xref: removing nameref attribute` and then
	// `declare -a xref=([1]="one")` at 0, and an *aimed* reference goes the
	// same way — `array=(p q); typeset -n xref=array; typeset -a
	// xref[1]=one` leaves `xref` holding the array and `array` untouched,
	// where ksh93 writes through it. The plain statement `xref[1]=one` is
	// the other road and keeps the reference (#4178).
	s.NamerefDroppedByASubscriptedOperand = interp.Yes
	// And an array letter written over a reference that has never been
	// aimed takes the reference away rather than standing beside it:
	// measured 2026-09-22 on 5.3.20, `typeset -n foo; typeset -a foo` lists
	// `declare -a foo` and `foo[0]=7` then lands in it, where ksh93u+ lists
	// `typeset -n -a foo` and answers the same write `foo: no reference
	// name`. See Semantics.ArrayLetterOverAnUnaimedReferenceDropsIt.
	s.ArrayLetterOverAnUnaimedReferenceDropsIt = interp.Yes
	// The `n` letter is read beside every other one here and each pair is
	// decided on its own. Measured 2026-09-18 on 5.3.20, `env -i` with a
	// scratch HOME, from a file, with `v=1`: `declare -nx r=v` and
	// `declare -nr r=v` make an exported and a frozen reference, `declare
	// -nu r=v` aims one at `V`, `declare -na r=v` makes a plain array, and
	// only `declare -ni r=v` is refused — at 1, silently, for a reason
	// about the *value* rather than about the pair. So there is no bundle
	// this shell will not read.
	s.NamerefLetterStandsAlone = interp.No
	// The target is the **text** the declaration was written with, looked up
	// again at every read: measured 2026-09-18 on 5.3.20, `a=(x y z); i=2;
	// declare -n r=a[i]` lists `declare -n r="a[i]"` and reads `x` after
	// `i=0`, and `declare -n s=u; declare -n s2=s` lists `s2="s"` and follows
	// `s` wherever it is re-aimed. Which is what lets `declare -n r=a[@]`
	// stand at all — `@` is not arithmetic, and here it never has to be
	// (#3124, #3172).
	s.NamerefTargetResolvedWhenAimed = interp.No
	// And a refusal an assigning declaration reaches through a reference is
	// spoken of under the operand: measured 2026-09-18 on 5.3.20, with `u=1;
	// readonly u; declare -n s=u`, `declare s=9` is `declare: s: readonly
	// variable` where the valueless `declare -i s` beside it is `declare: u:
	// readonly variable` (#3173).
	s.DeclarationThroughAReferenceNamesTheOperand = interp.Yes
	// The element and array writes beside it, measured 2026-10-16 (#6763): see
	// interp.Semantics.ContainerWriteThroughAReferenceNamesTheReference.
	s.ContainerWriteThroughAReferenceNamesTheReference = interp.Yes
	// Measured 2026-10-16 (#6781): see
	// interp.Semantics.DeclarationOperandSplitsAfterTheSubscript.
	s.DeclarationOperandSplitsAfterTheSubscript = interp.Yes
	// `export` and `readonly` over a reference aimed at one **element**
	// refuse the attribute by the target's own text and let the value
	// through: measured 2026-09-20 on 5.3.20, with `a=(p q r); typeset -n
	// b='a[1]'`, `export b=Z` writes ``export: `a[1]': not a valid
	// identifier`` at status **0**, leaves `a` as `p Z r`, and puts the
	// export letter nowhere — `declare -p a` is `declare -a a`. `readonly
	// b=Y` is the same sentence under its own name and leaves `a` unfrozen,
	// so a later `a[2]=N` is taken at 0. The control is `declare -x b` beside
	// it, which lands the letter on `a` silently: it is these two builtins'
	// name check and not the letter (#3881).
	s.ExportOrReadonlyTakesAReferenceToAnElement = interp.No
	s.ReadZeroTimeout = interp.ReadZeroTimeoutPolls
	// unanswered ReadArrayTakesOneNameOnly: this shell's array letter is
	// `-a` and takes its name as the option's own argument, so every operand
	// after it is an ordinary name to fill and there is no second array
	// operand to judge. Measured 2026-09-26 on bash 5.3.
	s.ReadPartialCountSucceeds = interp.No
	// A count of zero reads nothing: `printf 'ab\ncd\n' | { read -n 0 v;
	// read w; }` leaves v empty at 0 and w=ab. bash 3.2 and 4.4 read the
	// line instead, as though no count were given; 5.0 is the first release
	// that does not. This preset is bash 5's. See
	// interp.Semantics.ReadCountOfZeroIsNoCount (#7036).
	s.ReadCountOfZeroIsNoCount = interp.No
	s.ReadExactCountKeepsPartial = interp.Yes
	// A count reads a pipe at a number past 2 as it reads anything else, and
	// a count that found the end leaves the next read to find it too:
	// `printf 'abcdef\nxyz\n' | bash -c 'exec 3<&0; read -r -n 4 -u3 l; read
	// -r -u3 m'` is l=abcd at 0 and m=ef, and `: | bash -c 'read -r -n 4 l;
	// read -r m'` is 1 and then 1. Measured 2026-10-31 on bash 5.3.20. See
	// interp.Semantics.ReadCountOfAPipeExecOpenedFindsItsEnd and
	// ReadCountAtTheEndSticksTheDescriptor (#7179).
	s.ReadCountOfAPipeExecOpenedFindsItsEnd = interp.No
	s.ReadCountAtTheEndSticksTheDescriptor = interp.No
	// A `read -N` reads on through a trapped signal here, as every read does
	// (ReadIsAbandonedByATrappedSignal is no), so nothing it took is ever
	// left over for a later read to find. See
	// interp.Semantics.ReadExactCountGivenUpKeepsWhatItTook (#7177).
	s.ReadExactCountGivenUpKeepsWhatItTook = interp.No
	s.ReadTimeoutKeepsWhatArrived = interp.Yes
	s.ReadTimeoutBoundsReadability = interp.No
	// A trapped signal that lands while a `read` waits for input stops it to
	// run every handler waiting, and the read goes on; one that lands while
	// bytes are there, or came before the read, waits for that or for the
	// read's end. Measured 2026-10-31 on 5.3.20 and 3.2.57; see
	// interp.ReadHandlerMoment (#7031).
	s.ReadHandlersRun = interp.ReadHandlersRunWhenAWaitIsInterrupted
	// A handler's `return` there ends the read with no name touched, and the
	// function returns the handler's status: `return 5` mid-read is `out st=5
	// l=[old]`. Measured 2026-10-31 on 5.3.20; 3.2.57 touches no name either
	// and returns 0 (#7222). See
	// interp.Semantics.ReadLeftByAHandlersReturnAssignsWhatArrived (#7176).
	s.ReadLeftByAHandlersReturnAssignsWhatArrived = interp.No
	// A signal another process sends while a command substitution runs has
	// its handler run after the command, as for any other: `echo Y
	// "$(/bin/kill -INT $$)" "$(echo E >&3)"` writes `E`, `Y` and then `T`.
	// Measured 2026-10-31 on 5.3.20 and 3.2.57; see
	// interp.Semantics.HandlersRunAsASubstitutionEnds (#7175).
	s.HandlersRunAsASubstitutionEnds = interp.No
	// A directory the PATH search walked past leaves no trace: with nothing
	// runnable anywhere, bash says the name was never found at all.
	s.DirectoryOnPathIsACandidate = interp.No
	// And the candidate kept is the first one that **existed**, whatever it
	// was — which is a third reading beside the two PathCandidateReport
	// already held, and a row `$d` alone on PATH cannot reach. Measured
	// 2026-09-18 with `$d` a directory named `zzcmd` and `$g` a
	// non-executable file of that name: `$d:$g` is `zzcmd: command not
	// found` at 127 here where every other column reaches the file and says
	// `Permission denied` at 126, and the `$g:$d` control is the file in
	// every column. So the directory is kept and suppresses the later row,
	// rather than being walked past (#3578).
	s.PathCandidateReported = interp.FirstExistingCandidate
	// And what a `command -p` search resolved goes into the command hash here,
	// where it does not in the other four: with `PATH=/nonexistent_zz`, a
	// `command -p ls` and a plain `ls` after it, the second is 0 in this
	// column and 127 in zsh, dash, ksh93 and BusyBox ash. So a later bare name
	// runs a program the script's own PATH cannot reach, with nothing in the
	// script saying so. See interp.Semantics.DefaultPathSearchIsRemembered
	// (#2975).
	s.DefaultPathSearchIsRemembered = interp.Yes
	// set -E and -T carry the traps set -Eeuo pipefail scripts rely on.
	s.SetHasTheErrtraceLetter = interp.Yes
	s.SetHasTheFunctraceLetter = interp.Yes
	// `set -t`, which this shell also spells `set -o onecmd`: the line that
	// set it finishes and nothing more is read. Measured on a script file, on
	// standard input and at the invocation — `bash -t script.sh` runs the
	// first line of it and stops.
	s.SetHasTheTLetter = interp.Yes
	// `set -r`: restricted mode, and the one option in this shell that can
	// never be turned back off. Measured 2026-09-22 on bash 5.3.20 — see
	// Semantics.SetHasTheRestrictedLetter for what each of the nine
	// refusals was measured doing, and interp/restricted.go for the sites.
	s.SetHasTheRestrictedLetter = interp.Yes
	// And the mode locks behind you: `set +r` in a restricted shell is
	// `set: +r: invalid option` at 1 with the usage block, measured — where
	// ksh93 grants it and hands everything back. See
	// Semantics.RestrictedModeIsLeftByTheLetter.
	s.RestrictedModeIsLeftByTheLetter = interp.No
	// And the names the mode freezes really are readonly names here: the
	// refusal is `PATH: readonly variable`, `unset PATH` is `cannot unset:
	// readonly variable`, and `readonly -p` lists them — `declare -r ENV` in a
	// shell with no `$ENV` set. ksh93 answers no to all three.
	s.RestrictedFreezeIsAReadonly = interp.Yes
	// A writing redirection to a process substitution is refused like any
	// other: `echo a > >(cat)` is `>(cat): restricted: cannot redirect
	// output` at 1, measured 2026-10-02 (#5485).
	s.RestrictedRedirectTakesAProcessSubstitution = interp.No
	// unanswered RestrictedFreezeRefusesAValuelessDeclaration: the freeze is a
	// readonly here, so the readonly machinery refuses `local PATH` itself
	// (`local: PATH: readonly variable`, the function going on) and takes
	// `export PATH` at 0; the question is never asked.
	// unanswered RestrictedRefusalEndsWithTheBuiltinsStatus: the same reason;
	// no refusal of a frozen name ends a bash script.
	// unanswered RestrictedFreezeRefusesALetterThatChangesTheName: the same
	// reason; the readonly machinery answers every letter.
	// And none of the ten refusals ends the script: every one of them reports
	// 1 — or an assignment's 0 — and the line after it runs. Measured with
	// `echo tail` behind each.
	s.RestrictedBuiltinRefusalIsFatal = interp.No
	// `. ./x` is refused, and `command -p` carries on like the rest. See
	// Semantics.RestrictedDotRefusesAPath, measured 2026-10-02.
	s.RestrictedDotRefusesAPath = interp.Yes
	s.RestrictedCommandOptionRefusalIsFatal = interp.No
	// And the one route it does not reach. Measured: a `-c` string that turns
	// the option on runs to its end anyway, with `$-` showing `t` throughout.
	// The other shell with the letter stops there, which is what makes this
	// an axis rather than a rule.
	s.OneCommandStopsACommandString = interp.No
	// unanswered CommandTrackingStartsOn: this shell's startup letters spell
	// it, and a second declaration beside them is the disagreement #1951 was
	// — `$-` said `h` while `set -o` said `hashall off` in a shell that had
	// run nothing. `hB` is what bash 5.3.15 reports at startup and the `h` is
	// the claim, so Runner.commandTracking reads the letters here and never
	// reaches this axis. Measured 2026-09-25 on bash 5.3.20: a script's `$-`
	// is `hB` and `set -o` writes `hashall` on.
	s.JobControlAbsenceIsReportedFirst = interp.Yes
	// And the monitor alone is what `fg` and `bg` need: `set -m` in a script
	// with no terminal is granted here, and `fg` then runs the job.
	// Measured 2026-09-15 with and without a pseudo-terminal, and the answer
	// is the same both times (#2720).
	s.MonitorAloneResumesAJob = interp.Yes
	// The start notice is the other half of the same seam, and it answers
	// the other way: with the monitor on and nobody at a prompt, `sleep 1 &`
	// says nothing here. Measured 2026-09-15 on a pseudo-terminal, a script
	// file — the one route the shell that does announce can be asked on
	// (#2838).
	s.MonitorAloneAnnouncesAJob = interp.No
	// And it accounts for nothing on the way out either. Measured 2026-09-25
	// on a pseudo-terminal, `bash -m` over a script holding `sleep 30 &`:
	// nothing is written and the job runs on, in 5.3.20 and in 3.2.57 alike.
	// `-i` in place of `-m` is the same silence, and so is `-il`, which is
	// the pair `huponexit` needs at a prompt — so neither noun reaches this
	// shell without a prompt (#4542).
	s.MonitorAloneAccountsForJobsAtExit = interp.No
	// How a PATH hit is written back, before the command runs and after:
	// measured 2026-10-05 with `command -v`, `PATH=/bin/` is `/bin/ls` and
	// `PATH=`, `.` and `./` are all `./zz` — one separator, and an empty
	// entry as `.` (#6044). See Semantics.PathHitSpelled.
	// And what the hit is started by: measured 2026-10-05, `PATH=.` and an empty entry start `./z0`
	// (#6090). See Semantics.PathHitFromTheCurrentDirectoryRunsBare.
	// A remembered relative path written under `./` where the table answers,
	// and a table that holds only absolute entries: measured 2026-10-05
	// (#6069). See Semantics.HashedRelativePathReportedUnderDot and
	// Semantics.CommandTableHoldsOnlyAbsoluteEntries.
	// Whether `hash name` searches PATH again for a name already remembered
	// (#6110). See Semantics.HashNameSearchesAgain.
	// What waives a held exit for the exit after it (#6080). See
	// Semantics.HeldExitWaivedForTheNextCommandOnly.
	s.HeldExitWaivedForTheNextCommandOnly = interp.Yes
	s.HashNameSearchesAgain = interp.Yes
	s.HashedRelativePathReportedUnderDot = interp.Yes
	s.CommandTableHoldsOnlyAbsoluteEntries = interp.No
	s.PathHitFromTheCurrentDirectoryRunsBare = interp.No
	s.PathHitSpelled = interp.PathHitJoinedOnce
	// And a prompt is enough without the monitor: measured 2026-10-05
	// through a pseudo-terminal, `shopt -s checkjobs; set +m`, `sleep 3 &`,
	// `exit` is `There are running jobs.` and the shell stays, and with
	// `huponexit` at a login prompt the job is hung up, both as with the
	// monitor on (#6028).
	s.MonitorOffSilencesJobsAtExit = interp.No
	// A stopped job holds the exit back: the shell says so and stays,
	// and the next attempt leaves. Measured through a pseudo-terminal for
	// `exit` and for ^D alike.
	s.StoppedJobsHoldTheExit = interp.Yes
	// And a `wait` for one gives up rather than waiting on a process that is
	// not going to finish. Measured 2026-09-12 under `set -m`: a bare `wait`
	// warns and reports 0, and a `wait` naming the job or its process id
	// reports 145 — 128 plus SIGSTOP. See the axis for what the rest of the
	// panel does, and for why the monitor is the condition (#2227).
	s.WaitGivesUpOnAStoppedJob = interp.Yes
	// And a `wait` that reaps a job leaves the finished ones standing out of
	// the choice of `+` and `-`. Measured 2026-10-03; see the axis.
	s.AReapLeavesFinishedJobsUnmarked = interp.Yes
	// And `kill` reads a signal written onto its option with no space:
	// `kill -n9` and `kill -sKILL` both send. Measured 2026-09-12; bash 3.2
	// refuses both, which is why this is an axis and not the engine (#2227).
	s.KillReadsASignalJoinedToItsOption = interp.Yes
	// And writes the job table under the sentence while `checkjobs` is on —
	// measured, `[1]+  Stopped ./ticker` and `[2]-  Running sleep 40 &`
	// below `There are stopped jobs.`, and nothing below it with the option
	// off.
	s.HeldExitListsTheJobs = interp.Yes
	// `shopt -s huponexit` needs a login shell as well as an interactive one
	// — measured 2026-09-23, `-i` alone leaves the job running — and the
	// stopped half of the same question cannot be measured on this machine
	// at all, so No is what this implementation already did rather than a
	// reading. The signal goes after the EXIT trap, which is measured: the
	// trap's line arrives before the job's handler sees the hangup. All
	// three rows are on their axes.
	s.HangupAtExitNeedsALoginShell = interp.Yes
	s.HangupAtExitSkipsStoppedJobs = interp.No
	// A job started with the monitor off is hung up like any other:
	// measured 2026-10-05 at a login prompt with `huponexit`, `set +m`
	// before the job, and `exit` is `Hangup` (#6035).
	s.HangupAtExitSkipsJobsStartedWithoutTheMonitor = interp.No
	s.HangupAtExitPrecedesTheExitTrap = interp.No
	// `autocd` says what it did before doing it: with the option on, a bare
	// `subdir` writes `cd -- subdir` and then moves. Measured 2026-09-08
	// through a pseudo-terminal against bash 5.3.15, which is the only route
	// the name works on at all — zsh, the other shell with the option, moves
	// in silence.
	s.AutoCdAnnouncesTheSubstitution = interp.Yes
	s.TildePlusMinusExpands = interp.Yes
	// And a colon closes a tilde prefix in an ordinary word, not only in an
	// assignment's value — `echo ~:x` is the home and a colon — but only where
	// the whole word past the tilde is written plainly: `echo ~:x"y"` keeps
	// the tilde, and `echo ~/m:"x"` does not, because there the prefix ended
	// at the slash.
	s.TildeColonEndsAnOrdinaryWordsPrefix = interp.TildeColonEndsAPrefixInAPlainWord
	// And a *numbered* tilde reads the directory stack: `~1` is the entry
	// below the current directory and `~-1` the one above the bottom. The
	// parameter is this dialect's own view over the prelude's storage — see
	// dialect/bash/shellparameters.go — so naming it here is what lets a
	// tilde reach state the interpreter does not keep.
	s.DirectoryStackParameter = "DIRSTACK"
	s.UnderscoreTracksTheLastArgument = interp.Yes
	// And there is a parameter to move: `${_+x}` is non-empty before a
	// command has run, which the preset denies because POSIX names no such
	// parameter and dash and BusyBox ash have not got one.
	s.UnderscoreIsAParameterAtAll = interp.Yes
	// And every simple command moves it, wherever that command stands —
	// inside a loop, inside a function body, behind a `;` on one line. The
	// narrowed reading is ksh93's alone.
	s.UnderscoreMovesOnlyBetweenInputCommands = interp.No
	// A function body opens with what the *caller* had rather than with the
	// call's own last argument: `: outer` then `f one two` reads `outer` on
	// the first line of the body here and `two` in zsh. What the caller
	// reads once the call returns is `two` in both, and in ksh93, and is not
	// an axis.
	s.UnderscoreMovesBeforeAFunctionBody = interp.No
	// And it holds that argument across an `eval` or a `.`, where the
	// text's own last command would otherwise write through. Measured
	// 2026-09-23: `: SEED; eval ': inner'` leaves `: inner` here.
	s.UnderscoreHoldsTheCallAcrossEvalAndSource = interp.Yes
	// A bare `return` in a trap action hands back the status the action
	// was entered at: measured 2026-09-23 with every panel shell entering
	// the handler at 4 and this one answering 4.
	s.TrapReturnStatus = interp.TrapReturnTakesTheStatusBeforeIt
	// Alone in the panel, bash writes `$_` before the first command runs,
	// and what it writes is argv[0]: the same binary reached through a
	// symlink named `sh` writes `sh`. An `_` the environment carried wins
	// over it, which is the default answer and is left alone here.
	s.UnderscoreStartsAtTheInvocation = interp.Yes
	// hash counts builtins and functions and announces its empty table.
	s.FatalErrorStatusIsOne = interp.Yes
	s.HeredocExpandsInTheCommandsProcess = interp.Yes
	// And a `( … )`'s body with it: `n=0; ( cat ) <<END` with `$(( n+=5 ))`
	// in it leaves `n` at 0 here, where the same body on a group, a
	// function, a builtin or a loop leaves 5. A second axis rather than a
	// reading of the line above because ksh93 answers the two rows the other
	// way round — see the field's own panel (#4700).
	s.HeredocBodyOnASubshellExpandsInTheSubshell = interp.Yes
	s.RedirectTargetExpandsInTheCommandsProcess = interp.Yes
	// And a subshell's target is expanded in the subshell too: `( : ) >
	// "${u:=made}"` leaves `u` unset here, and `( echo RAN ) > $(( 1/0 ))`
	// writes one line, is caught by `||` and leaves 1 behind with the rest
	// of its own line still running — where the same target on a group, a
	// function or a builtin takes the rest of the line with it (#4695).
	s.RedirectTargetOnASubshellExpandsInTheSubshell = interp.Yes
	// A *target* this shell expanded itself and could not is not a failed
	// redirection here, it is this shell's own failed expansion: `: < $((
	// 1/0 )); echo SAME` writes no `SAME` and `|| echo CAUGHT` does not
	// fire, where `: < /nonexistent/f` gives both. So it takes the line
	// FailedExpansionAbandonsTheLine gives any other failed expansion, and
	// `${q?word}` in a target ends the shell exactly as it does in a word.
	// Measured 2026-09-26 on bash 5.3.20 (#4689).
	s.RedirectTargetFailureIsTheRedirections = interp.No
	// A body this shell expanded itself and could not costs what any other
	// failed expansion costs it, which is the line: `: <<END; echo SAME`
	// with `$(( 1/0 ))` in the body writes no `SAME` and leaves 1 for the
	// next line, exactly as `echo $(( 1/0 )); echo SAME` does — and a file
	// that will not open writes `SAME` there, which is what says this is not
	// the redirection's failure. `${q?word}` in the same body ends the shell,
	// as it does in a word. Measured 2026-09-26 on 5.3.20 (#4684).
	s.HeredocBodyFailureIsTheRedirections = interp.No
	// A loop variable that is not a name is checked when the loop runs here,
	// and the loop fails while the script carries on: `for $n in a b` prints
	// the complaint, the loop reports 1, and `echo "st=$?"` after it runs
	// and the script exits 0. POSIX mode moves it — see SetPosixMode — which
	// is the whole of the bash-as-`sh` column and is a mode rather than a
	// build: bash 3.2 and 5.3 agree under their own names.
	s.ForNameWhenTheLoopRuns = interp.ForNameFailsTheLoop
	// And the same shape for a function definition's name: the definition
	// fails, reports 1 and the script carries on — `function _p_${w} { :; };
	// echo st=$?; echo after` prints the complaint, `st=1` and `after`, and
	// exits 0. POSIX mode moves it to the syntax-error status exactly as it
	// moves the loop's, which is the bash-as-`sh` column.
	s.FunctionNameWhenTheDefinitionRuns = interp.FuncNameFailsTheDefinition
	s.ArithNameValueRecurses = interp.Yes
	// And a fixed depth is what stops it: a chain of sixty distinct names
	// ending in a number is `expression recursion level exceeded` rather
	// than the number. Measured 2026-09-18 (#3416).
	s.ArithRecursionBound = interp.ArithRecursionBoundedByDepth
	// `typeset +A` over a name that is an array is refused by name, at status
	// 1, with the table left intact; over a scalar or a name that does not
	// exist it is silent at 0. See interp.Semantics.ArrayAttributeRemoval
	// (#4241).
	s.ArrayAttributeRemoval = interp.ArrayAttributeRemovalRefusedForAnArray
	// A table literal mixing `[key]=` heads with bare words lets the **first**
	// element choose the reading, and a bare one after a head is refused by
	// name. An empty key is refused in both shapes, at different cost. See
	// interp.Semantics.MixedTableLiteral and .EmptyKeyInATableLiteral (#4241).
	s.MixedTableLiteral = interp.MixedTableLiteralFollowsTheFirstElement
	s.EmptyKeyInATableLiteral = interp.EmptyKeyInATableLiteralRefused
	// And an unset name found that way is a zero like any other unset name:
	// `x=abc; $((x+1))` is 1 and the script runs on. Measured 2026-09-11 —
	// ksh93 is the panel's holdout, where it is a fatal `parameter not set`.
	s.ArithRecursedNameMustBeSet = interp.No
	s.ArithSubscriptNameMustBeSet = interp.No
	// With nounset *on*, though, every arithmetic read of a name is the
	// option's business: `set -u; : $((b))` is `b: unbound variable` here
	// where the two fields above leave it zero. And it is fatal wherever it
	// is written — this is an expansion failure, and under `set -u` those
	// end the shell — so `(( b ))` and `let "x=b"` stop the script too,
	// where the same two constructs failing for an ordinary reason do not
	// (#3574).
	s.ArithUnsetNameUnderNounsetIsRefused = interp.Yes
	s.ArithNounsetRefusalIsFatal = interp.Yes
	// bash has no floats, so `2**-1` has no integer answer and stops the
	// expression; the two shells with floats answer 0.5 instead.
	s.ArithNegativeExponentIsError = interp.Yes
	// An expression reading a missing key leaves the table as it was.
	s.ArithKeyReadCreatesTheElement = interp.No
	// See interp.Semantics.ArithStoreIntoAFrozenTableIsTaken (#6826).
	s.ArithStoreIntoAFrozenTableIsTaken = interp.No
	// An expression's store the freeze refuses still takes a declared-only
	// array out of the declared-only set, and no other writer's refusal
	// does: `declare -a a; readonly a; (( a[5]=5 ))` lists `declare -ar
	// a=()`. See interp.Semantics.ArithStoreRefusedByAFreezeSetsTheArray
	// (#6847).
	s.ArithStoreRefusedByAFreezeSetsTheArray = interp.Yes
	// A refused store under a table's key leaves the table as it was. See
	// interp.Semantics.RefusedTableStoreKeepsTheKey (#6847).
	s.RefusedTableStoreKeepsTheKey = interp.No
	// `${a[k]=x}` on a missing key assigns it and `${a[k]?m}` refuses it,
	// as on any unset parameter. See
	// interp.Semantics.AssignOrErrorOperatorDeclaresTheKey (#7187).
	s.AssignOrErrorOperatorDeclaresTheKey = interp.No
	s.FrozenContainerTakesAnElementLiteral = interp.No
	s.ReadArrayIntoAnElementRefillsTheArray = interp.No
	// See interp.Semantics.ArithSubscriptDollarSingleIsDecoded (#6773).
	s.ArithSubscriptDollarSingleIsDecoded = interp.Yes
	// unanswered FloatLetterChangeRereadsTheRendering: this shell spells
	// neither float letter, so no declaration of its own can change one and
	// there is no rendering for a re-read to be about. Measured 2026-09-26
	// (#4486).
	// unanswered CompoundArithAssignmentConvertsItsValue: the same absence of
	// floats. `declare -i n=1; echo $(( n += 0.5 ))` cannot be written here at
	// all — the numeral is refused while the expression is read — so there is
	// no float value for an integer attribute to convert or keep, and the two
	// readings have nothing to disagree about. Measured 2026-09-26 (#4606).
	// unanswered ArithFloatOverflowIsZero: the same absence of floats. A
	// numeral this shell cannot hold is refused while it is being read —
	// `$((1e400))` is `value too great for base` — so there is no value for
	// an overflow rule to decide, and the question never reaches the axis.
	// unanswered ChainedSubscriptReadsANestedValue: the grammar for a chained
	// subscript is not this shell's — `${a[1][2]}` is a bad substitution or
	// a pattern here — so there is no chain for a reading to be about.
	// Measured 2026-09-15 (#2830).
	s.IndirectionYieldsName = interp.No
	// And not the third answer either: the `!` here is the indirection it
	// looks like, never zsh's `(k)` subscript flag. See
	// interp.Semantics.IndirectionIsTheSubscriptFlag, whose grid is the one
	// shell that means that by it.
	s.IndirectionIsTheSubscriptFlag = interp.No
	// And an operator written after `${!name[@]}` puts the `!` back to being
	// that indirection: the listing is the bare form only. Measured
	// 2026-09-14 on a script file, under `-c` and on standard input alike —
	// `typeset -A w; w[k]=tgt; tgt=HELLO; echo "${!w[@]#H}"` answers `ELLO`,
	// and `a=(p q); echo "${!a[@]#x}"` says `p q: invalid variable name`,
	// which is the indirection failing on the text it was handed (#2821).
	s.OperatorAfterTheSubscriptListingIsBad = interp.No
	s.BraceExpansion = interp.Yes
	// And every name the braces make expands the word again: `i=0; echo
	// {x,y,w}$((i++))` is `x0 y1 w2` with `i` left at 3 in bash 5.3.20 and
	// 3.2.57 alike, and `echo {x,y}$(echo TICK >&2; echo z)` writes TICK
	// twice for the two names it comes to. zsh and ksh93 run the word's
	// expansions once and hand every name the same results (#4694).
	s.BraceFanExpandsEachNameOnItsOwn = interp.Yes
	// And what the braces produced comes back to the word as shell *text*,
	// which is this shell alone: `var=baz; varx=vx; echo $var{x,y}` is
	// `vx vy` where ksh93 and zsh answer `bazx bazy`, and the backslash
	// `{Z..a}` counts reaches quote removal like any other unquoted one.
	s.BraceOutputRereadAsText = interp.Yes
	// A group that does not expand does not end the word, and the scan
	// resumes one byte past its open brace rather than past its close, so a
	// list nested inside it is still found: `@{x}{a,b}@` is `@{x}a@ @{x}b@`,
	// `{a{b,c}}` is `{ab} {ac}`, and the unclosed `{a{b,c}` is `{ab} {ac`.
	s.BraceRescanEntersFailedGroup = interp.Yes
	// The braces are found in the word the parse cut, so a comma a
	// parameter holds is an ordinary character: `e=a,b; echo {$e}` is the
	// one word `{a,b}` and `echo {x,$e}` is `x a,b`.
	s.BraceBodyReadAfterExpansion = interp.No
	// An alternative that came to nothing is removed with the word it left
	// empty: `f {a,}` is the one field `[a]`, `f {,}` is no field at all,
	// and an alternative an expansion emptied goes the same way — `unset u;
	// f {a,$u}` is `[a]`. ksh93 and zsh keep it. Measured on 5.3.20 and
	// 3.2.57 alike, which agree (#4800).
	s.BraceEmptyAlternativeIsAField = interp.No
	// A brace does not end field splitting here either: `IFS=:; v=a:b;
	// f x{p,q}$v` is `[xpa] [b] [xqa] [b]` on 5.3.20, the word expanded
	// once per name and split each time.
	s.BraceStopsFieldSplitting = interp.No
	// And it takes nothing from a pattern the expansion produced behind it:
	// with `za`, `zb` and `zq` in the directory, `g='*'; f {z,y}$g` is `[za]
	// [zb] [zq] [y*]` here where ksh93u+ answers `[z*] [y*]`, and `g='[ab]'`
	// goes the same way (#4843).
	s.BraceMakesAProducedStarOrBracketText = interp.No
	// And it frees nothing either: bash reads group syntax out of a value
	// everywhere, brace or no brace, which is
	// ExpansionResultSuppliesGroupSyntax above. With `za` and `ya` in the
	// directory, `g='?(a)'; f {z,y}$g` is `[z?(a)] [y?(a)]` on 5.3.20 —
	// `extglob` is off by default, and turning it on makes both rows live
	// rather than one of them (#4848).
	s.BraceFreesProducedGroupSyntax = interp.No
	// A produced brace is data here too, which is the same fact as the
	// braces being found in the word the parse cut: `e='{a,b}'; echo $e`
	// is `{a,b}` on 5.3.20 and 3.2.57 alike.
	s.BraceScanReadsProducedText = interp.No
	// `{01..3}` is `01 02 03`; `{10..1..3}` is `10 7 4 1` and `{1..10..-3}`
	// climbs anyway — the endpoints decide the direction and a step
	// contributes magnitude alone, so `{3..1..-1}` stays `3 2 1`.
	s.BraceRangePadsToEndpointWidth = interp.Yes
	s.BraceRangeStepPadsTheRange = interp.No
	s.BraceBodyIsACharacterClass = interp.No
	s.ZeroSubscriptIsTheFirstElement = interp.No
	// What a backslash in a substring range leaves; see the axis.
	s.RangeOperandBackslash = interp.RangeBackslashDoubleQuoteRules
	// bash 5.3.20 and 3.2 bring the name back plain after `unset`, the shell's own or not; see the axis.
	s.UnsetKeepsTheShellsOwnAttributes = interp.No
	// bash 5.3 and 3.2 keep the edge an expansion leaves at the end of a substituted word; see the axis.
	s.SubstitutedWordKeepsAnExpansionsEdges = interp.Yes
	// bash 5.3 execs the last command of a `-c` string only where it is a plain command at the top; see interp.TailExec.
	s.TailExec = interp.TailExecPlainTopLevel
	s.BraceRangeStepSignHonored = interp.No
	s.BraceRangeNegativeStepReverses = interp.No
	// Braces finish before parameters begin, so a range cannot be built
	// from one: `n=3; echo {1..$n}` is the literal `{1..3}`.
	s.BraceRangeEndpointsExpanded = interp.No
	// A character range is letters only — `{1..x}` is the word as written —
	// and it takes a step: `{a..z..2}` is `a c e …`. A step of zero counts as
	// one rather than never arriving, so `{1..2..0}` is `1 2`, and a body
	// that is range-shaped with a gap in it is the word as written.
	s.BraceCharRangeSpansAnyCharacter = interp.No
	s.BraceLetterRangeMayCrossCase = interp.Yes
	s.BraceRangeMissingEndCountsFromZero = interp.No
	s.BraceRangeZeroStepCountsAsOne = interp.Yes
	s.BraceRangeNumberMayCarryAPlus = interp.Yes
	s.BraceRangeThatCannotBeCounted = interp.BraceRangeFailureKeepsTheWord
	s.BracketCaretNegates = interp.Yes
	// One character-class name beyond the twelve POSIX ones. Measured
	// 2026-09-10, `[[ $c = [[:ascii:]] ]]` a character at a time: `a` is in
	// it, `é` and `日` are not, and bash 3.2 answers the same. ksh93 and dash
	// have no such name and match nothing with it, silently, which is what
	// this shell does with any other name outside the roster.
	s.PatternClasses = "ascii"
	// A negative *length* is refused for a list where it is accepted for a
	// string: `${a[@]:1:-1}` and `${@:1:-1}` are `-1: substring expression
	// < 0` at status 1, while `${v:1:-1}` of `abcdef` is `bcde`. The
	// subject decides, not the sign (#1735). bash 3.2 refuses the list form
	// the same way and the string form as well, which is a version
	// difference rather than a dialect's.
	s.ListSliceNegativeLengthIsAnError = interp.Yes
	// And the *string* spelling's own bound, which that line is deliberately
	// not: a negative length is accepted there and counts from the end until
	// the end it computes falls behind the offset. `v=abcdef; ${v:1:-9}` is
	// `-9: substring expression < 0` at 1 with the line given up, where
	// `${v:1:-2}` is `bcd` at 0 and `${v:1:-5}` — the end exactly at the
	// start — is empty at 0. The length **as written** is what is blamed,
	// which is the same verb the list sentence takes. Measured 2026-09-27
	// against 5.3.20, and 3.2.57 says it in the same words.
	s.SubstringEndBehindTheStart = interp.SubstringEndBehindStartIsRefused
	// A substring of a name with no value is empty before its offset and
	// length are read, so `unset u; "${u:1/0}"` is empty at 0 and `${u::=A}`
	// reaches no arithmetic. See Semantics.SubstringOfAnUnsetNameEvaluatesNothing.
	s.SubstringOfAnUnsetNameEvaluatesNothing = interp.Yes
	// `*//` lists `cx/` and `*//ax` lists `cx/ax`: a run of slashes behind
	// the first pattern component comes back as one, where every other
	// column writes the run back. See Semantics.SlashRunBehindAPatternIsOneSlash.
	s.SlashRunBehindAPatternIsOneSlash = interp.Yes
	s.RegexQuotingMakesLiteral = interp.Yes
	// An empty right operand is refused rather than matched: `[[ abc =~ "" ]]`
	// names an empty subexpression and exits 2, where Go's engine would
	// compile it and match at every position. bash 3.2 refuses it too, with
	// the same status and no diagnostic.
	s.EmptyRegexOperandIsAnError = interp.Yes
	// A failed `=~` empties the record rather than leaving the match before
	// last in it, and a group that took no part keeps its number as an empty
	// element — `[[ abcd =~ (b)(z)?(c) ]]` is four elements here (#2916).
	// Both measured 2026-09-18.
	s.RegexMatchSurvivesAFailedMatch = interp.No
	s.RegexMatchOmitsGroupsThatDidNotMatch = interp.No
	// A pattern match writes nothing there. Measured 2026-09-19 on 5.3.20:
	// `[[ abcd =~ (b)(c) ]]` fills BASH_REMATCH and `[[ abcd == a*d ]]`,
	// `case`, a glob and `${v#he}` all leave it exactly as that left it.
	s.PatternMatchWritesTheMatchRecord = interp.No
	// A process substitution may stand as a condition's operand here, and is
	// performed there: `[[ $v == <(cmd) ]]` runs cmd and matches against the
	// path, which is false for anything a script would have written down.
	// This shell alone — zsh refuses the word and ksh93 will not read it.
	s.ProcessSubstitutionInCondition = interp.Yes
	// And a substitution's body reads the input of the command it stands in,
	// which inside a pipeline element is that element's pipe: `printf
	// "PIPE\n" | cat <(cat)` with the shell reading a file of OUTER prints
	// PIPE here, in bash 3.2, and under `sh`. zsh is the one that parts.
	s.ProcessSubstitutionBodyReadsTheShellsInput = interp.No
	// bash diagnoses an unreadable condition operand and carries on: the
	// condition is false at status 1 and the next command runs. zsh and
	// ksh93 abandon the input there.
	s.ConditionArithmeticErrorIsFatal = interp.No
	s.ShiftPastEndFatal = interp.No
	// A declaration will not shadow a frozen name here: `typeset -r x=1;
	// f() { local x=2; echo $x; }` says `local: x: readonly variable`, then
	// prints the *outer* 1, and the function runs on — the builtin reports
	// 1 and nothing is abandoned. zsh takes the shadow instead. Measured
	// over `local x=2`, `local x`, `typeset x=3`, `local -r x=4` and
	// `local y=1 x=5 z=2`, all five refused here and all five taken there,
	// which is what makes it one field rather than five.
	s.DeclarationMayShadowAReadonly = interp.No
	// Unless the freeze is a *calling* function's own local, which this
	// shell lets a deeper declaration shadow: measured 2026-09-21,
	// `outer() { local -r x=O; inner; }` with `inner() { local x=I; }`
	// leaves `inner` holding `I` and `outer` still holding `O`. A freeze
	// this scope put there, and a freeze at the top level, are both still
	// refused — see the axis for the two controls.
	s.DeclarationMayShadowAnEnclosingScopesReadonly = interp.Yes
	// And the attribute cannot be taken off again: `typeset +r x` on a
	// frozen name is refused with the builtin named, reports 1 and leaves
	// the freeze standing. True of bash 3.2 and of the same binary under
	// argv[0] of `sh` alike, and of a name the shell was *started* frozen
	// with — `typeset +r UID` draws the identical refusal.
	s.ReadonlyAttributeCanBeRemoved = interp.No
	s.DeclaredNameWithoutValueIsEmpty = interp.No
	// And the name is still *recorded*: `typeset xyz; typeset -p xyz` writes
	// `declare -- xyz` at 0 while `${xyz-unset}` fires its default, so the
	// name is declared and unset at once. Measured 2026-09-15 under `env -i
	// PATH=/usr/bin:/bin` on bash 5.3.20 and on the same binary as `sh`,
	// through `-c` and from a file. ksh93 is the column that parts here,
	// which is what makes it a question of its own (#2999).
	s.ValuelessDeclarationRecordsTheName = interp.Yes
	// And the `unset` route into the same state writes the same row:
	// `v=g; f(){ local v; unset v; declare -p v; }; f` is `declare -- v` at
	// 0, with `${v-UNSET}` firing its default inside and the global back
	// afterwards. Measured 2026-09-27 under `env -i PATH=/usr/bin:/bin` on
	// bash 5.3.20, from a file. One answer for both routes here, which is
	// why this was one axis until zsh parted them (#4787).
	s.UnsetOfALocalRecordsTheName = interp.Yes
	// Never reached: the record is a row here, so a `-p` naming it is a
	// listing rather than a missing name. Answered so that no route can
	// refuse it, and answered `No` because it is the row that speaks.
	s.ValuelessRecordIsStillAName = interp.No
	// And the shell's own names are reported missing exactly as a script's
	// are: `unset RANDOM; typeset -p RANDOM` is `typeset: RANDOM: not found`
	// at 1, the same sentence and the same status as `w=1; unset w; typeset
	// -p w`. Measured 2026-09-27 on bash 5.3.20 and bash 3.2.57 alike, `env
	// -i PATH=/usr/bin:/bin` with a scratch HOME, from a file. This is the
	// column zsh parts from (#4896).
	s.RemovedShellOwnParameterIsStillAName = interp.No
	// A table the letters merely declared is not among the names a prefix
	// listing comes to: `declare -A q1; echo "[${!q@}]"` is `[]`, and the
	// same declaration with `=()` behind it is `[q1]`. Measured 2026-09-16 on
	// bash 5.3.20 under `env -i PATH=/usr/bin:/bin LC_ALL=C`, from a file.
	// bash 3.2.57 is the column that parts here — it lists a valueless
	// `declare -a` — so the answer is this build's rather than the family's.
	s.PrefixListingNamesADeclaredOnlyCompound = interp.No
	// An array that exists and holds no elements is unset to the `-`/`+`
	// test: `e=(); echo "[${e[@]+S}]"` is `[]` and `${e[@]-D}` is `[D]`.
	// Stated rather than inherited from the standard's value, because the
	// standard has no arrays and this is a measurement: bash 5.3.20 and
	// bash 3.2.57 both, 2026-09-16, from a file. zsh is the column that
	// parts (#2298).
	s.EmptyArrayIsSet = interp.No
	// And `[[ -v a[@] ]]`, which is not that question asked through another
	// operator: measured 2026-09-18, `${n[@]+S}` is `S` for a one-key table
	// here and `[[ -v n[@] ]]` is false on the same line. The subscript names
	// the array itself — `f=(x)` is set through it, `e=()` is not, following
	// EmptyArrayIsSet above — and a table falls through to the key lookup,
	// which `n[@]=x` then answers yes (#3436).
	s.ConditionWholeArraySubscript = interp.ConditionWholeArraySubscriptNamesTheParameter
	// The **count** of a whole array asks a third question again, and it is
	// the widest of the three: under `set -u`, `${#a[@]}` is refused unless
	// the name holds a list. A scalar is refused — `x=abc; ${#x[@]}` is
	// `x: unbound variable` while `${x[@]}` on the same line is `abc` — and
	// so is a name that was never mentioned; `a=(); ${#a[@]}` is `0`, which
	// is the row that says an assigned-and-empty array is not what this is
	// about. zsh 5.9.2 draws the line one name over (a scalar counts there)
	// and ksh93u+ refuses nothing.
	//
	// And the refusal **gives up the line rather than the shell**, which is
	// the opposite of what the same option does here one construct over:
	// `echo "c=${#a[@]}"; echo SAME` writes the sentence, never runs `SAME`,
	// leaves 1 behind and runs the next line, where `${#a}` on the same
	// unset name ends the shell. Measured 2026-09-18 on bash 5.3.20 and bash
	// 3.2.57 alike, under `env -i HOME=… PATH=/usr/bin:/bin LC_ALL=C`, from a
	// script file with `echo REACHED` on the line after (#3125).
	s.WholeArrayCount = interp.WholeArrayCountRefusesANameHoldingNoArray
	s.WholeArrayCountRefusalAbandonsTheLine = interp.Yes
	// The export letter says nothing about scope here: `declare -x v=1`
	// inside a function is an ordinary local.
	s.ExportLetterDeclaresAGlobal = interp.No
	// The two **container** letters and nothing else: measured 2026-09-21,
	// `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME, from a
	// script file, on bash 5.3.20 — `export -a ea=(asdf fdsa)` is 0 and
	// lists as `declare -ax ea=([0]="asdf" [1]="fdsa")`, `export -A
	// m=([k]=v)` is 0 and lists as `declare -Ax m=([k]="v" )`, and `-i`,
	// `-r` and `-l` are each `export: -X: invalid option` with the usage
	// line at 2. bash 3.2.57 takes `-a` and has no `-A` to take.
	//
	// So this word is not reading every declaration letter here: what it
	// takes is the pair that names a *container*, which is what its own
	// listing writes back for such a name — `declare -ax`. See
	// Semantics.ExportOptions (#4089).
	//
	// One neighboring row measured in the same run is left where it is,
	// narrower than the form the issue is about: `export -a` with **no
	// operands at all** is a listing filtered to the arrays, where this
	// shell still refuses the letter. The filtered form is
	// DeclarationListingFilter's question and `export` does not reach it.
	//
	// The valueless row that stood beside it — `export -a b` recording no
	// container letter there — is answered below.
	s.ExportOptions = "aA"
	// And the letter takes effect only where the operand carries a value: a
	// valueless `export -A n` records no container, converts none and refuses
	// none. That closes the two rows the paragraph above left standing and the
	// conversion refusal beside them. See
	// interp.Semantics.ExportContainerLetterNeedsAValue (#4089, #4179).
	s.ExportContainerLetterNeedsAValue = interp.Yes
	// And a valueless declaration of a standing name is silent.
	s.ValuelessDeclarationOfAHeldNameListsIt = interp.No
	// A plain word declared over a name holding an array replaces it and
	// says nothing.
	s.ScalarOverACompoundIsAnInconsistentType = interp.No
	// An array letter given a plain word: see
	// interp.Semantics.ArrayLetterWithAWordDeclaresAScalar (#5630).
	s.ArrayLetterWithAWordDeclaresAScalar = interp.No
	// A plus-only line's value, and the letters it takes off: see
	// interp.Semantics.PlusLetterComesOffAfterTheValueLands (#5663).
	s.PlusLetterComesOffAfterTheValueLands = interp.No
	// A `-p` listing whose first option word is plus-signed: see
	// interp.Semantics.PlusSignedPrintListsNoValues (#5642).
	s.PlusSignedPrintListsNoValues = interp.No
	// `readonly -a a` freezes the name and records no kind: it lists as
	// `declare -r a`, with no `a` in the cluster.
	s.ReadonlyRecordsTheCompoundAttribute = interp.No
	// And an exported name with no value reaches no child, whatever letters
	// its declaration wrote: `declare -ix Z; env` hands over nothing.
	s.NumericTypeWithNoValueReachesAChildAsZero = interp.No
	// A type letter and an array literal on one declaration is an array of
	// that type: `declare -ia z=(1 2)` is `declare -ai z=([0]="1" [1]="2")`,
	// and `declare -i z=(1 2)` with no array letter is the same line.
	s.TypeLetterAndAnArrayLiteralIsAnInconsistentType = interp.No
	// A container letter and a numeric one stand together too: `declare -ai
	// z` is `declare -ai z`, an array of integers.
	s.NumericAttributeReplacesTheArrayAttribute = interp.No
	// And an array literal over a name that is not an array types its
	// elements rather than re-creating the name, assigned or appended alike:
	// `declare -i a; a=(5+5 6+6)` is `declare -ai a=([0]="10" [1]="12")`.
	s.ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver = interp.No
	s.AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver = interp.No
	// The letters that say what a name's values are stand together here in
	// both orders: `declare -l z; declare -i z` and the reverse are both
	// `declare -il z="1"`.
	s.NumericAttributeReplacesTheCaseAttribute = interp.No
	// And `-u` beside the same letter records exactly as `-l` does. Measured
	// 2026-09-12, `typeset -ui v=4` lists `declare -iu v="4"` (#2541).
	s.UpperCaseLetterBesideANumericTypeLetterRecordsNothing = interp.No
	// Both case letters on one declaration cancel, and take off a standing
	// one with them. Measured 2026-09-12, `typeset -lu z=Ab` lists `declare
	// -- z="Ab"` with the value unfolded, and `typeset -l z=Ab; typeset -lu
	// z=Cd` lists `declare -- z="Cd"` (#2541).
	s.TwoCaseLettersOnOneDeclarationCancel = interp.Yes
	// The letter that lists is the letter that folds: see
	// interp.Semantics.CaseListingAndFoldAreSeparate (#5671).
	s.CaseListingAndFoldAreSeparate = interp.No
	s.CaseAttributeReplacesTheNumericAttribute = interp.No
	// One reader for both: `$((010))` and `typeset -i d=010` are eight
	// alike, where ksh93 answers eight and ten.
	s.IntegerAssignmentReadsALeadingZeroAsDecimal = interp.No
	// And one reader for a value too: `k=010; $((k))` is eight here.
	s.ArithStoredValueReadsALeadingZeroAsDecimal = interp.No
	// The same octal rule is why `010#5` is not base ten here: the zero opens
	// an octal constant, so the `#` behind it is a byte no numeral can hold
	// and the literal fails — `010#5: invalid number`, where `08#5` is
	// `value too great for base` because the `8` is a digit the base cannot
	// reach. Both readings fall out of this one answer.
	s.ArithBaseMayHaveALeadingZero = interp.No
	s.ArithBaseIsAtMostTwoDigits = interp.No
	s.ArithBaseZeroReadsTheDigitsAsWritten = interp.No
	// A radix prefix with nothing after it is a finished number worth zero:
	// `$(( 0x ))` is 0 and `$(( 0x+1 ))` is 1, in 5.3 and 3.2 alike.
	s.ArithEmptyRadixDigitsAreZero = interp.Yes
	// Refused, which is **5.3's** answer and not 3.2's: `$(( 8# ))` is
	// `invalid integer constant (error token is "8#")` in 5.3.20 on this
	// machine and in 5.3.15 in the pinned image — the build this preset
	// models — where bash 3.2.57 answers 0. This preset is 5.3, so the
	// corpus's `bash32` column is where the older reading lives, exactly as
	// it does for the other rows bash changed. See
	// Semantics.ArithEmptyBaseDigits.
	s.ArithEmptyBaseDigits = interp.ArithEmptyBaseDigitsRefused
	// A numeral past the word goes round it. `$(( 10000000000000000000 ))`
	// is -8446744073709551616 and `$(( 18446744073709551616 ))` is 0 — the
	// second being the row that says this is modular and not a clamp at the
	// top of the unsigned word. Measured identically in 5.3.20, 3.2.57 and
	// under the `sh` name (#3202).
	s.ArithNumeralPastTheWord = interp.NumeralPastTheWordWraps
	// And the same numeral out of a variable reads identically; only dash
	// parts the two.
	s.ArithStoredNumeralPastTheWordIsRefused = interp.No
	// And for `let`: `let "x=010"` is eight, the same as `(( ))`.
	s.LetReadsALeadingZeroAsDecimal = interp.No
	// An arithmetic assignment leaves an ordinary scalar: `(( x = 5 ));
	// declare -p x` is `declare -- x="5"`, and a later `x=2+3` is the three
	// characters.
	s.ArithmeticAssignmentDeclaresANumber = interp.No
	// An attribute added to a name that already holds a value waits for the
	// next assignment: `FOO=bar; typeset -i FOO` still reads `bar`, and
	// `d=MiXeD; typeset -u d` still reads `MiXeD`. ksh93 and zsh re-read on
	// the spot and store 0 and MIXED.
	s.AttributeRereadsTheValueItFinds = interp.No
	s.InheritedValueSurvivesADeclaredType = interp.Yes
	s.CompoundElementsGoThroughTheAttribute = interp.Yes
	// The case attributes fold once, on the value being stored: `declare -l
	// v=AB` holds `ab`, and taking the letter off leaves `ab` behind.
	s.CaseAttributeFoldsWhenRead = interp.No
	s.CompoundAttribute = interp.CompoundAttributeKeepsTheElements
	// The converse: an array or table letter given to a name already holding
	// a scalar promotes that value to the first element. Measured 2026-09-08,
	// `b=1; typeset -a b` lists as `declare -a b=([0]="1")` and `a=1;
	// typeset -A a` as `declare -A a=([0]="1" )`, and `$b` still reads `1`
	// under both. bash 3.2.57 answers the array letter the same way and has
	// no `-A` at all.
	// The kind letters accumulate here: `typeset -i q; typeset -a q` lists
	// `declare -ai q` and so does the pair the other way round, measured
	// 2026-09-27 on 5.3.20 and on 3.2.57, which has the same answer on the
	// rows it has. See interp.Semantics.KindLetterReplacesTheKind (#4881).
	s.KindLetterReplacesTheKind = interp.No
	s.ScalarUnderAnArrayDeclaration = interp.ScalarUnderACompoundBecomesTheFirstElement
	s.ScalarUnderATableDeclaration = interp.ScalarUnderACompoundBecomesTheFirstElement
	// A name holding an array or a table reaches no child at all: measured
	// 2026-09-12, `typeset -x a=(p q)` and `a=(p q); export a` both leave
	// nothing named `a` in a child's environment, and so does an exported
	// table. `export b=1` beside it is the control and does arrive, so it is
	// the compound and not the export (#1380).
	s.ExportedCompoundReachesAChildAsItsFirstValue = interp.No
	// And a subscripted operand's letters land on the name: `typeset -x
	// a[1]=v` lists `declare -ax a=([1]="v")` here (#1380).
	s.SubscriptedOperandCarriesTheAttributes = interp.Yes
	s.StoreRefusalOfADeclaredElementLeavesZero = interp.No
	// The refusal is not fatal here at all, so the builtin's own status is
	// what a script reads and there is no give-up whose number could move.
	s.StoreRefusalThroughPrintfLeavesZero = interp.No
	// No `set -A`, and a refused `read -a` is not fatal — `readonly v=1;
	// read -a v <<< x; echo after` writes `after` — so there is no give-up
	// whose number could move (#6486).
	s.ArrayStoreRefusalThroughSetOrReadLeavesZero = interp.No
	// `a=(1 2); a+=x` joins the first element and leaves the rest standing —
	// `declare -a a=([0]="1x" [1]="2")`, two elements, in 5.3.15, in the same
	// binary under argv[0] of `sh` and in 3.2.57. The value lands at the base
	// whether or not an element is there: `a=([5]=q); a+=x` is
	// `([0]="x" [5]="q")`.
	s.ScalarAppendedToAnArrayBecomesANewElement = interp.No
	// `a=(1 2 3); a=x` is `declare -a a=([0]="x" [1]="2" [2]="3")`: the
	// first element written, the rest standing, still an array of three.
	// The table spelling answers the same — `m=x` over `declare -A m` is
	// `([0]="x" [k]="v" )`. Both hold in 5.3.15, in the same binary under
	// argv[0] of `sh`, and in 3.2.57.
	s.ScalarAssignedOverACompoundReplacesTheName = interp.No
	// An array holding the scalar a plain store gave it: see
	// interp.Semantics.ScalarHeldUnderTheArrayLetterListsAsAScalar (#5643).
	s.ScalarHeldUnderTheArrayLetterListsAsAScalar = interp.No
	// Nothing refuses the store ahead of that: a table takes it. Measured
	// 2026-09-26 on 5.3.20, `declare -A h=([one]=1); h=string; declare -p h`
	// is `declare -A h=([0]="string" [one]="1" )` at 0 with the line after it
	// run, and `h+=string` joins at that same key. See
	// Semantics.ScalarStoredOverATableIsRefused.
	s.ScalarStoredOverATableIsRefused = interp.No
	s.ArrayLiteralAssignmentStartsTheNameOver = interp.No
	// `local u` hides the caller's `u` — the local exists unset.
	s.ValuelessDeclarationHidesTheOuterValue = interp.Yes
	// And under `set -a` that local is not marked, where `declare gd` at the
	// top level is: `local sa` lists `declare -- sa` and the top-level one
	// `declare -x gd`. Measured 2026-11-01 on 5.3.20; see the axis (#7283).
	s.AllexportMarksALocalDeclaredWithoutAValue = interp.No
	s.AHiddenExportStillReachesAChild = interp.Yes
	s.TypesetLocalNeedsKeywordFunction = interp.No
	s.ReadonlyReassignmentFatal = interp.No
	// `${1:=abc}` is refused rather than assigned: `$1: cannot assign in
	// this way`, in 5.3.15, in the same binary under argv[0] of `sh` and in
	// 3.2.57 alike. Only zsh assigns (#1541).
	s.AssignThroughExpansionMayNameAPositional = interp.No
	// An assignment prefix to a frozen name is reported and costs nothing
	// at all: measured 2026-09-11 in 5.3.15 and in 3.2.57, `readonly x=1;
	// x=2 /bin/echo RAN; echo after` prints the complaint, `RAN` and
	// `after` at status 0 — the command runs with the name still holding
	// its old value — and the same holds for a regular builtin, a special
	// one, a function and `command`. Uniform across every kind, which is
	// what separates this shell from the two that answer by kind (#1219).
	s.PrefixToARegularBuiltinIsRefused = interp.Yes
	s.PrefixRefusalFatality = interp.PrefixRefusalNeverFatal
	s.PrefixRefusalCostsTheCommand = interp.No
	// And POSIX mode moves both of those: measured 2026-09-18 from a script
	// file with `set -o posix; readonly v=1` in front of it, `v=3 true;
	// echo pre=$?` writes the complaint, never the `pre=`, and reports 1 on
	// the next line, while `v=3 :` ends the script at 1. See
	// Semantics.PosixModeSharpensAPrefixRefusal.
	s.PosixModeSharpensAPrefixRefusal = interp.Yes
	// A failed expansion gives up the line here and the shell carries on at
	// the next one, which is this shell alone among the four. Measured over
	// both routes and both separators — see the axis for the 2x2 — on a bad
	// substitution, a division by zero, a bad subscript and an arithmetic
	// expression the parser refused.
	s.FailedExpansionAbandonsTheLine = interp.Yes
	// And the command that named a `>(cmd)` does not wait for its body: the
	// body is a process holding this shell's standard output, so its bytes
	// land afterwards rather than before. Measured as an ordering — `printf
	// "PIPE\n" | tee >(read -r v; sleep 0.3; printf "[%s]" "$v") >/dev/null;
	// printf AFTER` is `AFTER[PIPE]` here and `[PIPE]AFTER` in zsh (#2197).
	s.WritingSubstitutionIsWaitedForAtTheCommand = interp.No
	// `<>` onto a process substitution this command made: the path is opened, and refused: `/dev/fd/63: Permission denied` (#5514).
	s.ReadWriteRedirectionTakesTheSubstitutionsEnd = interp.No
	// And the path it expands to is `/dev/fd/N` on both platforms, even
	// where /proc/self/fd is there and /dev/fd is a symlink to it: measured
	// 2026-09-21, `echo <(true)` is `/dev/fd/63` in the pinned Linux image
	// and on the panel machine alike, where zsh in that same image writes
	// the symlink's target instead (#3986).
	s.SubstitutionPathPrefersProcSelfFd = interp.No
	s.ReadonlyReassignmentByDeclarationFatal = interp.No
	// And `export x=2` and `readonly x=2` carry on too, until `set -o posix`
	// moves this one — see interp.Semantics.ReadonlyReassignmentBySpecialBuiltinFatal.
	s.ReadonlyReassignmentBySpecialBuiltinFatal = interp.No
	// A frozen name refuses a declaration's array literal here as much as it
	// refuses anything else: measured in 5.3 and in 3.2, `readonly q=1;
	// declare -g q=(b)` is `q: readonly variable` and the name is untouched,
	// where zsh retypes it. Both builds, so it is not a version's answer.
	s.ArrayLiteralOperandRetypesAFrozenScalar = interp.No
	// A declaration's value that came out as `( … )` is read again as an
	// array literal here, which is this column's answer alone among the
	// three that have the construct. Measured 2026-09-21 from script files
	// under `env -i PATH=/usr/bin:/bin LC_ALL=C`, `typeset -a a="(1 2)"`
	// then `echo "n=${#a[@]} zero=[${a[0]}]"` is `n=2 zero=[1]` in 5.3.20,
	// in 3.2.57 and under an argv[0] of `sh`, against `n=1 zero=[(1 2)]` on
	// ksh93u+. See the axis for the rest of the rows (#2298).
	s.DeclarationRereadsAParenthesizedValue = interp.Yes
	// And the letter half of the same rule is refused too. Measured
	// 2026-09-12 under `env -i`, `readonly q=1; typeset -i q=4` is
	// `typeset: q: readonly variable` in both builds and the name is left at
	// 1; `-F` is not even a float here — it is the function-listing letter,
	// and the complaint is about `-f` — and `export -i` is an invalid
	// option. So no spelling of the numeric type letter reaches a frozen
	// name (#2539).
	s.NumericTypeLetterRetypesAFrozenName = interp.No
	// And the wider rule the same probe found on its valueless half: a
	// declaration naming a value-shaping letter over a frozen name is
	// refused whole here, so nothing of it lands. Measured 2026-09-13 under
	// `env -i`, after a `readonly q=1`: `typeset -i q`, `typeset -u q` and
	// `typeset -a q` are each `typeset: q: readonly variable` at 1 with the
	// listing still `declare -r q="1"`, where `typeset -x q`, `typeset -t
	// q`, `typeset -r q` and a bare `typeset q` are all taken. **bash 3.2
	// is the other way on that half** and takes every one of them, which is
	// why this preset is bash 5.3's answer and the record carries 3.2's in
	// its own column (#2561).
	s.AttributeOverAFrozenNameIsRefused = interp.Yes
	// And the same with no value, which is the row the axis above was
	// measured on: `readonly c; typeset -i c` is `typeset: c: readonly
	// variable` at 1 in bash 5.3.20. Answered rather than left unanswered
	// because it was measured, though the axis above refuses first here and
	// this one is never reached (#3937).
	s.TypeLetterOverAFrozenNameWithNoValueIsRefused = interp.Yes
	// And a keyed letter over a frozen name holding a value, which bash
	// 5.3.20 refuses like every other attribute over a frozen name:
	// `c=1; readonly c; declare -A c` is `declare: c: readonly variable`
	// at 1. Never reached, the wide axis above refusing it first, and
	// answered rather than left silent for the same reason as the axis
	// before it (#3965).
	s.KeyedLetterOverAFrozenNameHoldingAValueIsRefused = interp.Yes
	s.BuiltinSyntaxErrorFatal = interp.No
	// An error inside a file `.` read ends the shell here, not just the file:
	// measured, a sourced file whose third line is `echo X${NOPE}` under
	// `set -u` prints nothing after it in the sourcing file either, and bash
	// exits 1. Same under argv[0] of `sh` and in bash 3.2.
	s.FatalErrorEndsBorrowedTextOnly = interp.No
	s.TraceFromBorrowedTextFollowsItsRedirection = interp.Yes
	// `${x?word}` is an error here rather than a request to stop, which shows
	// at the one boundary this shell does give up a file at: measured, a
	// `$BASH_ENV` whose second line is `echo X${NOPE?msg}` stops there and
	// the script the shell was started for still runs.
	s.ParamErrorIsAnExitRequest = interp.No
	s.DotMissingFileFatal = interp.No
	// And neither is a missing operand: `.` and `source` with none write
	// `filename argument required` and the builtin's usage, leave 2 behind,
	// and the script runs on. Measured 2026-09-18 on bash 5.3.20.
	s.DotWithNoOperandIsFatal = interp.No
	// POSIX mode moves it, and the invocation name is only a door into that
	// mode: measured 2026-09-19, `set -o posix` in front of the bare `.`
	// ends plain bash's script at 2 exactly as this binary called `sh` does,
	// and the control — `false` on the line before — runs on under both. So
	// the departure is the mode's and not argv[0]'s (#3818).
	s.DotWithNoOperandIsFatalInPosixMode = interp.Yes
	s.DotPassesArguments = interp.Yes
	// And the restore yields to the file's own `set`, which is this column
	// alone among those that pass the words. Measured 2026-09-21 on bash
	// 5.3.20 and bash 3.2.57, with a file holding `set -- m n o p`:
	// `set -- a b c; . ./g p q; echo "$@"` writes `m n o p` here and `a b c`
	// in zsh 5.9.2, ksh93u+ and BusyBox ash. `shift` in place of the `set` is
	// `a b c` in all four, so what moves is the replacement and not the
	// restore (#4063).
	s.DotSetCancelsTheRestore = interp.Yes
	// A directory operand is an error here and success in zsh and dash.
	// Measured, `. ./` is `bash: line 1: .: ./: is a directory` at 1, and
	// the script carries on — the same status and the same survival a file
	// that is not there gets, with a sentence of its own.
	s.DotDirectoryOperandIsAnError = interp.Yes
	// The only shell in the panel that looks in the current directory once
	// PATH has missed. Measured: `PATH=/usr/bin:/bin; . f.sh` finds an f.sh in
	// the current directory here, and is "not found" in the other three.
	s.DotFallsBackToCurrentDirectory = interp.Yes
	// And POSIX mode takes it away, which is the standard's own search:
	// measured 2026-09-21 on bash 5.3.20, `echo 'echo hi' > f; . f` reads
	// the file here and is `.: f: file not found` at 1 after `set -o
	// posix`, under `bash --posix` and under the `sh` name alike, and `set
	// +o posix` puts the reading back. The mode words the miss differently
	// too — the search sentence rather than the errno one — which follows
	// from the reading rather than being a second answer.
	s.DotFallsBackToCurrentDirectoryInPosixMode = interp.No
	// `. -p list file`, which is a 5.x addition: bash 3.2 calls the letter
	// an invalid option and prints a usage line without it.
	s.DotReadsOptions = interp.Yes
	s.DotTakesTheSearchPathOption = interp.Yes
	// `. -p list file`, which is a 5.x addition: bash 3.2 calls the letter
	// an invalid option and prints a usage line without it.
	s.DotReadsOptions = interp.Yes
	s.DotTakesTheSearchPathOption = interp.Yes
	// `eval` reads a leading dash-word the way every other builtin here
	// does: `eval -- cmd` runs `cmd`, and `eval -q cmd` is `eval: -q:
	// invalid option` with the usage line, at 2. There are no letters to
	// know, so the refusal is the whole of what the reading does.
	s.EvalOptions = interp.EvalReadsOptions
	// See interp.Semantics.BuiltinReadsOptions.
	s.BuiltinReadsOptions = interp.Yes
	// Only for the half where the PATH search came up with nothing. bash is
	// the one column that parts the two, and it is not a version move —
	// 5.3.20 and 3.2.57 answer alike. Measured 2026-09-21: `exec
	// nosuchcmd-xyz` runs the trap, and `exec ./nosuchcmd-xyz`, `exec
	// /nope/false`, a file without the execute bit, a directory, and a
	// non-executable file found *on PATH* all drop it (#3983).
	s.ExecFailureRunsExitTrap = interp.Yes
	s.ExecFailureOnAPathnameRunsExitTrap = interp.No
	s.ExecTakesOptions = interp.Yes
	// An `exec` of options alone runs nothing at 0 here, with its
	// redirection made: measured 2026-10-02 on bash 5.3.20 and 3.2.57,
	// `exec -c`, `exec -a x` and `exec --` are each `st=0`, and `exec -c >f`
	// makes `f`. See Semantics.ExecOptionsRequireACommand.
	s.ExecOptionsRequireACommand = interp.No
	// `builtin`'s words are matched before its name is looked up: measured
	// 2026-10-02 on bash 5.3.20 under failglob, `builtin nosuch nomatch*`
	// is `no match: nomatch*`. See Semantics.BuiltinNameIsLookedUpBeforeGlobbing.
	s.BuiltinNameIsLookedUpBeforeGlobbing = interp.No
	// A bare `trap` inside the running EXIT trap lists it (it does here,
	// measured 2026-10-02). See Semantics.ExitTrapListedWhileItRuns.
	s.ExitTrapListedWhileItRuns = interp.Yes
	// A trap's action is listed as it was written: measured 2026-10-02 on
	// bash 5.3.20, `trap 'echo E;   trap' USR1; trap` keeps the three blanks.
	// See Semantics.TrapActionListedFromItsParse.
	s.TrapActionListedFromItsParse = interp.No
	// Both letters, and `-l` reaches the name `-a` chose: `exec -l -a NAME`
	// hands the replacement `-NAME`, where zsh hands it `NAME`.
	s.ExecTakesTheLoginLetter = interp.Yes
	s.ExecTakesTheEmptyEnvironmentLetter = interp.Yes
	s.ExecLoginPrefixesTheGivenName = interp.Yes
	s.TestAcceptsDoubleEqual = interp.Yes
	// The operators past the three-word rules. `test -a f` is `-e`'s question
	// where two words say the letter is an operator rather than the
	// connective, `test -o errexit` asks whether a `set -o` name is on, and
	// `test -N f` asks whether the file was written since it was read.
	// `umask -p` prints the mask as a command that would set it again, and
	// `set -p` is the short spelling of `set -o privileged`.
	s.UmaskHasTheReusableLetter = interp.Yes
	s.SetHasThePrivilegedLetter = interp.Yes
	s.TestHasTheFileExistsLetter = interp.Yes
	// `-t`'s operand is not read as an arithmetic expression here: measured,
	// `x=0; [[ -t x++ ]]` leaves `x=0` and complains `[[: x++: integer
	// expected` at 2, where the one column that evaluates it leaves `x=1` and
	// says nothing. What this shell does is refuse anything that is not a
	// literal number — see TerminalTestRequiresANumber, which is that half
	// (#5058).
	s.TerminalTestOperandIsArithmetic = interp.No
	s.TestHasTheShellOptionOperator = interp.Yes
	s.TestHasTheModifiedSinceReadOperator = interp.Yes
	// And both ordering operators, which is the answer POSIX's XSI option
	// gives — where ksh93 has only the greater one.
	s.TestStringOrder = interp.TestStringOrderBoth
	// `f -nt missing` holds when f exists, in `test` and `[[ ]]` alike.
	s.MissingFileIsOlder = interp.Yes
	s.UmaskPrintsFourDigits = interp.Yes
	s.UmaskSetWithSPrints = interp.Yes
	s.PipefailOption = interp.Yes
	// A substituted element keeps the status its death produced, 128 plus
	// the signal, the same as anywhere else.
	s.PipefailSubstitutesTheBareSignal = interp.No
	s.ErrexitSeesPipefailFailure = interp.Yes
	// No implicit `return` for a failure, and no option that asks for one.
	// Measured 2026-09-25 on bash 5.3.20: `f() { false; echo x; }; f; echo
	// "post=$?"; echo done` writes `x`, `post=0` and `done` at 0; `set -o`
	// names `errexit` and `errtrace` and nothing else with `err` in it, and
	// `shopt` adds `gnu_errfmt` and `inherit_errexit`, neither of which is
	// this. The function-scoped half of `set -e` here is `set -E`, which
	// carries the ERR *trap* into a body rather than returning from one.
	s.FailureTakesAnImplicitReturn = interp.No
	// `time` changes nothing about the judging here: measured 2026-09-18,
	// `set -e; time false; echo survived` stops and `trap 'printf E' ERR;
	// time false` writes E, for a simple command, a pipeline, a group, a
	// function call and a loop alike.
	s.TimedCommandIsJudged = interp.Yes
	// And a redirection that cannot be opened on a compound command is a
	// failure both judges see, exactly as it is on a simple one.
	s.CompoundRedirectionFailureIsJudged = interp.Yes
	// A subshell written as the last element of a pipeline is judged twice:
	// once inside the element's own process, where the trap is still set, and
	// once for the pipeline. Measured 2026-09-18, `trap 'printf E' ERR; true
	// | ( exit 3 )` writes `EE` — and that element has no failing command in
	// it, so the second E is the subshell command itself and not its body. A
	// group holding the same subshell writes one E, a function whose body is
	// one writes one, and an element that is not the last does not fire at
	// all.
	s.ASubshellAsTheLastPipelineElementJudgesItself = interp.Yes
	// A pipeline waits for every element, not only its last: `{ sleep 0.3;
	// echo first >&2; } | :; echo after` writes `first` before `after`.
	// Measured 2026-11-01 on 5.3.20, 3.2.57 and 5.3 as `sh`; see
	// interp.Semantics.PipelineEndsWithItsLastElement (#7169).
	s.PipelineEndsWithItsLastElement = interp.No
	// `[ ( -n x ) ]` is 0: the group is read like any other (#3419).
	s.TestGroupedUnaryAloneLosesTheClosingParen = interp.No
	// A four-word `!` in front of a negation negates it, the way a
	// recursive reading predicts: `[ ! ! -n x ]` is 0 here and 1 in the one
	// column that drops the outer negation. Measured 2026-09-19 (#3700).
	s.TestFourWordsNegateANegationOnce = interp.No
	s.TestNegationBeforeAnOperatorIsAnOperand = interp.No
	// And at three words the connective is read before a leading `!`. The
	// probe that says so names a file that **exists**, because this shell
	// has a unary `-a`: `[ -a / ]` is 0, so negating it first would give 1,
	// and `[ ! -a / ]` is 0 — the both-set guard over the two non-empty
	// strings. `[ ! -a x ]` answers 0 under either reading and pins nothing.
	// dash and BusyBox ash refuse both, and ksh93u+ answers 1. Measured
	// 2026-09-19 (#3717).
	s.TestThreeWordsNegateBeforeAConnective = interp.No
	s.TestFailureInsideAnUnclosedGroupIsTheParen = interp.No
	// An operator with nothing behind it at the end of a long expression is
	// the word it is spelled with: `test -n xx -a -f` is 0 here, where bash
	// 3.2 and ksh93 refuse it (#4162).
	// `test -R r` asks whether the name is a reference: bash 3.2 has neither
	// name references nor the operator, which is what makes it an axis.
	s.TestHasTheNameReferenceOperator = interp.Yes
	s.TestTrailingUnaryOperatorIsAWord = interp.Yes
	// And a group holding one or two words is read by the counts: a lone
	// operator inside it is the word it is spelled with, where four words
	// inside is the grammar and the operator takes the parenthesis (#4162).
	s.TestShortGroupIsReadByTheCounts = interp.Yes
	// And a group with nothing in it is refused rather than false:
	// `[ ( ) ]` is `[: (: unary operator expected` at 2 (#3687).
	s.TestEmptyGroupIsFalse = interp.No
	// And `command local a=1` declares the local, which is what makes the
	// two shells that drop it a split rather than a rule (#3370).
	s.LocalThroughCommandDeclaresNothing = interp.No
	s.UnterminatedBracket = interp.BracketLiteral
	// And the same question where a `[:name:]`, a `[.x.]` or a `[=x=]`
	// inside it is what left it open: a literal `[`, and the rest of the pattern behind it — `[[:alpha:]` takes `[` plus one of `:alpha`'s five characters, which is the same reading the axis above gives a bare `[`.
	s.UnterminatedBracketAfterASubExpression = interp.BracketLiteral
	// A bracket the member reading cannot close stays **unterminated** here
	// rather than becoming an empty set: measured 2026-09-26 on 5.3.20 and
	// on 3.2.57 alike, `echo []` and `echo [!]` print the word itself, while
	// `echo []]` and `echo []a~b]` are ordinary sets in every column. See
	// interp.Semantics.EmptyBracketExpressionCompiles (#4644).
	s.EmptyBracketExpressionCompiles = interp.No
	// And inside a bracket expression the backslash protects the character
	// behind it and puts nothing of its own in the set: `[\)]` is the
	// one-character set `)`, and `[a\-z]` is the three members a, `-` and z,
	// the escape being what stops the dash reading as the range operator.
	// zsh adds the backslash to the set as well and BusyBox ash protects
	// nothing, so this value is what five of the seven columns share (#3271).
	s.BracketEscape = interp.BracketEscapeProtectsTheMember
	s.UnknownCharacterClass = interp.UnknownClassIsInert
	// `[[.a.]]` is the collating element `a` and `[[=a=]]` its equivalence
	// class, so both match `a`; zsh is the one column where they are the
	// ordinary characters they spell, and BusyBox ash the one that reads the
	// delimiters and finds an element in no body at all.
	//
	// A body of more than one character is a **name** from the portable
	// character set here and nowhere else in the panel: measured 2026-09-18
	// under `LC_ALL=C`, `[[.hyphen.]]` matches `-`, `[[.period.]]` matches
	// `.` and `[[=space=]]` matches a space, where ksh93 and dash match none
	// of the three. A name outside the roster is a body that is not an
	// element like any other, and the axis above says what that does to the
	// bracket — here, nothing: `[a[.nosuch.]b]` matches a and b and no
	// letter of the body (#3378).
	s.CollatingElements = interp.ACollatingElementMayBeNamed
	// `[[:]` is a bracket holding `[` and `:`: nothing closes the name, so there is no name and the two characters are ordinary members.
	// See interp.Semantics.UnterminatedCharacterClass (#1431).
	s.UnterminatedCharacterClass = interp.UnterminatedClassIsOrdinaryCharacters
	// A backslash that arrived in a value takes the metacharacter status off
	// what follows it and stays in the text itself: measured 2026-09-12 in a
	// directory holding `a\b` and `a*`, `v='a\*'; set -- $v` is the word
	// `a\*` in 5.3.15, in 3.2.57 and as `sh` — neither name matched, so the
	// `*` was not live and the backslash was not removed. ksh93 is the one
	// column that reads it the other way (#1367).
	s.ValueBackslashInAPattern = interp.ValueBackslashQuotesWhatFollows
	// And where the character it quoted is the `/` that closes a piece, the
	// backslash stays in that piece: `./[x]${bs}/e` matches a directory named
	// `x\` here and `./[y]${bs}/e` is the word as written, which is the
	// opposite of what dash and BusyBox ash answer.
	s.ValueBackslashSurvivesAPatternPiece = interp.Yes
	// A bracket carrying a live `/` is not a bracket here, so a word whose only
	// would-be metacharacter was that bracket is not a pattern: `shopt -s
	// nullglob` keeps `[a/b]` and `shopt -s failglob` does not refuse it, which
	// only a word that was never a pattern can do. Measured 2026-09-23 on
	// 5.3.15 and 5.3.20; a quoted separator leaves the bracket a bracket even
	// here. bash 3.2.57 answers the other way and is not followed, as elsewhere
	// in this file. See interp.Semantics.BracketHoldingASlashIsStillABracket
	// (#4158).
	s.BracketHoldingASlashIsStillABracket = interp.No
	// An escaped IFS whitespace character closing a `read` value is trimmed
	// off a value that took the *remainder* of the line and left alone on a
	// value that was its own field. Measured 2026-09-12 on 5.3.15, 3.2.57
	// and as `sh`: `printf 'a b c\\ \n' | read x y` leaves `b c` where
	// `printf 'a b\\ c\\ \n' | read x y` leaves `b c ` — the escaped space
	// in the middle joins the two words into one field, so there is one
	// field per name and no remainder to trim (#1360).
	s.ReadTrailingEscapedSeparator = interp.ReadTrailingEscapedSeparatorTrimmedFromARemainder
	// A longest match over an alternation takes the longest arm, whichever
	// order the arms were written in. Measured 2026-09-11 with extended
	// patterns on, `x=abc`: `${x##@(a|ab)}` and `${x##@(ab|a)}` are both `c`
	// in 5.3.15 and in 3.2.57, and 2026-09-12 for the substitution the axis
	// also governs — `${x//@(a|ab)/X}` and `${x//@(ab|a)/X}` are both `Xc`.
	s.LongestMatchTakesTheWrittenArm = interp.No
	// An empty pattern is declined outright, whatever the value: measured
	// 2026-09-12 on 5.3.15, on 3.2.57 and as `sh`, `v=abc` and `e=`,
	// `${v///X}` and `${v/$e/X}` are both `abc`, and `${e///X}` is empty
	// where the control `${e//*/X}` is `X`. Not a rule about empty matches —
	// with extglob on, `${v//@(|)/<>}` is `<>a<>b<>c` in the same build.
	s.EmptyReplacementPattern = interp.EmptyReplacementPatternMatchesNothing
	// `#` and `%` after the `/` are anchors here, and an anchored pattern
	// that is empty still matches at that end: `v=abcabc` gives `Xbcabc`,
	// `abcabX`, `Xabcabc` and `abcabcX` for `${v/#a/X}`, `${v/%c/X}`,
	// `${v/#/X}` and `${v/%/X}`. ksh93 takes the anchor and declines the
	// empty pattern behind it, and BusyBox ash has no anchors at all
	// (#3272).
	s.ReplacementAnchors = interp.Yes
	s.AnchoredEmptyReplacementPattern = interp.Yes
	// And the anchor is read after a single `/` only: after the global `//`
	// the character is the pattern's own first byte. Measured 2026-09-16 —
	// `${v//#a/X}` on `abcabc` is `abcabc` and, over a value holding the
	// character, `${w//#a/Q}` on `x#ay%bz` is `xQy%bz`, the `#a` found
	// inside. bash 3.2.57 and ksh93 agree; zsh is the one column that reads
	// an anchor there (#3307).
	s.GlobalReplacementAnchors = interp.No
	// And the empty match a global replacement refuses is the one at the end
	// of the value: `${v//@(b|)/<>}` is `<>a<><>c`, which takes the empty
	// match sitting where `b` ended and leaves the end alone.
	s.ReplacementEmptyMatchDeclined = interp.EmptyMatchDeclinedAtTheEnd
	s.StatusArgument = interp.StatusArgNumeric
	s.TraceAssignmentsSeparately = interp.Yes
	// A traced array literal is the words the script wrote, not what they
	// came to: `x="p q"; a=("$x" r)` is `a=("$x" r)` here, and the ordering
	// says so too — `a=($(echo x))` is traced and *then* the substitution
	// runs. Both builds agree.
	s.TraceArrayLiteralShowsTheExpandedElements = interp.No
	// And a subscripted literal is one line here too, the words as written.
	s.TraceSubscriptedArrayLiteralIsElementAssignments = interp.No
	// And the subscript is the text as written: `i=2; a[$i]=v` is
	// `a[$i]=v`, and so is `a[i]=v`.
	s.TraceElementSubscriptIsEvaluated = interp.No
	// An element write expands its value before its subscript:
	// `a[$(echo SUB >&2; echo 1)]=$(echo VAL >&2)x` writes VAL, then SUB.
	s.ElementSubscriptIsExpandedBeforeTheValue = interp.No
	// A frozen element is refused at its store, after the value, the
	// subscript's words and its arithmetic: `readonly a=(1 2);
	// a[$(echo SUB >&2; echo 1)]=$(echo VAL >&2)x` writes VAL, SUB, then the
	// refusal. A frozen whole literal is refused before any of its words.
	s.FrozenElementIsRefusedBeforeItsSubscript = interp.No
	s.FrozenLiteralIsExpandedBeforeItIsRefused = interp.No
	// A literal that fails partway through stores nothing, subscripted or
	// not: the name keeps what it held.
	s.FailedSubscriptedLiteralKeepsWhatItStored = interp.No
	// See interp.Semantics.TableLiteralStoresEachElementAsItIsExpanded (#6770).
	s.TableLiteralStoresEachElementAsItIsExpanded = interp.No
	// See interp.Semantics.DeclarationOperandsAreStoredInTurn (#6770):
	// `x=0; declare x=1 y=$x` leaves `y` holding 0.
	s.DeclarationOperandsAreStoredInTurn = interp.No
	// But an array-literal operand is placed before the word behind it
	// expands: `a=(0); declare -a a=(1) b=("${a[0]}")` leaves `b` holding 1,
	// and `declare -a a=(5) b=(6 $((1/0)))` keeps `a=(5)` (#6798). See
	// interp.Semantics.DeclarationArrayOperandsAreStoredInTurn.
	s.DeclarationArrayOperandsAreStoredInTurn = interp.Yes
	// A bare append onto a table that fails partway keeps the pairs it
	// had added, plain pairs too: `m+=(j new i $((1/0)))` keeps `j`. A
	// declaration's letter does not re-create a name whose literal failed.
	s.FailedTableAppendKeepsThePairsItAdded = interp.Yes
	s.FailedOperandLeavesTheLettersKind = interp.No
	// But a bare literal that fails still makes a name that was not an
	// array one: `b=s; b=(1 $((1/0)))` leaves `declare -a b=([0]="s")`, and
	// an unset name an empty array. See
	// interp.Semantics.FailedBareLiteralStillMakesTheNameAnArray (#7078).
	s.FailedBareLiteralStillMakesTheNameAnArray = interp.Yes
	// bash reports success if it signaled anything at all, where the others
	// count failures one way or another.
	s.ExitTrapRunsOnSignalDeath = interp.Yes
	// And dying is still dying once the trap has run: an `exit 5` in it does
	// not save the shell, and a second fatal signal while it runs ends the
	// shell at once by that second signal. Measured 2026-10-30 on bash 5.3.20,
	// 3.2.57 and 5.3 called `sh` (#7063, #7064).
	s.ExitInTheExitTrapOverridesASignalDeath = interp.No
	s.FatalSignalInTheExitTrapEndsOnlyTheTrap = interp.No
	// And a SEGV is a death like the rest: `kill -SEGV $$` and one from
	// another process run the EXIT trap first. Measured 2026-10-30 on bash
	// 5.3.20 and 3.2.57 (#7069).
	s.UntrappedSegvEndsTheShellWhereItLands = interp.No
	// A `kill` aimed at `$$` is taken at the next command, as a signal from
	// another process is, so the trap reads what `kill` left: `(exit 7); kill
	// -TERM $$` is `bye 0`, `! kill -TERM $$` is `bye 1`, and a function
	// changes neither. A `( … )` that sends it reads 7, the status from before
	// the copy. Measured 2026-10-30 on bash 5.3.20, 3.2.57 and 5.3 called `sh`
	// (#7062).
	s.SelfAimedDeathIsTakenAtTheNextCommand = interp.Yes
	s.DeathInAFunctionEndsTheCallAsKilled = interp.No
	// And a `function name { … }` call is no boundary for one: `trap 'echo
	// "bye $?"' EXIT; function f { /bin/sleep 3; }; f; echo after` with TERM
	// from another process writes `bye 0` and dies of it, and so does `kill
	// -TERM $$` in the body. Measured 2026-10-31 on bash 5.3.20 and 3.2.57
	// (#7067).
	s.SignalDeathEndsAtAKeywordFunctionCall = interp.No
	// And a `( … )` or `$( … )` is a child the signal never reaches, so the
	// shell dies at once: `trap 'echo "bye $?"' EXIT; (exit 7); ( /bin/sleep 2;
	// echo in ); echo after` with TERM from another process writes `bye 7` and
	// dies at once, the parentheses writing `in` at 2 s. Measured 2026-10-31 on
	// bash 5.3.20; bash 3.2.57 waits for a substitution (#7068).
	s.SignalDeathWaitsForAnUnforkedSubshell = interp.No
	// And a death that lands while the shell waits for a pipeline shows the
	// EXIT trap the status the pipeline began with: `trap 'echo "bye $?"'
	// EXIT; (exit 7); /bin/sleep 2 | /bin/cat` with TERM from another process
	// writes `bye 7`, and `bye 0` behind `true &&`. Measured 2026-11-01 on
	// bash 5.3.20; bash 3.2.57 writes `bye 0` (#7160).
	s.DeathDuringAPipelineShowsTheStatusBeforeIt = interp.Yes
	// And a builtin's write past `ulimit -f` with an EXIT trap set is the
	// death it is without one, the trap run for it: `trap 'echo t $?' EXIT;
	// ulimit -f 1; (exit 5); printf '%4000s' x > f` writes `t 5` and dies of
	// XFSZ. Measured 2026-11-01 on bash 5.3.20 (#7173).
	s.CaughtWritePastTheFileSizeLimitIsAFatalError = interp.No
	// And a `-c` string ending over an error the shell reported still runs
	// it: measured 2026-10-16 on bash 5.3.20, a division by zero, a readonly
	// reassignment and an unset name under `set -u` each print the handler
	// through `-c`, where zsh's does not (#6666).
	s.FatalErrorOnACommandStringSkipsTheExitTrap = interp.No
	s.ExitTrapRunsInsideTheExitingCall = interp.Yes
	// 5.3 ignores an untrapped QUIT when it is not interactive, where 3.2
	// dies by it — a divergence between two builds of the same shell, and
	// this preset is 5.3.
	s.QuitIgnoredWhenNotInteractive = interp.Yes
	// And `trap - QUIT` does not take that ignore away: measured, `trap -
	// QUIT; kill -QUIT $$; echo after` prints after and exits 0, with or
	// without a handler having been installed and removed first.
	s.QuitResetRestoresTheDefault = interp.No
	// Interactive, an untrapped XCPU, XFSZ or ALRM ends it as it ends a
	// shell that is not, in 5.3.20 and 3.2.57 alike: measured 2026-10-30 under `-i -c` and from another
	// process during a `read`, a `sleep` and a busy loop (#7058). See
	// interp.Semantics.InteractiveAlarm.
	s.InteractiveIgnoresResourceLimitSignals = interp.No
	s.InteractiveAlarm = interp.InteractiveAlarmKills
	s.HangupIsAnOrderlyExit = interp.No
	s.ExitInTrapReportsEarlierStatus = interp.Yes
	// A subshell's own EXIT trap runs however the subshell ended. Measured
	// 2026-09-18 over the nine rows of interp.SubshellExitTrapPolicy, each
	// plain and under `set -e` (#3612).
	s.SubshellExitTrapAfterAGiveUp = interp.SubshellExitTrapAlwaysRuns
	// `kill -n signum` is an option here, and `-s` with nothing after it is
	// an option missing its argument rather than a signal named `s`.
	s.KillReadsTheNumberOption = interp.Yes
	s.KillOptionWithNoArgumentIsASignalName = interp.No
	s.KillListAcceptsName = interp.Yes
	// `kill -l 129` is HUP and `kill -l 0` is EXIT; one subtraction, and a
	// number that still names nothing is refused.
	s.KillListReducesRepeatedly = interp.No
	s.KillListPrintsANumberItCannotName = interp.No
	s.KillListNamesZeroAsExit = interp.Yes
	// The one column that writes **nothing at all** for a signal it has no
	// name for, rather than the number. Measured 2026-09-17 on Linux, where
	// the question arises, and re-measured 2026-09-21 in the panel's pinned
	// image with `od -c` after "blank line" had been read as "empty line"
	// for long enough to put a newline in the code: `kill -l 32` here is an
	// empty file at status 0, where dash, zsh and BusyBox ash write `32`
	// (#3287, #3984).
	s.KillListLeavesAnUnnamedSignalBlank = interp.Yes
	s.SIGPrefixAccepted = interp.Yes
	s.RedirectsUseEveryTarget = interp.No
	s.KillStatus = interp.KillStatusAnySuccess
	s.SubshellJobTable = interp.SubshellJobsKeptOutsideACompound
	s.PrintfEmptyIsNotANumber = interp.Yes
	s.PrintfAbsentNumberIsAnEmptyOne = interp.No
	s.PrintfStarWithoutOperandIsRefused = interp.No
	// A `*` with no operand left is a zero: `printf '[%.*f]'` is `[0]`.
	s.PrintfStarWithoutOperandIsAbsent = interp.No
	s.PrintfStarComplaintCostsTheStatus = interp.Yes
	// C's own reading, which is C's library: `%G` of an infinity is `INF`
	// and `%10f` of one pads to ten.
	s.PrintfNonFiniteIsConverted = interp.Yes
	// The `'` flag, among the flags and nowhere else: `%'d` groups and
	// `%15'd` is `` `'': invalid format character ``.
	s.PrintfGroupingFlag = interp.Yes
	s.PrintfGroupingFlagAfterTheWidth = interp.No
	// A `*` beside a width's own digits is refused here too: `printf '%5*d' 4 42`
	// is a conversion character this shell does not have (#2824).
	s.PrintfStarBesideTheFieldDigits = interp.No

	// Refused at INT_MAX itself, where ash gives up one past it:
	// `printf '[%2147483647s]' x` is the complaint here and two billion
	// characters of padding there. Measured 2026-09-15.
	s.PrintfFieldBeyondAnInt = interp.PrintfFieldRefused

	// The complaint is the number's range rather than the field's, and the
	// field is still laid out: `printf 'A[%*s]B' 21474836470 x` is `A[x]B`
	// at 1 after `printf: 21474836470: Result too large`. Measured
	// 2026-09-15, and the other half of PrintfFieldBeyondAnInt above.
	s.PrintfStarBeyondAnInt = interp.PrintfStarIsOutOfRange
	s.CaseSubjectKeepsThePreviousLine = interp.No
	s.SubstringRangeThirdColonIsABadSubstitution = interp.No
	s.SubstringCountsBytes = interp.No
	s.PrintfReportsBadNumber = interp.Yes
	s.PrintfNumberOperand = interp.PrintfNumberLeadingNumber
	s.PrintfIntegerCharConstantAfterBlanks = interp.No
	// The digits are written back exactly: `printf '%d' 123456789012345678`
	// is the operand, not a double's nearest neighbor (#2907).
	// A flag past a field is no flag at all: the prefix ends there and the
	// byte arrives at the scan as the conversion character (#2910).
	// unanswered ArithDivisionByZeroYieldsAValue: the value a division by
	// zero leaves behind is only visible through a `printf` operand, and
	// this shell's printf evaluates no operand -- so the question has no
	// site here. Every other expression abandons the command, here as in
	// the two columns that do evaluate.
	s.PrintfFlagAfterTheField = interp.No
	s.PrintfIntegerOperandGoesThroughTheFloatingType = interp.No
	// C99's three: `%F` is `1.500000`, `%a` is `0x1.8p+0` and `%A` is
	// `0X1.8P+0`, all measured 2026-09-14 on bash 5.3.15 under `LC_ALL=C`.
	s.PrintfC99FloatConversions = interp.Yes
	// `%n`, and the count is the bytes this pass has produced: `printf
	// '%.10e%n\n' 1 count` leaves 16 in bash 5.3.20 and in 3.2.57 alike,
	// measured 2026-09-26.
	s.PrintfCountConversion = interp.Yes
	// And it leaves a plain value behind: `q=plain; printf 'abc%n' q` is
	// `q=3` here, where two of the panel make the name an integer.
	s.PrintfCountAttribute = interp.PrintfCountLeavesTheAttribute
	// The operand has to be a plain name, where `printf -v 'a[0]'` and
	// `read 'a[0]'` both fill the element: `arr=(x y z); printf 'abcd%n'
	// 'arr[2]'` is `` printf: `arr[2]': not a valid identifier `` at 1 and
	// leaves the array alone.
	s.PrintfCountOperandTakesASubscript = interp.No
	// An operand that is present and empty is the same as none at all here:
	// `printf 'ab%ncd' ''` writes `abcd`, says nothing and reports 0.
	s.PrintfCountEmptyNameIsIgnored = interp.Yes
	// A word that cannot be a name gives up the rest of the format —
	// `printf 'abc%nXYZ' '1bad'` writes `abc` at 1 — where a name this shell
	// may not write is reported and then stepped over: `typeset -r ro=1;
	// printf 'ab%ncd' ro` writes `abcd`, says `ro: readonly variable` and
	// reports 0. The two cells are why the two axes are not one.
	s.PrintfCountBadNameStopsThePass = interp.Yes
	s.PrintfCountFrozenNameStopsThePass = interp.No
	// And the shortest run of digits that names the value, which is C's
	// default: `printf '%a' 0.1` is `0x1.999999999999ap-4` here.
	s.PrintfHexFloatDefaultIsTwelveDigits = interp.No
	// And a `%a`'s zero fill goes between the `0x` and the digits, which is
	// where C puts it.
	s.PrintfHexFloatZeroFillPrecedesThePrefix = interp.No
	// unanswered PrintfRefusedOperandKeepsItsLeadingNumber: the question is
	// what survives an *arithmetic* failure, and this shell runs no
	// arithmetic over a printf operand. Keeping the number at the front is
	// its whole reading and is PrintfNumberLeadingNumber above, so there is
	// nothing left here to decide. The same goes for
	// PrintfFloatOperandIsEvaluatedTwice: an evaluation that never happens
	// cannot happen twice.
	s.PrintfBackslashC = interp.PrintfBackslashCLiteral
	// A format that ends inside a conversion is an error here, with a
	// second wording of its own — see PrintfMissingVerb.
	s.PrintfUnfinishedConversionIsAPercent = interp.No
	// The not-found handler, one letter short of zsh's name; see
	// interp.Semantics.CommandNotFoundHandler.
	s.CommandNotFoundHandler = "command_not_found_handle"
	// `\x41` is an `A`, and at most two digits: `\x0ff` is 0x0f then an
	// `f`. A `\x` with no digit after it stands as written, with a warning
	// on standard error and a status that is still zero.
	// And it says so when the digit run is empty, which is the half a
	// dialect answers separately from the wording: `printf 'a\xZb'` writes
	// the complaint, the two characters and a zero.
	s.PrintfReportsAMissingHexDigit = interp.Yes
	s.PrintfHexEscape = interp.PrintfHexEscapeByte
	// A `%b` argument reads the same `\x` a format does here, and both of
	// the escape-character spellings: `\e` and `\E` are both 0x1b.
	s.PrintfBHexEscape = interp.PrintfHexEscapeByte
	// `\u0041` is an `A` and `\U00000041` is the same `A`, at both sites:
	// four digits after the one and eight after the other, fewer accepted,
	// and the value written as UTF-8. A `\u` with no digit after it stands
	// as written, with a warning on standard error and a status that is
	// still zero — the same shape this shell's `\x` has. bash 3.2 has none
	// of it, which is why the corpus's `bash32` column differs from the other
	// two; `bash` and `bash-as-sh` agree, the escape being 5.3's rather than
	// something argv[0] turns off.
	s.PrintfUnicodeEscape = interp.PrintfUnicodeEscapeCodePoint
	s.PrintfBUnicodeEscape = interp.PrintfUnicodeEscapeCodePoint
	// Both spellings of the escape character in a *format*, in every build
	// of this shell: `printf 'a\eZ'` and `printf 'a\EZ'` are both
	// `61 1b 5a` in 5.3.20, in 3.2.57 and under argv[0] `sh` (#3225). The
	// `%b` site answers the same way, which is what makes this the one
	// column where the two sites cannot tell each other apart.
	s.PrintfEscEscape = interp.Yes
	s.PrintfCapitalEscEscape = interp.Yes
	// An escape a format does not define keeps its backslash — `printf
	// '[\q][\z][\8][\-]'` is `[\q][\z][\8][\-]` — and a floating
	// conversion's exact half goes to the even neighbor: `printf '%.0f
	// %.0f %.0f' 2.5 4.5 -2.5` is `2 4 -2` and `%.2f` of 0.125 is `0.12`.
	// Measured 2026-09-18 under `LC_ALL=C` from a script file; ksh93 is the
	// one column on the other side of both.
	s.PrintfUnknownEscapeDropsTheBackslash = interp.No
	// Except C's own three, which this column alone reads as C does: `printf
	// "[\'][\"][\?]"` is `['"?]` here and the six characters as written in
	// zsh, dash and BusyBox ash. Measured 2026-09-22 under `LC_ALL=C` from a
	// script file, on 5.3.20 and 3.2.57 alike, and at the format's site only
	// — `printf '%b' "\'"` is `\'` here as it is everywhere. ksh93 prints the
	// characters too and is not evidence for this axis: it drops the
	// backslash from every undefined escape, so its column cannot tell the
	// two readings apart. See interp.Semantics.PrintfQuoteAndQuestionEscapes
	// (#4169).
	s.PrintfQuoteAndQuestionEscapes = interp.Yes
	s.PrintfFloatHalf = interp.PrintfFloatHalfToEven
	s.PrintfBEscEscape = interp.Yes
	s.PrintfBCapitalEscEscape = interp.Yes
	// `printf '%b' 'a\101Z'` is `aAZ`: the octal needs no `\0` to introduce
	// it. Not this shell's answer for an `echo` argument, where `\101` is
	// four characters — the two sites are two tables.
	s.PrintfBOctalWithoutZero = interp.Yes
	// What a `\c` left goes through the conversion's field like any other
	// text: `printf '[%5b]' 'a\cb'` is `[    a`.
	s.PrintfBStopIsPadded = interp.Yes
	s.PrintfBTakesAField = interp.Yes
	// `%zX`, `%ld`, `%jd` and any run of the letters, all of them read and
	// thrown away: `%hhd` with 300 is 300.
	s.PrintfLengthModifiers = interp.PrintfLengthModifiersC99
	// A `%s` field is bytes, and an `l` on `%s` or `%c` makes it characters
	// where the locale has them: `printf '[%.2s|%.2ls|%lc]' αβγ αβγ αβγ` is
	// `[α|αβ|α]` under a UTF-8 locale, bash 5.3.20, 2026-09-16. bash 3.2.57
	// has no wide reading and writes `[α|α|` and the byte 0xce, so the `l`
	// answer is this build's rather than the family's (#2298).
	s.PrintfFieldCountsCharacters = interp.No
	s.PrintfLongModifierCountsCharacters = interp.Yes
	// `%(fmt)T`: an epoch through a date format, with -1 for now and -2 for
	// when the shell started. This shell alone in the panel — 3.2 has it not
	// either, which is why the two bash columns of the corpus differ here.
	s.PidListingFinishesWithAJob = interp.No
	s.PrintfTimeConversion = interp.Yes
	s.PrintfTimeOperandIsADateString = interp.No
	s.PrintfQuote = interp.PrintfQuoteAnsiCWord
	// C's `#` at a value of nought: `printf '%#x' 0` is `0` and
	// `printf '%#.0o' 0` is `0`, bash 5.3.20 and bash 3.2.57 alike.
	s.PrintfAlternateFormAsksTheValue = interp.Yes
	// And it counts that prefix against the width a `0` flag fills:
	// `printf '%#05x' 7` is `0x007` and is five characters, bash 5.3.20
	// and bash 3.2.57 alike.
	s.PrintfZeroFillCountsTheAlternatePrefix = interp.Yes
	// `$'\cA'` is 0x01 and `$'\c1'` is 0x11: the character uppercased and
	// masked to five bits, with `\c?` reading as DEL since 5.x.
	s.DollarSingleBackslashC = interp.DollarSingleControlMasked
	s.DollarSingleUnknownEscape = interp.DollarSingleUnknownKeepsBackslash
	s.DollarSingleNul = interp.DollarSingleNulEndsTheSpan
	// Two digits after `\x`, and an escape with no digit at all stays the
	// two characters it was written as: `$'\xzz'` is `\xzz` here.
	s.DollarSingleHexReadsEveryDigit = interp.No
	s.DollarSingleDigitlessEscapeIsAZeroByte = interp.No
	// The braced spelling is the exception to both of those: `\x{…}` takes
	// every digit the braces hold and keeps the **low byte** of it, and an
	// empty run is a zero rather than the characters written. Measured
	// 2026-09-22 under `LC_ALL=C` on bash 5.3.20 and bash 3.2.57 alike —
	// `$'a\x{41}b'` is `aAb`, `$'a\x{263a}'` is `a` then 3a, `$'a\x{100}b'`
	// is `a` alone because the zero byte ends the span, and `$'a\x{}b'` is
	// `a` for the same reason (#4165).
	s.DollarSingleBracedHex = interp.DollarSingleBracedHexIsAByte
	// An octal escape past 255 keeps the low byte: `$'\401'` is 01 and
	// `$'\777'` is ff. Measured 2026-09-18 by `od` (#3415).
	s.DollarSingleOctalPastAByteDropsTheLastDigit = interp.No
	// And neither `\C` nor `\M` is an escape here: `$'\C-A'` and `$'\M-x'`
	// are kept as written, where zsh reads them as one byte apiece and ksh93
	// reads a different escape out of the same spelling (#2345).
	s.DollarSingleCaretMeta = interp.DollarSingleCaretMetaAbsent
	// `$'\e'` and `$'\E'` are both the escape character, `$'\?'` is the
	// question mark alone, and both Unicode spellings are read as a code
	// point: $'\u0041' and $'\U00000041' are `41`. The last is a bash 4 feature
	// and the 3.2 macOS ships keeps the characters as written, which is a
	// version this dialect does not model — the preset is 5.3 (#3270).
	s.DollarSingleEscEscape = interp.Yes
	s.DollarSingleQuestionEscape = interp.Yes
	s.DollarSingleUnicodeEscapes = interp.Yes
	s.GetoptsAssignmentRestartsWord = interp.Yes
	// A cursor past the word after the last is moved back to it: measured
	// 2026-10-15 on 5.3.20 and 3.2.57, `set -- -a -b -c; OPTIND=100;
	// getopts abc o` is the end of the options with OPTIND 4 (#6575).
	s.GetoptsCursorPastTheEnd = interp.GetoptsCursorPastTheEndIsClamped
	s.GetoptsNewParametersRestartTheScan = interp.No
	// A name operand that is not an identifier is refused on the *store*,
	// after the scan has already moved the cursor. Measured 2026-09-21:
	// `set -- -a; getopts a opt-var` leaves OPTIND at 2 at status 1, while
	// `getopts x 1bad foo` — nothing for the option to be — leaves it at 1,
	// so it is the scan and not an increment.
	s.GetoptsRefusedNameStillScans = interp.Yes
	// `kill %1` reaches the job's process. dash aims at the group.
	s.KillJobSpecAimsAtTheGroup = interp.No
	// A stopped job is left stopped: measured 2026-09-25 through a
	// pseudo-terminal on both builds, `sleep 300 &` then `kill -STOP %1`,
	// with the process's state read from outside the shell. `kill -0 %1` and
	// `kill -WINCH %1` both leave `ps -o stat=` reading `T`; `kill -CONT %1`
	// moves it to `S`, which is the control. Only zsh continues.
	s.KillJobSpecContinuesAStoppedJob = interp.No
	// bash 5.3.20 sends it: `kill -0 -- -1` is 0.
	s.KillRefusesTheAllProcessesTarget = interp.No
	// bash 5.3.20 and 3.2.57 both store it: `typeset a=(VAL) 2>/nope/x`
	// leaves `a` holding VAL. A scalar operand is not stored, which is
	// what keeps this to the array literal.
	s.ArrayOperandIsStoredPastAFailedOpen = interp.Yes
	// bash 5.3.20 reaches the target behind either spelling.
	s.KillTakesEndOfOptionsAfterTheSignal = interp.Yes
	// Measured 2026-09-26 against bash 5.3.20: `kill a b c` writes all
	// three `not a pid or valid job spec` lines. The status is 1 rather
	// than 3 because this shell's KillStatus counts a success and not a
	// failure, which is a separate axis and a separate row.
	s.KillKeepsGoingPastAnOperandThatIsNotAPid = interp.Yes
	// And past a `%` spec that names no job: `kill %99 a` and `kill a %99`
	// are both two lines at status 1. Measured 2026-09-26 (#4666).
	s.KillKeepsGoingPastAJobSpecThatNamesNoJob = interp.Yes
	// The operands are read in the order they were written.
	s.KillReadsJobSpecsBeforeTheOtherOperands = interp.No
	// A stopped job wins the jobs-at-exit sentence wherever it sits: both
	// orders of a running job and a stopped one say `There are stopped
	// jobs.` Measured 2026-09-26 with `shopt -s checkjobs` (#4544).
	s.JobsAtExitSentenceFollowsTheTableOrder = interp.No
	// A trim on `$@` runs over each field. dash and BusyBox ash run it
	// over the joined list once.
	s.OperatorDistributesOverTheFieldList = interp.Yes
	// OPTIND names the word until its last letter has been read: `-abc`
	// reads `a` with OPTIND still 1. dash and BusyBox ash count it at the
	// first letter instead.
	s.GetoptsCountsTheWordAtItsFirstLetter = interp.No
	// And it does not wait for the next call to count it either: the word is
	// counted at its last letter, which is POSIX's own wording and four of
	// the seven columns. The run that reports "no more options" writes `?`
	// into the name (#3275).
	s.GetoptsCountsTheWordOnTheNextCall = interp.No
	s.GetoptsEndOfOptionsNamesIt = interp.Yes
	// A `case` that runs no command ends on 0: measured 2026-10-16 on
	// bash 5.3.20, `case a in $(false)) echo m ;; esac; echo $?` writes 0 (#6710).
	s.CaseWithoutACommandKeepsTheSubstitutionStatus = interp.No
	// unanswered ProducedIntegerReadsInItsBase: `declare -i` takes no output base here, so no
	// produced integer can carry one to read it in.
	// A `+`-prefixed word is an operand and ends the scan: measured
	// 2026-09-17 on 5.3.20, on 3.2.57 and under the name `sh`, `getopts a o
	// +a` is 1 with the name `?`.
	s.GetoptsTakesAPlusPrefixedOption = interp.No
	// `getopts 'n#' o` is two options here, neither of which takes anything:
	// `-n 5` leaves the 5 as an operand and `-n5` reports 5 as unknown.
	// Measured 2026-09-16, bash 5.3.20 (#2947).
	s.GetoptsOptionStringHasANumericType = interp.No
	// OPTERR is this shell's alone, and setting it to zero turns the
	// `getopts` diagnostic off without moving to the silent form — which is
	// the only spelling for "keep `?` and lose the noise", the leading colon
	// changing the name and OPTARG as well. Measured 2026-09-17 on 5.3.20, on
	// 3.2.57 and under the name `sh`: a bad option and a missing argument are
	// each one line on stderr at OPTERR=1 and none at OPTERR=0, with the name,
	// the status and OPTARG the same either way.
	s.GetoptsOptErrSilencesTheComplaint = interp.Yes
	s.GetoptsClearsOptarg = interp.No
	s.GetoptsEmptiesOptargForAnArgumentlessOption = interp.No
	// A freeze on OPTARG or OPTIND is consulted here — `readonly OPTARG;
	// getopts a: o` writes `OPTARG: readonly variable` — and costs the
	// builtin nothing: the name is still filled in and the status is still
	// 0. Both measured 2026-09-16 on 5.3.20 and 3.2.57 alike.
	// The call that reports "no more options" unsets OPTARG, so the last
	// option's argument is not left standing for a script to read after its
	// loop.
	s.GetoptsUnsetsOptargAtEndOfOptions = interp.Yes
	// And that clearing — like the ones a bad option and a missing argument
	// make, and unlike the one an argument-less option makes — takes the name
	// away rather than writing over it, so a `readonly OPTARG` neither stops
	// it nor survives it. bash alone; measured 2026-09-16 on 5.3.20, on
	// 3.2.57 and under the name `sh`.
	s.GetoptsClearingOptargIsARealUnset = interp.Yes
	// And over a name operand spelled OPTARG, a reported fault writes OPTARG
	// after the name where every other path writes it before: `getopts :x
	// OPTARG -y` leaves `y` and `getopts x: OPTARG -x6` leaves `x`. Measured
	// 2026-10-15 on 5.3.20, on 3.2.57 and under the name `sh`; see the axis.
	s.GetoptsOptargOrder = interp.GetoptsOptargAfterTheNameOnAnError
	s.GetoptsOwnParametersIgnoreAFreeze = interp.No
	s.GetoptsRefusedWriteEndsTheBuiltin = interp.No
	// A freeze on the **name** is reported and the script runs on, wherever
	// the scan was: measured 2026-09-17 on 5.3.20 and 3.2.57, `readonly N`
	// over `set -- -a v` and over `set -- x` alike.
	s.GetoptsFrozenNameAtTheEndOfTheOptionsIsFatal = interp.No
	// The one `getopts` store that ends the shell here is a typed name's
	// value that will not evaluate, and it leaves 1, not 0: measured
	// 2026-10-15 on 5.3.20, `declare -i v; set -- -z; getopts a v; echo
	// after` is the arithmetic complaint and status 1 from `-c` and from a
	// script file (#6588).
	s.GetoptsStoreRefusalLeavesZero = interp.No
	// A value the typed name will not take ends the script, from `getopts`
	// and `read` alike: measured 2026-10-15 on bash 5.3.20 and 3.2.57,
	// `typeset -i n=5; getopts x n -y` and `read n <<< 1+` are each the
	// arithmetic complaint and nothing after it. See the two axes.
	s.GetoptsTypedValueFailureIsAFreeze = interp.No
	s.ReadTypedValueFailureIsAnAssignmentFailure = interp.Yes
	// And that cost is the rest of the line rather than the script, off a
	// `-c` string: measured 2026-10-15 on 5.3.20 and 3.2.57 from a script
	// file, `typeset -i n=5`, then `n=1+; echo same`, then `echo next $?` is
	// the complaint and `next 1` (#6631).
	s.TypedValueFailureGivesUpTheLine = interp.Yes
	// `read` stops at the first name a freeze refuses and leaves the names
	// after it alone, which is the other answer from `getopts` above and is
	// why the two are separate axes: `readonly a; printf 'x y\n' | { read a
	// b; }` leaves b untouched here and fills it in ksh93.
	s.ReadRefusedWriteEndsTheBuiltin = interp.Yes
	// And the status parts "I stopped early" from "a write failed": 2 where a
	// name was left to fill and 1 where the frozen name was the last one.
	// Measured over three names in 5.3.20, 2026-09-16 (#3208).
	s.ReadRefusedWriteIsOneOnTheLastName = interp.Yes
	// And the refusal is not the end of the script: `getopts a: o; echo
	// reached` prints `reached`, where a plain `x=2` over a frozen `x` gives
	// up the rest of the line in every shell here.
	s.ReadonlyRefusalInABuiltinIsFatal = interp.No
	// The scan position is shared with the caller across a shell function
	// call, which is why every option-parsing helper here begins with
	// `local OPTIND=1`: without it a second call starts where the first
	// stopped and reads nothing.
	s.GetoptsFunctionPosition = interp.GetoptsFunctionPositionIsShared
	// unanswered OptindUnsetInACallStaysUnbound: a call shares the caller's
	// cursor here, so there is none of its own for an unset to take away.
	// And `getopts` writes an unset OPTIND back on its next call: measured
	// 2026-10-16 on bash 5.3.20, `unset OPTIND; getopts a o -a` leaves it at
	// 2, and the scan after the unset starts at the first word (#6660).
	s.GetoptsLeavesAnUnsetOptindUnset = interp.No
	// A `local OPTIND` gives the call the whole cursor and gives the caller
	// the whole cursor back — the position inside a clustered word along
	// with the number. bash 3.2 restores only the number, so a loop whose
	// body calls a function that declares one never finishes there; this is
	// the newer answer and the one `getopts ab o; g; getopts ab o` reads `b`
	// from (#2226).
	s.GetoptsLocalOptindRestoresTheCursor = interp.Yes
	// Measured 2026-09-12: `OLDPWD=/nonexistent bash -c 'echo ${OLDPWD-UNSET}'`
	// answers UNSET, and the same with a directory answers the directory. A
	// plain file is dropped and a mode-000 directory is kept, so the test is a
	// stat rather than the one `cd` makes. bash 3.2 drops an inherited OLDPWD
	// whatever it names, which is a fourth answer and not this dialect's.
	s.InheritedOldpwd = interp.InheritedOldpwdTakenIfADirectory
	// The depth, counted and told to every child — and the one column of the
	// panel with a ceiling on it: a level of 1000 or more is refused with a
	// warning and the count starts again. See interp.ShellLevelPolicy.
	s.ShellLevel = interp.ShellLevelCountedToACeiling
	// Read whole or not at all: `SHLVL=3x` is counted from nothing.
	// Measured 2026-10-02 on 5.3.20 (#5145).
	s.ShellLevelReading = interp.ShellLevelReadsTheWholeValue
	// And a shell that replaces this process stands in its place rather than
	// under it: measured 2026-09-18, `exec /usr/bin/env` from a script file
	// under `env -i` hands over `SHLVL=0` where this shell holds 1, so the
	// shell it starts reads the same number this one did. The floor above is
	// what makes an inherited `-1` read 1 on the far side rather than 0.
	s.ShellLevelExec = interp.ShellLevelExecNotCounted
	// What an interrupt from outside does while a foreground program runs.
	// See interp.Semantics.InterruptWaitsForTheProgram (#5416).
	s.InterruptWaitsForTheProgram = interp.InterruptWaitsForAnyProgram
	// `PWD` is a different answer from OLDPWD's here: the starting directory
	// is named by what the kernel reports, in 5.3 and 3.2 alike.
	s.StartupPwdName = interp.StartupPwdNameFromTheKernel
	s.CdWithoutHomeIsAnError = interp.Yes
	// `env -i bash -c 'echo "[$HOME]"'` is empty, so nothing is seeded — and
	// a written `~` is a path all the same, which is the password entry read
	// at the tilde rather than at startup and is TildeWithNoHome's question.
	// Measured 2026-09-26 in 5.3.20 and 3.2.57 alike.
	s.StartupFillsAnAbsentHome = interp.No
	// And a `HOME` that has been removed is an absent one: `HOME=/tmp; unset
	// HOME; cd` is `cd: HOME not set` at 1 in both builds, exactly as a shell
	// that never had one answers. See Semantics.CdRemembersAHomeThatWasUnset.
	s.CdRemembersAHomeThatWasUnset = interp.No
	s.CdDashPrintsTheDirectory = interp.Yes
	// `cd -` reads the OLDPWD parameter: `cd /; OLDPWD=/usr; cd -` goes to
	// /usr. Measured 2026-10-03.
	s.CdDashFollowsTheShellsOwnRecord = interp.No
	// `cd .` in a directory that has been renamed out from under the shell
	// moves, and `$PWD` becomes the new name. Measured 2026-09-26 on
	// /opt/homebrew/bin/bash (5.3) and /bin/bash (3.2) alike: with `d`
	// renamed to `e`, `cd .` is 0 and `$PWD` reads `…/e`, and `cd s` lands in
	// `…/e/s`. Both builds, same answer, so this is not a version.
	// See Semantics.CdDestinationIsNotThere for the pair that fixes the noun.
	s.CdDestinationIsNotThere = interp.CdDestinationNotThereEntersAndTakesTheKernelsName
	// `cd nosuch/..` is refused here, in 5.3.20 and in 3.2.57 alike: every
	// component a `..` cancels is looked at first, whether it came from the
	// operand or from the directory the shell is in. `cd real/..` is 0 in the
	// same tree, which is the control. See Semantics.CdCancelsADotDot (#4627,
	// #4668).
	s.CdCancelsADotDot = interp.CdDotDotLooksAtEveryCanceledComponent
	// `cd` pushes nothing, in the one other shell that *has* a directory
	// stack — which is what makes this column the control rather than a
	// vacuous no. Measured 2026-09-26 on bash 5.3.20: `pushd sub` leaves
	// `dirs` reading two entries, so the stack works; `cd ..` after it leaves
	// two entries still, with the top replaced by the new `$PWD`, so the move
	// added nothing. Neither `shopt` nor `set -o` names an option with
	// `pushd` in it, and the same greps find `huponexit` and `errexit`.
	s.CdPushesTheDirectoryItLeaves = interp.No
	s.PrintfAssignsWithV = interp.Yes
	s.PrintfRejectsUnknownOption = interp.Yes
	s.TrapParsesOptions = interp.Yes
	s.TrapPrintsWithP = interp.Yes
	s.TrapPrintsBareWithConditions = interp.No
	s.TrapPrintsBareWithP = interp.Yes
	s.TrapListsSignalsWithL = interp.Yes
	s.TrapOneArgumentIsACondition = interp.Yes
	// A number this kernel delivers that no name in the table covers is a
	// condition here. Measured 2026-09-19 on linux/arm64, bash 5.2.15 in
	// Debian bookworm, the script file `trap 'echo CAUGHT' $1; kill -$1 $$;
	// echo SURVIVED`: 15, 40 and 64 all print CAUGHT then SURVIVED at 0.
	s.TrapTakesASignalNumberItCannotName = interp.Yes
	s.TrapReportsAnUnknownSingleCondition = interp.Yes
	s.TrapSingleUnknownConditionIsUsage = interp.Yes
	// All three pseudo-conditions, RETURN being this shell's alone. None of
	// them follows the script into a function it was not set in, or into a
	// subshell — measured: `trap 'echo E' ERR; f(){ false; :; }; f` prints
	// nothing, and a command substitution captures no handler output.
	s.TrapHasErrCondition = interp.Yes
	s.TrapHasDebugCondition = interp.Yes
	s.TrapHasReturnCondition = interp.Yes
	s.ErrTrapRunsInsideFunctions = interp.No
	s.ErrTrapRunsInSubshells = interp.No
	s.ErrtraceReachesSubshells = interp.Yes
	// And the command that *ran* a failure fires the condition again, but
	// only where a trap was set before that command began. Both halves are
	// measured, from a script file with the action printing $LINENO: with
	// the trap set at the top and `set -E` carrying it in, `g(){ false; }`
	// fires at the body's line and again at the call's; with the trap set
	// for the first time *inside* g it fires at the body's line alone, with
	// or without the option. `. ./lib.sh` over a file holding `false` fires
	// twice with nothing asked, because a sourced file never bounded this
	// trap in the first place.
	s.ErrTrapRefiresForTheCommandItFiredInside = interp.ErrTrapRefiresWhereItWasSetFirst
	// An error inside a handler ends the handler — a signal's, ERR's,
	// DEBUG's or RETURN's — except an unset parameter under `set -u`, which
	// ends the shell from there as from anywhere; and the error itself
	// raises no ERR. Measured 2026-10-02 on 5.3.20, see
	// interp.HandlerErrorReach (#5360).
	s.AHandlersErrorEnds = interp.HandlerErrorEndsTheHandlerUnlessAParameterIsUnset
	// Several handlers waiting together — signals that came while a
	// foreground command ran — run lowest signal number first, whatever
	// order they came in. Measured 2026-10-31 on 5.3.20 and 3.2.57, and on
	// 5.3 under the name `sh`; see interp.WaitingHandlerOrder (#7039).
	s.WaitingHandlersRun = interp.WaitingHandlersRunUpTheSignalNumbers
	// And one of them that lost its trap to an earlier one before its turn
	// is warned about at its turn, and sent again where the trap went back
	// to the default: `trap - USR2` in HUP's handler dies of USR2 after the
	// two warnings. Measured 2026-11-01 on 5.3.20, 3.2.57 and 5.3 under the
	// name `sh`; see interp/waitingtrapgone.go (#7170).
	s.WaitingSignalWithoutItsTrapIsResent = interp.Yes
	// And one that arrived again while it waited runs once: `kill -USR1 $$
	// $$` is `T after`, as are two `/bin/kill`s 0.2 s apart during one
	// `/bin/sleep`. Measured 2026-11-01 on 5.3.20 and 3.2.57; see
	// interp/waitingsignalagain.go (#7166).
	s.WaitingSignalArrivingAgainRunsOnce = interp.Yes
	s.ErrTrapFiresForAnErrorTheShellGaveUpOver = interp.No
	// `read -d £` stops at the first byte of the `£`, under a UTF-8 locale
	// too, and the second begins the next record. Measured 2026-10-02 on
	// 5.3.20 (#5153).
	s.ReadDelimiterIsTheLocalesCharacter = interp.No
	// `printf '[%q]'` with no operand quotes the empty string: `['']`,
	// measured 2026-10-02 on 5.3.20 (#5153).
	s.PrintfQuoteOfNoArgumentIsEmpty = interp.No
	// An element store arithmetic refuses is reported and the expression
	// goes on with the value: `(( a[-10] = 1 ))` is 0 after the complaint.
	// Measured 2026-10-02 on 5.3.20 (#5145).
	s.ArithStoreRefusalIsAnError = interp.No
	// Reached only under `shopt -s lastpipe`: the last element is judged as a
	// statement and then the pipeline is, so `true | false` and `true |
	// /usr/bin/false` both write EE there.
	s.FailingPipelineWhoseLastElementRanHere = interp.LastElementJudgedThenThePipeline
	s.DebugTrapRunsInsideCalls = interp.No
	s.DebugActionArmingErrExitSkipsTheCommand = interp.No
	s.DebugTrapRefiresOnEnteringAFunction = interp.Yes
	// The heads whose own work is a word or an expression: `case`, `[[`,
	// `((` and `select` once each, the list `for` on every pass and each of
	// the arithmetic `for`'s three expressions every time one is evaluated —
	// written or not. `if`, `while`, a group, a subshell and a function
	// definition write nothing. The zero value, and the same in all three
	// bash columns.
	s.DebugTrapCompoundHeads = interp.DebugTrapHeadsWordAndArithmetic
	// A repeated `for` head names its own line on every pass.
	s.DebugListLoopHeadKeepsTheLine = interp.No
	// A pipeline fires once for each element that is a **simple command**,
	// and it fires in the shell running the pipeline rather than in the
	// element — which is what lets a trap fire at all here, since this
	// shell does not carry one into a subshell. Measured: `trap 'echo d'
	// DEBUG; echo a | tr a-z A-Z` writes `d d A`, both actions in lower
	// case because neither went down the pipe, and `case x in x) echo hi;;
	// esac | cat` writes one firing rather than two — an element that is
	// not a simple command fires nothing, whatever head it would fire for
	// standing on its own.
	s.DebugTrapPipelines = interp.DebugTrapPipelinePerSimpleElement
	// An `&&`/`||` list has no rule of its own here: each operand fires
	// whatever it would standing alone. Measured 2026-09-25 on 5.3.20 run
	// `--noprofile --norc` — `echo x && echo y` writes two firings, `echo w
	// && echo v && echo u` three, and each names its own operand in
	// `$BASH_COMMAND` (#4556). Written rather than left to the zero value,
	// because it is a measurement and the shells disagree.
	s.DebugTrapSublists = interp.DebugTrapSublistPerOperand
	s.DebugTrapRunsInSubshells = interp.No
	// Ahead of the command, with no option to move it: measured 2026-09-25
	// on bash 5.3 with an action printing `$LINENO` and `$?`, a `trap` on
	// line 1 and `echo A`, `false`, `echo B` on 2, 3 and 4 — `2`, `A`, `3`,
	// `4 st=1`, `B`, so every firing stands ahead of its command and the
	// status the action reads is the previous one's.
	s.DebugTrapRunsBeforeTheCommand = interp.Yes
	// The shell that keeps the parent's trap listing across every boundary
	// but a process substitution — `(trap)`, `$(trap)`, `trap | cat` and
	// `trap &` all print what the parent had, EXIT trap included, though a
	// handled signal no longer fires there. Measured: `trap 'echo x' USR1;
	// (kill -USR1 $BASHPID; echo alive)` dies of the default action while
	// `(trap)` still lists the trap.
	s.SubshellKeepsTrapListing = interp.Yes
	s.PipelineElementKeepsTrapListing = interp.Yes
	// unanswered ALoneTrapCommandKeepsTrapListing: every boundary it is
	// asked behind keeps the listing here, so it is never reached.
	s.BackgroundJobKeepsTrapListing = interp.Yes
	s.KeptTrapListingIncludesExit = interp.Yes
	s.UlimitBlockIsKilobyte = interp.Yes
	// An empty operand is refused here — `ulimit -n ""` is `: invalid
	// number` at 1 — where bash 3.2.57 reads it as nought and lowers the
	// limit to 0. A 5.x change, so the newer answer stands, as it does for
	// the permission copy above (#3064). Measured 2026-09-18.
	s.UlimitEmptyOperandIsZero = interp.No
	// And CDPATH is a search *beside* the ordinary relative lookup: a miss
	// falls back to the operand as it stands, and a `./` operand is not
	// searched at all (#2896).
	s.CdpathReplacesTheRelativeLookup = interp.No
	s.UlimitSetsBothLimits = interp.Yes
	// `ulimit -n hard` and `-n soft`, which bash alone has both of.
	s.UlimitTakesHardKeyword = interp.Yes
	s.UlimitTakesSoftKeyword = interp.Yes
	s.UlimitOperandIsArithmetic = interp.No
	s.BadOptionToSpecialBuiltinFatal = interp.No
	// `alias` is no more special here than POSIX makes it: the complaint is
	// said and the next command runs. Measured with `alias -g x`.
	s.AliasBadOptionFatal = interp.No
	// A name an alias may not carry, swept over every printable ASCII
	// character on 2026-09-12: whitespace and the shell's own
	// metacharacters, plus `/`. The five ksh93 refuses beside them —
	// `* ? [ { }` — are taken here, which is why the set is a value and not
	// an axis (#2413). `=` cannot be in it: the first one separates the name
	// from the value, so a name reaching the check never holds one.
	s.AliasNameRefusedCharacters = "\t\n \"$&'()/;<>\\`|"
	// Only a definition is checked. `alias 'a$b'` is `alias: a$b: not found`
	// here, which is the answer any name it does not hold gets.
	s.AliasNameCheckReachesALookup = interp.No
	// And the complaint costs the builtin 1 and the script nothing: with the
	// refusal on line 2 of three, line 3 still runs and the shell exits 0.
	s.AliasInvalidNameFatal = interp.No
	s.EarlierDeclarationLetterBlocksALaterPlus = interp.No
	// A letter written under both signs: see
	// interp.Semantics.ALetterUnderBothSignsComesOff (#5673).
	s.ALetterUnderBothSignsComesOff = interp.Yes
	// And the mirror, a minus word after a plus one: see
	// interp.Semantics.EarlierPlusMakesALaterLetterARemoval (#5667).
	s.EarlierPlusMakesALaterLetterARemoval = interp.No
	// A typed declaration whose own evaluation fails ends the script at the
	// fatal status: see interp.Semantics.DeclaredTypedValueFailureLeavesZero
	// (#5677).
	s.DeclaredTypedValueFailureLeavesZero = interp.No
	// A redirection that cannot be made is where bash parts from POSIX and
	// from three of the panel: `exec 3>/nope/x; echo after` complains and
	// prints `after` at status 0. It is the starting value rather than a
	// fixed one — `set -o posix` moves it, which is the whole of the
	// bash-as-`sh` column; see the posix option below.
	// bash reads the number and fails at run time if nothing is open there:
	// `10: Bad file descriptor`, status 1, and the script carries on.
	s.MultiDigitDuplicationTargetIsAnError = interp.No
	s.RedirectErrorOnSpecialBuiltinFatal = interp.No
	// `echo hi >&qq` writes the file `qq` and puts both output streams in
	// it, which is the csh spelling bash kept. A word that expanded to
	// nothing is not a name, and is refused as a descriptor instead.
	s.GreatAmpTarget = interp.GreatAmpTargetNamesAFile
	// `exec 5<f; exec 6<&5-` moves the descriptor and reports 0, in 5.3, in
	// 3.2 and under argv[0] of `sh` alike. Two steps rather than one, which
	// shows twice: `true 6<&5-` leaves 5 closed once the command has ended,
	// where the plain `true 5<&-` is undone; and `{v}<&$w-` hands the name
	// the number *above* the one it moved from, because the destination is
	// chosen before the source is given up.
	s.FdMove = interp.FdMoveDuplicatesThenCloses
	// And the `-` has to be written: one that arrives by expansion leaves
	// `9-`, which is not a descriptor — `x=9-; exec 4<&$x` is `9-: ambiguous
	// redirect` (#6746). See interp.Semantics.FdMoveSuffixMustBeWritten.
	s.FdMoveSuffixMustBeWritten = interp.Yes
	// A command word beginning with `%` is a job specification here too, and
	// this shell asks it of the **name the lookup is about to use** rather
	// than of the word the script wrote: `"%prep"`, `\%prep`, a parameter
	// holding the text and even `command %prep` are all job specs, where zsh
	// takes only an unquoted, unescaped leading `%`. Measured 2026-09-29 on
	// 5.3.20; nothing in the plain `%prep` row can tell the two readings
	// apart, which is why the axis is a form. See
	// interp.Semantics.JobSpecCommandWord.
	s.JobSpecCommandWord = interp.JobSpecCommandWordAfterExpansion
	// A frozen name does not stop the close of the descriptor it holds here:
	// measured 2026-09-29 on 5.3.20, a script file, `exec {m}>f; readonly m;
	// exec {m}>&-; echo "after=$?"` closes it in silence at 0, where zsh and
	// ksh93 refuse. The close reads the name rather than writing it, which is
	// what makes this a question at all — see
	// interp.Semantics.ReadonlyFdVariableRefusesAClose.
	s.ReadonlyFdVariableRefusesAClose = interp.No
	// And a frozen name is found out only when the number will not store:
	// `readonly v=1; true {v}>/nodir/f` reports the open's failure and not
	// the name, measured 2026-10-15 on 5.3.20 (#6571). See
	// interp.Semantics.FdVariableReadonlyRefusedFirst.
	s.FdVariableReadonlyRefusedFirst = interp.No
	// And a `{name}` store is made after the command's own prefix of the
	// same name and into it: `f() { echo "in[$v]"; }; v=pre f {v}</dev/null`
	// is `in[10]`, measured 2026-10-15 (#6569). See
	// interp.Semantics.FdVariableStoredAfterThePrefix.
	s.FdVariableStoredAfterThePrefix = interp.Yes
	// And a command with no command word whose redirections carry a
	// `{name}` makes the whole list in a child: `{v}>/dev/null; echo "[$v]"`
	// is `[]`, and the next `exec {w}` still gets 10, measured 2026-10-15 on
	// 5.3.20 (#6630). See interp.Semantics.FdVariableMakesABareListASubshell.
	s.FdVariableMakesABareListASubshell = interp.Yes
	// And `set -C` does not protect the name: `exec {m}>f1; exec {m}>f2`
	// allocates a second descriptor and overwrites it in silence, measured
	// 2026-09-29. See interp.Semantics.NoclobberProtectsAnFdVariable.
	s.NoclobberProtectsAnFdVariable = interp.No
	// `( : ) {v}</dev/null` makes the allocation in the subshell: the shell
	// keeps neither `v` nor descriptor 10, measured 2026-10-15 on 5.3.20
	// (#6520). See interp.Semantics.FdVariableOnASubshellIsTheSubshells.
	s.FdVariableOnASubshellIsTheSubshells = interp.Yes
	// A `{name}` store is an ordinary assignment here: attributes and
	// export are kept. See interp.Semantics.FdVariableStoreTyping.
	s.FdVariableStoreTyping = interp.FdVariableStoreIsAnAssignment
	// And a program is handed the environment from before its own `{name}`
	// store: `export v=keep; /usr/bin/env {v}</dev/null` shows it
	// `v=keep`, measured 2026-10-15 (#6521). See
	// interp.Semantics.FdVariableReachesTheProgramsEnvironment.
	s.FdVariableReachesTheProgramsEnvironment = interp.No
	// unanswered FdVariableBadName: this grammar never lexes a `{word}` that
	// is not a name — `echo {a-b}</dev/null` and `echo {1}</dev/null` print
	// the word, measured 2026-10-15 on 5.3.20 — so there is nothing for the
	// question to be about (#6522).
	s.DuplicationTargetError = interp.DuplicationTargetErrorCarriesOn
	// A target a process substitution begins with more after it is opened
	// as the path it comes to: `cat < <(echo a)x` is `/dev/fd/63x`, measured
	// 2026-10-16 (#6766). See interp.Semantics.SubstitutionTargetWithMoreIsRefused.
	s.SubstitutionTargetWithMoreIsRefused = interp.No
	// A redirection target's substitution body: `set -x; cat < <(echo hi)` writes `++ echo hi`, measured
	// 2026-10-16 (#6799). See
	// interp.Semantics.RedirectTargetSubstitutionBodyIsTraced.
	s.RedirectTargetSubstitutionBodyIsTraced = interp.Yes
	s.LocalOutsideAFunctionIsAnError = interp.Yes
	s.LocalOutsideAFunctionIsFatal = interp.No
	// bash reports every operand that is not a name, exports the ones that
	// are, and carries on with a status of 1.
	s.BadNameToDeclarationFatal = interp.No
	s.BadNameToUnsetFatal = interp.No
	// `read` is not a special builtin anywhere, so its bad name is not fatal
	// here either — `read 1bad; echo after` prints both lines.
	s.BadNameToReadFatal = interp.No
	// And `getopts`: measured 2026-09-18, `echo A; getopts x 1bad -x; echo
	// "B st=$?"` writes all three lines here, so the refusal costs the
	// script nothing beyond its own status (#3555).
	s.BadNameToGetoptsFatal = interp.No
	// And `printf -v` refuses the same operands without ending anything:
	// measured 2026-09-17, `printf -v '1x' %s Q` is
	// ``printf: `1x': not a valid identifier`` at 2 with the rest of the
	// line still running, and `a-b` is the same sentence.
	s.BadNameToPrintfFatal = interp.No
	// The refusal to unset a readonly name is reported and not fatal — and
	// this is the one of bash's `unset` answers that POSIX mode moves, which
	// is why it is not BadNameToUnsetFatal read twice. `set -o posix` makes
	// bash 5.3 stop here and changes nothing about `unset 1x`. bash 3.2 is
	// fatal in neither mode; this preset is bash 5's.
	s.UnsetReadonlyFatal = interp.No
	s.DeclarationNameOperands = interp.PlainNamesOnly
	// bash 5.3's bare `unset` checks nothing — `unset 1x`, `unset "a b"` and
	// `unset -- -` are all quiet — while `unset -v 1x` refuses. bash 3.2
	// refused all of them, so the panel's two bash columns differ here.
	s.UnsetNameOperands = interp.AnythingIsAName
	// `read` sits back on the strict answer whatever `unset` does: `read 1`,
	// `read "a b"` and `read -- -x` are each `not a valid identifier` in
	// bash 5.3 and in the 3.2 that macOS ships.
	s.ReadNameOperands = interp.PlainNamesOnly
	// And bash has no prompt operand — `read "v?p"` is the bad name `v?p`,
	// quoted back whole. The prompt is `-p` here and only `-p`.
	s.ReadPromptOperand = interp.ReadOperandIsAllName
	// bash 5.3 judges the first operand before it goes to the stream:
	// `printf 'AAA\nBBB\n' | { read 1bad; cat; }` prints both lines. bash
	// 3.2 reads first and prints only BBB, so the panel's two bash columns
	// differ here as they do on UnsetNameOperands; this preset is bash 5's.
	s.ReadRefusesABadNameBeforeReading = interp.Yes
	s.ReadJudgesEveryNameBeforeReading = interp.No
	// A count changes nothing about which operands are judged here:
	// `read -n 3 a 1bad` refuses `1bad` and fills a with the three
	// characters, exactly as `read a 1bad` refuses it.
	s.ReadCountJudgesTheNamesAfterTheFirst = interp.Yes
	// See interp.Semantics.ReadCountFillsTheFirstNameWhole (#6776).
	s.ReadCountFillsTheFirstNameWhole = interp.No
	// unanswered ExportThroughASubscriptedOperandRecordsTheLetter: the
	// operand is refused one complaint earlier here, as
	// ``export: `a[1]': not a valid identifier`` at 1, so no element
	// is written and no letter could land.
	s.DeclarationTakesASubscript = interp.No
	// And a *declaration* takes one where `export` does not, which is this
	// shell alone splitting the two: `export a[1]=v` is `not a valid
	// identifier` and `typeset a[1]=v` creates the element, measured.
	s.TypesetTakesASubscript = interp.Yes
	s.UnsetTakesASubscript = interp.Yes
	// The one element goes and the rest of the array stands, inside a
	// subshell as at the top level.
	s.UnsetElementEmptiesAnUnwrittenArrayInASubshell = interp.No
	// `read 'a[2]'` and `printf -v 'a[2]'` fill the element, measured
	// 2026-09-10 on `a=(x y z)` — `x Q z`, in 3.2 as well as 5.3.
	s.StoreOperandTakesASubscript = interp.Yes
	// And `getopts` does not, in the shell that fills `read 'a[1]'`:
	// measured 2026-09-18, `getopts x 'o[1]' -x` draws this shell's
	// not-a-valid-identifier refusal at 1, brackets and all (#3555).
	s.GetoptsOperandTakesASubscript = interp.No
	// `r[@]` and `r[*]` on a builtin's operand are refused by the operand
	// as written and say nothing about arithmetic: measured 2026-09-17,
	// `r=(1 2 3); read 'r[@]' <<< Y; echo "same=$?"` is
	// `r[@]: bad array subscript`, `same=1`, the array whole and the rest
	// of the line still running. `printf -v 'r[@]'` answers identically,
	// 1 included — the number here is the refusal's and not the
	// builtin's own 2.
	s.StoreOperandWholeArraySubscript = interp.StoreOperandWholeArraySubscriptIsBad
	// Over a **table** the same brackets are an ordinary key, silently and
	// at 0: measured 2026-09-17, `typeset -A m=([k]=v); read 'm[@]'` on
	// `Z` leaves the keys `@` and `k` standing. So the array is refused
	// here and the table is taken, which is the way round this shell
	// answers the bare assignment too.
	s.StoreOperandWholeArraySubscriptOverATable = interp.WholeArraySubscriptIsAnOrdinaryKey
	// And what a declaration of an element may also do to the array: the
	// integer attribute lands on it and the element is stored converted —
	// `typeset -i a[1]=0x10` reads back 16 — and inside a function the array
	// becomes local, holding the element, with the caller's array back on
	// return. Both measured 2026-09-07.
	//
	// Semantics.ReadonlyElement is deliberately left unanswered: bash's
	// answer is a third one — `typeset -r a[1]=v` creates the array frozen
	// and empty, reports `a: readonly variable` and reports success — and
	// naming it here would need the refusal to run after the freeze rather
	// than instead of it (#1203).
	// Never asked here, because a bad name is not fatal in this shell: it
	// reports each one and declares the good ones by carrying on. The answer
	// is the one the same binary gives under an argv[0] of `sh`, where the
	// fatality is on and only the operands in front of the bad one are
	// declared.
	// `read -a` reads its edges as the ordinary field split does: a closing
	// run of separators is absorbed, whitespace or not, and a line with no
	// fields in it fills no elements at all — measured 2026-09-07, where
	// ksh93 and zsh both leave one empty element.
	s.ReadTrailingWhitespaceEndsAField = interp.No
	s.ReadNoFieldsIsOneEmptyElement = interp.No
	// The whitespace half of IFS is every space character of the C locale
	// here, which is `isspace` rather than POSIX's three: with `IFS` the
	// vertical tab alone, `a\v\vb` is two fields and `\va\v` is one, where
	// dash, zsh and BusyBox ash give three and two. The carriage return and
	// the form feed answer the same way. bash 3.2 is the three, so this is a
	// change within bash and the dialect takes 5.3's.
	s.IFSWhitespaceIsEverySpaceCharacter = interp.Yes
	// A bare `read` hands REPLY the record as it came: a line with two
	// spaces at each end reaches `[$REPLY]` with all four still on it, where
	// ksh93 and zsh trim it the way a named operand's value is trimmed. Both
	// bash columns answer alike.
	s.BareReadTakesTheLineWhole = interp.Yes
	s.BadNameDeclaresTheOperandsAfterIt = interp.No
	s.BadNameEndsTheDeclaringNotTheAssigning = interp.No
	s.SubscriptedOperandTakesTheIntegerAttribute = interp.Yes
	// And the container letter, which this shell takes: `typeset -A m[k]=v`
	// puts `v` under the key `k` and `typeset -a n[2]=v` puts it at index 2.
	s.SubscriptedOperandTakesTheContainerAttribute = interp.Yes
	s.SubscriptedOperandTakesALocalDeclaration = interp.Yes
	// A single subscript on a name that is no array is refused rather than
	// ignored: `a=v; unset "a[1]"` says `a: not an array variable` and fails,
	// where the other shell that reads a subscript as an element says
	// nothing. `a[0]` names the string itself and takes the whole name away
	// in both — measured on 5.3.15. 3.2.57 refuses that one too, so a corpus
	// case here splits the two bash columns; this column is the graded one.
	s.UnsetSubscriptOnAScalarIsAnError = interp.Yes
	// `unset a[@]` empties the array: `a=(x y z)` comes back with no elements
	// and `${#a[@]}` is 0. Measured in both bash builds, and the same for
	// `a[*]`. A name that holds a scalar is refused rather than emptied, and
	// one that holds nothing at all is quietly left alone.
	s.UnsetArraySpan = interp.UnsetArraySpanRemovesTheElements
	// Brackets with nothing between them are named and then answered: the
	// subscript is reported and the operand is zero, and the script carries
	// on at status 0. Measured 2026-09-10 in 5.3.15 and in 3.2.57, and it
	// does not depend on the name — declared, undeclared, a table, an array
	// and a scalar all report alike, which is what separates this from zsh's
	// answer and is why the axis is not a rule about arrays.
	//
	// The real shell writes the sentence *twice* for one subscript, in both
	// builds; that is an artifact of evaluating the word twice rather than a
	// fact about the construct, and once is what this writes.
	s.EmptyArithSubscript = interp.EmptyArithSubscriptIsReported
	// The same text where a *parameter expansion* reads it, and a different
	// answer: `${a[]}` is refused outright — `[${a[]}]: bad substitution`,
	// naming the quoting run — where the arithmetic site reports and carries
	// on with zero. Identical in 3.2.57 and under argv[0] of `sh` (#1763).
	s.EmptyParamSubscriptIsAnError = interp.Yes
	// An empty key is refused rather than stored: measured 2026-09-12 with
	// `declare -A m`, `m[""]=4` and `m[$w]=4` with an empty `$w` are both
	// `bad array subscript`, the table is untouched and the status is 1,
	// where ksh93 and zsh store. `m[ ]=7` is the control and stores under
	// one space in every column, so this is emptiness and not blankness
	// (#1938).
	s.EmptyAssociativeKeyIsAnError = interp.Yes
	// Neither array letter may take a name that is already the other kind:
	// measured 2026-09-12, `typeset -A h; h[k]=v; typeset -a h` is
	// `typeset: h: cannot convert associative to indexed array`, the table
	// is untouched, the status is 1 and the next command runs — and the
	// reverse direction is the same refusal in the other words. Identical
	// under argv[0] `sh`; 3.2.57 has no `-A` to reach it (#1375).
	s.TableUnderAnArrayDeclaration = interp.CompoundKindChangeRefused
	s.ArrayUnderATableDeclaration = interp.CompoundKindChangeRefused
	// The literal form is refused too, and it costs more: the complaint
	// names no builtin and the rest of the command list does not run.
	// Measured 2026-09-12 with `typeset -A h; h[k]=v` in front of it and the
	// commands newline-separated, `typeset -a h=(x)` is `h: cannot convert
	// associative to indexed array` at 1 with the table intact and the next
	// *line* running; the same four commands under `;` print nothing after
	// the complaint, by either invocation route (#2287).
	s.TableUnderAnArrayLiteralDeclaration = interp.CompoundKindChangeAbandonsTheLine
	s.ArrayUnderATableLiteralDeclaration = interp.CompoundKindChangeAbandonsTheLine
	// `a[@]=Z` over an indexed array is a bad subscript, and it costs the
	// command list rather than the input: measured 2026-09-12, `x=(p q);
	// x[@]=Z; echo "st=$?"; echo after` prints only `x[@]: bad array
	// subscript` where the same commands on separate lines print `st=1` and
	// `after`. Over a *table* the same spelling is an ordinary key and is
	// taken silently at 0, which is this shell swapping sides with zsh
	// between the two questions (#2285).
	s.WholeArraySubscriptAssigningAnArray = interp.WholeArraySubscriptIsABadSubscript
	s.WholeArraySubscriptAssigningATable = interp.WholeArraySubscriptIsAnOrdinaryKey
	// And written as a key in a table literal it is an ordinary key too:
	// measured 2026-10-15, `declare -A a; a=([k]=1 [*]=s)` lists
	// `(["*"]="s" [k]="1" )` at 0, and `[@]=` likewise (#6686).
	s.WholeArraySubscriptKeyInATableLiteral = interp.WholeArraySubscriptIsAnOrdinaryKey
	// Through a reference the same table refuses, where the bare form above
	// stores a key. Measured 2026-09-20; see
	// Semantics.WholeArraySubscriptThroughAReferenceToATable.
	s.WholeArraySubscriptThroughAReferenceToATable = interp.WholeArraySubscriptIsABadSubscript
	// And a *read* whose key comes out empty is reported too, with a
	// different subject and a different outcome: measured 2026-09-12,
	// `typeset -A m; m[k]=v; w=; ${m[$w]}` writes `m: bad array subscript` —
	// the name alone — and still answers the empty string at status 0, once
	// per read. `${m[ ]}` is the control and says nothing, so this is
	// emptiness rather than blankness (#1972).
	s.EmptyAssociativeKeyIsReportedWhenRead = interp.Yes
	// And the *length* of that same element is refused outright, which is a
	// third answer to one emptiness rather than a louder second: measured
	// 2026-09-12, `typeset -A m; m[k]=v; w=; echo "[${#m[$w]}]"; echo after`
	// writes `[$w]: bad array subscript` — the subscript as written, brackets
	// and all, with no name in front of it — leaves 1 behind and runs neither
	// the `echo` it is in nor the one after it, where ksh93 and zsh both
	// print `[0]` and `after` at 0. The length operator alone: every other
	// operator over the same emptiness takes the read's report and its 0. A
	// name the letters merely declared is not this either — `typeset -A m;
	// ${#m[$w]}` is a silent `0` where `typeset -A m; m=()` in front of it is
	// refused (#2286).
	s.EmptyAssociativeKeyRefusesTheLength = interp.Yes
	// `$@` with no positional parameters is **unset** here, so a colon-less
	// conditional fires: measured 2026-09-12 after `set --`, `"${@-word}"`
	// is `word` and `"${@+word}"` is empty in 5.3.15, 3.2.57 and as `sh`,
	// where dash and zsh answer the other way round (#1941).
	s.PositionalListWithNoneIsSet = interp.No
	// See interp.Semantics.SubstitutionRunsTheBodyReadWithItsLine, measured
	// 2026-10-03.
	s.SubstitutionRunsTheBodyReadWithItsLine = interp.No
	// And an empty expansion written beside a `$@` that produced nothing does
	// not bring the word back, on either side: `set --; e=; f "$e$@"` and
	// `f "$@$e"` are both no argument at all, where `f "x$@"` is one.
	s.EmptyListTakesTheWord = interp.EmptyListReachTheWord
	// The prefix is checked before the command's values are expanded and
	// before its redirections are opened, and this shell is alone in it.
	// Measured 2026-09-12 with `readonly x=1`: `x=$((1/0)) /bin/echo RAN`
	// says `x: readonly variable`, prints `RAN` and never mentions the
	// division, and `x=2 /bin/echo RAN >/nope/f` names the name and then the
	// file where dash, ksh93 and zsh name the file alone (#1943).
	s.PrefixToAFrozenNameIsCheckedFirst = interp.FrozenPrefixCheckedFirst
	// And the *value* the frozen name was being given is never evaluated
	// here either: `readonly r=1; r=$((1/0)) echo RAN` is the refusal and
	// nothing about the division, where the other four columns write the
	// division and never the name (#4685).
	s.FrozenPrefixIsCheckedBeforeItsValue = interp.Yes
	// The brackets of an arithmetic subscript hold an expression, and `*` is
	// not one: measured 2026-09-11 on 5.3.15, `typeset -A m; m[k]=9;
	// $(( m[*] ))` is 0, where the expansion `"${m[*]}"` is 9.
	s.ArithWholeArraySubscriptIsTheSlice = interp.No
	// It is *reported* instead, and the expression carries on with zero:
	// measured 2026-09-12, `a=(3 4 5); $(( a[*] ))` writes `a[*]: bad array
	// subscript` and answers 0 at status 0, and `$(( a[*] + 1 ))` on `(3)`
	// answers 1. Identical in 3.2.57 and under argv[0] `sh`, and the write
	// reports too — `(( a[*] = 5 ))` says it once and stores nothing. An
	// association is silent and zero here and never reaches the question
	// (#1978).
	s.ArithWholeArraySubscriptIsReportedAsBad = interp.Yes
	// A subscript whose quotation never closes is a bad subscript here, not
	// a key found by scanning again with the quoting ignored. Measured
	// 2026-09-20 on 5.3.20 from a script file, `typeset -A a; k="q'r";
	// a[$k]=4; let "++a[$k]"` writes `a[q'r]: bad array subscript` twice
	// and leaves the element at 4, where ksh93u+ and zsh 5.9.2 both answer
	// 5. `(( a[$k]++ ))` beside it is 5 in every column, which is the
	// control that says this is the already-expanded operand and not the
	// key, and `shopt -s assoc_expand_once` moves this column to 5 too —
	// see interp.Runner.ExpandsAnOperandsSubscriptAgain (#3796).
	s.ArithSubscriptQuotationMustClose = interp.Yes
	// And a subscript that arrived already word-expanded is the same
	// quoting context a written one is here, which is the half ksh93 does
	// not share: measured 2026-09-20 on 5.3.20 from a script file with
	// `typeset -A a; a[k]=1; a['"k"']=2`, both `(( ++a["k"] ))` and
	// `let '++a["k"]'` leave the bare `k` at 2 and the quoted key at 2,
	// where ksh93u+ answers the first the same way and the second by
	// incrementing the three-character key instead. `let '++a[\k]'` is the
	// bare key here too, so it is quote removal rather than one character.
	// The same holds for brackets that came out of a value — `e="m[q'r'z]";
	// (( $e = 42 ))` is the key `qrz` here and `q'r'z` there — which is why
	// the axis is about arrival and not about the builtin (#3871, #3917).
	s.ArrivedSubscriptIsAQuotingContext = interp.Yes
	// An apostrophe a script *writes* inside a subscript stops the
	// expansion it holds here, alone in the panel: measured 2026-09-20 on
	// 5.3.20 from a script file, `typeset -A m; kq=q; (( m['$kq'] = 42 ))`
	// leaves the three-character key `$kq`, where ksh93u+ stores under `q`
	// and zsh 5.9.2 under `'q'`. `(( m["$kq"] = 42 ))` beside it is `q` in
	// every column, which is the control that says this is the apostrophe
	// and not quoting in general (#3942).
	s.WrittenSubscriptQuotationStopsItsExpansion = interp.Yes
	// So no key here ends at such a run: there is no expansion performed in
	// one for it to end at. Measured in the same run, `(( m[q'$kq'z] = 42 ))`
	// is the whole of `q$kqz` here where ksh93u+ 2012-08-01 keeps only `q`
	// (#3968).
	s.SubscriptQuotationEndsTheKey = interp.No
	// A subscript that *expanded* to nothing is the expression that is zero,
	// so `${a[$w]}` with an empty `$w` is element zero — measured 2026-09-11
	// on 5.3.15, `a=(5 6 7); w=; ${a[$w]}` is `5` at status 0, and `${a[ ]}`
	// beside it is too.
	s.EmptySubscriptTextIsAMathError = interp.No
	// A literal's own reading of the same empty subscript, measured 2026-10-16
	// (#6738): see interp.Semantics.EmptySubscriptInAnIndexedLiteralIsBad.
	s.EmptySubscriptInAnIndexedLiteralIsBad = interp.Yes
	// A literal whose element was refused where it would go keeps what it had
	// placed in front of it: measured 2026-10-30, `b=(7); b=(1 [-5]=x 3)`
	// leaves `([0]="1")` (#6852). See
	// interp.Semantics.RefusedPlacementKeepsWhatTheLiteralPlaced.
	s.RefusedPlacementKeepsWhatTheLiteralPlaced = interp.Yes
	// An appending literal's bare elements go after the last element even
	// where the literal holds a subscripted one: measured 2026-11-01, `b=(7 8
	// 9); b+=(x [4]=y)` is `([0]="7" [1]="8" [2]="9" [3]="x" [4]="y")` on
	// 5.3.20 and 3.2.57 alike (#7079). See
	// interp.Semantics.AppendingSubscriptedLiteralStoresFromTheFirstPosition.
	s.AppendingSubscriptedLiteralStoresFromTheFirstPosition = interp.No
	// The comma inside a subscript is the arithmetic operator here, as it is
	// everywhere else: measured 2026-09-12, `a=(p q r s); i="2,3";
	// ${a[$i]}` is the *third* element, which is the operator's right
	// operand. This dialect has no ranges for the question to be about
	// (#2160).
	s.SubscriptExpressionStopsAtASeparator = interp.No
	// And at a byte the arithmetic cannot read (#5567).
	s.SubscriptExpressionStopsAtAnUnreadableByte = interp.No
	// Whitespace between the brackets is not that text and is not answered
	// by it: measured 2026-09-10 in 5.3.15 and in 3.2.57, `a=(1 2 3); echo
	// $(( a[ ] ))` is `1` with a clean stream — the blank expression is
	// zero, and zero names the first element — where `a[]` one character
	// shorter writes `a[]: bad array subscript` and answers with a flat
	// zero. The same split holds with the subscript as an assignment
	// target: `(( a[ ] = 9 ))` writes element zero (#1762).
	s.BlankArithSubscriptIsTheEmptyExpression = interp.Yes
	// `a[1]=(p q)` is refused and the script ends: measured 2026-09-07 in
	// 5.3.15 and in 3.2.57, which give the same sentence at status 1 and run
	// nothing after it. It does not depend on what the name holds — an array,
	// a scalar, a declared table and an unset name are all refused alike —
	// and it does not evaluate the subscript, which is quoted back as
	// written (#1330).
	s.SubscriptedArrayLiteral = interp.SubscriptedArrayLiteralRefused
	// A subscript that will not evaluate gives up the *command* here — the
	// rest of its line, and the enclosing function, list, `if`, loop or
	// subshell whole — and the script carries on at the next top-level
	// command with 1 behind it. Measured 2026-09-17 at all five sites; it
	// used to say "ends the script", which is what a probe written inside
	// `( … )` or under `-c` sees and is not what a script sees (#3485).
	s.BadSubscriptToUnset = interp.BadSubscriptAbandonsTheCommand
	// And a `test -v` operand's: see interp.Semantics.BadSubscriptToTestIsSet
	// (#5590).
	s.BadSubscriptToTestIsSet = interp.BadSubscriptAbandonsTheCommand
	// And the store a `read 'r[…]'` or a `printf -v 'r[…]'` operand walks
	// into gives up exactly as much, which is the half zsh answers
	// differently.
	s.BadSubscriptToAnOutputOperand = interp.BadSubscriptAbandonsTheCommand
	// And a declaration gives up exactly as much again — `declare 'a[b c]'=v`,
	// `typeset` and `local`, measured 2026-09-17 at the top level, in a
	// function, in an `&&` list, in an `if` condition, in a loop body, inside
	// `( … )` and inside `$( … )`. This is the site where the panel parts
	// company with bash twice over: zsh and ksh93 both end the script here,
	// where at `unset` both carry on (#3495).
	//
	// `readonly` and `export` never reach it: the bracketed operand is
	// refused one complaint earlier as ``readonly: `a[b c]': not a valid
	// identifier`` at 1, with the rest of the line still running.
	// A subscript inside `(( ))` is given up as the subscript's failure
	// here, which is the rule every other bracketed site follows in this
	// column: measured 2026-09-17, `a=(1 2 3); (( a[b c] )); echo "same=$?"`
	// prints no `same=`, the next line reads 1, and a `-c` string is given
	// up whole. `(( b c ))` — the construct's own arithmetic — is reported
	// and the line carries on, which is the control.
	s.BadSubscriptEscapesAnArithmeticCommand = interp.Yes
	// And so is an expansion that fails in the text of `(( ))`: measured
	// 2026-10-31, `(( $((1/0)) + 1 )); echo "after st=$?"` gives up the
	// line, untraced, and the next one reads 1, as `echo $((1/0))` does —
	// where `(( b = 1/0 ))` is traced and the line carries on. See
	// interp.Semantics.FailedExpansionEscapesAnArithmeticCommand (#7189).
	s.FailedExpansionEscapesAnArithmeticCommand = interp.Yes
	s.BadSubscriptToADeclaration = interp.BadSubscriptAbandonsTheCommand
	// An out-of-range store through each builtin, measured 2026-10-16 (#6835):
	// see interp.Semantics.OutOfRangeElementToAnOutputOperand.
	s.OutOfRangeElementToAnOutputOperand = interp.BadSubscriptReported
	s.OutOfRangeElementToADeclaration = interp.BadSubscriptReported
	// And a plain `a[]=6` gives up exactly as much again — the rest of the
	// line, the enclosing function or list with it, and 1 at the next
	// top-level command. Measured 2026-09-20 on 5.3.20 from a script file:
	// `a=(1 2 3); a[]=6; echo same-line` writes `a[]: bad array subscript`,
	// no `same-line`, and the array is still three long (#3949).
	s.EmptySubscriptToAnAssignment = interp.BadSubscriptAbandonsTheCommand
	// And with no value the brackets are never read at all, which is why the
	// axis above is reachable here only through an operand carrying one:
	// measured 2026-09-17, `a=(1 2 3); declare 'a[b c]'` is silent at 0 and
	// the array is untouched, `declare 'a[9]'` leaves it three long, and
	// `i=0; declare 'a[i++]'` leaves i at 0. The brackets say the *name* is
	// an array and nothing more — `declare a[3]` is `declare -a a` with
	// `${#a[@]}` at 0 (#1380).
	s.ValuelessSubscriptedOperand = interp.ValuelessSubscriptedOperandDeclaresTheName
	// But a name it has never heard of takes its brackets with it: `unset a
	// "a[x+]"` is silent at 0 here as in zsh, and `i=0; unset "nodecl[i++]"`
	// leaves i at 0. bash 3.2 agrees, so this is not a version split.
	s.UnsetSubscriptSkippedWhenNameUnset = interp.Yes
	// A failed operand's status is kept for the whole builtin rather than
	// overwritten by a later one. The subscript route cannot ask it — a bad
	// expression ends the command list here — but the readonly route can:
	// `readonly r=1; x=1; unset r x` is 1 in either order.
	s.UnsetStatusIsTheLastSubscripts = interp.No
	// An element write over a name holding a string promotes the string
	// first, so a subscript counting back from the end finds the element it
	// just made: `a=abc; a[-1]=x` writes the first one.
	s.NegativeSubscriptCountsOverAPromotedScalar = interp.Yes
	// A negative subscript that counts back past the first element is
	// refused rather than placed in front of it, and the refusal ends the
	// script. Measured in both bash builds.
	s.NegativeSubscriptPastTheStartInserts = interp.No
	s.SubscriptBeforeTheFirstElementRead = interp.SubscriptBeforeStartIsReported
	s.SubscriptBeforeTheFirstElementNeedsAnElement = interp.No
	// And the *length* of that element is a second answer here, which no
	// other column has: measured 2026-09-18, `a=(x y z); echo
	// "[${#a[-4]}]"; echo after` writes `[-4]: bad array subscript` and then
	// `after`, with the `echo` abandoned — where the read one line up names
	// the array, expands to nothing and carries on. Same rows for `a=()` and
	// for `a=x`; a name holding nothing at all is `[0]` and silent, which is
	// the length being answered before the subscript is looked at (#3591).
	s.SubscriptBeforeTheFirstElementRefusesTheLength = interp.Yes
	s.OperandSubscriptQuoting = interp.OperandSubscriptEveryQuote
	s.ArithmeticOnlyBodyIsAnArithmeticExpansion = interp.No
	// `declare a=1; declare a+=2` is `12`: a declaration's operand carries
	// the append operator here, where the other three refuse the name `a+`.
	s.DeclarationTakesAnAppendOperand = interp.Yes
	// And with a subscript on the name too: `typeset -a a; a[1]=p;
	// typeset a[1]+=q` leaves `pq` in the element. The one utility that
	// does not reach it is `export`, which refuses the brackets
	// themselves — see DeclarationTakesASubscript.
	s.DeclarationTakesASubscriptedAppendOperand = interp.Yes
	// And the same operator on an *array literal* operand, which this shell
	// takes too: `typeset u+=(3 4)` is silent at status 0 here where bash
	// 3.2.57 refuses the name `u+` it leaves behind.
	s.DeclarationTakesAnAppendingArrayOperand = interp.Yes
	// A `jobs` listing: which end it starts from, and whether a job that
	// has already ended appears in it at all.
	s.JobsListNewestFirst = interp.No
	// A stopped job keeps the current-job marker: measured 2026-09-12
	// through a pseudo-terminal, `sleep 40` stopped with ^Z and then
	// `sleep 41 &` lists `[1]+  Stopped` and `[2]-  Running`, and `jobs %+`
	// and `jobs %-` name those same two. bash 3.2.57 agrees. With two jobs
	// stopped and a third backgrounded it is `[1]-  [2]+  [3]` — the second
	// stopped job current, the first its runner-up, and the background job
	// unmarked at all, which no reading of the table's order produces.
	s.StoppedJobTakesTheCurrentJobMarker = interp.Yes
	// A subshell's first job is `[1]` and takes the `+`, measured with
	// `sleep 3 & ( sleep 2 & jobs )` — the answer three of the four columns
	// give, zsh being the one that does not.
	s.SubshellIsAJobInItsOwnTable = interp.No
	s.JobsListFinishedJobs = interp.Yes
	s.EndedJobIsListedAsRunningWithoutTheMonitor = interp.No
	// A notice never names the pid here and there is no option that asks
	// for one: measured 2026-09-25 on a pseudo-terminal under
	// `bash --norc --noprofile -i` with `set -m`, a finished `&` job is
	// `[1]+  Done                       sleep 0.2` and nothing else.
	s.JobNoticeNamesThePID = interp.No

	// `jobs`' letters. bash has the widest set in the panel: POSIX's `-l`
	// and `-p`, the state filters `-r` and `-s`, `-n` for what has changed
	// since it last said, and `-x` — which is not a listing at all but a
	// command to run with its job specs replaced by process ids. The last
	// two ride UnimplementedOptionLetters and are refused by name.
	s.JobsOptions = "lprs"
	// `jobs -p` here is the process ids and nothing else, which is what
	// makes `kill $(jobs -p)` mean what it is written to mean.
	// unanswered JobsListsWhatChangedSinceTheLastReport: `jobs -n` is not a
	// letter this shell has — it is not in JobsOptions — so the axis is
	// never consulted here. ksh93 is the one column with the letter, and
	// bash's letter of the same name is a different question that stays
	// unimplemented (#3390).
	s.JobsPidsOnlyOption = interp.Yes
	s.JobsPidsLetterOutranksTheLongLetter = interp.No
	// With both filters at once the last letter given decides, so
	// `jobs -rs` lists the stopped jobs and `jobs -sr` the running ones.
	s.JobsStateFiltersAccumulate = interp.No

	// Whether a `&` job's command appears in a `jobs` listing.
	s.JobsShowBackgroundCommand = interp.Yes
	// Measured 2026-10-01: `(exit 4) &` lists as `( exit 4 ) &`. See
	// Semantics.JobCommandIsReprinted.
	s.JobCommandIsReprinted = interp.Yes
	// `${v/#%pat/rep}`: the two anchors together, or a start anchor over a
	// `%`. Measured 2026-10-02. See Semantics.ReplacementAnchorsCombine.
	s.ReplacementAnchorsCombine = interp.No

	// Whether a backgrounded job is announced to whoever is typing.
	// Whether `export -f` carries a function to a child.
	// Whether an unassigned subscript is an element.
	s.ArraysAreSparse = interp.Yes
	// An operator on `${a[*]}` trims each element before the join here.
	s.OperatorDistributesOverStarSubscript = interp.Yes
	// A *replacement* operand is a word of its own, so its quotes quote and
	// are removed: measured 2026-09-07 in 5.3.15 and as `sh`, `"${s/a/'$v'}"`
	// on `s=xay` and `v=VAL` is `x$vy`, the `$v` never substituted. 3.2.57
	// gives `x'VAL'y`, so this moved inside the lineage and the preset is the
	// current build. The *word* operand takes the enclosing quoting in both,
	// unanimously with the rest of the panel, which is why that one is not an
	// axis (#1209).
	s.ReplacementOperandTakesTheEnclosingQuoting = interp.No
	s.ExportCarriesFunctions = interp.Yes
	// The two letters that are attributes of a *function* here, in the order
	// a listing writes them — `declare -frx d` for one that is both. bash is
	// the only shell in the panel with the notion at all: zsh reads `-F` as a
	// float's precision and refuses `export -f`, and ksh93, dash and BusyBox
	// ash each end the script over `readonly -f`. Measured 2026-09-16 on
	// 5.3.20 and 3.2.57, which agree line for line (#3192). See
	// Semantics.FunctionAttributeLetters for what the letters do.
	//
	// `t` joined the two in #3051. It is the trace mark, measured on
	// 5.3.20 the same day as the other two: `declare -ft a` is a silent 0,
	// `declare -F` writes `declare -ft a`, `declare -Ft` narrows to the
	// marked functions, and a function carrying all three lists as `declare
	// -frtx a`. What a traced function *does* here — inherit the DEBUG and
	// RETURN traps, which `set -o functrace` asks for wholesale — is not
	// built, so the mark is recorded and listed and read by nothing.
	//
	// And the three case letters, after the other three and in that order,
	// one at a time since each takes the others off: measured 2026-10-03,
	// `typeset -fu f` is a silent 0 and `declare -F` then writes `declare
	// -fu f`, and a function marked `-fx`, `-fl`, `-ft` lists as `declare
	// -ftxl f`.
	s.FunctionAttributeLetters = "rtxluc"
	// The body listing writes a line of those letters after the body: see
	// interp.Semantics.FunctionBodyListingWritesItsAttributes (#5674).
	s.FunctionBodyListingWritesItsAttributes = interp.Yes

	// A call does not put the trace back here: measured 2026-09-28 from a
	// script file under `env -i PATH=/usr/bin:/bin`, `f() { set -x; }; f;
	// echo after` traces `echo after` in bash 5.3.20 and bash 3.2.57. One column in the panel
	// restores it and this is not that column — see
	// Semantics.FunctionCallRestoresTheTrace.
	s.FunctionCallRestoresTheTrace = interp.No
	// `export -n V` takes the attribute off and leaves V set: measured, the
	// name keeps its value in the shell and stops reaching a child. bash is
	// the only shell in the panel with the letter.
	s.ExportTakesTheAttributeOff = interp.Yes
	s.AnnouncesBackgroundJob = interp.Yes
	// And it keeps announcing with the monitor off, which is measured
	// 2026-09-10 on a pseudo-terminal: `set +m` then a background job still
	// prints `[1] <pid>` here where zsh goes quiet. The other end of the job
	// is silent in every column (#1738).
	s.AnnouncesBackgroundJobWithoutTheMonitor = interp.Yes
	// And bash alone leaves a hole a hole. Measured 2026-09-12 with three
	// jobs started and the middle one killed and reaped, `jobs %2` is still
	// no-such-job afterwards and the new job answers `%4` — where dash,
	// ksh93, zsh and ash all put it back in `%2`.
	s.NextJobNumberRefillsAHole = interp.No
	// bash 5.3 leaves the monitor off under `-i script.sh` with no terminal,
	// and says so twice — `cannot set terminal process group` and `no job
	// control in this shell`. It grants an explicit `set -m` there all the
	// same, which is what keeps this question and MonitorNeedsATerminal
	// apart.
	s.InteractiveMonitorNeedsATerminal = interp.Yes
	// A subshell is a process of its own here — measured, a child started
	// inside one reports the subshell rather than the shell as its parent —
	// so a signal aimed at `$$` never reaches it and it finishes its body.
	s.SubshellRunsOnAfterSignalingTheShell = interp.Yes
	// And with the signal trapped: `trap 'echo T' USR1; (kill -USR1 $$; echo
	// in); echo "after $?"` writes `in`, `T` and `after 0`. Measured
	// 2026-11-01 on bash 5.3.20 (#7168).
	s.SubshellEndsWhenItSendsTheShellATrappedSignal = interp.No
	// And bash is the one member of the panel that announces nothing on
	// this route. Measured on `-i script.sh` through a pseudo-terminal:
	// 5.3.15, 3.2.57 and 3.2 run as `sh` all print neither the start nor
	// the `Done` row, where the other three print at least one. It is the
	// route and not the terminal — `bash -i < script` announces both.
	s.InteractiveScriptAnnouncesJobs = interp.No
	// The other route is the other way round. Measured 2026-09-12 on `-i -c`
	// through a pseudo-terminal, with the job held open on a fifo the string
	// releases and then reaps: bash writes `[1] <pid>` where dash and ash write
	// nothing at all. So the two columns disagree and this is a second field.
	s.InteractiveCommandStringAnnouncesJobs = interp.Yes
	// And bash alone keeps the `Done` row back for a prompt. On that same
	// `-i -c` run 5.3.15 writes the start and never the end — with `wait`, with
	// `wait %1`, and with a whole second of `sleep` after the job died — while
	// the same program on a pipe, where `-i` draws a prompt between the lines,
	// writes `[1]+  Done` at the prompt after `wait`. It is the prompt and not
	// the route. 3.2.57 writes the row on both, which is why this is a field:
	// without it bash would have no single answer to give.
	s.FinishedJobNoticeNeedsAPrompt = interp.Yes
	// And when that prompt comes, the row is written *before* it rather than
	// at the moment the job ended. Measured 2026-09-25 on a pseudo-terminal,
	// `sleep 0.4 &` and then nothing: 5.3.20 and 3.2.57 both stay silent
	// until `echo AFTER` has run and put the row after its output.
	//
	// It is the default and not the whole of what this shell can do: `set -b`
	// is bash's own spelling of zsh's `NOTIFY`, and with it the same job is
	// reported at once — measured the same day on 5.3.20. The letter is not
	// wired to this axis yet, so what is answered here is where the shell
	// starts (#4524).
	s.FinishedJobNoticeArrivesAtOnce = interp.No
	// `$!` before any background command is *unset*, not set and empty:
	// measured, `${!-unset}` takes its default and `${!+set}` is empty, and
	// `set -u; echo "[$!]"` writes `$!: unbound variable` and stops at 127 in
	// 5.3.15, in 3.2.57 and in 3.2 run as `sh`. Without `set -u` it expands
	// to nothing, which is three other columns' answer too, so the noisy half
	// only shows where a script asked to be told.
	s.LastBackgroundPid = interp.LastBackgroundPidUnset
	// A job started with `&` reads an empty standard input, not the shell's:
	// measured 2026-09-07, `bash -c '/bin/cat & wait; echo ---; /bin/cat' < f`
	// writes `---` and then the file's line, in 5.3.15 and 3.2.57 alike. What
	// POSIX XCU 2.9.3 specifies, and what keeps the script's own `read` from
	// losing the lines a background job would otherwise eat.
	s.BackgroundJobInput = interp.BackgroundJobInputEmpty
	// And the substitution reaches the shell's **own** input and nothing a
	// script put there: `for … done < names` with a `{ read line; … } &`
	// inside it reads the file here, where an empty input would print blanks
	// at status 0. Measured 2026-09-23 on 5.3.20 and 3.2.57, which agree on
	// this row; see interp.Semantics.BackgroundJobInputIsOnlyTheShellsOwn for
	// the five shapes and for the pipeline row the two bash columns part on
	// (#4153).
	s.BackgroundJobInputIsOnlyTheShellsOwn = interp.Yes
	s.ProcessSubstitutionIsTheLastBackgroundJob = interp.Yes
	s.ReportsACommandKilledBySignal = interp.Yes
	s.ReportsAnyKilledPipelineElement = interp.No
	s.ChildInterruptEndsTheScript = interp.No
	// `cd ""` is refused where `HOME=` is not, which is the pair no single
	// field could express: `cd: null directory` at 1 for the operand, and
	// nothing at all at 0 for the empty HOME.
	s.CdEmptyOperandIsAnError = interp.Yes
	s.CdEmptyHomeIsAnError = interp.No
	// No substitution form here: a second operand is simply one too many,
	// and it is refused rather than ignored. Measured, `cd a b` says `cd:
	// too many arguments` and stays where it is.
	s.CdSubstitutesTheOperands = interp.No
	s.CdSubstitutionPrintsTheDirectory = interp.No
	s.CdRefusesExtraOperands = interp.Yes
	s.CdRefusesUnknownOption = interp.Yes
	s.CdHasQuietOption = interp.No
	s.CdHasSymlinkFreeOption = interp.No
	s.CdLastPathOptionWins = interp.Yes
	s.BadSetOptionNameFatal = interp.No
	// Neither spelling ends the script here: `set -o zzznosuch; echo one;
	// set -Z; echo two` prints both words and leaves at 0. Written rather
	// than inherited so that the agreement between the two is a measured
	// answer — the substrate assumed it until #2629 and was wrong about
	// BusyBox ash.
	s.BadSetOptionLetterFatal = interp.No
	// The one column in the panel with a POSIX mode, and it moves both:
	// `set -o posix; set -o zzznosuch; echo "st=$?"; echo after` prints the
	// complaint and nothing else at 2, and the same line with `set -Z` does
	// too. Measured 2026-09-14 on 5.3.15, and the `sh` name is the other
	// door to the same place — `bash-as-sh` stops for both spellings where
	// `bash` stops for neither. `set +o posix` puts the two answers above
	// back (#2641).
	//
	// bash 3.2.57 takes the letter and leaves the name — `set -o posix; set
	// -o zzznosuch` is still 1 and still carries on there, under `-o posix`
	// and under the `sh` name alike. This preset claims 5.3 and says so
	// here, because a reader arriving from macOS's /bin/bash would otherwise
	// read this as a claim about their shell.
	// A function named after a special builtin is refused in POSIX mode and
	// defined outside it — and the check is at the **definition**, so a
	// script may enter the mode from inside the same input and be refused by
	// it: `bash -c 'set -o posix; export() { :; }; printf b'` writes
	// ``export': is a special builtin`` and prints nothing. Measured
	// 2026-09-18 a name at a time against the roster: all sixteen refuse,
	// `.`, `:` and `source` included, while `local`, `true`, `read`, `cd`,
	// `echo`, `alias` and `pwd` define at 0 — so it is this shell's list of
	// special builtins and not a table of its own, unlike ksh93's and
	// dash's, which are the parser's (#2987).
	s.SpecialBuiltinNameIsNotAFunctionNameInPosixMode = interp.Yes
	// And in that mode a special builtin the shell already holds is reached
	// **before** a function of the same name — the order XCU 2.9.1.1 sets out
	// — in the search and in what the name-reporting builtins say. Measured
	// 2026-09-23 on 5.3.20, the function defined before the mode is entered
	// because the mode refuses the definition itself. See
	// Semantics.SpecialBuiltinOutranksAFunctionInPosixMode, where bash 3.2 is
	// the row that says the two faces can come apart.
	s.SpecialBuiltinOutranksAFunctionInPosixMode = interp.Yes
	// And the mode takes the loop-control sentence away: `break` with no loop
	// around it names the three loops under bash's own name and says nothing
	// at all under `set -o posix` or under the name `sh`, status 0 either way.
	// Measured 2026-09-23 on 5.3.20 and 3.2.57 alike, so it is the mode and
	// not the build. See Semantics.LoopControlOutsideALoopSilentInPosixMode,
	// where zsh-as-`sh` is the row that keeps this off the core.
	s.LoopControlOutsideALoopSilentInPosixMode = interp.Yes
	s.BadSetOptionNameFatalInPosixMode = interp.Yes
	s.BadSetOptionLetterFatalInPosixMode = interp.Yes
	// `-o` takes the next word and never the rest of its own: measured,
	// `set -oe x` is `x: invalid option name` with errexit left off, and
	// `bash -oerrexit -c cmd` refuses `-c` the same way. With no word behind
	// it, `set -oe` writes the option table and then turns errexit on.
	s.SetOLetterAttachesItsName = interp.No
	// Measured 2026-09-16, bash 5.3.20 and 3.2: `bash --xtrace -c 'echo ran'`
	// is `--xtrace: invalid option` followed by the table of the seventeen
	// GNU long options it does have, and nothing runs. A `set -o` name is not
	// among them, so a `--name` word here is a name bash looks up in a table
	// of its own rather than in the option namespace.
	s.LongOptionNamesASetOption = interp.No
	// The two of those seventeen that are `set -o` names under another
	// spelling. Measured 2026-10-03 on 5.3.20: `--posix` is posix on and a
	// later `set +o posix` turns it off, and `--verbose` answers `hvBc`.
	s.LongOptionsNamingSetOptions = "posix verbose"
	// And `-o` with nothing behind it is the listing, written in the order
	// the bundle was read — `bash -cecho hi` lists errexit and hashall on and
	// then refuses the space. Measured 2026-10-03 on 5.3.20.
	s.InvocationBareOListsTheOptions = interp.Yes
	// Which is refused at 1 and with no usage block, because errexit was
	// already on when the space was read: `bash -e -q` is the same, `bash -q
	// -e` is the usage error at 2. Measured 2026-10-03 on 5.3.20.
	s.InvocationLetterRefusedUnderErrexitFails = interp.Yes
	// unanswered LongOptionNameIgnoresHyphens: the fold is a rule of the
	// `--name` spelling, and the axis above says this shell has no such
	// spelling. There is no site here to put the question to.
	// unanswered LongOptionValueIsANumber: the `=value` it reads rides on a
	// `--name` option word, and the axis above says this shell has no such
	// word. There is no site here to put the question to.
	// An option name is read exactly as it is written here — no separators
	// come out of it and no `no` goes in front of it. Measured 2026-09-18 on
	// bash 5.3.20: `set -o err-exit` and `set -o noerrexit` are each
	// `set: <word>: invalid option name` at 2, where `set -o errexit` beside
	// them is 0 (#3155, #3254).
	s.OptionNamespaceIgnoresSeparators = interp.No
	s.OptionNamespaceTakesANoPrefix = interp.No
	// And the next word is only taken when it does not look like options
	// itself: `set -o -e` here writes the option table and turns errexit
	// **on**, where the name that would have been refused is `-e`. Only at
	// the builtin — `bash -o -e -c cmd` on the command line that starts the
	// shell is `-e: invalid option name`, which is the front end's own
	// position-sensitive parse and not this axis.
	s.SetODeclinesADashWord = interp.Yes
	s.SetListsOptionsOnceAtTheEnd = interp.No
	// A bare `-` turns `-x` and `-v` off here, and a bare `+` is consumed with no effect (#2699).
	s.BareOptionWord = interp.BareDashClearsTraceAndVerbose
	// And every option word's letters are read before any of them is
	// applied: `command set -e -Z` here leaves errexit **off**, where dash
	// and BusyBox ash leave it on. The pass that does it knows letters only —
	// `command set -e -o zzznosuch` is errexit on — and a letter welded
	// behind an `-o` is past it, which is why `set -ozzznosuch` reports 1 and
	// carries on where `set -Z` is 2 and ends an `sh` script.
	s.SetValidatesOptionLettersFirst = interp.Yes
	s.BadSetOptionNameAtInvocationExitsZero = interp.No
	s.UnknownConditionOptionIsAStatus = interp.No
	s.ReturnOutsideAFunctionIsRefused = interp.Yes
	// `break` with no loop around it is reported and then ignored here: the
	// next command on the line runs and the status stays 0. Measured with
	// `echo t; break; echo after` — `after` prints and `$?` is 0.
	s.LoopControlOutsideALoopIsFatal = interp.No
	// The place is looked at before the count: `break abc` with no loop to
	// leave names the loops POSIX has and never mentions `abc`, and the
	// script carries on at 0. Identical in 3.2 and, but for the silence,
	// in the same binary called as `sh`.
	s.LoopControlPlaceIsJudgedBeforeTheCount = interp.Yes
	// Both boundaries, and it says so: `f(){ break; }` called from a loop and
	// `( break )` inside one each draw the complaint once per pass and leave
	// the loop running. bash 3.2 answers the opposite way on both, which is
	// why neither is the core's.
	s.FunctionCallIsALoopControlBoundary = interp.Yes
	s.SubshellIsALoopControlBoundary = interp.Yes
	// A startup file is a sourced script, so a `return` in one is accepted
	// everywhere — the split is only over what its argument does. bash
	// discards it: measured through a pty, an rc of `return 3` leaves `$?`
	// as 0 at the first prompt and one of `false; return 3` leaves 1, while
	// `(exit 5)` as the last line leaves 5. So the file's status does
	// carry out and the number on the `return` does not.
	s.StartupFileReturnCarriesItsArgument = interp.No
	// A startup file that runs no command keeps the status before it: measured,
	// an empty `$BASH_ENV` after a `.bash_profile` of `false` leaves 1 for
	// `bash -l -c` (#5883).
	s.StartupFileThatRunsNothingLeavesZero = interp.No
	s.LoneDashIsAnOption = interp.No
	s.UnsetFunctionChecksTheName = interp.No
	s.UnsetFunctionReportsMissing = interp.No
	// A plain `unset NAME` falls through to the function table when the
	// name holds no variable, which is bash's alone: measured 2026-09-16,
	// 5.3.20, the same binary as `sh` and 3.2.57 all answer 127 to a call
	// after `unset b`, where zsh, ksh93u+, dash and BusyBox ash all still
	// run the function.
	s.UnsetReachesTheFunctionTable = interp.Yes

	// Whether a redirection target is expanded as an ordinary word.
	s.RedirectTargetIsAnOrdinaryWord = interp.Yes
	// The target is split and matched, which is the half of the reading
	// above that POSIX forbids — and this is the shell that changes sides
	// with the mode. `set -o posix` and the `sh` name both turn it off:
	// `cat < only-*.txt` becomes `No such file or directory` with the match
	// sitting there, `printf X > only-*.txt` creates a file called
	// `only-*.txt`, and `e="a b"; > $e` writes a file called `a b` rather
	// than being ambiguous. `> {c,d}` stays ambiguous in both modes, which
	// is what keeps this axis apart from the one above. Measured in 5.3.20
	// and 3.2.57, 2026-09-16 (#3207).
	s.RedirectTargetTakesPathnameExpansion = interp.Yes

	// Whether `type --` ends the options.
	// `type .` is `. is a shell builtin` in bash 5.3.20 and in bash 3.2.57
	// alike — one answer across twelve years of this shell. The same binary
	// invoked as `sh` says `a special shell builtin`, which is the POSIX
	// preset's row and not this one.
	s.TypeDistinguishesSpecialBuiltins = interp.No
	s.TypePrintsFunctionBody = interp.Yes
	s.TypeEndsOptionsWithDashDash = interp.Yes
	s.TypeLeadingDashWordAsksForTheBareAnswer = interp.No
	// type's letters, all implemented here: -a for every resolution, -p
	// speaking only where the plain answer would have been a file, -P
	// forcing the PATH search, -f leaving functions out, and -t — the one
	// shell in the panel with it at all — answering one bare word per name
	// and staying silent with status 1 for a name that is nothing.
	s.TypeOptions = "afpPt"
	s.TypePSearchesPathPastTheShell = interp.No
	s.TypePathAnswerIsASentence = interp.No
	s.TypeFSaysTheFunctionBack = interp.No

	// The letters `declare` and `local` read — one set under two names
	// here, with `-F` naming functions rather than setting a float's
	// precision, which no other engine spells this way. `-n` is here since
	// #2553: it is the **name reference**, a language feature rather than an
	// attribute, and interp/nameref.go is the whole of it. `-I` joined them
	// in #3434: it is the per-declaration spelling of `localvar_inherit`,
	// and interp/localinherit.go is the whole of it. `-t` joined them in
	// #3101: it is the **trace attribute**, which this shell records and
	// lists back and nothing else here reads — see attrTraced.
	s.DeclareOptions = "aAcfFgiIlnprtux"
	// unanswered FloatFormatLetterE: this shell has no `-E` on a declaration
	// to give a rendering to. Measured 2026-09-15 on 5.3.15 and 3.2.57
	// alike, `declare -E 3 a=1.5` is `declare: -E: invalid option` (#2559).
	// unanswered DeclareNumberDetachedOnlyAtTheWordEnd: no letter here takes
	// a detached number at all — DeclareOptionsTakingANumber is empty and
	// `-i` takes no base, so `declare -i 8 n=64` is `8: not a valid
	// identifier` — and there is no position for the rule to be about.
	// unanswered BareFloatLetterResetsThePrecision: there is no float
	// precision here to keep or reset; `-F` is the *function* listing under
	// this name, and `declare -F 3 a=1.5` is ``cannot use `-f' to make
	// functions``.
	// unanswered NumericTypeLetterPrecedence: nor can a rank between them be
	// asked, for the same reason — `-F` is the function listing here and
	// there is no second numeric letter to rank against `-i` (#2419).
	// unanswered DeclareHideInScopeLetter: nor a lower-case `h` under either
	// reading. Measured 2026-09-14, `typeset -h s q=1` and `declare -h s
	// q=1` are both `invalid option` with the usage line, so neither the
	// hide-in-scope attribute nor the string argument can be put to it.
	// unanswered SetArrayEmptyPrependMakesANonArrayAnEmptyArray: there is no
	// `set +A` here to write with no values behind it. Recorded in the corpus
	// at `setarray/no-values-and-whether-the-name-survives`: bash 5.3, bash 3.2
	// and bash-as-`sh` each answer `set: -A: invalid option` with the usage
	// line, so the letter is refused before the question of what an empty
	// prepend stores can be put.
	// unanswered ZeroFillKeepsLeadingBlanks, ZeroFillGoesAfterAnIntegersSign:
	// the same wall — there is no zero fill here to ask either question of.
	// unanswered DeclareZeroFillLetter: there is no `Z` letter here under
	// either reading. Measured 2026-09-18 on 5.3.20 and 3.2 alike, `declare
	// -Z 4 d=7` is `declare: -Z: invalid option` with the usage line, so the
	// question of whether it is a justification or a fill cannot be put.
	// unanswered WidthJustificationPrecedence: nor can a pair of them be
	// written to rank — `declare -LR 5 a=7` is the same refusal, at the `L`.
	// unanswered WidthNumberPrecedence: and no width number to rank either,
	// since the letter that would carry one is refused before its digits are
	// looked at (#4827).
	// unanswered WidthLettersExcludeTheIntegerLetter: nor the integer letter
	// beside one, for the same reason (#2859).
	// unanswered NumericTypeLettersAreExclusive: the pair cannot be written.
	// `-E` is not a letter at all and `-F` is the function listing, so
	// `declare -iF a=1` is that same refusal in 3.2 and `-i: invalid option`
	// in 5.3 — neither of them the question this asks.
	// unanswered DeclareMatchingLetter: bash has no `m` letter under either
	// reading. Measured 2026-09-13 on 5.3 and 3.2 alike, `declare -m q=1` is
	// `declare: -m: invalid option` followed by the usage line, so the
	// question of what the letter *means* never arises (#2345).
	// unanswered DeclareMappingLetter: the same, for `typeset -M tolower v`.
	// unanswered DeclareTypeLetter: nor a `T` letter under either reading.
	// Measured 2026-09-13 on 5.3.15 and 3.2.57 alike, `declare -T q=1` is
	// `declare: -T: invalid option` at 2 with the usage line under it, so
	// neither the tie nor the type can be asked for here (#2419).
	// unanswered DeclareHideValueLetter: the same, for `declare -H q=1`.
	// `local -n` is the common spelling of a reference and not an afterthought
	// of `declare -n`: a function taking the name of a variable to fill in is
	// what the letter exists for, and the letter has to be on this word for
	// the fresh binding to be the function's own. `-I` is here for the same
	// reason from the other side: it is what says the fresh binding is *not*
	// the function's own but a copy of the caller's, so it belongs on the
	// word that makes the binding (#3434).
	s.LocalOptions = "aAgiIlnprtux"
	// A bad `declare` option is reported and the script goes on.
	s.TypesetBadOptionFatal = interp.No
	// A listing spells bytes above ASCII out where the locale has no
	// characters there: measured 2026-10-03, `v=é; set` under `LC_ALL=C`
	// writes the bytes escaped and under a UTF-8 locale the character. See
	// Semantics.ListedNonAsciiFollowsTheLocale.
	s.ListedNonAsciiFollowsTheLocale = interp.Yes
	// A declaration's array literal over a frozen name, or over the other
	// kind of array under an explicit letter, is refused ahead of the
	// command and its redirections, as a bare assignment: measured
	// 2026-10-03 on bash 5.3.20. See
	// Semantics.ArrayOperandRefusedBeforeTheCommand.
	s.ArrayOperandRefusedBeforeTheCommand = interp.Yes
	// A plus letter selects nothing in a listing: measured 2026-10-03 on
	// bash 5.3.20, `typeset +x` and `typeset +i` are the bare listing with
	// values, and `typeset +x -i` the integers. See
	// Semantics.PlusLetterSelectsAListing.
	s.PlusLetterSelectsAListing = interp.No
	// And the array literals behind a refused letter are declared and
	// stored anyway: measured 2026-10-03 on bash 5.3.20, `typeset -U
	// a=(1 1 2)` refuses `-U` at 2 and leaves `a` holding all three
	// elements. See Semantics.RefusedDeclarationKeepsItsArrayLiterals.
	s.RefusedDeclarationKeepsItsArrayLiterals = interp.Yes
	// A lone `-` or `+` is a *name* here and not an option word, and not one
	// a script may declare: measured 2026-09-10, ``declare -`` is ``declare:
	// `-': not a valid identifier`` at 1 where zsh and ksh93 both read it as
	// an option word and list. See Semantics.SignAloneIsAnOptionWord.
	s.SignAloneIsAnOptionWord = interp.No
	// Nor on `export` and `readonly`: the sign is a name to them as well,
	// measured on bash 5.3.15 and 3.2.57 alike — ``export: `+': not a valid
	// identifier``.
	s.SignAloneIsAnOptionWordToExport = interp.No
	// `declare +f` takes the function attribute off rather than naming the
	// functions, which leaves the bare `declare` listing — every variable
	// and then every function. This engine has no bare listing for this
	// shell, so the letter goes on writing bodies (#1754). `declare -F` is
	// the names-only spelling here and is unaffected: it has its own letter.
	s.FunctionNamesUnderPlus = interp.No
	// `-i` takes no output base here, which is what separates this shell
	// from the two that have `integer`: measured 2026-09-06, `typeset -i2
	// c=5` is `-2: invalid option` and `typeset -i 16 b=255` calls the `16`
	// a name that is not a valid identifier. So nothing about a base is
	// refused and both words go on meaning what they meant — and there is no
	// `integer` here at all, which is why IntegerOptions stays empty.
	s.IntegerAttributeTakesABase = interp.No
	s.IntegerBaseComesFromTheValueAssigned = interp.No
	s.IntegerBaseNegativeIsTwosComplement = interp.No
	// Answered beside the two rows above rather than because a script can
	// reach it: `-i` takes no base here, so there is no mark to spell.
	s.IntegerBaseMarkIsCSpelled = interp.No
	// Answered for completeness rather than because a script can reach it:
	// there is no base to be the absence of, `-i10` being an invalid option
	// and a bare `10` not a valid identifier.
	s.IntegerBaseTenIsNoBase = interp.No
	// `declare -g x=new` writes the global cell even with a `local x`
	// standing in front of the name.
	s.DeclareGlobalReachesPastALocal = interp.Yes
	// A bare `local` writes the running function's own locals, each as a
	// clustered declaration — `declare -i n`, `declare -- x`.
	// `local -` saves the `set` table for the life of the call: measured
	// 2026-09-15, `f() { local -; set -f; }; f; echo $-` comes back without
	// the letter. The `set` table alone — a `shopt` the body changed
	// survives the return.
	s.LocalDashSavesTheShellOptions = interp.Yes
	s.BareLocalListing = interp.BareLocalListsLocals
	// And `local -p name` answers for the running call rather than for the
	// shell: a name the call did not declare is `local: name: not found` at
	// 1, whatever the shell holds under it. Measured 2026-09-21 — a global
	// and a caller's local are both refused, and the call's own local writes
	// the row `declare -p` writes.
	s.LocalListingIsTheRunningCallsOwn = interp.Yes
	// The *declaration* word lists something else again, and it is this
	// shell's `set` listing exactly: every variable as an assignment, then
	// every function. Measured 2026-09-12 with a scrubbed environment,
	// `diff <(declare) <(set)` empty. It was left unanswered while the two
	// values the form carried were neither of those, so a bare `declare`
	// refused; `declare +f` and `declare +F` are the same listing reached by
	// taking the function attribute off, and had nothing to fall through to
	// (#1754).
	s.BareTypesetListing = interp.BareLocalListsWhatSetLists
	// A bare `set` writes the variables and then every defined function,
	// values quoted only where they must be, `'\''` for an embedded quote
	// and `$'...'` once a control character appears.
	s.SetListing = interp.SetListingAssignmentsThenFunctions
	// The array is why this shell's `coproc` takes a name: the ends arrive in
	// it, and a script writes `echo hi >&"${COPROC[1]}"`.
	s.CoprocEndsInAnArray = interp.Yes
	// And the array goes when the coprocess does. Measured 2026-09-12:
	// `declare -p CP` answers `not found`, both descriptors are gone, and
	// `echo x >&${CP[1]}` is an ambiguous redirect at 1 rather than a write
	// into a pipe nobody is reading (#2411).
	s.ReapedCoprocessEnds = interp.CoprocEndsGoWithTheCoprocess
	// And the ends are put out of the way of the numbers a script allocates
	// for itself. Measured 2026-09-13: four coprocesses in a row publish
	// `63 60`, `62 58`, `61 56` and `59 54`, while `exec {v}>/dev/null` with
	// one running still answers 10 (#2596).
	s.CoprocessEndPlacement = interp.CoprocEndsAtTheTopOfTheTable
	// A process substitution's far end goes to the same region by the same
	// rule, counting down and skipping what is held. Measured 2026-09-21:
	// `echo <(true) <(true) <(true)` is `/dev/fd/63 /dev/fd/62 /dev/fd/61`
	// in 5.3.20 and in 3.2.57, `62 61` with 63 parked, and `3 4` at any
	// `ulimit -n` of 63 or below.
	s.SubstitutionEndPlacement = interp.SubstitutionEndsAtTheTopOfTheTable
	// bash saves the streams a redirected group replaces, once per
	// redirection rather than once per stream and with no copy of a
	// duplication's source: `{ exec {v}</dev/null; echo $v; }
	// 2>/dev/null 2>/dev/null` is 12 in 5.3.20 and `… 2>&1` is 11. Only a
	// `{v}` inside such a group can see it. Measured 2026-10-15 (#6502).
	s.OwnDescriptors = interp.EachRedirectionSavesItsStreams
	// A duplication's descriptor number has no ceiling of this shell's own
	// — `echo x >&99` is the ordinary bad-descriptor sentence (#3210) — the
	// `-v` echo ends a line the input did not end (#3130), and `read -t`
	// takes a written number rather than an expression (#3209).
	s.DescriptorNumberCeiling = interp.NoDescriptorNumberCeiling
	s.VerboseEchoAddsAMissingNewline = interp.Yes
	s.ReadTimeoutOperandIsArithmetic = interp.No
	s.SetListingQuoting = interp.ListingQuoteWhenNeededEscaped

	// `[[ -v 1 ]]` and `[[ -v 0 ]]` ask about a positional parameter here,
	// where ksh93 declines to read a digit as a name at all. The parameters
	// spelled as one punctuation character are the other way round: `[[ -v ?
	// ]]` is unset here and set in zsh. Measured on bash 5.3.15, and it is
	// the operator rather than the lookup — `[[ -n ${?+s} ]]` is set here.
	s.ParameterIsSetSeesPositionals = true

	// And `[[ -v a[k] ]]` reads the subscript the script wrote rather than
	// the one left standing after the operand is expanded, so a key with a
	// bracket in it is found when the bracket was quoted or came out of an
	// expansion. bash is alone in the panel here — see
	// interp.Semantics.ConditionIsSetReadsTheWrittenSubscript, including the
	// row where bash's own `test -v` answers the other way.
	s.ConditionIsSetReadsTheWrittenSubscript = interp.Yes

	// And `[[ a[k] -eq 5 ]]` reads it the same way, which is this shell's
	// alone: ksh93 reads a quotation inside a subscript in `(( ))` and still
	// refuses it here. See
	// interp.Semantics.ConditionArithmeticReadsTheWrittenSubscript (#3302).
	s.ConditionArithmeticReadsTheWrittenSubscript = interp.Yes

	// A subscript that reaches the condition as *text* is expanded once more
	// before it is looked up, so `k='x y'; [[ -v 'm[$k]' ]]` asks about the
	// key `x y` and not about the two characters `$k`. Only where no bracket
	// was written: the axis above has already answered `[[ -v m[$kk] ]]`,
	// where bash stops at the one expansion the written subscript gets. See
	// interp.Semantics.ConditionIsSetExpandsAFlatSubscript (#3298).
	s.ConditionIsSetExpandsAFlatSubscript = interp.Yes

	// And a declaration's operand expands its subscript the same way, in
	// every spelling of the utility: `declare 'd[$k]'=Q` writes the key
	// `x y`, and `declare 'arr[$i]'=Q` writes the element `$i` counts to.
	// This one does not move with `shopt -s assoc_expand_once` — measured
	// 2026-09-19, the key is `x y` with the option set and unset alike — so
	// it is the dialect's answer rather than a switch. See
	// interp.Semantics.DeclarationOperandExpandsItsSubscript (#3298).
	s.DeclarationOperandExpandsItsSubscript = interp.Yes

	// And the three surfaces where a script *can* turn the round off, which
	// is what `shopt -s assoc_expand_once` and its synonym name. The axis
	// says this shell rounds; the switch beside it, which only this dialect
	// wires, is what a script uses to stop it. See
	// interp.Runner.ExpandsAnOperandsSubscriptAgain.
	//
	// Measured 2026-09-19 on bash 5.3.20, from a script file with standard
	// input on /dev/null, over `k='x y'` and a table holding one element
	// under the key `x y` — the left column is this shell's default and the
	// right the same probe under `-O assoc_expand_once`:
	//
	//	unset -v 'a[$k]'      the element is gone   it survives
	//	unset 'a[$k]'         the element is gone   it survives
	//	printf -v 'c[$k]' P   the key `x y`         the key `$k`
	//	read 'b[$k]' <<<Z     the key `x y`         the key `$k`
	//	test -v 'g[$k]'       true                  false
	//	[ -v 'g[$k]' ]        true                  false
	//	[[ -v 'g[$k]' ]]      true                  **true**
	//
	// The last row is why `[[ -v ]]` is answered by the axis above and not
	// by one of these: the same question reaches the keyword and the builtin
	// and only the builtin's answer is a script's to change. The controls
	// are the same operands with nothing in them to expand — `unset
	// 'a[plain]'` removes the element and `test -v 'g[nope]'` is false in
	// every column — which say the rows above turn on the round rather than
	// on whether a quoted subscript reaches the builtin at all.
	s.UnsetExpandsAFlatSubscript = interp.Yes
	s.OutputOperandExpandsAFlatSubscript = interp.Yes
	s.TestIsSetExpandsAFlatSubscript = interp.Yes
	// And a prompt whose input is not a terminal writes each line it read
	// back, behind the prompt it was read at, because nothing else will.
	// The one dialect in the panel that does — see
	// Semantics.PromptEchoesTheLineWhereThereIsNoTerminal.
	s.PromptEchoesTheLineWhereThereIsNoTerminal = true
	// See Semantics.EndOfInputInAConstructEndsTheSession (#6263).
	s.EndOfInputInAConstructEndsTheSession = true
	// See Semantics.EndOfInputEndsAContinuedHeredocLine (#6273).
	s.EndOfInputEndsAContinuedHeredocLine = true
	// See Semantics.EndOfInputEndsOneHeredocBody (#6287).
	s.EndOfInputEndsOneHeredocBody = true
	// And it reads the editing keys there too: measured, `C-r` on a pipe is a
	// reverse-i-search in this shell and the characters of a command name in
	// the other three. See Semantics.EditorReadsKeysWhereThereIsNoTerminal.
	s.EditorReadsKeysWhereThereIsNoTerminal = true
	// And a status the prompt sets for a line that ran nothing — ^C, a
	// refusal — is PIPESTATUS as well as `$?`, where zsh leaves its record
	// alone. See Semantics.PromptStatusWritesThePipelineRecord.
	s.PromptStatusWritesThePipelineRecord = true
	// What a ^C at the prompt does with a trap on INT: bash sets 130 before the INT trap runs at a prompt, and gives the line up.
	// See Semantics.PromptInterruptTrap.
	s.PromptInterruptTrap = interp.PromptInterruptStatusThenTrap
	return s
}
