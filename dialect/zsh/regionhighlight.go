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
		// Stored as zsh reads it back, not as written: see regionelement.go.
		normalized := make([]string, len(values))
		for i, v := range values {
			normalized[i] = parseRegionText(v, rr.NearestColors()).String()
		}
		rr.SetArray(zleRegion, normalized)
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
	r.SetVar(zleRegionLine, "")
}

// zleRegionLine is the line the store's offsets were last lined up with, so
// that what the editor did to the line since can be told from what a widget
// assigned to it. Hidden for the reason the store is.
const zleRegionLine = ".zsh.zle.region.line"

// The offsets follow the line when the *editor* edits it, and stay where
// they are when a widget assigns to it.
//
// Measured 2026-10-04 against zsh 5.9.2 through a pseudo-terminal, with
// `abcdefgh` in the line and `1 3 bold`, `4 6 underline` and `6 7 standout
// memo=m` in `region_highlight` (#5874):
//
//	self-insert at 1, 2, 3, 6     every offset at or after the insert moves
//	                              right — `1 3` is `2 4`, `1 4`, `1 4`, `1 3`
//	delete-char at 2              `1 2 bold|3 5 underline|5 6 standout`
//	backward-kill-word from 5     `0 0 bold|0 1 underline|1 2 standout`
//	kill-line from 3              `1 3 bold|3 3 underline|3 3 standout`
//	yank `XY` at 2                `1 5 bold|6 8 underline|8 9 standout`
//	a key typed with no widget    the same as the widget's self-insert
//	BUFFER=, LBUFFER+=, RBUFFER=  nothing moves
//
// So an insert of n characters at q moves every offset at or after q by n,
// and a removal of [s, e) puts every offset inside it at s and moves every
// one after it back by e - s. A `P` element moves with the rest, and a memo
// is kept.
//
// What the editor did is not told to this package edit by edit; it is read
// off the difference between the line the store was lined up with and the
// line as it is now, which is one replacement. That is exact for an insert
// or a removal, which is what one action is, and for a run of them at one
// place — a paste, a key and its Backspace. Where the edit sits in a run of
// equal characters the difference alone cannot place it, so the cursor
// after the edit decides, where it is known.

// regionsFollow moves the store's offsets for what the editor did to the
// line since they were last lined up with it, and lines them up with line.
// cursor is where the editor left the cursor, or -1 where it is not known.
func regionsFollow(r *interp.Runner, line string, cursor int) {
	was, known := r.GetVar(zleRegionLine)
	if was == line {
		return
	}
	r.SetVar(zleRegionLine, line)
	if !known {
		// Nothing has lined the offsets up with any line yet, so there is no
		// edit to read: a difference from nothing would be the whole line
		// inserted in front of them.
		return
	}
	elems, _ := r.GetArray(zleRegion)
	if len(elems) == 0 {
		return
	}
	at, removed, inserted := lineEdit([]rune(was), []rune(line), cursor)
	moved := make([]string, len(elems))
	for i, elem := range elems {
		e := parseRegionText(elem, 0)
		e.start = shiftRegionOffset(e.start, at, removed, inserted)
		e.end = shiftRegionOffset(e.end, at, removed, inserted)
		moved[i] = e.String()
	}
	r.SetArray(zleRegion, moved)
}

// regionsAnchor lines the store up with line without moving anything: the
// difference is a widget's assignment, which moves nothing.
func regionsAnchor(r *interp.Runner, line string) {
	r.SetVar(zleRegionLine, line)
}

// shiftRegionOffset is where an offset lands after removed characters at at
// are replaced by inserted ones.
func shiftRegionOffset(p, at, removed, inserted int) int {
	if p < at {
		return p
	}
	if p < at+removed {
		p = at
	} else {
		p -= removed
	}
	return p + inserted
}

// lineEdit is the one replacement that turns was into now: where it starts,
// how many characters it removed and how many it inserted.
//
// The common prefix and suffix, which places an insert or a removal inside a
// run of equal characters at the run's far end. The cursor moves it back
// where it says the edit was: after an insert, at the insert's end, and
// after a removal, at its start.
func lineEdit(was, now []rune, cursor int) (at, removed, inserted int) {
	for at < len(was) && at < len(now) && was[at] == now[at] {
		at++
	}
	suffix := 0
	for suffix < len(was)-at && suffix < len(now)-at && was[len(was)-1-suffix] == now[len(now)-1-suffix] {
		suffix++
	}
	removed, inserted = len(was)-at-suffix, len(now)-at-suffix
	switch {
	case removed == 0 && inserted > 0:
		if q := cursor - inserted; q >= 0 && q < at &&
			slices.Equal(was[:q], now[:q]) && slices.Equal(was[q:], now[q+inserted:]) {
			at = q
		}
	case inserted == 0 && removed > 0:
		if q := cursor; q >= 0 && q < at &&
			slices.Equal(was[:q], now[:q]) && slices.Equal(was[q+removed:], now[q:]) {
			at = q
		}
	}
	return at, removed, inserted
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
	// Lined up with the line before it is read: a key the editor handled on
	// its own has moved the text under the offsets since a widget last saw
	// them. Not inside a widget, where the line is drawn by `zle -R` after an
	// assignment, which moves nothing, or by an action part-way through,
	// which the `zle` that called it follows once it returns.
	if !editorRunning(r) {
		regionsFollow(r, line, -1)
		elems, _ = r.GetArray(zleRegion)
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
// foreground and background color held as regionChannel keys.
type regionAttrs struct {
	bold, standout, underline bool
	fg, bg                    string
}

// colored says a foreground or a background is set.
func (a regionAttrs) colored() bool { return a.fg != "" || a.bg != "" }

// parseRegionElement turns one element into a layer, or reports that it is
// not one this shell can draw. chars is the length of the line in characters.
//
// It reads the element the way it is read back — see regionelement.go — so
// what is drawn is what a script finds in the array.
func parseRegionElement(elem string, chars int) (regionLayer, bool) {
	e := parseRegionText(elem, 0)
	// A `P` element counts a PREDISPLAY, which this shell does not have, so
	// it must not be drawn somewhere plausible instead.
	if e.predisplay {
		return regionLayer{}, false
	}
	// Out of the line's range is dropped rather than clamped, for the reason
	// the doc comment gives: a clamp invents a region.
	if e.start < 0 || e.end < e.start || e.end > chars {
		return regionLayer{}, false
	}
	attrs := e.spec.attrs()
	if attrs == (regionAttrs{}) {
		return regionLayer{}, false
	}
	return regionLayer{start: e.start, end: e.end, attrs: attrs}, true
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
	// bright is the parameter of the first of colors 8 to 15.
	bright int
}

// readRegionCodes reads `zle_highlight`'s code fields, leaving the defaults
// where a field is not there.
func readRegionCodes(r *interp.Runner) regionCodes {
	c := regionCodes{
		fg: regionChannelCodes{start: "\x1b[3", end: "m", def: "9", extended: "38", bright: 90},
		bg: regionChannelCodes{start: "\x1b[4", end: "m", def: "9", extended: "48", bright: 100},
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
		if n < 16 {
			// The bright half has a run of its own: measured on zsh 5.9.2,
			// `fg=8` is `ESC[90m` and `bg=15` is `ESC[107m` (#5512).
			return "\x1b[" + strconv.Itoa(c.bright+n-8) + "m"
		}
		return "\x1b[" + c.extended + ";5;" + key + "m"
	}
	return c.start + key + c.end
}

// off is what returns this channel to the terminal's own color.
func (c regionChannelCodes) off() string { return c.start + c.def + c.end }
