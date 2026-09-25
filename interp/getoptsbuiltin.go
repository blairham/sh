// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strconv"
	"strings"
)

// `getopts`, the fourth builtin that was not one — and the only one of them
// that a borrowed program could never have done.
//
// `test`, `[`, `kill` and `printf` all worked after a fashion while they were
// separate programs, because what they do is visible from outside. This is
// not: getopts sets `name`, `OPTARG` and `OPTIND` in the *calling* shell, and
// a child process cannot reach them. macOS ships a 120-byte /usr/bin/getopts,
// so the lookup found something, ran it, and got back a status of 1 — which
// reads as "no more options" and makes the loop around it exit immediately.
//
//	while getopts "ab:" opt; do ...; done
//
// never ran its body, said nothing, and reported success. That is the silent
// wrong answer this package exists to avoid, and it survived because no
// corpus case used getopts.
//
// The behavior is almost entirely unanimous, which is unusual for a builtin
// this fiddly: OPTIND, clustered options like `-ab`, an argument attached as
// `-bval` or separate as `-b val`, `--` ending the options, a non-option
// ending them, and the `:` prefix that turns complaints off and reports
// through OPTARG instead — all four agree on every one.
//
// Only the two complaints diverge, and they diverge in where they are printed
// as much as in what they say: bash names itself without a line where it
// names a line everywhere else, and dash prints neither.

func init() {
	builtins["getopts"] = biGetopts
}

func biGetopts(r *Runner, _ context.Context, args []string) int {
	// It has no options of its own, so a leading `-` word is the only thing
	// worth asking about: the dialects split between refusing it as an
	// option and reading it as the optstring — see the axis's comment for
	// which does which, and for the contaminated probe that first read ksh93
	// wrong. An ordinary `getopts ab o` reaches no question at all, which is
	// what keeps this off the path every use of it takes.
	if len(args) > 0 && len(args[0]) > 1 && args[0][0] == '-' {
		if args[0] == "--" {
			args = args[1:]
		} else if r.ask(r.sem().GetoptsRejectsUnknownOption, "`getopts -q` refused as an option rather than read as the optstring") {
			if r.unspecified {
				return r.status
			}
			return r.refuseOption("getopts", args[0], "")
		} else if r.unspecified {
			return r.status
		}
	}
	if len(args) < 2 {
		// The same line a bad option earns, and it is the dialect's own —
		// bash and ksh93 already have one in Diagnostics.BuiltinUsage and
		// write it with no location in front, which is what
		// builtinUsageLine does with it. Measured 2026-09-14, `getopts` with
		// no operands and with one draw the same line in all seven, and only
		// the word for the slot the shell writes into differs: `var` in dash
		// and BusyBox ash, `name` in bash. The fallback is the substrate's
		// own, for a dialect with no usage line at all (#2801).
		if r.diag().BuiltinUsage["getopts"] != "" {
			r.builtinUsageLine("getopts")
		} else {
			r.diagf("getopts: usage: getopts optstring name [arg]\n")
		}
		return orDefault(r.diag().GetoptsUsageStatus, 2)
	}
	optstring, name := args[0], args[1]
	refusedName := !r.isGetoptsName(name)
	if refusedName && r.sem().BadNameToGetoptsFatal == Yes {
		// The refusal ends the shell here, so whether the scan would have
		// run is not a question anything downstream can ask: nothing reads
		// OPTIND back. Asked before the axis rather than answered by it —
		// see Semantics.GetoptsRefusedNameStillScans, which records zsh as
		// unpinned for exactly this reason.
		return r.badGetoptsName(name)
	}
	if refusedName && !r.ask(r.sem().GetoptsRefusedNameStillScans,
		"a refused `getopts` name operand still scanning") {
		// Judged before anything is scanned, which is one of the two answers
		// — ksh93's. The note here used to say it was every column's and
		// that "OPTIND is not moved either way", which #3555's probe could
		// not have seen: it printed the status and never printed OPTIND. See
		// Semantics.GetoptsRefusedNameStillScans for the four columns that
		// were re-measured.
		return r.badGetoptsName(name)
	}
	if refusedName {
		// The other answer: the builtin does its ordinary work and the
		// refusal lands on the *store*. The diagnostic and the status are
		// the refusal's, and the scan below runs for its effect on OPTIND
		// alone — getoptsWrite is suppressed while it does, so the
		// parameter keeps the value it had.
		// The scan first and the refusal after, which is the order the
		// measurement implies and not merely an implementation choice: the
		// refusal is what a *store* earns, so everything that happens before
		// the store has already happened when it is written. Writing it
		// first also stops the scan, because a fatal refusal leaves the
		// runner unwilling to do any more work — which is how this was
		// first written, and it left OPTIND where it started.
		func() {
			defer func(was string) { r.getoptsRefusedName = was }(r.getoptsRefusedName)
			r.getoptsRefusedName = name
			r.getoptsScan(optstring, name, args[2:])
		}()
		return r.badGetoptsName(name)
	}
	return r.getoptsScan(optstring, name, args[2:])
}

// getoptsScan is the builtin's ordinary work: find the next option in the
// words, move OPTIND over what it consumed, and store what it found.
//
// Split out so that the dialects whose refused *name* still scans can run it
// for the cursor alone — see Semantics.GetoptsRefusedNameStillScans. The
// store is what the refusal takes away, and getoptsWrite is where it is
// taken; everything here happens either way, which is what makes `getopts x
// 1bad foo` leave OPTIND at 1 in bash while `getopts x 1bad -x` moves it to
// 2. A fixed increment would have got that pair wrong.
func (r *Runner) getoptsScan(optstring, name string, given []string) int {
	// The operands to scan are the ones given, or the shell's own parameters
	// when none are — which is what every use of it in a script relies on.
	words := given
	if len(words) == 0 {
		words = r.Params
	}

	// A leading colon turns the complaints off and reports through OPTARG
	// instead, which is how a script takes over the reporting.
	silent := strings.HasPrefix(optstring, ":")
	spec := strings.TrimPrefix(optstring, ":")

	ind := r.optIndex()
	if r.optindAssigned {
		// The script wrote OPTIND itself — resetting it to 1 to scan a second
		// list is the documented way to do that — so any position inside a
		// cluster belongs to the old scan. Three of the four drop it; asked
		// only here, where there is an assignment to have an opinion about.
		r.optindAssigned = false
		if r.ask(r.sem().GetoptsAssignmentRestartsWord, "assigning OPTIND restarting the word") {
			r.optChar = 1
		}
	}

	i := ind - 1
	if i < 0 || i >= len(words) {
		return r.getoptsEnd(name, ind)
	}
	word := words[i]
	if _, isOption := r.getoptsOptionSign(word); !isOption {
		return r.getoptsEnd(name, ind)
	}
	if word == "--" {
		return r.getoptsEnd(name, ind+1)
	}
	if r.optChar < 1 {
		r.optChar = 1
	}
	if r.optChar >= len(word) {
		// The cluster is spent; go on to the next word.
		r.optChar = 1
		return r.getoptsAt(name, spec, silent, words, ind+1)
	}
	return r.getoptsAt(name, spec, silent, words, ind)
}

// getoptsOptionSign reports which sign the scan reads this word as an option
// under, and whether it is an option word at all.
//
// A word of one character is never one — `-` and `+` alone are operands
// everywhere, and so is the empty word — and the axis is put only to a word
// that begins with `+`, so a scan over ordinary `-` words and ordinary
// operands reaches no question. `--` comes back an option word here and is
// caught by the caller: it ends the options rather than naming one, and `++`
// does not, which is the measured asymmetry.
func (r *Runner) getoptsOptionSign(word string) (byte, bool) {
	if len(word) < 2 {
		return 0, false
	}
	switch word[0] {
	case '-':
		return '-', true
	case '+':
		if r.ask(r.sem().GetoptsTakesAPlusPrefixedOption,
			"a `+`-prefixed word read as an option rather than as an operand") {
			return '+', true
		}
	}
	return 0, false
}

// getoptsLetter is the letter as the builtin reports it, which carries the
// sign of the word it came out of where that was a `+`.
//
// A bare letter for a `-` word rather than `-a`, because that is what every
// column writes into the name: the sign is how the one dialect with both
// spellings says which it read, and it would be noise where there is only
// one.
func getoptsLetter(sign byte, c byte) string {
	if sign == '+' {
		return "+" + string(c)
	}
	return string(c)
}

// getoptsAt reads the option at the current position.
func (r *Runner) getoptsAt(name, spec string, silent bool, words []string, ind int) int {
	i := ind - 1
	if i >= len(words) {
		return r.getoptsEnd(name, ind)
	}
	word := words[i]
	sign, isOption := r.getoptsOptionSign(word)
	if !isOption || word == "--" {
		// Re-checked because the position moved: `-a file` stops here rather
		// than reading `file` as a cluster.
		if word == "--" {
			return r.getoptsEnd(name, ind+1)
		}
		return r.getoptsEnd(name, ind)
	}
	if r.optChar >= len(word) {
		r.optChar = 1
		return r.getoptsAt(name, spec, silent, words, ind+1)
	}

	c := word[r.optChar]
	// The sign goes with the letter everywhere the letter is *reported* —
	// into the name, into OPTARG in silent mode, and into the complaint —
	// which is how a script tells `+a` from `-a` in the dialect that has
	// both. See Semantics.GetoptsTakesAPlusPrefixedOption. It is carried
	// beside the letter rather than glued to it because the two are wanted
	// apart in the wording: `getopts a o -+` has the sign of one word and the
	// letter of another.
	letter := getoptsLetter(sign, c)
	kind, known := r.getoptsLetterTakes(spec, c)
	switch {
	case !known || c == ':':
		if r.getoptsErrorEndsTheWord() {
			return r.getoptsBad(name, sign, c, silent, getoptsUnknownOption, func() bool {
				r.optChar = 1
				return r.setOptind(ind + 1)
			})
		}
		return r.getoptsBad(name, sign, c, silent, getoptsUnknownOption, func() bool { return r.advance(word, ind) })
	case kind == getoptsTakesANumber:
		return r.getoptsNumericArgument(name, sign, c, silent, word, words, ind)
	case kind == getoptsTakesAString:
		// The option takes an argument: the rest of this word if there is
		// any, and the next word otherwise.
		if rest := word[r.optChar+1:]; rest != "" {
			r.optChar = 1
			// OPTARG ahead of OPTIND, which is the order a refusal makes
			// visible: measured, a frozen OPTARG leaves dash and BusyBox
			// ash with OPTIND still at its old value, so neither had
			// written it by the time the refusal ended the builtin.
			if !r.getoptsWrite("OPTARG", rest) && r.getoptsRefusalEndsTheBuiltin() {
				return getoptsRefusedStatus
			}
			if !r.setOptind(ind+1) && r.getoptsRefusalEndsTheBuiltin() {
				return getoptsRefusedStatus
			}
			return r.getoptsSetName(name, letter, 0)
		}
		if ind >= len(words) {
			// The argument is missing, so nothing was consumed and this is
			// the same question advance asks: the word is spent and the
			// count stays where it was in the one column that lags — unless
			// the refusal is what ends the word, which counts it here and
			// is the only thing that can show this axis at a missing
			// argument. See Semantics.GetoptsErrorEndsTheWord.
			if !r.getoptsErrorEndsTheWord() && r.ask(r.sem().GetoptsCountsTheWordOnTheNextCall,
				"OPTIND staying on a spent word until the next `getopts` call") {
				r.optChar = len(word)
				return r.getoptsBad(name, sign, c, silent, getoptsMissingArgument, func() bool { return r.setOptind(ind) })
			}
			r.optChar = 1
			return r.getoptsBad(name, sign, c, silent, getoptsMissingArgument, func() bool { return r.setOptind(ind + 1) })
		}
		r.optChar = 1
		if !r.getoptsWrite("OPTARG", words[ind]) && r.getoptsRefusalEndsTheBuiltin() {
			return getoptsRefusedStatus
		}
		if !r.setOptind(ind+2) && r.getoptsRefusalEndsTheBuiltin() {
			return getoptsRefusedStatus
		}
		return r.getoptsSetName(name, letter, 0)
	default:
		// OPTARG first here too, for the reason the argument-taking branch
		// writes it first: a refusal leaves dash and BusyBox ash with OPTIND
		// where it was.
		if !r.clearOptargForAnArgumentlessOption() && r.getoptsRefusalEndsTheBuiltin() {
			return getoptsRefusedStatus
		}
		if !r.advance(word, ind) && r.getoptsRefusalEndsTheBuiltin() {
			return getoptsRefusedStatus
		}
		return r.getoptsSetName(name, letter, 0)
	}
}

// getoptsComplaint is which of the three things `getopts` found wrong. It
// decides the wording, and in silent mode it decides whether the name is
// written `?` or `:` — a letter the string does not have is the first, and
// anything about the argument the letter *does* take is the second.
type getoptsComplaint uint8

const (
	getoptsUnknownOption getoptsComplaint = iota
	getoptsMissingArgument
	getoptsBadNumericArgument
)

// getoptsArgumentKind is what a letter in the option string takes.
type getoptsArgumentKind uint8

const (
	getoptsTakesNothing getoptsArgumentKind = iota
	// getoptsTakesAString is the POSIX `letter:` — the rest of the word if
	// there is any, and the next word otherwise, whatever it holds.
	getoptsTakesAString
	// getoptsTakesANumber is `letter#`, which one dialect has and the rest
	// read as a second option letter. See
	// Semantics.GetoptsOptionStringHasANumericType.
	getoptsTakesANumber
)

// getoptsSpecLookup walks the option string and answers two questions about
// it at once: what the letter c takes, and whether a `#` sits anywhere a type
// marker goes.
//
// One walk for both because the second question decides how the first is
// read, and a separate scan for it is exactly the shape that lets the two
// come to disagree about where a marker is: with the numeric type off a `#`
// after a letter is a *letter*, so every position after it shifts, and a
// standalone "does this string have a numeric marker" scan would be reading a
// different string from the one the lookup walked.
//
// The lookup is a walk rather than an IndexByte because a marker is not an
// option: `getopts 'n#' o -#` is an unknown option in the shell that has the
// type, and `getopts '#n' o -#` — where the `#` follows no letter — is the
// option `#` in every shell here. Measured 2026-09-16 on ksh93u+ 2012-08-01.
func getoptsSpecLookup(spec string, c byte, numeric bool) (kind getoptsArgumentKind, found, marked bool) {
	for i := 0; i < len(spec); {
		letter := spec[i]
		i++
		k := getoptsTakesNothing
		if i < len(spec) {
			switch spec[i] {
			case ':':
				k, i = getoptsTakesAString, i+1
			case '#':
				marked = true
				if numeric {
					k, i = getoptsTakesANumber, i+1
				}
			}
		}
		if letter == c && !found {
			kind, found = k, true
		}
	}
	return kind, found, marked
}

// getoptsLetterTakes is that lookup with the axis put in, and it is the only
// place the axis is asked: an option string with no `#` where a marker goes —
// which is every option string a portable script writes — reaches no question
// at all.
func (r *Runner) getoptsLetterTakes(spec string, c byte) (getoptsArgumentKind, bool) {
	kind, found, marked := getoptsSpecLookup(spec, c, true)
	if !marked {
		return kind, found
	}
	if r.ask(r.sem().GetoptsOptionStringHasANumericType,
		"a `#` in an option string naming a numeric argument rather than another option letter") {
		return kind, found
	}
	kind, found, _ = getoptsSpecLookup(spec, c, false)
	return kind, found
}

// getoptsNumericArgument reads the argument a `letter#` option takes.
//
// It is the `letter:` branch's shape with one thing added and one thing taken
// away. Added: the argument has to *be* a numeral, and is complained about
// where it is not. Taken away: an attached argument no longer spends the
// word, because the numeral ends where it ends — `-n5x` reads 5 and leaves
// the scan on `x`, which the next call reports as an option of its own.
//
// Measured 2026-09-16 on ksh93u+ 2012-08-01, over a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin on /dev/null, `getopts 'n#' o`:
//
//	-n 5        n   OPTARG=5       OPTIND 1 -> 3
//	-n5         n   OPTARG=5       OPTIND 1 -> 2
//	-n -3       n   OPTARG=-3      OPTIND 1 -> 3
//	-n5x        n   OPTARG=5x      OPTIND 1 -> 1, then `-x: unknown option`
//	-n abc      ?   OPTARG unset   OPTIND 1 -> 3, `-n: numeric argument expected`
//	-nabc       ?   OPTARG unset   OPTIND 1 -> 2, the same line
//	-n          ?   OPTARG unset   OPTIND 1 -> 2, the same line
//
// The fourth row is the one worth writing down: OPTARG holds the **rest of
// the word** and not the numeral, so the `x` the scan is about to report as
// an option is in the argument as well. Recorded as measured rather than
// tidied — a shell reading OPTARG as a number stops at the same place the
// scan did.
func (r *Runner) getoptsNumericArgument(name string, sign, c byte, silent bool, word string, words []string, ind int) int {
	letter := getoptsLetter(sign, c)
	if rest := word[r.optChar+1:]; rest != "" {
		n, ok := getoptsNumeral(rest)
		if !ok {
			// Nothing numeric there, and the rest of the word goes with the
			// complaint rather than being read as more option letters.
			return r.getoptsBad(name, sign, c, silent, getoptsBadNumericArgument,
				func() bool { return r.advanceBy(word, ind, len(word)-r.optChar) })
		}
		// OPTARG ahead of OPTIND, for the reason the string branch writes it
		// first: a refusal is visible in the order.
		if !r.getoptsWrite("OPTARG", rest) && r.getoptsRefusalEndsTheBuiltin() {
			return getoptsRefusedStatus
		}
		if !r.advanceBy(word, ind, 1+n) && r.getoptsRefusalEndsTheBuiltin() {
			return getoptsRefusedStatus
		}
		return r.getoptsSetName(name, letter, 0)
	}
	if ind >= len(words) {
		// Missing outright, which is the same question the string branch
		// asks here — the word is spent and the count stays where it was in
		// the one column that lags. The wording is the numeric one even so:
		// the shell that has the type says `numeric argument expected` for a
		// missing argument as well as for an unreadable one.
		if r.ask(r.sem().GetoptsCountsTheWordOnTheNextCall,
			"OPTIND staying on a spent word until the next `getopts` call") {
			r.optChar = len(word)
			return r.getoptsBad(name, sign, c, silent, getoptsBadNumericArgument,
				func() bool { return r.setOptind(ind) })
		}
		r.optChar = 1
		return r.getoptsBad(name, sign, c, silent, getoptsBadNumericArgument,
			func() bool { return r.setOptind(ind + 1) })
	}
	r.optChar = 1
	arg := words[ind]
	if n, ok := getoptsNumeral(arg); !ok || n < len(arg) {
		// The word has to be a numeral all the way through, which is what
		// separates it from the attached spelling above: `-n 5x` is refused
		// where `-n5x` reads 5. Both words are consumed either way.
		return r.getoptsBad(name, sign, c, silent, getoptsBadNumericArgument,
			func() bool { return r.setOptind(ind + 2) })
	}
	if !r.getoptsWrite("OPTARG", arg) && r.getoptsRefusalEndsTheBuiltin() {
		return getoptsRefusedStatus
	}
	if !r.setOptind(ind+2) && r.getoptsRefusalEndsTheBuiltin() {
		return getoptsRefusedStatus
	}
	return r.getoptsSetName(name, letter, 0)
}

// getoptsNumeral measures the numeral at the front of s and reports how many
// bytes it took, including any blanks in front of it.
//
// The shape is strtol's rather than the arithmetic evaluator's, and the
// difference is what the measurements show: `1+1` is refused where `16#ff`,
// `0x1f`, `+4`, `08` and `007` are all taken, so this reads a *numeral* and
// never an expression. Measured 2026-09-16 on ksh93u+ 2012-08-01 with
// `getopts 'n#' o -n «word»`, taken where the whole word is consumed:
//
//	taken     5  -3  +4  0  00  007  08  0x10  0X1F  16#FF  36#z  64#_  " 5"  ""  " "
//	refused   abc  -x  --  -  +  1+1  3.5  .5  1e3  5x  "5 "  0b101  0x  0xg  1_0
//	          2#   2#12  1#0  0#5  99#5  65#1
//
// So: blanks, then a sign, then either `0x` and hex digits, or decimal digits
// and — where those name a base of 2 through 64 — a `#` and that base's
// digits. A word of blanks alone, and the empty word, are taken: nothing is
// left over, which is the whole of the test. baseDigitValue is the alphabet,
// shared with the arithmetic reader so the two cannot come to disagree about
// what a base-64 digit is.
func getoptsNumeral(s string) (int, bool) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\v' || s[i] == '\f' || s[i] == '\r') {
		i++
	}
	if i == len(s) {
		return i, true
	}
	if s[i] == '+' || s[i] == '-' {
		i++
	}
	if i+2 < len(s) && s[i] == '0' && (s[i+1] == 'x' || s[i+1] == 'X') && isHexDigitByte(s[i+2]) {
		i += 2
		for i < len(s) && isHexDigitByte(s[i]) {
			i++
		}
		return i, true
	}
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == start {
		return 0, false
	}
	if i < len(s) && s[i] == '#' {
		if base, err := strconv.Atoi(s[start:i]); err == nil && base >= 2 && base <= 64 {
			j := i + 1
			for j < len(s) {
				v, known := baseDigitValue(s[j], base)
				if !known || v >= base {
					break
				}
				j++
			}
			if j > i+1 {
				i = j
			}
		}
	}
	return i, true
}

func isHexDigitByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// getoptsRefusedStatus is what `getopts` reports when a freeze refused one of
// the three names it fills in.
//
// Measured 2026-09-16 over a script file, `env -i PATH=/usr/bin:/bin`, with
// `set -- -a val` and the name frozen: bash 5.3.20, ksh93u+ 2012-08-01, dash
// 0.5.12 and BusyBox ash 1.37.0 all end the builtin at 2. bash 3.2.57 answers
// 1 and no dialect here claims that build, so it is recorded rather than
// modeled. It is not Diagnostics.GetoptsUsageStatus: that is what `getopts`
// with too few operands reports, which zsh answers 1 to and which this shell
// never reaches over a freeze because zsh writes through one.
const getoptsRefusedStatus = 2

// getoptsWrite sets one of the three names `getopts` fills in — OPTARG,
// OPTIND and the name operand — and reports whether the write happened.
//
// # Why this is not r.setVar
//
// A builtin filling in its own output parameter is neither of the two shapes
// interp/runner.go's refusal knows about. A **bare assignment** that a freeze
// refuses gives up the rest of what the shell was running, and a
// **declaration** does not; `getopts` reached the first of those through
// setVar, so `readonly OPTARG; getopts a: o; echo reached` printed the
// refusal and never the `echo` — where every panel shell runs it (#3147).
//
// Every column carries on, so *that* much is not an axis: measured
// 2026-09-16, bash 5.3.20, bash 3.2.57, bash called as `sh`, ksh93u+
// 2012-08-01, dash 0.5.12 and BusyBox ash 1.37.0 all reach the next command
// on the same line, and so does zsh 5.9.2 — which reaches it by never
// refusing at all. What the refusal *costs* splits three ways, and the two
// axes below are that split.
func (r *Runner) getoptsWrite(name, value string) bool {
	if !r.getoptsMayWrite(name) {
		return false
	}
	if r.readonly[name] {
		// Written *through* the freeze rather than around it: the freeze is
		// lifted for the one store, so that everything else a scalar store
		// does — a reference aimed at another name, a `.set` discipline, a
		// compound the scalar replaces — still happens.
		delete(r.readonly, name)
		defer func() { r.readonly[name] = true }()
	}
	if r.getoptsRefusedName != "" && name == r.getoptsRefusedName {
		// This is the operand the refusal is about, and this dialect ran the
		// scan anyway: the cursor moves and nothing is stored. Reported as a
		// write that happened, because the caller's question is whether the
		// scan may carry on, and the answer is yes.
		//
		// Matched by name rather than by a flag, because OPTIND and OPTARG
		// come through here too — see Runner.getoptsRefusedName.
		return true
	}
	// setOperandValue rather than setVar, because this builtin's name
	// operand is one of the three a dialect may spell as a *position* —
	// see storeThroughPositional, which `read` and `printf -v` reach
	// through storeThroughOperand. Writing it here with setVar is how one
	// of the three would come to answer `getopts x 1` differently from
	// `read 1`, which is the shape that has cost this tree a bug more than
	// once.
	r.setOperandValue(name, value)
	return true
}

// getoptsMayWrite reports whether a freeze lets `getopts` touch one of the
// three names it fills in, having written the refusal where it does not.
//
// Taking the value away is a write as far as a freeze is concerned, which is
// why this is a question of its own rather than the top of getoptsWrite:
// measured 2026-09-16, `set -- -b; OPTARG=PRE; readonly OPTARG; getopts a:b
// o` writes `OPTARG: readonly variable` in bash 5.3.20 and leaves `PRE`
// standing, where the same line over an unfrozen name leaves OPTARG unset.
func (r *Runner) getoptsMayWrite(name string) bool {
	if !r.readonly[name] {
		return true
	}
	if (name == "OPTARG" || name == "OPTIND") &&
		r.ask(r.sem().GetoptsOwnParametersIgnoreAFreeze,
			"`getopts` writing OPTARG and OPTIND through a freeze") {
		return true
	}
	// assignedByBuiltin, which is neither of the two shapes the refusal was
	// written for: the builtin names itself where the dialect names one —
	// `getopts: OPTARG: is read only` in dash and BusyBox ash — and the rest
	// of the line is not given up, which is what #3147 was.
	r.reportReadonlyRefusal(name, assignedByBuiltin, r.getoptsRefusalIsFatal())
	return false
}

// getoptsRefusalIsFatal is what a freeze this builtin could not write through
// costs the script.
//
// Two questions rather than one, because a dialect splits the answer by which
// path inside `getopts` reached the freeze: the name operand written on the
// run that reports "no more options" ends the script there, while every other
// refused write the same builtin makes over the same name only reports. See
// Semantics.GetoptsFrozenNameAtTheEndOfTheOptionsIsFatal for the panel.
//
// The narrower question is put first and only where the narrower path is the
// one in flight, so a dialect answering the broad one Yes keeps that answer
// everywhere and no shell is asked the narrow one off that path.
func (r *Runner) getoptsRefusalIsFatal() bool {
	if r.optRanOut && r.ask(r.sem().GetoptsFrozenNameAtTheEndOfTheOptionsIsFatal,
		"a freeze on the `getopts` name ending the script where the scan had run out") {
		return true
	}
	return r.ask(r.sem().ReadonlyRefusalInABuiltinIsFatal,
		"a builtin's refused write to its own output parameter ending the script")
}

// getoptsSetName writes the name operand, returning ok where the write
// happened and the refused status where a freeze stopped it.
//
// The name is the one write whose refusal ends the builtin in every column
// that reaches it, so it does not go through the axis below: bash 5.3, ksh93,
// dash and BusyBox ash all answer 2 and leave the name alone. See
// getoptsRefusedStatus.
func (r *Runner) getoptsSetName(name, value string, ok int) int {
	if base, _, subscripted := r.subscriptOperand(name); subscripted && isPlainName(base) {
		// The operand named an *element*, which the two columns that take it
		// here really do fill: measured 2026-09-18, `getopts x 'o[1]' -x`
		// leaves the letter in `${o[1]}` under ksh93u+ and in `$o` under zsh
		// 5.9.2, where this shell made a parameter literally named `o[1]`
		// that no expansion in either dialect can read back. The store is
		// the one `read 'a[1]'` goes through, so the subscript is evaluated
		// and refused in one place rather than two.
		//
		// Only ever reached where isGetoptsName let the brackets past, which
		// is Semantics.GetoptsOperandTakesASubscript.
		if st, refused := r.storeThroughOperand(name, value); refused {
			return st
		}
		return ok
	}
	if r.getoptsWrite(name, value) {
		return ok
	}
	return getoptsRefusedStatus
}

// getoptsRefusalEndsTheBuiltin is the axis a refused **OPTARG or OPTIND**
// write asks, which the name does not: dash and BusyBox ash stop there and
// report, and bash writes the rest and reports success anyway.
//
// Asked only after a refusal, which is why a `getopts` over unfrozen names —
// every ordinary use of it — reaches no axis at all.
func (r *Runner) getoptsRefusalEndsTheBuiltin() bool {
	return r.ask(r.sem().GetoptsRefusedWriteEndsTheBuiltin,
		"a freeze on OPTARG or OPTIND ending `getopts` rather than only being reported")
}

// advance moves past the character just read, staying inside the word while
// there is more of the cluster to come.
//
// getoptsErrorEndsTheWord reports whether a refused letter gives up the rest
// of the word it was in and counts the word on the spot.
//
// Asked at the two refusals a script can reach without the numeric type — an
// unknown letter and a missing argument — and nowhere else, because it is
// only about what a refusal does. A letter that was accepted is counted by
// the two axes beside it, whatever this one says. See
// Semantics.GetoptsErrorEndsTheWord.
func (r *Runner) getoptsErrorEndsTheWord() bool {
	return r.ask(r.sem().GetoptsErrorEndsTheWord,
		"a refused `getopts` letter giving up the rest of its word")
}

// The letter that got here consumed no argument, which is what makes this the
// one place both counting axes are asked: an option that took an argument
// leaves OPTIND at the first word neither it nor its argument occupies in
// every column.
func (r *Runner) advance(word string, ind int) bool {
	return r.advanceBy(word, ind, 1)
}

// advanceBy is the same move over n characters rather than one, which is what
// a letter whose argument is *inside* the word consumes: `-n5x` under the
// numeric type reads `5` and leaves the scan on `x`, so the word is not spent
// and the two counting axes are asked exactly as a cluster asks them.
func (r *Runner) advanceBy(word string, ind, n int) bool {
	if r.optChar+n >= len(word) {
		if r.ask(r.sem().GetoptsCountsTheWordOnTheNextCall,
			"OPTIND staying on a spent word until the next `getopts` call") {
			// zsh has not counted the word at all. Leaving optChar past the
			// end is what says it is spent, and the next call walks on to
			// the following word by the check it already makes at the top —
			// which is this shell's own model, not a simulation of it.
			r.optChar = len(word)
			return r.setOptind(ind)
		}
		r.optChar = 1
		return r.setOptind(ind + 1)
	}
	r.optChar += n
	if r.ask(r.sem().GetoptsCountsTheWordAtItsFirstLetter,
		"OPTIND counting a clustered word at its first letter") {
		// dash and BusyBox ash have already counted past the word; the
		// place inside it is optChar and a script cannot see it. optIndex
		// takes the word back off so the scan carries on where it was.
		return r.setOptind(ind + 1)
	}
	return r.setOptind(ind)
}

// getoptsEnd reports that there are no more options.
func (r *Runner) getoptsEnd(name string, ind int) int {
	r.optChar = 1
	// Ahead of OPTIND, which is the order a refusal makes visible: measured
	// 2026-09-16, a frozen OPTARG leaves BusyBox ash with OPTIND still 1 over
	// `set -- -- x`, where every other column has already counted past the
	// `--`. So the clearing happens before the word count moves.
	if !r.clearOptargAtEndOfOptions() && r.getoptsRefusalEndsTheBuiltin() {
		return getoptsRefusedStatus
	}
	if !r.setOptind(ind) && r.getoptsRefusalEndsTheBuiltin() {
		return getoptsRefusedStatus
	}
	if !r.ask(r.sem().GetoptsEndOfOptionsNamesIt,
		"`getopts` writing `?` into the name when it runs out of options") {
		// One column leaves the name holding whatever it held, which reads
		// as the last letter again to anyone whose probe scanned first.
		return 1
	}
	// The one write whose freeze a dialect answers apart from every other
	// write this builtin makes, and nothing in the name or the value it is
	// given says so — see
	// Semantics.GetoptsFrozenNameAtTheEndOfTheOptionsIsFatal, which
	// getoptsRefusalIsFatal reads through this.
	r.optRanOut = true
	wrote := r.getoptsWrite(name, "?")
	r.optRanOut = false
	if wrote {
		return 1
	}
	// The one column whose refusal here ends the script carries the builtin's
	// **own** refused status out of it rather than the shell's fatal status:
	// measured 2026-09-17, ksh93u+ exits 2 from this line where a readonly
	// reassignment, a division by zero, a `shift` past the end and `${x?}`
	// all exit 1 — which is FatalErrorStatusIsOne, and it is Yes there. Read
	// rather than asked, for the reason getoptsBad reads the fatality of a
	// freeze directly: the question was already put at the write, and asking
	// it twice would record two answers for one disagreement.
	if r.sem().GetoptsFrozenNameAtTheEndOfTheOptionsIsFatal == Yes {
		return getoptsRefusedStatus
	}
	// The one refused name that does not carry the status with it. There is
	// no letter the builtin failed to report here, so the 1 that says "no
	// more options" stands: measured 2026-09-16, `set -- x; N=keep; readonly
	// N; getopts ab N` is `N: readonly variable` at **1** in bash 5.3.20 and
	// 3.2.57, with `N` still `keep` — where the same freeze over an option
	// the scan *found* ends the builtin at 2. dash 0.5.12 and BusyBox ash
	// 1.37.0 answer 2 here as they do everywhere else, which is the same
	// question they answer for OPTARG, so it is asked through the same axis.
	//
	// ksh93u+ is a third answer and is the axis above: this one refusal ends
	// the script there, while the same freeze over a letter the scan found
	// only reports and returns 2. getoptsRefusalIsFatal is where the two are
	// told apart, and the flag is set around the write because nothing in the
	// name or the value distinguishes the paths (#3183).
	if r.getoptsRefusalEndsTheBuiltin() {
		return getoptsRefusedStatus
	}
	return 1
}

// getoptsBad is an option the string does not have, or one whose argument is
// missing.
//
// Silent mode is the interesting half: the letter goes into OPTARG and the
// name becomes `?` for an unknown option and `:` for a missing argument, so a
// script can tell the two apart without reading a message.
// advanceOptind moves the word count on, and is the caller's rather than
// this function's so that it can happen **after** whatever is done to OPTARG:
// measured 2026-09-16, a frozen OPTARG over `set -- -z` leaves dash 0.5.12 and
// BusyBox ash 1.37.0 refusing at 2 with OPTIND still 1, so neither had counted
// past the word when the refusal ended the run.
func (r *Runner) getoptsBad(name string, sign, c byte, silent bool, why getoptsComplaint, advanceOptind func() bool) int {
	letter := getoptsLetter(sign, c)
	if silent {
		if !r.getoptsWrite("OPTARG", letter) && r.getoptsRefusalEndsTheBuiltin() {
			return getoptsRefusedStatus
		}
		if !advanceOptind() && r.getoptsRefusalEndsTheBuiltin() {
			return getoptsRefusedStatus
		}
		if why != getoptsUnknownOption {
			return r.getoptsSetName(name, ":", 0)
		}
		return r.getoptsSetName(name, "?", 0)
	}
	// The complaint about the option comes first, and a refusal the *name*
	// earns comes after it: measured 2026-09-16, `set -- -z; readonly o;
	// getopts a: o` writes the illegal-option line and then the readonly one
	// in bash 5.3, bash 3.2, ksh93, dash and BusyBox ash alike. So the name
	// is written below rather than here, and its refusal decides only the
	// status.
	//
	// The one column that reverses it is the one whose refusal is *fatal*:
	// zsh writes `read-only variable: o` and the script is over, so the
	// bad-option line it would have written never is. One order and a guard
	// rather than an axis, because the two are the same rule seen twice — a
	// refusal that ends the script is written before the complaint it would
	// otherwise have followed, and there is nothing after it to follow.
	if r.readonly[name] && r.sem().ReadonlyRefusalInABuiltinIsFatal == Yes {
		return r.getoptsSetName(name, "?", 0)
	}
	if !r.getoptsComplaintSilenced() {
		r.getoptsBadOptionComplaint(sign, c, why)
	}
	// And what becomes of OPTARG after it: measured 2026-09-16, dash 0.5.12
	// and BusyBox ash 1.37.0 write `Illegal option -z` and *then* `getopts:
	// OPTARG: is read only` over a frozen OPTARG, in that order.
	if !r.clearOptarg() && r.getoptsRefusalEndsTheBuiltin() {
		return getoptsRefusedStatus
	}
	if !advanceOptind() && r.getoptsRefusalEndsTheBuiltin() {
		return getoptsRefusedStatus
	}
	return r.getoptsSetName(name, "?", 0)
}

// getoptsComplaintSilenced reports whether a parameter the script set turns
// this builtin's complaint off.
//
// The parameter is read before the axis rather than after it, which is what
// keeps the question off the path every other shell takes: a script that has
// not set OPTERR, or has set it to something that is not zero, is not asking
// the question at all. See Semantics.GetoptsOptErrSilencesTheComplaint.
//
// The empty string is not zero here. `OPTERR=` writes the sentence exactly as
// an unset OPTERR does, so a name with nothing in it is read as no answer
// rather than as a zero — which is the one spelling that separates this from
// an ordinary number read.
func (r *Runner) getoptsComplaintSilenced() bool {
	v, ok := r.getVar("OPTERR")
	if !ok || v == "" || !leadingNumberIsZero(v) {
		return false
	}
	return r.ask(r.sem().GetoptsOptErrSilencesTheComplaint,
		"OPTERR reading zero silencing the `getopts` complaint")
}

// leadingNumberIsZero reads a number off the front of s the way a shell reads
// one out of a string that was never meant to hold one, and reports whether it
// came to zero.
//
// Blanks, then an optional sign, then decimal digits, stopping at the first
// byte that is not one — so `x`, `0x0`, `0abc`, `-`, `+` and a lone blank are
// all zero, and `08` is eight rather than an octal eight-that-is-not. All six
// of space, tab, newline, vertical tab, form feed and carriage return are
// skipped, each measured in front of a `0` and of a `1`. Whether
// a digit that is not `0` was seen is the whole of the test, which is also why
// no integer is built: `9999999999999999999999` is not zero and overflowing an
// int to decide that would be reading the string twice as carefully as the
// answer needs.
func leadingNumberIsZero(s string) bool {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' ||
		s[i] == '\v' || s[i] == '\f' || s[i] == '\r') {
		i++
	}
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		if s[i] != '0' {
			return false
		}
	}
	return true
}

// getoptsBadOptionComplaint writes the line a bad option or a missing
// argument earns, in a function of its own so that the name `getopts` fills
// in can be written after it — see getoptsBad.
func (r *Runner) getoptsBadOptionComplaint(sign, c byte, why getoptsComplaint) {
	// Not the builtin's complaint as far as the one dialect that names a
	// builtin in the location is concerned: `getopts` reports like `test`
	// rather than like `shift`, with no name between the shell and the line.
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()

	d := r.diag()
	wording, fallback := d.GetoptsBadOption, "illegal option -- %[1]s"
	switch why {
	case getoptsMissingArgument:
		wording, fallback = d.GetoptsMissingArgument, "option requires an argument -- %[1]s"
	case getoptsBadNumericArgument:
		wording, fallback = d.GetoptsNumericArgument, "option requires a numeric argument -- %[1]s"
	}
	// Two verbs: the letter, and the **sign** of the word it came out of.
	// Three of the five dialects spell the `-` into the wording itself, which
	// is honest there because a `+` word is an operand in all three — see
	// Semantics.GetoptsTakesAPlusPrefixedOption. The one dialect that reads
	// both signs takes the second verb instead, and writes `bad option: +z`
	// where it writes `bad option: -z`.
	msg := Wording(wording, fallback, string(c), string(sign))
	switch {
	case d.GetoptsUnprefixed:
		// Neither a name nor a location: one dialect prints the complaint on
		// its own.
		r.errf("%s\n", msg)
	case d.GetoptsNamesNoLine:
		// The shell's name and no line, where this dialect gives a line to
		// everything else it says.
		r.errf("%s: %s\n", r.name(), msg)
	default:
		r.diagf("%s\n", msg)
	}
}

// optIndex is the word the scan is at, and it reads OPTIND unless a call has
// a position of its own beside it.
//
// The two are the same number everywhere except across a function call in the
// dialect that gives the call its own cursor and leaves the *parameter* the
// shell's — see GetoptsFunctionPositionIsTheCallsOwn, which is the only thing
// that ever sets the override. Where they differ, the scan's own position is
// the one the shell scans from and OPTIND is what a script reads: that is the
// whole of the difference a script can see, and it is why the two halves are
// separate rather than one number.
//
// An assignment the *script* made is the exception and outranks both: writing
// OPTIND is how a scan is restarted, so the number written is the position.
// The override follows it rather than being dropped, because the call still
// has a cursor of its own afterwards.
func (r *Runner) optIndex() int {
	return r.optCursorWord() - r.optWordAlreadyCounted()
}

// optCursorWord is the same position in OPTIND's own units — the number the
// parameter holds, which in the dialect that counts a clustered word at its
// first letter is one ahead of the word the scan is at.
//
// Separate from optIndex because the two units are not interchangeable and
// mixing them is a bug that only one pair of dialects can see. The override
// and the parameter are both in *these* units — setOptind writes the one and
// the other with the same number — so a cursor saved across a call has to be
// saved here and not from optIndex, which has already taken the word off.
// Saved from optIndex and put back into the override, a call made part-way
// through `-ab` in dash came back to word *zero*: the one ahead was
// subtracted going in and again coming out.
func (r *Runner) optCursorWord() int {
	if r.optWord > 0 && !r.optindAssigned {
		return r.optWord
	}
	n := r.optindValue()
	if r.optWord > 0 {
		r.optWord = n
	}
	return n
}

// optWordAlreadyCounted is the one word OPTIND is ahead by, in the dialect
// that counts a clustered word at its first letter.
//
// Read rather than asked, and the difference matters: an unanswered axis has
// left every write to OPTIND at the other rule, so there is nothing ahead to
// take back. The question is put once, in advance, where the number is
// chosen. A script's own assignment outranks it for the reason it outranks
// the override above — writing OPTIND is how a scan is restarted, so the
// number written is the position and nothing is ahead of it.
func (r *Runner) optWordAlreadyCounted() int {
	if r.optChar > 1 && !r.optindAssigned &&
		r.sem().GetoptsCountsTheWordAtItsFirstLetter == Yes {
		return 1
	}
	return 0
}

// optindValue is the number OPTIND holds, which is the shell's and which a
// script may set.
func (r *Runner) optindValue() int {
	v, ok := r.getVar("OPTIND")
	if !ok {
		return 1
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func (r *Runner) setOptind(n int) bool {
	wrote := r.getoptsWrite("OPTIND", strconv.Itoa(n))
	if r.optWord > 0 {
		// The scan's own position moves with the parameter: they part only
		// at a call boundary, and this is not one.
		r.optWord = n
	}
	// This builtin's own write is not the script's.
	r.optindAssigned = false
	return wrote
}

// clearOptarg takes OPTARG away after a *bad* option, or empties it where the
// dialect empties it.
//
// Four of the five leave it unset and zsh sets it to the empty string, which
// a script testing `${OPTARG-}` can tell apart.
func (r *Runner) clearOptarg() bool {
	return r.clearOptargPossiblyForReal(r.ask(r.sem().GetoptsClearsOptarg,
		"`getopts` emptying OPTARG rather than unsetting it after a bad option"))
}

// clearOptargPossiblyForReal is the clearing the error paths and the end of
// the options share, and the one place the difference between *taking the
// name away* and *writing over it* is decided.
//
// A `delete` from the value map is not an unset: it leaves the name's
// attributes where they were, so a `readonly OPTARG` neither stopped the
// clearing nor was taken away by it. Both halves are measured, and they are
// one answer rather than two — bash's clearing removes the name, and removing
// a name removes what was recorded about it.
//
// See Semantics.GetoptsClearingOptargIsARealUnset for the panel, and for the
// one clearing this is *not* asked of: an option that takes no argument goes
// through the ordinary refusal even in bash, which reports it and leaves the
// value standing.
func (r *Runner) clearOptargPossiblyForReal(empty bool) bool {
	if !empty && r.readonly["OPTARG"] && r.ask(r.sem().GetoptsClearingOptargIsARealUnset,
		"`getopts` clearing OPTARG by removing the name rather than by writing over it") {
		delete(r.readonly, "OPTARG")
	}
	return r.emptyOrUnsetOptarg(empty)
}

// clearOptargAtEndOfOptions takes OPTARG away when the scan runs out of
// options, in the dialects that do.
//
// The run that reports "no more options" is also the one that clears OPTARG
// in five of the seven, and this shell did nothing there — so the **last
// option's argument was still standing** when the loop exited, and a script
// reading `$OPTARG` after its `while getopts` got the previous option's value
// instead of nothing, at status 0 (#3146).
//
// Not the empty-or-unset question the two clearings above ask. Every column
// that clears here *unsets*, so `${OPTARG-…}` finds nothing rather than an
// empty string in all three, and there is no second answer for an axis to
// hold. Measured 2026-09-16 over a script file under `env -i
// PATH=/usr/bin:/bin`, `OPTARG=PRESET; OPTIND=1; set -- x; getopts a: o`,
// and the same over `-- x`, over no words at all, and over the call after an
// option that was read:
//
//	bash 5.3.20   OPTARG unset
//	bash as `sh`  OPTARG unset
//	bash 3.2.57   OPTARG unset
//	ksh93u+       OPTARG unset
//	BusyBox ash   OPTARG unset
//	dash 0.5.12   PRESET stands
//	zsh 5.9.2     PRESET stands
//
// What the clearing *is* then splits three ways over a frozen OPTARG, and
// two of the three fall out of answers this builtin already has:
//
//   - ksh93 writes past the freeze without a word, which is
//     GetoptsOwnParametersIgnoreAFreeze.
//   - BusyBox ash is refused by it, reports, and ends the builtin at 2 with
//     OPTARG still `PRESET` — the same two answers it gives for every other
//     write here.
//   - bash is the third: silent, OPTARG gone, **and the freeze gone with
//     it**, so `OPTARG=written` on the line after succeeds where it was
//     refused before. That is GetoptsClearingOptargIsARealUnset, which bash
//     alone answers yes and which the error paths ask too.
func (r *Runner) clearOptargAtEndOfOptions() bool {
	if !r.ask(r.sem().GetoptsUnsetsOptargAtEndOfOptions,
		"`getopts` unsetting OPTARG when it runs out of options") {
		return true
	}
	return r.clearOptargPossiblyForReal(false)
}

// clearOptargForAnArgumentlessOption is the same for an option the string has
// and that takes no argument, which is a different axis because the columns
// line up differently — see the axis for the measurement that separates them.
func (r *Runner) clearOptargForAnArgumentlessOption() bool {
	return r.emptyOrUnsetOptarg(r.ask(r.sem().GetoptsEmptiesOptargForAnArgumentlessOption,
		"`getopts` emptying OPTARG rather than unsetting it after an option that takes none"))
}

// emptyOrUnsetOptarg is what both of them do with their answer, in one place
// so the two cannot come to mean different things by the same word.
func (r *Runner) emptyOrUnsetOptarg(empty bool) bool {
	if empty {
		return r.getoptsWrite("OPTARG", "")
	}
	if !r.getoptsMayWrite("OPTARG") {
		return false
	}
	delete(r.Vars, "OPTARG")
	if r.removed == nil {
		r.removed = map[string]bool{}
	}
	r.removed["OPTARG"] = true
	return true
}

// localizeGetoptsCursor arranges for this function call to have a `getopts`
// cursor of its own, in the dialect where OPTIND is local to a function.
//
// Both halves of the position are the cursor: OPTIND, which counts words, and
// optChar, which is how far into a clustered word the scan has read. Saving
// only OPTIND left a function that scans its own `-cd` with the caller's
// half-spent `-ab` still in hand, so it skipped `c` — the measurement that
// says the intra-word position travels with the value.
//
// # Where the axis is asked, and why not here
//
// Every function call comes through here, and most of them have nothing to do
// with getopts. Asking the axis on the way in would put the unanswered-axis
// diagnostic in front of every function call a bare `Semantics` ever makes —
// an axis is asked where the shells can be *told apart*, and at the top of a
// call they usually cannot be:
//
//   - A cursor already at the start of the first word is what zsh would hand
//     the call anyway, so the entry value is 1 either way and there is
//     nothing to reset. The question is put off to the return, and asked
//     there only if the body moved the cursor — a call that never ran
//     `getopts` leaves nothing for the two answers to disagree about.
//   - A cursor part-way along is the case where the entry value differs, so
//     that is where the question is asked on the way in.
//
// So the ask happens at most once per call and only where an answer changes
// what a script can see. `askedIn` is what keeps the return from asking a
// second time about the same call.
//
// # The two silences, both measured
//
// A call entered with OPTIND *unset* is not handed a cursor at 1: zsh reads
// the name as unset inside the function too, because `unset` of this
// parameter takes it away rather than emptying it. So there is nothing to
// localize, and nothing is asked.
//
// A call that unsets OPTIND itself does not get the caller's back either:
// `OPTIND=5; h() { unset OPTIND; }; h` leaves the name gone in every panel
// shell that can unset it at all (dash refuses the unset outright). That is
// the same fact from the other side — the parameter was removed, not
// shadowed — and it is why the restore asks whether the name is still there
// instead of putting the value back unconditionally.
func (r *Runner) localizeGetoptsCursor(sc *scope) {
	if _, set := r.getVar("OPTIND"); !set {
		return
	}
	held, inVars := r.Vars["OPTIND"]
	wasRemoved := r.removed["OPTIND"]
	char, assigned := r.optChar, r.optindAssigned
	// The caller's place in words, which is what a call with a cursor of its
	// own hands back — and which is *not* OPTIND once a call has already made
	// the two part company. Read before anything here moves it, and in the
	// units the override is put back in: see optCursorWord.
	cursor := r.optCursorWord()

	// Whether the caller had read anything yet. A cursor at the first
	// character of the first word is indistinguishable from the fresh one a
	// call would be handed, so only a used cursor makes the entry differ.
	fresh := cursor == 1 && char <= 1 && held == "1" && inVars
	askedIn := false
	position := GetoptsFunctionPositionIsShared
	if !fresh {
		askedIn = true
		position = r.getoptsFunctionPosition(sc)
		switch position {
		case GetoptsFunctionPositionIsLocal:
			// Quietly, because this is the shell handing the call a cursor
			// rather than the script assigning one: setVar would record an
			// assignment the script never made, and the axis that reads that
			// record drops the position inside a word on the strength of it.
			r.setVarQuietly("OPTIND", "1")
			r.optChar, r.optindAssigned = 1, false
		case GetoptsFunctionPositionIsTheCallsOwn:
			// The scan starts over and the parameter is left exactly as the
			// caller had it: a script reading `$OPTIND` on the way in sees
			// the caller's number, which is what separates this answer from
			// the one above.
			r.optWord, r.optChar, r.optindAssigned = 1, 1, false
		default:
			return
		}
		// Both answers that reach here reset the cursor on the way in, so
		// the caller's place inside a word is now held by this call's own
		// restore rather than by any declaration the body goes on to make —
		// which is the one thing a declaration's restore needs to know about
		// this one. Set once for both rather than in each arm, because it is
		// a fact about having reset the cursor and not about which answer
		// did: dash and BusyBox ash are the only columns that reach it with
		// a `no` behind them, and zsh — the other arm — hands the place back
		// either way, so nothing on this panel could tell a per-arm version
		// of this apart.
		sc.optindCallCursor = true
	}

	sc.onReturn = append(sc.onReturn, func() {
		now, still := r.getVar("OPTIND")
		if !askedIn {
			// Nothing was reset on the way in, so the call is only
			// distinguishable if its body moved the cursor.
			//
			// Against optCursorWord and not optIndex, because that is the
			// unit `cursor` was saved in. The two coincide here — the clause
			// beside this one pins optChar at the caller's, which the entry
			// required to be at the start of a word, and it is exactly a
			// place *inside* a word that the units differ by — so this is
			// the same comparison written the same way rather than a second
			// reading of the same number, which is how the units came to be
			// mixed in the first place.
			if still && now == held && r.optChar == char &&
				r.optindAssigned == assigned && r.optCursorWord() == cursor {
				return
			}
			position = r.getoptsFunctionPosition(sc)
		}
		if position == GetoptsFunctionPositionIsShared {
			return
		}
		// The scan position comes back whatever became of the parameter: it
		// is the caller's place in the caller's words, and a name the call
		// took away says nothing about that.
		//
		// Unless a declaration of OPTIND inside the call took the intra-word
		// half away and this dialect does not hand it back, which is
		// GetoptsLocalOptindRestoresTheCursor and is asked where the
		// declaration is. Put back unconditionally here, that answer was
		// overwritten by this one and the axis that was never measured won:
		// dash and BusyBox ash read the same letter for ever where the real
		// shells restart at the next word (#3293). The *word* half still
		// comes back — only the place inside it is lost — so what the caller
		// resumes at is the start of the word OPTIND names.
		if sc.optindCursorDropped {
			r.optChar = 1
		} else {
			r.optChar, r.optindAssigned = char, assigned
		}
		if position == GetoptsFunctionPositionIsTheCallsOwn {
			// The parameter is the shell's, so only the scan's own position
			// is put back — and it is put back as an override rather than
			// written into OPTIND, because the two have now genuinely parted:
			// `g -a -b` twice leaves OPTIND at 3 and reads the arguments both
			// times.
			r.optWord = cursor
			return
		}
		if !still {
			return
		}
		if inVars {
			r.Vars["OPTIND"] = held
		} else {
			delete(r.Vars, "OPTIND")
		}
		if wasRemoved {
			if r.removed == nil {
				r.removed = map[string]bool{}
			}
			r.removed["OPTIND"] = true
		} else {
			delete(r.removed, "OPTIND")
		}
	})
}

// getoptsFunctionPosition resolves the axis, reporting a dialect that has not
// answered it the way ask does — the call is about to be told apart by it, so
// a guess would be an invention.
//
// The answer keyed on the definition form is resolved here, against the call
// being asked about, into the one of the other answers that call gets — so
// nothing below has a fourth case to carry, and a POSIX-form call in that
// dialect is the shared answer in every respect.
func (r *Runner) getoptsFunctionPosition(sc *scope) GetoptsFunctionPositionPolicy {
	p := r.sem().GetoptsFunctionPosition
	switch p {
	case GetoptsFunctionPositionUnspecified:
		r.diagf("%s\n", r.unanswered(getoptsLocalAxis))
		r.status = 2
		r.unspecified = true
		return GetoptsFunctionPositionIsShared
	case GetoptsFunctionPositionIsLocalToAKeywordFunction:
		if sc.keyword {
			return GetoptsFunctionPositionIsLocal
		}
		return GetoptsFunctionPositionIsShared
	}
	return p
}

// getoptsLocalAxis names the axis in a diagnostic, in one place because two
// call sites ask it about the same call.
const getoptsLocalAxis = "the `getopts` cursor being local to a function"

// shadowGetoptsCursor makes the intra-word half of the scan position part of
// what a declaration of OPTIND shadows.
//
// OPTIND is a number of *words*, and it is only half of where the scan has
// got to: the other half is how far into a clustered word the letters have
// been read, and that half is not a parameter, so a `local OPTIND` that
// shadowed the parameter alone left it standing. The callee then read its own
// `-cd` from the middle, and — the part that matters — the caller came back to
// a cursor pointing at the start of a word it had already part-read, so
// `while getopts` around a call that declares one never runs out of options
// (#2226).
//
// Two things happen here and only one of them is an axis:
//
//   - Entering the call resets the position, and that is the core's answer
//     rather than a dialect's: every panel shell with a local scope at all
//     hands the callee a cursor at the start of a word, whether or not the
//     declaration carried a value. So it is not asked.
//   - Handing the caller its position back is where the panel splits, and it
//     is asked at the return — where a body that never moved the cursor
//     leaves the two answers nothing to disagree about.
//
// Called only for the declaration that *takes* the scope's copy. A second
// declaration of the same name in the same call is writing over a cell that
// is already its own, and re-recording the position there would save a cursor
// the call had already moved.
func (r *Runner) shadowGetoptsCursor(sc *scope) {
	if sc.optindShadowed {
		return
	}
	sc.optindShadowed = true
	sc.savedOptChar, sc.savedOptindAssigned = r.optChar, r.optindAssigned
	r.optChar, r.optindAssigned = 1, false
}

// restoreGetoptsCursor puts the caller's scan position back when a call that
// declared a local OPTIND unwinds. See shadowGetoptsCursor for why the
// question is asked here and not on the way in.
func (r *Runner) restoreGetoptsCursor(sc *scope) {
	if !sc.optindShadowed {
		return
	}
	if !sc.optindCallCursor &&
		r.optChar == sc.savedOptChar && r.optindAssigned == sc.savedOptindAssigned {
		// The body left the position where the declaration put it, so both
		// answers produce the same cursor and there is nothing to ask about.
		//
		// Only where the declaration saved the *caller's* position. A call
		// handed a cursor of its own had it reset on the way in, so what the
		// declaration saved is that reset and a body that left it alone has
		// still cost the caller its place inside a word — the answers differ
		// and the question is a real one. See scope.optindCallCursor.
		return
	}
	if !r.ask(r.sem().GetoptsLocalOptindRestoresTheCursor,
		"a local `OPTIND` handing back the caller's position inside a word") {
		// Recorded rather than merely not done, because in a dialect that
		// gives every call a cursor of its own the caller's half is held by
		// the call's restore and would be put back over the top of this.
		sc.optindCursorDropped = true
		return
	}
	r.optChar, r.optindAssigned = sc.savedOptChar, sc.savedOptindAssigned
}
