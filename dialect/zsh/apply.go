// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"os"
	"strconv"
	"sync"

	"github.com/blairham/sh/dialect/zsh/modules/files"
	"github.com/blairham/sh/dialect/zsh/modules/mathfunc"
	"github.com/blairham/sh/interp"
)

// Apply adds what zsh has and the substrate does not.
//
// `source` is the second name for `.`, which dash does not have at all — so
// the name is a dialect's answer, and it is the same function under a second
// name rather than a second implementation.
//
// It is not a *synonym*, which this comment used to say and which is true only
// of bash and ksh93. Measured 2026-09-12 on zsh 5.9.2, in a directory holding
// `plain.sh`:
//
//	source plain.sh   `sourced`, status 0
//	. plain.sh        `no such file or directory`, status 127
//
// The second row is what makes the measurement discriminating: a shell that
// simply searched the current directory for both would agree with the first
// and break the second. See where the name is registered below.
func Apply(r *interp.Runner) {
	// Room for everything registered below, before any of it is: see
	// tablesizes.go.
	r.ReserveTables(zshTables)
	// Everything this shell keeps under `.zsh.` is its own state and not a
	// parameter of the shell being emulated — the loaded module set, the
	// `zstyle` table, the scheduled events, the line editor's buffer and
	// twenty more. They live in the parameter tables because that is what a
	// subshell copies; the reference keeps the same facts where no script can
	// see them, so no listing here writes a row for one. See
	// interp.Runner.SetEngineOwnedPrefix, where the routes are (#5014).
	r.SetEngineOwnedPrefix(zshEngineStorePrefix)
	// `~[name]` is a directory a script's own function names. See
	// dynamicdir.go.
	r.DynamicDirectoryFunctions = zshDynamicDirectoryFunctions
	// The builtins that read an argument's first `=` as an assignment's, so
	// `builtin local x=~root` and `alias a=~root` both name the home (#5670).
	// See interp/equalstildebuiltins.go.
	for _, name := range []string{
		"alias", "hash", "typeset", "local", "export", "readonly", "declare", "float", "integer",
	} {
		r.MarkBuiltinWhoseEqualsOpensATildeContext(name)
	}
	r.NumberedTildeSwapsItsSigns = func(r *interp.Runner) bool { return recordedDeviates(r, "pushdminus") }
	// This shell has an `enable`, but a different one: it works on hash
	// tables and takes none of bash's options — `enable -n` is a bad option
	// there. Claiming a bash-shaped one would be worse than not having it.
	// Not removed but replaced: zsh has an `enable`, and it is a different
	// builtin from the one the core carries. See enable.go.
	registerEnable(r)
	// `log` is in the table for `disable log` to reach: macOS's `/etc/zshrc`
	// writes that line to keep the builtin out of the way of `/usr/bin/log`,
	// and a shell that reads the system-wide files meets it before the first
	// prompt (#2325). See log.go for what it does and does not do.
	r.Register("log", logBuiltin)
	// How zsh scripts actually change options, and how they change shells.
	// See setopt.go and emulate.go.
	registerSetopt(r)
	registerDebugCommand(r)
	registerZcompile(r)
	// The completion dump compinit reads back, reachable only from the
	// prelude's own function. See compdump.go (#6307).
	registerCompdump(r)
	registerCompctl(r)
	// And the new completion system's own two, which are what a `zle -C`
	// widget's function is written in. See compsys.go for the context they
	// refuse outside of, and #2776 for the rest of the module they belong to.
	registerCompadd(r)
	registerCompset(r)
	// And the module's other four features, which are conditions rather than
	// builtins: `[[ -prefix … ]]` and its three neighbors are the tests
	// `compset` performs without the move. See completioncondition.go, and
	// #3042 for the measurement that says loading the module is not what
	// makes them exist.
	registerCompletionConditions(r)
	// `limit` and `unlimit`, which are `zsh/rlimits` beside the `ulimit`
	// this shell already has. See rlimits.go (#4736).
	registerRlimits(r)
	// And `zsh/computil`'s eight, which is what the completion system zsh
	// *ships* is written in: `_arguments`, `_describe`, `_tags` and `_values`
	// are shell functions whose working parts are these. See computil.go.
	registerComputil(r)
	registerLocalOptions(r)
	// And the sticky emulation, which hangs off the same moment: see
	// sticky.go for why it is registered after the option table's save.
	registerStickyEmulation(r)
	// `**/` crosses directory levels here with no option asked for, and
	// there is no `setopt` name that turns it off — which is why this is a
	// state the dialect sets rather than a name registered above. Measured
	// 2026-09-07 against zsh 5.9.2 in a directory holding `ax`, `bx` and
	// `cx/dx/ax`: `**/a*` is `ax cx/ax cx/dx/ax`, so the component stands
	// for **zero or more** levels and the match in the starting directory is
	// part of the answer. Without this the component read as `*` and the
	// listing came back short at status 0 (#1339).
	//
	// Only the slashed form. Bare `**` is an ordinary pattern here — `echo
	// **` is `ax bx cx`, which is what `*` answers — so
	// StarStarAloneCrossesDirectories stays off, and `setopt globstarshort`,
	// which is the name that would move it, remains recorded rather than
	// acted on (see setopt.go).
	r.SetMatchOption(interp.StarStarCrossesDirectories, true)
	// And StarStarSeesLinkedDirectories stays off, which is the third
	// question about the same component and is measured separately rather
	// than assumed from the second. In a directory holding `r/x` and a
	// symlink `s` to `r`, `echo **/` is `r/` here and `r/ s/` in bash 5.3.15
	// under `shopt -s globstar`: the component stands for the levels the
	// walk crossed, and a link is not one of them. Neither shell **enters**
	// the link — `echo **/x` is `r/x` in both — so what splits is what `**/`
	// lists and not where it goes (#2360).
	//
	// `***/`, which does follow links, is a construct of its own rather than
	// this option turned on: pointed at a tree holding a link to its own
	// ancestor it walks until the kernel refuses, and zsh reports `too many
	// levels of symbolic links` once per link per level. That is the
	// distinction being deliberate, written down where somebody would
	// otherwise read the refusal above as an oversight.
	//
	// The fourth and fifth questions about the same component, and this
	// shell answers both the way bash does. A `**` that matched zero levels
	// is the directory the walk stood in — `d/**/` is `[d/][d/e/]` here and
	// in bash, against `[d/e/]` in ksh93 — and a walk may begin inside a
	// directory the pattern reached through a link: `s/**` is `[s/x]` here,
	// `[s/][s/x]` in bash, and no match at all in ksh93. Measured 2026-09-16
	// (#3152).
	r.SetMatchOption(interp.StarStarZeroLevelIsTheDirectoryItStartsFrom, true)
	r.SetMatchOption(interp.StarStarPatternsReadLinkedDirectories, true)
	// What `=~` matched is readable here, and under the *same* parameters a
	// reporting pattern fills rather than under a record of its own: `$MATCH`
	// is the whole match, `$match` the groups alone, and `$MBEGIN`/`$MEND`
	// with `$mbegin`/`$mend` where each began and ended. Measured 2026-09-11
	// against zsh 5.9.2 with `[[ 'hello world' =~ 'o (w[a-z]+)d' ]]`:
	// `MATCH=o world MBEGIN=5 MEND=11`, `match=(worl) mbegin=(7) mend=(10)`.
	//
	// A failing match leaves all of them alone — measured, a `zzz` that
	// matches nothing still reads back the match before it — which is the
	// reporting pattern's own rule and the opposite of the dense array the
	// other shell keeps. See interp/regexmatch.go.
	//
	// It is what makes `regexp-replace` possible: a global search and replace
	// in shell needs the *extent* of each match and not only that there was
	// one, and this is the only thing that reports it (#1968).
	r.SetRegexCaptureReport()
	// $LINES and $COLUMNS follow the window here too, and — like `**/` above
	// — with no option name to ask for it: this shell simply does it, so
	// there is nothing in setopt.go for a script to turn off. Measured
	// 2026-09-08 through a pseudo-terminal against zsh 5.9.2 started with
	// `-f`, which reports `COLUMNS=80 LINES=24` at its first prompt and
	// `132`/`40` at the next prompt after a resize.
	//
	// **Not the switch bash's `checkwinsize` moves**, which is what this line
	// used to be and what #2107 is. That switch is permission for the front
	// end to *assign* two variables once per prompt, and it is right for
	// bash — measured, bash has the pair only when it is interactive and
	// leaves both unset otherwise. This shell's are parameters of the
	// language: present under plain `-c` with no prompt anywhere, `0` rather
	// than unset where there is no terminal at all, `integer-special` to
	// `${(t)}`, and following a window that changes while a script runs. A
	// prompt theme measures itself against them — powerlevel10k saves
	// `$COLUMNS`, forces 1024, and shortens the directory to what is left —
	// so with the pair merely unset the arithmetic ran on an empty word and
	// the path came out at full length. See interp.Runner.ProvideWindowSize,
	// which holds the four answers and the rule about assignment.
	r.ProvideWindowSize()
	// The two integers the try-always block reports through, and recovers
	// through. The mechanism is the core's and only the spelling is here —
	// see interp.Runner.SetAlwaysBlockStatus for the measurements (#1234).
	r.SetAlwaysBlockStatus("TRY_BLOCK_ERROR", "TRY_BLOCK_INTERRUPT")
	// A job still running holds the exit here, where bash needs to be asked:
	// measured, zsh 5.9.2 started with `-f` answers `sleep 40 &` then `exit`
	// with `you have running jobs.` and stays, and bash 5.3.15 leaves. Both
	// shells spell the option `checkjobs`; only the defaults differ, and this
	// is where this one's is set. See interp.Runner.ChecksRunningJobsAtExit.
	r.SetChecksRunningJobsAtExit(true)
	// And the jobs that are still running when the session goes anyway are
	// sent SIGHUP, which is `setopt hup` and is **on** in a fresh zsh — the
	// switch the core starts off and bash only reaches through `shopt -s
	// huponexit`. Independent of `checkjobs` above, which is measured:
	// `unsetopt checkjobs` leaves at the first `exit` and still hangs the
	// jobs up and still says so. See interp.Runner.SendsHangupToJobsAtExit
	// and the three HangupAtExit axes for what this shell does differently
	// once the switch is on.
	r.SetSendsHangupToJobsAtExit(true)
	registerEmulate(r)
	// This shell's own question about a name, under two names: `whence` and
	// `where`. Not ksh93's builtin under the same spelling — the stream, the
	// statuses, the letters and every wording differ. See whence.go.
	registerWhence(r)
	// The styles database, which a real rc file fills in before it does
	// anything else. See zstyle.go.
	registerZstyle(r)
	// The two of `zsh/zutil`'s remaining builtins that can be learned by
	// running the real one: an option parser for shell functions, and a
	// string formatter. `zregexparse`, the fourth, is a completion-system
	// internal the manual describes in one sentence and is deliberately
	// absent — and `zmodload zsh/zutil` loads anyway, because a missing
	// builtin refuses by name at the word that runs it. See zparseopts.go,
	// zformat.go and the rule at the top of zmodload.go.
	//
	// #1405 asked for it beside `vared` and `zcompile` and said to order the
	// three by evidence. Swept over `~/.zi` and the installed function
	// directories on this machine: `zcompile` in 25 files, `vared` in 3,
	// `zregexparse` in **none**. It is absent because the rest of completion
	// is — #1282 recorded that `compinit` is not close, the file being
	// refused and `zmodload zsh/complete` refusing by name — so the one
	// parser would be reachable by nothing.
	//
	// Since #4761 that absence is a recorded **decision** rather than
	// unfinished work, and declined.go is where it is recorded with the
	// measurement: the manual gives the builtin one sentence and no
	// synopsis, so there is nothing on the green list to implement from, and
	// the cheap half — registering the name and refusing on arity — is
	// deliberately not done, because a builtin that takes its operands and
	// then answers wrongly is worse than one a caller cannot find.
	registerZparseopts(r)
	registerZformat(r)
	// And the line editor's key table, which a real rc file also reaches for.
	// See bindkey.go.
	registerBindkey(r)
	// And the other half of it: defining what a key can be bound *to*. The
	// table was here without it, so an rc file that bound a plugin's own
	// widget bound a name nothing ever answered to. See zle.go.
	registerZle(r)
	// Timed commands, which is what a plugin manager's deferred loading is
	// built on. Independent of the line editor despite arriving with it. See
	// sched.go.
	registerSched(r)
	// Stopping the shell itself, which is two halves: a refusal a script sees
	// and a stop only a binary that owns the process may make. Not bash's
	// builtin under the same spelling — job control decides nothing here and
	// the statuses differ. See suspend.go.
	registerSuspend(r)
	// Editing a variable with the line editor: everything a script outside a
	// session sees, which is all of it without a terminal. See vared.go.
	registerVared(r)
	// The module loader, which answers per module rather than pretending to
	// load anything. See zmodload.go.
	registerZmodload(r)
	// And the other half of a module system: a name defined from `$fpath`
	// the first time it is called. See autoload.go.
	registerAutoload(r)
	// Eleven of `zsh/parameter`'s thirty-three: this shell's own tables read
	// through as associations, produced when they are asked for rather than
	// stored. See parameter.go.
	registerParameterModule(r)
	// And `zsh/datetime`'s four, all of which this shell can answer honestly:
	// three clock reads through Runner.Now and a formatter over the same
	// format language `printf '%(fmt)T'` writes. See datetime.go.
	registerDatetimeModule(r)
	// And `zsh/stat`'s one builtin under the one of its two names a shell
	// may safely hold always: `zstat`. See statmodule.go for why `stat` is
	// not registered beside it.
	registerStatModule(r)
	// And `zsh/files`' nine file operations, under the nine `zf_`-prefixed
	// names — the same argument again, and the same split. See
	// dialect/zsh/modules/files/files.go.
	files.Register(r)
	// And `zsh/net/socket`'s one: a Unix-domain socket opened into this
	// shell's own descriptor table. See socketmodule.go.
	registerSocketModule(r)
	// And `zsh/mapfile`'s one: the filesystem as an association, where a key
	// is a path and the value is that file's bytes. See mapfile.go.
	// Its parameter is **not** installed here: the module declares no
	// autoloadable name, so `${+mapfile}` is 0 until `zmodload zsh/mapfile`.
	// See gatedparameters.go (#4922).
	// And `fc`'s three file letters over a history list this dialect keeps,
	// which is what `print -s` fills. The core `fc` stays the answer for
	// every other letter — this registration replaces it and delegates. See
	// fchistory.go.
	registerFcHistory(r)
	// And `zsh/terminfo`'s and `zsh/termcap`'s one parameter each: the
	// terminal's capabilities under two name systems, read out of the
	// description `$TERM` names by repl.TerminalCapabilities. See
	// terminfo.go.
	registerTerminfoModules(r)
	registerBuiltinSynonyms(r)
	registerHashFill(r)
	// And `zsh/langinfo`'s one: the locale's own vocabulary, answered from
	// the variables that name the locale rather than from a fixed table. See
	// langinfo.go.
	// And the same for `zsh/langinfo`'s one, which is gated the same way.
	// See gatedparameters.go.
	// And `zsh/zleparameter`'s two: the line editor's widgets and keymaps,
	// read off the tables `zle` and `bindkey` already keep. See
	// zleparameter.go.
	registerZleParameterModule(r)
	// And `zsh/system`, all nine features of it: two parameters, a math
	// function and the six builtins — three that move bytes, a file lock and
	// the two small ones that close the module. See system.go, systemio.go,
	// systemlock.go and systemseek.go.
	registerSystemModule(r)
	// And `zsh/zselect`'s one: the wait on descriptors that the same prompt
	// theme's worker uses as its only sleep. See zselect.go.
	registerZselectModule(r)
	// And `zsh/zpty`'s one: a command run on a pseudo-terminal of its own,
	// which is the last module of #1320's table and the first thing in this
	// dialect that needed a new seam on the substrate before it could exist
	// at all. See zpty.go and interp.Runner.StartConcurrent.
	registerZptyModule(r)
	// And `zsh/mathfunc`'s forty-seven, which are the C math library under
	// names arithmetic can call. See dialect/zsh/modules/mathfunc.
	mathfunc.Register(r)
	// And taken straight back out, because a fresh shell has none of them:
	// `$(( sqrt(4) ))` is `unknown function: sqrt` until the module is
	// loaded. Registered first and withdrawn after, which is what lets
	// `zmodload zsh/mathfunc` see the features it is about to offer — see
	// dialect/zsh/modules/mathfunc/gated.go and interp.Runner.SetMathFunctionWithdrawn (#5060).
	mathfunc.Withdraw(r)
	// And, after every one of them, which of the parameters they registered
	// are still waiting for the script's first reference. Last rather than
	// beside each registration, because it is one measured roster rather
	// than a property of the function that installed each name — see
	// deferredparameters.go for the route it was taken by and for the five
	// names it deliberately leaves out.
	// And the one pair this shell joins that is not a tie: `$WATCH` and
	// `$watch` are one parameter in two kinds, joined on `:`, and the
	// reference describes neither half with the word `tied`. See
	// watchpair.go, where the measurement that separates the join from the
	// word is. Ahead of the deferral roster, which names both halves.
	registerWatchPair(r)
	registerDeferredParameters(r)
	// This shell's richer `echo`, and not ksh93's builtin of the same
	// spelling: different letters, a different escape set and different
	// wordings, all measured side by side. See print.go.
	registerPrint(r)
	// And the stack `print -z` pushes onto, which `read -z` takes off. The
	// storage is this dialect's and the pop is the core's — see
	// editorbuffer.go and interp.Runner.SetEditorBufferPop.
	registerEditorBuffer(r)
	// And the same escape set, reached from the other end: the `(p)`
	// expansion flag reads the argument of a `j` or `s` behind it the way
	// `print` reads an operand, so `${(pj:\n:)a}` joins on a real newline.
	// One function for both, because the set is one measurement — see
	// print.go, and interp.Runner.SetFlagArgumentEscapes for why the
	// substrate asks rather than answers.
	r.SetFlagArgumentEscapes(expandFlagArgumentEscapes)
	// And the same set a third time, reached from a third end: the `(g)`
	// expansion flag reads the escapes in a *value*, with its option letters
	// naming which parts of the set are live. One decoder for all three, for
	// the reason above — see print.go, and interp.Runner.SetExpansionEscapes.
	r.SetExpansionEscapes(expandExpansionFlagEscapes)
	// No `compgen` here; it is bash's alone.
	r.Unregister("compgen")
	r.Unregister("compopt")
	r.Unregister("complete")
	// And neither `mapfile` nor its other name; both are bash's alone.
	r.Unregister("mapfile")
	r.Unregister("readarray")
	// The `set -o` names beyond the ones every shell has.
	//
	// `trackall` is here because zsh has *both* spellings of command
	// tracking — `set -o hashall` and `set -o trackall` each succeed, and
	// each moves the same state — where bash has only the first and ksh93
	// only the second. It was missing until the membership was re-measured
	// against the panel rather than read off `semantics.md`, which said
	// trackall belonged to ksh93 alone; `set +o trackall` was a refusal
	// here and a no-op in the shell this dialect imitates. Pinned by
	// `opt/command-tracking-has-two-long-names`.
	r.AddSetOptions(
		"braceexpand",
		// The state behind this shell's own `emacs` option, which its table
		// reads and writes through Runner.NamedOption exactly as `stdin`
		// below does. It was in the substrate's common table until #3366
		// took it out — BusyBox ash has no such name — so it is declared
		// here for the same reason and with the same effect: this list
		// reaches nothing but the two named-option calls setopt.go makes.
		"emacs",
		"hashall",
		"histexpand",
		"histignoredups",
		"onecmd",
		"physical",
		"pipefail",
		"privileged",
		// The state behind the `s` in `$-`, which this dialect's `shinstdin`
		// is read and written through. It is not in this shell's own `set -o`
		// roster and does not appear in one — zsh installs an option table of
		// its own, so this list reaches nothing but Runner.NamedOption and
		// Runner.ApplyNamedOption, which is exactly what setopt.go's
		// shinStdinOption calls (#3154).
		"stdin",
		"trackall",
	)
	// The statuses of the last pipeline's elements. The core keeps the
	// record and this names it; ksh93 and dash have no name for it at all.
	r.SetPipelineStatus("pipestatus")
	// And a write to it lands in the record, each value read as a number:
	// see interp.Runner.WritePipelineStatus (#6088).
	r.SetDynamicArrayWriter("pipestatus", func(r *interp.Runner, values []string) { r.WritePipelineStatus(values) })
	// And the shape a listing writes it in, which is the same line
	// dialect/bash gives `PIPESTATUS` and is here for the same reason: the
	// record is not in any of the tables a listing walks, so without a
	// declaration to hang the letters on the name is one `typeset -p` cannot
	// find. Measured 2026-09-27 on zsh 5.9.2 under `-f` from a script file,
	// in a shell that has run one command:
	//
	//	typeset -p pipestatus     typeset -a pipestatus=( 0 )
	//	a bare typeset -p         typeset -a pipestatus=(  )
	//
	// against `typeset:3: no such variable: pipestatus` at 1 here (#4865).
	r.SetDynamicDeclaration("pipestatus", interp.ProducedDeclaration{Array: true, ListsItsElements: true})
	// A read of the process, in library code, on purpose. The purity rule
	// covers dialect/ as well as interp/ — an embedder links this and Apply
	// runs inside the Runner — but it is about state a Runner owns and two
	// Runners could disagree about. A uid is neither: nothing a script does
	// changes it, two shells in one program genuinely have the same one, and
	// $UID has no other source. Same class as $$, which AGENTS.md's forbidigo note has
	// blessed since it was written.
	r.SetSpecial("UID", strconv.Itoa(os.Getuid()))
	// And the *group* id beside it, which is this shell's and not bash's.
	//
	// Measured 2026-09-25, `-f -c`, against zsh 5.9.2 on this machine, where
	// the uid and the gid differ — `uid=501 gid=20`, which is what makes the
	// row evidence rather than a coincidence:
	//
	//	zsh 5.9.2    UID=[501] EUID=[501] GID=[20] EGID=[20]
	//	bash 5.3.20  UID=[501] EUID=[501] GID=[]   EGID=[]
	//	bash 3.2.57  UID=[501] EUID=[501] GID=[]   EGID=[]
	//
	// So the pair is gated to this dialect, measured rather than assumed:
	// dialect/bash sets `UID` and `EUID` on the two lines this one models and
	// must not grow these, and `env GID=999 bash -c 'echo $GID'` writes 999
	// there because the name is nothing to that shell.
	//
	// `SetSpecial` rather than a plain store for the reason `$UID` uses it:
	// measured, `env GID=999 zsh -f -c 'print $GID'` and `env EGID=999 …`
	// both write 20, so this is a name the environment may not supply. It is
	// the same read-of-the-process the comment above blesses — a gid is
	// per-process, no script changes it, and two Runners in one program
	// genuinely have the same one.
	//
	// **Unset is not a refusal**, which is what the issue that asked for this
	// was about (#4476): a word that expands to nothing is dropped from the
	// command line rather than complained about, so `chgrp $EGID file` ran as
	// `chgrp file` and the *system's* `chgrp` wrote the `usage:` line. The
	// symptom was a third-party program's message and none of ours.
	//
	// Not marked readonly, and that is measured rather than left out: the
	// four identity names are ordinary writable parameters in this shell,
	// unlike bash's, so `MarkReadonly` is the bash answer and is not copied
	// across.
	//
	// The integer letter and the `special` suffix *are* here, and they were
	// the whole of #4488 — `${(t)GID}` is `integer-special` in the reference
	// against a bare `scalar` here, and so was `${(t)UID}`, `${(t)IFS}` and
	// every other name this hook stores. See markTheShellsOwnParameters
	// below for both halves and for why the hook does not do it itself.
	//
	// Assignment is still the other half and is still deliberately absent:
	// `GID=999` in an unprivileged shell is `failed to change group ID:
	// operation not permitted` and stops the shell, while `GID=20` — the gid
	// it already has — is taken at 0, so the rule is "the setgid call
	// happened", not a flat refusal. A library may not make that call (see
	// the purity rule above), and a privileged path is not testable from
	// here, so neither is guessed at.
	r.SetSpecial("GID", strconv.Itoa(os.Getgid()))
	// The login name for that same uid, for the `%n` prompt escape. Asked
	// here for the reason the uid above is: nothing a script does changes
	// it, two shells in one program genuinely have the same one, and it has
	// no other source. Measured as a fact about the *process* rather than
	// about the environment — `%n` ignores `$USER`, `$LOGNAME` and
	// `$USERNAME` however they are set — so it is not read out of a variable.
	//
	// Handed over as a *question* rather than as an answer, and that is the
	// whole of #1403's zsh column. `user.Current` measures 0.83-1.10 ms on
	// darwin — Directory Services, and no cheaper with cgo off — and Apply
	// runs on every invocation, so `zsh -c ':'` and every subshell paid a
	// millisecond to learn a name only a prompt can draw. Asked when `%n` is
	// drawn, the cost lands on the one route that wants it and the answer is
	// kept for the redraws. What is *not* different is who may ask: the core
	// still does not, this dialect still does, and the reasoning above about
	// uid, process and environment stands unchanged.
	//
	// The question itself is interp.LoginName rather than a closure written
	// here, and that is #1446: bash needs the same answer for `\u`, and while
	// this was the only asker the drawer grew a lookup of its own that read
	// `$USER` — which is empty under `env -i` and names the wrong person under
	// `env USER=someone-else`. One question, every asker.
	//
	// Asked **once for three readers** since #4903: `%n` here, and the
	// `$USERNAME` and `$LOGNAME` parameters registered below. sync.OnceValue
	// rather than three closures, so a session that draws a prompt and reads
	// both names pays for one lookup — SetPromptUserFunc wraps what it is
	// given in exactly this, so handing it the shared question changes
	// nothing about the escape.
	loginName := sync.OnceValue(interp.LoginName)
	r.SetPromptUserFunc(loginName)
	// The machine's name, for `%m` and `%M`, from the same place — but by a
	// different road, because this shell puts it in a *parameter* first.
	//
	// Measured 2026-09-13 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin`, `-c`:
	// `$HOST` is the machine's name at startup, and `HOST=a.b.c.d; print -rP
	// '%m|%2m|%M'` writes `a|a.b|a.b.c.d`. So the escapes read the parameter
	// rather than asking the system each time, and the two facts are
	// separable: a shell that set `$HOST` and still asked the system would
	// answer the machine's own name to all three of those. That is what
	// SetPromptHostParameter says and what SetPromptHostFunc could not —
	// the latter keeps its first answer, which an assignable name must not.
	//
	// An inherited `HOST` wins, which is the half `SetSpecial` alone does not
	// give: it guards on the *stored* table, and the environment is a layer
	// under that one, so the name it found unstored was a name the shell had
	// been handed. Measured, `env HOST=injected.example zsh -c 'print -rP
	// "$HOST|%m"'` writes `injected.example|injected`, and this shell wrote
	// the machine's own name for both until the guard read the environment
	// too. It is an ordinary exported scalar then and an ordinary unexported
	// one otherwise — `export HOST=…` against `typeset HOST=…` — and
	// `${(t)HOST}` is a bare `scalar` in both, with none of the `special`
	// that `$UID` and `$IFS` carry. `unset HOST` therefore removes it
	// outright and `%m` draws nothing at 0.
	//
	// Not folded into SetSpecial, because the other users of that hook want
	// what it does: measured, `env UID=999 zsh -c 'print $UID'` writes this
	// uid and not 999, so `$UID` is precisely the parameter the environment
	// may not name. `HOST` is the one that may.
	//
	// Asked eagerly, where the login name beside it is deferred, and the
	// asymmetry is the point rather than an oversight. A parameter has to
	// exist before the first `$HOST` is read and there is no `%m` to hang the
	// question on, so the choice is between paying it and not having the
	// parameter. It is affordable where `user.Current` was not: `os.Hostname`
	// measures 1.5 µs on this machine against 0.83-1.10 ms for the login
	// name, three orders of magnitude apart, and #1403's zsh column was about
	// the millisecond. The issue that asked for this (#2576) quoted the
	// millisecond for the host name too; that figure is the login name's.
	if _, inherited := r.GetVar("HOST"); !inherited {
		r.SetSpecial("HOST", interp.MachineName())
	}
	r.SetPromptHostParameter("HOST")
	// And the prompt-escape table, which is the *same* value the prompt
	// drawer is handed — repl.PromptStyle is an alias for interp.PromptStyle,
	// not a copy. This is what makes `print -P '%F{196}…'` and a drawn prompt
	// one answer rather than two: before it, this shell drew `%F{196}` at a
	// prompt and refused it by name in a script (#1090).
	r.SetPromptStyle(PromptStyle())
	r.SetSpecial("EUID", strconv.Itoa(os.Geteuid()))
	// And the effective gid, the fourth of the identity parameters this shell
	// starts with. See the `$GID` line above for what was measured and for
	// why the pair stops at the value.
	r.SetSpecial("EGID", strconv.Itoa(os.Getegid()))
	r.SetDynamic("RANDOM", func(rr *interp.Runner) string { return rr.Randoms() })
	// And an assignment seeds it, which is what makes a script that uses
	// `RANDOM` reproducible: measured 2026-09-14, `RANDOM=42` twice in one
	// shell gives the same pair of numbers both times here, in ksh93u+ and
	// in zsh 5.9.2. Without the writer the assignment was heard and stored
	// for the producer to find, and the producer had no state to find it
	// with (#2827).
	r.SetDynamicWriter("RANDOM", func(rr *interp.Runner, value string) { rr.SeedRandoms(value) })
	// And the sequence a seeded `RANDOM` answers, which differs from bash's in
	// one bit of the state and from ksh93's in both halves. See random.go, and
	// `docs/spec/random.md` (#4240).
	registerRandoms(r)
	// `typeset -p RANDOM` is `typeset -i10 RANDOM=13859` here — the base
	// rides on the letter in this shell's listing form, and both are facts
	// the parameter has to be told, having no attribute record of its own
	// (#2451).
	r.SetDynamicDeclaration("RANDOM", interp.ProducedDeclaration{Integer: true, Base: 10})
	// And the third answer, which is neither a row nor a refusal: measured
	// 2026-09-12, `typeset -p LINENO` writes nothing at all and reports 0,
	// where the same probe is a row in bash and ksh93 and was
	// `LINENO: not found` here.
	//
	// The integer letter and its base ride along, because the silence is the
	// `-p` word's and the forms that do write the name want them: `typeset
	// -r` writes `integer 10 readonly LINENO=…` and `${(t)LINENO}` is
	// `integer-readonly-special` (#2552).
	r.SetDynamicDeclaration("LINENO", interp.ProducedDeclaration{Integer: true, Base: 10, Silent: true})
	// And it is read-only here, which no other column in the panel says.
	// Measured 2026-09-12, zsh 5.9.2, `env -i PATH=/usr/bin:/bin` with a
	// scratch HOME, over a script file:
	//
	//	unset LINENO       ./b.sh:2: read-only variable: LINENO, and the
	//	                   shell stops
	//	LINENO=9           ./c.sh:1: read-only variable: LINENO
	//	${(t)LINENO}       integer-readonly-special
	//
	// where bash 5.3, bash 3.2, ksh93, dash and BusyBox ash all take both
	// and leave `$LINENO` empty after the `unset` (#2519). It was an
	// ordinary produced parameter here, so a script could remove the name
	// this shell keeps counting into.
	//
	// The mark is deliberately no wider than the two writes above, and that
	// was measured before it was written rather than assumed: a *local*
	// declaration with no value is still allowed — `f() { typeset LINENO;
	// echo $LINENO; }` prints 0 in zsh 5.9.2 and the outer count is intact
	// afterwards — so a script that shadows the name in a function must go
	// on working. `local LINENO=5` is refused, which is the assignment and
	// not the declaration.
	r.MarkReadonly("LINENO")
	// `$ARGC`, this shell's name for `$#` — see argc.go for what was
	// measured and for the startup that could not run without it.
	registerARGC(r)
	r.SetDynamic("SECONDS", func(rr *interp.Runner) string {
		// Whole seconds unless the parameter carries the float attribute,
		// which is how a script asks this one to count in fractions:
		// `typeset -F 3 SECONDS=0` is what a plugin loader opens with and
		// what it reads back to time itself (#1453). Measured 2026-09-07 —
		// three places after `-F 3`, ten after a bare `-F`, and whole
		// seconds with neither.
		//
		// The producer asks rather than the attribute reaching it, because a
		// counted value never passes through the fold every stored name
		// does: there is nothing to fold until the moment of the read.
		if places, ok := rr.FloatPlacesOf("SECONDS"); ok {
			return strconv.FormatFloat(rr.SecondsFrom(), 'f', places, 64)
		}
		return strconv.Itoa(int(rr.SecondsFrom()))
	})
	// `typeset -i10 SECONDS=0`, the same shape RANDOM lists in. The float
	// attribute a script puts on the name with `typeset -F 3 SECONDS=0` is
	// the script's own record and lists from the attribute tables as it
	// always did; this is the shape of the name nobody has declared.
	r.SetDynamicDeclaration("SECONDS", interp.ProducedDeclaration{Integer: true, Base: 10})
	// A NUL as well as the three whitespace characters, which is zsh's alone.
	r.SetSpecial("IFS", " \t\n\x00")
	// The two names the null-command options point at, which no script can
	// reach and which never move — see nullcommand.go in this package.
	registerNullCommandParameters(r)
	tieTheBuiltInPairs(r)
	// And what all of those names say about *themselves*, which nothing said
	// until #4488. After the ties, because half the names it marks are made
	// by them.
	markTheShellsOwnParameters(r)
	// And which of them a `private` declaration may not take into a scope,
	// which is a different set from the one above and is measured rather than
	// derived from it — see scopefixedparameters.go, and #4802.
	markTheScopeFixedParameters(r)
	markTheKindFixedParameters(r)
	// And the names it starts with a *value* in that had no parameter here
	// at all — see startupvalues.go for the ten, for the one the
	// environment may not supply, and for which of them this shell reads
	// back (#4866).
	registerTheStartupValues(r)
	// And the pair that says what the shell is currently **inside**, over a
	// stack the core keeps — see evalcontext.go for the fifteen rows it was
	// measured in, and for the five shapes that deliberately push nothing
	// (#4908).
	registerTheEvalContext(r)
	// And the one of those values that is a **bound** rather than a knob
	// with no reader: the parameter, the bound being that parameter, the
	// sentence that names it, and the refusal ending the script are one
	// change because three of them alone are a wrong answer where an absent
	// parameter is merely a missing one — see funcnesting.go (#4905).
	boundFunctionNesting(r)
	// And the names whose parameter here is **not** the environment's entry
	// of that name, which is what keeps the `export` the environment brought
	// off a value the shell supplies itself — see envnotadopted.go for the
	// sweep, and for why this is a table rather than the rule `special`
	// would have been (#4940).
	markTheEntriesThisShellDoesNotAdopt(r)
	// And the roster this shell's `trap` takes, under the name a script
	// walks to install one — see signalsparameter.go, and note that the
	// middle of it is the `kill -l` this shell already answered correctly
	// (#4906).
	registerTheSignalNames(r)
	// And the three counters the shell keeps and names — the history event,
	// how many subshells deep this is, and how long the terminal has been
	// idle. See counters.go, and note that the second one is the name a
	// probe's own apparatus changes (#4904).
	registerTheCounters(r)
	// And the nine that say what machine this is, which build and which
	// session — see identityvalues.go for the one of the nine whose value
	// the environment wins, and buildtriple.go for the platform table
	// (#4903).
	registerTheIdentityValues(r, loginName)
	// And the four scalar pairs that are one parameter under two names —
	// see promptnames.go, and the theme that could not draw without them.
	registerPromptNames(r)
	// A fifth pair, and the one whose *value* was the fault rather than the
	// tie: `histchars` and `HISTCHARS` — see histchars.go, and the pattern
	// that matched every word once the parameter behind it was empty.
	registerHistoryCharacters(r)
	if dot, ok := r.Builtin("."); ok {
		// The same function under a second name — and *not* a synonym, which
		// is the one row this shell does not share with bash. `source` looks
		// in the current directory before `$path` and `.` never looks there
		// at all, so the second name is the same builtin with one thing asked
		// of it. See interp.Runner.DotLooksInCurrentDirectoryFirst for what
		// was measured and why the difference cannot be an axis.
		r.Register("source", func(rr *interp.Runner, ctx context.Context, args []string) int {
			restore := rr.DotLooksInCurrentDirectoryFirst()
			defer restore()
			return dot(rr, ctx, args)
		})
	}
	// `declare` is `typeset` under a second name rather than a second
	// implementation. ksh93 has only the older name and dash has neither, so
	// which names exist is a dialect's answer and not an axis.
	// The precommand modifiers this shell has and the core does not, as a
	// table of names — see interp.PrecommandModifier for what was measured
	// about each of them and why `command` is not among them.
	//
	// `noglob` is the one with an effect: the plugin manager on the rc file
	// this shell has to run reaches `.zi-set-m-func`, whose whole body is
	// `noglob unset functions[m]`, and an unmatched pattern is fatal here —
	// so the word being ignored ended that function rather than producing a
	// wrong one (#1526).
	//
	// `builtin` and `exec` are named only so the scan reads past them:
	// `builtin noglob echo a[b]c` and `exec noglob echo a[b]c` both print
	// the three characters, measured. They keep their own builtins and do
	// their own work.
	// The words behind which the next written word still names the command,
	// which is a different list from the precommand scan's: `command` stops
	// that scan and is an ordinary builtin here, and the word behind it is
	// still a command name. Measured 2026-09-26 under `setopt
	// magicequalsubst` — `command --opt=~`, `noglob --opt=~` and `builtin
	// --opt=~` all report the word with its tilde as written. See
	// interp/equalscontextposition.go.
	r.SetCommandWordModifier("command")
	r.SetCommandWordModifier("nocorrect")
	// A command word that expanded to exactly `-` belongs to this family,
	// where the other six columns look it up and report 127. It is a
	// modifier and not a word thrown away — it puts a dash on the front of
	// the command's argv[0] and switches off the option scan of every
	// modifier behind it. Not the same question as LoneDashIsAnOption, which
	// this shell also answers yes: that one is a dash-word a builtin was
	// handed (#3236, #5018, #5028).
	r.SetPrecommand("-", interp.PrecommandDash)
	r.SetPrecommand("noglob", interp.PrecommandNoGlob)
	r.SetPrecommand("builtin", interp.PrecommandTransparent)
	// `exec`'s own no-command form is the redirection form, so it is the one
	// word in the family that leaves a redirection something to belong to:
	// `exec >f` is silent at 0 where `builtin >f` and `command >f` are
	// `redirection with no command`.
	r.SetPrecommand("exec", interp.PrecommandRedirectionForm)
	// `command` stops the scan — what follows it is a command name and not a
	// modifier, measured: `command builtin >f` reports 127 for `builtin`.
	// Naming it changes no lookup this shell did not already do; what it adds
	// is that the scan knows a modifier stood there.
	r.SetPrecommand("command", interp.PrecommandStopsTheScan)
	// `integer` is `typeset` with the type already decided, and it is one of
	// the two shells that has the word — `add-zsh-hook` declares integers
	// before it does anything else, so a startup file that installs a
	// hook cannot run without it. Registered rather than built here, so both
	// dialects get the *same* declaration — see interp/integerbuiltin.go.
	r.Register("integer", interp.IntegerBuiltin())
	// And the assignment rule follows the name the way it follows `declare`:
	// `integer n=5+2` is a declaration's operand and not a word to split.
	r.SetDeclaring("integer")
	// `float` is the same thing one letter along, and it is this shell's
	// alone: ksh93 has the word as the preset alias `typeset -lE` and the
	// other three answer `float: not found`. It was missing outright until
	// #3291 — `float c=1.5` was `command not found` here, in a shell whose
	// own `$reswords` names the word — so `add-zsh-hook`'s neighbors in a
	// startup file had one declaration keyword of the seven that did not
	// exist.
	r.Register("float", interp.FloatBuiltin())
	r.SetDeclaring("float")
	// `private` is `local` making a binding the functions this call goes on
	// to invoke read straight **past** — to whatever the declaration
	// displaced, not to nothing — which is the one thing a `local` cannot be
	// wrapped into doing and the whole reason the word exists. Registered
	// rather than built here for `integer`'s reason: the declaration itself
	// is interp's, and only the visibility rule is new. See
	// interp/privatescope.go for the measured table and
	// interp/privatebuiltin.go for why the word is here from the start
	// rather than behind `zmodload zsh/param/private` — the shell being
	// modeled autoloads the module off the word, so a script that never
	// writes the `zmodload` still has it.
	r.Register("private", interp.PrivateBuiltin())
	r.SetDeclaring("private")
	// And the classification this shell's grammar gives all seven of them.
	// zsh's reserved-word table is not its parser's in the sense the other
	// four dialects' are: the seven declaration commands and four words the
	// rest of the panel has no construct for are in it, and `in` and `]]` —
	// which the POSIX grammar's table holds — are not. zshReservedWords is
	// that table, measured twice and already relied on by `$reswords`, so
	// the classification reads the one list rather than a second copy of it.
	r.SetReservedWords(zshReserves)
	// `functions` is `typeset -f` under this shell's own name and
	// `unfunction` is `unset -f` under its own — the same registration
	// `integer` gets and for the same reason, so that the listing and the
	// removal have one implementation between two words each. See
	// interp/functionsbuiltin.go.
	//
	// The plugin manager on the rc file this shell has to run is why they
	// exist: `unfunction` is on 47 lines of it, nine of them consecutive in
	// the path out of every plugin load, and a `command not found` there
	// leaves that loader's temporary stubs standing over `compdef`,
	// `autoload`, `source`, `bindkey`, `zstyle`, `alias` and `zle` for the
	// rest of the session (#1487).
	// `rehash` and `unhash`, which are this shell's names for clearing the
	// command hash and for taking one entry out of a table. The behavior was
	// here — `hash -r` empties the table — and the *names* were not, so a
	// startup file that added a directory to `$path` and called `rehash`
	// died on that line at 127, which reads to a script exactly like a typo.
	// Registered rather than written as prelude functions because each
	// refuses under its own name: `rehash -x` is `bad option: -x` naming
	// `rehash`, where a function calling `hash -r` would have named `hash`.
	// See interp/rehashbuiltin.go (#3109).
	r.Register("rehash", interp.RehashBuiltin())
	r.Register("unhash", interp.UnhashBuiltin())
	r.Register("functions", functionsCopying(interp.FunctionsBuiltin()))
	r.Register("unfunction", interp.UnfunctionBuiltin())
	if typeset, ok := r.Builtin("typeset"); ok {
		r.Register("declare", typeset)
		// And the assignment rule follows the name: `declare x=*` stores the
		// character here, where in a shell without the name it would be an
		// ordinary command with an ordinary globbed argument.
		r.SetDeclaring("declare")
	}
	// How a function is laid out when something says one back — `typeset
	// -f` here; this shell writes nothing into the environment, so the
	// exported arrangement is the shown one.
	r.SetFunctionLayout(FunctionLayout(), FunctionLayout())
	// And, **last**, the builtins a module has to be loaded for: they are
	// registered above like every other one and taken out of the table here,
	// so that the registration stays where the builtin is written and this
	// is the only line that knows which of them wait. Ordering is the whole
	// of why it is here rather than beside each: a withdrawal written before
	// a registration is undone by it. See gatedbuiltins.go (#4997).
	withdrawGatedBuiltins(r)
	// And the parameters a shell started as `sh` or `ksh` never had, after
	// every registration for the same reason. See emulationhiding.go.
	hideTheParametersAnEmulationDoesNotHave(r)
}
