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
// Five of them are unanimous across the panel and belong here; the rest are
// not, and belong to whichever dialects have them. Which *variables* a shell
// provides is the same kind of question as which builtins it has — neither
// grammar nor a conflict about meaning — so it is answered through the same
// seam, in each dialect's Apply.
//
// This was found by running a real script rather than by the corpus. A system
// script began `if [ $UID -ne 0 ]`, and with UID unset that is `[ -ne 0 ]` —
// which is not the same test and does not fail in the same way.
//
// Two of the five have to be produced when they are read rather than stored:
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
			// The tracked argument in the dialects that move `$_`.
			if r.lastArgSet &&
				r.ask(r.sem().UnderscoreTracksTheLastArgument, "`$_` following the last argument") {
				return r.lastArg
			}
			return r.underscoreAtStartup()
		}
	}
	if _, ok := r.Vars["PPID"]; !ok && !r.removed["PPID"] {
		r.setVarQuietly("PPID", strconv.Itoa(os.Getppid()))
	}
	if _, ok := r.Vars["OPTIND"]; !ok && !r.removed["OPTIND"] {
		// The index the next `getopts` will read, which is 1 before the
		// builtin has ever run — unanimous across the panel, so it is a
		// starting value here rather than an axis. Writing it here and not
		// inside `getopts` is the whole point: a script that tests OPTIND
		// before entering its loop, or that is handed no options at all,
		// reads a number in every real shell and read nothing here.
		//
		// It is written into Vars rather than left to the environment
		// because the panel *overwrites* what the environment brought:
		// `OPTIND=7 sh -c 'echo $OPTIND'` prints 1 in all six, so an
		// inherited value must not show through.
		r.setVarQuietly("OPTIND", "1")
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
	if r.LoginShell && r.sem().LoginShowsLInDollarDash == Yes {
		// A login shell, in the two shells that say so. No majority to
		// follow — see Semantics.LoginShowsLInDollarDash — and read from
		// the fact the front end carried in for the reason `i` above is:
		// login-ness is an invocation fact and no `set` letter writes it.
		b.WriteByte('l')
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

// ForgetLastArgument puts `$_` back to the value it has before any command
// has run, which is what a front end calls after sourcing text the script's
// author did not write.
//
// A dialect's prelude is shell, so it moves `$_` exactly as a script would:
// bash's ends in an assignment, which leaves the parameter empty everywhere
// that tracks it, and a script whose first line read `$_` got that instead of
// the startup value. It is the same reasoning that already holds the
// invocation's `set` options back until the prelude has run — tracing the
// dialect's own plumbing under `-x`, or stopping on it under `-e`, reports on
// machinery nobody wrote.
//
// Exported rather than done inside Run because only the caller knows which
// text was the script's: the interpreter is handed a parsed file either way
// and the two are indistinguishable to it.
func (r *Runner) ForgetLastArgument() { r.lastArg, r.lastArgSet = "", false }

// underscoreAtStartup is `$_` before anything has moved it, and it is still
// the answer afterwards in the shells that never move it at all.
//
// Two questions in order, because that is the order the panel answers them
// in: an `_` the environment carried wins, and the invocation is written only
// where the environment said nothing. bash is the only member that writes
// one, and it writes argv[0] — so it is the invocation and not `$0`, which a
// `-c` shell takes from its first operand.
//
// A shell that *discards* the environment's `_` still reaches the second
// question, which the panel cannot decide either way — the one member that
// discards is also the one that writes nothing — so the two axes compose
// rather than the first swallowing the second.
//
// Neither is asked where the answer cannot matter. An axis a dialect has not
// answered reports itself, and a shell that was handed no `_` and has no
// invocation to write has nothing to disagree about.
func (r *Runner) underscoreAtStartup() string {
	if v, ok := r.inheritedValue("_"); ok {
		if r.ask(r.sem().UnderscoreInheritsFromTheEnvironment, "`$_` taking the value the environment brought") {
			return v
		}
		if r.sem().UnderscoreInheritsFromTheEnvironment != No {
			// Refused for want of an answer rather than answered no, and
			// the refusal has already been reported. Asking the second
			// question would report the same thing twice about one read.
			return ""
		}
	}
	if r.Invocation != "" &&
		r.ask(r.sem().UnderscoreStartsAtTheInvocation, "`$_` starting at the invocation") {
		return r.Invocation
	}
	return ""
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

// SetDynamicWriter says what happens when a script assigns to a produced
// scalar parameter.
//
// The write half of SetDynamic, and required rather than optional for any
// produced scalar a script is allowed to assign to. The failure it prevents is
// the one SetDynamicAssocWriter documents for a table: the producer answers
// ahead of the stored value, so an assignment with nowhere to go is accepted
// in silence and then read back as whatever the producer says. Nothing about
// the name changes and no diagnostic is written — the script simply finds that
// setting the parameter did nothing.
//
// A produced scalar a script must *not* assign to is marked readonly instead,
// which refuses with a sentence. And a producer that only wants to *know* what
// was last assigned needs neither: Assigned already holds it, which is how
// SECONDS counts from a value it was given.
func (r *Runner) SetDynamicWriter(name string, write func(r *Runner, value string)) {
	if r.dynamicWriters == nil {
		r.dynamicWriters = map[string]func(*Runner, string){}
	}
	r.dynamicWriters[name] = write
}

// UnsetDynamic takes a produced parameter away again, writer and all.
//
// The counterpart of SetDynamic for a parameter that exists only while
// something is happening. A dialect whose parameters are those of a *call* —
// the line a line editor is holding while it runs an action of the shell's
// own, say — has to be able to end them, because a produced parameter answers
// every read and a script that finds one outside the call would be told a
// value where the shell being modeled says nothing at all.
//
// It leaves no trace: not the removal `unset` records, which would keep a
// later SetDynamic from answering, not the message Assigned holds, which
// belongs to a call that is over, and **not the readonly mark**, which is the
// one that had to be reached for rather than reasoned about. A produced
// parameter a script must not assign to is marked readonly rather than given a
// writer, and there is no other way to lift that mark; leaving it would make
// the name refuse an ordinary assignment for the rest of the session, long
// after the thing it belonged to had finished. `$WIDGET` in dialect/zsh is the
// case: read-only while a widget runs — measured, real zsh answers
// `read-only variable: WIDGET` — and an ordinary variable outside one.
func (r *Runner) UnsetDynamic(name string) {
	delete(r.Dynamic, name)
	delete(r.dynamicWriters, name)
	delete(r.assigned, name)
	delete(r.readonly, name)
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

// FloatPlacesOf is how many decimal places a name's float attribute writes it
// in, and whether the name has one at all — `typeset -F 3 SECONDS=0`.
//
// Exported because a *produced* parameter cannot see the attribute tables:
// its value is counted on each read by a function the dialect registered, so
// the attribute that decides how that value is written back is the one thing
// the producer has to come here for. Every stored name meets the attribute in
// attributeFolded instead and needs nothing from this.
func (r *Runner) FloatPlacesOf(name string) (int, bool) {
	prec, ok := r.floatPrecision[name]
	if !ok {
		return 0, false
	}
	return floatPlaces(prec), true
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
