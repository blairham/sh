// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme

import "strings"

// The layout pass, which is the part that makes a preset data.
//
// There is one algorithm here, per side of each line:
//
//	for each segment that rendered:
//	  emit a separator, whose colors are decided by whether this
//	    segment's background differs from the previous one
//	  emit whitespace, icon and content in the segment's own colors
//	emit the side's end symbol in the last background
//
// With no backgrounds and the separator set to a space, that loop produces a
// lean two-line prompt. With a background per segment and the separator set to
// a powerline arrow, **the same loop** produces a framed one. That genericity
// is why a new look costs data and no code, and it is the single most
// important structural decision in the engine — so the test that matters here
// drives one loop from two configurations and expects both.

// Screen is what the engine borrows from whatever is drawing it.
//
// Both are functions rather than something this package implements, for the
// reason the spec gives for width: the editor already measures cells with the
// generated East Asian tables, and a second reader of the same question is how
// a fix lands in one of them and not the other.
type Screen struct {
	// Width is how many cells a rendered string occupies, with escape
	// sequences discounted. A nil Width means the engine cannot measure, and
	// it then declines to place anything that needs a measurement rather than
	// guessing.
	Width func(string) int

	// Mark wraps a run of escape bytes so the editor's arithmetic is not
	// charged for cells they do not occupy.
	Mark func(escapes string) string
}

// Prompt is a rendered prompt.
type Prompt struct {
	// Text is the whole left prompt, newlines and all. The last line is the
	// one being typed on.
	Text string

	// Right is the right prompt for the line being typed on. It is separate
	// from Text so the editor can hide it when the typed line grows into it,
	// rather than it being baked into text that would then wrap.
	Right string

	// Cont is the prompt for the rest of an unfinished construct.
	Cont string
}

// Engine draws a prompt from a configuration.
type Engine struct {
	// Settings is the configuration, and Roster resolves an element name to
	// the segment that draws it.
	Settings *Settings
	Roster   *Roster

	// Screen is what the engine borrows from the editor.
	Screen Screen

	// Icons resolves a segment's icon key to a glyph. A nil Icons draws no
	// icons, which is what a configuration that asked for none should get.
	Icons func(key string) (string, bool)

	// Bare is what a prompt with no elements configured draws, and Continued
	// is its continuation. They come from the caller because the character a
	// shell prompts with is the caller's business: this package names no
	// shell, and a default prompt that is fast and plain is a better first
	// minute than a rich one that needs a file.
	Bare      string
	Continued string
}

// promptGap is the single space between the end of the left prompt and
// whatever the person types.
//
// It is deliberately not any of the settings it looks like, and it is not
// routed through one: a whitespace setting that emptied it would produce a
// prompt that cannot be read. It belongs to the line being typed on alone —
// banner lines end where their content ends, and a trailing space there would
// fight the gap that places their right side.
const promptGap = " "

// Render draws one prompt.
func (e *Engine) Render(ctx *Context) Prompt {
	cont := e.Settings.Str("CONTINUATION", e.Continued)
	left := elementLines(e.Settings.List("LEFT_ELEMENTS"))
	right := elementLines(e.Settings.List("RIGHT_ELEMENTS"))

	lines := max(len(left), len(right))
	if lines == 0 {
		return Prompt{Text: e.Bare, Cont: cont}
	}

	rows := make([]string, lines)
	rightPrompt := ""
	for i := range lines {
		leftText := e.side(sideLeft, lineAt(left, i), ctx)
		rightText := e.side(sideRight, lineAt(right, i), ctx)
		prefix := e.expand(Style{}, nil, e.Settings.Str(framePrefix(i, lines), ""))
		suffix := e.expand(Style{}, nil, e.Settings.Str(frameSuffix(i, lines), ""))

		if i == lines-1 {
			// The last line is the one being typed on.
			rows[i] = prefix + leftText + promptGap
			rightPrompt = rightText + suffix
			continue
		}
		rows[i] = e.banner(prefix+leftText, rightText+suffix, ctx.Columns)
	}

	text := strings.Join(rows, "\n")
	if e.Settings.Bool("ADD_NEWLINE", false) {
		text = "\n" + text
	}
	return Prompt{Text: text, Right: rightPrompt, Cont: cont}
}

// banner places the two halves of a line that is not being typed on at
// opposite edges, filling the middle.
//
// When the width is unknown or the halves would collide, the right half is
// dropped rather than wrapped. A wrapped frame line corrupts every repaint
// after it, which is the same reason the editor hides a right prompt rather
// than letting it push the line around.
func (e *Engine) banner(left, right string, columns int) string {
	if right == "" || e.Screen.Width == nil || columns <= 0 {
		// A width of zero is not a width. Gluing the right side on wraps the
		// line at whatever the real edge turns out to be, and a wrapped frame
		// line smears on every repaint after it.
		return left
	}
	gap := columns - e.Screen.Width(left) - e.Screen.Width(right)
	if gap < 1 {
		return left
	}
	filler := e.Settings.Str("GAP_CHAR", " ")
	if filler == "" {
		filler = " "
	}
	width := max(e.Screen.Width(filler), 1)
	style := Style{}
	if color, ok := ParseColor(e.Settings.Str("GAP_FOREGROUND", "")); ok {
		style.Fg = color
	}
	return left + e.paint(style, strings.Repeat(filler, gap/width)) + right
}

// side runs one line's segments on one side and composes what rendered.
func (e *Engine) side(s side, elements []string, ctx *Context) string {
	drawn := make([]placed, 0, len(elements))
	for _, element := range elements {
		segment, known := e.Roster.Resolve(element)
		if !known {
			// Named under "not yet" by the roster, and costing no space here:
			// an element with no segment renders nothing.
			continue
		}
		out, show := segment.Render(e.Settings, ctx)
		if !show {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(element))
		drawn = append(drawn, placed{
			name:  name,
			out:   out,
			style: e.Settings.SegmentStyle(name, out.State, Style{}),
		})
	}
	if len(drawn) == 0 {
		return ""
	}

	var b strings.Builder
	var previous Color
	for i, p := range drawn {
		b.WriteString(e.separator(s, i == 0, p, previous))
		b.WriteString(e.body(p))
		previous = p.style.Bg
	}
	b.WriteString(e.end(s, drawn[len(drawn)-1], previous))
	return b.String()
}

// placed is a segment that rendered, with its appearance resolved.
type placed struct {
	name  string
	out   Rendered
	style Style
}

// separator emits what goes before a segment: the side's start symbol for the
// first one, then either a subsegment separator — same background as the
// previous segment, so the boundary is a hairline — or a full segment
// separator, backgrounds differing, so the boundary is drawn from one
// background into the other.
func (e *Engine) separator(s side, first bool, p placed, previous Color) string {
	if first {
		symbol := e.Settings.Param(p.name, p.out.State, s.key("START_SYMBOL"), "")
		return e.paint(arrow(s, p.style.Bg, Color{}), symbol)
	}
	if previous == p.style.Bg {
		symbol := e.Settings.Param(p.name, p.out.State, s.key("SUBSEGMENT_SEPARATOR"), " ")
		style := Style{Bg: p.style.Bg}
		if color, ok := ParseColor(e.Settings.Param(p.name, p.out.State, "SUBSEGMENT_SEPARATOR_FOREGROUND", "")); ok {
			style.Fg = color
		}
		return e.paint(style, symbol)
	}
	symbol := e.Settings.Param(p.name, p.out.State, s.key("SEGMENT_SEPARATOR"), " ")
	return e.paint(arrow(s, p.style.Bg, previous), symbol)
}

// end closes a side in the last background.
func (e *Engine) end(s side, p placed, previous Color) string {
	symbol := e.Settings.Param(p.name, p.out.State, s.key("END_SYMBOL"), "")
	return e.paint(arrow(s, Color{}, previous), symbol)
}

// arrow colors a separator. On the left it is drawn in the previous
// background over the next one; on the right the relationship is mirrored,
// which is why the two sides need different glyphs as well as different
// colors.
func arrow(s side, next, previous Color) Style {
	if s == sideLeft {
		return Style{Fg: previous, Bg: next}
	}
	return Style{Fg: next, Bg: previous}
}

// body is the segment itself: whitespace, the content template, and the
// whitespace again, all in the segment's own colors.
//
// The template is the configuration's, not the segment's, and it reaches what
// the segment computed through ${CONTENT} and ${ICON}. That is what keeps a
// segment's text *text*: a directory holding a percent sign is drawn and not
// read as markup, because nothing a segment produces is ever expanded.
func (e *Engine) body(p placed) string {
	space := e.Settings.Param(p.name, p.out.State, "WHITESPACE", "")
	template := e.Settings.Param(p.name, p.out.State, "CONTENT", "${ICON}${CONTENT}")
	prefix := e.Settings.Param(p.name, p.out.State, "PREFIX", "")
	suffix := e.Settings.Param(p.name, p.out.State, "SUFFIX", "")

	icon := ""
	if e.Icons != nil && p.out.Icon != "" {
		if glyph, ok := e.Icons(p.out.Icon); ok {
			icon = glyph
		}
	}
	lookup := func(name string) string {
		switch name {
		case "CONTENT":
			return p.out.Content
		case "ICON":
			return icon
		case "STATE":
			return p.out.State
		}
		return ""
	}
	return e.expand(p.style, lookup, space+prefix+template+suffix+space)
}

// paint puts plain text in one appearance.
func (e *Engine) paint(style Style, text string) string {
	if text == "" {
		return ""
	}
	if style.Empty() {
		return text
	}
	return e.mark(style.SGR()) + text + e.mark(Reset)
}

// expand reads a value's markup in a segment's appearance.
func (e *Engine) expand(base Style, lookup func(string) string, value string) string {
	if value == "" {
		return ""
	}
	out := Expander{Lookup: lookup, Mark: e.Screen.Mark, Base: base}.Expand(value)
	if out == "" || base.Empty() {
		return out
	}
	return out + e.mark(Reset)
}

func (e *Engine) mark(escapes string) string {
	if e.Screen.Mark == nil || escapes == "" {
		return escapes
	}
	return e.Screen.Mark(escapes)
}

// side distinguishes the two halves of a line. The separator vocabulary is
// mirrored between them, so the names differ only in their first word.
type side int

const (
	sideLeft side = iota
	sideRight
)

func (s side) key(name string) string {
	if s == sideLeft {
		return "LEFT_" + name
	}
	return "RIGHT_" + name
}

// elementLines splits an elements list at each `newline`, which is how a side
// is given more than one line.
func elementLines(elements []string) [][]string {
	if len(elements) == 0 {
		return nil
	}
	lines := [][]string{{}}
	for _, element := range elements {
		if strings.EqualFold(strings.TrimSpace(element), "newline") {
			lines = append(lines, []string{})
			continue
		}
		lines[len(lines)-1] = append(lines[len(lines)-1], element)
	}
	return lines
}

// lineAt is the elements of one line of a side, or none where that side has
// fewer lines than the other.
func lineAt(lines [][]string, i int) []string {
	if i < 0 || i >= len(lines) {
		return nil
	}
	return lines[i]
}

// framePrefix and frameSuffix name the decoration for a line, which differs
// for the first line, the last, and any between.
func framePrefix(i, lines int) string {
	switch {
	case lines == 1 || i == lines-1:
		return "LAST_PREFIX"
	case i == 0:
		return "FIRST_PREFIX"
	default:
		return "MIDDLE_PREFIX"
	}
}

func frameSuffix(i, lines int) string {
	switch {
	case lines == 1 || i == lines-1:
		return "LAST_SUFFIX"
	case i == 0:
		return "FIRST_SUFFIX"
	default:
		return "MIDDLE_SUFFIX"
	}
}
