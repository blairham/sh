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
// The *set* is measured here and the *value* each reset name goes back to is
// measured below — they are two tables of the same size and #2515 built only
// the first, which is why #2549 existed. What is still not modeled is what
// most of those values then *do*: 130 of the 185 names are recorded rather
// than implemented, so an emulation now puts them at the emulation's own
// state and the state is still read by nothing. The ones with behavior behind
// them — `multios`, `bareglobqual`, `globsubst`, `typesetsilent`,
// `checkjobs`, `unset`, `equals` — are where a wrong value was a wrong shell.
//
// # Nobody to disagree with
//
// No other shell in the panel has `emulate` at all: bash, dash, ksh93 and
// BusyBox ash each answer `command not found`, and ash cannot be reached for
// this question even in principle. So this is a dialect answer written down
// in `dialect/zsh` and not a semantics axis — the same conclusion #2426
// reached for `functrace` and `extdebug`, and for the same reason.

// # And what each reset name goes back to
//
// The set above is one table and the *value* is a second of the same size.
// Real zsh has a default per emulation — `emulate sh` turns `posixbuiltins`
// on and `multios` off — and until #2549 every reset name went back to
// **zsh's** default here, so 49 of the 81 a bare `emulate sh` puts back
// landed on the wrong value on every call.
//
// Measured 2026-09-13 on zsh 5.9.2, `emulate -R <mode>` from a fresh
// `env -i zsh` reading a script, one `${options[name]}` per line for all 197
// names in the table, four runs. `-R` is what exposes the emulation's default
// for every name; a bare `emulate` exposes it only for the ones it resets.
// The probe prints from *inside* the emulation, one print per name, because
// the obvious shape does not survive the thing it is measuring:
// `${(ko)options}` under `emulate -R sh` answers one line, since the
// emulation has already changed how that expansion splits.
//
// 62 names differ from zsh's default in at least one emulation. Three of
// them are compat spellings of another — `braceexpand`, `histexpand` and
// `promptvars` — so they are not here: an alias resolves to its canonical
// entry before anything reads a default, and a second entry for one option
// is a second place for it to disagree with itself. That leaves 59, of which
// 49 are in the bare-reset 81 and the other ten move only under `-R`.
//
// A name absent from this table has the same default in all four, which is
// most of the table and is why this is a map rather than three fields on
// every one of 197 entries.

// emulationDefault is what one option is set to when the shell emulates this
// mode: the mode's own default where they differ, and the table's otherwise.
func emulationDefault(o zshOption, mode string) bool {
	d, ok := emulationDefaults[o.base]
	if !ok {
		return o.def
	}
	switch mode {
	case "sh":
		return d.sh
	case "ksh":
		return d.ksh
	case "csh":
		return d.csh
	}
	// `emulate zsh` is the table's own default for every name, which is what
	// makes zsh the mode with no column here.
	return o.def
}

// emulationDeviates reports whether this mode's default for a name is a
// deviation from the table's — the question the recorded store asks, since
// what it holds is deviations rather than states.
//
// The second result is what keeps a recordedOver name out of it. Those hold a
// base state that is *not* the table default — `hashdirs`, `login` and `rcs`
// — so a deviation computed against `o.def` would be the wrong bit for them.
// None of the three is in the table below, and
// TestNoRecordedOverNameHasAnEmulationDefault keeps it that way rather than
// this sentence doing it.
func emulationDeviates(o zshOption, mode string) (dev, known bool) {
	if _, ok := emulationDefaults[o.base]; !ok {
		return false, false
	}
	return emulationDefault(o, mode) != o.def, true
}

// emulationDefaults are the 59 canonical names whose default differs from
// zsh's in at least one emulation, with each emulation's own value. See the
// measurement above.
var emulationDefaults = map[string]struct{ sh, ksh, csh bool }{
	"aliasfuncdef":        {sh: true, ksh: true, csh: false},
	"appendcreate":        {sh: true, ksh: true, csh: false},
	"badpattern":          {sh: false, ksh: false, csh: true},
	"banghist":            {sh: false, ksh: false, csh: true},
	"bareglobqual":        {sh: false, ksh: false, csh: false},
	"bgnice":              {sh: false, ksh: false, csh: true},
	"bsdecho":             {sh: true, ksh: false, csh: false},
	"checkjobs":           {sh: false, ksh: false, csh: false},
	"checkrunningjobs":    {sh: false, ksh: false, csh: false},
	"cprecedences":        {sh: true, ksh: true, csh: true},
	"cshjunkiehistory":    {sh: false, ksh: false, csh: true},
	"cshjunkieloops":      {sh: false, ksh: false, csh: true},
	"cshjunkiequotes":     {sh: false, ksh: false, csh: true},
	"cshnullcmd":          {sh: false, ksh: false, csh: true},
	"cshnullglob":         {sh: false, ksh: false, csh: true},
	"equals":              {sh: false, ksh: false, csh: false},
	"evallineno":          {sh: false, ksh: false, csh: false},
	"extendedhistory":     {sh: false, ksh: false, csh: true},
	"functionargzero":     {sh: false, ksh: false, csh: true},
	"globalexport":        {sh: false, ksh: false, csh: false},
	"globassign":          {sh: false, ksh: false, csh: true},
	"globsubst":           {sh: true, ksh: true, csh: true},
	"hup":                 {sh: false, ksh: false, csh: false},
	"ignorebraces":        {sh: true, ksh: false, csh: false},
	"interactivecomments": {sh: true, ksh: true, csh: false},
	"ksharrays":           {sh: true, ksh: true, csh: false},
	"kshautoload":         {sh: true, ksh: true, csh: false},
	"kshglob":             {sh: false, ksh: true, csh: false},
	"kshoptionprint":      {sh: false, ksh: true, csh: false},
	"localoptions":        {sh: false, ksh: true, csh: false},
	"localtraps":          {sh: false, ksh: true, csh: false},
	"multifuncdef":        {sh: false, ksh: false, csh: false},
	"multios":             {sh: false, ksh: false, csh: false},
	"nomatch":             {sh: false, ksh: false, csh: true},
	"notify":              {sh: false, ksh: false, csh: false},
	"octalzeroes":         {sh: true, ksh: false, csh: false},
	"pathscript":          {sh: true, ksh: true, csh: false},
	"posixaliases":        {sh: true, ksh: true, csh: false},
	"posixbuiltins":       {sh: true, ksh: true, csh: false},
	"posixcd":             {sh: true, ksh: true, csh: false},
	"posixidentifiers":    {sh: true, ksh: true, csh: false},
	"posixjobs":           {sh: true, ksh: true, csh: false},
	"posixstrings":        {sh: true, ksh: true, csh: false},
	"posixtraps":          {sh: true, ksh: true, csh: false},
	"promptbang":          {sh: false, ksh: true, csh: false},
	"promptpercent":       {sh: false, ksh: false, csh: true},
	"promptsubst":         {sh: true, ksh: true, csh: false},
	"rmstarsilent":        {sh: true, ksh: true, csh: false},
	"sharehistory":        {sh: false, ksh: true, csh: false},
	"shfileexpansion":     {sh: true, ksh: true, csh: false},
	"shglob":              {sh: true, ksh: true, csh: false},
	"shnullcmd":           {sh: true, ksh: true, csh: false},
	"shoptionletters":     {sh: true, ksh: true, csh: false},
	"shortloops":          {sh: false, ksh: false, csh: true},
	"shwordsplit":         {sh: true, ksh: true, csh: false},
	"singlelinezle":       {sh: false, ksh: true, csh: false},
	"typesetsilent":       {sh: true, ksh: true, csh: false},
	"typesettounset":      {sh: true, ksh: true, csh: false},
	"unset":               {sh: true, ksh: true, csh: false},
}

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
