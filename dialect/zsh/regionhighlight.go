// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"slices"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// `region_highlight`: how a widget asks for part of the line to be colored.
//
// Measured 2026-09-12 against zsh 5.9.2 under a pseudo-terminal; the table of
// what each spec paints, and the grammar of one element, are in
// docs/spec/editing.md under "Coloring the line as it is typed". This file
// implements that section and nothing beyond it.
//
// **Why this parameter and not a highlighter of our own.** repl already has a
// highlighting seam (repl/highlight.go, #803) and it is deliberately
// in-process and deliberately not a plugin. This is that seam's dialect-side
// supplier: every zsh syntax highlighter in the wild — the two on the
// maintainer's machine included — computes its runs in shell code and then
// assigns them to this one array. Nothing here adds a round trip that
// repl/highlight.go's note rules out; the widget was already going to run.
//
// **The store is a shell variable and not a Go map.** zle.go keeps the widget
// table the same way and for the same reason: a map hanging off the runner is
// shared with whatever the runner was cloned into, and interp/clonetables.go
// records where that ended — `fatal error: concurrent map read and map write`,
// which panicguard cannot catch.

// zleRegion is where the elements live between the widget that wrote them and
// the redraw that reads them.
//
// Hidden, so that `set` and `typeset` do not show it, and an ordinary array
// rather than anything special: what makes `region_highlight` special is the
// view opened over it for the length of a widget call, not the storage.
const zleRegion = ".zsh.zle.region"

// regionHighlightName is the parameter itself, kept beside the store's name so
// the two cannot drift apart in a reader's eye.
const regionHighlightName = "region_highlight"

// openRegionHighlight publishes `region_highlight` for the length of one
// widget call.
//
// A view over the store rather than a copy, which is what lets one widget read
// what an earlier one on the same line left — measured, and the table in the
// spec section records the three keystrokes that establish it.
func openRegionHighlight(r *interp.Runner) {
	r.SetDynamicArray(regionHighlightName, func(rr *interp.Runner) []string {
		elems, _ := rr.GetArray(zleRegion)
		return elems
	})
	r.SetDynamicArrayWriter(regionHighlightName, func(rr *interp.Runner, values []string) {
		rr.SetArray(zleRegion, values)
	})
}

// closeRegionHighlight takes the parameter away again, leaving the store.
//
// The store outlives the call and the parameter does not, which is the whole
// of the difference between "persists within a line" and
// "array-local-special".
func closeRegionHighlight(r *interp.Runner) {
	r.UnsetDynamic(regionHighlightName)
}

// ResetRegionHighlight empties the store, which is what the editor does when
// it starts a line.
//
// Exported because the moment is repl's and not this package's: zsh's rule is
// that the effect "disappears as soon as the line is accepted", and the only
// code that knows a line was accepted is the read loop.
func ResetRegionHighlight(r *interp.Runner) {
	r.SetArray(zleRegion, nil)
}

// RegionHighlights is what the store means, as the codes repl writes between
// the line's characters.
//
// The dialect's answer to driver.Shell.HighlightLine. The line comes in
// because the offsets have to be converted against it: zsh counts characters
// and repl.Highlight counts bytes, and nothing else here knows the text.
//
// An element this shell cannot honor is **dropped**, never approximated. A
// `P` flag counts a PREDISPLAY that does not exist here, and an offset pair
// that does not parse is not a region at all — drawing either one somewhere
// plausible would put color under text the highlighter did not name, which is
// worse than leaving it plain and is the kind of silent wrong answer the
// editor has no way to report.
//
// The answer is points rather than runs — repl.Highlight.Point — because
// where one region ends inside another, what goes out depends on both. See
// regionTransition for the rule.
func RegionHighlights(r *interp.Runner, line string) []repl.Highlight {
	elems, _ := r.GetArray(zleRegion)
	if len(elems) == 0 {
		return nil
	}
	// The rune offsets of every byte boundary, built once: an element is
	// three small integer lookups after this, where converting per element
	// would walk the line again for each.
	starts := runeStarts(line)
	var layers []regionLayer
	for _, elem := range elems {
		if layer, ok := parseRegionElement(elem, len(starts)-1); ok {
			layers = append(layers, layer)
		}
	}
	if len(layers) == 0 {
		return nil
	}
	codes := readRegionCodes(r)
	var out []repl.Highlight
	var before regionAttrs
	var covering []int
	for i := 0; i < len(starts); i++ {
		var now []int
		if i < len(starts)-1 {
			for j, l := range layers {
				if l.start <= i && i < l.end {
					now = append(now, j)
				}
			}
		}
		after := regionWord(layers, now)
		if codes := regionTransition(layers, covering, now, before, after, codes); codes != "" {
			out = append(out, repl.Highlight{Start: starts[i], Style: codes, Point: true})
		}
		before, covering = after, now
	}
	return out
}

// runeStarts is the byte offset of each character of the line, with the length
// of the line on the end so that an offset one past the last character — which
// is what an element covering the whole line writes — converts like any other.
func runeStarts(line string) []int {
	out := make([]int, 0, len(line)+1)
	for i := range line {
		out = append(out, i)
	}
	return append(out, len(line))
}

// regionLayer is one element: the characters it covers and what it asks for.
type regionLayer struct {
	start, end int
	attrs      regionAttrs
}

// regionAttrs is what a character is drawn with: three attributes, and a
// foreground and background color held as regionColor keys.
type regionAttrs struct {
	bold, standout, underline bool
	fg, bg                    string
}

// colored says a foreground or a background is set.
func (a regionAttrs) colored() bool { return a.fg != "" || a.bg != "" }

// parseRegionElement turns one element into a layer, or reports that it is
// not one this shell can draw. chars is the length of the line in characters.
func parseRegionElement(elem string, chars int) (regionLayer, bool) {
	fields := strings.Fields(elem)
	if len(fields) < 3 {
		return regionLayer{}, false
	}
	// This is also where a `P` element goes, in both of its spellings. The
	// flag says the offsets count a PREDISPLAY, which this shell does not
	// have, so the element must not be drawn — and neither `P` nor `P0` is a
	// number, so it is not.
	//
	// **Deliberately not a branch of its own.** One was written first and
	// mutation testing found it inert: deleting it changed no behavior and
	// failed no test, because every element it claimed to catch was already
	// being dropped here. A guard that cannot be removed by a test is a
	// comment promising something the code does not do.
	start, err1 := strconv.Atoi(fields[0])
	end, err2 := strconv.Atoi(fields[1])
	if err1 != nil || err2 != nil {
		return regionLayer{}, false
	}
	// Out of the line's range is dropped rather than clamped, for the reason
	// the doc comment gives: a clamp invents a region.
	if start < 0 || end < start || end > chars {
		return regionLayer{}, false
	}
	attrs, ok := regionSpec(fields[2])
	if !ok {
		return regionLayer{}, false
	}
	return regionLayer{start: start, end: end, attrs: attrs}, true
}

// regionSpec is what one spec asks for, and false for a spec that paints
// nothing.
func regionSpec(spec string) (regionAttrs, bool) {
	var a regionAttrs
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "bold":
			a.bold = true
		case part == "standout":
			a.standout = true
		case part == "underline":
			a.underline = true
		case strings.HasPrefix(part, "fg="):
			a.fg = regionColor(strings.TrimPrefix(part, "fg="))
		case strings.HasPrefix(part, "bg="):
			a.bg = regionColor(strings.TrimPrefix(part, "bg="))
		}
		// Anything else contributes nothing, and that is the whole of what
		// the rest have in common: `none`, which overrides a default rather
		// than adding to one; `memo=token`, which names the plugin that wrote
		// the element so it can find it again and selects nothing to draw; an
		// empty part, from a trailing comma; and a word this shell does not
		// know. Each had a case of its own and every one of them was inert —
		// staticcheck caught the third, and the `P` guard above is the same
		// story. A branch that only falls through is not documentation.
	}
	return a, a != regionAttrs{}
}

// regionWord is what a character covered by the given layers is drawn with.
//
// The layers apply in the array's order. One that sets a color **replaces**
// everything under it, attributes included; one that sets only attributes
// adds them to what is under it and keeps its colors. Measured 2026-10-02 on
// zsh 5.9.2 through a pseudo-terminal, `standout` over 1–5 and `fg=2` over
// 3–7: the overlap is drawn green and not reversed. The same pair the other
// way round draws the overlap both.
func regionWord(layers []regionLayer, covering []int) regionAttrs {
	var w regionAttrs
	for _, j := range covering {
		l := layers[j].attrs
		if l.colored() {
			w = l
			continue
		}
		w.bold = w.bold || l.bold
		w.standout = w.standout || l.standout
		w.underline = w.underline || l.underline
	}
	return w
}

// The sequences that end each attribute. Bold has no ending of its own and
// is ended by ending everything, which zsh does without putting back what
// that also ended — measured: `bold` over 1–5 and `underline` over 3–7 leave
// the last two characters of the underline plain.
const (
	regionBoldOff      = "\x1b[0m"
	regionStandoutOff  = "\x1b[27m"
	regionUnderlineOff = "\x1b[24m"
)

// regionTransition is what goes out between two characters: was is the
// layers covering the one before and what it was drawn with, now and after
// the same for the one after.
//
// Fitted to 93 measured pairs of regions on zsh 5.9.2 through a
// pseudo-terminal, 2026-10-02 — every pair of nine specs over 1–5 and 3–7 —
// and then checked against 60 more drawn at random, two and three regions at
// arbitrary offsets, which it reproduced byte for byte. The rule:
//
//   - **What is turned off** is everything an element that ends here asked
//     for, whether or not it was showing, and anything that was showing and
//     is not now. A color changing to another color is not turned off.
//   - **Everything showing is then written again** if any of these holds:
//     something turned off was showing and is wanted again; a color that was
//     showing was turned off and something is still showing in its place or
//     as an attribute; something is showing now that was not, or in another
//     color; or an element starts here and the character is drawn
//     differently from the one before.
//
// Written again means all of it, in a fixed order — bold, standout,
// underline, foreground, background — which is the order the ending
// sequences go out in too.
func regionTransition(layers []regionLayer, was, now []int, before, after regionAttrs, codes regionCodes) string {
	var off regionAttrs
	for _, j := range was {
		if !slices.Contains(now, j) {
			l := layers[j].attrs
			off.bold = off.bold || l.bold
			off.standout = off.standout || l.standout
			off.underline = off.underline || l.underline
			if l.fg != "" {
				off.fg = "off"
			}
			if l.bg != "" {
				off.bg = "off"
			}
		}
	}
	off.bold = off.bold || (before.bold && !after.bold)
	off.standout = off.standout || (before.standout && !after.standout)
	off.underline = off.underline || (before.underline && !after.underline)
	if before.fg != "" && after.fg == "" {
		off.fg = "off"
	}
	if before.bg != "" && after.bg == "" {
		off.bg = "off"
	}
	var b strings.Builder
	for _, o := range []struct {
		on  bool
		seq string
	}{
		{off.bold, regionBoldOff},
		{off.standout, regionStandoutOff},
		{off.underline, regionUnderlineOff},
		{off.fg != "", codes.fg.off()},
		{off.bg != "", codes.bg.off()},
	} {
		if o.on {
			b.WriteString(o.seq)
		}
	}
	// A color turned off counts only where one was showing: an ending
	// element's color that a later one had replaced puts nothing back.
	// Attributes need no such test — one turned off while not showing and
	// wanted now is showing now and not before, which the clause after them
	// already answers.
	shownFg, shownBg := off.fg != "" && before.fg != "", off.bg != "" && before.bg != ""
	afterAttrs := after.bold || after.standout || after.underline
	again := (off.bold && after.bold) || (off.standout && after.standout) ||
		(off.underline && after.underline) ||
		((shownFg || shownBg) && (afterAttrs || (shownFg && after.fg != "") || (shownBg && after.bg != ""))) ||
		(after.bold && !before.bold) || (after.standout && !before.standout) ||
		(after.underline && !before.underline) ||
		(after.fg != "" && after.fg != before.fg) || (after.bg != "" && after.bg != before.bg)
	if !again && after != before && after != (regionAttrs{}) {
		for _, j := range now {
			if !slices.Contains(was, j) {
				again = true
				break
			}
		}
	}
	if again {
		if after.bold {
			b.WriteString("\x1b[1m")
		}
		if after.standout {
			b.WriteString("\x1b[7m")
		}
		if after.underline {
			b.WriteString("\x1b[4m")
		}
		if after.fg != "" {
			b.WriteString(codes.fg.color(after.fg))
		}
		if after.bg != "" {
			b.WriteString(codes.bg.color(after.bg))
		}
	}
	return b.String()
}

// regionColorNames is the eight zsh sets by name, in the order the terminal
// numbers them. A name abbreviates: measured, `b` and `bl` both select black,
// so what is matched is a prefix.
var regionColorNames = []string{
	"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
}

// regionColor is the color one `fg=` or `bg=` value names, as a key: the
// palette number in decimal, or `#` and the three channels of a hex triplet
// joined by `;`. regionCodes.color turns it into what is written.
//
// Empty for `default`, which measured paints nothing — the parameter's way of
// saying "leave the terminal's own color alone" — and empty for a value that
// names nothing, which is the drop this file applies everywhere else. Out of
// the palette's range is nothing too, rather than a color the person did not
// ask for.
func regionColor(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "default" {
		return ""
	}
	if after, ok := strings.CutPrefix(value, "#"); ok {
		r, g, b, ok := parseHexTriplet(after)
		if !ok {
			return ""
		}
		return "#" + strconv.Itoa(r) + ";" + strconv.Itoa(g) + ";" + strconv.Itoa(b)
	}
	if n, err := strconv.Atoi(value); err == nil {
		if n < 0 || n > 255 {
			return ""
		}
		return strconv.Itoa(n)
	}
	// "Colour is also known as color", and both spellings of grey are zsh's
	// too — but neither is one of the eight, so a name is matched against the
	// eight and nothing else, as a prefix.
	for n, name := range regionColorNames {
		if strings.HasPrefix(name, strings.ToLower(value)) {
			return strconv.Itoa(n)
		}
	}
	return ""
}

// regionCodes is how a color is written, which `zle_highlight` can change:
// each channel's start, end and default-color codes.
//
// The manual names six fields and their defaults — `fg_start_code` (`\e[3`),
// `fg_default_code` (`9`), `fg_end_code` (`m`) and the three `bg_` ones with
// `\e[4` — and says the start code is "followed by one to three ASCII digits
// representing the colour". Measured 2026-10-02 on zsh 5.9.2 through a
// pseudo-terminal, with `zle_highlight=( fg_start_code:"S|" fg_end_code:"|E"
// bg_start_code:"B|" bg_end_code:"|F" )`:
//
//	fg=1                 S|1|E   ending S|9|E
//	fg=196               S|196|E
//	bg=2                 B|2|F   ending B|9|F
//	fg=#ff0000           ESC[38;2;255;0;0m, ending S|9|E
//	fg_default_code:D    fg=1 ends ESC[3Dm
//
// So a palette color above 7 is the start code and its number only where a
// start code was given: with none, it is `ESC[38;5;n m`, the table in
// docs/spec/editing.md. A hex triplet is written in full whatever the codes
// say, and its ending is the channel's default like any other.
type regionCodes struct {
	fg, bg regionChannelCodes
}

// regionChannelCodes is one channel's three codes, and whether its start code
// was given rather than defaulted.
type regionChannelCodes struct {
	start, end, def string
	given           bool
	extended        string
}

// readRegionCodes reads `zle_highlight`'s code fields, leaving the defaults
// where a field is not there.
func readRegionCodes(r *interp.Runner) regionCodes {
	c := regionCodes{
		fg: regionChannelCodes{start: "\x1b[3", end: "m", def: "9", extended: "38"},
		bg: regionChannelCodes{start: "\x1b[4", end: "m", def: "9", extended: "48"},
	}
	fields, _ := r.GetArray("zle_highlight")
	for _, f := range fields {
		name, value, ok := strings.Cut(f, ":")
		if !ok {
			continue
		}
		switch name {
		case "fg_start_code":
			c.fg.start, c.fg.given = value, true
		case "fg_end_code":
			c.fg.end = value
		case "fg_default_code":
			c.fg.def = value
		case "bg_start_code":
			c.bg.start, c.bg.given = value, true
		case "bg_end_code":
			c.bg.end = value
		case "bg_default_code":
			c.bg.def = value
		}
	}
	return c
}

// color is what selects key on this channel.
func (c regionChannelCodes) color(key string) string {
	if rgb, ok := strings.CutPrefix(key, "#"); ok {
		return "\x1b[" + c.extended + ";2;" + rgb + "m"
	}
	if n, _ := strconv.Atoi(key); n > 7 && !c.given {
		return "\x1b[" + c.extended + ";5;" + key + "m"
	}
	return c.start + key + c.end
}

// off is what returns this channel to the terminal's own color.
func (c regionChannelCodes) off() string { return c.start + c.def + c.end }

// parseHexTriplet reads `#rgb` and `#rrggbb`, the two lengths the manual gives.
//
// The three-digit form doubles each digit, which is what makes `#f80` and
// `#ff8800` the same color.
func parseHexTriplet(s string) (int, int, int, bool) {
	var width int
	switch len(s) {
	case 3:
		width = 1
	case 6:
		width = 2
	default:
		return 0, 0, 0, false
	}
	out := make([]int, 3)
	for i := range out {
		part := s[i*width : (i+1)*width]
		n, err := strconv.ParseUint(part, 16, 16)
		if err != nil {
			return 0, 0, 0, false
		}
		if width == 1 {
			out[i] = int(n)*16 + int(n)
		} else {
			out[i] = int(n)
		}
	}
	return out[0], out[1], out[2], true
}
