// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "strings"

// Which options an emulation puts back, which it leaves where it found them,
// and which it never touches at all. Three lists rather than one, because the
// answer is three-valued and the wrong two-valued reading is what #2515 was.
//
// # What was measured
//
// zsh 5.9.2 on 2026-09-12, one option at a time, through `${options[…]}`. For
// every name in `$options` the probe read the state three ways — as a default
// shell has it, after the option had been moved away from that default, and
// after the same move followed by `emulate -R` — and compared the three. A
// name whose state after the emulation is the same whichever state it started
// from was *reset*; one that still holds what the script put there was *left*.
// Both starting points are needed: where the emulation's own default happens
// to equal the moved-to state, the flip alone cannot tell the two readings
// apart, and a bulk answer taken from either half alone hides exactly the
// exceptions this file is for.
//
// The result splits the 185 names in three:
//
//   - 81 that **every** form of `emulate` resets. They are the
//     portability-relevant ones — how a word splits, what an unmatched glob
//     does, where an array starts, the nine `posix*` names, the `csh*` and
//     `ksh*` and `sh*` families, and the local-scoping switches `emulate -L`
//     then turns back on.
//   - 95 that only `emulate -R` resets. They are the ones a *person* sets:
//     history, completion, correction, the line editor, the prompt, `xtrace`
//     and `verbose`. A bare `emulate sh` in the middle of somebody's rc file
//     leaves all of them alone.
//   - 9 that **no** form resets, eight of them measured and one unanswerable.
//
// # The nine
//
// Eight of them describe how the shell was started rather than how it
// behaves, and every one is left standing by `emulate -R zsh` as squarely as
// by a bare `emulate sh`: `interactive`, `login`, `monitor`, `privileged`,
// `restricted`, `shinstdin`, `singlecommand` and `zle`. Five of those refuse
// to move at all in a script on a pipe, so they were measured a second time
// through a pseudo-terminal, where `monitor` and `zle` do move — and there
// they still stand through all four emulations. (Read inside `$( … )` they
// both read `off`, which is the command substitution's own subshell and not
// the emulation: the first pass of this probe captured its readings that way
// and had them down as reset.)
//
// `exec` is the ninth and it is **unanswerable from inside the shell**. The
// option off is `set -n`: commands are parsed and not run, so the `emulate`
// that would answer the question never executes and neither does anything
// that could report. It is listed here rather than with the 81 because
// leaving an option alone cannot make a shell run something it was told not
// to run, and because its two siblings — `verbose` and `xtrace`, the other
// two of `set -n -v -x` — are both measured as left alone.
//
// # What this is not
//
// The *set* is measured; the *value* each reset name goes back to is not
// modeled beyond the four axes emulate.go swaps. Real zsh has a default per
// emulation — `emulate sh` turns `posixbuiltins` on and `multios` off, where
// this shell puts both back to zsh's default — and 47 of the 81 differ that
// way under `sh`. That is a separate gap from this one and does not change
// which names move.
//
// # Nobody to disagree with
//
// No other shell in the panel has `emulate` at all: bash, dash, ksh93 and
// BusyBox ash each answer `command not found`, and ash cannot be reached for
// this question even in principle. So this is a dialect answer written down
// in `dialect/zsh` and not a semantics axis — the same conclusion #2426
// reached for `functrace` and `extdebug`, and for the same reason.

// emulationClass is what one option does when the shell emulates.
type emulationClass uint8

const (
	// emulationKeeps: no form of `emulate` touches this name.
	emulationKeeps emulationClass = iota
	// emulationStrictOnly: `emulate -R` resets it; a bare `emulate` does not.
	emulationStrictOnly
	// emulationAlways: every form resets it.
	emulationAlways
)

// emulationAlwaysReset are the 81 a bare `emulate` puts back.
const emulationAlwaysReset = `
	aliases aliasfuncdef allexport appendcreate autocd badpattern
	bareglobqual bgnice braceccl bsdecho cdablevars chasedots chaselinks
	checkjobs checkrunningjobs clobber cprecedences cshjunkiehistory
	cshjunkieloops cshjunkiequotes cshnullcmd cshnullglob equals errexit
	errreturn evallineno extendedglob functionargzero glob globalexport
	globassign globdots globstarshort globsubst histsubstpattern hup
	ignorebraces ignoreclosebraces ksharrays kshautoload kshglob
	kshoptionprint localloops localoptions localpatterns localtraps
	magicequalsubst multifuncdef multios nomatch nullglob
	numericglobsort octalzeroes pathdirs pathscript pipefail
	posixaliases posixargzero posixbuiltins posixcd posixidentifiers
	posixjobs posixstrings posixtraps pushdignoredups pushdminus
	pushdtohome rcexpandparam rcquotes shfileexpansion shglob shnullcmd
	shoptionletters shortloops shortrepeat shwordsplit typesetsilent
	typesettounset unset warncreateglobal warnnestedvar
`

// emulationStrictReset are the 95 only `emulate -R` puts back.
const emulationStrictReset = `
	alwayslastprompt alwaystoend appendhistory autocontinue autolist
	automenu autonamedirs autoparamkeys autoparamslash autopushd
	autoremoveslash autoresume banghist bashautolist bashrematch beep
	caseglob casematch casepaths cbases cdsilent clobberempty
	combiningchars completealiases completeinword continueonerror
	correct correctall debugbeforecmd dvorak emacs extendedhistory
	flowcontrol forcefloat globalrcs globcomplete hashcmds hashdirs
	hashexecutablesonly hashlistall histallowclobber histbeep
	histexpiredupsfirst histfcntllock histfindnodups histignorealldups
	histignoredups histignorespace histlexwords histnofunctions
	histnostore histreduceblanks histsavebycopy histsavenodups
	histverify ignoreeof incappendhistory incappendhistorytime
	interactivecomments kshtypeset kshzerosubscript listambiguous
	listbeep listpacked listrowsfirst listtypes longlistjobs mailwarning
	markdirs menucomplete multibyte notify overstrike printeightbit
	printexitvalue promptbang promptcr promptpercent promptsp
	promptsubst pushdsilent rcs recexact rematchpcre rmstarsilent
	rmstarwait sharehistory singlelinezle sourcetrace sunkeyboardhack
	transientrprompt trapsasync verbose vi xtrace
`

// emulationNeverReset are the nine no emulation touches. See the eight and
// the one above.
const emulationNeverReset = `
	exec interactive login monitor privileged restricted shinstdin
	singlecommand zle
`

// emulationClasses is the three lists as one lookup, built once. A name
// missing from all three would silently read as `emulationKeeps`, which is
// what TestTheEmulationPartitionCoversTheTable exists to stop: the lists are
// paired against the option table rather than trusted, because a partition
// only half of which is written down is the shape that has drifted here
// before.
var emulationClasses = func() map[string]emulationClass {
	m := make(map[string]emulationClass, len(zshOptions))
	for _, name := range strings.Fields(emulationAlwaysReset) {
		m[name] = emulationAlways
	}
	for _, name := range strings.Fields(emulationStrictReset) {
		m[name] = emulationStrictOnly
	}
	for _, name := range strings.Fields(emulationNeverReset) {
		m[name] = emulationKeeps
	}
	return m
}()

// resetByEmulation reports whether one canonical name goes back to a default
// when the shell emulates. strict is the `-R` form.
func resetByEmulation(base string, strict bool) bool {
	switch emulationClasses[base] {
	case emulationAlways:
		return true
	case emulationStrictOnly:
		return strict
	}
	return false
}
