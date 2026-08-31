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
func (r *Runner) tracePrefix() string {
	if r.diag().TraceStyle == TraceNameLine {
		// Inside a function zsh names the function and reports line 0
		// rather than the line the call was on.
		if r.inFunc != "" {
			return "+" + r.inFunc + ":0> "
		}
		return "+" + r.name() + ":" + itoa(r.line) + "> "
	}
	return "+ "
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
