// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"strings"

	"github.com/blairham/sh/interp"
)

// `comparguments`: the option-spec language `_arguments` is written in, and
// the biggest of `zsh/computil`'s eight.
//
// # The call shape, measured
//
// Traced through a pseudo-terminal against zsh 5.9.2 on 2026-09-15 over
// eighteen shipped completions — see computil.go for the instrument. Every
// call the shipped `_arguments` makes has one of seven verbs, and the first
// is always the same shape:
//
//	comparguments -i '' -s : '(-m -n -r -s -v)-a[print all basic information]' …
//
// which is `-i`, a match specification, `_arguments`' own switches, a bare
// `:` separating them from the specs, and the specs. The separator is what
// the manual calls out — "all options to _arguments itself may be separated
// from the spec forms by a single colon" — and the shipped function always
// writes it, so it is read here as the end of the switches rather than
// guessed at.
//
// The six queries that follow, with what each answered for `uname -` and what
// that says the verb is:
//
//	-D descrs actions subcs   1, empty — the argument specs applying here
//	-a                        1        — whether any normal argument is described
//	-O next direct odirect equal
//	                          0, next=(-a:print all basic information …)
//	-M matcher                0, r:|[_-]=* r:|=*
//	-s single                 1        — whether a stack of single-letter
//	                                     options is being continued
//	-W line opt_args 0        0        — the normal arguments and the options
//	                                     already on the line
//
// # The four option lists, and what decides between them
//
// `-O` sorts the options this position could still take into four arrays, and
// `_arguments` gives each its own `compadd`, because the *suffix* differs.
// Measured against `make -` and `gzip -`, where all four are non-empty:
//
//	next      -opt, and -opt whose argument is a separate word
//	direct    -opt-   the argument must be attached; compadd -S ''
//	odirect   -opt+   the argument may be attached
//	equal     --opt=  the argument follows an `=`; compadd -qS=
//
// Each element is `name:description`, and a name with no description is the
// bare name — measured, `gzip`'s `--fast` and `-1` come back with no colon
// while `-c:write on standard output` has one.
//
// # Three readings a guess gets wrong
//
//   - **The word under the cursor counts as already on the line.** Measured
//     with `uname -a` and a spec set holding `(-m)-a`, `-m` and `-f+`:
//     `opt_args` holds `-a`, and `-O` answers with an empty `next` — `-a` is
//     spent, `-m` is excluded by it, and `-f+` is in `odirect`. A shell that
//     read the current word as "not yet typed" would offer `-a` again.
//   - **`-i` answers whether there is anything to complete here at all**, not
//     whether the specs parsed. `comparguments -i '' '-v[verbose]'` against a
//     word `che` in argument position is 1, because an option cannot start
//     there and no argument is described; add `:cmd:(alpha beta)` and it is 0.
//   - **`-D` answers for the argument position even when the word is an
//     option.** `uname -` with a `:cmd:` spec reports `descrs=(cmd)`
//     `subcs=(argument-1)`, because both an option and the first argument can
//     be completed at that position, and `_arguments` offers both.
//
// # Option stacking, and who does the offering
//
// This file used to say that `-s` was read but the rest of a stack was never
// offered. **It is offered, and it always was** — because `_arguments` builds
// the offering rather than this builtin. When `comparguments -s` answers 0 the
// shipped function takes the names out of `-O`'s own four arrays, keeps the
// single-letter ones, strips the leading `-` and writes `$PREFIX` back in
// front; so a stacked word needs nothing from here but `-s`'s status and
// `-O`'s arrays.
//
// Measured on zsh 5.9.2, 2026-09-16 through a pseudo-terminal with `compinit`
// over this machine's own functions: `uname -a<TAB>` completes to `uname -ap `
// on `/bin/zsh` and here alike, and `gzip -c<TAB>` twice lists the same
// twenty-three stacked words from both. A function shadowing `compdescribe` on
// the same line read the array the shipped `_arguments` had built out of
// `_arguments`' own frame, and it is identical in the two shells:
//
//	_a_12=(-cd -cf -ch -ck -cl -cL -cn -cN -cq -cr -ct -cv -cV -c1 … -cS)
//
// `$single`, the parameter `-s` names, was empty in every one of those traces
// and in every synthetic spec set asked beside them — `-af`, `-ad`, `-an`,
// `-ao` against options whose arguments attach, follow, or come after an `=`
// — so the documented `direct`/`next`/`equal` values belong to a shape neither
// shell has been made to produce here, and the empty string is what zsh
// answers with.

// optionArgStyle is where an option's first argument may be written, which is
// the whole of what separates `-O`'s four arrays.
type optionArgStyle int

const (
	optArgNone        optionArgStyle = iota // -opt
	optArgSeparate                          // -opt with a `:…` — the next word
	optArgDirect                            // -opt-  attached, and only attached
	optArgOptDirect                         // -opt+  attached or the next word
	optArgEqual                             // -opt=  after an `=`, or the next word
	optArgEqualDirect                       // -opt=- after an `=`, and only there
)

// attachesToTheName reports whether this option's **first** argument may be
// written against the option's own name with nothing between them, which is
// what decides whether the cursor sitting at the end of the name is sitting
// in that argument.
//
// Measured on zsh 5.9.2, 2026-09-18, the word under the cursor being exactly
// an option's name and `comparguments -D` asked from inside a widget:
//
//	-f+[file]:file:   cmd -f<TAB>   option-f-1     the argument attaches
//	-d-[dir]:dir:     cmd -d<TAB>   option-d-1     and here it must
//	-T[sep]:t:        cmd -T<TAB>   nothing        the next word holds it
//	-n[none]          cmd -n<TAB>   nothing        there is no argument
func (s optionArgStyle) attachesToTheName() bool {
	return s == optArgDirect || s == optArgOptDirect
}

// takesTheNextWord reports whether this option's **first** argument may be a
// word of its own, which is what decides whether the empty word after the
// option is that argument rather than the first normal one.
//
// Measured the same way, with a blank after the option:
//
//	-f+[file]:file:   cmd -f <TAB>  option-f-1     either way, so yes
//	-T[sep]:t:        cmd -T <TAB>  option-T-1     the next word is where it is
//	-o=[out]:out:     cmd -o <TAB>  option-o-1     an `=` is not the only way
//	-d-[dir]:dir:     cmd -d <TAB>  argument-1     attached and only attached
//	-e=-[eq]:eq:      cmd -e <TAB>  argument-1     after an `=` and only there
func (s optionArgStyle) takesTheNextWord() bool {
	return s == optArgSeparate || s == optArgOptDirect || s == optArgEqual
}

// optionSpec is one `optspec` from the spec list: what may be typed, what it
// shuts off, and what follows it.
type optionSpec struct {
	names   []string // `-+foo` describes both `-foo` and `+foo`
	descr   string   // the `[…]` explanation, empty where there is none
	repeat  bool     // a leading `*`: may appear more than once
	hidden  bool     // a leading `!`: understood on the line, never offered
	excl    []string // the `(…)` list: what this option shuts off
	style   optionArgStyle
	optargs []argumentSpec
}

// argumentSpec is one normal-argument description, `n:message:action` and its
// relatives, or the `*:…` that covers the rest.
type argumentSpec struct {
	position int  // 1-based, or 0 for the rest specification
	rest     bool // `*:…`
	optional bool // `::…`
	message  string
	action   string
	excl     []string
}

// optionArgHere is an option's own argument, being written at the cursor.
type optionArgHere struct {
	name  string       // the option, as it is written on the line
	index int          // 1-based: which of the option's arguments this is
	spec  argumentSpec // its `:message:action`
}

// tag is what `_tags` is asked for on this argument's behalf, which is the
// option's name and the argument's place in it rather than a position on the
// line. Measured on zsh 5.9.2, 2026-09-18 from inside a `zle -C` widget:
// `cmd -f<TAB>` against `-f+[file]:file:_files` reports `option-f-1`,
// `cmd --orphan=<TAB>` reports `option--orphan-1`, and `cmd -T a <TAB>`
// against a two-argument `-T` reports `option-T-2`. So **one** leading dash
// goes and the rest of the name stays — `--orphan` keeps the second — and the
// index counts the option's own arguments rather than a place on the line.
func (o optionArgHere) tag() string {
	return "option-" + strings.TrimLeft(o.name[:1], "-+") + o.name[1:] + "-" + itoa(o.index)
}

// tag is what `_tags` is asked for on this argument's behalf, and is the
// third array `-D` fills.
func (a argumentSpec) tag() string {
	if a.rest {
		return "argument-rest"
	}
	return "argument-" + itoa(a.position)
}

// argumentsState is one `comparguments -i` and everything the queries read
// back out of it.
type argumentsState struct {
	matchSpec string
	stacking  bool   // -s
	skipDash  bool   // -S: options stop at a `--`
	ignorePat string // -A: options stop at the first argument not matching

	opts []optionSpec
	args []argumentSpec

	// What the line analysis found.
	//
	// optionsPossible is whether this *position* still takes options at all
	// — what `-O` answers about. optionsHere is that and the word under the
	// cursor being one an option could be written into, which is what `-i`
	// answers about. See optionsCompletable, where the two are separated and
	// the measurement that separates them is.
	optionsPossible bool
	optionsHere     bool
	// cursorIsOption records that the word being typed is already a whole
	// option the specs know — see analyze, where the measurement is.
	cursorIsOption bool
	// cursorOption is that option's name, where the word is exactly it. It
	// is the one spent option that may still be offered — see offeredBack.
	cursorOption string
	// cursorOptArg is the option argument the cursor is standing in, where
	// it is standing in one: the option's name and which of its arguments.
	// Nil is a cursor that is not writing an option's argument, which is
	// every position `-D` used to answer for. See optionArgumentHere.
	cursorOptArg *optionArgHere
	// stackInProgress is whether the word under the cursor is a stack of
	// single-letter options being continued: what `-s` answers, and half of
	// what decides offeredBack.
	stackInProgress bool
	here            []int    // indices into args of the specs applying at the cursor
	line            []string // the normal arguments, `$line`
	optArgs         map[string]string
	optArgValues    map[string]int  // how many values each name has collected
	spent           map[string]bool // option names already on the line
	shutOff         map[string]bool // what an option on the line excluded
	// shutOffShort is what the option *under the cursor* excluded, which
	// reaches only the single-letter names. See spend, which carries the
	// measurement.
	shutOffShort map[string]bool
}

// defaultArgumentsMatcher is what `-M` answers when `_arguments` was given no
// `-M` of its own. Documented in zshcompsys(1) and measured to be what a
// shipped completion is handed.
const defaultArgumentsMatcher = `r:|[_-]=* r:|=*`

func compargumentsBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	if !compArity(r, args, 1, -1) {
		return 1
	}
	cs, st, ok := computilFrom(r, ctx)
	if !ok {
		return 1
	}
	if args[0] == "-i" {
		return compargumentsInit(r, cs, st, args[1:])
	}
	if st.arguments == nil {
		r.Diagnosef("no parsed state\n")
		return 1
	}
	return compargumentsQuery(r, cs, st.arguments, args)
}

// compargumentsQuery is the six read-back verbs, each of which reports
// whether there is anything of its kind to complete.
func compargumentsQuery(r *interp.Runner, cs *completionState, a *argumentsState, args []string) int {
	switch args[0] {
	case "-D":
		return a.describeArguments(r, cs, args[1:])
	case "-O":
		return a.offerOptions(r, args[1:])
	case "-M":
		return a.reportMatcher(r, args[1:])
	case "-W":
		return a.reportLine(r, args[1:])
	case "-s":
		return a.reportStack(r, cs, args[1:])
	case "-a":
		return boolStatus(len(a.args) > 0)
	}
	r.Diagnosef("invalid option: %s\n", args[0])
	return 1
}

// compargumentsInit parses the specs and analyzes the line, and answers
// whether anything at all can be completed at the cursor.
func compargumentsInit(r *interp.Runner, cs *completionState, st *computilState, args []string) int {
	if len(args) == 0 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	a := &argumentsState{matchSpec: args[0]}
	rest, ok := a.readSwitches(r, args[1:])
	if !ok {
		return 1
	}
	if !a.readSpecs(r, rest) {
		return 1
	}
	a.analyze(r, cs)
	st.arguments = a
	// An option's own argument counts as something to complete, which is
	// what makes `-i` and `-D` agree. Measured: `cmd -f x<TAB>` against
	// `-f+[file]:file:_files` is 0 and describes `option-f-1`; without this
	// the word is not an option, no normal argument applies, and a 0 from
	// `-D` sat behind a 1 from `-i`.
	return boolStatus(a.optionsHere || len(a.here) > 0 || a.cursorOptArg != nil)
}

// readSwitches takes `_arguments`' own letters off the front, stopping at the
// bare `:` the shipped function always writes — or, where there is none, at
// the first word that is not one of them, so a hand-written call still parses.
func (a *argumentsState) readSwitches(r *interp.Runner, args []string) ([]string, bool) {
	for i := 0; i < len(args); i++ {
		switch word := args[i]; word {
		case ":":
			return args[i+1:], true
		case "-s":
			a.stacking = true
		case "-S":
			a.skipDash = true
		case "-n", "-w", "-W", "-C", "-R", "-0":
			// Read and not acted on: `-n` and `-C` name parameters
			// `_arguments` sets itself, `-R` changes the status it returns,
			// and `-w`/`-W` loosen stacking, which is not offered here.
		case "-A", "-O", "-M":
			if i+1 >= len(args) {
				r.Diagnosef("argument expected after %s\n", word)
				return nil, false
			}
			i++
			if word == "-A" {
				a.ignorePat = args[i]
			}
			if word == "-M" {
				a.matchSpec = args[i]
			}
		default:
			return args[i:], true
		}
	}
	return nil, true
}

// readSpecs turns each remaining word into an option or an argument
// description. A word that is neither is an error naming itself, which is the
// measured diagnostic: `comparguments -i ” x` is `invalid argument: x`.
func (a *argumentsState) readSpecs(r *interp.Runner, specs []string) bool {
	next := 1
	for _, spec := range specs {
		excl, body, star, hidden := specPrefixes(spec)
		switch {
		case body == "" && !star:
			r.Diagnosef("invalid argument: %s\n", spec)
			return false
		case star && (body == "" || body[0] == ':'):
			arg := parseArgumentSpec(body, 0)
			arg.rest, arg.excl = true, excl
			a.args = append(a.args, arg)
		case body != "" && body[0] == ':':
			arg := parseArgumentSpec(body, next)
			arg.excl = excl
			next++
			a.args = append(a.args, arg)
		case body[0] >= '1' && body[0] <= '9':
			n, rest := leadingNumber(body)
			if rest == "" || rest[0] != ':' {
				r.Diagnosef("invalid argument: %s\n", spec)
				return false
			}
			arg := parseArgumentSpec(rest, n)
			arg.excl = excl
			next = n + 1
			a.args = append(a.args, arg)
		case body[0] == '-' || body[0] == '+':
			opt, ok := parseOptionSpec(body)
			if !ok {
				r.Diagnosef("invalid argument: %s\n", spec)
				return false
			}
			opt.excl, opt.repeat, opt.hidden = excl, star, hidden
			a.opts = append(a.opts, opt)
		default:
			r.Diagnosef("invalid argument: %s\n", spec)
			return false
		}
	}
	return true
}

// specPrefixes takes the three things that may stand in front of any spec:
// the `!` that keeps a spec off the offering, the `(…)` exclusion list, and
// the `*` that makes it repeatable — **in that order**, which is the order
// the shipped specs are written in (`(-)*:: :->option-or-argument`,
// `!(--no-guess)--guess`).
//
// The `!` goes *before* the list and not after it, and that is measured
// rather than chosen. On zsh 5.9.2, 2026-09-16, asking `comparguments -i`
// from inside a widget with `-y[why]` beside each:
//
//	!(-y)-x      accepted; `-x` is understood, never offered, and `-y` is
//	             gone from the offering once `-x` is on the line
//	(-y)!-x      refused — `invalid argument: (-y)!-x`
//	!-x          accepted, with no exclusion list
//	*!(-y)-x     refused — `invalid rest argument definition`
//
// Reading the list first is what refused `!(--no-guess)--guess`, and the
// refusal is *printed*: `git checkout <TAB>` scribbled
// `_arguments:comparguments:327: invalid argument: !(--no-guess)--guess`
// over the line, twice, before offering anything.
func specPrefixes(spec string) (excl []string, body string, star, hidden bool) {
	body = spec
	if strings.HasPrefix(body, "!") {
		hidden, body = true, body[1:]
	}
	if strings.HasPrefix(body, "(") {
		if end := strings.IndexByte(body, ')'); end >= 0 {
			excl = strings.Fields(body[1:end])
			body = body[end+1:]
		}
	}
	if strings.HasPrefix(body, "*") {
		star, body = true, body[1:]
	}
	return excl, body, star, hidden
}

// parseOptionSpec reads `-name`, the modifier that says where its first
// argument goes, the `[…]` explanation and the `:…` argument descriptions.
func parseOptionSpec(body string) (optionSpec, bool) {
	var o optionSpec
	name, rest := readSpecField(body, "[:")
	if name == "" {
		return o, false
	}
	name, o.style = splitOptionModifier(name)
	if name == "" || (name[0] != '-' && name[0] != '+') {
		return o, false
	}
	o.names = optionNames(name)
	if strings.HasPrefix(rest, "[") {
		if end := strings.IndexByte(rest, ']'); end >= 0 {
			o.descr, rest = rest[1:end], rest[end+1:]
		}
	}
	o.optargs = parseOptargs(rest)
	if len(o.optargs) > 0 && o.style == optArgNone {
		o.style = optArgSeparate
	}
	return o, true
}

// splitOptionModifier takes the trailing character that says where the first
// argument may be written. `=-` is read before `-` and `=`, because it is
// both of them and the longest reading is the right one.
func splitOptionModifier(name string) (string, optionArgStyle) {
	switch {
	case strings.HasSuffix(name, "=-"):
		return name[:len(name)-2], optArgEqualDirect
	case strings.HasSuffix(name, "="):
		return name[:len(name)-1], optArgEqual
	case strings.HasSuffix(name, "+"):
		return name[:len(name)-1], optArgOptDirect
	case len(name) > 2 && strings.HasSuffix(name, "-"):
		return name[:len(name)-1], optArgDirect
	}
	return name, optArgNone
}

// optionNames is the one or two names a spec describes: `-+foo` and `+-foo`
// are both `+foo` and `-foo`, and everything else is itself.
//
// **The `+` spelling comes first**, which is measured rather than chosen —
// it is the order the two arrive in `-O`'s arrays. On zsh 5.9.2, 2026-09-16
// from inside a `zle -C` widget with `-+a[plus]`, `-+b[bee]`, `-o[opt]:val:`
// and `-p[proc]` declared:
//
//	cmd -o val foo<TAB>   next=(+a:plus -a:plus +b:bee -b:bee -p:proc)
//	cmd +a<TAB>           next=(-a:plus +b:bee -b:bee -o:opt -p:proc)
//
// so the specs are in declaration order and each pair is `+` then `-`. The
// second row is what says the pair is two names and not one: `+a` is spent
// and `-a` is still offered.
func optionNames(name string) []string {
	if len(name) > 2 && (strings.HasPrefix(name, "-+") || strings.HasPrefix(name, "+-")) {
		return []string{"+" + name[2:], "-" + name[2:]}
	}
	return []string{name}
}

// parseOptargs reads the `:message:action` groups that follow an option name.
func parseOptargs(rest string) []argumentSpec {
	var out []argumentSpec
	for strings.HasPrefix(rest, ":") {
		var arg argumentSpec
		rest = rest[1:]
		for strings.HasPrefix(rest, ":") {
			arg.optional, rest = true, rest[1:]
		}
		field, after := readSpecField(rest, ":")
		rest = after
		if strings.HasPrefix(field, "*") {
			// `:*pattern:message:action` — the pattern says how far the
			// repetition runs, and the message and action follow it.
			field, rest = readSpecField(strings.TrimPrefix(rest, ":"), ":")
		}
		arg.message = field
		if strings.HasPrefix(rest, ":") {
			arg.action, rest = readSpecField(rest[1:], ":")
		}
		out = append(out, arg)
	}
	return out
}

// parseArgumentSpec reads `:message:action` at the given position.
func parseArgumentSpec(body string, position int) argumentSpec {
	specs := parseOptargs(body)
	if len(specs) == 0 {
		return argumentSpec{position: position}
	}
	arg := specs[0]
	arg.position = position
	return arg
}

// readSpecField reads up to the first unescaped character in stop, and hands
// back the field with `\:` written as the colon it stands for. A literal
// colon in a message or an action is backslashed, which is the manual's rule
// and the reason this cannot be a Cut.
func readSpecField(s, stop string) (string, string) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == ':' {
			b.WriteByte(':')
			i++
			continue
		}
		if strings.IndexByte(stop, s[i]) >= 0 {
			return b.String(), s[i:]
		}
		b.WriteByte(s[i])
	}
	return b.String(), ""
}

// leadingNumber is the argument number a spec opens with, and the rest of it.
func leadingNumber(s string) (int, string) {
	n := 0
	i := 0
	for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n, s[i:]
}

// itoa is strconv.Itoa for the one place a tag name needs it.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// boolStatus is the shell's own truth: 0 for yes.
func boolStatus(yes bool) int {
	if yes {
		return 0
	}
	return 1
}
