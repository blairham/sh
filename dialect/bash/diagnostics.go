// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Diagnostics is how bash 5 reports failure.
func Diagnostics() interp.Diagnostics {
	return interp.Diagnostics{ //nolint:gosec // G101: diagnostic text, not a credential
		// A math complaint raised by a builtin names it: `let '1+'` is
		// `bash: line 1: let: 1+: arithmetic syntax error: …`.
		ArithErrorNamesTheBuiltin: true,
		// The three loops POSIX has, spelled with this shell's own quoting.
		// It says this and carries on, which is the axis beside it.
		LoopControlOutsideALoop: "%[1]s: only meaningful in a `for', `while', or `until' loop",
		// A count that is a number and is not positive gets its own
		// sentence here, and this is the one column that then carries on:
		// `for i in 1 2; do break 0; echo tail; done; echo after` prints
		// `after` at status 0 with no `tail`, so the loop still ends and the
		// count was taken as 1. The other four end the script. A word that
		// is no number at all takes NumericArgument's sentence — `break:
		// abc: numeric argument required` — and ends the script here too
		// (#2800).
		LoopControlCountOutOfRange: "%[1]s: %[2]s: loop count out of range",
		// The builtin names itself, because bash's layout puts the shell and
		// the line in front of it: `bash: line 3: break: too many arguments`.
		NumericOperandTooMany: "%[1]s: too many arguments",
		TypeKeyword:           "%[1]s is a shell keyword",
		TypeFunction:          "%[1]s is a function",
		// The third sentence for an external, once the name is in the
		// command hash: measured 2026-09-18, `type ls` is `ls is /bin/ls`
		// on a first lookup and `ls is hashed (/bin/ls)` after an `ls` has
		// run, and `command -V` writes whichever of the two `type` writes
		// (#3579).
		TypeHashedExternal: "%[1]s is hashed (%[2]s)",
		// The only wording in the panel that carries its own quotes, and the
		// only verb that is not "alias".
		TypeAlias:     "%[1]s is aliased to `%[2]s'",
		CommandVAlias: "alias %[1]s=%[2]s",
		TypeNotFound:  "type: %[1]s: not found",
		// The same complaint from `command -V`, blaming `command`.
		CommandVNotFound: "command: %[1]s: not found",
		// The target as it was written, not as it expanded.
		AmbiguousRedirect: "%[1]s: ambiguous redirect",
		// The reading side of the csh spelling, which bash never opens as a
		// file: `cat <&qq` is the redirect being ambiguous, and `<&""` is a
		// descriptor that is not one — named as it was *written*, quotation
		// marks and all. bash 3.2 names the descriptor number instead, which
		// is a version difference rather than a dialect one; 5.3 is what this
		// preset is.
		DuplicationTargetIsNotADescriptor: "%[2]s: ambiguous redirect",
		EmptyDuplicationTarget:            "%[1]s: Bad file descriptor",
		// With a `{name}` in front the name is what is refused, and an
		// empty word is a duplication that failed (#6868).
		FdVariableDuplicationTargetIsNotADescriptor: "%[3]s: ambiguous redirect",
		FdVariableEmptyDuplicationTarget:            "%[2]s: Bad file descriptor",
		EmptyDuplicationTargetOnAnotherDescriptor:   "%[4]d: Bad file descriptor",
		JobStarted: "[%[1]d] %[2]d",
		// The one line all three members of the panel's bash write when an
		// interactive shell has no terminal to run the monitor on: 5.3.15 and
		// 3.2.57 as `bash`, and 3.2 run as `sh`, which writes it with its own
		// name in front. 5.3.15 writes a further line above it about the
		// terminal process group, and it is not reproduced — see
		// Diagnostics.NoJobControlAtStartup.
		NoJobControlAtStartup: "no job control in this shell",
		// And the line above it, which bash 5.3 writes and bash 3.2 does
		// not. Measured 2026-09-12 with no terminal on any of the three
		// standard streams, on `-ic` and `-lic` alike:
		//
		//	bash: cannot set terminal process group (91050): Inappropriate ioctl for device
		//	bash: no job control in this shell
		//
		// The errno half is fixed text rather than a rendered error, and
		// deliberately: this line is only ever written by a shell that has
		// no terminal, so the failure it describes is always the same one.
		// See Diagnostics.CannotSetTerminalProcessGroup (#1036).
		CannotSetTerminalProcessGroup: "cannot set terminal process group (%[1]d): Inappropriate ioctl for device",
		// And the word it writes as a prompt session ends, which is this
		// shell alone in the panel — measured at a pseudo-terminal on `^D`
		// and on a pipe with and without a trailing `exit`, the same line
		// each way. See Diagnostics.LeavingAPromptSession.
		LeavingAPromptSession: "exit",
		// And `logout` for a login shell, on the same routes. See
		// Diagnostics.LeavingALoginSession.
		LeavingALoginSession: "logout",
		// And what it says instead of leaving, under IGNOREEOF. Measured
		// 2026-10-06 through a pseudo-terminal. See
		// Diagnostics.EndOfInputRefused (#6239).
		EndOfInputRefused:                `Use "exit" to leave the shell.`,
		EndOfInputRefusedInALoginSession: `Use "logout" to leave the shell.`,
		// And the one route that says the same word with no prompt drawn at
		// all: `-i -c 'exit 3'` writes it and `-i script.sh` whose script
		// runs the same `exit` writes nothing, measured on 5.3.20 and
		// 3.2.57 alike. See Diagnostics.LeavingIsAlsoSaidOnTheseRoutes
		// (#4008).
		LeavingIsAlsoSaidOnTheseRoutes: syntax.RouteFromCommandString,
		// And those two lines alone name this shell by the last element
		// of the path it was started by, where every other diagnostic it
		// writes names the path — measured within one run of one binary.
		// See Diagnostics.NoJobControlAtStartupNamesTheBaseName.
		NoJobControlAtStartupNamesTheBaseName: true,
		// One sentence for both ends of the range, naming the word as
		// written. The difference between them is not the wording: a count
		// below zero is always complained about, and a count above `$#` is
		// silent until `shopt -s shift_verbose` — see
		// Runner.ReportsShiftPastTheEnd, which Configure turns off here
		// because this shell's default is the quiet one (#3465).
		//
		// Measured 2026-09-17 on bash 5.3.20 and 3.2.57 from a script file,
		// with the option on: `set -- a; shift 3` is `shift: 3: shift count
		// out of range` at 1 with `$#` untouched, and `shift` with nothing
		// left drops the slot rather than filling it — hence the second
		// wording. `shift 0` at `$#` of nought is 0 and silent, and so is a
		// count that lands exactly on `$#`, so the complaint really is the
		// range and not the emptiness.
		//
		// bash 3.2 names the marker where 5.3 names the count for
		// `shift -- 5`, which is a change within bash and is the `bash32`
		// column's to record rather than this preset's — the same age
		// `break -- -1` already has in the corpus.
		ShiftTooMany:            "shift: %[2]s: shift count out of range",
		ShiftTooManyWithNoCount: "shift: shift count out of range",
		ShiftNegativeCount:      "shift: %[2]s: shift count out of range",
		// Measured from a terminal: `[1]+` then two spaces, the state in a
		// 27-wide column, then the command — with the `&` back on it while
		// the job runs and gone once it has ended.
		ExpansionFailureStatusFromCommandString: 127,
		// The word, not the `${…}` inside it: `echo "[${x@QQ}]"` is refused
		// as `[${x@QQ}]: bad substitution`. The run of the word that shares
		// the expansion's quoting, and with the quotes off — `echo
		// 'lit'"${x@QQ}"` names `${x@QQ}` alone, where `echo
		// "pre${x@QQ}post"` names all of `pre${x@QQ}post`.
		BadSubstitution: "%[1]s: bad substitution",
		// An unterminated `${` in a here-document body, quoting the body.
		// Measured 2026-10-02 on bash 5.3.20 (#5379).
		HeredocBraceRanOutInTheName: "%s: bad substitution",
		HeredocBraceUnclosed:        "bad substitution: no closing `}' in %s",
		BadSubstitutionNames:        interp.NamesTheQuotingRun,
		// The run as a function listing writes it, and inside `$(( ))` the
		// expression instead. See the two fields.
		BadSubstitutionRunIsListed:                    true,
		BadSubstitutionInArithmeticNamesTheExpression: true,
		ParamNullOrNotSet:                             "parameter null or not set",
		// `${!v}` refusing its source, which 5.3 has and 3.2 does not; see
		// interp.Runner.refuseIndirection.
		IndirectionUndeclared: "%[1]s: invalid indirect expansion",
		IndirectionNotAName:   "%[1]s: invalid variable name",
		// The same sentence this shell writes for a name nothing declared,
		// and a different one from ksh93's — see the field.
		IndirectionUnaimedReference: "%[1]s: invalid indirect expansion",
		// The letter as the script spelled it, sign and all: `set +q` is
		// refused as `+q` here where dash and zsh write `-q` either way.
		// The refusal is this shell's ordinary bad-option complaint, so
		// `set`'s usage line follows it — and does not follow the different
		// sentence a bad `-o` name earns, which is why
		// SetInvalidOptionNameUsage is not set.
		SetInvalidOptionLetter: "set: %[1]s: invalid option",
		// The two spellings answer alike here, and they are written rather
		// than left to the zero so that the sameness is a measurement.
		// bash 3.2 is why that matters: the *same shell* three versions back
		// reports 1 for the name and 2 for the letter. Measured 2026-09-13
		// on 5.3.15 with `set -o zzznosuch` and `set -Z` (#2629).
		SetInvalidOptionNameStatus:   2,
		SetInvalidOptionLetterStatus: 2,
		// bash refuses a `set` option letter from its own command-line
		// parser: its name, the sentence, and the whole shell usage block
		// under it, with no location and no second name. The long spelling
		// goes to the builtin instead and reads as one — see
		// InvocationNameRefusalNamesTheShell.
		InvocationUsage: "Usage:\t%[1]s [GNU long option] [option] ...\n" +
			"\t%[1]s [GNU long option] [option] script-file ...\n" +
			"GNU long options:\n" +
			"\t--debug\n\t--debugger\n\t--dump-po-strings\n\t--dump-strings\n" +
			"\t--help\n\t--init-file\n\t--login\n\t--noediting\n\t--noprofile\n" +
			"\t--norc\n\t--posix\n\t--pretty-print\n\t--rcfile\n\t--restricted\n" +
			"\t--verbose\n\t--version\n" +
			"Shell options:\n" +
			"\t-ilrsD or -c command or -O shopt_option\t\t(invocation only)\n" +
			"\t-abefhkmnptuvxBCEHPT or -o option",
		// A `--word` this front end could not place: the same sentence a
		// refused letter gets, with the whole word in it, and the same
		// twenty-two-line block under it. Measured 2026-09-16 on bash 5.3.20
		// with standard input on /dev/null — `bash --badopt` and `bash -q`
		// each write one sentence and then the identical block, both at 2 —
		// where this shell wrote a single `unknown option "--badopt"` and no
		// block at all (#2298). The word is echoed whole: `--initfile` is
		// refused as `--initfile` and never as the `--init-file` it is one
		// character from.
		InvocationBadLongOption:            "%[1]s: invalid option",
		InvocationNameRefusalNamesTheShell: true,
		JobLine:                            "[%[1]d]%[2]s  %-27[3]s%[4]s",
		// `jobs -l`: the same 27-wide state column, with the process id
		// spending one of the two spaces after the marker rather than
		// pushing the rest of the row along.
		JobLineLong: "[%[1]d]%[2]s %[3]d %-27[4]s%[5]s",
		// The same twenty-seven-column state field the listing above uses,
		// with the process id in front of it rather than the job number: one
		// formatter, said in two places.
		KilledCommandNotice: "%5[1]d %-27[2]s%[3]s",
		// SIGTERM alone, and 5.3 alone: no location and no process id.
		KilledCommandNoticeBareForTerminate: "%-27[1]s%[2]s",
		JobRunning:                          "Running",
		JobStopped:                          "Stopped",
		JobDone:                             "Done",
		JobExited:                           "Exit %[1]d",
		// A job a signal ended is named by the signal, the host's words
		// and its number: `Terminated: 15`. Measured 2026-10-02 on bash
		// 5.3.20 (#5389).
		JobSignaled:              "%[1]s",
		WaitIsQuietForTerminate:  true,
		JobRunningShowsAmpersand: true,
		// ^Z prints the listing's own row — JobStoppedNotice is left empty
		// for that — under a newline of its own, because the terminal has
		// just echoed `^Z` where the cursor was.
		JobStoppedNoticeOnANewLine: true,
		// `fg` names the command alone, which is the empty default. `bg`
		// puts the row's head in front of it and the `&` after: measured,
		// `[1]- sleep 40 &`, with one space after the marker rather than the
		// two a listing row has.
		JobResumedInBackground: "[%[1]d]%[2]s %[3]s &",
		// `bg` on a job that is not stopped: the builtin is named in the
		// sentence and the job by number, and the status is **0** — the
		// shell complains and reports success, where zsh complains and
		// reports 1. Measured 2026-09-15 from a script with `set -m` on a
		// pseudo-terminal and again at an interactive prompt, the same both
		// times (#2838).
		JobAlreadyInBackground: "%[1]s: job %[2]d already in background",
		// No name and no punctuation before it, and the held `exit` reports
		// 1 — measured through a pseudo-terminal: `echo $?` after the
		// refusal says 1, where the ^D that was refused leaves the status
		// alone.
		StoppedJobsAtExit:       "There are stopped jobs.",
		RunningJobsAtExit:       "There are running jobs.",
		StoppedJobsAtExitStatus: 1,
		// No verb at all: the name, then the OS string. Same either way —
		// bash does not distinguish opening from creating.
		CannotOpen: "%[1]s: %[2]s",
		// The descriptor this shell picks for a `{name}` redirection is
		// announced separately when the process cannot hold it: one sentence
		// with no line in it, then the ordinary refusal above with the target
		// and the same errno. Measured 2026-09-22 under `ulimit -n 8`.
		FdPickedNumberUnusable:     "redirection error: cannot duplicate fd: %[1]s",
		RedirectWithoutATargetName: "file descriptor out of range",
		// And a duplication whose source is not open names the word the
		// script *wrote*: `n=10; echo x >&$n` is `$n: Bad file descriptor`
		// here where ksh93 and zsh both say `10`. The sentence is the
		// substrate's; only the naming moves (#734).
		NamesTheDuplicationTargetAsWritten: true,
		// A builtin whose write went nowhere: `echo hi >&-` is
		// `echo: write error: Bad file descriptor`, and the same shape with
		// printf, pwd or type in front — the builtin is a verb, not a set of
		// messages.
		BuiltinWriteError: "%[1]s: write error: %[2]s",
		// Measured: bash reports a failed open at the redirect's own line.
		RedirectFailureLine: interp.LineOfRedirect,
		// A `$( … )` body that will not parse costs 127 when the program was
		// a `-c` string, and 2 from a file or from standard input. See
		// interp.Diagnostics.SubstitutionParseFailureStatusFromCommandString
		// for the rows and the four controls (#4697).
		SubstitutionParseFailureStatusFromCommandString: 127,
		// And 1 for the same refusal, or a body the input ran out inside,
		// when the shell is interactive. See
		// interp.Diagnostics.SubstitutionParseFailureStatusFromAnInteractiveCommandString
		// (#6267).
		SubstitutionParseFailureStatusFromAnInteractiveCommandString: 1,
		// A here-document body that failed is reported where the reader had
		// got to, for a compound spelled with a reserved word. See
		// interp/heredoccommandend.go (#4690, #4712).
		HeredocBodyOnAReservedWordCompoundIsLocatedWhereTheCommandEnds: true,
		CannotCreate:            "%[1]s: %[2]s",
		NoclobberRefusal:        "%[1]s: cannot overwrite existing file",
		NamesTheInputInLocation: true,
		// The newlines between a `$(` and the first command of its body
		// count for nothing here, so that command is on the opener's line —
		// `echo "$(` on line 2 with the command under it reports `$LINENO`
		// as 2 where zsh, ksh93 and dash all report 3, and three blank lines
		// between them still report 2. Measured 2026-09-17 (#3362).
		CommandIsLocatedWhereItsFirstWordEnds:                 true,
		HiddenSubstitutionIsRefusedBelowItsLine:               true,
		SubstitutionBodyIsNumberedFromWhereTheShellWasReading: true,
		// And a *backquoted* body's refusal is placed at the failure's own
		// file line plus the newlines inside the backquotes, where the
		// substitution stands in the command's first token: `` v=`echo hi⏎for`
		// `` opening line 2 is reported at line 4 where `` cat `echo hi⏎for` ``
		// is reported at 3, and `` v=1 w=`echo hi⏎for` `` is 3 again.
		// Measured 2026-09-18 over forty-four shapes (#3553).
		BackquotedSubstitutionFailureAddsItsBodysNewlines: true,
		EchoesTheOffendingLine:                            true,
		// A substitution body read at expansion time and refused is named
		// as the construct it came from, which goes with the failure being
		// the word's rather than the script's — see
		// interp.Semantics.SubstitutionParseErrorIsFatal (#2703).
		SubstitutionParseFailureNamesTheConstruct: true,
		// And a declaration's quoted `( … )` value that will not come out as
		// an array literal is named the same way, for the same reason: the
		// re-read is its own input, so the script's line is no longer the
		// whole story. This column alone re-reads such a text at all — see
		// interp.Semantics.DeclarationRereadsAParenthesizedValue — so the
		// wording is the switch as well as the sentence (#4035).
		QuotedArrayLiteralFailureNames: "array assign",
		// And the sentence for a subscripted operand whose value is
		// parenthesized, which bash declines to re-read and says so about.
		// Measured 2026-09-21: `declare a[1]="(var)"` writes
		// `warning: a[1]=(var): quoted compound array assignment deprecated`
		// and stores the five characters at 1. See
		// interp.Runner.warnQuotedCompoundAtASubscript.
		QuotedCompoundArrayAssignmentDeprecated: "warning: %[1]s: quoted compound array assignment deprecated",
		// A script operand it could not read, worded the same way as `.` and
		// as a redirection: the path, then the operating system's own text.
		// The two numbers are the measurement — 127 for a path that names
		// nothing, the number a missing command carries, and 126 for one that
		// is there and will not open, the number an unrunnable one carries.
		// Measured on a missing path, a missing parent, a dangling symlink, a
		// mode-000 file and a directory.
		ScriptNotFound:          "%[1]s: %[2]s",
		ScriptNotFoundStatus:    127,
		ScriptNotReadableStatus: 126,
		// And this shell alone names the operand *twice* once it has the
		// file open: `bash ./adir` is `./adir: ./adir: Is a directory`
		// where `bash ./nosuch` and a mode-000 file both open with the
		// shell's own name. See ScriptOperandNamedByItselfOnceOpened.
		ScriptOperandNamedByItselfOnceOpened: true,
		// And what it says about one it read and would not run: `bash lsbin`
		// where lsbin is a copy of `/bin/ls` is `<path>: <path>: cannot
		// execute binary file` at 126, with no errno behind it — nothing
		// failed to open. See ScriptBinaryContent.
		ScriptBinaryContent: "%[1]s: cannot execute binary file",
		// `bash -c` with nothing behind it. The rest of the panel words it
		// differently enough that nothing here is shared; see the field.
		InvocationMissingOptionArgument: "%[1]s: option requires an argument",
		// A `#!` line naming an interpreter that is not there, which this
		// shell reads the file to find out about. `./x23` whose first line
		// is `#!nosuchfile` is `<$0>: ./x23: nosuchfile: bad interpreter:
		// No such file or directory`, and with no line in it — measured
		// beside a plain missing command from the same script, which keeps
		// one.
		BadInterpreter:                   "%[1]s: %[2]s: bad interpreter: %[3]s",
		BadInterpreterLocatedByNameAlone: true,
		SelectPrompt:                     "#? ",
		Location:                         interp.LocationLineWord,
		// And no line at all at a prompt: measured 2026-09-11 under `-i`,
		// `if; then` is `bash: syntax error near unexpected token `;'` where
		// the same line in a script is `s.sh: line 1: …`. The line *inside* a
		// sentence stays — `unexpected end of file from `if' command on line
		// 1` is what it writes at a prompt too — which is why this is the
		// location and not a second wording.
		PromptLocation: interp.LocationNameOnly,
		// And a builtin's complaint drops the line there too, which needs
		// saying separately because BuiltinLocation is a field of its own:
		// measured 2026-09-11 under `-i`, `cd /nope` is `bash: cd: /nope: No
		// such file or directory` where the same line in a script names one
		// (#2024).
		PromptBuiltinLocation: interp.LocationNameOnly,
		// And a command string run by `-i` is located the same way. See the
		// field.
		InteractiveCommandStringIsLocatedAsAPrompt: true,
		NotFound: "%s: command not found",
		// The four restricted-shell refusals, measured a spelling at a time
		// on bash 5.3.20, 2026-09-22. Each reports 1 and leaves the shell
		// running; see interp/restricted.go.
		Restricted:            "%[1]s: restricted",
		RestrictedOperand:     "%[1]s: %[2]s: restricted",
		RestrictedCommandName: "%[1]s: restricted: cannot specify `/' in command names",
		RestrictedRedirect:    "%[1]s: restricted: cannot redirect output",
		// The parameter spelling of the hash names no builtin, which is
		// measured: `BASH_CMDS[a]=zz` in a restricted shell is `zz: not
		// found` where `hash -p zz a` is `hash: zz: not found`.
		RestrictedHashNotFound: "%[1]s: not found",
		UnboundVariable:        "%s: unbound variable",
		// And the **count** of a whole array names the bare name where every
		// other subscripted refusal here writes the brackets back:
		// `${#a[@]}` is `a: unbound variable` and `${a[@]}` on the same
		// unset name in bash 3.2.57 is `a[@]: unbound variable`. Measured
		// 2026-09-18 on bash 5.3.20 and bash 3.2.57, from a script file
		// under `set -u` (#3125).
		WholeArrayCountNamesTheBareName: true,
		// The sigil written back, which no other column does: `${@:=w}`
		// with no parameters is `$@: cannot assign in this way`, and
		// `${1:=w}` names `$1`. Identical in 3.2.57 and in the same binary
		// under argv[0] of `sh` (#1541).
		AssignThroughExpansionBadName: "$%[1]s: cannot assign in this way",
		UnboundPositional:             "$%s: unbound variable",
		NumericArgument:               "%[1]s: %[2]s: numeric argument required",
		// A subscript before the first element, named as it was written:
		// `a[x-2]`, not the -1 it evaluated to. Identical in bash 3.2.
		BadArraySubscript:                  "%[1]s[%[2]s]: bad array subscript",
		SubscriptBeforeTheFirstElementRead: "%[1]s: bad array subscript",
		// The length's subject is the subscript as written, brackets and
		// all, where the read's is the array's name.
		SubscriptBeforeTheFirstElementLength: "[%[1]s]: bad array subscript",
		CannotConvertTableToArray:            "%[2]s: %[1]s: cannot convert associative to indexed array",
		CannotConvertArrayToTable:            "%[2]s: %[1]s: cannot convert indexed to associative array",
		// One verb rather than two: the literal form's complaint comes from
		// the assignment and names no builtin. See
		// Semantics.TableUnderAnArrayLiteralDeclaration.
		CannotConvertTableToArrayAtTheAssignment: "%[1]s: cannot convert associative to indexed array",
		CannotConvertArrayToTableAtTheAssignment: "%[1]s: cannot convert indexed to associative array",
		EmptyAssociativeKeyRead:                  "%[1]s: bad array subscript",
		EmptyAssociativeKeyLength:                "[%[1]s]: bad array subscript",
		ArithEmptySubscript:                      "%[1]s[]: bad array subscript",
		// And written twice for every read, measured 2026-10-04 on 5.3.20:
		// `$(( a[] ))` is two lines, `$(( a[] * a[] ))` four.
		ArithEmptySubscriptWrittenTwice:       true,
		ArithEmptySubscriptTarget:             "`%[1]s[]': not a valid identifier",
		ArrayAttributeNotRemovable:            "%[2]s: %[1]s: cannot destroy array variables in this way",
		BareElementInASubscriptedTableLiteral: "%[1]s: %[2]s: must use subscript when assigning associative array",
		// A declaration's operand names the value it came to, single-quoted,
		// where the assignment above names the text as written.
		BareElementInASubscriptedTableLiteralOperand: "%[1]s: '%[2]s': must use subscript when assigning associative array",
		EmptyKeyInATableLiteralPair:                  "%[1]s: bad array subscript",
		EmptyKeyInATableLiteralElement:               "%[1]s: bad array subscript",
		// A declaration's operand with a value complains about the
		// *subscript*, in the words the read of one gets; with no value the
		// whole operand is refused as a name, builtin and all. Measured
		// 2026-09-17 on bash 5.3.20, a script file: `typeset 'a[]'=v` is
		// `a[]: bad array subscript` and `typeset 'a[]'` is ``typeset:
		// `a[]': not a valid identifier``, both at 1 with the rest of the
		// line still running.
		DeclarationEmptySubscript:          "%[1]s[]: bad array subscript",
		ValuelessDeclarationEmptySubscript: "%[2]s: `%[1]s[]': not a valid identifier",
		SubscriptedPrefixIsNotAName:        "`%[1]s': not a valid identifier",
		ArithWholeArraySubscript:           "%[1]s[%[2]s]: bad array subscript",
		ArithSubscriptUnclosedQuote:        "%[1]s[%[2]s]: bad array subscript",
		// The store's own sentence, which is the builtin's complaint about
		// its whole operand and not the subscript's: measured 2026-09-20 on
		// 5.3.20 from a script file with `typeset -A a; k="q'r"; a[$k]=4`,
		// `let "a[$k] = 9"` writes this once and no `bad array subscript`
		// at all, where `let "x = a[$k] + 1"` writes that one twice and
		// this one never. The leading backquote is bash's own (#3870).
		ArithSubscriptUnclosedQuoteTarget: "`%[1]s[%[2]s]': not a valid identifier",
		ArrayLiteralThroughASubscript:     "%[1]s[%[2]s]: cannot assign list to array member",
		// Through a literal the element is named as it stands between the
		// parentheses, with no array name in front of it. bash 3.2 says the
		// same and does not end the script, which is the one place the two
		// builds differ here.
		BadArrayLiteralSubscript: "[%[2]s]=%[3]s: bad array subscript",
		// Reached from `unset` the same boundary drops the array's name and
		// keeps the bare subscript, with the builtin named in front. bash 3.2
		// words it without the builtin — and refuses every negative subscript
		// besides, which is an absence rather than a wording; the preset
		// follows 5.3.
		UnsetSubscriptBeforeTheFirstElement: "unset: [%[2]s]: bad array subscript",
		// `unset a[@]` where `a` holds a scalar. Identical in bash 3.2.
		UnsetNotAnArray: "unset: %[1]s: not an array variable",
		ArithError:      `%[1]s: %[2]s (error token is "%[3]s")`,
		// The expression is quoted back from its first non-blank
		// character, and a construct names itself in front of it.
		// A conditional missing either of its two values is a sentence of its
		// own here, apart from the ordinary `operand expected`: `$(( 1 ? ))`
		// and `$(( 1 ? 2 : ))` are both `expression expected`.
		ArithConditionalThen:        "expression expected",
		ArithConditionalElse:        "expression expected",
		ArithConditionalColon:       "`:' expected for conditional expression",
		ArithErrorSkipsLeadingSpace: true,
		ArithErrorNamesTheConstruct: true,
		// The bytes an expansion put inside brackets the script wrote come
		// back escaped, and the expression bash quotes back shows them that
		// way: with `declare -A assoc; key='x],b[$(echo 9)'`, the refusal
		// `(( 'assoc[$key]++' ))` earns names `'assoc[x\],b\[\$(echo 9)]++'`.
		ArithValueShownEscaped: true,
		DivisionByZero:         "division by 0",
		ArithNegativeExponent:  "exponent less than 0",

		// bash reserves its generic arithmetic wording for operands that are
		// not literals, so a bad digit gets a reason of its own.
		DigitTooGreatForBase: "value too great for base",
		// The third of bash's three numeral sentences, for a named base with
		// nothing after its `#`. Measured 2026-09-28 in 5.3.20 and in the
		// pinned 5.3.15 image alike (#5061).
		ArithEmptyBaseDigits: "invalid integer constant",
		// A byte that is no digit in any base is a second sentence here:
		// `010#5` and `0#5` are `invalid number` where `08#5` and `1@2` are
		// the one above. See Diagnostics.ArithByteIsNoDigit.
		ArithByteIsNoDigit: "invalid number",
		// And a base outside 2..64 is a third: `$(( 1#0 ))`.
		ArithInvalidBase: "invalid arithmetic base",
		// A call the shell will not enter because too many are already
		// active, at whatever bound `FUNCNEST` set. Measured 2026-09-23 on
		// 5.3.20 with `FUNCNEST=5`.
		FunctionNestingLimit: "%[1]s: maximum function nesting level exceeded (%[2]d)",
		// A function name is written back bare, whatever is in it, and the one
		// shape a bare header could not read back takes the keyword instead:
		// `function a=2 () `, where `a=2 () ` would be an assignment followed
		// by a group. Measured 2026-09-23 on 5.3.20 — see
		// interp.FunctionListingNameSpelling for the panel and for `=x`, which
		// is the control.
		FunctionListingNameSpelling:         interp.FunctionListingNameIsBareWithAKeywordForAnAssignment,
		FunctionListingAssignmentNameHeader: "function %[1]s () \n%[2]s",
		ArithRecursionLimit:                 "expression recursion level exceeded",
		ArithErrorNamesThePrefix:            true,
		// set -o pads to fifteen and tabs; kill -l numbers five to a row.
		// The width is named rather than written, because `shopt -o -s`
		// writes this listing narrowed and has to pad it the same way.
		OptionListingWidth:          setOptionListingWidth,
		OptionListingTabbed:         true,
		KillListing:                 interp.KillListingNumbered,
		TraceQuoting:                interp.QuoteShell,
		DiagnosticNamesAWordEscaped: true,
		// And the widest character set of the three, with a position rule
		// the other two do not have: `~a` and `#a` are quoted and `a~b` and
		// `a#b` are not, so the tilde and the hash count only where they
		// would have started an expansion or a comment. `^` and `!` are
		// quoted anywhere, which is bash alone, and `=` is quoted nowhere,
		// which is also bash alone. bash 3.2.57 agrees on every row, so this
		// is not a version split. Measured 2026-09-12 over 36 words.
		TraceMetacharacters: interp.TraceMetacharacters{
			Anywhere: "*?[]{}^!",
			Leading:  "~#",
		},
		TraceForHeader: interp.TraceForSource,
		// The header as written, once, before the subject is expanded — the
		// same reading the `for` header gets, and from the same field the
		// parser keeps for it.
		TraceCaseHeader: interp.TraceCaseSource,
		// An appending prefix is traced as the value it came to and with no
		// operator — `v=14; v+=5 cmd` is `+ v=145` — where a bare `w+=2`
		// keeps both. This shell alone; see interp/xtraceprefix.go.
		TracePrefixAppendIsTheJoinedValue: true,
		// A condition's operands are *not* quoted here, which is the one
		// place bash's two trace renderings part: `x="a b"; echo "$x"` traces
		// `echo 'a b'` and `[[ $x == y ]]` traces `[[ a b == y ]]`. Measured
		// 2026-09-12 on 5.3.15 and 3.2.57 alike; the zero value says it, and
		// it is written out because the neighboring TraceQuoting says the
		// opposite two lines up.
		TraceConditionQuoting: interp.QuoteNever,
		// The panel's only shell that says how deep the text it is reading
		// came from: `set -x; eval :` traces `+ eval :` and then `++ :`.
		// Measured 2026-09-11 on 5.3.15 and 3.2.57 alike.
		TracePrefixRepeatsAtIndirection: true,
		// An empty *value* is written bare here where an empty argument is
		// written as two quotes — see TraceEmptyAssignmentValueIsBare.
		TraceEmptyAssignmentValueIsBare: true,
		// `export ev=1` is traced twice — `+ export ev=1` and then `+ ev=1`
		// — and `typeset -x tx=1` once, on the same binary in the same run,
		// so it is the command word and not the export attribute that
		// decides. `local` and `declare -x` are one line too. Measured
		// 2026-09-19 on 5.3.20 and 3.2.57 alike; one line per operand, in
		// order, so `export e1=1 e2=2` is three lines. See
		// interp/xtracedeclaration.go.
		TraceRepeatsAScalarOperandAfter: []string{"export", "readonly"},
		// An operand whose value is a parenthesized list goes the other way:
		// it is taken off the command line and written in front, where a
		// scalar stays where it was. `typeset b=(3 4)` is `+ b=('3' '4')` and
		// then `+ typeset b`, measured 2026-09-19 on 5.3.20 and 3.2.57 alike,
		// and every element is quoted whether or not it needs to be — which
		// is what the second field says. See interp/xtracearrayoperand.go.
		TraceDeclarationArrayOperand:        interp.TraceOperandSplitBefore,
		TraceArrayOperandQuotesEveryElement: true,
		// And a `$'…'` in a **bare** literal is written as the characters it
		// stands for: `a=( $'\t' )` traces a literal tab inside single
		// quotes. Measured 2026-09-22 on 5.3.20; see the field.
		TraceArrayLiteralDecodesAnsiCQuoting: true,
		// bash names the construct and the line it opened on, and nothing
		// about what would have closed it.
		EvalNaming:       interp.SourceBeforeLocation,
		SourceFileNaming: interp.SourceReplacesShell,
		// Runtime diagnostics move with the source too: a failure inside a
		// sourced file, or inside a function defined in one, names that file
		// as written — `./inc.sh: line 1: nosuch: command not found` — where
		// dash and ksh93 keep the script's own name.
		LocationNamesTheCurrentFile: true,
		UnterminatedEndsOnNextLine:  true,
		// A substring range puts the parameter in front of the sentence, where
		// the same shell blames a bad *subscript* on the expression alone.
		SubstringRangeError: "%[1]s: %[2]s",
		// And the negative length a list slice refuses names the length
		// alone, with no parameter in front of it: `-1: substring
		// expression < 0`.
		ListSliceNegativeLength: "%[1]s: substring expression < 0",
		// The same sentence for the string spelling's end-behind-start
		// bound, which is a different axis reaching one wording here and a
		// wholly different one in zsh.
		SubstringEndBehindTheStart: "%[1]s: substring expression < 0",
		ArithOperandExpected:       "arithmetic syntax error: operand expected",
		ArithOperatorExpected:      "arithmetic syntax error in expression",
		// A name whose subscript never closes is a *bad subscript* here rather
		// than leftover text, and the refusal names it from the name: measured
		// 2026-09-23, `let 'b[c'` is `b[c: bad array subscript (error token is
		// "b[c")` where this shell blamed `[c` as an operator. See
		// interp.Diagnostics.ArithUnclosedSubscript for the rows and the two
		// controls that stay on the sentence above (#4175).
		ArithUnclosedSubscript: "bad array subscript",
		ArithBadOperator:       "arithmetic syntax error: invalid arithmetic operator",
		ArithAssignToNonPlace:  "attempted assignment to non-variable",
		ArithMissingCloseParen: "missing `)'",
		// Only for an operand left in the group: a byte that could be no
		// operator is worded as itself. See the field (#6803).
		ArithGroupWordsTheLeftover: true,
		ArithFailureStatus:         1,
		SyntaxUnexpected:           "syntax error near unexpected token `%[1]s'",
		// And, inside a `$( … )` body, what the shell was still looking
		// for. See Diagnostics.SubstitutionBodyExpecting for the six rows
		// and for the two shapes that get nothing (#3467).
		SubstitutionBodyExpecting: " while looking for matching `%[1]s'",
		// And an *unclosed* one answers with that same refusal instead of
		// the unmatched-parenthesis sentence. Measured 2026-09-21: `v=$(esac`
		// with no closer is `line 1: syntax error near unexpected token
		// `esac' while looking for matching `)'` — the same line this shell
		// already wrote for `v=$(esac)`, where it had been writing `line 2:
		// unexpected EOF while looking for matching `)'`.
		UnterminatedSubstitutionIsItsBodysRefusal: true,
		// A refused word is echoed as it was written: `"zzz"` keeps its
		// quotes and `$x` is not the name `x`. See UnexpectedWordNaming.
		UnexpectedWordNaming: interp.UnexpectedWordIsSourceText,
		// Inside `[[ ]]` the same token gets two lines and neither is the
		// one above: a sentence about the construct at the `[[`'s line, then
		// a shorter `near` at the token's. Measured on bash 5.3.15 —
		// `[[ -n x` newline `-z "" ]]` names line 1 and then line 2.
		CondSyntaxPreamble: "syntax error in conditional expression: unexpected token `%[1]s'",
		// And the one position it words as a statement about what it was
		// waiting for: a newline behind a term whose shape is not yet
		// settled. Measured 2026-09-18 on 5.3.20 and 3.2 alike (#3627).
		CondTermUndecidedPreamble: "unexpected token `%[1]s', conditional binary operator expected",
		// And a *second* sentence for a token standing where a condition was
		// to begin, with no sentence at all for the closer itself — see
		// CondCommandPreamble, and CondGroupUnclosed for the line each open
		// `(` adds under both (#2909).
		CondCommandPreamble: "unexpected token `%[1]s' in conditional command",
		CondGroupUnclosed:   "expected `%[1]s'",
		// And the same closer named in the sentence itself where the
		// refusal fell where that closer was wanted rather than inside one
		// of the group's terms. See Diagnostics.CondGroupCloserExpected for
		// the measured rows.
		CondGroupCloserExpected: "unexpected token `%[1]s', expected `%[2]s'",
		CondSyntaxUnexpected:    "syntax error near `%[1]s'",
		// A refused `{name}` in front of a redirection is quoted with the
		// text around it, in the sentence the condition uses. See the field
		// (#6552).
		SyntaxUnexpectedFdVariableReadText: "syntax error near `%[1]s'",
		// And what it quotes there is the text it had read, not the token:
		// `(a` for the `(` of `(ab)`. See conditions.md.
		CondSyntaxNamesTheTextRead: true,
		// And a `[[` the input ran out inside of gets a line of its own,
		// naming the closer it was waiting for. Measured: this shell writes
		// it for `[[` and for nothing else — `if`, `for`, `case`, `{` and
		// `(` left open each get the one ordinary line.
		CondUnterminatedPreamble: "unexpected EOF while looking for `%[1]s'",
		// A parse failure by every other measure, and 1 rather than this
		// dialect's syntax-error status.
		ForNameStatus: 1,
		ForName:       "`%[1]s': not a valid identifier",
		// A C-style `for` header with the wrong number of separators, worded
		// by the count: too few is a statement about the expression that was
		// not there, too many blames the separator. Measured 2026-09-12 on
		// 5.3.15, 3.2.57 and the same binary as `sh`, which all agree —
		// `for ((i=0))`, `for (())`, `for ((;))` and `for ((1;2))` give the
		// first, `for ((;;;))` and `for ((1;2;3;4))` the second (#2225).
		ForArithHeader:    "syntax error: arithmetic expression required",
		ForArithSeparator: "syntax error: `;' unexpected",
		// And a second line under either, echoing the header alone rather
		// than the line it was written on — the only failure in this dialect
		// whose echo is not the source line.
		ForArithHeaderEcho: "syntax error: `%[1]s'",
		// The same sentence for a function definition's name, which is
		// measured rather than borrowed: `` `_p_${w}': not a valid
		// identifier `` is what bash writes there, word for word. This
		// dialect never reaches the *expanded*-name route the field also
		// serves — PunctuatedFunctionNameIsRefused is No here, `f-g()` being
		// a perfectly good definition — so there is one wording to keep.
		FunctionNameInvalid: "`%[1]s': not a valid identifier",
		Unterminated:        "syntax error: unexpected end of file from `%[1]s' command on line %[2]d",
		// With nothing open to name — `f()` with no body — the sentence
		// stops after the diagnosis rather than naming an empty construct.
		UnterminatedNoConstruct: "syntax error: unexpected end of file",
		// One sentence for every unmatched delimiter, always naming the
		// closer — only the line it lands on differs by construct.
		UnmatchedQuote:      "unexpected EOF while looking for matching `%[2]s'",
		UnmatchedCmdSubst:   "unexpected EOF while looking for matching `%[2]s'",
		UnmatchedBraceSubst: "unexpected EOF while looking for matching `%[2]s'",
		// The same sentence as the other three openers, with the closer it
		// echoes coming from the construct: `)` for `$((1+2` and `]` for
		// `$[1+2`, which is measured and is why the closer is a verb rather
		// than being written into each wording.
		UnmatchedArithSubst:       "unexpected EOF while looking for matching `%[2]s'",
		UnmatchedReportedAtOpener: true,
		CmdSubstUnmatchedAtEnd:    true,
		SyntaxErrorStatus:         2,
		// Except over a failure, when the input ran out inside a quote, a
		// `${` or a `$((`: `false` and then `echo "abc` exits 1, and over
		// `exit 7` it exits 7. bash 3.2 says 2 for both (#5882).
		// An interactive shell names itself, by its name's last component,
		// and no line, whatever text the message comes from (#6008).
		InteractiveShellSpeaksAsAtAPrompt: true,
		ParseFailureKeepsAFailingStatus:   interp.FailingStatusKeptOverAnUnmatchedOpener,
		// Measured: `.` of a file it cannot open reports 1 and carries on,
		// where a missing operand is 2 — two numbers for what reads like one
		// failure, which is why they are two fields.
		DotCannotOpen: "%[1]s: %[2]s",
		// `. -p list f` whose list missed, which is not the sentence a file
		// that would not open gets: lower case, and naming the builtin.
		// The invoked word and not a written-in dot: `.` and `source` are
		// one builtin and each names itself here, measured 2026-09-21 on
		// bash 5.3.20 — `source -p /tmp nope` is `source: nope: file not
		// found` where `. -p /tmp nope` is `.: nope: file not found`. The
		// same verb DotIsADirectory takes, and it was the one dot sentence
		// still spelling the name out.
		DotSearchPathMiss:   "%[3]s: %[1]s: file not found",
		DotCannotOpenStatus: 1,
		// The one `.` failure bash gives a sentence of its own, and the one
		// it names the builtin in — `source ./` says `source:` where `. ./`
		// says `.:`, which is why the format takes the invoked name. Its
		// reason is spelled in lower case, unlike the strerror text
		// DotCannotOpen carries, so it is written out rather than passed in.
		DotIsADirectory: "%[3]s: %[1]s: is a directory",
		// bash names neither the builtin nor the operation: just the command
		// and the reason, the same shape it uses for `.`.
		CannotExecute: "%[1]s: %[2]s",
		// And it has a phrase of its own for a file the kernel would not
		// start that turned out not to be shell text either. Measured
		// 2026-09-13, `./elf64_hdr.bin` and `exec ./elf64_hdr.bin` both:
		// `<file>: cannot execute binary file: Exec format error` at 126,
		// where zsh says `exec format error: <file>` and ksh93 says
		// `<file>: cannot execute [Exec format error]` — the strerror alone,
		// arranged by their own CannotExecute.
		BinaryFileReason: "cannot execute binary file: Exec format error",
		// bash names the builtin only when the command was not found at all.
		ExecNotFound: "exec: %[1]s: not found",
		// bash is the only one that says which *kind* of operator it wanted.
		TestUnaryExpected:    "%[2]s: %[1]s: unary operator expected",
		TestBinaryExpected:   "%[2]s: %[1]s: binary operator expected",
		TestIntegerExpected:  "%[2]s: %[1]s: integer expected",
		TestTooManyArguments: "%[2]s: too many arguments",
		TestOperandExpected:  "%[2]s: argument expected",
		TestMissingBracket:   "[: missing `]'",
		// A group the reading never closed. Two sentences, because this shell
		// names the word it found where the parenthesis belonged and has
		// nothing to name when the arguments simply ran out — and its `[`
		// always has one, since the `]` is that word. Measured 2026-09-22 on
		// 5.3.20 and on 3.2.57 under the `sh` name, which answer alike.
		// A leftover word spelled like an operator is a second sentence:
		// `test 1 -ne 2 -ne 3` names the `-ne` where `test a = b = c` is the
		// ordinary count. Measured 2026-09-22 on 5.3.20; 3.2.57 writes the
		// count for both, so this is 5.3's alone.
		TestLeftoverOperator:          "%[2]s: syntax error: `%[1]s' unexpected",
		TestClosingParenExpected:      "%[2]s: `)' expected",
		TestClosingParenExpectedFound: "%[2]s: `)' expected, found %[1]s",
		// An empty `=~` right operand, which this shell names the construct
		// for and quotes the operand back in — the operand being empty, so
		// the quotes close on nothing. Status 2, the construct's own failure,
		// where zsh calls it a match that did not happen and leaves 1.
		// Measured 2026-09-18, `[[ abc =~ "" ]]` in a script file (#3279).
		EmptyRegexOperand: "[[: invalid regular expression `': empty (sub)expression",
		// A function named after a special builtin, quoted the way this
		// shell quotes a reserved word. See
		// Semantics.SpecialBuiltinNameIsNotAFunctionName, which is the state
		// it is written in (#2987).
		FunctionNameIsASpecialBuiltin: "`%[1]s': is a special builtin",
		// A word this shell's POSIX mode divided one way and the run divides
		// the other, where the `${` no longer closes. It blames the **brace**
		// here and names the word as it was written — a different sentence
		// from the parse-time failure the same text gets under the protecting
		// reading alone, which blames the quote. Measured 2026-09-18 on bash
		// 5.3.20 as `sh`: `"${v-'a}"` expanded after `set +o posix` is
		// ``bad substitution: no closing `}' in "${v-'a}"`` at 1, with the
		// words beside it on the same line expanding normally (#2969).
		SecondReadingBadSubstitution: "bad substitution: no closing `}' in %[1]s",
		BraceOutputBadSubstitution:   "bad substitution: no closing \"%[1]s\" in %[2]s",
		// `kill` puts the process in parentheses and the reason after a dash,
		// which is the only wording in the panel a script could not confuse
		// with a message about a signal name.
		GetoptsBadOption:       "illegal option -- %[1]s",
		GetoptsMissingArgument: "option requires an argument -- %[1]s",
		// bash names itself and no line here, where it gives a line to
		// everything else it says.
		GetoptsNamesNoLine: true,
		CdCannotChange:     "cd: %[1]s: %[2]s",
		CdHomeNotSet:       "cd: HOME not set",
		// An empty operand, which is a fifth branch and not a path that
		// would not open: the sentence names no path at all.
		CdEmptyOperand: "cd: null directory",
		// A second operand is one too many here, and the refusal is a usage
		// error rather than a directory that would not open — 2 where
		// CdStatus is 1. No usage block goes with it.
		CdTooManyOperands:           "cd: too many arguments",
		CdTooManyOperandsStatus:     2,
		CdOldpwdNotSet:              "cd: OLDPWD not set",
		PrintfBadNumber:             "printf: %[1]s: invalid number",
		PrintfBadHexNumber:          "printf: %[1]s: invalid hex number",
		PrintfBadOctalNumber:        "printf: %[1]s: invalid octal number",
		PrintfNumberOutOfRange:      "printf: %[1]s: Result too large",
		PrintfBadVerb:               "printf: `%[1]s': invalid format character",
		PrintfMissingVerb:           "printf: `%[1]s': missing format character",
		PrintfMissingHexDigit:       `printf: missing hex digit for \x`,
		PrintfMissingUnicodeDigit:   `printf: missing unicode digit for \%s`,
		PrintfBadOption:             "printf: %[1]s: invalid option",
		TrapBarePrintNeedsCondition: "trap: -P requires at least one signal name",
		PrintfBadOptionShowsUsage:   true,
		LetNoExpression:             "let: expression expected",
		UlimitBadOption:             "ulimit: -%[1]s: invalid option",
		// `ulimit -a`, row for row as the engine writes it. The pipe row is
		// not a resource limit and never moves; its value is this machine
		// family's constant.
		// The seventeen rows bash 5.3 prints on Linux, which is the whole of
		// its table: a kernel without a limit drops that row and moves
		// nothing else, so the eleven macOS prints are these with `-R`,
		// `-e`, `-i`, `-q`, `-r` and `-x` taken out — measured 2026-09-18 on
		// bash 5.3.20 (aarch64-apple-darwin25) and 5.3.9 in the panel's
		// Alpine image, byte-identical row for row, padding included
		// (#2806).
		//
		// `pipe size` is the platform's own number rather than a limit and
		// is 1 block here and 8 there, which is what makes it a resource
		// read through the hooks instead of a constant in this table. The
		// letter is still read and still cannot be set: `ulimit -p 8` is
		// `ulimit: pipe size: cannot modify limit: Invalid argument` at 1,
		// for every operand including the value the row already holds.
		UlimitListing: []interp.UlimitListingRow{
			{
				Prefix: "real-time non-blocking time  (microseconds, -R) ",
				Letter: 'R',
				Res:    interp.ResourceRealtimeTime,
				Scale:  1,
			},
			{Prefix: "core file size              (blocks, -c) ", Letter: 'c', Res: interp.ResourceCore},
			{Prefix: "data seg size               (kbytes, -d) ", Letter: 'd', Res: interp.ResourceData, Scale: 1024},
			{Prefix: "scheduling priority                 (-e) ", Letter: 'e', Res: interp.ResourceSchedulingPriority, Scale: 1},
			{Prefix: "file size                   (blocks, -f) ", Letter: 'f', Res: interp.ResourceFileSize},
			{Prefix: "pending signals                     (-i) ", Letter: 'i', Res: interp.ResourcePendingSignals, Scale: 1},
			{Prefix: "max locked memory           (kbytes, -l) ", Letter: 'l', Res: interp.ResourceLockedMemory, Scale: 1024},
			{Prefix: "max memory size             (kbytes, -m) ", Letter: 'm', Res: interp.ResourceResidentSet, Scale: 1024},
			{Prefix: "open files                          (-n) ", Letter: 'n', Res: interp.ResourceOpenFiles, Scale: 1},
			{Prefix: "pipe size                (512 bytes, -p) ", Letter: 'p', Res: interp.ResourcePipeBuffer, Scale: 512},
			{Prefix: "POSIX message queues         (bytes, -q) ", Letter: 'q', Res: interp.ResourceMessageQueues, Scale: 1},
			{Prefix: "real-time priority                  (-r) ", Letter: 'r', Res: interp.ResourceRealtimePriority, Scale: 1},
			{Prefix: "stack size                  (kbytes, -s) ", Letter: 's', Res: interp.ResourceStack, Scale: 1024},
			{Prefix: "cpu time                   (seconds, -t) ", Letter: 't', Res: interp.ResourceCPUTime, Scale: 1},
			{Prefix: "max user processes                  (-u) ", Letter: 'u', Res: interp.ResourceProcesses, Scale: 1},
			{Prefix: "virtual memory              (kbytes, -v) ", Letter: 'v', Res: interp.ResourceAddressSpace, Scale: 1024},
			{Prefix: "file locks                          (-x) ", Letter: 'x', Res: interp.ResourceFileLocks, Scale: 1},
		},
		UlimitBadNumber: "ulimit: %[1]s: invalid number",
		// bash names the resource by the same label `ulimit -a` gives it.
		UlimitCannotChange: "ulimit: %[1]s: cannot modify limit: %[3]s",
		BuiltinBadOption:   "%[1]s: %[2]s: invalid option",
		WaitBadJob:         "wait: `%[1]s': not a pid or valid job spec",
		WaitNoSuchJob:      "wait: %[1]s: no such job",
		// `fg` and `bg` in a script, which has no job control: bash refuses
		// before it reads the operand and words it the same for both. Spelled
		// out rather than left to the shared fallback because empty here
		// means silence — see Diagnostics.NoJobControl, which is ksh93's
		// answer.
		NoJobControl:       "%[1]s: no job control",
		AmbiguousJobSpec:   "%[1]s: %[2]s: ambiguous job spec",
		KillNoSuchJob:      "kill: %[1]s: no such job",
		DisownNoCurrentJob: "disown: current: no such job",
		// And `fg` and `bg` with no operand and no current job, in the
		// same words: bash names the spec it defaulted to (#5860).
		NoCurrentJob:     "%[1]s: current: no such job",
		WaitBadJobStatus: 1,
		WaitNotOurChild:  "wait: pid %[1]d is not a child of this shell",
		// A job given up on because it stopped, in the two wordings bash has
		// for it: the bare `wait` names the job and its process, and the one
		// that named a job speaks from inside its own wait (#2227).
		WaitJobStopped:    "wait: warning: job %[1]d[%[2]d] stopped",
		WaitForJobStopped: "warning: wait_for_job: job %[1]d is stopped",
		// And what a waiting signal that lost its trap to an earlier handler
		// says at its turn, and, where the trap went back to the default, as
		// it is sent again. Measured 2026-11-01 on 5.3.20; see
		// interp.Diagnostics.WaitingSignalLostItsTrap (#7170).
		WaitingSignalLostItsTrap: "warning: run_pending_traps: bad value in trap_list[%[1]d]: 0x%[2]x",
		WaitingSignalResent:      "warning: run_pending_traps: signal handler is SIG_DFL, resending %[1]d (SIG%[2]s) to myself",
		// The kinds of variable a function cannot be. See
		// interp.Diagnostics.VariableOnlyLettersOnAFunctionLine.
		UnsetFunctionAndVariable: "unset: cannot simultaneously unset a function and a variable",
		DeclareMakesNoFunction:   "%[1]s: cannot use `-f' to make functions",
		VariableOnlyLettersOnAFunctionLine: map[string]string{
			"declare": "niAa",
			"typeset": "niAa",
		},
		UnimplementedOptionLetters: map[string]string{
			// `set` letters bash has and this shell does not: -b job
			// notices, -k assignment-anywhere, -r restricted, -H history
			// expansion, -P physical paths. Measured 2026-09-05 by asking
			// bash 5.3 for every letter of the alphabet in both cases and
			// both signs; the ones missing from here it refuses itself, and
			// those get SetInvalidOptionLetter.
			//
			// Two names have left this sentence without leaving the string,
			// which is the drift #3088 swept for — a comment naming a letter
			// as missing is read the same way the string is. `-B` is gone
			// because brace expansion is built and `set -B` is silently 0;
			// `-p` is gone because it is refused under its *name*, `set:
			// privileged: not implemented`, and never reaches this table.
			//
			// `-H` left it when the expander was built (#3093): the letter
			// and `set -o histexpand` are one request, so leaving it here
			// while the name was wired would refuse what the name grants —
			// the same pairing `-t` describes below.
			//
			// `-t` left this list when the option behind it was built. The
			// two tables are one table: the letter and `set -o onecmd` are
			// the same request, so a letter listed here while the name is
			// wired would refuse what the name grants — see
			// Semantics.SetHasTheTLetter.
			//
			// `r` left this list when restricted mode was built (#4168):
			// the letter is the whole of how that mode is entered, so a
			// letter listed here while the mode exists would refuse what
			// the mode grants — the same pairing `-t` and `-H` describe
			// above.
			//
			// `-P` left in #4163, and it is the `-p` shape rather than the
			// `-t` one: `physical` is a name this shell's `set -o` table
			// already grants — it records the state and runs nothing — so
			// the letter is the short spelling of a request that was
			// already being taken. Listed here, `set -P` was refused while
			// `set -o physical` was granted, which is the two-sentences-
			// for-one-question the `-p` note above forbids. The letter now
			// routes to the name through SetOptionLetterNames, which is
			// also what puts `P` into `$-`.
			"set": "b",
			// Options these builtins have here and this shell does not.
			"wait": "f",
			// disown's sweepers: -a for every job, -h for HUP shielding
			// alone, -r for the running ones.
			"disown": "ahr",
			// `jobs -n` lists what has changed since the shell last said,
			// which needs a record of what it has already reported — and
			// the two shells with the letter do not agree on what counts:
			// a job that has only just started is a change here and is not
			// one in ksh93. `jobs -x` is not a listing at all: it runs a
			// command with the job specs in its arguments replaced by
			// process ids.
			"jobs": "nx",
			// Nothing of read's is left. -e and -E joined ReadOptions above
			// when it was measured that the pair changes nothing off a
			// terminal, which is where every `read` in a script runs; -i
			// was never here, because it seeds the editor -e opens and
			// there is never one to seed. The -p prompt is implemented:
			// parsed always, printed only to a terminal, which is the
			// measured whole of it.
			// `local`'s function letters, which this shell takes and ignores
			// where no operand is a function. `-n` left this list when name
			// references landed (#2553), `-I` when inheritance did (#3434)
			// and `-t` when the trace attribute did (#3101); the two tables
			// are one table, so a letter named here while DeclareOptions
			// spells it would refuse what the attribute grants. `declare`
			// and `typeset` have left the table entirely with the last of
			// them.
			"local": "fF",
		},
		// bash's own words for the two -u failures it can meet here; the
		// non-number wordings per letter are not modeled yet, so those fall
		// back to the substrate's.
		ReadBadFileDescriptor: "read: %[1]s: invalid file descriptor: Bad file descriptor",
		// A `=~` pattern that will not compile, with regcomp's reason.
		// Measured 2026-10-03; see interp.Diagnostics.InvalidRegex.
		InvalidRegex:              "[[: invalid regular expression `%[1]s': %[2]s",
		ReadBadTimeout:            "read: %[1]s: invalid timeout specification",
		ReadBadDescriptorSpec:     "read: %[1]s: invalid file descriptor specification",
		ReadFromAClosedDescriptor: "read: %[1]s: read error: Bad file descriptor",
		// 128 plus SIGALRM, the signal a timeout is.
		ReadTimeoutStatus: 142,
		HereDocumentAtEOF: "warning: here-document at line %[1]d " +
			"delimited by end-of-file (wanted `%[2]s')",
		// And what it says when it feeds one of those from the lines after
		// the enclosing command instead, which is the `$( )` and `<( )`
		// spellings only — measured 2026-09-20, one document and two (#3711).
		HeredocCarriedOutOfSubstitution: "warning: command substitution: " +
			"%[1]d unterminated here-document%[2]s",
		// And when one took in a NUL it dropped: once per substitution, on
		// the command's line. Measured 2026-10-01; see
		// Diagnostics.SubstitutionDroppedANul.
		SubstitutionDroppedANul: "warning: command substitution: ignored null byte in input",
		// bash names the builtin for its own two spellings and not for the
		// two POSIX has: `declare: r: readonly variable` against a plain
		// `r: readonly variable` from `export`.
		ReadonlyVariableInDeclaration: "%[2]s: %[1]s: readonly variable",
		// `local` belongs here with the other two: `typeset -r x=1; f() {
		// local x=2; }` says `local: x: readonly variable` in this shell,
		// naming the word the script wrote. It was missing because `local`
		// reached the refusal only through the assignment, where the
		// builtin's name had already been put aside (#1159).
		ReadonlyRefusalNamesBuiltin: map[string]bool{
			"declare": true, "typeset": true, "local": true,
		},
		// `readonly -a a=(1)` inside a function names the function where the
		// two spellings above name themselves. The letter is what moves it —
		// see Diagnostics.LiteralOperandAfterADeclarationIsLocatedUnderTheCall
		// for the eight measured rows and for what the letter changes about
		// when the store runs.
		LiteralOperandAfterADeclarationIsLocatedUnderTheCall: true,
		// `local qux=(one two)` over a frozen name refuses twice here: the
		// store, then the builtin. See the field for the eight measured rows.
		LiteralOperandRefusedAlsoSpeaksForTheStore: true,
		// The three sentences a frozen function has. Measured 2026-09-16 on
		// 5.3.20 and 3.2.57 alike, each at status 1 and none of them fatal:
		//
		//	readonly -f nosuchfn    readonly: nosuchfn: not a function
		//	b() { :; }              b: readonly function
		//	unset -f b              unset: b: cannot unset: readonly function
		//
		// The second carries the shell's ordinary location prefix and names
		// no builtin, because no builtin was written — it is a definition
		// that was refused.
		ReadonlyNotAFunction:      "readonly: %[1]s: not a function",
		ReadonlyFunctionRedefined: "%[1]s: readonly function",
		UnsetReadonlyFunction:     "unset: %[1]s: cannot unset: readonly function",
		// And the *line* that prefix carries is the definition's last, not
		// its first: this shell reads the whole definition before it runs
		// any of it and reports where its reader had got to. Measured
		// 2026-09-22 on 5.3.20 and 3.2.57 alike over five spellings of a
		// definition — see the field, which carries the table. It reaches
		// the refusal above and the `not a valid identifier` a name the
		// shell will not bind draws, both of which are the definition's own
		// and neither of which any other column reaches at run time.
		FunctionDefinitionRefusalIsLocatedWhereTheShellWasReading: true,
		// And a `select` whose variable is not a name is located where this
		// shell's reader stands rather than at the clause — the `for`
		// spelling beside it is located at its own clause. Measured
		// 2026-09-22 over five shapes; the field carries the table.
		SelectNameIsLocatedAtTheReader: true,
		// `declare -p nosuch` — the name it was invoked by is in front,
		// which declarePrint writes, so the wording carries only the rest.
		DeclareNoSuchVariable: "%[1]s: not found",
		// The three sentences a name reference has. Measured 2026-09-15 on
		// 5.3.15, `env -i` with a scratch HOME:
		//
		//	declare -n r=1bad    declare: `1bad': invalid variable name
		//	                     for name reference                    st 1
		//	declare -n r=r       declare: r: nameref variable self
		//	                     references not allowed                st 1
		//	declare -n a=b       silent 0, and `echo "$a"` then writes
		//	declare -n b=a       warning: a: circular name reference
		//
		// The target is quoted back with bash's own `'` pair, the way every
		// bad-name refusal on this builtin quotes one — see BuiltinBadName,
		// which is the sentence this replaces for the `n` letter alone.
		NamerefBadTarget: "`%[1]s': invalid variable name for name reference",
		// Except for the empty word, which never reaches the sentence above:
		// measured 2026-09-22 on 5.3.20, `declare -n r=` is ``declare: `':
		// not a valid identifier`` — the ordinary bad-name refusal this
		// builtin gives any word it will not bind — where `declare -n r=1x`
		// is the line above it. An aimed reference is left aimed either way.
		NamerefEmptyTargetIsAnOrdinaryBadName: "`%[1]s': not a valid identifier",
		NamerefSelfReference:                  "%[1]s: nameref variable self references not allowed",
		// The same sentence ksh93 writes, and the only one the two shells
		// share on this letter — carried here so the dialect says which
		// wording it means rather than leaning on a fallback.
		NamerefCannotBeAnArray: "%[1]s: reference variable cannot be an array",
		// Spoken as the shell rather than as the builtin, measured: the line
		// is `bash: line 1: warning: a: circular name reference` with no
		// `declare:` in it, where the two refusals above carry the name.
		NamerefCircularWarning:               "warning: %[1]s: circular name reference",
		NamerefArrayLiteralDropsTheAttribute: "warning: %[1]s: removing nameref attribute",
		// And the write through one, which bash reports as a depth rather
		// than as a circle: `f() { local -n r=r; r=SET; }` writes `warning:
		// r: maximum nameref depth (8) exceeded` and the value reaches the
		// global cell. Measured 2026-09-15 on 5.3.20 (#3048).
		NamerefDepthWarning: "warning: %[1]s: maximum nameref depth (8) exceeded",
		// The second sentence a refused `{name}>` store writes, after
		// whatever the store itself said: measured 2026-09-17, `declare -n
		// s; exec {s}>/dev/null` is ``exec: `10': not a valid identifier``
		// and then this, at 1, and `readonly s=1` in front of the same line
		// is `s: readonly variable` and then this. No other column was asked:
		// zsh refuses `typeset -n` and ksh93 has no nameref of this spelling.
		CannotAssignFdToVariable: "%[1]s: cannot assign fd to variable",
		// `true {v}>&3` with 3 closed says this unlocated line before the
		// usual `3: Bad file descriptor`, measured 2026-10-15 on 5.3.20
		// (#6572); a numbered `4>&3` says only the second.
		FdVariableDuplicationFailed: "redirection error: cannot duplicate fd: %[1]s",
		TrapPrintsSignalPrefix:      "SIG",
		// One wording for all three, and the operand quoted back exactly as
		// given: `export 1x=v` says `1x=v', not `1x'.
		BuiltinBadName: map[string]string{
			"export":   "%[1]s: `%[2]s': not a valid identifier",
			"readonly": "%[1]s: `%[2]s': not a valid identifier",
			"unset":    "%[1]s: `%[2]s': not a valid identifier",
			"local":    "%[1]s: `%[2]s': not a valid identifier",
			// The declaration builtins, which say the same as the rest. Named
			// rather than left to the default so that the table is the whole
			// answer for every builtin that asks it.
			"typeset": "%[1]s: `%[2]s': not a valid identifier",
			"declare": "%[1]s: `%[2]s': not a valid identifier",
			// `read 1bad` says the same, and quotes the word back whole:
			// `read "v?p"` is `` `v?p' `` here, because bash has no prompt
			// operand to split it at.
			"read": "%[1]s: `%[2]s': not a valid identifier",
			// And `getopts`, whose name operand says the same — and says it
			// to `o[1]` as well, in a shell that fills `read 'a[1]'`: this
			// builtin does not take a subscript here. Measured 2026-09-18
			// (#3555).
			"getopts": "%[1]s: `%[2]s': not a valid identifier",
			// And `printf -v 1bad`, which is the same sentence under this
			// builtin's name at this builtin's own status — measured
			// 2026-09-17, ``printf: `1x': not a valid identifier`` at 2.
			// Named rather than left to the default for the reason the two
			// declaration builtins above are: the table is meant to be the
			// whole answer for every builtin that asks it.
			"printf": "%[1]s: `%[2]s': not a valid identifier",
		},
		// `printf` counts the identical refusal as a usage error where `read`
		// counts it as a failure: measured 2026-09-17, a script file,
		// `printf -v '1x' %s Q` and `printf -v 'a[]' X` are both 2 with the
		// rest of the line still running, where `read '1x'` and `read 'a[]'`
		// are 1. See Diagnostics.BuiltinBadNameStatusFor.
		BuiltinBadNameStatusFor: map[string]int{"printf": 2},
		// And `printf`'s own 2 is the *option*'s: a `%n` whose operand is
		// not a name is an ordinary runtime failure and reports 1.
		// Measured 2026-09-26: `printf -v '1x' %s Q` is 2 and
		// `printf 'abc%n' '1bad'` is 1, on the same sentence.
		PrintfCountBadNameStatus: 1,
		BuiltinBadNameKeepsValue: true,
		BuiltinUsageUnprefixed:   true,
		BuiltinHelpStatus:        2,
		BuiltinHelp:              builtinHelp(),
		BuiltinUsage:             builtinUsage(),
		// bash words this as a statement about the operator that was
		// waiting rather than about the token it met: `[[ $k == (a|b) ]]`
		// is "unexpected argument `(' to conditional binary operator", where
		// ksh93 and zsh name the token and stop.
		CondOperand:            "unexpected argument `%[1]s' to conditional %[2]s operator",
		PrintfUsage:            "printf: usage: printf [-v var] format [arguments]",
		UmaskBadMask:           "umask: %[1]s: octal number out of range",
		AliasNotFound:          "%[1]s: %[2]s: not found",
		AliasInvalidName:       "%[1]s: `%[2]s': invalid alias name",
		UnaliasNotFound:        "%[1]s: %[2]s: not found",
		AliasListPrefix:        "alias ",
		UnaliasUsage:           "unalias: usage: unalias [-a] name [name ...]",
		UnaliasUsageUnprefixed: true,
		// bash names the character and says which kind it wanted.
		UmaskBadSymbolicMode:     "umask: `%[2]s': invalid symbolic mode character",
		UmaskBadSymbolicOperator: "umask: `%[2]s': invalid symbolic mode operator",
		UmaskBadOption:           "umask: %[1]s: invalid option",
		UmaskUsage:               "umask: usage: umask [-p] [-S] [mode]",
		UmaskUsageUnprefixed:     true,
		// The usage carries no location, as `kill`'s does not.
		PrintfUsageUnprefixed:     true,
		TrapBadSignal:             "trap: %[1]s: invalid signal specification",
		KillNoSuchProcess:         "kill: (%[1]s) - No such process",
		KillNotPermitted:          "kill: (%[1]s) - Operation not permitted",
		KillInvalidSignal:         "kill: %[1]s: invalid signal specification",
		KillIllegalOption:         "kill: %[1]s: invalid signal specification",
		KillNotAPid:               "kill: `%[1]s': not a pid or valid job spec",
		KillMissingSignalArgument: "kill: %[1]s: option requires an argument",
		KillUsage: "kill: usage: kill [-s sigspec | -n signum | -sigspec] pid | jobspec ... " +
			"or kill -l [sigspec]",
		// The usage is the one bash diagnostic with no location in front of
		// it — every other message here carries "bash: line N:".
		KillUsageUnprefixed: true,
		KillUsageStatus:     2,
		TimesDecimals:       3,
		// The `time` keyword's report: three decimals where ksh93 gives
		// the same lines two.
		TimeDecimals: 3,
		// $TIMEFORMAT, read every time the keyword runs. The wording names
		// the variable as well as the offending character, where ksh93
		// names only the character.
		TimeFormatVariable:     "TIMEFORMAT",
		TimeFormatBadDirective: "%[1]s: `%[2]s': invalid format character",
		// A path that is not there is the OS reason and does not name the
		// builtin; a bare name off PATH does the reverse.
		PathNotFound: "%[1]s: No such file or directory",
		// bash names the path it tried, absolute, where the other three
		// report the operand as written.
		NamesResolvedPath: true,
		// `shopt -s failglob` refusing a pattern that matched nothing.
		// Three words where zsh's is four, measured 2026-09-13 on bash
		// 5.3.15: `no match: nosuch*` against zsh's `no matches found:
		// nosuch*`. Same event, same position prefix, different sentence,
		// which is why the substrate carries neither.
		GlobNoMatch: "no match: %s",
		// A bare `hash` announces the table, on standard output.
		HashEmptyTable: "hash: hash table empty",
		// The `hits<TAB>command` table, which is bash's alone: it is the only
		// column that keeps a count to print. See interp.HashListingForm.
		HashListing:  interp.HashListingHitsAndPath,
		HashDisabled: "hash: hashing disabled",
		// Two lines, which is bash rather than a mistake: it prints the
		// complaint and then a usage line, and only the first carries the
		// shell's own prefix.
		DotNoOperand: "%[1]s: filename argument required\n" +
			"%[1]s: usage: %[1]s [-p path] filename [arguments]",
		DotNoOperandStatus: 2,
	}
}
