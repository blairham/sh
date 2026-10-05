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
	a.shutOffShort = map[string]bool{}
	a.optArgs, a.optArgValues = map[string]string{}, map[string]int{}
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
			if cursor && !a.readsAsALetterStack(word) && a.cursorWritesAnArgumentLater(word) {
				// A name whose argument can only be reached by typing more
				// into this same word is not yet an option: the person has
				// written a prefix of one. Measured — see
				// cursorWritesAnArgumentLater — and it is why `cmd -o<TAB>`
				// against `-o=[out]:out:` answers with the *first normal
				// argument* and reports `-o` on `$line`.
				break
			}
			if skip := a.takeOption(word, cs.words[i+1:], cursor); skip >= 0 {
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
				if cursor {
					a.cursorIsOption = true
					a.cursorOption = a.optionSpelledOut(word)
					a.cursorOptArg = a.optionArgumentInTheWord(word)
				}
				if at := cs.current - 1; !cursor && i < at && at <= i+skip {
					// The option ate the word the cursor is in, so the cursor
					// is writing one of the option's own arguments rather
					// than a normal one. Which of them is how many the option
					// had already taken when it reached this word.
					a.cursorOptArg = a.optionArgumentAfterTheWord(word, at-i-1)
					if n := at - i - 1; a.cursorOptArg == nil && n < len(a.stackArgs) {
						// A stack's letters took the words after it.
						here := a.stackArgs[n]
						a.cursorOptArg = &here
					}
				}
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
	a.stackInProgress = a.continuingAStack(cs)
	a.optionsPossible = options
	a.optionsHere = options && a.optionsCompletable(cs)
	if (a.cursorOptArg == nil && !a.cursorIsOption) || a.cursorInAnOptionalWord() {
		a.here = a.applicable(position)
	}
	a.line = a.normalArguments(r, cs)
}

// optionsCompletable is whether the word under the cursor could be an option
// at all: an empty word could become one, and a word already opening with `-`
// or `+` is one being written. A word that has begun as anything else cannot.
//
// **It gates `-i` and not `-O`**, which is the pair of answers that separates
// them and the divergence #3039 recorded. `-i` is asked whether there is
// anything to complete at the cursor and reads the word; `-O` is asked what
// options this *position* could still take and does not. Measured on zsh
// 5.9.2, 2026-09-16 through a pseudo-terminal from inside a `zle -C` widget,
// with `-v[verbose]` and `-o[opt]:val:` the only specs:
//
//	line        zsh -i  zsh -O
//	cmd <TAB>   0       0, next=(-v:verbose -o:opt)
//	cmd -<TAB>  0       0, next=(-v:verbose -o:opt)
//	cmd f<TAB>  1       0, next=(-v:verbose -o:opt)
//
// so the third row is the one that tells them apart, and `-O` answering 1
// with four empty arrays there was this builtin's own reading rather than
// zsh's. `_arguments` does the filtering itself — `[[ "$PREFIX" = [-+]* ]] &&
// tmp1=( "${(@M)tmp1:#${PREFIX[1]}*}" )` — and reads `-O`'s status to decide
// whether to ask `_tags` for the `options` tag at all, so a 1 here is a
// shipped completion told this position takes no options.
//
// What does still stop `-O` is the position: measured on the same day, `cmd a
// foo<TAB>` under `(-)1:first:(a b)` and `cmd sub foo<TAB>` under `*:: :->rest`
// are both 1 with four empty arrays on zsh, because the argument and the
// sub-command's takeover really did shut the options off.
func (a *argumentsState) optionsCompletable(cs *completionState) bool {
	word := cs.prefix
	return word == "" || word[0] == '-' || word[0] == '+'
}

// takeOption marks one word as the option it names and answers how many
// following words its arguments ate, or -1 where the word is not an option
// this spec set knows.
func (a *argumentsState) takeOption(word string, after []string, cursor bool) int {
	name, value, attached := a.lookupOption(word)
	if name == "" {
		if !a.stackable(word) {
			return -1
		}
		return a.takeStack(word, after, cursor, true)
	}
	a.spend(name, cursor)
	spec := a.optionNamed(name)
	if spec == nil {
		return 0
	}
	a.openOptArg(name, spec)
	if attached {
		// **And goes on taking the ones it has left.** An option carrying
		// its first argument against its own name still wants a word each
		// for the rest: measured on zsh 5.9.2, 2026-09-18 with
		// `-u=[u2]:u:_u:v:_v` declared, `cmd -u=a <TAB>` reports `-u` mapped
		// to `a:` — the attached `a`, the separator, and the empty word the
		// cursor is in. This stopped at the attached one, so a second
		// argument was an ordinary word on `$line`.
		a.pushOptArg(name, value)
		return a.eatOptionArgs(name, spec.optargs[1:], after)
	}
	// **An option is in `$opt_args` whether or not its argument is there
	// yet.** Measured on zsh 5.9.2, 2026-09-16 from inside a `zle -C` widget
	// with `-n[next]:nx:`, `-e=-[eqd]:ed:`, `-d-[dir]:dir:`, `-f+[file]:file:`
	// and `-a[plain]` declared, asking `comparguments -W` about the word under
	// the cursor: every one of `cmd -n`, `cmd -e`, `cmd -d`, `cmd -f` and
	// `cmd -a` answers with that option mapped to an empty string, and
	// `cmd -fval` and `cmd -o=val` map theirs to `val`. This recorded nothing
	// at all for the four that declare an argument and had not been given one.
	//
	// **An option that takes more than one word joins them with a colon.**
	// Measured with `-C+[copy]:from:(f1 f2):to:(t1 t2)` declared and
	// `cmd -C a b foo<TAB>`: zsh reports `-C` mapped to `a:b` and `$line` as
	// `foo` alone. This kept only the last word, so the two-argument options
	// the shipped completions declare lost their first. The colon is the
	// default separator — the third argument `-W` takes is
	// `_arguments`' own `$opt_args_use_NUL_separators`, and it is empty on
	// every call measured.
	return a.eatOptionArgs(name, spec.optargs, after)
}

// eatOptionArgs takes one word for each of the option's remaining arguments
// and records them under the option's name.
func (a *argumentsState) eatOptionArgs(name string, want []argumentSpec, after []string) int {
	eaten := 0
	for range want {
		if eaten >= len(after) {
			break
		}
		a.pushEatenWord(name, after[eaten])
		eaten++
	}
	return eaten
}

// pushEatenWord records a word an option took as one of its arguments.
//
// **An optional argument takes the next word as a required one does**, and
// **a word an option takes is still read as an option if it names one.**
// Measured on zsh 5.9.2, 2026-10-05 from inside a `zle -C` widget, with
// `-a[all]`, `-q[q]::qq:`, `-n[n]:nn:` and `1:first:(a b)` declared, asking
// `-D` and `-W` (#6189):
//
//	cmd -q a b <TAB>   argument-rest   opt_args -q a         $line=(b '')
//	cmd -q -z <TAB>    argument-1      opt_args -q -z        $line=('')
//	cmd -q -a <TAB>    argument-1      opt_args -q -a, -a ''
//	cmd -n -a <TAB>    argument-1      opt_args -n -a, -a '' and -O no longer offers -a
//
// This stopped at an optional argument, so its word was the command's first
// normal one, and an option's word that named an option was only a value.
func (a *argumentsState) pushEatenWord(name, word string) {
	a.pushOptArg(name, word)
	if named, _, attached := a.lookupOption(word); named == word && !attached {
		if spec := a.optionNamed(named); spec != nil {
			a.spend(named, false)
			a.openOptArg(named, spec)
		}
	}
}

// openOptArg records that the option is on the line, whether or not it has
// been given an argument yet.
//
// Measured on zsh 5.9.2: `cmd -f<TAB>` against `-f+[file]:file:` reports
// `$opt_args` holding `-f` mapped to the empty string — so an option with an
// argument it has not been given is *there*, and a completion testing
// `(( $+opt_args[-f] ))` sees it.
//
// A **repeatable** option keeps what earlier occurrences gave it and anything
// else starts again, which is the whole of the difference the `*` makes to
// this map. Measured: `cmd -I/u -I/v<TAB>` against `*-I+[inc]:dir:` reports
// `/u:/v`, and `cmd -I/u -I<TAB>` reports `/u` — the second occurrence had
// nothing to add, so nothing was added, not even a separator.
func (a *argumentsState) openOptArg(name string, spec *optionSpec) {
	if _, had := a.optArgs[name]; had && spec.repeat {
		return
	}
	a.optArgs[name] = ""
	a.optArgValues[name] = 0
}

// pushOptArg adds one argument value to what the option has collected, with
// the separator that goes between two of them.
//
// The count and not the text decides the separator: an option given one empty
// argument holds the empty string and one given two holds a lone colon, and
// nothing about the text tells those apart. Measured — `cmd -T <TAB>` against
// a two-argument `-T` is `-T ”` and `cmd -T a <TAB>` is `-T 'a:'`.
//
// **No line can tell the two readings apart today**, and that is worth saying
// rather than leaving as an untested branch: a word is empty only where the
// cursor is in it, so an empty value is always the last one, and a rule that
// looked at the text so far would agree on every line that can be typed. The
// count is what the shape *is*; the text is a coincidence of where a cursor
// can stand.
func (a *argumentsState) pushOptArg(name, value string) {
	if a.optArgValues[name] > 0 {
		a.optArgs[name] += ":"
	}
	a.optArgs[name] += value
	a.optArgValues[name]++
}

// stackable is whether a word on the line could be a stack of single-letter
// options at all: `-s` given, a `-` or a `+` in front, and something after it.
//
// **A `+` leads a stack as a `-` does.** Measured on zsh 5.9.2, 2026-09-16
// with `-s` and `-+a[plus]`, `-+b[bee]` declared: `cmd +ab<TAB>` reports
// `$opt_args` as `+a ” +b ”` and an empty `$line`, exactly as `cmd -ab`
// reports `-a ” -b ”`. This read the `-` spelling only, so a `+` stack
// landed on `$line` as an ordinary argument.
func (a *argumentsState) stackable(word string) bool {
	return a.stacking && len(word) > 1 &&
		(word[0] == '-' || word[0] == '+') && word[1] != word[0]
}

// takeStack is `-s`: a word of single letters, each of which is an option.
// The letters are read so that `-xy` spends both `-x` and `-y`.
//
// **All of them or none of them.** A word that reaches a letter the specs do
// not know is not a stack at all, and the letters before it are not spent
// either — this used to spend as it walked and then give up part-way, which
// left `-a` on the line for a word zsh reads as an ordinary argument.
// Measured on zsh 5.9.2, 2026-09-16 with `-s` and `-n[next]:nx: -a[plain]
// -p[proc] 1:first:(x y)` in force:
//
//	typed       zsh $opt_args   zsh $line   zsh -O next
//	cmd -az     (empty)         -az         -n -a -p
//	cmd -na     -a '' -n ''     (empty)     -p
//
// so `-az` spends nothing and is described as the first argument, while
// `-na` — every letter an option — spends both.
//
// **Unless a letter takes its argument in the word, which ends the walk**
// (#6189): see stackSplit for where, and for what the option is given. And a
// letter whose argument is a word of its own takes the words after the
// stack, in the order the letters are written — `cmd -na <TAB>` and
// `cmd -an x<TAB>` are both describing `-n`'s argument in zsh 5.9.2.
//
// record is false for the second walk normalArguments makes, which only
// counts the words and must not record the stack a second time.
func (a *argumentsState) takeStack(word string, after []string, cursor, record bool) int {
	letters, attach, ok := a.stackSplit(word)
	if !ok {
		return -1
	}
	if record {
		a.stackArgs = a.stackArgs[:0]
	}
	eaten := 0
	for i, letter := range letters {
		spec := a.optionNamed(letter)
		if record {
			a.spend(letter, cursor)
			if spec == nil || len(spec.optargs) == 0 {
				a.optArgs[letter] = ""
			} else {
				a.openOptArg(letter, spec)
			}
		}
		if spec == nil || len(spec.optargs) == 0 {
			continue
		}
		want := spec.optargs
		if i == len(letters)-1 && attach > 0 {
			if record {
				a.pushOptArg(letter, word[1:])
			}
			want = want[1:]
		} else if !spec.style.takesTheNextWord() {
			continue
		}
		taken := len(spec.optargs) - len(want)
		for _, arg := range want {
			if eaten >= len(after) {
				break
			}
			taken++
			if record {
				a.pushEatenWord(letter, after[eaten])
				a.stackArgs = append(a.stackArgs, optionArgHere{name: letter, index: taken, spec: arg})
			}
			eaten++
		}
	}
	return eaten
}

// readsAsALetterStack is whether the bare name of an option whose argument
// comes after an `=` is read, at the cursor, as a stack of one letter rather
// than as a prefix of the option still being typed: only under `-s`, and
// only for a single-letter name. Measured on zsh 5.9.2, 2026-10-05 from
// inside a `zle -C` widget, with `--opt=[o]:oo:` and `-o=[oo]:ooo:` beside
// `1:first:(a b)`:
//
//	                -s                                   no -s
//	cmd -o<TAB>     option-o-1 [-o|], opt_args -o ''     argument-1, $line=(-o)
//	cmd --opt<TAB>  argument-1, $line=(--opt)            the same
//
// This kept the prefix reading for every name whenever `-s` was absent and
// for none when it was given, so `cmd --opt<TAB>` under `-s` described
// nothing and put `--opt` in `$opt_args` (#6189). An `=-` letter is not read
// this way: `cmd -e<TAB>` against `-e=-[ee]:…` describes nothing either way.
func (a *argumentsState) readsAsALetterStack(word string) bool {
	return a.stacking && singleLetterOption(word)
}

// stackSplit reads a word as a stack of single-letter options: the letters,
// and where the argument attached to the last of them begins, or 0 where
// none is written in the word. ok is false for a word that is not a stack.
//
// **A letter whose argument may be written against it takes the rest of the
// word**, whatever that rest holds. Measured on zsh 5.9.2, 2026-10-05 from
// inside a `zle -C` widget under `-s`, with `-a[all]`, `-f+[file]:…`,
// `-x-[dir]:…`, `-o=[oo]:…`, `-e=-[ee]:…`, `-n[n]:…` and `-q[q]::…`, asking
// `-D` at the end of the word and `-W` with the cursor after it:
//
//	cmd -afz    option-f-1  [-af|z]     opt_args -a '' -f afz
//	cmd -afaa   option-f-1  [-af|aa]    the rest need not be letters it lacks
//	cmd -axq    option-x-1  [-ax|q]
//	cmd -aoz    option-o-1  [-ao|z]     an `=` form needs no `=` in a stack
//	cmd -ao=z   option-o-1  [-ao=|z]    and takes one where it is written
//	cmd -aez    option-e-1  [-ae|z]     `=-` as well
//	cmd -af=z   option-f-1  [-af|=z]    while `+` keeps it as argument text
//	cmd -anz    argument-rest           `-n`'s argument is a word of its own
//	cmd -aqz    argument-rest           and so is an optional one
//
// The value `opt_args` gives the option is the **whole word without its
// dash** — `afz`, `ao=z` — and not the text after the letter, which is how
// zsh reports it and so how it is recorded here.
func (a *argumentsState) stackSplit(word string) (letters []string, attach int, ok bool) {
	if !a.stackable(word) {
		return nil, 0, false
	}
	for i := 1; i < len(word); i++ {
		letter := word[:1] + word[i:i+1]
		spec := a.optionNamed(letter)
		if spec == nil {
			return nil, 0, false
		}
		letters = append(letters, letter)
		if i+1 < len(word) && len(spec.optargs) > 0 && spec.style.attachesInAStack() {
			return letters, i + 1, true
		}
	}
	return letters, 0, true
}

// spend records that an option is on the line, and shuts off whatever its
// exclusion list names.
//
// **An option still under the cursor shuts off only the single-letter ones**,
// which is the `(-f --force){-f,--force}` idiom's whole behavior and was
// applied as written here (#3230). Measured on zsh 5.9.2, 2026-09-18 through
// a pseudo-terminal from inside a `zle -C` widget, one exclusion list at a
// time and `comparguments -O` read back:
//
//	list          cursor word   shut off        kept
//	(-x --yy)     -a            -x              --yy
//	(-x --yy)     -a␣           -x --yy         nothing
//	(--yy -x)     --aa          -x              --yy
//	(-x --yy)     -ab           -x              --yy
//	(-abc +z --yy) -a           +z              -abc --yy
//	(-x -xy)      -a            -x              -xy
//	(-)           -a            every -x        --yy
//
// So it is the **length of the name being shut off**, and nothing about the
// cursor word: two characters — a `-` or a `+` and one letter — are shut off
// and everything longer survives, whatever the word under the cursor is
// spelled like. The second row is the control: the same list one keystroke
// later, with the word finished, shuts off both.
//
// That is measured and not explained, and it is written down that way rather
// than dressed in a reason: a rule about what a word could still *become*
// would predict the opposite of row one, and the letter count is what
// twenty observations agree on. It is what makes `git checkout -f<TAB>` go
// on offering `--force` in that shell.
func (a *argumentsState) spend(name string, cursor bool) {
	a.spent[name] = true
	spec := a.optionNamed(name)
	if spec == nil {
		return
	}
	off := a.shutOff
	if cursor {
		off = a.shutOffShort
	}
	for _, name := range spec.excl {
		off[name] = true
	}
}

// singleLetterOption reports whether a name is a `-` or a `+` and one letter,
// which is the only kind an option still under the cursor shuts off. See
// spend.
func singleLetterOption(name string) bool {
	return len(name) == 2 && (name[0] == '-' || name[0] == '+')
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

// optionSpelledOut is the option the word under the cursor writes out in
// full, which is the one spent option `-O` may still offer — see offeredBack
// — or the empty string where the word does not.
//
// **The name alone, or the name and its `=`.** An option whose argument
// follows an `=` is still offered while the cursor is anywhere after that
// `=`, and that is what makes `_arguments` go on to ask `-L` for the
// argument. Measured on zsh 5.9.2, 2026-10-05 from inside a `zle -C` widget,
// asking `-O` with `--color=-[col]:color:`, `--opt=[o]:oo:`, `-f+[file]:f:`
// and `-x-[dir]:xx:` declared:
//
//	cmd --color=<TAB>    equal=(--color:col --opt:o)
//	cmd --color=a<TAB>   equal=(--color:col --opt:o)
//	cmd --opt=o<TAB>     equal=(--color:col --opt:o)
//	cmd -fx<TAB>         odirect=()           — attached with no `=`: spent
//	cmd -xq<TAB>         direct=()            — and the same
//
// **And only a name longer than one letter.** `-o=[out]:out:` beside
// `--oo=[o2]:out:`, with and without `-s`:
//
//	cmd -o=val<TAB>      equal=(--oo:o2 --color:col)          — `-o` is spent
//	cmd -o=<TAB>         equal=(--oo:o2 --color:col)
//	cmd --oo=val<TAB>    equal=(-o:out --oo:o2 --color:col)   — `--oo` is not
//
// which is the same split by length spend draws for what a cursor option
// shuts off. This offered none of the long ones back, so `ls --color=<TAB>`
// never reached the `-L` that describes the colors.
func (a *argumentsState) optionSpelledOut(word string) string {
	name, _, attached := a.lookupOption(word)
	if name == word {
		return name
	}
	if spec := a.optionNamed(name); attached && spec != nil && !singleLetterOption(name) &&
		(spec.style == optArgEqual || spec.style == optArgEqualDirect) {
		return name
	}
	return ""
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
	//
	// **Unless it is optional** (`n::…`), which leaves the rest standing
	// beside it, the numbered one first — and never the next numbered one.
	// Measured 2026-10-05 against zsh 5.9.2 (#6151): at `cmd <TAB>`,
	// `1::a *:r` is `argument-1 argument-rest` and `1::a 2:b` is
	// `argument-1` alone. `_kill` is that first shape, so without the rest
	// `kill <TAB>` offered signals and no process.
	for _, i := range out {
		if a.args[i].rest {
			continue
		}
		if !a.args[i].optional {
			return []int{i}
		}
		keep := []int{i}
		for _, j := range out {
			if a.args[j].rest {
				keep = append(keep, j)
			}
		}
		return keep
	}
	return out
}

// normalArguments is `$line`: the words that are not options and not an
// option's argument.
//
// **A `*::` rest specification does not take its arguments out of it.** #3039
// recorded the opposite — that zsh moves the rest-covered words out of `$line`
// and into `$words`, leaving `$#line` 0 where this reports 2 — and it is not
// so. Measured on zsh 5.9.2, 2026-09-16 through a pseudo-terminal two ways:
// asking the builtin from inside a `zle -C` widget, and letting the shipped
// `_arguments` run a completion of its own and printing the `$line` it was
// left holding. With `cmd sub arg <TAB>` under `-v[verbose] -o[opt]:val:
// *:: :->rest`, `$line` is `(sub arg ”)` and `$words` is `(sub arg ”)` as
// well — the rest specification *copies* into `$words`, it does not move.
//
//	specification              $line             $words
//	*:: :->rest                sub arg ''        sub arg ''
//	*::: :->rest               sub arg ''        sub arg ''
//	(-)*:: :->rest             sub arg ''        sub arg ''
//	1:first:(a b) *:: :->rest  a sub ''          a sub ''
//
// each asked on the same line, so that a shell reading the colon count
// differently would come apart on one of the rows.
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
		if options && (strings.HasPrefix(word, "-") || strings.HasPrefix(word, "+")) && word != "-" &&
			(i != cs.current-1 || a.readsAsALetterStack(word) || !a.cursorWritesAnArgumentLater(word)) {
			// The same rule analyze applies, and it has to be applied here
			// too: a name whose argument can only be reached by typing more
			// into this word is a normal argument being written, and `$line`
			// is measured to hold it — `cmd -o<TAB>` against `-o=[out]:out:`
			// reports `line=(-o)`.
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
		if a.stackable(word) {
			// The bookkeeping is the first walk's; this one only counts
			// words, so what the stack shuts off does not matter here.
			return a.takeStack(word, after, false, false)
		}
		return -1
	}
	spec := a.optionNamed(name)
	if spec == nil {
		return 0
	}
	want := spec.optargs
	if attached {
		// The name's own word carried the first one; the rest are words. See
		// takeOption, which counts them the same way.
		want = want[min(1, len(want)):]
	}
	// Every argument takes a word, optional ones included: see
	// pushEatenWord.
	return min(len(want), len(after))
}

// describeArguments is `-D`: the message, the action and the tag of every
// argument spec applying at the cursor.
func (a *argumentsState) describeArguments(r *interp.Runner, cs *completionState, names []string) int {
	if o := a.cursorOptArg; o != nil {
		// An **option's own** argument, which is a different question from
		// which normal argument this position is and was answered with
		// nothing until #3229. `gzip -S<TAB>` wants the suffix `-S` takes and
		// `git checkout --orphan=<TAB>` wants a branch name; neither is the
		// command's first argument, and the tag says so — `option-S-1` rather
		// than `argument-1`.
		descrs, actions, subcs := []string{o.spec.message}, []string{o.spec.action}, []string{o.tag()}
		if a.cursorInAnOptionalWord() {
			// An optional argument that is a word of its own is described
			// beside the normal argument the same word could be: measured,
			// `cmd -q <TAB>` and `cmd -q x<TAB>` against `-q[q]::qq:` and
			// `1:first:(a b)` describe `qq` and then `first` (#6189).
			for _, i := range a.here {
				descrs = append(descrs, a.args[i].message)
				actions = append(actions, a.args[i].action)
				subcs = append(subcs, a.args[i].tag())
			}
		}
		r.SetArray(names[0], descrs)
		r.SetArray(names[1], actions)
		r.SetArray(names[2], subcs)
		o.moveOptionIntoIPrefix(cs)
		return 0
	}
	if len(a.here) == 0 {
		// **Nothing to describe leaves the three names as they were**; see
		// compargumentsQuery.
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
	return 0
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
	if o := a.cursorOptArg; o != nil && o.inWord == 0 && !o.spec.optional {
		// **The cursor is in an option's argument that is a word of its own,
		// and no option goes there.** Unless the argument is optional, when
		// an option may go there instead: `cmd -q <TAB>` against
		// `-q[q]::qq:` offers `-a` and `-n` at 0. Non-zero, nothing assigned (see
		// compargumentsQuery), and 2 rather than 1 for the first argument of
		// a single-letter option under `-s`. Measured on zsh 5.9.2,
		// 2026-10-05 from inside a `zle -C` widget, the four names set to a
		// marker first (#6189):
		//
		//	                       -s    no -s
		//	cmd -n <TAB>           2     1      `-n[n]:nn:`
		//	cmd -n x<TAB>          2     1
		//	cmd -o <TAB>           2     1      `-o=[oo]:ooo:`
		//	cmd -f <TAB>           2     1      `-f+[file]:fname:`
		//	cmd -T <TAB>           2     1      `-T[t]:t1:(a):t2:(b)`
		//	cmd -T a <TAB>         1     1      its second argument
		//	cmd --opt <TAB>        1     1      `--opt=[o]:oo:`
		//	cmd --long <TAB>       1     1      `--long[l]:ll:`
		//	cmd -an x<TAB>         2            a stack's letter, `-a[all]` first
		//
		// This offered every option the position could take, as if the word
		// were the next normal argument.
		if a.inALettersFirstArgumentWord() {
			return 2
		}
		return 1
	}
	lists := make([][]string, 4)
	if a.optionsPossible {
		for _, opt := range a.opts {
			if opt.hidden || a.excluded(opt) {
				continue
			}
			for _, name := range opt.names {
				if a.spent[name] && !opt.repeat && !a.offeredBack(opt, name) {
					continue
				}
				at := optionListIndex(opt.style)
				lists[at] = append(lists[at], optionOffer(name, opt))
			}
		}
	}
	offered := false
	for _, list := range lists {
		offered = offered || len(list) > 0
	}
	if !offered {
		// See compargumentsQuery.
		return 1
	}
	for i, name := range names[:4] {
		r.SetArray(name, lists[i])
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
	return 0
}

// describeOption is `-L`: the first argument of one named option, as `-D`
// would describe it with the cursor in that argument — message, action, and
// the `option-NAME-1` tag.
//
// `_arguments` asks it when the word under the cursor is an option written
// with its `=` — measured over `ls --color=<TAB>` on zsh 5.9.2, 2026-10-05,
// the trace reads `-D`, `-O`, `-M`, `-W` and then
// `comparguments -L --color descrs actions subcs`. Its answers, from inside
// a `zle -C` widget against one spec set holding every argument form:
//
//	-L -f        0  fname  _files          option-f-1   -f+[file]:fname:_files
//	-L --color   0  color  (never always)  option--color-1
//	-L -x        0  xx     (x1)            option-x-1   -x-[dir]:xx:(x1)
//	-L -m        0  first  (x y)           option-m-1   two arguments: the first
//	-L -a        1  — no argument; the names keep what they held
//	-L -zz       1  — no such option
//	-L --color=  1  — the name must be exact
//
// So it is a lookup by exact name and nothing about the line: none of those
// options was on it. Before this it was refused as an invalid option, which
// `_arguments` printed over the line.
func (a *argumentsState) describeOption(r *interp.Runner, names []string) int {
	spec := a.optionNamed(names[0])
	if spec == nil || len(spec.optargs) == 0 {
		return 1
	}
	here := optionArgHere{name: names[0], index: 1, spec: spec.optargs[0]}
	r.SetArray(names[1], []string{here.spec.message})
	r.SetArray(names[2], []string{here.spec.action})
	r.SetArray(names[3], []string{here.tag()})
	return 0
}

// offeredBack is the one spent option that is offered all the same: the one
// the word under the cursor spells out, where writing it again is what the
// person is doing.
//
// **A spent option is withheld and this one is not**, which is the shape a
// guess gets wrong in both directions. Measured on zsh 5.9.2, 2026-09-16
// through a pseudo-terminal from inside a `zle -C` widget, asking
// `comparguments -O` with the word under the cursor being exactly the option
// named — all twelve cells, because the rule turns on the argument form *and*
// on whether `_arguments` was given `-s`:
//
//	spec form         cursor word   without -s   with -s
//	-a[plain]         -a            offered      withheld
//	-n[next]:nx:      -n            offered      offered
//	-o=[out]:out:     -o            offered      withheld
//	-e=-[eqd]:ed:     -e            offered      withheld
//	-d-[dir]:dir:     -d            withheld     withheld
//	-f+[file]:file:   -f            withheld     withheld
//
// and it is the word under the cursor and no other: with `--all` and
// `--almost` declared, `cmd --all --almost<TAB>` offers `--almost` back and
// not `--all`.
//
// The two rules that table is:
//
//   - **an argument that attaches with nothing between it and the option is
//     already being written**, so `-d-` and `-f+` are never offered back —
//     the word under the cursor is the option plus the start of its argument,
//     and `comparguments -D` is what describes that argument.
//   - **where a stack is being continued, everything else is its next
//     letter**, which `_arguments` builds by writing `$PREFIX` in front of
//     each name `-O` hands it; offering `-a` back there would produce `-aa`.
//     The one exception is an argument written as its own word, because a
//     stack cannot continue past one.
//
// It is the stack and not the switch: `-s` given and `--all` under the cursor
// offers `--all` back, because a long option is not a stack — measured beside
// the short `-a` in the same spec set, where it is withheld. That is the same
// predicate `-s` answers with, so the two are asked of one field.
//
// Visible, and not only in the status: `git checkout --force<TAB>` closes the
// word and adds a space on `/bin/zsh` against this machine's own functions,
// and offered nothing here.
func (a *argumentsState) offeredBack(opt optionSpec, name string) bool {
	if name == "" || name != a.cursorOption {
		return false
	}
	switch opt.style {
	case optArgDirect, optArgOptDirect:
		return false
	case optArgSeparate:
		return true
	}
	return !a.stackInProgress
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
	// And the half an option still under the cursor shut off, which reaches
	// only the single-letter names — `-` standing for every option included.
	// See spend.
	for _, name := range opt.names {
		if !singleLetterOption(name) || name == a.cursorOption {
			// **An option does not shut itself off while it is the word
			// being typed.** Measured: `(-a)-a[all]` answers `cmd -a<TAB>`
			// with `-a -m -p` and `(-)-a[all]` answers it with `-a --yy`, so
			// a list naming its own option — `-` included — leaves that
			// option offered. It is the same "offered back" the cursor word
			// already gets, and reading the list first would take it away.
			continue
		}
		if a.shutOffShort["-"] || a.shutOffShort[name] {
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
//
// **Whether there is a description is whether there were brackets**, not
// whether they held anything: measured on zsh 5.9.2, 2026-10-05, `-j[]:jm:`
// and `-k[]` are offered as `-j:` and `-k:`, and `-l[ ]` as `-l: `. This
// dropped the colon for the empty pair, which put the option among the
// undescribed ones `compdescribe` packs together.
func optionOffer(name string, opt optionSpec) string {
	if !opt.described {
		return name
	}
	return name + ":" + opt.descr
}

// reportMatcher is `-M`: the match specification option names are completed
// with, which is `_arguments`' own `-M` where it had one and the documented
// default otherwise.
func (a *argumentsState) reportMatcher(r *interp.Runner, names []string) int {
	spec := a.matchSpec
	if spec == "" {
		spec = defaultArgumentsMatcher
	}
	r.SetVar(names[0], spec)
	return 0
}

// reportLine is `-W`: the normal arguments as `$line`, the options already
// written as `$opt_args`, and a third argument the shipped `_arguments`
// always passes — its own `$opt_args_use_NUL_separators`, which is empty.
//
// **Three arguments, not two.** Measured on zsh 5.9.2, 2026-09-16 from inside
// a `zle -C` widget: `comparguments -W line opt_args` is
// `comparguments:9: not enough arguments` at status 1, and the same call with
// a third argument is 0. This took two and answered.
func (a *argumentsState) reportLine(r *interp.Runner, names []string) int {
	r.SetArray(names[0], a.line)
	r.SetAssoc(names[1], a.optArgs)
	return 0
}

// reportStack is `-s`: whether a stack of single-letter options is being
// continued at the cursor, and — in the parameter it names — the one shape
// where `_arguments` should close the word rather than go on stacking.
//
// Measured on zsh 5.9.2: `uname -` with `-s` in force answers 1, `uname -a`
// answers 0, and `uname --a` answers 1 — a long option is not a stack. The
// parameter is empty in all three, and in every one of the eighteen shipped
// traces; the one line that fills it is a stack of **exactly one letter**
// whose option takes its argument as a separate word:
//
//	specs `-n[next]:nx:` `-a[plain]` `-p[proc]`, with -s
//
//	cmd -<TAB>    1, single=          not a stack yet
//	cmd -n<TAB>   0, single=next      one letter, and its argument is a word
//	cmd -a<TAB>   0, single=          one letter, and no argument
//	cmd -an<TAB>  0, single=          two letters, so the stack goes on
//	cmd -na<TAB>  0, single=
//
// `_arguments` reads `next` there and writes `compadd -Q - "$PREFIX$SUFFIX"`,
// which is why the same row is the one where `-O` offers the option back; see
// offeredBack. The `direct` and `equal` values `_arguments` also tests for
// were not produced by any spec form asked here — `-d-`, `-f+`, `-o=` and
// `-e=-` each answer with an empty parameter — so they are left unwritten
// rather than guessed at.
func (a *argumentsState) reportStack(r *interp.Runner, cs *completionState, names []string) int {
	if a.inALettersFirstArgumentWord() {
		// The same position `-O` answers 2 for, and `-s` answers it as a
		// stack with nothing to say: measured, `cmd -n <TAB>`, `cmd -f <TAB>`
		// and `cmd -an x<TAB>` under `-s` are 0 with the parameter emptied,
		// where `cmd -T a <TAB>` and `cmd --opt <TAB>` are 1 (#6189).
		r.SetVar(names[0], "")
		return 0
	}
	if !a.stackInProgress {
		return 1
	}
	r.SetVar(names[0], a.singleOption())
	return 0
}

// continuingAStack is that question asked of the word under the cursor: is
// this a `-xy…` under `-s` with another letter still to come.
//
// **Every letter after the dash has to be a single-letter option, and the
// word must not be a longer option of its own.** Measured on zsh 5.9.2,
// 2026-09-16 from inside a `zle -C` widget with `-s` in force, asking
// `comparguments -s`:
//
//	specs                           typed        -s
//	-n:nx: -a -p                    cmd -a       0  — a letter they know
//	-n:nx: -a -p                    cmd -na      0  — and so is the next
//	-n:nx: -a -p                    cmd -z       1  — `-z` is not one
//	-n:nx: -a -p                    cmd -az      1  — nor is `-z` here
//	-ab:x: -a -p                    cmd -ab      1  — `-b` is not one
//	-ab:x: -a -b -p                 cmd -ab      1  — and `-ab` is an option
//	-o=[out]: -f+[file]: -p         cmd -o=val   1  — nor `=`, `v`, `a`, `l`
//	-o=[out]: -f+[file]: -p         cmd -fval    1
//	-a -m                           uname --a    1  — `--` is not one
//
// The last two rows of the first group are what separate the two halves of
// the rule: with `-b` undeclared the letter walk already refuses `-ab`, and
// with it declared only "the word is itself a longer option" does. A lone `-`
// and a `--` fall out of the letter walk, which is why neither is spelled
// out here: the mutation that removed a `word[1] == '-'` guard killed no
// test, so the guard was not carrying the answer.
//
// The point of the rule is that a word reaching a letter the specs do not
// know stops being a stack rather than becoming one with a typo in it — and
// see takeStack, where the same words are read off the line.
func (a *argumentsState) continuingAStack(cs *completionState) bool {
	word := cs.prefix
	if !a.stacking || len(word) < 2 || (word[0] != '-' && word[0] != '+') {
		return false
	}
	if len(word) > 2 && a.optionNamed(word) != nil {
		return false
	}
	// A letter that has taken the rest of the word as its argument ends the
	// stack: measured, `cmd -fx<TAB>` with `-f+[file]:…` and `-x-[dir]:…`
	// both declared answers `-s` with 1 and leaves the parameter alone,
	// where `cmd -af<TAB>` answers 0.
	_, attach, ok := a.stackSplit(word)
	return ok && attach == 0
}

// cursorInAnOptionalWord is whether the cursor is in an option's optional
// argument written as a word of its own, which is also where a normal
// argument may be.
func (a *argumentsState) cursorInAnOptionalWord() bool {
	o := a.cursorOptArg
	return o != nil && o.inWord == 0 && o.spec.optional
}

// inALettersFirstArgumentWord is whether the cursor is in the first argument
// of a single-letter option, written as a word of its own, under `-s` — the
// one position `-O` answers 2 for and `-s` answers 0. See offerOptions.
func (a *argumentsState) inALettersFirstArgumentWord() bool {
	o := a.cursorOptArg
	return o != nil && o.inWord == 0 && a.stacking && singleLetterOption(o.name) && o.index == 1
}

// singleOption is the value `-s` writes into the parameter it names: `next`
// where the stack is one letter whose argument is its own word, and nothing
// otherwise.
func (a *argumentsState) singleOption() string {
	if !a.stackInProgress {
		return ""
	}
	if spec := a.optionNamed(a.cursorOption); spec != nil &&
		spec.style == optArgSeparate {
		return "next"
	}
	return ""
}

// cursorWritesAnArgumentLater reports whether the word under the cursor is an
// option's bare name whose argument cannot be written until more is typed
// into this same word.
//
// Such a word is not read as an option at all. Measured on zsh 5.9.2,
// 2026-09-18 from inside a `zle -C` widget, the word under the cursor being
// exactly the name:
//
//	-o=[out]:out:_o    cmd -o<TAB>   argument-1, $line=(-o), $opt_args empty
//	-u=[…]:u:_u:v:_v   cmd -u<TAB>   the same
//	-e=-[eq]:eq:_e     cmd -e<TAB>   nothing described, $opt_args=(-e '')
//	-T[sep]:t:_t       cmd -T<TAB>   nothing described, $opt_args=(-T '')
//	-f+[file]:file:    cmd -f<TAB>   option-f-1 — the argument attaches here
//
// So it is `-opt=` alone, and the rows around it are what say the reading is
// that narrow: `-e=-` needs the same `=` and *is* read as an option, and
// `-o=<TAB>` with the `=` written is `option-o-1`. What the two `=` styles
// differ about is whether the next word may hold the argument, which is the
// only thing left to hang it on.
//
// It matters because the answer is not nothing: the shell offers the
// command's own first argument there, so a completion that has both writes
// what it has rather than falling silent.
func (a *argumentsState) cursorWritesAnArgumentLater(word string) bool {
	spec := a.optionNamed(word)
	return spec != nil && spec.style == optArgEqual && len(spec.optargs) > 0
}

// optionArgumentInTheWord is the option argument the cursor is standing in
// where the cursor's own word holds it — `-fval`, `--orphan=b`, or the bare
// `-f` of an option whose argument attaches to its name.
//
// Which argument it is, is how many separators the word already holds: an
// option taking two attached arguments is not a shape the shipped
// completions write, so this answers the first and the later ones are reached
// as words of their own. See optionArgumentAfterTheWord.
func (a *argumentsState) optionArgumentInTheWord(word string) *optionArgHere {
	name, value, attached := a.lookupOption(word)
	if name == "" {
		return a.optionArgumentInTheStack(word)
	}
	spec := a.optionNamed(name)
	if spec == nil || len(spec.optargs) == 0 {
		return nil
	}
	if !attached && !spec.style.attachesToTheName() {
		if a.readsAsALetterStack(word) && spec.style == optArgEqual {
			// A one-letter stack, whose argument goes straight against the
			// letter: see readsAsALetterStack.
			return &optionArgHere{name: name, index: 1, spec: spec.optargs[0], inWord: len(word)}
		}
		// The name alone, and the argument is not written against it. The
		// cursor is at the end of a whole option rather than inside one of
		// its arguments.
		return nil
	}
	return &optionArgHere{
		name: name, index: 1, spec: spec.optargs[0],
		inWord: len(word) - len(value),
	}
}

// optionArgumentAfterTheWord is the option argument the cursor is standing in
// where the option took some of its arguments as words of their own and the
// cursor is the next of them.
//
// taken is how many words of its own the option had already eaten when it
// reached the cursor, so the cursor is its argument number taken+1. The first argument is there only where the option
// takes one as a word at all; everything after the first always is, which is
// measured — `-T[two]:one:(a):two:(b)` answers `cmd -T a <TAB>` with
// `option-T-2` and `-u=[…]` answers `cmd -u=a <TAB>` with `option-u-2`.
func (a *argumentsState) optionArgumentAfterTheWord(word string, taken int) *optionArgHere {
	name, _, attached := a.lookupOption(word)
	spec := a.optionNamed(name)
	if spec == nil {
		return nil
	}
	index := taken + 1
	if attached {
		// The word carried the first argument itself, so the cursor is the
		// second of them at the earliest.
		index = taken + 2
	}
	if index > len(spec.optargs) {
		return nil
	}
	if index == 1 && !spec.style.takesTheNextWord() {
		return nil
	}
	return &optionArgHere{name: name, index: index, spec: spec.optargs[index-1]}
}

// optionArgumentInTheStack is the argument the cursor is standing in where
// the word is a stack of single-letter options: the **last** letter's, which
// is the only one that can still be given an attached argument.
//
// Measured on zsh 5.9.2, 2026-09-18 with `-s` and one spec set holding every
// argument form, the word under the cursor being the stack:
//
//	cmd -ao<TAB>   option-o-1    `-o=[out]:out:`
//	cmd -ad<TAB>   option-d-1    `-d-[dir]:dir:`
//	cmd -af<TAB>   option-f-1    `-f+[file]:file:`
//	cmd -ae<TAB>   option-e-1    `-e=-[eqd]:ed:`
//	cmd -an<TAB>   nothing       `-n[next]:nx:` — its argument is a word
//
// So inside a stack every form but the separate one attaches, `=` and `=-`
// included: a stack is written without separators by definition, and the
// argument goes straight against the letter.
//
// **The bare `-o` under `-s` is a stack of one letter**, and is answered by
// optionArgumentInTheWord — see readsAsALetterStack (#6189).
func (a *argumentsState) optionArgumentInTheStack(word string) *optionArgHere {
	letters, attach, ok := a.stackSplit(word)
	if !ok {
		return nil
	}
	last := letters[len(letters)-1]
	spec := a.optionNamed(last)
	if spec == nil || len(spec.optargs) == 0 || !spec.style.attachesInAStack() {
		return nil
	}
	inWord := len(word)
	if attach > 0 {
		// The option's part of the word is the stack up to its letter, and
		// the `=` an `=` form may be written with: see stackSplit.
		inWord = attach
		if (spec.style == optArgEqual || spec.style == optArgEqualDirect) && word[attach] == '=' {
			inWord++
		}
	}
	return &optionArgHere{name: last, index: 1, spec: spec.optargs[0], inWord: inWord}
}
