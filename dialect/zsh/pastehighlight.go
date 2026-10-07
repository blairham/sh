// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strings"

	"github.com/blairham/sh/interp"
)

// PastedTextStyle is what text a paste or a yank put in the line is drawn
// between: `zle_highlight`'s `paste` context, standout where it names none.
// The dialect's answer to driver.Shell.PastedTextStyle (#6271).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, `ab`
// yanked after `zle_highlight=(paste:SPEC)`:
//
//	SPEC                                   drawn
//	(no paste context, or none at all)     \e[7m ab \e[27m
//	none                                   ab
//	bold                                   \e[1m ab \e[0m
//	standout,underline  underline,standout \e[7m\e[4m ab \e[27m\e[24m
//	fg=red,bold         bold,fg=red        \e[1m\e[31m ab \e[0m\e[39m
//	bg=blue                                \e[44m ab \e[49m
//	fg=196                                 \e[38;5;196m ab \e[39m
//	fg=#ff0000                             \e[38;2;255;0;0m ab \e[39m
//	none,bold                              bold's
//	standout,none                          ab
//
// So the order is the attribute's and not the spec's — bold, standout,
// underline, then the foreground and the background — the endings come in
// the same order, and `none` clears what came before it. A bracketed paste is
// drawn the same way. The colors are written as `region_highlight`'s are,
// `zle_highlight`'s code fields included.
func PastedTextStyle(r *interp.Runner) (on, off string) {
	spec, found := "", false
	fields, _ := r.GetArray("zle_highlight")
	for _, f := range fields {
		if v, ok := strings.CutPrefix(f, "paste:"); ok {
			spec, found = v, true
		}
	}
	if !found {
		return "\x1b[7m", "\x1b[27m"
	}
	var s regionElementSpec
	for _, part := range strings.Split(spec, ",") {
		s.apply(part, 0)
	}
	a := s.attrs()
	codes := readRegionCodes(r)
	var b, e strings.Builder
	if a.bold {
		b.WriteString("\x1b[1m")
		e.WriteString(regionBoldOff)
	}
	if a.standout {
		b.WriteString("\x1b[7m")
		e.WriteString(regionStandoutOff)
	}
	if a.underline {
		b.WriteString("\x1b[4m")
		e.WriteString(regionUnderlineOff)
	}
	if a.fg != "" {
		b.WriteString(codes.fg.color(a.fg))
		e.WriteString(codes.fg.off())
	}
	if a.bg != "" {
		b.WriteString(codes.bg.color(a.bg))
		e.WriteString(codes.bg.off())
	}
	return b.String(), e.String()
}
