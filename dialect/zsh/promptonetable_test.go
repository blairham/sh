// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// One prompt-escape table, and the three claims that say so.
//
// There used to be two: `interp` carried four escapes and refused the rest by
// name, and `repl.PromptStyle` carried about forty for the prompt *drawer*. So
// this shell drew `%F{196}` at a prompt and refused it when a script wrote
// `print -P '%F{196}…'` — one question, two answers, and the larger table was
// the one a script could not reach (#1090).
//
// The claims, in the order they have to hold:
//
//  1. the type is one type, which the compiler checks below;
//  2. the *value* the interpreter reads is the value the drawer is handed,
//     which TestTheDrawerAndTheInterpreterReadOneTable checks;
//  3. the two spellings of the script's reader agree byte for byte, which
//     TestPrintPAndThePercentFlagAgree checks — #1091 established that
//     `print -P` *is* the `${(%)…}` expansion under another name, and a test
//     is what keeps that from becoming an assumption.

// Claim 1, at compile time: `repl.PromptStyle` is an alias for
// [interp.PromptStyle] rather than a copy of it. Two struct types with
// identical fields would satisfy neither of these without a conversion.
var (
	_ interp.PromptStyle = repl.PromptStyle{}
	_ repl.PromptStyle   = interp.PromptStyle{}
	_ repl.PromptField   = interp.FieldUser
	_ interp.PromptField = repl.FieldUser
)

// Claim 2: the table the interpreter answers from is the table the prompt
// drawer is handed.
//
// `Apply` installs it, so a runner this dialect has been applied to answers
// prompt escapes from the same value `PromptStyle()` gives the editor. Asserted
// as the whole value rather than a spot check: a row added to one and not the
// other is exactly the drift this is about, and a comparison of the whole
// table is the only assertion that cannot miss one.
func TestTheDrawerAndTheInterpreterReadOneTable(t *testing.T) {
	// Built through the preset so that the runner is told which shell it is,
	// which every runner in this suite has to be: a nil Dialect is the core,
	// and a nested parse would then run as a shell these tests are not about.
	r := preset.Runner(dialecttest.Base{Dir: t.TempDir()})
	drawn := zsh.PromptStyle()
	got := r.PromptStyleValue()
	// The one field that cannot be compared by value: reflect.DeepEqual
	// calls two non-nil funcs different however they were built, so leaving
	// Expand in would fail this for every table including a table compared
	// with itself. It is compared by *identity* instead — the same function,
	// not merely one that answers alike — and then taken out of both copies
	// so the rest is still asserted whole.
	drawnExpand, gotExpand := expandOf(drawn), expandOf(got)
	if drawnExpand == 0 {
		t.Error("the dialect's Expand is nil, so `setopt prompt_subst` could never turn it on")
	}
	if gotExpand != drawnExpand {
		t.Errorf("the interpreter's Expand is not the drawer's: %v against %v", gotExpand, drawnExpand)
	}
	got.Expand, drawn.Expand = nil, nil
	if !reflect.DeepEqual(got, drawn) {
		t.Errorf("the interpreter's table is not the drawer's:\n interp: %+v\n drawer: %+v", got, drawn)
	}
	// And it is not the empty table, which would make the comparison above
	// pass for the wrong reason.
	if drawn.Escape == 0 || len(drawn.Codes) == 0 {
		t.Fatalf("the dialect's table is empty: %+v", drawn)
	}
}

// Claim 3: `print -P` and `${(%)…}` are one expansion, byte for byte.
//
// Every case is written twice in one snippet and compared to itself, so the
// assertion is the *agreement* and not a second copy of what either answers.
// The bytes are compared with the escape sequences in them — a test that
// stripped them would pass while a color was wrong, which is the failure a
// prompt test is most exposed to.
func TestPrintPAndThePercentFlagAgree(t *testing.T) {
	dir := t.TempDir()
	for _, text := range []string{
		// The color family, which is what #1099 was about and what the one
		// table delivered: a script could not reach these at all before.
		`%F{red}x%f`,
		`%F{196}x%f`,
		`%F{#ff8800}x%f`,
		`%K{blue}x%k`,
		`%F{bogus}`,
		`%F`,
		`%F{}`,
		`%Fred`,
		// The visual sequences.
		`%B%U%S%b%u%s%E`,
		// The fields the interpreter holds.
		`%%`, `%n`, `%m`, `%M`, `%~`, `%d`, `%/`, `%c`, `%C`, `%#`,
		`%?`, `%j`, `%_`, `%{X%}`, `%x`, `%N`,
		// The clock, whose two readers have two clocks and one formatter —
		// asked as the *shape* of the answer rather than the moment, since a
		// second between the two calls would otherwise fail the test.
		`%D{%Y}`, `%W`,
		// And the shapes with no code in them at all.
		`plain text`, `100%%`, `x%`, ``,
	} {
		// `print -rP` so the backslash pass cannot touch the text, and the
		// flag spelling beside it on the same line so the two are asked of
		// one runner in one moment. The marker is computed by the shell and
		// cannot appear in either answer.
		//
		// The text goes through a *parameter* rather than through a `:-`
		// operand written into the source, and that is not a style choice:
		// half of these cases contain a `}`, and a `}` in the operand ends
		// the expansion. Measured on zsh 5.9.2, 2026-09-08 — every
		// brace-carrying row disagrees when written as `"${(%):-%F{red}x%f}"`,
		// and it is the *flag* side that is short, `$'\033'[31mx%f}` against
		// `$'\033'[31mx$'\033'[39m`, because the operand stopped at the first
		// brace and `x%f}` arrived as ordinary text after it. The same seven
		// rows agree through a parameter, in zsh and here alike.
		//
		// This test read as though it pinned the brace cases and pinned the
		// opposite: it passed only while this shell balanced a bare `{` inside
		// a double-quoted expansion, which zsh does not do, so the row that
		// looked like coverage of `%F{red}` was really a record of #1586's
		// bug. Writing it through a parameter asks the question the claim is
		// about — that the two spellings are one expansion — and leaves where
		// an operand *ends* to the tests that are about that.
		src := "v=" + shquote(text) + "; a=$(print -rP -- \"$v\"); b=\"${(%)v}\"; " +
			`[[ "$a" == "$b" ]] && print -r -- "AGREE" || printf 'print=%q flag=%q\n' "$a" "$b"`
		out, st := runZsh(t, dir, src)
		if out != "AGREE\n" || st != 0 {
			t.Errorf("%q: %s (status %d), want the two spellings to agree", text, strings.TrimRight(out, "\n"), st)
		}
	}
}

// What is still refused by name once there is one table, and it is a shorter
// list than before: the working directory, the host, the clock and the exit
// status all became *answers* the moment the interpreter read the dialect's
// table (#1090). Three kinds remain, and each is refused rather than guessed.
//
// printprompt_test.go already pins that the two spellings refuse alike and in
// which words. This is the *set* — the shapes a reader would otherwise assume
// had quietly started working.
func TestTheRefusalsThatRemainAfterOneTable(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`print -P '%q'`, "zsh:print:1: the %q prompt escape is not implemented\n"},
		{`echo "${(%):-%q}"`, "zsh:1: ${(%):-%q}: the %q prompt escape is not implemented\n"},
		// The ternary is answered now (#1695), and what remains of it is a
		// *test letter* the table has and this reader cannot count: the
		// eval depth. Named by the character that opens the construct and
		// the letter together, because the letter alone is not an escape.
		{`print -P '%(e.y.n)'`, "zsh:print:1: the %(e prompt escape is not implemented\n"},
		{`echo "${(%):-%(e.y.n)}"`, "zsh:1: ${(%):-%(e.y.n)}: the %(e prompt escape is not implemented\n"},
		// A code whose value is a fact about a *session*: the table has
		// the row, and a runner reading a script has no history to number.
		{`print -P '%h'`, "zsh:print:1: the %h prompt escape is not implemented\n"},
		{`print -P '%y'`, "zsh:print:1: the %y prompt escape is not implemented\n"},
		// The one *before* the one that fails is still the one named: a word
		// holding several escapes says which of them this shell could not
		// answer.
		{`print -P '%n%h'`, "zsh:print:1: the %h prompt escape is not implemented\n"},
	} {
		out, st := runZsh(t, dir, tc.src)
		if out != tc.want || st == 0 {
			t.Errorf("%s = %q (status %d), want %q at a failure", tc.src, out, st, tc.want)
		}
	}
}

// The code whose braces are a `strftime` format, asked of the *script's*
// reader.
//
// Measured through a pty and again under `-c`: `%D` is the numeric date,
// `%D{%H:%M}` is the clock through that format, and `%D{}` is **nothing at
// all**. The last one is the case with an answer of its own — an empty format
// is a format, and the absence of one is the plain date — and a mutant that
// treated the two alike survived until this was written.
//
// The year is what a test can name without naming the day it ran; the plain
// shape is asked for by its width, which is fixed.
func TestTheTimeFormatCodeInAScript(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`print -rP '[%D{%Y}]'`, "[" + timeNow().Format("2006") + "]\n"},
		// Empty braces: nothing, and not the date.
		{`print -rP '[%D{}]'`, "[]\n"},
		{`v='[%D{}]'; print -r -- "${(%)v}"`, "[]\n"},
		// No braces: the plain numeric date, `yy-mm-dd`, asked for by its
		// width because that is the part of a date a test can name.
		{`v=$(print -rP '%D'); print -r -- "n=${#v}"`, "n=8\n"},
		// And the letters after an unbraced code are text.
		{`v=$(print -rP '%Dz'); print -r -- "n=${#v} tail=${v[-1]}"`, "n=9 tail=z\n"},
	} {
		out, st := runZsh(t, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// shquote is single quotes with the awkward one doubled back, for putting a
// prompt text into a snippet without the shell reading it twice.
func shquote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// timeNow is the wall clock, for the one assertion that has to name a piece
// of it. A year is the largest part a test can name and still be right the
// next time it runs.
func timeNow() time.Time { return time.Now() }

// A `}` written into a `${(%):-…}` operand ends the expansion, so the prompt
// escape that carries one cannot be spelled that way at all.
//
// It is the prompt-side face of #1586 and it is measured rather than reasoned:
// on zsh 5.9.2, 2026-09-08, `"${(%):-%F{red}x%f}"` is `$'\033'[31mx%f}` — the
// operand stopped at the first brace, `%F{red` was prompt-expanded to the red
// escape, and `x%f}` came out as the ordinary text that followed the closing
// brace. `print -rP '%F{red}x%f'` answers `$'\033'[31mx$'\033'[39m` instead,
// which is the whole escape.
//
// This is here because the disagreement looks exactly like a broken prompt
// table and is not one: the two spellings *are* one expansion, and what
// differs is where the shell decided the operand stopped. A previous version
// of TestPrintPAndThePercentFlagAgree asserted these agreed, and passed only
// while this shell nested a bare `{` inside double quotes — so the row that
// looked like the strongest coverage in the file was a record of a lexer bug.
func TestABraceInTheFlagOperandEndsIt(t *testing.T) {
	dir := t.TempDir()
	// Asked as the tail of the answer and as the disagreement, rather than
	// byte for byte: the escape itself is a color this test is not about, and
	// `printf %q` renders it differently here than zsh does, which would make
	// this a test of the quoter.
	out, st := runZsh(t, dir, `b="${(%):-%F{red}x%f}"; a=$(print -rP -- '%F{red}x%f')
[[ $b == *'x%f}' ]] && print -r -- "STOPPED-AT-BRACE"
[[ $a == $b ]] || print -r -- "AND-DIFFERS-FROM-PRINT-P"`)
	const want = "STOPPED-AT-BRACE\nAND-DIFFERS-FROM-PRINT-P\n"
	if out != want || st != 0 {
		t.Errorf("the flag operand with a brace = %q (status %d), want %q", out, st, want)
	}
}

// expandOf is a PromptStyle.Expand as a comparable identity, which is the
// only way to ask whether two tables carry the *same* function.
func expandOf(st interp.PromptStyle) uintptr {
	return reflect.ValueOf(st.Expand).Pointer()
}
