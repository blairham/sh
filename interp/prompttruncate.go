// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"unicode/utf8"
)

// Prompt truncation: `%N<string<` and `%N>string>` cut what follows them to N
// columns, putting string where the cut was. See PromptStyle.Truncation.
//
// Measured 2026-10-01 on zsh 5.9.2 (`print -P`, `-f`, `env -i`), each row
// written between brackets so the edges show:
//
//	%5<..<abcdefgh]            ..gh]       the segment runs to the end, `]` too
//	%5>..>abcdefgh]            abc..       and the cut is on the right
//	%5<..<abc]                 abc]        within the width: untouched
//	%3<..<abcdefgh]            ..]         the string counts against the width
//	%2<..<abcdefgh]            ..          and is drawn whole however wide
//	%0<..<  %<..<              untouched   no count is no truncation
//	%5<<abcdefgh]              efgh]       an empty string
//	%5<..<ab%10<**<cdef…p]     ab**jklmnop]  the next one ends this one
//	%5<..<ab%(?.cdefgh.x)ij]   ..ij]       a segment runs across a conditional
//	%(?.%5<..<abcdefgh.x)ij]   ..fghij]    and one begun in an arm ends with it
//	%5<..<ab%F{red}cdefgh%f]   ..\e[31mgh\e[39m]  sequences survive, after the string
//	%5>..>ab%F{red}cdefgh%f]   ab\e[31mc..\e[39m  and before it on the right
//	%5<..<a%{XYZ%}bcdefgh]     ..XYZgh]    as does a %{…%} run, at no width
//	%5<.%v.<abcdefgh]          .%v.]       the string is drawn as written
//	%5<..<éèêëēėęa]            ..ęa]       and it counts characters
//
// Not modeled: the column a later `%(N^…)` or `%(Nl…)` reads still counts
// what was cut away.
type promptTruncation struct {
	start int    // where in the walk's buffer the segment began
	width int    // the count
	with  string // what stands in for what was cut
	left  bool   // `<`: cut from the left and keep the end
	done  bool   // applied, so an arm that ended it does not apply it twice
}

// truncationAt reads `%N<string<` or `%N>string>` at the code i and begins a
// segment, ending the one before it. It answers the index of the last
// character it consumed.
func (w *promptWalk) truncationAt(runes []rune, i int, num string) int {
	delim := runes[i]
	j := i + 1
	for j < len(runes) && runes[j] != delim {
		j++
	}
	with := string(runes[i+1 : min(j, len(runes))])
	w.endTruncation()
	if n := promptCount(num); n > 0 {
		w.trunc = &promptTruncation{start: w.b.Len(), width: n, with: with, left: delim == '<'}
	}
	return min(j, len(runes)-1)
}

// endTruncation applies the open segment, if any, to what has been drawn
// since it began.
func (w *promptWalk) endTruncation() {
	t := w.trunc
	w.trunc = nil
	if t == nil || t.done {
		return
	}
	t.done = true
	all := w.b.String()
	seg := all[t.start:]
	units := w.promptUnits(seg, t.start)
	visible := 0
	for _, u := range units {
		if !u.zero {
			visible++
		}
	}
	if visible <= t.width {
		return
	}
	keep := max(0, t.width-utf8.RuneCountInString(t.with))
	var out strings.Builder
	if t.left {
		// The string first, then every sequence the cut passed over, then
		// the last `keep` characters with whatever stands among them.
		drop := visible - keep
		seen, cut := 0, len(units)
		var zeros strings.Builder
		for k, u := range units {
			if !u.zero {
				if seen == drop {
					cut = k
					break
				}
				seen++
				continue
			}
			zeros.WriteString(u.text)
		}
		out.WriteString(t.with)
		out.WriteString(zeros.String())
		for _, u := range units[cut:] {
			out.WriteString(u.text)
		}
	} else {
		// The first `keep` characters with what stands among them, the
		// string, and then every sequence after the cut.
		seen, cut := 0, len(units)
		for k, u := range units {
			if !u.zero {
				if seen == keep {
					cut = k
					break
				}
				seen++
			}
			out.WriteString(u.text)
		}
		out.WriteString(t.with)
		for _, u := range units[cut:] {
			if u.zero {
				out.WriteString(u.text)
			}
		}
	}
	w.b.Reset()
	w.b.WriteString(all[:t.start])
	w.b.WriteString(out.String())
	w.zero = w.zero[:0]
}

type promptUnit struct {
	text string
	zero bool
}

// promptUnits splits drawn text into what occupies a column and what does
// not: an escape sequence the walk wrote, and anything drawn inside `%{ … %}`.
// base is where the text began in the buffer, to read the hidden ranges
// against.
func (w *promptWalk) promptUnits(s string, base int) []promptUnit {
	var out []promptUnit
	for i := 0; i < len(s); {
		if hidden := w.hiddenEndAt(base + i); hidden > base+i {
			end := min(hidden-base, len(s))
			out = append(out, promptUnit{text: s[i:end], zero: true})
			i = end
			continue
		}
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) {
				j++
			}
			out = append(out, promptUnit{text: s[i:j], zero: true})
			i = j
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		out = append(out, promptUnit{text: s[i : i+size]})
		i += size
	}
	return out
}

// hiddenEndAt is the end of the `%{ … %}` range starting at byte at, or at
// itself where none does.
func (w *promptWalk) hiddenEndAt(at int) int {
	for _, z := range w.zero {
		if z[0] == at {
			return z[1]
		}
	}
	return at
}
