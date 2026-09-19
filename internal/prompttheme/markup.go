// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme

import "strings"

// Markup inside a value.
//
// A setting like a frame prefix is one string carrying both text and
// appearance — "╭─" in color 242 is one value, not two settings — so values
// carry a small markup vocabulary, read here and by nothing else:
//
//	%F{c}   foreground on      %f      foreground back to the base
//	%K{c}   background on      %k      background back to the base
//	%B %b   bold on/off        %U %u   underline on/off
//	%%      a literal %
//
// The spelling is deliberately the one people who configure prompts already
// have in their fingers. Reading it is not interpreting zsh: it is a
// fixed-vocabulary color markup with no expansion, no substitution and no
// control flow. Anything outside the vocabulary passes through untouched
// rather than being guessed at, which is why a prompt character that happens
// to be a percent sign survives unread.
//
// Substitution is limited to ${NAME} against a caller-supplied lookup, which
// is how a segment's content template reaches the values the segment
// computed. Command substitution and arithmetic are not part of this; a
// configuration that wants real shell reaches a segment function instead.

// Expander reads markup.
//
// The zero Expander resolves every reference to nothing and emits escapes
// plainly, which is what a test wants and what a caller with no editor
// underneath it wants.
type Expander struct {
	// Lookup resolves ${NAME}. A nil Lookup, and a name it does not know,
	// both expand to nothing: an unset reference is empty, never the literal
	// text of the reference.
	Lookup func(name string) string

	// Mark wraps each run of escape bytes so that something measuring the
	// result is not charged for cells the escapes do not occupy. A renderer
	// passes the editor's non-printing marker here; a nil Mark emits the
	// escapes plainly.
	//
	// It is a function rather than a constant so this package neither imports
	// the editor nor restates its markers, which are the interpreter's and
	// would go stale here the moment they moved.
	Mark func(escapes string) string

	// Base is the appearance the value starts in and returns to, which is the
	// segment's own when the layout is expanding a segment's content.
	//
	// It is what makes markup *inside* a segment composable: %f means the
	// segment's foreground rather than the terminal's, so a value that colors
	// one word of itself does not knock out the background the layout painted
	// around it. The zero Base is the terminal's own, and then nothing is
	// emitted until the value asks for something — which is what lets the
	// layout tell a segment that rendered from one that declined.
	Base Style
}

// Expand resolves markup into terminal bytes.
func (e Expander) Expand(s string) string {
	if s == "" {
		return ""
	}
	var (
		b       strings.Builder
		touched bool
	)
	style := e.Base
	if !e.Base.Empty() {
		b.WriteString(e.mark(e.Base.SGR()))
		touched = true
	}
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		switch {
		case runes[i] == '$' && i+1 < len(runes):
			name, last, ok := reference(runes, i)
			if !ok {
				b.WriteRune(runes[i])
				continue
			}
			i = last
			if e.Lookup != nil {
				b.WriteString(e.Lookup(name))
			}
		case runes[i] == '%' && i+1 < len(runes):
			last, changed, ok := escape(runes, i, &style, e.Base, &b)
			if !ok {
				b.WriteRune(runes[i])
				continue
			}
			i = last
			if changed {
				touched = true
				b.WriteString(e.mark(style.SGR()))
			}
		default:
			b.WriteRune(runes[i])
		}
	}
	out := b.String()
	// Leave the appearance as we found it, but only if we changed it. A value
	// that painted nothing must not emit anything, or the layout cannot tell
	// a segment that rendered from one that declined.
	back := e.mark(e.Base.SGR())
	if touched && !strings.HasSuffix(out, back) {
		out += back
	}
	return out
}

func (e Expander) mark(escapes string) string {
	if e.Mark == nil || escapes == "" {
		return escapes
	}
	return e.Mark(escapes)
}

// Expand reads markup with no references and no marking.
func Expand(s string) string { return Expander{}.Expand(s) }

// escape handles one %-escape beginning at i. It reports the index of the
// escape's last rune, whether the appearance changed, and whether this was an
// escape at all — anything it does not know is not an escape, and its percent
// sign is written out as itself.
func escape(runes []rune, i int, style *Style, base Style, b *strings.Builder) (last int, changed, ok bool) {
	switch runes[i+1] {
	case '%':
		b.WriteByte('%')
		return i + 1, false, true
	case 'f':
		style.Fg = base.Fg
		return i + 1, true, true
	case 'k':
		style.Bg = base.Bg
		return i + 1, true, true
	case 'B':
		style.Bold = true
		return i + 1, true, true
	case 'b':
		style.Bold = base.Bold
		return i + 1, true, true
	case 'U':
		style.Underline = true
		return i + 1, true, true
	case 'u':
		style.Underline = base.Underline
		return i + 1, true, true
	case 'F', 'K':
		spec, end, found := braced(runes, i+1)
		if !found {
			return i, false, false
		}
		color, _ := ParseColor(spec)
		if runes[i+1] == 'F' {
			style.Fg = color
		} else {
			style.Bg = color
		}
		return end, true, true
	}
	return i, false, false
}

// reference parses ${NAME} at i, returning the name and the index of its
// closing brace. A bare $NAME is not a reference: the vocabulary is the
// braced form, and everything else passes through as the text it is.
func reference(runes []rune, i int) (name string, last int, ok bool) {
	arg, end, found := braced(runes, i)
	if !found || !plainName(arg) {
		return "", i, false
	}
	return arg, end, true
}

// braced parses {arg} immediately after position i, returning the argument
// and the index of the closing brace.
func braced(runes []rune, i int) (arg string, last int, ok bool) {
	if i+1 >= len(runes) || runes[i+1] != '{' {
		return "", i, false
	}
	for j := i + 2; j < len(runes); j++ {
		if runes[j] == '}' {
			return string(runes[i+2 : j]), j, true
		}
	}
	return "", i, false
}

func plainName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !isNameRune(r) {
			return false
		}
	}
	return true
}

func isNameRune(r rune) bool {
	return r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}
