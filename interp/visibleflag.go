// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"unicode/utf8"
)

// `${(V)x}`: make the characters a terminal would act on visible instead.
//
// One shell in the panel has the flag. What it is *for* is putting text
// somebody else produced into a prompt or a listing without letting it move
// the cursor, change the color, or ring the bell — so a branch name holding
// an escape draws as `^[` rather than starting an escape sequence. A prompt
// theme runs every field of a repository's status through it before drawing
// (#1760's theme does exactly that with `${(V)VCS_STATUS_LOCAL_BRANCH}`),
// which is the shape that made this worth having.
//
// # Measured, every control character one at a time
//
// Against zsh 5.9.2 on 2026-09-10, with `a<c>b` for each code, because the
// rule is not the one a reader would guess from the two escapes it does
// use:
//
//	 9  tab        a\tb      one of exactly two with a letter escape
//	10  newline    a\nb      and this is the other
//	11  vertical   a^Kb      *not* `\v`
//	12  form feed  a^Lb      *not* `\f`
//	13  return     a^Mb      *not* `\r`
//	 1-8, 14-31    a^Ab … a^_b
//	27  escape     a^[b
//	127 delete     a^?b
//	92  backslash  a\b       **unchanged** — the flag does not double it
//
// So: tab and newline get `\t` and `\n`, every other character below space
// and `\x7f` get a caret and the code with bit 6 flipped, and nothing else
// is touched. The backslash row is the one worth naming, because a flag
// that escapes control characters and *not* the escape character is the
// opposite of what a quoting flag would do — and `(V)` is not a quoting
// flag. `(q)` is, it runs before this, and `${(Vq)}` on a tab is
// `a$'\t'b`: quoted first, by which point there is no control character
// left for this to see.
//
// Bytes rather than runes, which is what keeps `héllo` intact: every byte
// this rewrites is below `\x80`, and every byte of a multi-byte character
// is above it, so a byte walk cannot reach inside one.
func visibleText(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r < ' ' || r == 0x7f }) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := range len(s) {
		switch c := s[i]; {
		case c == '\t':
			b.WriteString(`\t`)
		case c == '\n':
			b.WriteString(`\n`)
		case c < ' ' || c == 0x7f:
			// The caret spelling: the control code with bit 6 flipped, so
			// `\x01` is `^A` and `\x7f` is `^?`.
			b.WriteByte('^')
			b.WriteByte(c ^ 0x40)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// visibleFlagText is visibleText for the `(V)` flag, which reaches past ASCII
// too: a byte that begins no character is written `\M-` and the visible
// form of its low seven bits. Under a locale whose characters are counted
// that is every such byte; under one whose are not, every byte the C library
// does not class as printing — which is a platform's answer, see
// cLocalePrintsHighByte — and the rest are written as they are. Measured
// 2026-10-02 on zsh 5.9.2: under `en_US.UTF-8` a lone `\x9b` is `\M-^[`,
// `\x89` is `\M-\t`, `\xe1` is `\M-a` and `\xff` is `\M-^?`, a whole `é`
// is `é`, and `\xe2\x82` is `\M-b\M-^B`; under `C` the `\x9b` and the
// `\x89` are the same and `\xe1` and `\xff` are the bytes themselves
// (#5315).
func (r *Runner) visibleFlagText(s string) string {
	if isASCII(s) {
		return visibleText(s)
	}
	chars := r.countsTheLocalesCharacters()
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			b.WriteString(visibleText(s[i : i+1]))
			i++
			continue
		}
		if chars {
			if u, n := utf8.DecodeRuneInString(s[i:]); u != utf8.RuneError || n > 1 {
				b.WriteString(s[i : i+n])
				i += n
				continue
			}
		}
		if chars || !cLocalePrintsHighByte(c) {
			b.WriteString(`\M-` + visibleText(string(rune(c&0x7f))))
		} else {
			b.WriteByte(c)
		}
		i++
	}
	return b.String()
}
