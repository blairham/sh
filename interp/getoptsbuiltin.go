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
		r.diagf("getopts: usage: getopts optstring name [arg]\n")
		return 2
	}
	optstring, name := args[0], args[1]
	// The operands to scan are the ones given, or the shell's own parameters
	// when none are — which is what every use of it in a script relies on.
	words := args[2:]
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
	if word == "" || word[0] != '-' || word == "-" {
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

// getoptsAt reads the option at the current position.
func (r *Runner) getoptsAt(name, spec string, silent bool, words []string, ind int) int {
	i := ind - 1
	if i >= len(words) {
		return r.getoptsEnd(name, ind)
	}
	word := words[i]
	if word == "" || word[0] != '-' || word == "-" || word == "--" {
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
	at := strings.IndexByte(spec, c)
	switch {
	case at < 0 || c == ':':
		r.advance(word, ind)
		return r.getoptsBad(name, string(c), silent, false)
	case at+1 < len(spec) && spec[at+1] == ':':
		// The option takes an argument: the rest of this word if there is
		// any, and the next word otherwise.
		if rest := word[r.optChar+1:]; rest != "" {
			r.optChar = 1
			r.setOptind(ind + 1)
			r.setVar("OPTARG", rest)
			r.setVar(name, string(c))
			return 0
		}
		if ind >= len(words) {
			r.optChar = 1
			r.setOptind(ind + 1)
			return r.getoptsBad(name, string(c), silent, true)
		}
		r.optChar = 1
		r.setOptind(ind + 2)
		r.setVar("OPTARG", words[ind])
		r.setVar(name, string(c))
		return 0
	default:
		r.advance(word, ind)
		r.clearOptarg()
		r.setVar(name, string(c))
		return 0
	}
}

// advance moves past the character just read, staying inside the word while
// there is more of the cluster to come.
func (r *Runner) advance(word string, ind int) {
	if r.optChar+1 >= len(word) {
		r.optChar = 1
		r.setOptind(ind + 1)
		return
	}
	r.optChar++
	r.setOptind(ind)
}

// getoptsEnd reports that there are no more options.
func (r *Runner) getoptsEnd(name string, ind int) int {
	r.optChar = 1
	r.setOptind(ind)
	r.setVar(name, "?")
	return 1
}

// getoptsBad is an option the string does not have, or one whose argument is
// missing.
//
// Silent mode is the interesting half: the letter goes into OPTARG and the
// name becomes `?` for an unknown option and `:` for a missing argument, so a
// script can tell the two apart without reading a message.
func (r *Runner) getoptsBad(name, letter string, silent, missingArg bool) int {
	if silent {
		r.setVar("OPTARG", letter)
		if missingArg {
			r.setVar(name, ":")
		} else {
			r.setVar(name, "?")
		}
		return 0
	}
	r.clearOptarg()
	r.setVar(name, "?")

	// Not the builtin's complaint as far as the one dialect that names a
	// builtin in the location is concerned: `getopts` reports like `test`
	// rather than like `shift`, with no name between the shell and the line.
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()

	d := r.diag()
	wording, fallback := d.GetoptsBadOption, "illegal option -- %[1]s"
	if missingArg {
		wording, fallback = d.GetoptsMissingArgument, "option requires an argument -- %[1]s"
	}
	msg := Wording(wording, fallback, letter)
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
	return 0
}

// optIndex reads OPTIND, which is the shell's and which a script may set.
func (r *Runner) optIndex() int {
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

func (r *Runner) setOptind(n int) {
	r.setVar("OPTIND", strconv.Itoa(n))
	// This builtin's own write is not the script's.
	r.optindAssigned = false
}

// clearOptarg takes OPTARG away, or empties it where the dialect empties it.
//
// Three of the four leave it *unset* and one sets it to the empty string,
// which a script testing `${OPTARG-}` can tell apart. Asked only here, where
// there is no argument to put in it.
func (r *Runner) clearOptarg() {
	if r.ask(r.sem().GetoptsClearsOptarg, "`getopts` emptying OPTARG rather than unsetting it") {
		r.setVar("OPTARG", "")
		return
	}
	delete(r.Vars, "OPTARG")
	if r.removed == nil {
		r.removed = map[string]bool{}
	}
	r.removed["OPTARG"] = true
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

	// Whether the caller had read anything yet. A cursor at the first
	// character of the first word is indistinguishable from the fresh one
	// zsh would install, so only a used cursor makes the entry differ.
	fresh := held == "1" && inVars && char <= 1
	askedIn := false
	if !fresh {
		askedIn = true
		if !r.ask(r.sem().GetoptsPositionIsFunctionLocal, getoptsLocalAxis) {
			return
		}
		// Quietly, because this is the shell handing the call a cursor
		// rather than the script assigning one: setVar would record an
		// assignment the script never made, and the axis that reads that
		// record drops the position inside a word on the strength of it.
		r.setVarQuietly("OPTIND", "1")
		r.optChar, r.optindAssigned = 1, false
	}

	sc.onReturn = append(sc.onReturn, func() {
		now, still := r.getVar("OPTIND")
		if !askedIn {
			// Nothing was reset on the way in, so the call is only
			// distinguishable if its body moved the cursor.
			if still && now == held && r.optChar == char && r.optindAssigned == assigned {
				return
			}
			if !r.ask(r.sem().GetoptsPositionIsFunctionLocal, getoptsLocalAxis) {
				return
			}
		}
		// The scan position comes back whatever became of the parameter: it
		// is the caller's place in the caller's words, and a name the call
		// took away says nothing about that.
		r.optChar, r.optindAssigned = char, assigned
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

// getoptsLocalAxis names the axis in a diagnostic, in one place because two
// call sites ask it about the same call.
const getoptsLocalAxis = "the `getopts` cursor being local to a function"
