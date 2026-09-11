// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// `set -x` printing.
//
// The structure is unanimous: every simple command is written to stderr
// before it runs, with its words already expanded, and compound commands are
// not traced — only the simple ones inside them. Everything else about it is
// decoration, and all four shells decorate differently.

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

// traceAssignments writes the trace line for a command that is only
// assignments.
//
// The *value* is quoted and the name is not: every shell that quotes at all
// writes `x='hello wor'` rather than `'x=hello wor'`, which reads as a
// command name with a space in it.
func (r *Runner) traceAssignments(assigns []*syntax.Assign, values []string) {
	if !r.xtrace || len(assigns) == 0 {
		return
	}
	r.awaitTraceTurn()
	defer r.releaseTraceTurn()
	d := r.diag()
	words := make([]string, len(assigns))
	for i, a := range assigns {
		words[i] = a.Name + "=" + traceQuote(values[i], d.TraceQuoting)
	}
	if len(words) > 1 && r.ask(r.sem().TraceAssignmentsSeparately, "each assignment getting its own trace line") {
		for _, w := range words {
			r.traceLine(w, d)
		}
		return
	}
	r.traceLine(strings.Join(words, " "), d)
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
// drawing one would be inventing it.
func (r *Runner) renderTracePrefix(v string) string {
	if v == "" {
		return ""
	}
	st := r.promptStyle
	expand := func() {
		if st.Expand != nil && st.Expand(r) {
			v = r.Expand(v)
		}
	}
	escapes := func() {
		if out, _, ok := ExpandPromptStyle(st, v, r.tracePromptField, r.promptQuantity); ok {
			v = out
		}
	}
	if st.ExpandBeforeEscapes {
		expand()
		escapes()
	} else {
		escapes()
		expand()
	}
	return v
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
