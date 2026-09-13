// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
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

// RegionHighlights is what the store means, as runs repl can draw.
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
func RegionHighlights(r *interp.Runner, line string) []repl.Highlight {
	elems, _ := r.GetArray(zleRegion)
	if len(elems) == 0 {
		return nil
	}
	// The rune offsets of every byte boundary, built once: an element is
	// three small integer lookups after this, where converting per element
	// would walk the line again for each.
	starts := runeStarts(line)
	out := make([]repl.Highlight, 0, len(elems))
	for _, elem := range elems {
		if run, ok := parseRegionElement(elem, starts); ok {
			out = append(out, run)
		}
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

// parseRegionElement turns one element into a run, or reports that it is not
// one this shell can draw.
func parseRegionElement(elem string, starts []int) (repl.Highlight, bool) {
	fields := strings.Fields(elem)
	if len(fields) < 3 {
		return repl.Highlight{}, false
	}
	// This is also where a `P` element goes, in both of its spellings. The
	// flag says the offsets count a PREDISPLAY, which this shell does not
	// have, so the element must not be drawn — and neither `P` nor `P0` is a
	// number, so it is not.
	//
	// **Deliberately not a branch of its own.** One was written first and
	// mutation testing found it inert: deleting it changed no behaviour and
	// failed no test, because every element it claimed to catch was already
	// being dropped here. A guard that cannot be removed by a test is a
	// comment promising something the code does not do.
	start, err1 := strconv.Atoi(fields[0])
	end, err2 := strconv.Atoi(fields[1])
	if err1 != nil || err2 != nil {
		return repl.Highlight{}, false
	}
	// Out of the line's range is dropped rather than clamped, for the reason
	// the doc comment gives: a clamp invents a region.
	if start < 0 || end < start || end > len(starts)-1 {
		return repl.Highlight{}, false
	}
	style := regionStyle(fields[2])
	if style == "" {
		return repl.Highlight{}, false
	}
	return repl.Highlight{Start: starts[start], End: starts[end], Style: style}, true
}

// regionStyle is the escape sequence one spec opens with, or empty for a spec
// that paints nothing.
//
// Only the opening sequence: repl ends every run with a full reset of its own,
// which is repl/highlight.go's stated design and not something to work around
// from here. The spec section records what that costs and where it shows.
//
// The order is the shell's and not the spec's — attributes, then foreground,
// then background, whatever order they were written in. Measured:
// `fg=red,bold` goes out `ESC[1m` `ESC[31m`.
func regionStyle(spec string) string {
	var attrs, fg, bg string
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "" || part == "none":
			// `none` overrides a default rather than adding anything, and an
			// element that says only `none` paints nothing at all.
			continue
		case part == "bold":
			attrs += "\x1b[1m"
		case part == "standout":
			attrs += "\x1b[7m"
		case part == "underline":
			attrs += "\x1b[4m"
		case strings.HasPrefix(part, "fg="):
			fg = colorEscape(strings.TrimPrefix(part, "fg="), true)
		case strings.HasPrefix(part, "bg="):
			bg = colorEscape(strings.TrimPrefix(part, "bg="), false)
		case strings.HasPrefix(part, "memo="):
			// Carried verbatim by zsh and parsed no further. It names the
			// plugin that wrote the element so the plugin can find it again;
			// it selects nothing to draw.
			continue
		}
	}
	return attrs + fg + bg
}

// regionColorNames is the eight zsh sets by name, in the order the terminal
// numbers them. A name abbreviates: measured, `b` and `bl` both select black,
// so what is matched is a prefix.
var regionColorNames = []string{
	"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
}

// colorEscape is what one `fg=` or `bg=` value paints.
//
// Empty for `default`, which measured paints nothing — the parameter's way of
// saying "leave the terminal's own color alone" — and empty for a value that
// names nothing, which is the drop this file applies everywhere else.
func colorEscape(value string, foreground bool) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "default" {
		return ""
	}
	base, extended := "3", "38"
	if !foreground {
		base, extended = "4", "48"
	}
	// A hex triplet is 24-bit color: `ESC[38;2;r;g;bm`, measured.
	if after, ok := strings.CutPrefix(value, "#"); ok {
		r, g, b, ok := parseHexTriplet(after)
		if !ok {
			return ""
		}
		return "\x1b[" + extended + ";2;" +
			strconv.Itoa(r) + ";" + strconv.Itoa(g) + ";" + strconv.Itoa(b) + "m"
	}
	if n, err := strconv.Atoi(value); err == nil {
		return paletteEscape(n, base, extended)
	}
	// "Colour is also known as color", and both spellings of grey are zsh's
	// too — but neither is one of the eight, so a name is matched against the
	// eight and nothing else, as a prefix.
	for n, name := range regionColorNames {
		if strings.HasPrefix(name, strings.ToLower(value)) {
			return paletteEscape(n, base, extended)
		}
	}
	return ""
}

// paletteEscape is a numbered color, in whichever of the two forms its number
// falls in.
//
// The manual describes one form — `fg_start_code` then "one to three ASCII
// digits" — which would make `fg=200` into `ESC[3200m`. Measured, it is
// `ESC[38;5;200m`, and only 0 through 7 take the short form. Out of range is
// nothing rather than a color the person did not ask for.
func paletteEscape(n int, base, extended string) string {
	switch {
	case n < 0 || n > 255:
		return ""
	case n <= 7:
		return "\x1b[" + base + strconv.Itoa(n) + "m"
	default:
		return "\x1b[" + extended + ";5;" + strconv.Itoa(n) + "m"
	}
}

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
