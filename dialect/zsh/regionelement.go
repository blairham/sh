// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
)

// One `region_highlight` element, as zsh reads it and as it hands it back.
//
// zsh does not keep the text a widget assigned: it parses each element as it
// is stored and answers a read with its own spelling of what it parsed, so
// `region_highlight+=(…)` and `typeset -p region_highlight` both see the
// normalized form. Measured 2026-10-02 on zsh 5.9.2 through a
// pseudo-terminal, assigning inside a widget and reading back with `(V)`:
//
//	0 4 fg=green memo=a,future=x      0 4 fg=green memo=a
//	0 4 bold,fg=green                 0 4 fg=green,bold
//	0 4 underline,bold,standout,fg=red,bg=green
//	                                  0 4 fg=red,bg=green,bold,standout,underline
//	P 0 4 bold                        P0 4 bold
//	x y bold                          -1 -1 none
//	1x 2 bold                         1 -1 none
//	0 4x bold                         0 4 none
//	1 2                               1 2 none
//	0 4 fg=red extra                  0 4 fg=red
//	0 4 fg=red memo=a b               0 4 fg=red memo=a
//	0 4 fg=red,memo=x                 0 4 fg=red memo=x
//	0 4 memo=x fg=red                 0 4 none memo=x
//	0 4 bold memo=a<NUL>b             0 4 bold memo=a
//	0 4 fg=#F00                       0 4 fg=#ff0000
//	0 4 fg=1 / fg=re / fg=            fg=red / fg=red / fg=black
//	0 4 fg=Red / fg=300 / fg=-1 / fg=#12 / fg=default / reverse
//	                                  0 4 none
//	0 4 fg=red,fg=blue                0 4 fg=magenta
//	0 4 fg=red,fg=196                 0 4 fg=197
//	0 4 fg=#ff0000,fg=#00ff00         0 4 fg=#ffff00
//	0 4 fg=196,fg=#00ff00             0 4 fg=#00ffc4
//	0 4 bold,none / fg=red,none       0 4 none
//	0 4 none,bold                     0 4 bold
//	0 4 fg=1,none,fg=2                0 4 fg=yellow
//	4 0 bold / 0 99 bold / -1 4 bold  as written
//
// Which is a parser with five rules, all of them visible in the rows:
//
//   - The offsets are read as numbers from the front, the first one after an
//     optional `P`. One that does not read as a number is -1, and the spec
//     after it is not read at all.
//   - The spec is the next run of non-blank characters, split on commas. An
//     unknown word is ignored; `none` turns off everything set so far.
//   - A color is a number, not a choice: a second `fg=` combines with the
//     first bit by bit, a hex triplet sets a flag saying the number is 24-bit,
//     and `none` takes the color off without forgetting its number — so
//     `fg=1,none,fg=2` is 3. Names are the eight, matched as a case-sensitive
//     prefix, which makes an empty name black.
//   - A `memo=` is read inside the spec or as the next blank-separated word
//     after it, up to a comma, a blank or a NUL. Nothing after that is kept.
//   - The spelling back is `[P]start end spec[ memo=token]`, the spec as
//     fg, bg, bold, standout, underline — `none` where none of them is set —
//     a palette color under 8 by its name and a 24-bit one as six hex digits.
//
// The drawing reads the same parse, so what is drawn is what is read back:
// `fg=red,fg=196` is drawn in color 197.

// regionElement is one parsed element.
type regionElement struct {
	predisplay bool
	start, end int
	spec       regionElementSpec
	memo       string
	hasMemo    bool
}

// regionElementSpec is a spec as zsh keeps it.
type regionElementSpec struct {
	bold, standout, underline bool
	fg, bg                    regionChannel
}

// regionChannel is one channel's color: the number, whether it is a 24-bit
// one, and whether it is set — which `none` clears without touching the other
// two.
type regionChannel struct {
	value     int
	trueColor bool
	set       bool
}

// key is the channel as a key regionCodes writes, empty where it is not set.
func (c regionChannel) key() string {
	switch {
	case !c.set:
		return ""
	case c.trueColor:
		return fmt.Sprintf("#%d;%d;%d", c.value>>16&0xff, c.value>>8&0xff, c.value&0xff)
	default:
		return strconv.Itoa(c.value)
	}
}

// spelled is the channel as zsh writes it back.
func (c regionChannel) spelled() string {
	switch {
	case c.trueColor:
		return fmt.Sprintf("#%06x", c.value&0xffffff)
	case c.value >= 0 && c.value < len(regionColorNames):
		return regionColorNames[c.value]
	default:
		return strconv.Itoa(c.value)
	}
}

// attrs is what the spec draws.
func (s regionElementSpec) attrs() regionAttrs {
	return regionAttrs{
		bold: s.bold, standout: s.standout, underline: s.underline,
		fg: s.fg.key(), bg: s.bg.key(),
	}
}

// parseRegionText parses one element the way zsh does.
//
// near is the terminal's color count where `zsh/nearcolor` is loaded and 0
// where it is not; it turns a hex triplet into the nearest palette color as
// the element is read: measured, the module
// loaded, `fg=#ff0000` reads back `fg=196`.
func parseRegionText(elem string, near int) regionElement {
	var e regionElement
	rest := strings.TrimLeft(elem, " \t\n")
	if after, ok := strings.CutPrefix(rest, "P"); ok {
		e.predisplay, rest = true, after
	}
	var ok bool
	if e.start, rest, ok = regionNumber(rest); !ok {
		e.start, e.end = -1, -1
		return e
	}
	if e.end, rest, ok = regionNumber(rest); !ok {
		e.end = -1
		return e
	}
	rest = strings.TrimLeft(rest, " ")
	word, rest := nextRegionWord(rest)
	for _, part := range strings.Split(word, ",") {
		if memo, ok := strings.CutPrefix(part, "memo="); ok {
			e.memo, e.hasMemo = memo, true
			continue
		}
		e.spec.apply(part, near)
	}
	if e.hasMemo {
		return e
	}
	rest = strings.TrimLeft(rest, " ")
	if memo, ok := strings.CutPrefix(rest, "memo="); ok {
		end := strings.IndexAny(memo, ", \x00")
		if end < 0 {
			end = len(memo)
		}
		e.memo, e.hasMemo = memo[:end], true
	}
	return e
}

// nextRegionWord is the run of characters up to a blank or a NUL, and what
// follows it.
func nextRegionWord(s string) (word, rest string) {
	end := strings.IndexAny(s, " \t\n\x00")
	if end < 0 {
		return s, ""
	}
	return s[:end], s[end:]
}

// regionNumber reads a number at the front of s, after any blanks: an
// optional sign and at least one digit.
func regionNumber(s string) (int, string, bool) {
	s = strings.TrimLeft(s, " \t\n")
	i := 0
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		i++
	}
	digits := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == digits {
		return 0, s, false
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil {
		return 0, s, false
	}
	return n, s[i:], true
}

// apply adds one word of a spec.
func (s *regionElementSpec) apply(part string, near int) {
	switch {
	case part == "none":
		s.bold, s.standout, s.underline = false, false, false
		s.fg.set, s.bg.set = false, false
	case part == "bold":
		s.bold = true
	case part == "standout":
		s.standout = true
	case part == "underline":
		s.underline = true
	case strings.HasPrefix(part, "fg="):
		s.fg.add(strings.TrimPrefix(part, "fg="), near)
	case strings.HasPrefix(part, "bg="):
		s.bg.add(strings.TrimPrefix(part, "bg="), near)
	}
}

// add combines one color into the channel. `default`, and a value that
// names nothing, change nothing.
func (c *regionChannel) add(value string, near int) {
	if hex, ok := strings.CutPrefix(value, "#"); ok {
		rgb, ok := parseRegionHex(hex)
		if !ok {
			return
		}
		if near != 0 {
			// The terminal's palette, or no color at all on one neither
			// palette fits: measured under TERM=xterm-16color, `fg=#82c9b0`
			// reads back `none`.
			if n, ok := interp.NearestColor(near, rgb>>16&0xff, rgb>>8&0xff, rgb&0xff); ok {
				c.value |= n
				c.set = true
			}
			return
		}
		c.value |= rgb
		c.trueColor, c.set = true, true
		return
	}
	if value != "" && value[0] >= '0' && value[0] <= '9' {
		n, err := strconv.Atoi(value)
		if err != nil || n > 255 {
			return
		}
		c.value |= n
		c.set = true
		return
	}
	if value == "default" {
		return
	}
	for n, name := range regionColorNames {
		if strings.HasPrefix(name, value) {
			c.value |= n
			c.set = true
			return
		}
	}
}

// parseRegionHex reads `rgb` or `rrggbb` as one 24-bit number; the short
// form doubles each digit.
func parseRegionHex(s string) (int, bool) {
	if len(s) != 3 && len(s) != 6 {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, false
	}
	if len(s) == 3 {
		r, g, b := n>>8&0xf, n>>4&0xf, n&0xf
		n = (r*17)<<16 | (g*17)<<8 | b*17
	}
	return int(n), true
}

// String is the element as zsh hands it back.
func (e regionElement) String() string {
	var b strings.Builder
	if e.predisplay {
		b.WriteByte('P')
	}
	b.WriteString(strconv.Itoa(e.start))
	b.WriteByte(' ')
	b.WriteString(strconv.Itoa(e.end))
	b.WriteByte(' ')
	var parts []string
	if e.spec.fg.set {
		parts = append(parts, "fg="+e.spec.fg.spelled())
	}
	if e.spec.bg.set {
		parts = append(parts, "bg="+e.spec.bg.spelled())
	}
	for _, a := range []struct {
		on   bool
		name string
	}{{e.spec.bold, "bold"}, {e.spec.standout, "standout"}, {e.spec.underline, "underline"}} {
		if a.on {
			parts = append(parts, a.name)
		}
	}
	if len(parts) == 0 {
		parts = []string{"none"}
	}
	b.WriteString(strings.Join(parts, ","))
	if e.hasMemo {
		b.WriteString(" memo=")
		b.WriteString(e.memo)
	}
	return b.String()
}
