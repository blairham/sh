// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strings"

	"github.com/blairham/sh/interp"
)

// What `comparguments` reads off the command line, and the six queries that
// report it. See comparguments.go for the spec language and for how the
// protocol was measured.

// analyze walks the words up to and including the one under the cursor,
// deciding which options are spent, which are shut off, how many normal
// arguments have been written, and therefore what can be completed here.
//
// **Including the one under the cursor**, which is the measured rule and the
// one a guess gets wrong — see comparguments.go's third reading.
func (a *argumentsState) analyze(r *interp.Runner, cs *completionState) {
	a.spent, a.shutOff = map[string]bool{}, map[string]bool{}
	a.optArgs = map[string]string{}
	position := 1
	options := true
	// words[0] is the command; the cursor is at the one-based cs.current.
	for i := 1; i < cs.current && i < len(cs.words); i++ {
		word := cs.words[i]
		cursor := i == cs.current-1
		if options && a.skipDash && word == "--" {
			options = false
			continue
		}
		if options && (strings.HasPrefix(word, "-") || strings.HasPrefix(word, "+")) && word != "-" {
			if skip := a.takeOption(word, cs.words[i+1:]); skip >= 0 {
				// **A word under the cursor that is already a whole option
				// describes no argument.** Measured on zsh 5.9.2, 2026-09-16,
				// with `-v[verbose]`, `-o[opt]:val:` and `*:: :->rest` in
				// force, asking `comparguments -D` from inside a widget:
				//
				//	cmd -v<TAB>    1, nothing described
				//	cmd -o<TAB>    1, nothing described
				//	cmd -vv<TAB>   1, nothing described — a stack is one too
				//	cmd -x<TAB>    0, the rest specification
				//	cmd -<TAB>     0, the rest specification
				//	cmd --<TAB>    0, the rest specification
				//
				// which is the same predicate as "is this word an option",
				// asked of the word being typed: a name the specs know, or a
				// stack of letters they all know. A lone `-`, a `--` and an
				// option nobody declared are not, and each of those three is
				// an argument being written.
				//
				// It matters beyond the status, because the rest
				// specification is what moves `$words` — so without it
				// `cmd -v<TAB>` would hand the completer a word list holding
				// only the option the person is still typing.
				a.cursorIsOption = a.cursorIsOption || cursor
				i += skip
				continue
			}
		}
		if cursor {
			// The word being completed is not yet an argument that has been
			// written: it is the one about to be.
			break
		}
		if options && a.ignorePat != "" && !r.MatchPattern(a.ignorePat, word) {
			options = false
		}
		// **An argument specification has an exclusion list too, and writing
		// the argument is what spends it.** `(-)1:first:…` means "once a
		// first argument is here, no options are"; the shipped `_git` writes
		// two of them, `(-): :->command` and `(-)*:: :->option-or-argument`,
		// and they are why `git checkout -<TAB>` offers *checkout's* options
		// rather than git's own. Measured on zsh 5.9.2, 2026-09-16 with
		// `-v[verbose]` beside each and `comparguments -O` asked from inside
		// a widget:
		//
		//	(-)1:first:(a b)  cmd -<TAB>    -v      nothing written yet
		//	(-)1:first:(a b)  cmd a -<TAB>  nothing the argument spent it
		//	1:first:(a b)     cmd a -<TAB>  -v      no list, no exclusion
		//	(-v)1:first:(a b) cmd a -<TAB>  nothing a named option, not `-`
		//
		// so it is the same mechanism an option's own `(…)` uses — including
		// `-` standing for every option — asked at the position the argument
		// landed on rather than of a name.
		for _, i := range a.applicable(position) {
			for _, off := range a.args[i].excl {
				a.shutOff[off] = true
			}
		}
		if options && a.restTakesOver(position) {
			options = false
		}
		position++
	}
	a.optionsHere = options && a.optionsCompletable(cs)
	if !a.cursorIsOption {
		a.here = a.applicable(position)
	}
	a.line = a.normalArguments(r, cs)
}

// optionsCompletable is whether the word under the cursor could be an option
// at all: an empty word could become one, and a word already opening with `-`
// or `+` is one being written. A word that has begun as anything else cannot.
func (a *argumentsState) optionsCompletable(cs *completionState) bool {
	word := cs.prefix
	return word == "" || word[0] == '-' || word[0] == '+'
}

// takeOption marks one word as the option it names and answers how many
// following words its arguments ate, or -1 where the word is not an option
// this spec set knows.
func (a *argumentsState) takeOption(word string, after []string) int {
	name, value, attached := a.lookupOption(word)
	if name == "" {
		if !a.stacking || !strings.HasPrefix(word, "-") || strings.HasPrefix(word, "--") {
			return -1
		}
		return a.takeStack(word)
	}
	a.spend(name)
	spec := a.optionNamed(name)
	if spec == nil {
		return 0
	}
	if attached {
		a.optArgs[name] = value
		return 0
	}
	eaten := 0
	for _, arg := range spec.optargs {
		if arg.optional || eaten >= len(after) {
			break
		}
		a.optArgs[name] = after[eaten]
		eaten++
	}
	if len(spec.optargs) == 0 {
		a.optArgs[name] = ""
	}
	return eaten
}

// takeStack is `-s`: a word of single letters, each of which is an option.
// The letters are read so that `-xy` spends both `-x` and `-y`; what is not
// done is offering the rest of such a word — see comparguments.go.
func (a *argumentsState) takeStack(word string) int {
	for i := 1; i < len(word); i++ {
		letter := word[:1] + word[i:i+1]
		if a.optionNamed(letter) == nil {
			return -1
		}
		a.spend(letter)
		a.optArgs[letter] = ""
	}
	return 0
}

// spend records that an option is on the line, and shuts off whatever its
// exclusion list names.
func (a *argumentsState) spend(name string) {
	a.spent[name] = true
	spec := a.optionNamed(name)
	if spec == nil {
		return
	}
	for _, off := range spec.excl {
		a.shutOff[off] = true
	}
}

// lookupOption is the spec a word names, the argument attached to it, and
// whether there was one.
func (a *argumentsState) lookupOption(word string) (string, string, bool) {
	for i := range a.opts {
		for _, name := range a.opts[i].names {
			if word == name {
				return name, "", false
			}
			if !strings.HasPrefix(word, name) {
				continue
			}
			value := word[len(name):]
			switch a.opts[i].style {
			case optArgDirect, optArgOptDirect:
				return name, value, true
			case optArgEqual, optArgEqualDirect:
				if strings.HasPrefix(value, "=") {
					return name, value[1:], true
				}
			}
		}
	}
	return "", "", false
}

// optionNamed is the spec describing one exact option name.
func (a *argumentsState) optionNamed(name string) *optionSpec {
	for i := range a.opts {
		for _, have := range a.opts[i].names {
			if have == name {
				return &a.opts[i]
			}
		}
	}
	return nil
}

// applicable is which argument specs describe the position the cursor is at:
// the numbered one if there is one, and the rest specification otherwise.
// restTakesOver is whether the argument at this position is the first one an
// *optional* rest specification covers — a `*::…` or `*:::…` rather than a
// `*:…`.
//
// It is the question "have the sub-command's own words begun", and the answer
// stops this specification reading options at all. Measured on zsh 5.9.2,
// 2026-09-16 with `-v[verbose]`, `-o[opt]:val:` and a rest specification in
// force, reading `$words` back after `comparguments -D`:
//
//	specification    line                 $words             options read?
//	*:: :->rest      cmd -v<TAB>          cmd -v             yes — `-v` is an option
//	*:: :->rest      cmd sub -v<TAB>      sub -v             no  — a word came first
//	*:: :->rest      cmd sub -o val <TAB> sub -o val ''      no  — nor its argument
//	*: :->rest       cmd sub -v<TAB>      cmd sub -v         yes — one colon, never
//	1:first: *::     cmd a -v<TAB>        cmd a -v           yes — the rest has not begun
//	1:first: *::     cmd a b -v<TAB>      a b -v             no  — `b` began it
//
// So it is not the position being *covered* that stops them; it is a word
// having been taken by the rest specification. `cmd -v` is covered by `*::`
// at position 1 and `-v` is still read as an option, because no argument has
// been written yet for the sub-command to own.
func (a *argumentsState) restTakesOver(position int) bool {
	for _, i := range a.applicable(position) {
		if a.args[i].rest && a.args[i].optional {
			return true
		}
	}
	return false
}

func (a *argumentsState) applicable(position int) []int {
	var out []int
	for i, arg := range a.args {
		if arg.rest || arg.position == position {
			out = append(out, i)
		}
	}
	// A numbered spec wins over the rest specification at its own position,
	// which is what `n:…` means: the rest describes "when neither of the
	// first two forms was provided".
	for _, i := range out {
		if !a.args[i].rest {
			return []int{i}
		}
	}
	return out
}

// normalArguments is `$line`: the words that are not options and not an
// option's argument.
//
// **The word under the cursor is one of them**, unless it is itself an
// option. Measured on zsh 5.9.2, 2026-09-15: `uname -a -` reports
// `line=(-)`, `uname -` reports `line=(-)` and `uname -a` reports an empty
// one — so the rule is about what the word *is*, not about where the cursor
// is. A shell that stopped one word short would hand `_arguments` a `$line`
// that is right only while the last word happens to be an option.
func (a *argumentsState) normalArguments(r *interp.Runner, cs *completionState) []string {
	var out []string
	options := true
	for i := 1; i < cs.current && i < len(cs.words); i++ {
		word := cs.words[i]
		if options && a.skipDash && word == "--" {
			options = false
			continue
		}
		if options && (strings.HasPrefix(word, "-") || strings.HasPrefix(word, "+")) && word != "-" {
			if skip := a.takeOptionQuietly(word, cs.words[i+1:]); skip >= 0 {
				i += skip
				continue
			}
		}
		if options && a.ignorePat != "" && !r.MatchPattern(a.ignorePat, word) {
			options = false
		}
		out = append(out, word)
	}
	return out
}

// takeOptionQuietly is takeOption without the bookkeeping: how many words an
// option eats, asked a second time while collecting `$line`.
func (a *argumentsState) takeOptionQuietly(word string, after []string) int {
	name, _, attached := a.lookupOption(word)
	if name == "" {
		if a.stacking && strings.HasPrefix(word, "-") && !strings.HasPrefix(word, "--") {
			return a.takeStack(word)
		}
		return -1
	}
	spec := a.optionNamed(name)
	if attached || spec == nil {
		return 0
	}
	eaten := 0
	for _, arg := range spec.optargs {
		if arg.optional || eaten >= len(after) {
			break
		}
		eaten++
	}
	return eaten
}

// describeArguments is `-D`: the message, the action and the tag of every
// argument spec applying at the cursor.
func (a *argumentsState) describeArguments(r *interp.Runner, cs *completionState, names []string) int {
	if len(names) < 3 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	descrs, actions, subcs := []string{}, []string{}, []string{}
	shift := false
	for _, i := range a.here {
		descrs = append(descrs, a.args[i].message)
		actions = append(actions, a.args[i].action)
		subcs = append(subcs, a.args[i].tag())
		shift = shift || (a.args[i].rest && a.args[i].optional)
	}
	r.SetArray(names[0], descrs)
	r.SetArray(names[1], actions)
	r.SetArray(names[2], subcs)
	if shift {
		a.shiftWords(r, cs)
	}
	return boolStatus(len(a.here) > 0)
}

// shiftWords is what a `*::` rest specification does to `$words` and
// `$CURRENT`, and it is the whole of why `git checkout <TAB>` reached the
// wrong completer.
//
// `_arguments` never touches either name — it is this builtin that replaces
// them, and the replacement outlives the call, because the completer whose
// action the rest specification names reads `$words[1]` to decide what to do
// next. The shipped `_git` is exactly that shape: its top-level specification
// ends in `(-)*:: :->option-or-argument` and its `option-or-argument` arm
// calls `_git-$words[1]`. With the words left as the line has them that is
// `_git-git`, which does not exist, so the fallback offered *files* where a
// person expected branches.
//
// **What it is replaced with is the normal arguments** — the same list `-W`
// reports as `$line`, options and their arguments removed, the word under the
// cursor included as the last element — and `$CURRENT` is that word's
// position in it. Measured through a pseudo-terminal on zsh 5.9.2,
// 2026-09-16, from inside a `zle -C` widget whose function calls the builtin
// itself, with `-v[verbose]`, `-o[opt]:val:` and `*:: :->rest` in force:
//
//	line                  $words before      $words after   $CURRENT
//	cmd <TAB>             cmd ''             ''             1
//	cmd sub <TAB>         cmd sub ''         sub ''         2
//	cmd sub x<TAB>        cmd sub x          sub x          2
//	cmd -o val sub arg    cmd -o val sub arg sub arg ''     3
//	cmd a b c <TAB>       cmd a b c ''       a b c ''       4
//
// and it is the *number of colons* that decides, not the star: `*:` leaves
// both alone, `*::` and `*:::` both replace them, and a numbered `2::` — an
// optional argument that is not the rest — leaves them alone as well. Each of
// those three was asked on the same line so that the answers separate.
//
//	specification            cmd sub <TAB> leaves $words
//	(-)*: :->rest            cmd sub ''
//	(-)*:: :->rest           sub ''
//	(-)*::: :->rest          sub ''
//	1:first:(a b) 2::second: cmd sub ''
//
// It is done here, on `-D`, rather than on `-i` or `-W`, because that is
// where zsh does it: a logging function shadowing the builtin over the same
// line recorded `$words` unchanged across `-i`, replaced across `-D`, and
// unchanged across `-O`, `-M` and `-W`.
func (a *argumentsState) shiftWords(r *interp.Runner, cs *completionState) {
	words := a.argumentsBefore(r, cs)
	if cs.current-1 < len(cs.words) {
		// The word under the cursor, **verbatim**. It is the one place this
		// differs from `$line`, which drops it when it is an option:
		// measured, `cmd sub -v<TAB>` leaves `$words` as `sub -v` while
		// `$line` has only what precedes it. The completer being handed the
		// words is going to complete that word, so it has to be there.
		words = append(words, cs.words[cs.current-1])
	}
	cs.words, cs.current = words, len(words)
}

// argumentsBefore is the normal arguments written *before* the word under the
// cursor: options and the words they take are dropped, everything else is
// kept, in order.
func (a *argumentsState) argumentsBefore(r *interp.Runner, cs *completionState) []string {
	out := []string{}
	options, position := true, 1
	for i := 1; i < cs.current-1 && i < len(cs.words); i++ {
		word := cs.words[i]
		if options && a.skipDash && word == "--" {
			options = false
			continue
		}
		if options && (strings.HasPrefix(word, "-") || strings.HasPrefix(word, "+")) && word != "-" {
			if skip := a.takeOptionQuietly(word, cs.words[i+1:]); skip >= 0 {
				i += skip
				continue
			}
		}
		if options && a.ignorePat != "" && !r.MatchPattern(a.ignorePat, word) {
			options = false
		}
		out = append(out, word)
		if options && a.restTakesOver(position) {
			options = false
		}
		position++
	}
	return out
}

// offerOptions is `-O`: the options still available here, sorted into the
// four arrays by where their argument may be written.
func (a *argumentsState) offerOptions(r *interp.Runner, names []string) int {
	if len(names) < 4 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	lists := make([][]string, 4)
	if a.optionsHere {
		for _, opt := range a.opts {
			if opt.hidden || a.excluded(opt) {
				continue
			}
			for _, name := range opt.names {
				if a.spent[name] && !opt.repeat {
					continue
				}
				at := optionListIndex(opt.style)
				lists[at] = append(lists[at], optionOffer(name, opt.descr))
			}
		}
	}
	offered := false
	for i, name := range names[:4] {
		r.SetArray(name, lists[i])
		offered = offered || len(lists[i]) > 0
	}
	// **The status is whether anything was offered**, not whether an option
	// could stand here. The two come apart as soon as everything is spent or
	// excluded, and `_arguments` reads the status rather than the arrays: a 0
	// with four empty arrays is a shipped completion told the options were
	// handled, which is how `git checkout -<TAB>` stopped before reaching
	// `_git-checkout`. Measured on zsh 5.9.2, 2026-09-16, with `-v[verbose]`
	// and `*:rest:`:
	//
	//	cmd -<TAB>     0, next=(-v:verbose)
	//	cmd -v -<TAB>  1, next=()             `-v` is spent
	//
	// and with `(-)1:first:(a b)` beside them, `cmd a -<TAB>` is 1 for the
	// same reason from the other direction — the argument shut the options
	// off.
	return boolStatus(offered)
}

// excluded is whether something already on the line shut this option off. The
// list may name an option, an argument number, `:` for every normal argument
// or `-` for every option.
func (a *argumentsState) excluded(opt optionSpec) bool {
	if a.shutOff["-"] {
		return true
	}
	for _, name := range opt.names {
		if a.shutOff[name] {
			return true
		}
	}
	return false
}

// optionListIndex is which of `-O`'s four arrays an option belongs in.
func optionListIndex(style optionArgStyle) int {
	switch style {
	case optArgDirect:
		return 1
	case optArgOptDirect:
		return 2
	case optArgEqual, optArgEqualDirect:
		return 3
	}
	return 0
}

// optionOffer is one element of those arrays: `name:description`, or the bare
// name where the spec gave none. Measured — `gzip`'s `--fast` comes back with
// no colon.
func optionOffer(name, descr string) string {
	if descr == "" {
		return name
	}
	return name + ":" + descr
}

// reportMatcher is `-M`: the match specification option names are completed
// with, which is `_arguments`' own `-M` where it had one and the documented
// default otherwise.
func (a *argumentsState) reportMatcher(r *interp.Runner, names []string) int {
	if len(names) < 1 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	spec := a.matchSpec
	if spec == "" {
		spec = defaultArgumentsMatcher
	}
	r.SetVar(names[0], spec)
	return 0
}

// reportLine is `-W`: the normal arguments as `$line`, and the options
// already written as `$opt_args`.
func (a *argumentsState) reportLine(r *interp.Runner, names []string) int {
	if len(names) < 2 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	r.SetArray(names[0], a.line)
	r.SetAssoc(names[1], a.optArgs)
	return 0
}

// reportStack is `-s`: whether a stack of single-letter options is being
// continued at the cursor.
//
// Measured: `uname -` with `-s` in force answers 1, and `uname -a` answers 0
// with the named parameter left empty. Nothing is offered on the strength of
// it here — see comparguments.go, where the gap is written down.
func (a *argumentsState) reportStack(r *interp.Runner, cs *completionState, names []string) int {
	if len(names) < 1 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	r.SetVar(names[0], "")
	word := cs.prefix
	stacked := a.stacking && len(word) > 1 &&
		(word[0] == '-' || word[0] == '+') && word[1] != '-'
	return boolStatus(stacked)
}
