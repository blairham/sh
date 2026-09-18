// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package dash answers the substrate's questions the way dash does.
package dash

import (
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Dialect is what dash parses.
func Dialect() syntax.Dialect {
	// dash is the POSIX shell language and nothing more.
	d := syntax.POSIX()
	// dash expands aliases in a script, with no option to turn on, and by
	// every route: `-c`, a file and standard input all expand.
	// A here-document body line that joined *before* any text of it was
	// written still reaches the delimiter — `\` alone over `ABC` ends an
	// `ABC` document — and one that joined after text does not. bash and zsh
	// take both and ksh93 neither (#2430).
	d.HeredocDelimiterAcrossAContinuation = syntax.HeredocDelimiterAfterALeadingContinuation
	d.AliasesExpandUnlessTold = true
	d.ExpandAliasesInProgramText = syntax.RouteOnEveryRoute
	// And it splices the body's text, so a newline in one is a line of the
	// program: everything after an expansion shifts down by one per newline.
	// Set here rather than inherited because this dialect starts from the
	// bare POSIX vector rather than from the core.
	d.AliasBodyCountsLines = true
	// A backslash an alias body ends with reaches the newline after the
	// alias word, where it is an ordinary line continuation and joins the
	// next line to the word. zsh and bash 3.2 are the columns that do not.
	// See syntax.Dialect.AliasBodyBackslashJoinsTheNextLine (#2710).
	d.AliasBodyBackslashJoinsTheNextLine = true
	// Not a construct it adds but how it reads one it already has: a name
	// followed by `(` is a function definition here, whether or not the `)`
	// comes next, which is what decides the token a malformed one is blamed
	// on.
	d.FuncDefAtParen = true
	// And having committed, it checks: a name with punctuation is refused
	// once the parens close — `Bad function name`.
	d.FunctionNamePunctuation = false
	// The same refusal for a name that is perfectly well formed and is a
	// *special* builtin's. dash's special builtins are the fifteen POSIX
	// marks special plus `local`; `.` and `:` are two of them and are
	// already refused by the rule above, so the set holds the other
	// fourteen. `true`, `read`, `cd` and every other regular builtin are
	// ordinary names here. The refusal is
	// while *reading*, so `printf a; export() { :; }; printf b` prints
	// neither word and `if false; then export() { :; }; fi` is refused as
	// well. Measured 2026-09-15 on all three routes; see
	// syntax.Dialect.FunctionNamesRefused (#2932).
	d.FunctionNamesRefused = map[string]bool{
		"break": true, "continue": true, "eval": true, "exec": true,
		"exit": true, "export": true, "local": true, "readonly": true,
		"return": true, "set": true, "shift": true, "times": true,
		"trap": true, "unset": true,
	}
	// One operator may stand where a case pattern belongs, opening an arm
	// that matches nothing — measured by running it, not inferred from the
	// error it causes elsewhere.
	d.CasePatternAcceptsOperator = true
	// The loop-variable position is read as a name whatever stands there, so
	// `for` with nothing after it is `Bad for loop variable` here and an
	// unexpected token in the other three.
	d.ForNonWordIsANameError = true
	return d
}

// Semantics is what dash means where the shells conflict.
func Semantics() interp.Semantics {
	s := interp.PosixSemantics()
	// unanswered BuiltinReadsOptions: there is no `builtin` here to read one —
	// `builtin -q` is `builtin: not found` at 127 (#3217).
	//
	// unanswered WritingSubstitutionIsWaitedForAtTheCommand: this shell has
	// no process substitution, so there is no `>(cmd)` body for a command to
	// wait for or not. `echo >(:)` is the two characters as written (#2197).
	// `export a+=2` is `a+: bad variable name` here, so the append operator
	// is not an operand this shell's declarations take.
	s.DeclarationTakesAnAppendOperand = interp.No
	// A prefix to a function is the call's environment and nothing after it,
	// which is the answer six of the seven columns give and the one POSIX
	// leaves open. Measured 2026-09-12: `f(){ env | grep "^v="; }; v=1; v=9
	// f; echo "[$v]"` prints `v=9` from the child and `[1]` after. Worth
	// saying out loud in the shell that targets the standard's text —
	// 2.9.1 makes both of these unspecified, so this is a measurement of
	// this shell rather than compliance with anything (#2407).
	s.AssignmentPrefixPersistsAfterAFunction = interp.No
	s.PrefixToAFunctionIsExported = interp.Yes
	// Not at a builtin, though: `v=1; v=9 eval 'env | grep "^v="'` shows the
	// child nothing, and the attribute this shell already had is left where it
	// was. Measured 2026-09-16 (#3437).
	s.PrefixExportAtABuiltin = interp.PrefixExportAtABuiltinUnchanged
	// This shell has no declaration word but `readonly` and `export`, both
	// special builtins whose prefix persists here by the axis above — so the
	// value a promoting shell keeps is one this shell keeps for another
	// reason, and there is nothing left for this axis to move (#3437).
	s.DeclarationPromotesThePrefixEntry = interp.No
	// unanswered SubscriptedAssignmentPrefix, SubscriptedPrefixIsTakenBack:
	// a subscripted word is no assignment in this grammar, so `a[1]=v f` is
	// the *command* `a[1]=v` and the complaint is `a[1]=v: not found`. The
	// word never becomes an assignment prefix and neither axis is reached.
	// Measured 2026-09-16 on dash 0.5.12 (#3433).
	// An unquoted list is its elements taken one at a time, never their
	// join — the reading POSIX describes, and the one an empty element
	// disappears under: `IFS=:; set -- x "" y` is two fields here and three
	// in bash.
	s.UnquotedListJoinsOnIFS = interp.No
	// `${1:=abc}` is `1: bad variable name` and ends the script at 2 — a
	// positional is no more assignable through an expansion here than `@`
	// is (#1541).
	s.AssignThroughExpansionMayNameAPositional = interp.No
	// The one shell in the panel that expands a here-document body in the
	// shell rather than in the process the redirection is for, so what the
	// body writes is still there afterwards: `unset u; cat <<END` with a
	// body of `${u:=zz}` leaves `u=zz` here and leaves it unset in bash,
	// ksh93 and zsh. Its own failure half still costs only the command —
	// `set -u` on an unset name in such a body reports 2 and the script
	// carries on — which is unanimous and so is not asked.
	s.HeredocExpandsInTheCommandsProcess = interp.No
	// And the same for a redirection's *target*, where this shell is alone
	// again and where the split costs more: `cat /dev/null > "${u:=made}"`
	// leaves `u` set here and unset in the other three, and `> "$NOPE"`
	// under `set -u` ends the script here where the other three lose only
	// the command. A second axis rather than the one above, because the two
	// positions do not share an answer — a body's failed expansion costs
	// this shell the command and not the script (#1228).
	s.RedirectTargetExpandsInTheCommandsProcess = interp.No
	// POSIX makes an unquoted `$@` behave as `$*` where nothing is split,
	// and this shell complies: `IFS=-; set -- x y z; v=${@}` is `x-y-z`
	// here and in zsh, against `x y z` in bash and ksh93. It has no arrays,
	// so the positional spelling is the whole of the question here.
	s.UnsplitAtListJoinsOnIFS = interp.Yes
	s.CommandNotFoundStatusIsNotFound = interp.Yes
	// `command -v ./bb/tool` is `./bb/tool` here: the operand back, not the
	// absolute path POSIX asks for. Measured 2026-09-14 against 0.5.12, the
	// panel's closest reading of the standard declining to follow it.
	s.APathnameOperandIsReportedAbsolute = interp.No
	s.SetFTurnsOffGlobbing = interp.Yes
	// Neither editing mode is ever selected on its own here — measured
	// 2026-09-11, `set -o` reports both off in a script and under `-i`.
	s.InteractiveSelectsEmacs = interp.No
	// This shell has no braces to expand and no `-B` either, so the letter
	// is refused as the invalid option it is rather than asked about.
	s.SetBTurnsOffBraceExpansion = interp.No
	// The panel's only shell with no multibyte decoder: `s=héllo; echo
	// ${#s}` is 6 here in every locale, where bash, ksh93 and zsh answer 6
	// under `LC_ALL=C` and 5 under a UTF-8 one. Measured 2026-09-05 across
	// LC_ALL, LC_CTYPE and LANG, and dash does not move for any of them.
	s.MultibyteEncodingIsHonored = interp.No
	// The one shell that refuses the -h letter POSIX names.
	s.SetHasTheHLetter = interp.No
	// And no keyword option either. Measured 2026-09-16: `set -k` is
	// `set: Illegal option -k` at 2 and the file ends there, and `set -o
	// keyword` is refused the same way. The refusal is dash's answer rather
	// than a gap, which is what this states.
	s.KeywordAssignments = interp.No
	// A declaration utility is recognized by the name of the utility that
	// runs, however the word was written: `cmd=export; $cmd v=$b`, `\export
	// v=$b`, `'export' v=$b` and `e=; $e export v=$b` all keep `x y` whole,
	// where zsh, bash and ksh93 split at least one of them. The zero value,
	// stated here because it is measured and not inherited — 2026-09-16 on
	// dash 0.5.12, for export, readonly and local.
	s.DeclarationCommandWord = interp.DeclarationByUtilityName
	// And `command export v=$b` keeps it whole too, in every spelling of the
	// prefix and with `-p` as well. See #3341.
	s.CommandPrefixKeepsADeclaration = interp.Yes

	// unanswered KeywordPromotesADeclarationsOperand: there is no keyword
	// option here to reach a declaration with — `set -k` is `Illegal option
	// -k` and ends the file — so the question cannot be put to this shell.
	// Job control wants the tty: with none, `set -m` earns the remark
	// `can't access tty; job control turned off` — a remark, measured, not
	// a failure: the option stays off and `set` still reports 0.

	s.MonitorNeedsATerminal = interp.Yes
	// The monitor alone is what `fg` and `bg` need, which here means with a
	// terminal: the monitor is denied without one, so the gate answers no
	// through the line above. Measured 2026-09-15 on a pseudo-terminal —
	// `set -m; sleep 0 &; fg` prints `sleep 0` at 0 — and off one, where the
	// denial leaves `fg: job (null) not created under job control` at 2
	// (#2720).
	s.MonitorAloneResumesAJob = interp.Yes
	// It resumes on the monitor alone and still announces nothing on it: a
	// script with `set -m` on a pseudo-terminal prints no start notice here,
	// though it does report the job *ending*. Measured 2026-09-15 (#2838).
	s.MonitorAloneAnnouncesAJob = interp.No
	// dash leaves it off with no terminal too, remarking `can't access tty;
	// job control turned off` — the same sentence its `set -m` refusal uses,
	// from the same shell, about two different questions.
	s.InteractiveMonitorNeedsATerminal = interp.Yes
	// The same, and measured the same way: dash forks for `( )`, so the
	// subshell outlives the signal its `kill` sent the shell.
	s.SubshellRunsOnAfterSignalingTheShell = interp.Yes
	// dash has somebody to tell on this route, and tells them exactly one
	// thing: measured on `-i script.sh` through a pseudo-terminal it writes
	// `[1] + Done sleep 0.3` and never the line that starts the job. The
	// start is AnnouncesBackgroundJob, which dash answers No, and the two
	// fields are separate because of exactly this shell.
	s.InteractiveScriptAnnouncesJobs = interp.Yes
	// And nothing whatever on the other one. Measured 2026-09-12 on `-i -c`
	// through a pseudo-terminal, with the job held open on a fifo the string
	// releases and then reaps: no start line and no `Done` row, where bash,
	// ksh93 and zsh all write at least the start. It is a real answer and not
	// an absent one — `$-` on that route is `mi`, so the shell is interactive
	// with the monitor running and still says nothing.
	s.InteractiveCommandStringAnnouncesJobs = interp.No
	// And where it does speak it speaks at once: the `Done` row on
	// `-i script.sh` is written between the commands, on a route that draws no
	// prompt at all.
	s.FinishedJobNoticeNeedsAPrompt = interp.No
	// And `$!` before any background command is unset here as it is in bash,
	// in its own words and with its own status: measured,
	// `set -u; echo "[$!]"` writes `!: parameter not set` — the name without
	// its `$` — and stops at 2.
	s.LastBackgroundPidIsUnsetBeforeAnyJob = interp.Yes
	// A job started with `&` reads an empty standard input, not the shell's:
	// measured 2026-09-07, `dash -c '/bin/cat & wait; echo ---; /bin/cat' < f`
	// writes `---` and then the file's line. POSIX XCU 2.9.3.
	s.BackgroundJobInput = interp.BackgroundJobInputEmpty
	// And it reads as nothing rather than as a zero: `echo "[$!]"` is `[]`.
	s.LastBackgroundPidIsZeroBeforeAnyJob = interp.No
	s.ProcessSubstitutionIsTheLastBackgroundJob = interp.No
	// DefaultOptionLetters stays empty on purpose: measured, dash's `$-`
	// starts blank however it is invoked, save the `s` of the
	// standard-input route, which is unanimous and comes from Runner.Route.
	// Under `-c` it stays blank — neither letter, where bash and ksh93 show
	// `c` and ksh93 also shows `s`.
	//
	// InteractiveOptionLetters stays empty too, and here that is an answer
	// rather than a silence: measured 2026-09-05, `-i script.sh` reports `i`
	// and nothing else, so the interactive set is the same empty set. dash is
	// the one shell in the panel that adds nothing at a prompt, which is what
	// makes the other three's additions evidence rather than a coincidence.
	// The namespace's own way of saying `-i`, which this shell has and
	// prompts for: measured 2026-09-16 with a program on a pipe, `dash -o
	// interactive` writes `can't access tty; job control turned off` and then
	// a `$ ` prompt around the line, exactly as `dash -i` does. There is no
	// negative spelling — `-o nointeractive` is `Illegal option` here — so
	// the second name stays empty and `+o interactive` is the only way back
	// (#3195).
	s.InteractiveOptionName = "interactive"
	// Login-ness written out, and the short spelling only: measured
	// 2026-09-05, `dash -l -c cmd` reads `~/.profile` and `dash --login`
	// is refused outright with `Illegal option --` at status 2, where the
	// other three take both. dash has no escape hatch of any kind, which is
	// also measured — a startup file that breaks it is escaped by moving
	// the file.
	s.StartupFileOptions = interp.StartupFileOptions{Login: "-l"}
	// The panel's only shell that goes on to read standard input as a program
	// once the `-c` string has run, when `-s` was given too. Measured
	// 2026-09-12: `printf 'echo LINE1\necho LINE2\n' | dash -sc 'echo
	// FROM-C'` writes all three lines where the other five write FROM-C
	// alone, and the order the two options are written in makes no
	// difference.
	s.StdinOptionSurvivesTheCommandString = true
	// Semantics.SystemStartupFiles stays at the POSIX preset's `/etc/profile`,
	// which is measured for this shell and not only inherited: `dash -l -c
	// 'echo ${PATH%%:*}'` with a `~/.profile` that reports the same thing
	// sees `path_helper`'s answer in both places, where a shell that read no
	// system file would have seen the inherited one.
	// And no version option, which is left as the zero value deliberately:
	// measured 2026-09-11, `dash --version` is `Illegal option --` at status
	// 2, the same refusal every long option but `--login`'s absence gets.
	// This shell is the panel's only one that will not name its version.
	// `+i` takes the prompt back, and it is the only spelling that does —
	// there is no negative option name here. Measured 2026-09-16 with the
	// program on a pipe: `dash -i +i -c 'echo $-'` writes an empty `$-` and
	// nothing about a terminal, where `dash -i -c` writes `can't access tty;
	// job control turned off` first.
	s.PlusSignedInteractiveLetterStillPrompts = interp.No
	s.CommandStringShowsCInDollarDash = interp.No
	s.LoginShowsLInDollarDash = interp.No
	s.CommandStringShowsSInDollarDash = interp.No
	// The order of the letters, and this shell is the reason the axis is a
	// sequence rather than a discipline: it is neither sorted, nor the order
	// the script set them in, nor capitals apart. Measured 2026-09-12, the
	// `s` rows by feeding the program on standard input and the last through
	// a pseudo-terminal:
	//
	//	set -f; set -u; set -e             ufe
	//	set -a -b -C -e -f -u -v -E -I     ubaCEvIfe
	//	set -x -a -C -e -u -v              uaCvxe
	//	set -C -e, on stdin                Cse
	//	set -x -C -v -u -a -f -e, on stdin uaCvxsife
	//	-i -c, at a terminal               mi
	//
	// `n` is in the string and is the one letter here nobody can measure
	// directly: the option it stands for stops the `echo` that would read
	// `$-`. Its place is the one the rest of the order implies — this
	// shell's `set -o` listing runs `errexit noglob ignoreeof interactive
	// monitor noexec stdin xtrace verbose vi emacs noclobber allexport
	// notify nounset`, which is this string reversed, and `noexec` sits
	// between `stdin` and `interactive` there. The `mi` row is the check on
	// that: `m` precedes `i` in `$-` exactly as the reversal predicts, and
	// nothing else in the panel puts them that way round.
	s.DollarDashLetterOrder = "ubaCEVvxsnmiIfe"
	// This shell has no `typeset`, but it can still be asked and it answers:
	// `readonly` is POSIX and `local` is the one declaration here, so the
	// two words that make the question both exist. Measured 2026-09-07:
	//
	//	readonly x=1
	//	f() { local x=2; echo "in=[$x]"; }
	//	f
	//	→ local: x: is read only, and the script ends
	//
	// So `No`, and now for a measured reason rather than because the
	// question looked out of reach — it was reached with the shell's own
	// spelling of the freeze rather than with bash's.
	s.DeclarationMayShadowAReadonly = interp.No
	// ReadonlyAttributeCanBeRemoved stays unanswered: there is no `typeset`
	// or `declare` here to write a plus form with, and `readonly +r x` is a
	// bad variable name rather than an option — so nothing can ask it, and
	// nothing will reach the refusal an unanswered axis makes.
	s.DeclaredNameWithoutValueIsEmpty = interp.No
	// And no record of the bare declaration is kept. `local x` is the only
	// way to write one here and this shell has no declaration listing to
	// read it back with, so nothing in it can tell the two answers apart —
	// measured 2026-09-15, `f(){ local x; set; }; f` names nothing. The axis
	// is still answered rather than left out, because `local x` reaches it
	// and an unanswered axis refuses at run time (#2272, #2999).
	s.ValuelessDeclarationRecordsTheName = interp.No
	// unanswered PrefixListingNamesADeclaredOnlyCompound: there is neither a
	// `${!prefix@}` nor a compound to declare — `${!q@}` is `Bad
	// substitution` — so nothing here can reach the axis. Measured
	// 2026-09-16 on dash 0.5.12.
	// $(( )) with nothing in it wants a primary and stops the script.
	s.EmptyArithExpressionIsAnError = interp.Yes
	// The one column that clamps a numeral past the word instead of letting
	// it go round: every over-large numeral is 9223372036854775807, decimal,
	// hexadecimal or octal alike, and nothing is said about it. The other two
	// POSIX columns wrap, which is what the preset follows (#3202).
	s.ArithNumeralPastTheWord = interp.NumeralPastTheWordSaturates
	// And the saturation is the *written* numeral's alone: the same digits
	// out of a variable are `Illegal number` at status 2, which is the one
	// place this shell's two number readers part (#3202).
	s.ArithStoredNumeralPastTheWordIsRefused = interp.Yes
	// A bare read wants a name here, where the other three fill REPLY.
	s.ReadRequiresAVariableName = interp.Yes
	// The odd one out on `echo … | sh`: dash takes the program off standard
	// input in blocks and keeps what it took, so a `read` in the script
	// finds end of input and the data line is run as a command. The other
	// three read a line at a time and leave the rest on the descriptor.
	s.StdinProgramReadInBlocks = true
	// `-c` and `-s` together: the command string names the operands here,
	// so `sh -sc CMD name a` has `$0` of `name` and one parameter. ksh93
	// and zsh let `-s` name them instead.
	s.StdinOptionNamesTheOperands = interp.No
	// read takes -r and, alone among its letters, bash's -p prompt — an
	// argument, printed only to a terminal. The rest of bash's set (-s, the
	// counts, -d, -t, -u) is refused as unknown here.
	s.ReadOptions = "rp:"
	// dash has the two POSIX letters and calls anything else illegal.
	// unanswered DollarSingleEscEscape, DollarSingleQuestionEscape and
	// DollarSingleUnicodeEscapes: dash has no `$'…'` at all, so the `$` is an
	// ordinary character and what follows it is an ordinary quoted string —
	// measured 2026-09-16, `printf '%s' $'\e'` writes `$` then `\e`, three
	// bytes, where every other column writes one. The five other axes of that
	// construct are unanswered here for the same reason and have been since
	// the dialect was written (#3270).
	// unanswered ReplacementAnchors: dash has no span replacement at all, so
	// there is no `/` for an anchor to stand after. Measured 2026-09-16 from
	// a script file, `v=abcabc; printf '%s' "${v/b/X}"` is `Bad
	// substitution` at 2 with nothing written and the script over (#3272).
	//
	// unanswered AnchoredEmptyReplacementPattern: the same absence one step
	// in — an empty pattern behind an anchor needs the anchor first (#3272).
	//
	// unanswered GlobalReplacementAnchors: and so does the global spelling
	// that would carry a second one (#3307).
	//
	// Three paragraphs rather than one sentence naming three axes, which is
	// what this was: `make axis-coverage` reads `unanswered NAME:` and a
	// line naming more than one matches nothing, so a reason was written and
	// no instrument could see it.
	// unanswered TraceArrayLiteralShowsTheExpandedElements: there is no
	// array literal to trace. Measured 2026-09-14, `a=(1 2)` is `Syntax
	// error: "(" unexpected` at 2 — the parenthesis, not the assignment — so
	// no line of this shape ever reaches a trace (#1959).
	// unanswered TraceSubscriptedArrayLiteralIsElementAssignments: and the
	// subscripted shape is the same literal, so it does not parse either.
	// unanswered TraceElementSubscriptIsEvaluated: nor a subscript. `a[1]=v`
	// is an ordinary word here, assigned to a variable literally named
	// `a[1]`, so there is nothing for the trace to resolve.
	// unanswered ArithFloatOverflowIsZero: dash has no floats, so `1e400` is
	// not a number out of range but a word its arithmetic cannot read at
	// all. Measured 2026-09-14, `$((1e400))` is `arithmetic expression:
	// expecting EOF: "1e400"` at 2, which is the same complaint `$((1e4))`
	// gets — the axis is about a value, and no value is ever read (#2766).
	// unanswered EmptyAssociativeKeyRefusesTheLength: there is no keyed
	// table to take the length of an element of. Measured 2026-09-12, `w=;
	// typeset -A m` is `typeset: not found` and `${#m[$w]}` is `Bad
	// substitution` at 2 — no subscript reaches a parameter expansion here
	// at all, so the question is refused one construct earlier than this
	// axis (#2286).
	// unanswered AttributeOverAFrozenNameIsRefused: the axis is about a type
	// **letter** meeting a frozen name, and dash has no letters to bring.
	// Measured 2026-09-13, `typeset -i q=1` is `typeset: not found` at 127,
	// so the declaration the axis asks about never happens. `export` over a
	// `readonly` name — the only attribute this shell has — is taken at 0,
	// which is a different question and not this one (#2561).
	// unanswered DeclareMatchingLetter: there is no declaration utility to
	// spell the letter on. Measured 2026-09-13, `declare -m q=1` and
	// `typeset -m q=1` are both `not found` at 127, so neither reading of
	// the letter can be put to this shell (#2345).
	// unanswered DeclareMappingLetter: the same wall. `typeset -M tolower v`
	// is `typeset: not found` here too.
	// unanswered DeclareTypeLetter: the same wall again. Measured
	// 2026-09-13, `typeset -T TS ts` and `declare -T TS ts` are both `not
	// found` at 127, so there is no builtin here to read the letter either
	// way (#2419).
	// unanswered DeclareHideValueLetter: the same, for `typeset -H h=hid`.
	// unanswered ProducedParameterListing: the axis is what a listing with no
	// operands writes for a produced parameter, and there is no listing.
	// Measured 2026-09-13, `env -i PATH=/usr/bin:/bin` with a scratch HOME:
	// `typeset -p` and `declare -p` are both `not found` at 127, and this
	// dialect registers no producer with SetDynamicDeclaration for one to
	// name anyway — it has neither RANDOM nor SECONDS (#2518).
	// unanswered ExpansionResultSuppliesGroupSyntax: dash has no pattern
	// groups at all, so `(`, `)` and `|` out of a value are text here
	// however they arrived and there is nothing for the axis to choose.
	// unanswered TableLetterReachesItsOwnOperandsSubscript: dash has no
	// arrays and no table letter, so `typeset -A m[k]=v` is a bad command
	// name here rather than a declaration whose ordering could be measured.
	// unanswered KeyedLiteralAppendJoinsTheReplacedValue: there is no keyed
	// table and no literal to write one with. Measured 2026-09-12,
	// `m=([k]+=x)` is `Syntax error: "(" unexpected` before any element is
	// looked at, so no value is ever joined to anything (#2405).
	// unanswered StoreRefusalThroughPrintfLeavesZero: this shell's
	// `printf` has no `-v`, so no store is reached through it.
	//
	// unanswered StoreRefusalOfADeclaredElementLeavesZero:
	// there is no declaration utility and no array for one's operand to name
	// an element of, so no store of this shell's can refuse a declaration's
	// element on either route. Measured 2026-09-14, `a=(x y); typeset
	// "a[0]"=v; echo after` is `Syntax error: "(" unexpected` at 2 from `-c`
	// and from a script file alike — the parenthesis is refused before any
	// subscript is read, which is the same wall #2250 records for the
	// neighboring array axes (#1770).
	// unanswered ArrayLiteralOperandRetypesAFrozenScalar: there is no
	// declaration utility and no array literal to be one's operand. Measured
	// 2026-09-12, `readonly q=1; typeset -g q=(b)` is `Syntax error: "("
	// unexpected` at 2 — the parenthesis is refused before anything has a
	// frozen name to think about, so neither half of the question can be put
	// to this shell (#2250).
	// unanswered NumericTypeLetterRetypesAFrozenName: there is no numeric
	// type letter to write. Measured 2026-09-12, `typeset` is `not found`
	// here and `export -i q=4` is `Illegal option -i`, so the only two words
	// that could carry the letter refuse it before a frozen name is reached
	// (#2539).
	// unanswered AttributeOverAFrozenNameIsRefused: there is no declaration
	// command to write an attribute letter with. Measured 2026-09-12,
	// `typeset` is `not found` here, so a frozen name is never reached with
	// one (#2561).
	// unanswered UpperCaseLetterBesideANumericTypeLetterRecordsNothing and
	// unanswered TwoCaseLettersOnOneDeclarationCancel: there is no
	// declaration command, so neither case letter can be written at all.
	// Measured 2026-09-12, `typeset -lu z=Ab` is `typeset: not found` and
	// leaves `z` unset — the same wall the other declaration axes meet, and
	// there is no second word here that reads a case letter (#2541).
	// unanswered TableUnderAnArrayLiteralDeclaration and
	// unanswered ArrayUnderATableLiteralDeclaration: there is neither kind of
	// array to convert between nor a literal to carry a value in. Measured
	// 2026-09-12, `typeset -A h` is `typeset: not found` and `h=(x)` is
	// `Syntax error: "(" unexpected`, so the question is refused twice over
	// before there is a compound to change the kind of (#2287).
	// unanswered WholeArraySubscriptAssigningAnArray and
	// unanswered WholeArraySubscriptAssigningATable: there are no arrays, so
	// a subscript on the left of an assignment is not a subscript. Measured
	// 2026-09-12, `x=(p q)` is `Syntax error: "(" unexpected` and `x[@]=Z`
	// on its own is `x[@]=Z: not found` — the word is a command name here,
	// which is a third thing again and not an answer to either field (#2285).
	// unanswered EarlierDeclarationLetterBlocksALaterPlus: there is no
	// declaration command to write the letter on. `typeset` is not a
	// builtin here and `integer` is not a word, so neither sign of `-i`
	// can be put to this shell at all (#2345).
	// unanswered FloatFormatLetterE: there is no declaration command to write
	// the letter on. Measured 2026-09-15, `typeset -E 3 a=1.5` is `typeset:
	// not found` at 127 (#2559).
	// unanswered DeclareNumberDetachedOnlyAtTheWordEnd: the same wall — no
	// declaration builtin, so no option word for a number to stand at the
	// end of.
	// unanswered BareFloatLetterResetsThePrecision: nor any float attribute
	// to have a precision.
	// unanswered NumericTypeLetterPrecedence: nor a rank between them, the
	// declaration utility being absent entirely (#2419).
	// unanswered DeclareHideInScopeLetter: the same wall, for `typeset -h s
	// q=1`, which is `typeset: not found` at 127.
	// unanswered NumericTypeLettersAreExclusive: nor a pair of numeric
	// letters to write together.
	// unanswered PrefixToAKeywordFunctionIsScopedToTheCall: there is no
	// `function` keyword here, so there is no second function spelling for a
	// prefix to be scoped differently in front of. Measured 2026-09-18,
	// `function kf { :; }` is `Syntax error: "{" unexpected` (#3161).
	// unanswered NamerefCycleIsRefused: there are no name references to make
	// a cycle of. Measured 2026-09-15, `typeset` is not a builtin here —
	// `typeset: not found` at 127 — so the declaration that would build one
	// cannot be written (#2553).
	// unanswered NamerefArrayRefusal: the same wall, and no arrays either —
	// `r=(a b)` is `Syntax error: "(" unexpected` — so neither half of the
	// question can be put (#3103).
	// unanswered UnsetReferenceLetterRemovesANonReference: this shell has no
	// `-n` on `unset` to ask it with. Measured 2026-09-12, `unset -n x` is
	// `unset: Illegal option -n` and the operand is never read, so there is no
	// name for the question to be about (#932).
	// unanswered UnsetSubscriptSkippedWhenNameUnset: there is no subscript
	// for `unset` to skip or read. Measured 2026-09-12, `unset "nope[x+]"`
	// is `nope[x+]: bad variable name` and ends the script, so the operand
	// never reaches a base name and a bracket (#2373).
	// unanswered UnsetStatusIsTheLastSubscripts: no operand here can fail
	// and be outlived by a later one. A bad variable name, a bracketed name
	// and a readonly all end the script, measured, so there is never a
	// status left behind for a following operand to overwrite or keep
	// (#2373).
	s.UnsetOptions = "vf"
	// `readonly` keeps POSIX's single letter: measured 2026-09-12,
	// `readonly -a zz` is `readonly: Illegal option -a` and, this being a
	// special builtin, it ends the shell. `-A`, `-f` and `-n` are refused
	// the same way. Stated rather than left to the inherited value, since
	// the interpreter used to fix the set at `paAf` and this column
	// accepted three letters its shell has never had (#2277).
	s.ReadonlyOptions = "p"
	s.ExportListing = interp.DeclareListingCommandWord
	s.ReadonlyListing = interp.DeclareListingCommandWord
	// dash single-quotes every listed value; it has no declare, so this
	// style exists for the two -p listings alone.
	//
	// Doubled out and never escaped, which is the same rule AliasQuoting,
	// TrapQuoting and SetListingQuoting below already carry. It has to be:
	// this shell has no `$'…'` and a backslash inside single quotes is a
	// backslash, so `'a'\''b'` would read back as five characters and the
	// listing's whole job is to be re-readable. Measured 2026-09-16 on
	// Apple's dash-16 and on upstream 0.5.12 built from source, both:
	//
	//	v="a'b"; export v      export v='a'"'"'b'
	//	r="a'b"; readonly r    readonly r='a'"'"'b'
	//
	// It was ListingQuoteAlwaysEscaped until now, so the two -p listings
	// wrote zsh's spelling while a bare `set` three fields down wrote dash's
	// — one shell's single-quoting rule held in four places and wrong in the
	// one whose comment did not repeat the rule. Found by
	// share/suite/dash/variables.tests, which asks the two listings and the
	// bare `set` in one file for exactly that reason.
	s.DeclareValueQuoting = interp.ListingQuoteAlwaysDoubled
	// unanswered ChainedSubscriptReadsANestedValue: the grammar for a chained
	// subscript is not this shell's — `${a[1][2]}` is a bad substitution or
	// a pattern here — so there is no chain for a reading to be about.
	// Measured 2026-09-15 (#2830).
	// unanswered ListedBangIsOrdinary: every listing style this shell uses
	// quotes whatever it is given, so `'!'`, `'^'`, `'a=b'` and `'é'` say
	// nothing about which bytes a listing may leave bare. Measured
	// 2026-09-14 with a bare `set` over all four (#2820).
	// unanswered ListedCaretIsOrdinary: the same always-quoting style.
	// unanswered ListedEqualsIsOrdinary: the same always-quoting style.
	// unanswered ListedNonAsciiIsOrdinary: the same always-quoting style,
	// and no `$'...'` listing here for the escaped half of the question.
	// unanswered ListedAssignmentPrefixIsBare: a bare head is only visible
	// in a listing that leaves anything bare, and this one leaves nothing.
	// unanswered OperatorAfterTheSubscriptListingIsBad: `${!name[@]}` is a
	// bad substitution here in the *bare* form too, so there is no listing
	// for an operator to come after. Measured 2026-09-14 (#2821).
	s.EchoInterpretsEscapes = interp.Yes
	// Neither spelling of the escape character: this shell's set is the XSI
	// list alone, so `\e` and `\E` are the two characters they are written
	// as.
	// This shell expands escapes without `-e` and still has neither of these:
	// `echo 'a\u0041Z'` is the characters as written.
	s.EchoExpandsUnicodeEscapes = interp.No
	s.EchoExpandsEscEscape = interp.No
	s.EchoExpandsCapitalEscEscape = interp.No
	s.LengthOfSpecialIsCount = interp.No
	s.UnterminatedBracket = interp.BracketNoMatch
	// And the same question where a `[:name:]`, a `[.x.]` or a `[=x=]`
	// inside it is what left it open: a class that can never match, the same as a bare `[`.
	s.UnterminatedBracketAfterASubExpression = interp.BracketNoMatch
	// And inside a bracket expression the backslash protects the character
	// behind it and puts nothing of its own in the set: `[\)]` is the
	// one-character set `)`, and `[a\-z]` is the three members a, `-` and z,
	// the escape being what stops the dash reading as the range operator.
	// zsh adds the backslash to the set as well and BusyBox ash protects
	// nothing, so this value is what five of the seven columns share (#3271).
	s.BracketEscape = interp.BracketEscapeProtectsTheMember
	// `echo .*` is `. .. .dot` here and `echo .*/` is `../ ./`, which is
	// ksh93's and bash 3.2's answer rather than bash 5.3's. Measured
	// 2026-09-14 (#2748). There is no parameter of GLOBIGNORE's kind, so
	// the four axes beside this one are never asked.
	s.GlobListsDotAndDotDot = interp.Yes
	// unanswered IgnoredNamesValueIsOnePattern: there is no parameter of
	// GLOBIGNORE's kind in this shell — measured 2026-09-14, setting
	// `GLOBIGNORE` and `FIGNORE` alike changes nothing about what `echo *`
	// produces — so the three questions about *how* its value is read
	// cannot be put to it. IgnoredNamesMatchTheLastComponent and
	// IgnoredNamesFollowTheParameter are unanswered beside it for the same
	// reason, and IgnoredNamesVariable being empty is what keeps any of the
	// three from ever being reached (#2748).
	// unanswered IgnoredNamesMatchTheLastComponent: see above.
	// unanswered IgnoredNamesFollowTheParameter: see above.
	s.UnknownCharacterClass = interp.UnknownClassEndsTheScan
	// `[[.a.]]` and `[[=a=]]` are the collating element and the equivalence
	// class, matching `a`. A body this shell cannot read ends the scan the
	// way an unknown class name does: measured 2026-09-16,
	// `[a[.nosuch.]b]` matches a and not b.
	s.CollatingSymbols = interp.Yes
	// `[[:]` stops the bracket where the `[:` stands: a member written before it still matches and nothing after it does — the same shape this shell gives a class name it has not got.
	// See interp.Semantics.UnterminatedCharacterClass (#1431).
	s.UnterminatedCharacterClass = interp.UnterminatedClassEndsTheScan
	// The bash column's reading of a value's backslash, measured the same
	// way and on the same day: `v='a\*'; set -- $v` is `a\*` here, so the
	// `*` behind the backslash is not a metacharacter (#1367).
	s.ValueBackslashInAPattern = interp.ValueBackslashQuotesWhatFollows
	// An empty positional list is a set parameter here, with zsh: measured
	// 2026-09-12, `set --; "${@-word}"` is empty and `"${@+word}"` is
	// `word`, where bash and ksh93 answer the other way round (#1941).
	s.PositionalListWithNoneIsSet = interp.Yes
	// The value is expanded and the redirection opened before the prefix is
	// checked, so a failure in either is what gets reported and the frozen
	// name is never named. Measured 2026-09-12, with ksh93 and zsh against
	// the three bash builds (#1943).
	s.PrefixToAFrozenNameIsCheckedFirst = interp.No
	// The mask reaches the trim, so an escaped IFS whitespace character
	// closing a `read` value is data and stays — this shell alone.
	// Measured 2026-09-12: `printf 'a b c\\ \n' | read x y` leaves `b c `
	// here and `b c` in the other five (#1360).
	s.ReadTrailingEscapedSeparator = interp.ReadTrailingEscapedSeparatorKept
	// `.` with no filename at all is not an error here: dash does nothing and
	// reports success, where the other three complain. Everything else about
	// `.` and `eval` is the POSIX answer, which dash keeps and the others have
	// each moved away from.
	s.DotWithNoOperandIsAnError = interp.No
	s.DotReadsOptions = interp.Yes
	// `eval` reads none, unlike `.` above: `eval -- echo hi` is `eval: --:
	// not found` at 127, so the marker is the command.
	s.EvalOptions = interp.EvalReadsNoOptions
	s.DotTakesTheSearchPathOption = interp.No
	// And a directory operand is no error either: measured, `. ./` is
	// silent at status 0, which zsh agrees with and bash and ksh93 do not.
	s.DotDirectoryOperandIsAnError = interp.No
	s.ExitTrapRunsOnSignalDeath = interp.No
	s.QuitIgnoredWhenNotInteractive = interp.No
	// unanswered QuitResetRestoresTheDefault: that axis is what a reset does
	// to the *ignore* above, and this shell has no ignore to take away — an
	// untrapped QUIT kills it whether or not `trap - QUIT` has been run, so
	// both readings run every script identically and there is nothing to
	// measure a preference from.
	s.HangupIsAnOrderlyExit = interp.No
	s.ExitInTrapReportsEarlierStatus = interp.Yes
	// `-n` is not an option here, so `kill -n 99` is the dash-word `n` read
	// as option letters: `kill: Illegal option -n` at 2, measured
	// 2026-09-17 on dash 0.5.12. And `-s` with nothing after it is its own
	// complaint, `No arg for -s option`, rather than the signal `s`.
	s.KillReadsTheNumberOption = interp.No
	s.KillOptionWithNoArgumentIsASignalName = interp.No
	s.KillListAcceptsName = interp.No
	// One subtraction, and everything else is refused — including 0, which
	// is the one column with no EXIT and no number printed back.
	s.KillListReducesRepeatedly = interp.No
	s.KillListPrintsANumberItCannotName = interp.No
	s.KillListNamesZeroAsExit = interp.No
	// And one name short of the machine's table on Linux: `kill -l 16` is
	// `16` here where bash, zsh and BusyBox ash write STKFLT, and
	// `kill -STKFLT` is `Illegal option -S`. Measured 2026-09-17 on dash
	// 0.5.12 in the panel's Alpine image. The number in range is still sent,
	// which is what separates this from the refusal a number out of range
	// draws at 2.
	s.SignalNamesTheShellLacks = "STKFLT"
	s.SIGPrefixAccepted = interp.No
	s.RedirectsUseEveryTarget = interp.No
	s.KillStatus = interp.KillStatusAnyFailure
	s.SubshellJobTable = interp.SubshellJobsCleared
	s.PrintfEmptyIsNotANumber = interp.No
	s.PrintfAbsentNumberIsAnEmptyOne = interp.No
	s.PrintfStarWithoutOperandIsRefused = interp.No
	s.PrintfStarComplaintCostsTheStatus = interp.Yes
	s.PrintfNonFiniteIsConverted = interp.Yes
	// No `'` flag: the character is the conversion this shell does not
	// have, and `%'d` is `printf: %': invalid directive` at 2.
	s.PrintfGroupingFlag = interp.No
	s.PrintfGroupingFlagAfterTheWidth = interp.No
	// A `*` beside a width's own digits is refused here too: `printf '%5*d' 4 42`
	// is a conversion character this shell does not have (#2824).
	s.PrintfStarBesideTheFieldDigits = interp.No

	// Refused at INT_MAX itself, as bash is, and with a wording and a
	// status of its own — see Diagnostics. Measured 2026-09-15.
	s.PrintfFieldBeyondAnInt = interp.PrintfFieldRefused

	// The column that splits the two routes: dash refuses the literal
	// `%21474836470s` and wraps the same number arriving through a star —
	// `printf 'A[%*s]B' 21474836470 x` is `A[x         ]B` at 0, the -10 an
	// int32 leaves. Measured 2026-09-15.
	s.PrintfStarBeyondAnInt = interp.PrintfStarWrapsToAnInt
	s.CaseSubjectKeepsThePreviousLine = interp.No
	// unanswered SubstringRangeThirdColonIsABadSubstitution: no substring
	// range at all here, so there is no third segment to refuse.
	s.PrintfReportsBadNumber = interp.Yes
	s.PrintfNumberOperand = interp.PrintfNumberLeadingNumber
	// Exact, as in bash: `printf '%d' 1000000000000000001` keeps its last
	// digit (#2907).
	// A flag past a field is no flag at all: the prefix ends there and the
	// byte arrives at the scan as the conversion character (#2910).
	// unanswered ArithDivisionByZeroYieldsAValue: the value a division by
	// zero leaves behind is only visible through a `printf` operand, and
	// this shell's printf evaluates no operand -- so the question has no
	// site here. Every other expression abandons the command, here as in
	// the two columns that do evaluate.
	s.PrintfFlagAfterTheField = interp.No
	s.PrintfIntegerOperandGoesThroughTheFloatingType = interp.No
	// C99's three, answering exactly as bash does on every row measured
	// 2026-09-14 — `%a` of 1.5 is `0x1.8p+0` and of 0.1 is
	// `0x1.999999999999ap-4`, the shortest run that names the value.
	s.PrintfC99FloatConversions = interp.Yes
	s.PrintfHexFloatDefaultIsTwelveDigits = interp.No
	// And a `%a`'s zero fill goes between the `0x` and the digits.
	s.PrintfHexFloatZeroFillPrecedesThePrefix = interp.No
	// unanswered PrintfRefusedOperandKeepsItsLeadingNumber: as in bash — the
	// reading above never evaluates an operand, so no arithmetic failure ever
	// reaches this question, and none reaches
	// PrintfFloatOperandIsEvaluatedTwice either.
	s.PrintfBackslashC = interp.PrintfBackslashCLiteral
	s.PrintfUnfinishedConversionIsAPercent = interp.No
	// No `\x` in a format at all: `printf 'a\x41Z'` is the six characters
	// as written, which is the whole panel's one holdout.
	// unanswered PrintfReportsAMissingHexDigit: there is no `\x` escape here
	// for a digit run to be empty after, so the complaint's site is out of
	// reach. The two columns that do reach it split, and
	// TestPrintfMissingHexDigit pins the pair (#3239).
	s.PrintfHexEscape = interp.PrintfHexEscapeAbsent
	// Nor in a `%b` argument, and neither spelling of the escape character.
	s.PrintfBHexEscape = interp.PrintfHexEscapeAbsent
	// No `\u` or `\U` at either site: `printf 'a\u0041Z'` is the ten
	// characters as written, as it is for this shell's `\x`.
	s.PrintfUnicodeEscape = interp.PrintfUnicodeEscapeAbsent
	s.PrintfBUnicodeEscape = interp.PrintfUnicodeEscapeAbsent
	// Neither spelling in a format either, which makes this the panel's one
	// column with no `\e` anywhere: `printf 'a\eZ'` is `61 5c 65 5a` in
	// 0.5.12 (#3225). XCU gives the format the XSI set and nothing else, so
	// this is the text read as written rather than an omission.
	s.PrintfEscEscape = interp.No
	s.PrintfCapitalEscEscape = interp.No
	s.PrintfBEscEscape = interp.No
	s.PrintfBCapitalEscEscape = interp.No
	// The octal needs no `\0` here, which is the one thing this shell and
	// bash agree on that ksh93 and zsh do not.
	s.PrintfBOctalWithoutZero = interp.Yes
	s.PrintfBStopIsPadded = interp.Yes
	// None: `%ld` is the conversion `l`, which dash does not have.
	s.PrintfLengthModifiers = interp.PrintfLengthModifiersAbsent
	// Bytes in every locale — dash decodes none — and no `l` to ask about:
	// `printf '[%.2s]' αβγ` is `[α]`, dash 0.5.12, 2026-09-16.
	s.PrintfFieldCountsCharacters = interp.No
	s.PrintfLongModifierCountsCharacters = interp.No
	// No `%(fmt)T`: `%(` is a directive this shell does not have.
	s.PidListingFinishesWithAJob = interp.No
	s.PrintfTimeConversion = interp.No
	s.PrintfTimeOperandIsADateString = interp.No
	s.PrintfQuote = interp.PrintfQuoteAbsent
	// C's `#` at a value of nought, which this shell hands straight to the
	// C library: `printf '%#x' 0` is `0`.
	s.PrintfAlternateFormAsksTheValue = interp.Yes
	// And counts it against the width: `printf '%#05x' 7` is `0x007`,
	// five characters, on dash 0.5.12.
	s.PrintfZeroFillCountsTheAlternatePrefix = interp.Yes
	// unanswered BraceRescanEntersFailedGroup: dash has no brace expansion,
	// so there is no scan to resume. `@{x}{a,b}@` is the one word it was
	// written as, and the question of how far past a group that did not
	// expand the scan steps cannot be put to a shell that never steps at
	// all. The nine `BraceRange…` and `BraceCharRange…` axes are unanswered
	// here for the same reason — `{1..}`, `{1..x}` and `{1..2..0}` are all
	// the words they were written as, and a shell that counts no range at
	// all has no reading of one it could not count.
	//
	// The three `$'…'` axes are left unanswered on purpose: dash has no
	// `$'…'` at all — `$'a\tb'` is the six characters it was written as,
	// dollar included — so the grammar refuses the form before any of them
	// can be asked. An answer here would be an invention.
	s.GetoptsAssignmentRestartsWord = interp.Yes
	// `kill %1` aims at the job's process group, which a script never has:
	// the monitor is off, the job leads no group, and the send is ESRCH.
	s.KillJobSpecAimsAtTheGroup = interp.Yes
	// A trim on `$@` runs over the whole list once, not over each field:
	// `set -- aa ab ba` makes `"${@#a}"` into `a ab ba` here and
	// `a b ba` in bash, zsh and ksh93.
	s.OperatorDistributesOverTheFieldList = interp.No
	// And nor does it over `"$*"`: `set -- aa ab ba` makes `"${*#a}"`
	// into `a ab ba` here and in zsh and BusyBox ash, against `a b ba` in
	// bash and ksh93. Measured 2026-09-16. Unanswered until now, so the
	// construct was a refusal in a shell that has it.
	s.OperatorDistributesOverStarSubscript = interp.No
	// OPTIND names the word *after* a cluster from its first letter on, so
	// `-abc` reads `a` with OPTIND already 2. The place inside the word is
	// kept somewhere a script cannot see. bash, ksh93 and zsh all leave
	// OPTIND naming the word until its last letter has been read.
	s.GetoptsCountsTheWordAtItsFirstLetter = interp.Yes
	// Counting a word early is the opposite end of the same question from
	// counting it late, and this shell is at the early end (#3275).
	s.GetoptsCountsTheWordOnTheNextCall = interp.No
	s.GetoptsEndOfOptionsNamesIt = interp.Yes
	// A `+`-prefixed word is an operand and ends the scan: measured
	// 2026-09-17 on 0.5.12, `getopts a o +a` is 1 with the name `?`.
	s.GetoptsTakesAPlusPrefixedOption = interp.No
	// A shell function call gets a `getopts` scan of its own here, while
	// OPTIND itself stays the shell's: a helper called twice reads its
	// arguments twice, and `$OPTIND` inside the call is still the caller's
	// number. Measured 2026-09-15 — `g() { while getopts ab o "$@"; do :;
	// done; }` called twice on `-a -b` sees both options both times, where
	// bash 5.3 and ksh93u+ see them once (#2944).
	s.GetoptsFunctionPosition = interp.GetoptsFunctionPositionIsTheCallsOwn
	// `#` in an option string is another option letter here. Measured
	// 2026-09-16, dash 0.5.12 (#2947).
	s.GetoptsOptionStringHasANumericType = interp.No
	// OPTERR is an ordinary variable here: measured 2026-09-17 on 0.5.12, a
	// bad option is one line on stderr with it set to 0 as with it set to 1.
	s.GetoptsOptErrSilencesTheComplaint = interp.No
	s.GetoptsClearsOptarg = interp.No
	// But OPTARG is *emptied* rather than unset when the option that was read
	// is one the string has and takes no argument, which `${OPTARG-…}` and
	// `${OPTARG+…}` tell apart — the two spellings a careful script uses to
	// ask whether the option it just read carried a value. zsh agrees here
	// and disagrees on the row above, which is why the two are two axes.
	s.GetoptsEmptiesOptargForAnArgumentlessOption = interp.Yes
	// A freeze on OPTARG stops the builtin: `readonly OPTARG; getopts a: o`
	// is `getopts: OPTARG: is read only` at status 2 with the name unset and
	// OPTIND still at 1 — so the word count had not moved and nothing after
	// the refusal ran. Measured 2026-09-16 on 0.5.12.
	// The last option's argument is still there when the scan runs out, which
	// dash shares with zsh alone — and not with BusyBox ash, which unsets it.
	s.GetoptsUnsetsOptargAtEndOfOptions = interp.No
	// And every clearing this shell does make is an ordinary write a freeze
	// refuses: `OPTARG=P; readonly OPTARG; set -- -z; getopts a:b o` leaves
	// `P` standing and the freeze on.
	s.GetoptsClearingOptargIsARealUnset = interp.No
	s.GetoptsOwnParametersIgnoreAFreeze = interp.No
	s.GetoptsRefusedWriteEndsTheBuiltin = interp.Yes
	// And a freeze on the **name** ends the builtin at 2 either way without
	// ending the script: measured 2026-09-17 on 0.5.12, `readonly N` over
	// `set -- -a v` and over `set -- x`.
	s.GetoptsFrozenNameAtTheEndOfTheOptionsIsFatal = interp.No
	// And so does `read`, on the same reading and with the same status: 2
	// wherever the frozen name stood, where bash answers 1 for the last one
	// (#3208).
	s.ReadRefusedWriteEndsTheBuiltin = interp.Yes
	s.ReadRefusedWriteIsOneOnTheLastName = interp.No
	// The builtin stops; the script does not. `echo reached` on the same
	// line runs, where `readonly x=1; x=2` ends this shell outright.
	s.ReadonlyRefusalInABuiltinIsFatal = interp.No
	// Measured 2026-09-12: `OLDPWD=/nonexistent dash -c 'echo $OLDPWD'` answers
	// the path it was given, and `cd -` then answers `can't cd to` it at 2.
	s.InheritedOldpwd = interp.InheritedOldpwdTaken
	// The one column of the panel with no depth at all: dash 0.5.12 under
	// `env -i` leaves `SHLVL` unset and exports nothing, and a shell it
	// starts counts from one. Not an omission here — see
	// interp.ShellLevelPolicy.
	s.ShellLevel = interp.ShellLevelNotCounted
	// dash names its starting directory by asking the kernel, whatever it was
	// handed, which is where it parts company with the other ash-derived
	// shell in the panel.
	s.StartupPwdName = interp.StartupPwdNameFromTheKernel
	s.CdWithoutHomeIsAnError = interp.No
	s.CdDashPrintsTheDirectory = interp.Yes
	s.PrintfAssignsWithV = interp.No
	s.PrintfRejectsUnknownOption = interp.Yes
	// dash has no options here at all, so every letter is refused.
	s.TrapParsesOptions = interp.Yes
	s.TrapPrintsWithP = interp.No
	s.TrapPrintsBareWithConditions = interp.No
	s.TrapPrintsBareWithP = interp.No
	s.TrapListsSignalsWithL = interp.No
	s.TrapOneArgumentIsACondition = interp.Yes
	s.TrapReportsAnUnknownSingleCondition = interp.Yes
	s.TrapSingleUnknownConditionIsUsage = interp.No
	// The sole holdout on the pseudo-conditions: `trap … ERR` is refused
	// with the same words any word that names no signal gets, and DEBUG
	// and RETURN with it. Measured, and the refusal does not end the
	// script — the status is 1 and the next command runs.
	s.TrapHasErrCondition = interp.No
	// unanswered FailingPipelineWhoseLastElementRanHere: every element of a
	// pipeline is a subshell here and nothing moves one into this shell, so a
	// pipeline is judged once by its status and the question is never put.
	// unanswered ErrTrapRefiresForTheCommandItFiredInside: the axis is how
	// many times one failure fires the ERR trap, and this shell has no ERR
	// trap for it to fire. The question is only reached where a trap with a
	// body exists, which is why the two axes above it are unanswered here
	// too.
	s.TrapHasDebugCondition = interp.No
	// No DEBUG condition, so no head to fire one at. Not an unanswered
	// axis — DebugTrapHeads has no unspecified value, because a head
	// either fires or does not and there is no third thing for a
	// dialect to be silent about.
	s.DebugTrapCompoundHeads = interp.DebugTrapHeadsNone
	// And a pipeline fires nothing either, because there is no condition to
	// fire: `trap … DEBUG` is refused here before a pipeline is reached.
	// The value is the absence of a pipeline rule rather than a reading of
	// one, which is what this shell would want if it ever had the trap.
	s.DebugTrapPipelines = interp.DebugTrapPipelineInEachElement
	s.TrapHasReturnCondition = interp.No
	// A subshell's listing shows only what survived the entry — the ignored
	// signals — in every boundary measured: `(trap)`, `$(trap)`, a pipeline
	// element and a background job all print nothing for a handled trap and
	// `trap -- '' INT` for an ignored one. The POSIX answer, restated here
	// because it was measured rather than assumed. Whether a kept listing
	// includes EXIT is left unanswered: nothing is ever kept to ask it of.
	s.SubshellKeepsTrapListing = interp.No
	s.PipelineElementKeepsTrapListing = interp.No
	s.BackgroundJobKeepsTrapListing = interp.No
	s.SubshellHidesInheritedIgnoredTraps = interp.No
	s.UmaskPrintsFourDigits = interp.Yes
	// dash parses no options for `alias`, so `-p` is a name there.
	s.AliasParsesOptions = interp.No
	s.AliasHasPrintOption = interp.No
	// Nor an `-x`, and for the same reason the line above gives: this
	// shell reads no options for `alias` at all, so `alias -x` is a
	// *name* here and the answer is `-x not found`.
	s.AliasHasExportOption = interp.No
	// It reads no options at all, so `-g` and `-s` are names it cannot
	// find rather than kinds it has.
	s.GlobalAliases = interp.No
	s.SuffixAliases = interp.No
	s.AliasListsAsDefinitions = interp.No
	s.AliasRestrictsToRegularKind = interp.No
	s.AliasOperandsCanBePatterns = interp.No
	s.AliasPlusPrintsNamesOnly = interp.No
	s.TypeNamesAnAliasOnlyWhenExpanded = interp.No
	s.AliasReportsNotFound = interp.Yes
	s.UnaliasReportsNotFound = interp.Yes
	s.AliasNotFoundStatusCounts = interp.No
	// A removed alias leaves nothing behind: `unalias h` twice is 0 then
	// 1 here.
	s.AliasRemembersTheNamesItNames = interp.No
	s.AliasSeparatorEndsTheLookup = interp.No
	s.UnaliasAllRefusesOperands = interp.No
	s.AliasQuoting = interp.ListingQuoteAlwaysDoubled
	s.AliasListingQuotesTheName = interp.No
	s.TrapQuoting = interp.ListingQuoteAlwaysDoubled
	s.TrapActionIsParsedWhenSet = interp.No
	s.TrapParseFailureNamesWhereItFired = interp.No
	s.SymbolicMaskTakesMoreThanOneOperator = interp.Yes
	s.SymbolicMaskWhoAloneSetsIt = interp.No
	s.SymbolicMaskTakesTheSetuidLetter = interp.Yes
	// Both, in silence — this shell sets and prints nothing.
	s.SymbolicMaskTakesAPermissionCopy = interp.Yes
	s.SymbolicMaskTakesTheConditionalExecuteLetter = interp.Yes
	s.SymbolicMaskTakesTheStickyLetter = interp.No
	// No options and no marker: `shift -x`, `shift -1` and `shift --` are all
	// numbers this shell calls illegal, which is the one wording it has here.
	s.ShiftOptionWords = interp.ShiftOptionWordsNone
	s.ShiftDoubleDashEndsOptions = interp.No
	// No marker in front of a numeric operand either: `break -- 1` is
	// `break: Illegal number: --` and the script ends there, which is the
	// same answer this shell gives `shift --`.
	s.NumericOperandDoubleDashEndsOptions = interp.No
	// The count is taken and the rest of the line is not read.
	s.ExtraNumericOperand = interp.ExtraNumericOperandIgnored
	s.ShiftNamesAreArrays = interp.No
	s.ShiftNegativeIsOutOfRange = interp.No
	s.WaitReadsOptions = interp.Yes
	// Only numbers, `%%`, `%+` and `%-` resolve here: a `%name` is a job
	// that is not there. `wait` complains about it with its own wording and
	// status, and there is no -n and no disown at all.
	s.JobSpecsByName = interp.No
	s.WaitReportsAMissingJob = interp.Yes
	// And a job it has already reported stays waitable by its process id.
	s.WaitRemembersAReapedJob = interp.Yes
	s.WaitNextJob = interp.WaitNextJobAbsent
	// Nor `-p`: `wait: Illegal option -p`. Measured 2026-09-13.
	s.WaitPNamesTheFinishedJob = interp.No
	// A trapped signal cuts a `wait` short with 128 plus the signal, and the
	// form that names a job answers the same as the bare one.
	s.WaitForAJobFailsWhenInterrupted = interp.No
	s.CommandRejectsUnknownOption = interp.Yes
	// And the word is a boundary around everything it runs: `eval 'export -q;
	// echo INNER'` stops the script and `command eval '…'` reports 2 and
	// carries on, with the same split on `${NOPE?bad}`, a readonly
	// reassignment and an unset name under `set -u`. The first of those is
	// caught here even though this shell reads `${x?word}` as a request to
	// stop elsewhere — ParamErrorIsAnExitRequest is yes and the subshell
	// still prints `alive` behind the word.
	s.FatalErrorEndsAtTheCommandWord = interp.Yes
	// Both string-ordering operators, which this shell has and the three
	// unary additions past the three-word rules it does not: `test -a f`,
	// `test -o errexit` and `test -N f` are all `unexpected operator` here.
	s.TestStringOrder = interp.TestStringOrderBoth
	s.GetoptsRejectsUnknownOption = interp.No
	s.ShiftCountIsArithmetic = interp.No
	s.TrapBodyRunsWhatParsed = interp.Yes
	s.ReportsAKilledCommandInACommandSubstitution = interp.Yes
	// The standard's reading, and dash takes it: a substitution's body is a
	// subshell environment with the shell's options in it. Measured
	// 2026-09-15, `set -e; echo "end[$(false; echo no)]"` is `end[]`.
	s.ErrExitEntersACommandSubstitution = interp.Yes
	s.UmaskSetWithSPrints = interp.No
	s.UlimitBlockIsKilobyte = interp.No
	s.UlimitHasResidentSet = interp.Yes
	s.UlimitHasProcessCount = interp.No
	s.UlimitSetsBothLimits = interp.Yes
	s.UlimitTakesHardKeyword = interp.No
	s.UlimitTakesSoftKeyword = interp.No
	s.UlimitOperandIsArithmetic = interp.No
	s.BadOptionToSpecialBuiltinFatal = interp.Yes
	// `alias` is no more special here than POSIX makes it: the complaint is
	// said and the next command runs. Measured with `alias -g x`.
	s.AliasBadOptionFatal = interp.No
	// unanswered AliasNameCheckReachesALookup, AliasInvalidNameFatal: this
	// shell checks no alias name at all, so AliasNameRefusedCharacters is
	// empty and neither question is ever reached. Measured 2026-09-12:
	// `alias 'a b'=echo` is accepted in silence here and the name is listed
	// back, where the two shells that check refuse it (#2413).
	// And so is a redirection that cannot be made, which is POSIX's rule
	// verbatim: `exec 3>/nope/x` stops the script, at 2 like every other
	// fatal error here.
	// The one shell in the panel that will not take a duplication target
	// wider than one digit — `>&10` and `>&08` alike, and `>&$n` once the
	// word has expanded to one. It words it as a syntax error and stops.
	s.MultiDigitDuplicationTargetIsAnError = interp.Yes
	s.RedirectErrorOnSpecialBuiltinFatal = interp.Yes
	// dash refuses the word while parsing — `Syntax error: Bad fd number`,
	// and nothing in the line runs. The grammar takes it here, so the
	// refusal is this axis and the wording is the ordinary one.
	s.GreatAmpTarget = interp.GreatAmpTargetIsADescriptor
	// dash has no move operator: `5-` is a word naming no descriptor and is
	// refused as one, with the same `Bad fd number` sentence and the same 2
	// that `<&qq` gets. Worded as a syntax error and raised at run time —
	// the same text inside `if false; then … fi` runs clean.
	s.FdMove = interp.FdMoveIsNotAnOperator
	s.DuplicationTargetError = interp.DuplicationTargetErrorEndsTheShell
	s.LocalOutsideAFunctionIsAnError = interp.Yes
	s.LocalOutsideAFunctionIsFatal = interp.Yes
	// A special builtin's failure is fatal, and a bad name is one — for all
	// three of them.
	s.BadNameToDeclarationFatal = interp.Yes
	s.BadNameToUnsetFatal = interp.Yes
	// `read` is not one of the three, so its bad name is reported at dash's
	// usual 2 and the script carries on.
	// unanswered BadNameToPrintfFatal: `printf` has no `-v` here, so no
	// output operand is ever judged — PrintfAssignsWithV is what stands in
	// front of it, and the option is refused before a name is read.
	s.BadNameToReadFatal = interp.No
	// And so is a readonly name it is asked to remove.
	s.UnsetReadonlyFatal = interp.Yes
	s.DeclarationNameOperands = interp.PlainNamesOnly
	s.UnsetNameOperands = interp.PlainNamesOnly
	s.ReadNameOperands = interp.PlainNamesOnly
	// No prompt operand: `read "v?p"` is `v?p: bad variable name`.
	s.ReadPromptOperand = interp.ReadOperandIsAllName
	// dash reads the line first and refuses afterwards, so the line is gone:
	// `printf 'AAA\nBBB\n' | { read 1bad; cat; }` prints only BBB.
	s.ReadRefusesABadNameBeforeReading = interp.No
	// ReadCountJudgesTheNamesAfterTheFirst is left unanswered: dash's
	// `read` has no count letter — `-n` and `-N` are both `Illegal option`
	// — so nothing here can ask it.
	// unanswered ExportThroughASubscriptedOperandRecordsTheLetter: no
	// arrays and no subscripted operand, so nothing reaches it.
	s.DeclarationTakesASubscript = interp.No
	// No arrays at all, so no subscripted operand is a name to any
	// declaration here either.
	// A fatal refusal takes the operands after it with it: measured
	// 2026-09-07, `export ok1=1 ":" ok2=2` leaves ok1 set and ok2 unset.
	// No arrays and no `read -a`, so neither edge question can be reached.
	// Answered anyway rather than left to refuse: the preset's reading is
	// the ordinary field split, which is this shell's everywhere else.
	s.ReadTrailingWhitespaceEndsAField = interp.No
	s.ReadNoFieldsIsOneEmptyElement = interp.No
	s.BadNameDeclaresTheOperandsAfterIt = interp.No
	s.TypesetTakesASubscript = interp.No
	// unanswered UnsetElementEmptiesAnUnwrittenArrayInASubshell: no
	// arrays, so no element for a subshell to remove.
	s.UnsetTakesASubscript = interp.No
	// No arrays at all, so a bracketed `read` operand is a word holding
	// characters a variable name may not hold: measured 2026-09-17,
	// `read 'r[2]' < in.txt` is `read: r[2]: bad variable name` at 2 with
	// nothing stored, and `r[b c]`, `r[]`, `r[@]` and `r[*]` all get that
	// one sentence — the same one the bare `1x` gets.
	//
	// unanswered StoreOperandWholeArraySubscript: the operand is refused as
	// a name before any subscript is read, so there is no whole-array
	// reading to have here.
	// unanswered StoreOperandWholeArraySubscriptOverATable: this shell has
	// no tables either, so the keyed half of that question is one further
	// out of reach again.
	s.StoreOperandTakesASubscript = interp.No
	// unanswered BadSubscriptToUnset: there is no subscript to evaluate here,
	// so the arithmetic the axis is about is never reached. Measured
	// 2026-09-17: `q=1; unset 'q[b c]'` is `unset: q[b c]: bad variable name`
	// and the script ends at 2 — the name is refused one complaint earlier.
	//
	// unanswered BadSubscriptToAnOutputOperand: the same wall one builtin
	// over. `echo Y | read 'q[b c]'` is `read: q[b c]: bad variable name` at
	// 2, so the store that walks the brackets is never reached either.
	//
	// unanswered BadSubscriptToADeclaration: and the third site does not
	// exist at all. Measured 2026-09-17: dash has no `declare` or `typeset`
	// (`declare: not found`, 127), and its `readonly` and `export` take a
	// name rather than a subscript — `readonly 'q[b c]'=v` is `readonly:
	// q[b c]: bad variable name` at 2, the same complaint as `unset`.
	//
	// unanswered BadSubscriptEscapesAnArithmeticCommand: there is no
	// `(( ))` grammar here at all — `(( 1 ))` is a subshell running the
	// command `1` — so no subscript ever fails inside one.
	//
	// unanswered ValuelessSubscriptedOperand: the same wall, one operand
	// shape over. `readonly 'q[b c]'` with no value is the same `bad
	// variable name` at 2, so there is no subscripted operand here to read
	// the brackets of.
	// A `jobs` listing: which end it starts from, and whether a job that
	// has already ended appears in it at all.
	s.JobsListNewestFirst = interp.Yes
	// And a stopped job keeps the current-job marker, which is a separate
	// question from the order the rows come out in: measured 2026-09-12
	// through a pseudo-terminal, `sleep 40` stopped with ^Z and then
	// `sleep 41 &` lists `[1] + Suspended` before `[2] - Running`. This
	// shell will not resolve `%+` or `%-` at all — both are `No current
	// job` — so the listing is the whole of the evidence here.
	s.StoppedJobTakesTheCurrentJobMarker = interp.Yes
	s.JobsListFinishedJobs = interp.Yes

	// `jobs`' letters: POSIX's pair and nothing else. `-r`, `-s`, `-n` and
	// `-x` are all "Illegal option" here, which is why the letter set is a
	// dialect answer rather than one string in the engine.
	s.JobsOptions = "lp"
	// unanswered JobsListsWhatChangedSinceTheLastReport: `jobs -n` is not a
	// letter this shell has — it is not in JobsOptions — so the axis is
	// never consulted here. ksh93 is the one column with the letter, and
	// bash's letter of the same name is a different question that stays
	// unimplemented (#3390).
	s.JobsPidsOnlyOption = interp.Yes

	// Whether a `&` job's command appears in a `jobs` listing.
	s.JobsShowBackgroundCommand = interp.No

	// Whether a backgrounded job is announced to whoever is typing.
	// Whether `export -f` carries a function to a child.
	s.ExportCarriesFunctions = interp.No
	// And whether it has `-n` at all. It does not: measured, dash answers
	// `export: Illegal option -n` and the script ends there, `export` being
	// a special builtin.
	s.ExportTakesTheAttributeOff = interp.No
	s.AnnouncesBackgroundJob = interp.No
	// And nothing with the monitor off either, which is the same answer
	// reached twice: this shell announces neither end of a job in any state
	// (#1738).
	s.AnnouncesBackgroundJobWithoutTheMonitor = interp.No
	// The hole gets refilled: measured 2026-09-12, three jobs with the middle
	// one killed and reaped and then a fourth started, `jobs %2` answers 0 and
	// there is no `%4`. `jobs %2` is 0 *before* the new job as well, so the
	// dead job kept the slot here until something reused it — which is why the
	// probe asks both and not only the second.
	s.NextJobNumberRefillsAHole = interp.Yes
	s.ReportsACommandKilledBySignal = interp.Yes
	s.ReportsAnyKilledPipelineElement = interp.Yes
	s.ChildInterruptEndsTheScript = interp.No
	// An empty operand and an empty HOME are both somewhere here: `cd ""`
	// and `HOME=; cd` say nothing and report 0, staying put.
	s.CdEmptyOperandIsAnError = interp.No
	s.CdEmptyHomeIsAnError = interp.No
	// And everything after the first operand is ignored rather than
	// refused: `cd alpha beta` goes to alpha and says nothing.
	s.CdSubstitutesTheOperands = interp.No
	s.CdSubstitutionPrintsTheDirectory = interp.No
	s.CdRefusesExtraOperands = interp.No
	s.CdRefusesUnknownOption = interp.Yes
	s.CdHasQuietOption = interp.No
	s.CdHasSymlinkFreeOption = interp.No
	s.CdLastPathOptionWins = interp.Yes
	s.BadSetOptionNameFatal = interp.Yes
	// And the letter too: `set -Z; echo one` prints nothing and exits 2.
	s.BadSetOptionLetterFatal = interp.Yes
	// This shell has no POSIX mode — `set -o posix` is `Illegal option -o
	// posix` — so the only door into the core's is argv[0] of `sh`, and it
	// is measured through that one: `sh -c 'set -o zzznosuch; …'` and the
	// same with `set -Z` both stop at 2, exactly as under its own name. The
	// mode moves nothing here, which is the answer written down rather than
	// left to a refusal (#2641).
	s.BadSetOptionNameFatalInPosixMode = interp.Yes
	s.BadSetOptionLetterFatalInPosixMode = interp.Yes
	// As in bash: `set -oe x` is `Illegal option -o x`, and `set -ozzznosuch`
	// with nothing behind it lists the options and then stops at `-z`.
	s.SetOLetterAttachesItsName = interp.No
	// Measured 2026-09-16: `dash --xtrace -c 'echo ran'` is `dash: 0: Illegal
	// option --`. It does not reach the name at all — the word is two dashes
	// and a tail, and the tail is never read.
	s.LongOptionNamesASetOption = interp.No
	// unanswered LongOptionNameIgnoresHyphens: the fold is a rule of the
	// `--name` spelling, and the axis above says this shell has no such
	// spelling. There is no site here to put the question to.
	// unanswered LongOptionValueIsANumber: the `=value` it reads rides on a
	// `--name` option word, and the axis above says this shell has no such
	// word. There is no site here to put the question to.
	// And it takes the next word whatever it looks like: `set -o -e` is
	// `Illegal option -o -e` at 2, with the dash word refused as the name.
	s.SetODeclinesADashWord = interp.No
	s.SetListsOptionsOnceAtTheEnd = interp.No
	// A bare `-` turns `-x` and `-v` off here, and a bare `+` is consumed with no effect (#2699).
	s.BareOptionWord = interp.BareDashClearsTraceAndVerbose
	// And it applies as it goes: `command set -e -Z` leaves errexit **on**,
	// which is why the unguarded `set -e -Z` ends the script — errexit was
	// already in force when the refusal failed the builtin.
	s.SetValidatesOptionLettersFirst = interp.No
	s.BadSetOptionNameAtInvocationExitsZero = interp.No
	// dash has no `[[ ]]` to ask it in; answered so that a shell built from
	// this preset with the construct turned back on is not left refusing.
	s.UnknownConditionOptionIsAStatus = interp.No
	s.ReturnOutsideAFunctionIsRefused = interp.No
	// `break` with no loop around it is ignored here, silently: measured,
	// `echo t; break; echo after` prints both and ends at 0, with no
	// wording to go with it.
	s.LoopControlOutsideALoopIsFatal = interp.No
	// The count is read first: `break abc` with no loop around it is
	// `break: Illegal number: abc` and the script ends, where this shell
	// has nothing at all to say about the place.
	s.LoopControlPlaceIsJudgedBeforeTheCount = interp.No
	// A call is a boundary and a subshell is not, which is the pairing that
	// makes these two fields rather than one: `f(){ break; }` called from a
	// loop leaves the loop running, and `( break )` inside one leaves only
	// the subshell. Silently in both cases — the complaint is
	// LoopControlOutsideALoop's, and this shell writes none.
	s.FunctionCallIsALoopControlBoundary = interp.Yes
	s.SubshellIsALoopControlBoundary = interp.No
	// A startup file is a sourced script, so a `return` in one is accepted
	// everywhere — the split is only over what its argument does, and this
	// shell keeps it: measured through a pty, an rc of `return 3` and one
	// of `false; return 3` both leave `$?` as 3 at the first prompt, where
	// bash leaves 0 and 1. A `return` with no argument means the last
	// command's status here as everywhere.
	s.StartupFileReturnCarriesItsArgument = interp.Yes
	s.LoneDashIsAnOption = interp.No
	// And a lone `+` is a name to `export` and `readonly` here as well —
	// measured 2026-09-12, `export +` is `export: +: bad variable name` at
	// 2. There is no `typeset` in this shell for the other half of the
	// question, so only this field is answered.
	s.SignAloneIsAnOptionWordToExport = interp.No
	s.UnsetFunctionChecksTheName = interp.No
	s.UnsetFunctionReportsMissing = interp.No
	s.UnsetReachesTheFunctionTable = interp.No

	// Whether a redirection target is expanded as an ordinary word.
	s.RedirectTargetIsAnOrdinaryWord = interp.No
	// And no pathname expansion: POSIX's rule, followed here. `cat <
	// only-*.txt` is `cannot open only-*.txt: No such file` at 2 with the
	// match sitting there, and the output side creates a file by that name
	// (#3207).
	s.RedirectTargetTakesPathnameExpansion = interp.No

	// Whether `type --` ends the options.
	// `type .` is `. is a special shell builtin` in dash 0.5.12, and `type
	// echo` is a plain one.
	// dash 0.5.12 ends the script from inside `( … )`, printing `start` and
	// exiting 2 where ksh93 carries on (#3274). The POSIX preset answers the
	// same way and this says so out loud, because an answer inherited in
	// silence is how six axes came to hold dash's value for another shell.
	s.SubstitutionParseErrorEscapesASubshell = interp.Yes
	// And from a here-document body: measured 2026-09-17 on dash 0.5.12,
	// `cat <<END` over a body holding `$(echo hi; for)` prints `start` and
	// exits 2, where bash 5.3.20 and ksh93u+ carry the script on (#3318).
	s.SubstitutionParseFailureInAHeredocBodyEndsTheShell = interp.Yes
	// The status is the refusal's own here. Measured 2026-09-17, the
	// substitution at the top level of a script, inside a file `.` read and
	// inside an `eval` argument all exit 2 — the column that says bash's 1 is
	// the borrowed route's doing rather than a different reading of the
	// failure (#3319).
	s.SubstitutionParseFailureCarriesTheFatalStatus = interp.No
	s.TypeDistinguishesSpecialBuiltins = interp.Yes
	// And `local` is on that side here, which POSIX's list does not have at
	// all. Measured 2026-09-16 against both builds — Apple's dash-16 and
	// 0.5.12 in `debian:stable-slim`, which agree line for line: `type
	// local` is `local is a special shell builtin` where `type echo` is
	// plain; `local qq` outside a function ends the script at 2; and inside
	// one, `LV=1 local x` leaves LV at 1 where `CV=1 command true` leaves CV
	// unset. The fatality was already right here and reached from somewhere
	// else, which is exactly the drift a membership closes (#3290).
	s.SpecialBuiltinsBeyondPosix = "local"
	s.TypePrintsFunctionBody = interp.No
	s.TypeEndsOptionsWithDashDash = interp.No

	// `local` reads no options at all here — `local -r x` declares a
	// variable named `-r` and then refuses it as the bad name it is — so
	// LocalOptions stays empty. A bare `local` writes nothing, and a bare
	// `set` lists the variables alone, every value single-quoted with an
	// embedded quote doubled out: `'quo'"'"'te'`.
	// `local -` is the one operand this shell's `local` reads as something
	// other than a name, and it reads it even though the builtin takes no
	// option letters at all: measured 2026-09-15, `f() { local -; set -f; };
	// f; echo $-` comes back without the letter.
	s.LocalDashSavesTheShellOptions = interp.Yes
	s.BareLocalListing = interp.BareLocalListsNothing
	s.SetListing = interp.SetListingAssignments
	s.SetListingQuoting = interp.ListingQuoteAlwaysDoubled
	// A descriptor number the process cannot hold is not checked here: with
	// `ulimit -n 6`, `exec 8>f` reports success and the descriptor is
	// unusable afterwards, where bash and ksh93 hand the kernel's refusal
	// back. Unreachable for a two-digit number, which this shell does not
	// read as one at all.
	s.FdNumberBoundedByOpenFileLimit = interp.No

	return s
}

// Diagnostics is how dash reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{
		// An assignment written in front of a command is traced on the
		// command's own line, exactly as the script spells it: `+ A=3 f zz`,
		// where bash writes two lines and ksh93 writes the command first.
		TracePrefixAssignment: interp.TracePrefixOnTheCommandLine,
		// This shell names the text a failure came out of in a *run-time*
		// diagnostic and not only in a parse failure, which is the panel's
		// fourth answer to that question and the only one the placement enum
		// below has to be asked for: `./s.sh: 3: ./p.sh: NOPE: parameter not
		// set` for a sourced file, and `./e.sh: 3: eval: …` for text `eval`
		// is running. EvalNaming and SourceFileNaming are left at their zero
		// value, SourceAfterLocation, which is where this shell puts it and
		// is already what its parse failures use (#1128).
		BorrowedTextIsNamedAtRunTime: true,
		// A duplication target wider than one digit is refused before the
		// descriptor is looked at, and worded as a syntax error even though
		// the parse succeeded — no number, no file, one sentence.
		MultiDigitDuplicationTarget: "Syntax error: Bad fd number",
		// And a target that is not a number at all gets the same sentence,
		// again with the parse already over: `echo A; echo hi >&qq` prints
		// `A` first and then this, and exits 2 without running what follows.
		// Neither the word nor the number appears in it.
		DuplicationTargetIsNotADescriptor: "Syntax error: Bad fd number",

		TypeKeyword:  "%[1]s is a shell keyword",
		TypeFunction: "%[1]s is a shell function",
		// `a is an alias for echo hi`, body raw.
		TypeAlias:     "%[1]s is an alias for %[2]s",
		CommandVAlias: "alias %[1]s=%[2]s",
		// No shell name and no location in front of it, alone among the
		// messages dash prints.
		TypeNotFound:           "%[1]s: not found",
		TypeNotFoundUnprefixed: true,
		// And on standard output, where the sentences about the names it
		// *could* account for go. dash has no option letters on `type` at
		// all, so `type -t f cd if ls` reads every word as a name and prints
		// five lines — two misses and three answers — in one stream, in
		// order. Measured: `type nope 1>/dev/null` prints nothing.
		TypeNotFoundOnStdout: true,
		// `command -V` says it the same way, shell's name and all.
		CommandVNotFound: "%[1]s: not found",
		// And a missing *command*'s status rather than a plain failure.
		TypeNotFoundStatus: 127,
		// `[1] + ` — a space each side of the marker — then a 27-wide state.
		// The command column is empty for a `&` job because dash kept no
		// text for one, which JobKeepsBackgroundCommand answers; a job dash
		// stopped itself does print its command here.
		JobLine: "[%[1]d] %[2]s %-27[3]s%[4]s",
		// `jobs -l`: the process id after the marker, and the state column
		// narrowed by exactly what the id took — dash is the one shell in
		// the panel that keeps the command column where it was. The 21 is
		// the measured width for a five-digit id; nothing sees the
		// difference, because dash's command column is empty for a `&` job
		// and what moves is trailing whitespace.
		JobLineLong: "[%[1]d] %[2]s %[3]d %-21[4]s%[5]s",
		// The words and nothing else — no process id, no command, and alone
		// among this shell's messages, no name and no line in front of it.
		KilledCommandNotice: "%[2]s",
		// The operator is part of the sentence here: this shell writes `-o`
		// whichever way it was asked.
		ParamNullOrNotSet:    "parameter not set or null",
		SetInvalidOptionName: "set: Illegal option -o %[1]s",
		// The sign is part of the sentence for the same reason `-o` is:
		// this shell writes `-q` for `set +q` as well as for `set -q`, so
		// the wording takes the bare letter and spells the dash itself.
		SetInvalidOptionLetter: "set: Illegal option -%[2]s",
		// A `--word` this front end could not place, and the one sentence in
		// the panel that names no word: measured 2026-09-16, `dash --badopt`,
		// `dash --xyz`, `dash --a` and `dash --login=x` each write exactly
		// `<shell>: 0: Illegal option --` at status 2 — the two dashes and
		// nothing after them. So the format has no verb in it and Wording
		// passes it through, and the `0:` is this shell's own unread-line
		// prefix rather than part of the sentence.
		InvocationBadLongOption: "Illegal option --",
		// Both 2, and both written down: see bash's pair for why a shell
		// that answers the two spellings alike still says so (#2629).
		SetInvalidOptionNameStatus:   2,
		SetInvalidOptionLetterStatus: 2,
		UnimplementedOptionLetters: map[string]string{
			// `set` letters dash has and this shell does not: -b job
			// notices, -i interactive, and the three about its line editor
			// and end-of-file — -E emacs, -V vi, -I ignoreeof. Measured
			// 2026-09-05 by asking dash for every letter of the alphabet in
			// both cases and both signs.
			//
			// `s` was the sixth and is gone: the letter abbreviates the
			// `stdin` name this dialect already lists, so it is declared in
			// the letter table Apply installs and moves the same state the
			// name moves (#3411).
			"set": "biEIV",
		},
		// Said whichever spelling asked, so the verb goes unused; status 0,
		// the field's default, is what makes it a remark rather than an
		// error.
		MonitorDenied: "set: can't access tty; job control turned off",
		// The same sentence without `set: ` in front of it, because nothing
		// asked: this one is the shell's own decision at startup rather than a
		// builtin refusing. It is located, like every other dash diagnostic —
		// `<name>: 0: …`, at the line it has not reached yet, which
		// InvocationNamesTheUnreadLine already spells.
		NoJobControlAtStartup: "can't access tty; job control turned off",
		// And it names the script it was handed rather than itself, which is
		// `$0` — measured, `-i script.sh` writes the script's path here and
		// `-i -c` and `-i -s` write dash's own. bash writes its own name on
		// all three.
		NoJobControlAtStartupNamesTheScript: true,
		// Backticks alone: this shell numbers a `$( … )` body from the file
		// like the other three, and a backquoted one from one.
		BackquotedSubstitutionRestartsLines: true,
		KilledCommandNoticeUnprefixed:       true,
		JobRunning:                          "Running",
		// dash names the signal that stopped it rather than calling it
		// stopped: `Suspended: 18`, where 18 is SIGTSTP.
		JobStopped: "Suspended: %[1]d",
		// ^Z prints the listing's own row, straight after the `^Z` the
		// terminal echoed — no newline of its own, which is where dash and
		// ksh93 part company with bash and zsh. Measured through a
		// pseudo-terminal on 2026-09-05.
		//
		// `fg` names the command alone, which is the empty default; `bg`
		// prints the job number and the command, with no marker and no `&`.
		JobResumedInBackground: "[%[1]d] %[3]s",
		JobDone:                "Done",
		JobExited:              "Done(%[1]d)",
		Location:               interp.LocationColonLine,
		// The one shell in the panel that tells neither failure from the
		// other: a script operand that is missing and one that will not open
		// share a wording and a status, and the status is the 2 it gives a
		// usage error rather than the 127 or 126 the other three reach for.
		// The reason is its own — `No such file`, from FileNotFound — and it
		// writes the line it has not reached yet, `<shell>: 0: cannot open …`,
		// which nothing else in the panel does.
		ScriptNotFound:               "cannot open %[1]s: %[2]s",
		ScriptNotFoundStatus:         2,
		ScriptNotReadableStatus:      2,
		InvocationNamesTheUnreadLine: true,
		// dash names what it wanted instead, and calls the token by its
		// class rather than by name.
		// A bad digit is not a diagnosis dash reaches at all: the literal
		// simply ends there and what follows is left over, so `08`, `09`,
		// `0778` and `0x1z` are all "expecting EOF" — the same sentence it
		// gives $((1 2)). Ours calls it a digit too great for its base and
		// dash words that as the leftovers it would have seen.
		DigitTooGreatForBase:   "expecting EOF",
		ArithOperandExpected:   "expecting primary",
		ArithOperatorExpected:  "expecting EOF",
		ArithMissingCloseParen: "expecting ')'",
		SyntaxUnexpected:       "Syntax error: \"%[1]s\" unexpected",
		SyntaxUnexpectedWord:   "Syntax error: word unexpected",
		// And the newline is unquoted with it — `newline unexpected` beside
		// `";;" unexpected` — and blamed on the line it ends (#1364).
		// A line typed at a prompt is numbered by the session: the second
		// line typed is line 2, for a parse failure and for a command that
		// was not found alike (#2022).
		PromptCountsTheSessionsLines:     true,
		SyntaxUnexpectedNewline:          "Syntax error: newline unexpected",
		UnexpectedNewlineIsOnTheNextLine: true,
		SyntaxExpecting:                  " (expecting \"%[1]s\")",
		// dash does not name the token when it is a redirection operator.
		SyntaxRedirectUnexpected: "Syntax error: redirection unexpected",
		ForName:                  "Syntax error: Bad for loop variable",
		Unterminated:             "Syntax error: end of file unexpected (expecting \"%[4]s\")",
		// With nothing open there is nothing it could have been expecting,
		// and the parenthetical goes rather than standing empty.
		UnterminatedNoConstruct: "Syntax error: end of file unexpected",
		UnmatchedQuote:          "Syntax error: Unterminated quoted string",
		UnmatchedBackquote:      "Syntax error: EOF in backquote substitution",
		UnmatchedCmdSubst:       "Syntax error: end of file unexpected (expecting \")\")",
		UnmatchedBraceSubst:     "Syntax error: Missing '}'",
		// `echo ${x` with the input running out at the newline after the
		// name is a line earlier here than `echo ${ echo hi` is, although
		// this shell has no command form and reads both as an ordinary
		// expansion. See UnmatchedBraceSubstDropsTheNameNewline for the
		// rows (#2232).
		UnmatchedBraceSubstDropsTheNameNewline: true,
		// This shell names the two characters that never came rather than
		// the one the other dialect echoes, so the closer is written in
		// rather than taken from the construct. It has no `$[` at all —
		// `echo $[1+2` runs and prints the text — so there is one spelling
		// to word here and not two.
		UnmatchedArithSubst: "Syntax error: Missing '))'",
		SyntaxError:         "Syntax error: %[1]s",
		BadSubstitution:     "Bad substitution",
		// The bare name and the shell's own sentence for a name nothing may
		// be assigned to: `${@:=w}` with no parameters is `@: bad variable
		// name`, and `${1:=w}` is `1: bad variable name`. It exits 2 where
		// the others exit 1, which FatalErrorStatusIsOne already answers
		// (#1541).
		AssignThroughExpansionBadName: "%[1]s: bad variable name",
		// The builtin in front, which its plain form does not have.
		ReadonlyVariableInDeclaration: "%[2]s: %[1]s: is read only",
		// And `local`, the third and last declaration this shell has:
		// measured 2026-09-07, `readonly x=1; f() { local x=2; }; f` is
		// `local: x: is read only`. It names all three of them, where bash
		// names only the two POSIX has not got (#1168).
		// And `getopts`, which names itself for a refused write to OPTARG,
		// to OPTIND or to the name it was given: measured 2026-09-16,
		// `readonly OPTARG; getopts a: o` is `getopts: OPTARG: is read
		// only`. It is the one entry here that is not a declaration utility,
		// which is the point — a builtin filling in its own output parameter
		// reports like one.
		// And `read`, which names itself the same way: measured 2026-09-16,
		// `a=A; readonly a; printf 'x\n' | read a` is `read: a: is read
		// only`.
		ReadonlyRefusalNamesBuiltin: map[string]bool{
			"export": true, "readonly": true, "local": true, "getopts": true,
			"read": true,
		},
		ReadonlyVariable: "%s: is read only",
		UnsetReadonly:    "unset: %s: is read only",
		InvalidNumber:    "Illegal number: %s",
		// A conditional with no `:` says which byte it wanted; a missing
		// value is the ordinary `expecting primary`, so ArithConditionalValue
		// stays empty.
		ArithConditionalColon: "expecting ':'",
		NumericArgument:       "%[1]s: Illegal number: %[2]s",
		ArithError:            "arithmetic expression: %[2]s: \"%[1]s\"",
		FileNotFound:          "No such file",
		TestNamesFirstOperand: true,
		// 2 rather than the 1 the other three report, for a read and a write
		// alike. Not fatal — the script carries on — so this is a different
		// question from FatalErrorStatusIsOne, which is about a failure that
		// ends the shell.
		RedirectFailureStatus: 2,
		// A verb, and a different one for each direction.
		CannotOpen:   "cannot open %[1]s: %[2]s",
		CannotCreate: "cannot create %[1]s: %[2]s",
		// Under `set -C` a target that is not a regular file is opened
		// rather than created — the file is already there — and dash is the
		// one shell whose verb follows: `cannot open d: Is a directory` with
		// the option on, `cannot create d: Is a directory` with it off,
		// measured 2026-09-10 against dash 0.5.12.
		NoclobberFallbackIsAnOpen: true,
		// The name twice — `dash: 1: echo: echo: I/O error` — and its own
		// fixed reason whatever the errno was, so the format never mentions
		// %[2]s. Measured on echo, printf, pwd and type: the doubling is the
		// builtin's, not the location's.
		BuiltinWriteError: "%[1]s: %[1]s: I/O error",
		// Its own text for ENOENT, and a different one each way: a read that
		// finds nothing is "No such file", a write that cannot make one is
		// "Directory nonexistent". The OS says "No such file or directory"
		// for both.
		DirectoryNotFound: "Directory nonexistent",
		ShiftTooMany:      "shift: can't shift that many",
		SyntaxErrorStatus: 2,
		// dash ends the script rather than reporting a status here, so only the
		// wording speaks — and it uses two, where the other three use one.
		//
		// The reason verb goes unused in the first: dash truncates strerror's
		// "No such file or directory" to "No such file" for this message
		// alone, so the text is spelled out rather than taken from the error.
		// An indexed format may ignore an argument, which is what makes that
		// safe.
		DotCannotOpen: ".: cannot open %[1]s: No such file",
		DotNotFound:   ".: %[1]s: not found",
		// A command word names no builtin; `exec` names itself.
		CannotExecute:     "%[1]s: %[2]s",
		ExecCannotExecute: "exec: %[1]s: %[2]s",
		ExecNotFound:      "exec: %[1]s: not found",
		// One message for either operator position.
		// dash names no pid at all, and prints a blank line after the one
		// message it has for a target — measured rather than assumed, because
		// an invisible trailing newline is exactly what a golden record is for.
		// No reason at all, so `cd` onto a file and `cd` onto nothing read
		// identically here — the one shell whose message cannot tell you
		// which it was.
		GetoptsBadOption:       "Illegal option -%[1]s",
		GetoptsMissingArgument: "No arg for -%[1]s option",
		// Nothing in front of it at all — the only diagnostic in the panel
		// that names neither the shell nor a line.
		GetoptsUnprefixed: true,
		// The usage line does carry a location, unlike the two above, and it
		// opens with a capital where bash's is lower case. Measured
		// 2026-09-14: `<script>: 1: getopts: Usage: getopts optstring var
		// [arg]`, the same `var` BusyBox ash writes (#2801).
		BuiltinUsage: map[string]string{
			"getopts": "getopts: Usage: getopts optstring var [arg]",
		},
		CdCannotChange:         "cd: can't cd to %[1]s",
		CdStatus:               2,
		PrintfBadNumber:        "printf: %[1]s: expected numeric value",
		PrintfIncompleteNumber: "printf: %[1]s: not completely converted",
		PrintfNumberOutOfRange: "printf: %[1]s: Result too large",
		// Every complaint about an argument is 2 here, as it is elsewhere.
		PrintfBadVerbStatus:     2,
		PrintfBadVerb:           "printf: %[2]s: invalid directive",
		PrintfMissingVerb:       "printf: missing format character",
		PrintfMissingVerbStatus: 2,
		PrintfBadOption:         "printf: Illegal option %[1]s",
		// A width or a precision past a C int, which dash reports from the
		// formatting call that failed rather than naming the number.
		// Measured 2026-09-15: `printf '[%21474836470s]' x` writes `[` and
		// then `printf: xvsnprintf failed` at 2.
		PrintfFieldBeyondAnInt:       "printf: xvsnprintf failed",
		PrintfFieldBeyondAnIntStatus: 2,
		UmaskBadMask:                 "umask: Illegal number: %[1]s",
		// No shell and no line in front of either, which dash does almost
		// nowhere else.
		AliasNotFound:             "%[1]s: %[2]s not found",
		AliasNotFoundUnprefixed:   true,
		UnaliasNotFound:           "%[1]s: %[2]s not found",
		UnaliasNotFoundUnprefixed: true,
		// dash quotes the whole argument back and does not say what in it
		// was wrong, so there is no operator wording to go with this.
		UmaskBadSymbolicMode: "umask: Illegal mode: %[1]s",
		UmaskBadOption:       "umask: Illegal option %[1]s",
		UmaskBadMaskStatus:   2,
		UlimitBadOption:      "ulimit: Illegal option -%[1]s",
		UlimitBadNumber:      "ulimit: bad number",
		// dash names neither the resource nor the operand, and reports 2.
		UlimitCannotChange:       "ulimit: error setting limit (%[3]s)",
		UlimitCannotChangeStatus: 2,
		UlimitBadNumberStatus:    2,
		BuiltinBadOption:         "%[1]s: Illegal option %[2]s",
		// The same sentence kill already had for its own missing argument,
		// measured for the builtins' shared reader with `read -p`.
		OptionNeedsArgument: "%[1]s: No arg for -%[2]s option",
		ShiftBadNumber:      "shift: Illegal number: %[1]s",
		WaitBadJob:          "wait: Illegal number: %[1]s",
		WaitBadJobStatus:    2,
		WaitNoSuchJob:       "wait: No such job: %[1]s",
		WaitNoSuchJobStatus: 2,
		KillNoSuchJob:       "kill: No such job: %[1]s",
		// The same shape for `jobs`, `fg` and `bg`, which is dash's house
		// order everywhere: the sentence first and the spec after it.
		// Measured on `jobs %9`, `fg %9` and `bg %9` — dash is the one shell
		// that reaches this for all three in a script.
		NoSuchJob: "%[1]s: No such job: %[2]s",
		// And dash's usage number rather than a plain failure, which is what
		// it reports for every one of the three.
		NoSuchJobStatus: 2,
		// dash is the one member that reads `fg` and `bg`'s operand before
		// noticing it has no job control, so it is the one that reaches a
		// resolved job and has to say why it will not resume it. Measured
		// 2026-09-13 from a script with no terminal, `sleep 0 & fg %1`.
		JobNotUnderJobControl:       "%[1]s: job %[2]s not created under job control",
		JobNotUnderJobControlStatus: 2,
		// A bare `fg` has no operand, and dash's message shows the null
		// pointer its formatter was handed rather than leaving a hole.
		AbsentJobSpec: "(null)",
		// And with nothing in the table to resolve to, a different sentence
		// again — measured on `fg` and `bg` alike in a script that started
		// no job.
		NoCurrentJob:          "%[1]s: No current job",
		NoCurrentJobStatus:    2,
		LocalOutsideAFunction: "local: not in a function",
		// One wording for all three, naming the part in front of any `=`.
		BuiltinBadName: map[string]string{
			"export":   "%[1]s: %[2]s: bad variable name",
			"readonly": "%[1]s: %[2]s: bad variable name",
			"unset":    "%[1]s: %[2]s: bad variable name",
			"local":    "%[1]s: %[2]s: bad variable name",
			// `read` says the same and carries dash's 2 with it, which is
			// how a caller tells it from the 1 that means end of input.
			"read": "%[1]s: %[2]s: bad variable name",
		},
		// A `local` name that starts with a digit is refused with the
		// builtin's name left off — `1y: bad variable name` against
		// `local: -r: bad variable name` — where its other bad names keep
		// it. Measured from both shapes.
		BuiltinBadNameNumeric: map[string]string{
			"local": "%[2]s: bad variable name",
		},
		BuiltinBadNameStatus: 2,
		// `ulimit -a`, row for row as the engine writes it.
		UlimitListing: []interp.UlimitListingRow{
			{Prefix: "time(seconds)        ", Res: interp.ResourceCPUTime, Scale: 1},
			{Prefix: "file(blocks)         ", Res: interp.ResourceFileSize},
			{Prefix: "data(kbytes)         ", Res: interp.ResourceData, Scale: 1024},
			{Prefix: "stack(kbytes)        ", Res: interp.ResourceStack, Scale: 1024},
			{Prefix: "coredump(blocks)     ", Res: interp.ResourceCore},
			{Prefix: "memory(kbytes)       ", Res: interp.ResourceResidentSet, Scale: 1024},
			{Prefix: "locked memory(kbytes) ", Res: interp.ResourceLockedMemory, Scale: 1024},
			{Prefix: "process              ", Res: interp.ResourceProcesses, Scale: 1},
			{Prefix: "nofiles              ", Res: interp.ResourceOpenFiles, Scale: 1},
			{Prefix: "vmemory(kbytes)      ", Res: interp.ResourceAddressSpace, Scale: 1024},
		},
		PrintfUsage:             "printf: usage: printf format [arg ...]",
		TrapBadSignal:           "trap: %[1]s: bad trap",
		TrapBadSignalUnprefixed: true,
		KillNoSuchProcess:       "kill: No such process\n",
		KillNotPermitted:        "kill: Operation not permitted\n",
		KillInvalidSignal:       "kill: invalid signal number or name: %[1]s",
		// `kill -l` is handed an exit status rather than a signal, and this
		// shell is the one that says so in the refusal.
		KillListBadNumber: "kill: invalid signal number or exit status: %[1]s",
		// The first character alone: dash stopped reading there.
		KillIllegalOption:         "kill: Illegal option -%[2]s",
		KillNotAPid:               "kill: Illegal number: %[1]s",
		KillMissingSignalArgument: "kill: No arg for %[1]s option",
		KillUsage: "kill: Usage: kill [-s sigspec | -signum | -sigspec] [pid | job]... or\n" +
			"kill -l [exitstatus]",
		// dash answers every complaint about its arguments with 2, which is the
		// one dialect where the option and the operand are not two questions.
		KillUsageStatus:     2,
		KillBadOptionStatus: 2,
		KillArgumentStatus:  2,
		// `[ -Q x -a -n x ]` is `[: -Q: unexpected operator` here, naming the
		// first of the two operands the primary is left with (#1290).
		TestUnknownLongOperator: interp.TestUnknownOperatorLeavesAnOperand,
		TestUnaryExpected:       "%[2]s: %[1]s: unexpected operator",
		TestBinaryExpected:      "%[2]s: %[1]s: unexpected operator",
		TestIntegerExpected:     "%[2]s: Illegal number: %[1]s",
		// No "too many arguments" sentence at all: a well-formed expression
		// with words left over is the *last word the parse took* called an
		// unexpected operator. `test a = b = c` is `test: b: unexpected
		// operator` and `test a b c d` is `test: a:` — one past the end of
		// what parsed, never the leftover itself. Measured 2026-09-16 on
		// Apple's dash-16 and Debian's dash 0.5.12, which agree.
		TestTooManyArguments: "%[2]s: %[1]s: unexpected operator",
		TestOperandExpected:  "%[2]s: argument expected",
		TestMissingBracket:   "[: missing ]",
		// Six decimal places, the most of any shell in the panel.
		TimesDecimals: 6,
		// dash hands the path to execve rather than checking first, so a
		// directory comes back as a permission error.
		DirectoryReason:     "Permission denied",
		ReadArgCount:        "read: arg count",
		OptionListingHeader: "Current option settings",
		OptionListingWidth:  16,
		// And the order, which is this shell's own option table rather than
		// a sort: the other two shells with a `set -o` listing sort their
		// names and this one does not, so `set -o | head` reads a different
		// line here. Measured 2026-09-13 on dash, `env -i dash -c 'set -o'`,
		// and again with three of them on — the order does not move with the
		// states.
		OptionListingOrder: []string{
			"errexit", "noglob", "ignoreeof", "interactive", "monitor",
			"noexec", "stdin", "xtrace", "verbose", "vi", "emacs",
			"noclobber", "allexport", "notify", "nounset", "nolog", "debug",
		},
		KillListing: interp.KillListingZeroFirst,
		// And when that directory was the PATH search's only match, dash
		// names it in the message and still numbers the failure 127.
		DirectoryOnPathStatus: 127,
	}
}

// Apply makes any adjustment that is not a vector value. dash needs none:
// it has `local`, which is the only builtin the panel disagrees about.
func Apply(r *interp.Runner) {
	// The prompt table, installed for the same reason the other three
	// dialects install theirs even though this one's has no escape language
	// in it: what a shell does to a prompt parameter before drawing it is the
	// dialect's answer, and "nothing but expansion" is an answer rather than
	// an absence. A runner told it draws `\u` as `\u` because this table says
	// so, not because nobody told it anything. #1455.
	r.SetPromptStyle(PromptStyle())
	// The three `set -o` names this shell has and the rest of the panel does
	// not, and all three are about **how the shell was started** rather than
	// about a behavior a script chose — which is why a script reads `set -o`
	// for them at all. Measured 2026-09-13: `interactive` is on under `-i`,
	// `stdin` is on when the program arrived on standard input, and `debug`
	// is a name this shell's shipped build lists and acts on no more than
	// this one does. All three are settable in both directions at 0, and the
	// first two carry the `$-` letter naming the same fact. See
	// interp's extraSetOptions, which holds what happens for each.
	//
	// Their absence was three rows missing from a seventeen-row listing
	// (#2624), and the two that describe the invocation are exactly the two
	// nothing else in the language can tell a dash script.
	// `emacs` and `nolog` are here rather than in the substrate's common
	// table since #3366: BusyBox ash has neither, so the table stopped
	// being unanimous. Both are in this shell's own listing, measured on
	// dash 0.5.12.
	r.AddSetOptions("interactive", "stdin", "debug", "emacs", "nolog")
	// And the letter that abbreviates the second of them. `set -s` is not a
	// request this shell has to build anything for: it is the `stdin` name
	// above, spelled short, and the letter and the name are one request with
	// one answer — measured 2026-09-17 from a script file under
	// `env -i PATH=/usr/bin:/bin`, `set -s` leaves `$-` as `s` and `stdin
	// on` in the listing, `set +s` empties both, and nothing else was
	// observed to move: the program is not re-read.
	//
	// Declared in this dialect's own letter table rather than in the shared
	// one because `s` is a letter the panel spells three different options
	// with, which is exactly what that table is for — ksh93 sorts its
	// operands with it (Semantics.SetSLetterSortsTheOperands), zsh means its
	// own invocation option, and bash refuses it outright. The shared
	// reading answers on that axis, which is `No` here, so before this the
	// letter reached the refusal `s` carried in
	// Diagnostics.UnimplementedOptionLetters above and a script that wrote
	// it ended at 2 where dash carries on (#3411). See
	// interp.Runner.SetOptionLetterNames.
	r.SetOptionLetterNames(map[rune]string{'s': "stdin"})
	// dash has no `builtin`.
	r.Unregister("builtin")
	// No `compgen` here; it is bash's alone.
	r.Unregister("compgen")
	r.Unregister("compopt")
	// fc really is an external here: `command -v fc` answers /usr/bin/fc.
	r.Unregister("fc")
	r.Unregister("complete")
	// And neither `mapfile` nor its other name; both are bash's alone.
	r.Unregister("mapfile")
	r.Unregister("readarray")
	// dash has neither `typeset` nor `declare`. The core provides `typeset`
	// because three of the four do; the one that does not takes it away, the
	// same way ksh93 takes `local` away.
	r.Unregister("typeset")
	// dash has no disown: real process groups or not, the name is simply
	// not a builtin there and resolves like any other missing command.
	r.Unregister("disown")
	// And no `let`. The other three evaluate arithmetic with it; dash has
	// only `$(( ))`, and reports `let: not found` like any other command it
	// has never heard of.
	r.Unregister("let")
	// This shell has no `enable` either.
	r.Unregister("enable")
}
