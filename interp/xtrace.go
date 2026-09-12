// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// `set -x` printing.
//
// Every simple command is written to stderr before it runs, with its words
// already expanded. That much is unanimous. What is *not* true, and was
// asserted here until #2126, is that compound commands are never traced:
// every shell that has `[[ … ]]` prints it, every shell that has `(( … ))`
// prints that, bash and zsh each print something for `case`, and the three
// parts of a `for ((;;))` header are traced as arithmetic commands of their
// own. The rule that holds is narrower — a `while`, `until` or `if` header,
// a `( )` subshell and a `{ }` group are printed by nobody.
//
// The difference is not cosmetic. A trace that drops a whole command kind
// reads as a script that did not reach those lines, which is how a gap in
// gitstatus's own xtrace log became a wrong diagnosis; and `emulate -L sh`
// legitimately turning xtrace off for the rest of a function means some gaps
// really are real, so a spurious one has cover.
//
// Everything else about it is decoration, and all four shells decorate
// differently — fifteen divergences at the last count, which
// docs/spec/semantics.md enumerates.

// TraceStyle is how a shell introduces a traced command.
type TraceStyle int

const (
	// TracePlain is `+ `, which dash, bash and ksh93 all use.
	TracePlain TraceStyle = iota
	// TraceNameLine is zsh's `+zsh:1> `, naming the script and the line —
	// and the function and 0 when inside one.
	TraceNameLine
)

// TraceForHeader is what a `for` loop prints at each iteration under
// `set -x`. Three answers, all measured, which is why it is a type of its own
// rather than a bool: dash and ksh93 print nothing and go straight to the
// body, bash reprints the header as written, and zsh prints neither but shows
// the assignment the iteration performed.
type TraceForHeader int

const (
	// TraceForNone prints nothing for the loop itself, only its body. dash
	// and ksh93, and the substrate's own.
	TraceForNone TraceForHeader = iota
	// TraceForSource reprints the header as written, once per iteration:
	// `+ for i in $x` — unexpanded, quotes and all. bash.
	TraceForSource
	// TraceForAssign prints the assignment the iteration made, `+ i=1`,
	// which is the one thing the header does not say. zsh.
	TraceForAssign
)

// TraceArrayLiteral is how `set -x` renders the parenthesized list of
// `a=(1 2)`.
//
// Measured 2026-09-11 with `set -x; a=(1 2)`:
//
//	bash 5.3.15, bash 3.2.57	a=(1 2)
//	ksh93, zsh 5.9.2        	a=( 1 2 )
//	dash                    	no array syntax
//
// It is the words and not the source text, which is what makes one rule
// rather than a slice of the script: `a=(1    2` with `3)` on the next line
// traces `a=(1 2 3)` in bash and `a=( 1 2 3 )` in the other two, so every
// shell that has the construct rebuilds the list from the elements and only
// the spacing inside the parentheses is in dispute.
//
// Decoration, so it is on Diagnostics rather than on Semantics for the
// reason TraceStyle and TraceQuoting are: it decides what is *written* and
// not what happens.
type TraceArrayLiteral int

const (
	// TraceArrayTight writes the list with nothing between the parentheses
	// and the elements: bash, and the substrate's own.
	TraceArrayTight TraceArrayLiteral = iota
	// TraceArraySpaced writes a space inside each parenthesis: ksh93 and
	// zsh, which trace `a=( 1 2 )` and an empty literal as `a=( )`.
	TraceArraySpaced
)

// TraceCondition is how a shell traces `[[ … ]]`.
//
// Measured 2026-09-12 on bash 5.3.15, bash 3.2.57, ksh93 and zsh 5.9.2 with
// `set -x; [[ -n a && -n b && -n c ]]`. Every shell that has the construct
// traces it — which is the whole of #2126, since we traced none of it — and
// they part company only on how many lines one condition is worth:
//
//	bash, bash 3.2, ksh93	three lines, `[[ -n a ]]` `[[ -n b ]]` `[[ -n c ]]`
//	zsh                  	one line, `[[ -n a && -n b && -n c ]]`
//
// Both print only what was *evaluated*: `[[ -n a || -n b ]]` is one line in
// every column, because the right-hand operand never ran. And both drop a
// `( )` group — `[[ ( -n a ) && -n b ]]` traces without the parentheses
// everywhere — while `!` stays attached to the primary it negates.
type TraceCondition int

const (
	// TraceCondPrimary writes one line per primary as it is evaluated: bash,
	// ksh93, and the substrate's own.
	TraceCondPrimary TraceCondition = iota
	// TraceCondWhole writes one line for the condition once it has finished,
	// holding the primaries that were reached and the operators between
	// them: zsh.
	TraceCondWhole
)

// TraceArithSpelling is how a traced arithmetic expression is wrapped.
//
// Measured 2026-09-12 with `set -x; ((n))` and `set -x; (( n + 1 ))`, which
// separate the two readings: a shell that adds a space of its own writes
// `((  n + 1  ))` for the second, and one that reprints the text between the
// parentheses writes `(( n + 1 ))`.
//
//	bash 5.3.15, bash 3.2.57, zsh 5.9.2	`(( n ))`	`((  n + 1  ))`
//	ksh93                              	`((n))`  	`(( n + 1 ))`
//
// The text is the expression *after* expansion and before evaluation —
// `n=3; (( $n + 1 ))` traces `3 + 1` in all three — so the trace is written
// from the string the evaluator was handed rather than from the source, and
// nothing is expanded a second time to print it.
type TraceArithSpelling int

const (
	// TraceArithSpaced writes `(( ` and ` ))` around the text: bash, zsh, and
	// the substrate's own.
	TraceArithSpaced TraceArithSpelling = iota
	// TraceArithTight writes `((` and `))` with nothing added: ksh93.
	TraceArithTight
	// TraceArithBare writes the text with no parentheses at all. zsh's answer
	// for the three parts of `for ((init; cond; post))` and for nothing else:
	// the same shell wraps a `(( ))` *command* in spaced parentheses, so the
	// two sites are separate fields rather than one.
	TraceArithBare
)

// TraceCaseHeader is what `case` prints under `set -x`.
//
// Three answers, the same shape `for` divides into and measured the same way
// — 2026-09-12 with `set -x; y="a b"; case $y in "a b") : ;; esac`:
//
//	dash, ksh93	nothing at all; only the commands in the arm that ran
//	bash       	`case $y in`, once, as written and unexpanded
//	zsh        	`case a b (a\ b)`, once per arm it tries, with the subject
//	           	expanded and the arm's patterns joined by ` | `
//
// zsh stops at the arm that matched, so the line count says how far down the
// arms the subject got — which is the half of the trace a reader of a third
// party log is actually using it for.
//
// The subject is printed bare — `a b`, unquoted, even holding a space — and
// the patterns come from the strings the matcher was handed, so a
// metacharacter that was quoted carries a backslash. zsh escapes a quoted
// *space* there as well, which is recorded in the corpus and not reproduced;
// see xtrace/case-header-diverges.
type TraceCaseHeader int

const (
	// TraceCaseNone prints nothing for the construct, only the commands in
	// the arm that ran: dash, ksh93, and the substrate's own.
	TraceCaseNone TraceCaseHeader = iota
	// TraceCaseSource prints the header as written, once: bash.
	TraceCaseSource
	// TraceCaseArm prints the expanded subject and the patterns of each arm
	// as it is tried: zsh.
	TraceCaseArm
)

// TraceQuoting is how a shell renders a word that needs quoting.
type TraceQuoting int

const (
	// QuoteNever prints the expanded word as it is: dash. `echo 'a b'`
	// traces as `+ echo a b`, which cannot be told from two arguments.
	QuoteNever TraceQuoting = iota
	// QuoteShell single-quotes anything with a space or a quote in it, and
	// falls back to `$'…'` for a control character: bash and zsh.
	QuoteShell
	// QuoteDollar is ksh93, which reaches for `$'…'` for an embedded quote
	// where bash and zsh write `'it'\''s'`.
	QuoteDollar
)

// traceCommand writes one command's trace line.
func (r *Runner) traceCommand(words []string) {
	if !r.xtrace || len(words) == 0 {
		return
	}
	r.awaitTraceTurn()
	defer r.releaseTraceTurn()
	if r.disablesTrace(words) && !r.ask(r.sem().TraceShowsItsOwnDisabling, "whether `set +x` traces itself") {
		// ksh93 applies the change before printing the command that makes
		// it, so `set +x` leaves no trace of itself. The other three print
		// it and then stop.
		return
	}
	d := r.diag()
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = traceQuote(w, d.TraceQuoting)
	}
	r.errf("%s%s\n", r.tracePrefix(), strings.Join(quoted, " "))
}

// traceAssignments writes one trace line for a run of assignments.
//
// The *value* is quoted and the name is not: every shell that quotes at all
// writes `x='hello wor'` rather than `'x=hello wor'`, which reads as a
// command name with a space in it.
//
// How many assignments a line holds is not decided here. A shell that writes
// one line each writes it as soon as that value is known, so the split is about
// *when* as much as about how many, and only the caller performing them knows
// when — see Runner.assignAll.
func (r *Runner) traceAssignments(assigns []*syntax.Assign, values []string) {
	if !r.xtrace || len(assigns) == 0 {
		return
	}
	r.awaitTraceTurn()
	defer r.releaseTraceTurn()
	d := r.diag()
	words := make([]string, len(assigns))
	for i, a := range assigns {
		words[i] = traceAssign(a, values[i], d)
	}
	r.traceLine(strings.Join(words, " "), d)
}

// traceAssign spells one assignment: the target as the script wrote it, and
// then the value.
//
// The *target* comes from the tree and the *value* from the expansion, which
// is the split every shell that has these constructs makes. An element write
// keeps its subscript, an append keeps its `+=`, and an array literal keeps
// its elements — `a=(1 2)`, `a[0]=z` and `x+=b` traced as `a=”`, `a=z` and
// `x=b` while this was built from the name and one scalar, which is three
// different wrong answers to "what did the script do" and no diagnostic
// (#1937).
//
// Unexpanded is a fact about the subscript and not a shortcut. Measured
// 2026-09-11 with `i=2; a[$i]=hello`: bash 5.3.15, bash 3.2.57 and zsh 5.9.2
// all trace `a[$i]=hello`, where ksh93 traces `a[2]=hello` — so two of the
// three print what was typed, and printing what it came to is one shell's
// answer rather than the rule. ksh93's reading is recorded in the corpus and
// deliberately not modeled: evaluating the subscript here would evaluate it a
// second time, with `a[$((i++))]=v` incrementing twice, which is exactly the
// double run #1915 fixed for a scalar's value. Tracked as #1959.
func traceAssign(a *syntax.Assign, value string, d Diagnostics) string {
	var b strings.Builder
	b.WriteString(a.Name)
	if a.Index != nil {
		b.WriteString("[")
		b.WriteString(syntax.PrintWord(a.Index))
		b.WriteString("]")
	}
	if a.Append {
		b.WriteString("+")
	}
	b.WriteString("=")
	if a.IsArray {
		b.WriteString(traceArrayLiteral(a.Elems, d.TraceArrayLiteral))
		return b.String()
	}
	b.WriteString(traceQuote(value, d.TraceQuoting))
	return b.String()
}

// traceArrayLiteral renders `(1 2)` from the elements as they were written.
//
// From the words rather than from the value stored, because the trace stands
// in front of the assignment: bash traces `a=($(echo x))` and *then* runs the
// substitution, measured 2026-09-11, so the list it prints cannot be the one
// the name came to hold.
//
// ksh93 and zsh do print the expanded elements — `x='p q'; a=("$x" r)` traces
// `a=( 'p q' r )` in both — and that half is recorded in the corpus and not
// modeled here, for the reason traceAssign gives about the subscript:
// expanding the elements to print them would expand them twice. Tracked as
// #1959.
func traceArrayLiteral(elems []*syntax.Word, style TraceArrayLiteral) string {
	words := make([]string, len(elems))
	for i, w := range elems {
		words[i] = syntax.PrintWord(w)
	}
	joined := strings.Join(words, " ")
	if style != TraceArraySpaced {
		return "(" + joined + ")"
	}
	if joined == "" {
		return "( )"
	}
	return "( " + joined + " )"
}

// traceLine writes one trace line.
// traceForIteration prints what the dialect prints when a `for` loop takes
// another turn, which is a different thing in three of the four shells.
//
// It does not go through traceLine: zsh appends a space to an assignment that
// stands alone as a command and does not append one here, and this records
// that rather than tidying it away.
// traceForNames is traceForIteration for a loop that may bind more than one
// name on a pass.
//
// The two wordings differ in how many lines a pass is worth, which is why this
// is here rather than a call in a loop: the shell that quotes the *header*
// writes it once per pass however many names were bound, and the shell that
// writes the assignments writes one line each — measured, `set -x; for a b (
// 1 2 3 4 ) { : }` in zsh 5.9.2 gives `a=1`, `b=2`, `:`, `a=3`, `b=4`, `:`.
// Calling the single-name form per name would have repeated the header.
func (r *Runner) traceForNames(header string, names, items []string, at int) {
	if !r.xtrace {
		return
	}
	if r.diag().TraceForHeader == TraceForSource {
		r.traceForIteration(header, "", "")
		return
	}
	for j, name := range names {
		value := ""
		if at+j < len(items) {
			value = items[at+j]
		}
		r.traceForIteration(header, name, value)
	}
}

func (r *Runner) traceForIteration(header, name, value string) {
	if !r.xtrace {
		return
	}
	d := r.diag()
	var line string
	switch d.TraceForHeader {
	case TraceForSource:
		line = header
	case TraceForAssign:
		if name == "" {
			// `for ((init; cond; post))` binds no name, so the shell that
			// reports the *assignment* an iteration made has nothing to
			// report and prints nothing — measured, zsh 5.9.2 traces the
			// three arithmetic parts and no iteration line at all. This
			// wrote `=''` once per pass, which reads as an assignment to a
			// nameless parameter.
			return
		}
		line = name + "=" + traceQuote(value, d.TraceQuoting)
	default:
		return
	}
	if line == "" {
		return
	}
	r.awaitTraceTurn()
	defer r.releaseTraceTurn()
	r.errf("%s%s\n", r.tracePrefix(), line)
}

// condTrace is what a `[[ … ]]` has traced so far.
//
// It exists because the two readings need different amounts of state: a shell
// that writes a line per primary needs none, and one that writes a line for
// the whole condition has to hold the primaries it reached until the
// condition is over. One accumulator serves both, because the line-per-primary
// reading is the same accumulator flushed at every primary.
type condTrace struct {
	// whole is TraceCondWhole: hold the parts and write one line at the end.
	whole bool
	// parts are the rendered primaries and the operators between them.
	parts []string
	// pending are the `!`s read since the last primary. They belong to the
	// primary they negate under both readings — `[[ ! -z a ]]` is one line in
	// every column — so they wait for it rather than standing as parts.
	pending []string
}

// beginConditionTrace starts tracing one `[[ … ]]` and returns the function
// that ends it.
//
// The previous tracer is restored rather than cleared, because an operand may
// hold a command substitution that runs a condition of its own: `[[ -n
// $(f) ]]` where `f` tests something is a condition inside a condition, and
// the inner one must not flush its primaries into the outer one's line.
func (r *Runner) beginConditionTrace() func() {
	if !r.xtrace {
		return func() {}
	}
	prev := r.condTrace
	t := &condTrace{whole: r.diag().TraceCondition == TraceCondWhole}
	r.condTrace = t
	return func() {
		r.condTrace = prev
		if t.whole && len(t.parts) > 0 {
			r.traceConditionLine(strings.Join(t.parts, " "))
		}
	}
}

// traceConditionPrimary records one evaluated test.
//
// It is called once the operands have been expanded and before the test is
// answered, which is where the shells put it: a substitution in an operand
// traces its own commands first, and the line reporting the test sits under
// them holding what they came to.
func (r *Runner) traceConditionPrimary(words ...string) {
	t := r.condTrace
	if t == nil {
		return
	}
	text := strings.Join(append(t.pending, words...), " ")
	t.pending = nil
	if t.whole {
		t.parts = append(t.parts, text)
		return
	}
	r.traceConditionLine(text)
}

// traceConditionNot records a `!`, which prints with the primary it negates.
func (r *Runner) traceConditionNot() {
	if t := r.condTrace; t != nil {
		t.pending = append(t.pending, "!")
	}
}

// traceConditionOp records a `&&` or `||` that the evaluation went *through*.
//
// Called on the way to the right-hand side rather than on the way in, so a
// short-circuited operator leaves nothing behind — which is what makes the
// line say what ran. The line-per-primary reading has nowhere to put it and
// drops it, as its shells do.
func (r *Runner) traceConditionOp(op string) {
	if t := r.condTrace; t != nil && t.whole {
		t.parts = append(t.parts, op)
	}
}

// traceConditionLine writes one `[[ … ]]` line.
func (r *Runner) traceConditionLine(text string) {
	r.awaitTraceTurn()
	defer r.releaseTraceTurn()
	r.errf("%s[[ %s ]]\n", r.tracePrefix(), text)
}

// traceCondOperand renders a condition operand that is a value rather than a
// pattern.
//
// Its own quoting rather than the command trace's, because bash uses two:
// `x="a b"; echo "$x"` traces `echo 'a b'` and `[[ $x == y ]]` traces
// `[[ a b == y ]]`, measured 2026-09-12 on 5.3.15. ksh93 and zsh quote in both
// places, in their own spellings.
func (r *Runner) traceCondOperand(s string) string {
	if s == "" {
		// Unanimous, and not the same answer the quoting gives on its own:
		// `[[ -z "" ]]` traces `[[ -z '' ]]` in bash 5.3.15, bash 3.2.57,
		// ksh93 and zsh 5.9.2 alike, including in the shell that quotes
		// nothing else here. An operand rendered as nothing at all would
		// leave `[[ -z ]]`, which is a condition no shell would accept.
		return "''"
	}
	return traceQuote(s, r.diag().TraceConditionQuoting)
}

// traceArithCommand writes the line for a traced arithmetic expression.
//
// The text is the expression as the evaluator received it — expanded, not the
// source — because that is what the shells print and because it is already in
// hand: expanding it again to print it would run a substitution in it twice.
func (r *Runner) traceArithCommand(text string, spelling TraceArithSpelling) {
	if !r.xtrace {
		return
	}
	r.awaitTraceTurn()
	defer r.releaseTraceTurn()
	var line string
	switch spelling {
	case TraceArithTight:
		line = "((" + text + "))"
	case TraceArithBare:
		line = text
	default:
		line = "(( " + text + " ))"
	}
	r.errf("%s%s\n", r.tracePrefix(), line)
}

// traceCaseHeader writes what `case` prints before it tries its arms.
func (r *Runner) traceCaseHeader(header string) {
	if !r.xtrace || r.diag().TraceCaseHeader != TraceCaseSource || header == "" {
		return
	}
	r.awaitTraceTurn()
	defer r.releaseTraceTurn()
	r.errf("%s%s\n", r.tracePrefix(), header)
}

// traceCaseArm writes what `case` prints for one arm it is about to try.
//
// The patterns are the strings the matcher was handed — expanded, with the
// parts that were quoted carrying a backslash, which is the rendering the one
// shell that prints them uses: `p='a*'; case $y in $p) …` traces `(a\*)`,
// where the same two characters written out trace `(a*)`. Taken from the very
// strings the match used, so that nothing is expanded twice.
func (r *Runner) traceCaseArm(subject string, patterns []string) {
	if !r.xtrace || r.diag().TraceCaseHeader != TraceCaseArm {
		return
	}
	r.awaitTraceTurn()
	defer r.releaseTraceTurn()
	r.errf("%scase %s (%s)\n", r.tracePrefix(), subject, strings.Join(patterns, " | "))
}

func (r *Runner) traceLine(line string, d Diagnostics) {
	if d.TraceStyle == TraceNameLine {
		// zsh puts a space after an assignment-only line and nowhere else.
		// Measured rather than reasoned about; it is decoration, and this
		// records it rather than tidying it away.
		line += " "
	}
	r.errf("%s%s\n", r.tracePrefix(), line)
}

// tracePrefix renders what comes before the command.
//
// `PS4` decides it wherever the shell has one, which is every shell in the
// panel — measured 2026-09-11, `PS4="XX "; set -x; :` traces `XX :` in bash
// 5.3.15, bash 3.2.57, dash, ksh93 and zsh alike, whether the parameter was
// assigned in the shell or inherited from the environment. This drew the
// built-in `+ ` in every case until #1454.
//
// Unset is the dialect's own prefix rather than nothing, which is what the
// shells start with: the parameter holds `+ ` before any script runs in three
// of them and a name-and-line form in the fourth. What a script's own
// `unset PS4` does — three draw nothing at all and ksh93 keeps `+ ` — needs
// that default to be a *parameter* this shell seeds as well, and is measured
// and deliberately not modeled; see docs/spec/invocation.md.
func (r *Runner) tracePrefix() string {
	if v, ok := r.getVar("PS4"); ok {
		return r.tracePrefixDepth(r.renderTracePrefix(v))
	}
	if r.diag().TraceStyle == TraceNameLine {
		// Inside a function zsh names the function and reports line 0
		// rather than the line the call was on.
		if r.inFunc != "" {
			return "+" + r.inFunc + ":0> "
		}
		return "+" + r.name() + ":" + itoa(r.line) + "> "
	}
	return r.tracePrefixDepth("+ ")
}

// renderTracePrefix turns the value of `PS4` into the text to write, the same
// way this dialect turns a prompt parameter's value into a prompt.
//
// **It is the prompt language**, and that is measured rather than assumed:
// with `PS4='<\u>'` bash 5.3.15 and bash 3.2.57 both trace `<bhamilton>`,
// ksh93 drops the backslash and traces `<u>`, dash has no table and traces
// `<\u>` unchanged, and zsh reads its own `%` escapes there instead —
// `PS4='%n '` traces the user name in zsh and the two characters everywhere
// else. Those are exactly each dialect's PromptStyle, so this asks the value
// the prompt drawer asks rather than growing a second table — #1090's rule,
// one reader further down.
//
// It is the only route to a prompt escape that needs no terminal, which is
// what makes it worth having beyond the trace itself: the corpus can reach
// it, where every other escape this shell draws is pinned only by a test
// that builds a session.
//
// The expansion is the same question again. Three shells expand `PS4` at
// every trace — `PS4='+$LINENO '` follows the line — and zsh does not unless
// a script has turned its prompt-substitution option on, which is precisely
// what PromptStyle.Expand answers and why that is a function rather than a
// bool.
//
// So this is the prompt drawer's render minus its history pass: whether a `!`
// in a trace prefix becomes the history number is not measured yet, and
// drawing one would be inventing it. Both passes and their order are
// [RenderPromptValue], which the drawer and `${v@P}` call too — a refusal
// there is left where it was written, because a diagnostic about a trace
// prefix belongs even less in the middle of a trace than the decoration does.
func (r *Runner) renderTracePrefix(v string) string {
	if v == "" {
		return ""
	}
	out, _, _ := RenderPromptValue(r.promptStyle, r, v, r.tracePromptField, r.promptQuantity)
	return out
}

// tracePromptField is the trace's resolver: the Runner's own answers, and the
// *drawer's* policy for a code it has none for.
//
// A prompt has to draw something and so does a trace prefix, where a script's
// `${(%)…}` refuses an escape it cannot answer by name. Refusing here would
// put a diagnostic about the decoration in the middle of the trace, so this
// falls through to whatever this dialect does with an escape that is in no
// table — which is measured and is three different things: bash keeps both
// characters, ksh93 drops the backslash and draws `u` for `\u`, and zsh drops
// the pair. See PromptStyle.Unknown, and repl's promptField, which is the
// same fallthrough one reader over.
func (r *Runner) tracePromptField(f PromptField, arg string, braced bool) (string, bool) {
	if v, ok := r.promptField(f, arg, braced); ok {
		return v, true
	}
	switch r.promptStyle.Unknown {
	case DropEscape:
		return arg, true
	case DropBoth:
		return "", true
	default:
		return string(r.promptStyle.Escape) + arg, true
	}
}

// tracePrefixDepth repeats the prefix's first character once per level of
// indirection, where the dialect does that.
//
// bash alone, and measured 2026-09-11: `set -x; eval :` traces `+ eval :` and
// then `++ :`, a second `eval` inside that reaches `+++ :`, and a sourced file
// and a command substitution each count as one level the same way. A function
// call and a subshell do not — `f(){ :; }; f` and `(:)` stay at one `+` — so
// what counts is *text being read again* rather than the depth of the stack.
//
// The first character of the prefix and not the whole of it: with `PS4='XY '`
// bash traces `XY eval :` and then `XXY :`.
func (r *Runner) tracePrefixDepth(prefix string) string {
	if r.indirection == 0 || prefix == "" || !r.diag().TracePrefixRepeatsAtIndirection {
		return prefix
	}
	first := []rune(prefix)[0]
	return strings.Repeat(string(first), r.indirection) + prefix
}

// traceQuote renders one expanded word the way the dialect would.
func traceQuote(s string, q TraceQuoting) string {
	if q == QuoteNever {
		return s
	}
	if s == "" {
		return "''"
	}
	if ctl := strings.IndexFunc(s, func(c rune) bool { return c < 0x20 }); ctl >= 0 {
		return dollarQuote(s)
	}
	if !strings.ContainsAny(s, " \t'\"$`\\|&;<>()") {
		return s
	}
	if strings.Contains(s, "'") && q == QuoteDollar {
		return dollarQuote(s)
	}
	// A single quote cannot appear inside single quotes, so it is closed,
	// escaped and reopened — which is what `'it'\''s'` is.
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// dollarQuote renders a word with `$'…'`, where a control character has a
// readable spelling.
func dollarQuote(s string) string {
	var b strings.Builder
	b.WriteString("$'")
	for _, c := range s {
		switch c {
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\'':
			b.WriteString(`\'`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(c)
		}
	}
	b.WriteString("'")
	return b.String()
}

// disablesTrace reports whether a command is the `set` that turns tracing off.
func (r *Runner) disablesTrace(words []string) bool {
	if len(words) == 0 || words[0] != "set" {
		return false
	}
	for _, w := range words[1:] {
		if strings.HasPrefix(w, "+") && strings.Contains(w, "x") {
			return true
		}
	}
	return false
}

// awaitTraceTurn blocks until the element before this one has traced.
//
// Pipeline elements run at the same time, so without this their trace lines
// come out in whatever order the scheduler chose — `+ cat` before `+ echo a`
// as often as not. Real shells print them left to right because they fork in
// that order, and a corpus cannot record a coin flip.
func (r *Runner) awaitTraceTurn() {
	if r.traceWait != nil {
		<-r.traceWait
	}
}

// releaseTraceTurn lets the next element print.
func (r *Runner) releaseTraceTurn() {
	if r.traceDone == nil || r.traceOnce == nil {
		return
	}
	r.traceOnce.Do(func() { close(r.traceDone) })
}
