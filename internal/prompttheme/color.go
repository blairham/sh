// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme

import (
	"strconv"
	"strings"
)

// Colors, in three spellings, because a person writing a configuration
// reaches for whichever they know and a preset carrying exact colors needs
// the precise one:
//
//	4          an xterm-256 index, 0-255
//	#1e66f5    a 24-bit hex triple, #rgb or #rrggbb
//	blue       a name, optionally prefixed bright-
//
// An index renders as 38;5;N rather than the terse 30-37 range even for 0-7:
// it is the same color on every terminal that supports either, and one code
// path is one fewer thing to get wrong.

// Color is a resolved color, or the unset zero value.
type Color struct {
	set    bool
	params string // SGR parameters after the 38;/48; selector
}

// Set reports whether a color was resolved at all. An unset color emits
// nothing and leaves the terminal's default in place, which is the honest
// rendering of a spelling this package did not understand.
func (c Color) Set() bool { return c.set }

// Foreground returns the SGR parameters that set this as a foreground, or the
// empty string when unset.
func (c Color) Foreground() string { return c.selector("38") }

// Background returns the SGR parameters that set this as a background.
func (c Color) Background() string { return c.selector("48") }

func (c Color) selector(which string) string {
	if !c.set {
		return ""
	}
	return which + ";" + c.params
}

// baseColors are the eight names every configuration can rely on. Anything
// else is expected to be an index or a hex triple.
var baseColors = map[string]int{
	"black": 0, "red": 1, "green": 2, "yellow": 3,
	"blue": 4, "magenta": 5, "cyan": 6, "white": 7,
}

// ParseColor resolves a color spec. ok is false for the empty string and for
// anything unrecognized, so a caller falls back rather than painting
// something the person did not ask for. An unrecognized *name* in particular
// resolves to unset and never to a neighboring color: a prompt that quietly
// substitutes is the failure class this repository treats as its worst.
func ParseColor(spec string) (Color, bool) {
	s := strings.ToLower(strings.TrimSpace(spec))
	if s == "" {
		return Color{}, false
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 || n > 255 {
			return Color{}, false
		}
		return indexColor(n), true
	}
	if rgb, ok := parseHex(s); ok {
		return Color{set: true, params: "2;" + rgb}, true
	}
	name, bright := strings.CutPrefix(s, "bright-")
	n, known := baseColors[name]
	if !known {
		return Color{}, false
	}
	if bright {
		n += 8
	}
	return indexColor(n), true
}

func indexColor(n int) Color {
	return Color{set: true, params: "5;" + strconv.Itoa(n)}
}

// parseHex accepts #rgb and #rrggbb, returning "r;g;b" in decimal.
func parseHex(s string) (string, bool) {
	digits, ok := strings.CutPrefix(s, "#")
	if !ok {
		return "", false
	}
	if len(digits) == 3 { // #abc is #aabbcc
		var expanded strings.Builder
		for _, r := range digits {
			expanded.WriteRune(r)
			expanded.WriteRune(r)
		}
		digits = expanded.String()
	}
	if len(digits) != 6 {
		return "", false
	}
	parts := make([]string, 3)
	for i := range parts {
		v, err := strconv.ParseUint(digits[i*2:i*2+2], 16, 8)
		if err != nil {
			return "", false
		}
		parts[i] = strconv.FormatUint(v, 10)
	}
	return strings.Join(parts, ";"), true
}

// Style is the full appearance of one piece of text.
type Style struct {
	Fg, Bg    Color
	Bold      bool
	Underline bool
}

// Empty reports whether the style would emit no escapes at all.
func (s Style) Empty() bool { return !s.Fg.set && !s.Bg.set && !s.Bold && !s.Underline }

// SGR returns the escape sequence that enters the style.
//
// The whole appearance is written at every change rather than a delta from
// whatever came before, because the layout concatenates independently
// rendered pieces and a delta would be correct only in the order it was
// produced in. A style with nothing in it enters the terminal's default,
// which is what Reset spells.
func (s Style) SGR() string {
	params := []string{"0"}
	if s.Bold {
		params = append(params, "1")
	}
	if s.Underline {
		params = append(params, "4")
	}
	if fg := s.Fg.Foreground(); fg != "" {
		params = append(params, fg)
	}
	if bg := s.Bg.Background(); bg != "" {
		params = append(params, bg)
	}
	return "\x1b[" + strings.Join(params, ";") + "m"
}

// Reset is the escape that returns the terminal to its own default.
const Reset = "\x1b[0m"

// SegmentStyle resolves a segment's appearance through the three-step chain,
// with def underneath each of the four settings separately: a configuration
// that names only a foreground keeps the caller's background rather than
// losing it.
func (s *Settings) SegmentStyle(segment, state string, def Style) Style {
	style := def
	if s.ParamSet(segment, state, "FOREGROUND") {
		style.Fg, _ = ParseColor(s.Param(segment, state, "FOREGROUND", ""))
	}
	if s.ParamSet(segment, state, "BACKGROUND") {
		style.Bg, _ = ParseColor(s.Param(segment, state, "BACKGROUND", ""))
	}
	style.Bold = s.ParamBool(segment, state, "BOLD", def.Bold)
	style.Underline = s.ParamBool(segment, state, "UNDERLINE", def.Underline)
	return style
}
