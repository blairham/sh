// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "sort"

// Coloring the line as it is typed.
//
// **This is in-process and it is never a plugin, and that is the point of it
// being a seam of its own.** A highlighter re-renders the whole buffer on
// every keystroke, so a cross-process one would put a round trip between a key
// and the character appearing. docs/design/plugins.md excludes hot paths in as
// many words — "a completion per keystroke… a round trip is microseconds and
// these happen thousands of times per line" — and this is the hottest of them:
// completion happens on Tab, highlighting happens on every key. Anyone reading
// this file with the idea of generalizing it into a plugin role should stop
// here; the seam it would be generalizing is the one that must not be.
//
// **What does the highlighting, measured before it was designed.** A full
// re-parse per keystroke is affordable, which is the number #803 said would
// decide the shape. On an Apple M5 Max, parsing one line with syntax.Core:
//
//	ls -la                                                     0.86 µs
//	git log --oneline --graph -20 | head -n 5 > /tmp/out.txt    3.5 µs
//	a 120-character for loop with a pipeline in it             7.7 µs
//	a 540-character line of twenty pipelines                    39 µs
//	echo "one two three   (unterminated)                       0.87 µs
//
// A person types perhaps ten characters a second, so even the 540-character
// case spends four ten-thousandths of the budget between two keystrokes.
// BenchmarkReparsingALineOnEveryKeystroke keeps the number honest.
//
// That answers the shape question: **the seam hands over the line and nothing
// else.** No pre-parsed tree, no incremental lexer, no cache to invalidate. A
// highlighter that wants the parser runs it, at a cost that does not show; one
// that wants something cheaper does something cheaper. Plumbing a parse
// through the seam would have committed every highlighter to one grammar and
// one moment of parsing, to save four microseconds nobody can perceive.

// Highlight is a run of the line drawn differently.
type Highlight struct {
	// Start and End are byte offsets into the line, as Completion's are, so
	// line[Start:End] is the run. A run that is empty, backwards, or off the
	// end of the line is dropped rather than a panic — a highlighter working
	// in runes and forgetting to convert produces exactly those.
	Start, End int

	// Style is written to the terminal immediately before the run. It is an
	// escape sequence, not a name: the front end supplying a highlighter knows
	// what it wants the terminal to do, and a table of names in between would
	// be a vocabulary this package would have to keep up with.
	//
	// What follows the run is this package's, and is a full reset — a run that
	// chose its own ending could leave the rest of the line in a color, which
	// is a line editor that gradually paints the screen.
	Style string
}

// Highlighter colors a line as it is typed.
//
// Called on every keystroke, on the editor's own goroutine, immediately before
// the line is drawn. Everything in the note at the top of this file applies:
// it is in-process, it is synchronous, and it is on the hottest path this
// package has.
//
// A highlighter that panics takes the shell, and unlike a prompt provider it
// is **not** guarded. That asymmetry is deliberate. A prompt is drawn between
// lines, where a diagnostic has somewhere to go and the person reads it; a
// highlight is drawn in the middle of redrawing the line, where there is
// nowhere to put a complaint that would not land on top of what is being
// typed, and swallowing it silently would leave a shell that stopped coloring
// with nothing to say why. A front end that wants a guard writes one — the
// seam is a Go interface and nothing constrains what an implementation does
// before it answers, which is the argument docs/design/sandboxing.md makes
// about a Gate that wants to call out to a supervisor.
type Highlighter interface {
	Highlight(line string) []Highlight
}

// HighlighterFunc adapts a function to Highlighter.
type HighlighterFunc func(string) []Highlight

// Highlight calls f.
func (f HighlighterFunc) Highlight(line string) []Highlight { return f(line) }

// highlightReset ends a run: SGR with no parameters, which is every attribute
// back to the terminal's default.
const highlightReset = "\x1b[0m"

// styled is the line as it goes to the terminal.
//
// The runs are applied in order of where they start, and a run overlapping one
// already applied is dropped: two colors over one character is not a thing a
// terminal can be told, so the first claim wins and the rule is stated rather
// than left to whichever order a map happened to produce.
//
// The cursor arithmetic elsewhere in this package is untouched by any of this,
// and that is why highlighting is safe to add here: place and cells count the
// *runes* of the line, and what this adds occupies no cells.
func (e *editor) styled() string {
	line := string(e.line)
	if e.highlighter == nil {
		return line
	}
	runs := e.highlighter.Highlight(line)
	if len(runs) == 0 {
		return line
	}
	runs = append([]Highlight(nil), runs...)
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].Start < runs[j].Start })

	var b []byte
	at := 0
	for _, r := range runs {
		if r.Start < at || r.End <= r.Start || r.End > len(line) || r.Style == "" {
			continue
		}
		b = append(b, line[at:r.Start]...)
		b = append(b, r.Style...)
		b = append(b, line[r.Start:r.End]...)
		b = append(b, highlightReset...)
		at = r.End
	}
	if at == 0 {
		// Nothing was applied, so nothing was copied.
		return line
	}
	return string(append(b, line[at:]...))
}

// UnclosedQuote colors the part of a line that is inside a quotation which has
// not been closed, and is the highlighter this package ships.
//
// This case rather than a general grammar coloring, because it is the one
// where the shell knows something the person does not. A quote left open turns
// the next Return into a continuation prompt instead of a command, and the
// only sign of it before that is a screen that looks exactly like a screen
// with the quote closed. Everything else a highlighter might color — a command
// word that resolves, a redirection, a keyword — is something already visible
// in the text.
//
// The run is the whole word carrying the quotation, from where the word begins
// to the end of the line. A word always begins unquoted — see wordSpan, which
// is the same scan completion uses to decide where a word starts — so the run
// is exactly the text the open quotation has swallowed.
//
// No dialect chooses the color, and no dialect chooses whether this is on.
// Measured: none of bash, zsh, dash or ksh93 colors a line as it is typed,
// which is what makes this not a dialect question at all — a dialect field is
// where the four *disagree*, and here there is nothing to disagree about. It
// is the front end's, and an empty Style is off.
type UnclosedQuote struct {
	// Style is the escape sequence written before the unclosed run. Empty
	// highlights nothing, which is what every real shell does.
	Style string
}

// Highlight colors the unclosed run, or nothing when the line has none.
func (u UnclosedQuote) Highlight(line string) []Highlight {
	if u.Style == "" {
		return nil
	}
	runes := []rune(line)
	start, quote := wordSpan(runes, len(runes))
	if quote == 0 {
		return nil
	}
	return []Highlight{{Start: len(string(runes[:start])), End: len(line), Style: u.Style}}
}
