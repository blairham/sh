// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A prompt's numeric escapes: how long a run of octal digits is, and the
// hexadecimal spelling one column has and the others do not.
//
// The octal run used to be a flag, on the reading that a shorter run "is not a
// number at all". That is one column's answer; the other reads one, two or
// three digits and stops before the value would pass 255 — so `\400` is a
// space followed by the character `0` there and a NUL here. See [PromptOctal]
// for the nine probes both were measured over.
func TestAPromptsNumericEscapes(t *testing.T) {
	exact := PromptStyle{Escape: '\\', Octal: OctalExactlyThree}
	upTo := PromptStyle{
		Escape: '\\', Octal: OctalUpToThree,
		Hex: true, HexWithNoDigits: "?",
	}
	for _, c := range []struct {
		text      string
		wantExact string
		wantUpTo  string
	}{
		{`\101`, "A", "A"},
		{`\007`, "\a", "\a"},
		// A short run is two characters in one column and a byte in the other.
		{`\1|`, `\1|`, "\x01|"},
		{`\10|`, `\10|`, "\b|"},
		// A fourth digit is not part of the number in either.
		{`\1011`, "A1", "A1"},
		// Past 255. The exact-three column takes the low byte; the other one
		// stops at two digits rather than overflowing.
		{`\400`, "\x00", " 0"},
		// A digit that is not octal is not a number at all in either, so both
		// fall through to the code the table has no field for — written back
		// whole by this test's resolver, since what a dialect then does with
		// it is Unknown's question and not this one.
		{`\8`, `\8`, `\8`},
		// The hexadecimal spelling, which only one column has.
		{`\x41`, `\x41`, "A"},
		{`\x4|`, `\x4|`, "\x04|"},
		{`\xA1`, `\xA1`, "\xa1"},
		{`\x411`, `\x411`, "A1"},
		// An `x` with no hexadecimal digit after it is the substitute, and it
		// consumes the `x` alone.
		{`\xg`, `\xg`, "?g"},
		{`\x|`, `\x|`, "?|"},
	} {
		t.Run(c.text, func(t *testing.T) {
			if got := drawPrompt(t, exact, c.text); got != c.wantExact {
				t.Errorf("exactly three: %q drew %q, want %q", c.text, got, c.wantExact)
			}
			if got := drawPrompt(t, upTo, c.text); got != c.wantUpTo {
				t.Errorf("up to three: %q drew %q, want %q", c.text, got, c.wantUpTo)
			}
		})
	}
}

// The last component of the root directory is the separator in one column and
// nothing at all in the other — the difference between "the last component"
// and "the text after the last slash".
func TestTheLastComponentOfTheRootMayBeEmpty(t *testing.T) {
	base := PromptStyle{Escape: '\\', Codes: map[rune]PromptField{'W': FieldCwdBase}}
	empty := base
	empty.CwdBaseAtRootIsEmpty = true
	for _, c := range []struct{ dir, component, whenEmpty string }{
		{"/", "/", ""},
		{"/usr", "usr", "usr"},
		{"/usr/share", "share", "share"},
	} {
		t.Run(c.dir, func(t *testing.T) {
			if got := drawPromptIn(t, base, c.dir, `\W`); got != c.component {
				t.Errorf("in %s drew %q, want %q", c.dir, got, c.component)
			}
			if got := drawPromptIn(t, empty, c.dir, `\W`); got != c.whenEmpty {
				t.Errorf("in %s drew %q, want %q", c.dir, got, c.whenEmpty)
			}
		})
	}
}

func drawPrompt(t *testing.T, st PromptStyle, text string) string {
	t.Helper()
	return drawPromptIn(t, st, "", text)
}

func drawPromptIn(t *testing.T, st PromptStyle, dir, text string) string {
	t.Helper()
	r := newTestRunner(t, &Runner{
		Name: "testsh", Stdout: &strings.Builder{}, Stderr: &strings.Builder{},
	})
	r.SetPromptStyle(st)
	if dir != "" {
		r.SetVar("PWD", dir)
		r.SetVar("HOME", "/nowhere")
	}
	// A resolver that keeps an unknown code whole. The runner's own refuses
	// one by name — that split is deliberate and is documented at the walker
	// — and what is under test here is the *numeric* reading, so a code the
	// table has no field for is written back rather than ending the render.
	field := func(f PromptField, arg string, braced bool) (string, bool) {
		if f == FieldNone {
			return string(st.Escape) + arg, true
		}
		return r.PromptField(f, arg, braced)
	}
	out, refused, ok := ExpandPromptStyle(st, text, field, nil)
	if !ok {
		t.Fatalf("the table refused %q at %q", text, refused)
	}
	return out
}

// The expansion that runs in front of the table may be asked to leave the
// escape pairs alone, so that what the table reads is what the value held.
//
// Without it the pair is quoting to the *expander* and the escape is eaten:
// `\$` becomes a literal dollar before anything can ask who is typing, which
// is a shell whose own default prompt draws the wrong character for root.
func TestTheExpansionMaySkipThePromptsEscapes(t *testing.T) {
	skips := PromptStyle{
		Expand: PromptExpandsAlways, ExpandBeforeEscapes: true,
		ExpansionSkipsTheEscapes: true,
		Escape:                   '\\', Unknown: DropEscape,
		Codes: map[rune]PromptField{'w': FieldCwd},
		// Fixed strings rather than fields, so the rows are the same
		// whoever runs the test.
		Sequences: map[rune]string{'$': "#", '/': "[slash]"},
	}
	plain := skips
	plain.ExpansionSkipsTheEscapes = false
	for _, c := range []struct{ text, want, whenPlain string }{
		// The pair reaches the table, and the `$` it was in front of does
		// not expand. This is the row the field exists for.
		{`<\$HOME>`, "<#HOME>", "<$HOME>"},
		// A doubled escape collapses either way, and the `$` behind it does
		// expand — the control that says the collapse is not a suppression.
		{`<\\$HOME>`, "<[slash]nowhere>", "<[slash]nowhere>"},
		// A code out of a parameter is decoded either way, which is what
		// ExpandBeforeEscapes is for and what this must not undo.
		{`<$D>`, "</set/by/the/test>", "</set/by/the/test>"},
		// And a code the expander has no reading of is untouched by both.
		{`<\w>`, "</set/by/the/test>", "</set/by/the/test>"},
	} {
		t.Run(c.text, func(t *testing.T) {
			if got := renderWithVars(t, skips, c.text); got != c.want {
				t.Errorf("skipping: %q drew %q, want %q", c.text, got, c.want)
			}
			if got := renderWithVars(t, plain, c.text); got != c.whenPlain {
				t.Errorf("not skipping: %q drew %q, want %q", c.text, got, c.whenPlain)
			}
		})
	}
}

func renderWithVars(t *testing.T, st PromptStyle, text string) string {
	t.Helper()
	r := newTestRunner(t, &Runner{
		Name: "testsh", Stdout: &strings.Builder{}, Stderr: &strings.Builder{},
	})
	r.SetPromptStyle(st)
	r.SetVar("PWD", "/set/by/the/test")
	r.SetVar("HOME", "/nowhere")
	r.SetVar("D", `\w`)
	out, refused, ok := RenderPromptValue(st, r, text, r.PromptField, nil)
	if !ok {
		t.Fatalf("the table refused %q at %q", text, refused)
	}
	return out
}
