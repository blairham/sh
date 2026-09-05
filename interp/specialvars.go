// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"time"
)

// The parameters a shell provides without a script setting them.
//
// Four of them are unanimous across the panel and belong here; the rest are
// not, and belong to whichever dialects have them. Which *variables* a shell
// provides is the same kind of question as which builtins it has — neither
// grammar nor a conflict about meaning — so it is answered through the same
// seam, in each dialect's Apply.
//
// This was found by running a real script rather than by the corpus. A system
// script began `if [ $UID -ne 0 ]`, and with UID unset that is `[ -ne 0 ]` —
// which is not the same test and does not fail in the same way.
//
// Two of the four have to be produced when they are read rather than stored:
// LINENO is wherever execution has reached, and `$-` is whatever `set` has
// done by the time it is read — a stored copy of either would describe the
// line, and the options, the shell started with. That is what Dynamic is for.

// ensureSpecials gives the unanimous parameters their values.
//
// IFS is the one that matters most and the one that looked least urgent:
// splitting already used a default when it was unset, so everything *worked*
// while `echo "$IFS"` printed nothing and a script could neither read it nor
// tell it had been changed.
func (r *Runner) ensureSpecials() {
	if r.Dynamic == nil {
		r.Dynamic = map[string]func(*Runner) string{}
	}
	if _, ok := r.Vars["IFS"]; !ok && !r.removed["IFS"] {
		r.setVarQuietly("IFS", " \t\n")
	}
	if _, ok := r.Dynamic["_"]; !ok {
		// Registered once, not every time: RunPart comes back through here
		// while a process substitution's goroutine may be reading the shared
		// table, and rewriting the same producer was a write all the same.
		r.Dynamic["_"] = func(r *Runner) string {
			// The tracked argument in the dialects that move `$_`;
			// elsewhere, and before anything ran, whatever the environment
			// brought — the invoking shell's own note of what it last ran.
			if r.lastArgSet &&
				r.ask(r.sem().UnderscoreTracksTheLastArgument, "`$_` following the last argument") {
				return r.lastArg
			}
			v, _ := r.inheritedValue("_")
			return v
		}
	}
	if _, ok := r.Vars["PPID"]; !ok && !r.removed["PPID"] {
		r.setVarQuietly("PPID", strconv.Itoa(os.Getppid()))
	}
	if _, ok := r.Dynamic["LINENO"]; !ok {
		r.Dynamic["LINENO"] = func(r *Runner) string {
			// One dialect numbers lines inside a function from the line the
			// function was written on; the rest count from the file, which
			// r.line already is. Asked only inside a function.
			if r.inFunc != "" && r.funcLine > 0 &&
				r.ask(r.sem().LinenoCountsFromTheFunction, "`$LINENO` inside a function counting from it") {
				return strconv.Itoa(r.line - r.funcLine)
			}
			return strconv.Itoa(r.line)
		}
	}
	if _, ok := r.Dynamic["-"]; !ok {
		r.Dynamic["-"] = (*Runner).optionLetters
	}
}

// optionLetters is `$-`: the single-letter options currently in effect.
//
// Produced when it is read for the same reason LINENO is — it changes with
// every `set`, and a stored copy would be the options the shell started with.
// Scripts branch on it: `case $- in *e*)` is the standard errexit check, and
// while this expanded to nothing both arms of that test silently took the
// wrong branch.
//
// The string is the dialect's startup letters followed by a letter for each
// option this runner tracks and has on. Presence is the contract, not order:
// measured, no two shells in the panel order the merged string the same way —
// one even appends in the order the script set them — so a script can ask
// whether a letter is present and nothing more, and ours come out in a fixed
// order of their own. pipefail earns no letter anywhere, which is unanimous
// and so needs no axis; the noglob letter is the one the shells disagree on.
func (r *Runner) optionLetters() string {
	var b strings.Builder
	b.WriteString(r.startupOptionLetters())
	if r.Interactive {
		// Measured unanimous and so no axis: every shell in the panel puts
		// `i` here for an interactive shell and none of them puts it there
		// for anything else — `-i` on every route including `-c` and a
		// script operand, and a terminal with nothing to run. It is not a
		// `set` option and no letter of `set` turns it on, which is why it
		// is read from the fact the front end carried in rather than from a
		// field the option table writes.
		b.WriteByte('i')
	}
	if r.Route == RouteCommandString && r.sem().CommandStringShowsCInDollarDash == Yes {
		// A command string, in the two shells that say so. No majority to
		// follow — see Semantics.CommandStringShowsCInDollarDash.
		b.WriteByte('c')
	}
	if r.showsS() {
		b.WriteByte('s')
	}
	if r.allexport {
		b.WriteByte('a')
	}
	if r.errexit {
		b.WriteByte('e')
	}
	if r.noexec {
		b.WriteByte('n')
	}
	if r.monitor {
		b.WriteByte('m')
	}
	if r.verbose {
		b.WriteByte('v')
	}
	if r.errtrace {
		b.WriteByte('E')
	}
	if r.functrace {
		b.WriteByte('T')
	}
	if r.noglob {
		if r.ask(r.sem().NoglobLetterIsF, "which letter `$-` shows for noglob") {
			b.WriteByte('f')
		} else {
			b.WriteByte('F')
		}
	}
	if r.nounset {
		b.WriteByte('u')
	}
	if r.xtrace {
		b.WriteByte('x')
	}
	if r.noclobber {
		b.WriteByte('C')
	}
	return b.String()
}

// startupOptionLetters is the letters `$-` begins with: the options this shell
// turned on for itself before the script had a chance to.
//
// Two strings and not one plus an addition, because one shell in the panel
// turns an option *off* when it is interactive — its `$-` goes from `hB` to
// `imBE`, dropping the command-tracking letter — so what an interactive shell
// starts with is a different set rather than a longer one. A dialect that has
// nothing separate to say leaves the second empty and gets the first for both,
// which is one of the four.
//
// The letters that describe the invocation rather than an option follow this
// and are not in either string: `i` for being interactive at all, `c` and `s`
// for the route, and `m` for a monitor that is really running.
func (r *Runner) startupOptionLetters() string {
	if r.Interactive {
		if letters := r.sem().InteractiveOptionLetters; letters != "" {
			return letters
		}
	}
	return r.sem().DefaultOptionLetters
}

// showsS reports whether `$-` carries the `s` of the standard-input route.
//
// Two of the three ways it gets there are unanimous and so need no axis,
// measured 2026-09-05 across bash 5.3, dash, ksh93 and zsh:
//
//   - The program arriving on standard input — `sh -s`, `sh` with no
//     operands reading a pipe or a file, and a shell at a prompt, which is
//     the same route with a person on the other end.
//   - `-s` written even where something else supplied the program: all four
//     run the command string for `sh -s -c cmd` and all four still show `s`.
//
// bash 3.2 is the dissent on the first, and it is a shell disagreeing with
// its own later build rather than a panel split: it shows `s` only where
// `-s` was written. Recorded in docs/spec/invocation.md, not modeled.
//
// The third way is the split: ksh93 alone shows `s` under `-c` as well.
func (r *Runner) showsS() bool {
	if r.Route == RouteStandardInput || r.StandardInputOption {
		return true
	}
	return r.Route == RouteCommandString && r.sem().CommandStringShowsSInDollarDash == Yes
}

// setVarQuietly assigns without going through the readonly check or the
// OPTIND bookkeeping, because none of that applies to the shell setting up
// its own parameters before a script has run.
func (r *Runner) setVarQuietly(name, value string) {
	if r.Vars == nil {
		r.Vars = map[string]string{}
	}
	r.Vars[name] = value
}

// SetDynamicArray registers an *array* whose elements are produced when they
// are read.
//
// The same seam as SetDynamic and for a stronger reason: a call stack is not
// a value a shell can store and keep correct. It changes with every function
// call and every sourced file, and an array set once would be right until the
// first `source` — which is the worst shape a bug can have, because the
// script that reads it is usually the one working out where it lives.
func (r *Runner) SetDynamicArray(name string, value func(*Runner) []string) {
	if r.DynamicArrays == nil {
		r.DynamicArrays = map[string]func(*Runner) []string{}
	}
	r.DynamicArrays[name] = value
}

// SetDynamic registers a parameter whose value is produced when it is read.
//
// This is the seam a dialect uses for the parameters it has and the others do
// not: `RANDOM` is a different number every time, `SECONDS` counts, and
// neither can be a stored string. A dialect calls this from Apply, the same
// place it registers a builtin.
func (r *Runner) SetDynamic(name string, value func(*Runner) string) {
	if r.Dynamic == nil {
		r.Dynamic = map[string]func(*Runner) string{}
	}
	r.Dynamic[name] = value
}

// SetSpecial gives a parameter a fixed value unless a script has already set
// one, which is how a dialect supplies something like UID.
func (r *Runner) SetSpecial(name, value string) {
	if _, ok := r.Vars[name]; ok || r.removed[name] {
		return
	}
	r.setVarQuietly(name, value)
}

// Assigned reports what a script last assigned to a produced parameter, so
// its producer can take that as a starting point — which is what `SECONDS`
// does with it.
func (r *Runner) Assigned(name string) (string, bool) {
	v, ok := r.assigned[name]
	return v, ok
}

// Randoms is the source `RANDOM` draws from, in the range every shell in the
// panel uses.
func Randoms() string { return strconv.Itoa(rand.IntN(32768)) }

// Now is what this shell calls the current time: the Clock hook where one is
// set, and the wall clock otherwise. Every place in the engine that needs the
// time of day goes through it, so an embedder that must not be at the mercy
// of the clock has one place to say so.
func (r *Runner) Now() time.Time {
	if r.Clock != nil {
		return r.Clock()
	}
	return time.Now()
}

// StartedAt is when this runner began, which is what `SECONDS` counts from
// and what `printf '%(fmt)T' -2` writes. Before the first Run it is the
// current time, so the value is never the zero one.
func (r *Runner) StartedAt() time.Time {
	if r.started.IsZero() {
		return r.Now()
	}
	return r.started
}

// Uptime is what `SECONDS` counts, from when the runner was made.
func (r *Runner) Uptime() time.Duration {
	if r.started.IsZero() {
		return 0
	}
	return time.Since(r.started)
}

// SecondsFrom is `SECONDS`: the time since the runner started, counted from
// whatever a script last assigned to it.
func (r *Runner) SecondsFrom() float64 {
	base := 0.0
	if v, ok := r.Assigned("SECONDS"); ok {
		if n, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			base = n
		}
	}
	return base + r.Uptime().Seconds()
}
