// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"sort"
	"unicode/utf8"
)

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

	// Point says Style is written at Start and nothing else: no run, and no
	// reset of this package's after it. End is ignored.
	//
	// For a highlighter that decides every transition itself, which is the
	// shape zsh's `region_highlight` has: what goes out where one region ends
	// inside another depends on both, and a run with a fixed reset after it
	// cannot say that. A highlighter answering in points owns the attributes
	// it leaves on at the end of the line too — the reset rule above is the
	// default, not a guarantee it overrides by accident.
	Point bool
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

// BufferHighlighter is a Highlighter that is also told where the line ends
// inside the text it is handed.
//
// The text is what is **displayed** — the line and then any postdisplay, see
// styled — and for most highlighters that is all there is to know: a run is
// where it is. One that keeps state about the line between keystrokes is
// different, because a postdisplay is not an edit. zsh's `region_highlight`
// moves its offsets when the editor edits the line (#5874), and told only the
// displayed text it read a suggestion appearing after the cursor as an insert
// there and moved every offset past it off the text it was set for (#6412).
//
// start and end are where the line is within displayed, in bytes:
// displayed[start:end] is the line, anything before it is a predisplay and
// anything after it a postdisplay, neither of them part of the line. The
// start is there because zsh counts `region_highlight` from the line's start
// and not the displayed text's, unless an element says otherwise (#6414). The
// editor calls this in place of Highlight when it is there.
type BufferHighlighter interface {
	Highlighter
	HighlightBuffer(displayed string, start, end int) []Highlight
}

// highlightReset ends a run: SGR with no parameters, which is every attribute
// back to the terminal's default.
const highlightReset = "\x1b[0m"

// styleRun is one run of the line drawn differently: where it is, in bytes,
// and what is written on each side of it.
//
// Both sides, because the two things that produce one end it differently. A
// highlighter's run ends in a full reset, which is this package's choice and
// documented on Highlight.Style. A run that arrived as a paste ends by turning
// off the one attribute it turned on — measured, that is what both shells that
// mark a paste write, and matching them is the whole point of asking.
type styleRun struct {
	start, end int
	on, off    string
	// point is Highlight.Point: on is written at start and the run is empty.
	point bool
}

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
// The text it colors is what is **displayed** rather than the line, which is
// what lets a run reach into a predisplay or a postdisplay: measured,
// `region_highlight` with offsets past `$#BUFFER` colors the suggestion after
// it, so the offsets count one text and not two (#4217). A session with
// neither hands over exactly the line, which is every session in every dialect
// but one.
func (e *editor) styled() string {
	line := string(e.displayed())
	runs := e.styleRuns(line)
	if len(runs) == 0 {
		return hideControls(line)
	}
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].start < runs[j].start })

	var b []byte
	at := 0
	for _, r := range runs {
		if r.point {
			if r.start < at || r.start > len(line) || r.on == "" {
				continue
			}
			b = append(b, hideControls(line[at:r.start])...)
			b = append(b, e.guardedCodes(r.on)...)
			at = r.start
			continue
		}
		if r.start < at || r.end <= r.start || r.end > len(line) || r.on == "" {
			continue
		}
		b = append(b, hideControls(line[at:r.start])...)
		b = append(b, e.guardedCodes(r.on)...)
		b = append(b, hideControls(line[r.start:r.end])...)
		b = append(b, e.guardedCodes(r.off)...)
		at = r.end
	}
	if len(b) == 0 {
		// Nothing was applied, so nothing was copied.
		return hideControls(line)
	}
	// Each piece of the line is parked as it is copied rather than the whole
	// line before the runs are cut, because the runs are byte offsets into
	// the line as it is — the offsets a highlighter and `region_highlight`
	// count — and parking a character changes its length. See
	// controlglyph.go.
	return string(append(b, hideControls(line[at:])...))
}

// styleRuns is every run to be drawn differently: what the paste left marked,
// and what the highlighter asked for.
//
// The paste first, so that a tie in where two runs start goes to it — the sort
// above is stable. A highlighter run and a paste over the same text is a
// terminal being told two things about one cell, and the rule for that is the
// one styled already states.
//
// It returns nil for the case that is every keystroke of every ordinary
// session: no highlighter, and nothing pasted on the line.
func (e *editor) styleRuns(line string) []styleRun {
	var runs []styleRun
	// Where the line starts in what is displayed: past a predisplay, which
	// every offset into the line has to be moved over. Zero for every
	// session without one. Counted as displayed spells it, which is what
	// line is.
	pre := 0
	if e.predisplay != "" {
		pre = runesBytes([]rune(e.predisplay))
	}
	if e.pastedTo > e.pastedFrom && e.pastedTo <= len(e.line) && e.pastedStyle != "" {
		// Positions in the line are runes and a run is bytes, which is the
		// same conversion Highlight documents for a highlighter working in
		// runes — done here rather than asked of the caller, because the
		// caller is this package.
		runs = append(runs, styleRun{
			start: pre + len(string(e.line[:e.pastedFrom])),
			end:   pre + len(string(e.line[:e.pastedTo])),
			on:    e.pastedStyle,
			off:   e.pastedStyleEnd,
		})
	}
	if e.highlighter == nil {
		return runs
	}
	var asked []Highlight
	shift := 0
	if bh, ok := e.highlighter.(BufferHighlighter); ok {
		asked = bh.HighlightBuffer(line, pre, pre+runesBytes(e.line))
	} else {
		// A highlighter that knows only one text is handed the line and
		// what follows it, as it was before there was a predisplay, and its
		// runs are moved over the predisplay: a quote inside text a widget
		// drew is not one in the line.
		asked, shift = e.highlighter.Highlight(line[pre:]), pre
	}
	for _, r := range asked {
		if r.Point {
			runs = append(runs, styleRun{start: shift + r.Start, end: shift + r.Start, on: r.Style, point: true})
			continue
		}
		runs = append(runs, styleRun{start: shift + r.Start, end: shift + r.End, on: r.Style, off: highlightReset})
	}
	return runs
}

// runesBytes is len(string(rs)) without building the string: the length of
// the line in bytes, as the displayed text spells it, on the path every
// keystroke takes. A rune that is not one is written as U+FFFD, three bytes,
// which is what the conversion writes for it.
func runesBytes(rs []rune) int {
	n := 0
	for _, r := range rs {
		if l := utf8.RuneLen(r); l > 0 {
			n += l
		} else {
			n += utf8.RuneLen(utf8.RuneError)
		}
	}
	return n
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
