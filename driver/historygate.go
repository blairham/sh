// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"fmt"
	"strings"

	"github.com/blairham/sh/internal/histexpand"
	"github.com/blairham/sh/internal/histjoin"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The history gate: how a shell that parses a program whole comes to expand
// `!!` a **physical line at a time**, which is the only way the feature works.
//
// # The shape of the problem
//
// bash expands each physical line as it reads it, and everything observable
// about the feature follows from that one fact. Measured 2026-09-16 on bash
// 5.3.20 and 3.2.57 from a script file, and again from `-c` and from standard
// input, which behave identically:
//
//   - `set -H` on line 2 affects line 3 and not line 2 — and `set -H; echo !!`
//     written on *one* line expands nothing, because the whole line was read
//     before any of it ran;
//   - a reference inside a function body is expanded when the body is
//     **read**, not when the function is called: `f() {` / `echo !!` / `}`
//     defines a function whose body is already the expansion;
//   - a reference on the far side of a line continuation is expanded, and the
//     echo to standard error shows that physical line alone;
//   - a quote crossing a line boundary carries: `echo 'a` / `!!` / `b'` leaves
//     the reference alone, `echo "a` / `!!` / `b"` expands it, and `echo 'a` /
//     `b' !!` expands the one *after* the quote closes;
//   - a here-document's body is not expanded, with a quoted delimiter or an
//     unquoted one;
//   - `eval`, `source` and a command substitution's own text are not expanded
//     — only the program the shell is reading.
//
// # What this is, and what was rejected
//
// driver.program parses a script's text as a whole: wholeProgram holds the
// source and nextLine hands the parser one *logical* line out of it. So there
// is no per-line read to hook, which is why #3093 landed the prompt route and
// left this one.
//
// Four designs were weighed.
//
//  1. **Expand the whole text up front.** Wrong twice over: it expands lines
//     the script may never reach, and it does it before the `set -H` that
//     turns the feature on has run.
//  2. **Expand inside the lexer.** The quoting rules are not the lexer's —
//     `echo "it's !!"` expands, so the apostrophe that opens a string for the
//     lexer opens nothing for this — and a reference has to be resolved
//     before any token is formed, because it can expand to a whole command.
//  3. **A pass over the parsed words.** Same objection from the other end: a
//     reference can expand to a multi-line command, which has to be parsed
//     after it is expanded and not before.
//  4. **Feed every script one physical line at a time**, which is what the
//     standard-input route already does. Correct, and rejected on risk: it
//     changes the read path of the most heavily tested route in the tree for
//     every script that never asks for the feature.
//
// So: the gate, which is (4) **installed the moment the feature is asked
// for**. A program keeps its whole-text parse until `set -o history` starts
// the list; at that point the text not yet run is handed to a gate, and from
// there the parser is fed one physical line at a time through the expander.
// A script that never writes the option never builds one, and its read path
// is byte for byte what it was.
//
// # Where the pieces come from
//
// The gate does not decide what a line begins inside — the parser does, and
// syntax.Parser.OpenQuote is its answer. Asking the expander to work it out
// from the previous line would have re-implemented the lexer, badly: `$'`,
// backquotes, a `'` inside a double-quoted string and a here-document body
// are four separate answers and the lexer already has all of them.
//
// The list is the dialect's, reached through interp.Runner.SetHistoryStore.
// For bash that is the same array `history` itself keeps, which is what makes
// `history -s planted` visible to a later `!!` — measured, it is.
type histGate struct {
	// rest is text already in hand and not yet handed to the parser, and
	// more is where the rest of the program comes from. A script file and a
	// `-c` string arrive whole, so rest holds all of it and more is nil; a
	// program on standard input arrives as it runs, so rest is short and more
	// is the reader the program was already using.
	rest string
	more func() (string, bool)

	// open is what the parser was still inside when it asked for this line,
	// as syntax.Parser.OpenQuote spells it. Set by program.fill — which
	// also sets it a second time inside one read, where a line ending in a
	// backslash makes the reader take the next one without going back to
	// the parser.
	open string
	// heredocExpands is the second half of open where it is a
	// here-document: the delimiter was not quoted, so the body is shell
	// text and the reader resolved a line continuation in it. Set by
	// program.fill beside open. See syntax.Parser.OpenHeredocExpands.
	heredocExpands bool
	// dialect is the grammar the lines are being read under, which is what
	// says whether a `#` opens a comment. Set by program.fill beside open,
	// because a builtin on one line can change it for the next.
	dialect syntax.Dialect
	// flush says the parser finished a logical line before asking for this
	// one, so what has been collected is a command and belongs in the list.
	flush bool

	// cur is the command being read, one physical line at a time. The rule
	// for joining those lines into the single entry a list holds is
	// internal/histjoin's, shared with `fc`'s editor road — which reaches
	// the same question through bash's same input stream.
	cur histjoin.Entry

	r *interp.Runner
	// report words a complaint about a reference the list does not hold, in
	// the shape a script's diagnostic has: measured, bash says `t.sh: line 3:
	// !nosuch: event not found` from a file and names itself on the `-c` and
	// standard-input routes, where the prompt route says `bash: !nosuch:
	// event not found` with no line at all.
	report func(line int, msg string) string
	// at is the physical line number of the line just handed over, which is
	// what that complaint names. Counted here because the parser is fed
	// expanded text and cannot be asked about a line it has not seen.
	at int
}

// Both of the things the gate writes go to the **script's** standard error
// rather than the front end's, and that is measured rather than assumed:
// `exec 2>file` earlier in the script captures the echo and the complaint
// alike, while a redirection on the command carrying the reference captures
// neither — the expansion happens while the line is being read, before
// anything that line says has taken effect.
func (g *histGate) errf(format string, args ...any) {
	_, _ = fmt.Fprintf(g.r.Err(), format, args...)
}

// next hands the parser the next physical line, expanded.
//
// It is program.more once the gate is installed, so every route into the
// parser goes through it and there is no second way in.
func (g *histGate) next() (string, bool) {
	if g.flush {
		g.flush = false
		g.record()
	}
	for {
		line, ok := g.take()
		if !ok {
			// The end of the program. What was collected is still a command
			// — the last one — and nothing else is going to ask for it.
			g.record()
			return "", false
		}
		body := strings.TrimSuffix(line, "\n")
		if g.open == "<<" {
			// A here-document's body, which is not shell text and is not
			// expanded: measured with a quoted delimiter and an unquoted one.
			// It still belongs to the command, and the list holds it.
			g.at++
			// keep reads `g.open` and takes the newline separator and the
			// trailing one from it, which is what a body line needs.
			g.keep(body)
			return line, true
		}
		if !g.r.HistoryRecording() {
			// `set +o history` part way through stops the expander as well
			// as the list, and that is measured rather than assumed: after
			// it, `echo !!` prints the two characters again, and a later
			// `set -o history` starts both again over the entries the list
			// still holds.
			//
			// So the two options are an AND held continuously and not a
			// sequence — which is the same reading the gate itself is built
			// on, where the list is what decides whether this route exists
			// at all.
			g.at++
			g.keep(body)
			return line, true
		}
		res, err := g.r.ExpandHistoryIn(body, quoteOf(g.open), g.r.HistoryEntries(), g.r.HistoryFirst())
		if err != nil {
			// A reference the list does not hold. bash complains, does not
			// run the line, leaves the status where the command before it put
			// it, and goes on — so the line is **dropped**, and dropped
			// before the parser ever sees it.
			//
			// Which is visible, and is why this hands back nothing rather
			// than a blank line in its place: measured, everything after a
			// dropped line is numbered as though the file had never held it.
			// `echo !nosuch` on line 3 followed by a second one on line 5 is
			// `line 3` and then `line 4`, a `$LINENO` on line 4 reads 3, and
			// a syntax error on line 5 is reported at line 4. The complaint
			// itself names the line the counter is *about* to reach, which is
			// the one line the file and the counter still agree on.
			g.errf("%s", g.report(g.at+1, g.r.HistoryExpansionRefusal(err)+"\n"))
			continue
		}
		if res.Changed {
			// The whole bargain of `!!` is that whoever wrote it sees what it
			// became. bash writes it before running the line, to standard
			// error.
			g.errf("%s\n", res.Line)
		}
		if res.Print {
			// `:p` shows the expansion and runs nothing, which is what makes
			// it the safe way to look at what a reference resolves to. The
			// line still joins the list — that is what lets the next line
			// recall it — and it is dropped from the parser's count exactly
			// as an unanswered reference is: measured, a `$LINENO` on the
			// line after a `:p` reads one less than the file's.
			g.keep(res.Line)
			if g.cur.Len() == 1 {
				// It stood on its own, so it is a command and the list takes
				// it now — there will be no later flush for it, because the
				// parser is handed nothing and hands back no command for the
				// front end to record it at. Inside a compound command it is
				// not a command: the lines around it are still being
				// collected, and recording there would put half a construct
				// in the list.
				g.record()
			} else {
				g.cur.SpaceNext()
			}
			continue
		}
		g.at++
		g.keep(res.Line)
		return res.Line + "\n", true
	}
}

// take is the next physical line of the program, or false at the end of it.
func (g *histGate) take() (string, bool) {
	for g.rest == "" {
		if g.more == nil {
			return "", false
		}
		text, ok := g.more()
		if !ok {
			g.more = nil
			return "", false
		}
		g.rest = text
	}
	if i := strings.IndexByte(g.rest, '\n'); i >= 0 {
		line := g.rest[:i+1]
		g.rest = g.rest[i+1:]
		return line, true
	}
	line := g.rest
	g.rest = ""
	return line, true
}

// keep adds one physical line to the command being read, with what the parser
// was still inside when it asked for it.
func (g *histGate) keep(line string) {
	g.cur.Add(line, histjoin.At{
		Open:           g.open,
		HeredocExpands: g.heredocExpands,
		Dialect:        g.dialect,
	})
}

// record puts the command that has been collected into the list.
func (g *histGate) record() {
	if g.cur.Len() == 0 {
		return
	}
	entry := g.cur.Take()
	if !g.r.HistoryRecording() {
		return
	}
	g.r.RecordHistoryEntry(entry)
}

// quoteOf reads the lexer's answer as the expander's.
//
// Only two of the spellings silence anything, and both are a single quote:
// `'` and the `$'` of an ANSI-C string, which the scanner closes at the same
// character. A double quote is carried because the *state* matters to the
// scanner even though expansion happens inside one either way — a `'` written
// inside a double-quoted string opens nothing, and a line seeded wrongly would
// have it open a string that never closes. Everything else — `${`, `$((`,
// `$[`, a backquote, an unfinished pattern — is ordinary text as far as this
// is concerned, and measured: `x=$(echo` / `!!)` expands.
func quoteOf(open string) histexpand.Quote {
	switch open {
	case "'", "$'":
		return histexpand.InSingleQuotes
	case `"`:
		return histexpand.InDoubleQuotes
	}
	return histexpand.Unquoted
}
