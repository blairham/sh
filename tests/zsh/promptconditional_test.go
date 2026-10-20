// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The conditional prompt escape, `%(x.true.false)`.
//
// It was the last thing a real interactive startup put on standard error
// (#1695), from powerlevel10k's own prompt-length routine — which measures a
// prompt by binary-searching on `%$y(l.1.0)`, so the construct has to be
// right and not merely present.
//
// Every case below was measured against zsh 5.9.2 before it was written, and
// the probes are the same text run through `${(%)…}` so that what is being
// pinned is the shell's answer and not a test's paraphrase of it.

// promptExpands runs one piece of prompt text through the expansion flag and
// says what it drew.
//
// Through a parameter rather than written into the source, so that the
// operand's own parsing — where a `)` ends a word, what a `%` means to the
// lexer — is nobody's question here. The arms are full of both characters.
func promptExpands(t *testing.T, dir, text string) (string, int) {
	t.Helper()
	return runZsh(t, dir, "v="+shquote(text)+`; print -r -- "[${(%)v}]"`)
}

// The test letters, one at a time, with the count swept where the count is
// what the letter compares against.
//
// The comparison is the measurement: `%2(?.…)` is false after a success and
// `%2(j.…)` is *true* with three jobs, so an equality and a floor are not
// interchangeable and neither can be guessed from the other.
func TestTheConditionalsTestLetters(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		// The status, which is an equality: measured, `%(?.T.F)` is `T`
		// after a success and `%5(?.T.F)` is `T` only after a command that
		// exited 5.
		{`true; print -r -- "${(%):-%(?.T.F)}"`, "T"},
		{`false; print -r -- "${(%):-%(?.T.F)}"`, "F"},
		{`(exit 5); print -r -- "${(%):-%5(?.T.F)}"`, "T"},
		{`(exit 5); print -r -- "${(%):-%4(?.T.F)}"`, "F"},
		{`(exit 5); print -r -- "${(%):-%6(?.T.F)}"`, "F"},
		// The sign is nothing to it, measured: `%-5(?.…)` answers exactly
		// where `%5(?.…)` does.
		{`(exit 5); print -r -- "${(%):-%-5(?.T.F)}"`, "T"},
		// The jobs, which is a floor.
		{`print -r -- "${(%):-%(j.T.F)}"`, "T"},
		{`print -r -- "${(%):-%1(j.T.F)}"`, "F"},
		// The shell level, also a floor, read from the shell's own variable.
		{`SHLVL=5; print -r -- "${(%):-%5(L.T.F)}${(%):-%6(L.T.F)}"`, "TF"},
		// How long the shell has been running.
		{`SECONDS=7; print -r -- "${(%):-%7(S.T.F)}${(%):-%8(S.T.F)}"`, "TF"},
		// The prompt array: how many elements it has, and whether the one
		// the count names is set and not empty. Measured with an empty third
		// element, which is what tells the two apart.
		{`psvar=(a b ''); print -r -- "${(%):-%3(v.T.F)}${(%):-%4(v.T.F)}"`, "TF"},
		{`psvar=(a b ''); print -r -- "${(%):-%2(V.T.F)}${(%):-%3(V.T.F)}"`, "TF"},
		// A count of nought names the first element, so a bare `%(V.…)` is a
		// question about `$psvar[1]`.
		{`psvar=(a); print -r -- "${(%):-%(V.T.F)}"`, "T"},
		{`psvar=(); print -r -- "${(%):-%(V.T.F)}"`, "F"},
		// The working directory, counted whole and counted with the home
		// directory written `~`. The `~` stands for every component it
		// replaced, which is what makes the two answers differ.
		{`cd /; print -r -- "${(%):-%(/.T.F)}${(%):-%1(/.T.F)}"`, "TF"},
		// Nothing is open in a script that has parsed, which is the same
		// answer `%_` gives here. Measured: every count above nought is `F`.
		{`print -r -- "${(%):-%(_.T.F)}${(%):-%1(_.T.F)}"`, "TF"},
	} {
		out, st := runZsh(t, dir, tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, got, st, tc.want)
		}
	}
}

// The working directory's two counts, which differ by exactly what the `~`
// stands for.
//
// Written against a home directory one level up from the working directory so
// that the *abbreviated* count is 2 — the marker and one component — whatever
// the path to the temporary directory happens to be. The unabbreviated count
// is that path's own depth, which is why only the abbreviated side is named
// here and the other is asked as a comparison.
func TestTheConditionalCountsTheWorkingDirectoryTwice(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "z"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := `HOME=` + shquote(dir) + `; cd ` + shquote(filepath.Join(dir, "z")) + `; ` +
		`print -r -- "${(%):-%2(c.T.F)}${(%):-%3(c.T.F)}` +
		`${(%):-%2(~.T.F)}${(%):-%2(..T.F)}${(%):-%3(/.T.F)}${(%):-%3(C.T.F)}"`
	out, st := runZsh(t, dir, src)
	// The two abbreviated answers, then the two spellings that share them,
	// then the unabbreviated pair — which counts the whole path and so is
	// true at 3 where the abbreviated one is not.
	if got := strings.TrimRight(out, "\n"); got != "TFTTTT" || st != 0 {
		t.Errorf("got %q (status %d), want %q", got, st, "TFTTTT")
	}
}

// The clock's four letters and the day of the week, each an equality and each
// measured against a pinned clock rather than against the day the test runs.
//
// The month is the one that cannot be guessed: it answered 8 in September, so
// it is the count of months already gone and not the month's number, and a
// reading that used the number is wrong for eleven months of the year and
// right for none of the ones a careless test would pick.
func TestTheConditionalsClock(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`print -r -- "${(%):-%8(D.T.F)}${(%):-%9(D.T.F)}"`, "TF"},
		{`print -r -- "${(%):-%10(d.T.F)}${(%):-%11(d.T.F)}"`, "TF"},
		{`print -r -- "${(%):-%11(T.T.F)}${(%):-%12(T.T.F)}"`, "TF"},
		{`print -r -- "${(%):-%9(t.T.F)}${(%):-%10(t.T.F)}"`, "TF"},
		{`print -r -- "${(%):-%4(w.T.F)}${(%):-%5(w.T.F)}"`, "TF"},
	} {
		out, st := runZshAt(t, promptConditionalClock, tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, got, st, tc.want)
		}
	}
}

// The grammar: where an arm ends, what a delimiter is, and how a nested
// construct is skipped.
//
// Three of these are what a reading that "looks for the next delimiter" gets
// wrong, and each was measured before it was written.
func TestTheConditionalsGrammar(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ text, want string }{
		// The delimiter is whatever character follows the letter, including
		// the parenthesis that closes the construct and a space.
		{`%(?.T.F)`, "T"},
		{`%(?:T:F)`, "T"},
		{`%(?|T|F)`, "T"},
		{`%(?)T)F)`, "T"},
		{`%(? T F )`, "T"},
		{`%(?%T%F)`, "T"},
		// The true arm ends at the delimiter and the false arm at the
		// closing parenthesis — so what follows the false arm's delimiter is
		// part of the false arm and what follows the true arm's is not.
		{`%(?.T.F.G)`, "T"},
		{`%(1?.T.F.G)`, "F.G"},
		{`%(?.T.F)tail`, "Ttail"},
		// Either arm may be empty.
		{`%(?..F)`, ""},
		{`%(?.T.)`, "T"},
		// An arm that runs off the end of the text is the rest of the text,
		// and a construct with no true arm at all draws nothing.
		{`%(?.T`, "T"},
		{`%(?.`, ""},
		{`%(?`, ""},
		{`%(`, ""},
		{`%(?.T)x`, "T)x"},
		{`%(1?.T)x`, ""},
		// The count may stand inside the parentheses, where it beats one in
		// front of the escape.
		{`%(0?.T.F)`, "T"},
		{`%2(0?.T.F)`, "T"},
		// A digit where the letter should be is the count, and the letter is
		// whatever follows it — which is what makes `%(0.T.F)` the path test
		// with `T` for a delimiter rather than a conditional on nought.
		{`%(0.T.F)`, ".F)"},
		{`%(.a.b)`, ".b)"},
		// Nesting. The inner construct is skipped whole while the outer arm
		// is scanned, so neither a delimiter nor a closing parenthesis
		// inside it ends anything belonging to the outer one.
		{`%(?.a%(1?.X.Y)b.c)`, "aYb"},
		{`%(1?.a.b%(?.X.Y)c)`, "bXc"},
		{`%(?.a%(?:X:Y)b.c)`, "aXb"},
		{`%(?.a%(1?.).Y)b.c)`, "aYb"},
		{`%(?.a%(1?.X.Y.Z)b.c)`, "aY.Zb"},
		{`%(?.%(?.i.j).k)`, "i"},
		{`%(1?.x.%(?.p.q))`, "p"},
		{`%(?.%(1?.a.b.c).d)`, "b.c"},
		// A bare parenthesis inside an arm is a character and not a nesting.
		{`%(?.(.)`, "("},
		{`%(?.).x)`, ")"},
		// An escape hides the character after it, so a code whose letter *is*
		// the delimiter does not end the arm.
		{`%(1?.a.b%%c)`, "b%c"},
		{`%(1?.x.a%)b)`, "a)b"},
		// And the arms hold further escapes, which are drawn rather than
		// written out.
		{`%(?.%%.b)`, "%"},
		{`%(?.%F{red}R%f.no)`, "\x1b[31mR\x1b[39m"},
		// The character that closes the construct, written after an escape,
		// is itself — wherever it stands.
		{`a%)b`, "a)b"},
		// And the character that opens one swallows the rest of the text
		// when there is nothing to close it.
		{`a%(b`, "a"},
		{`%)`, ")"},
	} {
		out, st := promptExpands(t, dir, tc.text)
		if got := strings.TrimRight(out, "\n"); got != "["+tc.want+"]" || st != 0 {
			t.Errorf("%q = %s (status %d), want [%s]", tc.text, got, st, tc.want)
		}
	}
}

// A letter the prompt language has no test for is **not** a refusal.
//
// Measured: `%(a.T.F)X` is `X` in zsh 5.9.2, at status 0 and with nothing on
// standard error — so an unknown letter draws nothing and swallows both arms.
// The paired half is below: a letter that *is* in the table and that a script
// cannot count is refused by name.
func TestALetterWithNoTestDrawsNothingAndIsNotRefused(t *testing.T) {
	dir := t.TempDir()
	for _, text := range []string{`%(a.T.F)X`, `%(z.T.F)X`, `%(%.T.F)X`} {
		out, st := promptExpands(t, dir, text)
		if got := strings.TrimRight(out, "\n"); got != "[X]" || st != 0 {
			t.Errorf("%q = %s (status %d), want [X] at status 0", text, got, st)
		}
	}
}

// And the letter that is in the table and cannot be answered here.
//
// `%(e.…)` counts the function calls and evals the expansion is inside, which
// a Runner's frames are not: an `eval` adds to that count and nothing to the
// frames, so an answer taken from them would be right for a function and
// wrong for the construct. Refused by name, with the character that opened
// the construct in front of the letter — the letter alone is not an escape
// and would send a reader looking for the wrong thing.
func TestATestLetterWithNoAnswerIsRefusedByName(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `print -r -- "${(%):-%(e.y.n)}"`)
	want := "zsh:1: ${(%):-%(e.y.n)}: the %(e prompt escape is not implemented\n"
	if out != want || st == 0 {
		t.Errorf("got %q (status %d), want %q at a failure", out, st, want)
	}
}

// `%(l.…)`: how many columns have already been drawn on this line.
//
// The one condition that is a fact about the prompt being drawn rather than
// about the shell drawing it, and the one powerlevel10k's `_p9k_prompt_length`
// binary-searches on. What counts and what does not is measured: a color
// sequence is bytes the terminal reads and not a column, the text between
// `%{ %}` is hidden, and `%G` is a column with no text at all.
func TestTheColumnCondition(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ text, want string }{
		{`%(l.T.F)`, "T"},
		{`%1(l.T.F)`, "F"},
		{`x%1(l.T.F)`, "xT"},
		{`ab%2(l.T.F)`, "abT"},
		{`ab%3(l.T.F)`, "abF"},
		// A conditional's own output is part of what the next one counts.
		{`ab%2(l.T.F)cd%4(l.T.F)`, "abTcdT"},
		// A color is not a column.
		{"%F{red}abc%3(l.T.F)", "\x1b[31mabcT"},
		{"%F{red}abc%4(l.T.F)", "\x1b[31mabcF"},
		// Nor is anything between the non-printing markers, though the text
		// itself is still drawn.
		{`%{XY%}ab%2(l.T.F)`, "XYabT"},
		{`%{XY%}ab%3(l.T.F)`, "XYabF"},
		// `%G` is the other way round: a column and no text.
		{`a%Gb%3(l.T.F)`, "abT"},
		{`a%Gb%4(l.T.F)`, "abF"},
		{`%{a%Gb%}%1(l.T.F)`, "abT"},
		{`%{a%Gb%}%2(l.T.F)`, "abF"},
		// Columns and not characters: an East Asian wide character is two,
		// a combining mark is none, and a tab reaches the next multiple of
		// eight.
		{"日本%4(l.T.F)", "日本T"},
		{"日本%5(l.T.F)", "日本F"},
		{"héllo%5(l.T.F)", "hélloT"},
		{"héllo%6(l.T.F)", "hélloF"},
		{"\t%8(l.T.F)", "\tT"},
		{"\t%9(l.T.F)", "\tF"},
		// A newline starts the count again.
		{"ab\ncd%2(l.T.F)", "ab\ncdT"},
		{"ab\ncd%3(l.T.F)", "ab\ncdF"},
		// A negative count asks about the space left rather than the space
		// used, and it is the only test the sign means anything to.
		{`xxx%-7(l.T.F)`, "xxxT"},
		{`xxx%-8(l.T.F)`, "xxxF"},
	} {
		out, st := runZsh(t, dir, "COLUMNS=10; v="+shquote(tc.text)+`; print -r -- "[${(%)v}]"`)
		if got := strings.TrimRight(out, "\n"); got != "["+tc.want+"]" || st != 0 {
			t.Errorf("%q = %s (status %d), want [%s]", tc.text, got, st, tc.want)
		}
	}
}

// The column wraps at the shell's own COLUMNS, which is what makes the
// routine this whole escape is for work: it sets `local -i COLUMNS=1024` and
// measures inside that.
//
// Two wraps rather than one, and each has a case here that the other gets
// wrong. A character that will not *fit* starts a new line before it is
// drawn — three wide characters at width five leave the column at two, not at
// nought — and a line filled exactly wraps after, which is what leaves five
// narrow characters at width five at nought.
func TestTheColumnWrapsAtColumns(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		cols, text, want string
	}{
		{"5", `xxxxx%(l.T.F)`, "xxxxxT"},
		{"5", `xxxxx%1(l.T.F)`, "xxxxxF"},
		{"5", "日日日%2(l.T.F)", "日日日T"},
		{"5", "日日日%3(l.T.F)", "日日日F"},
		{"5", "x日日%(l.T.F)", "x日日T"},
		{"5", "x日日%1(l.T.F)", "x日日F"},
		{"3", "x日%(l.T.F)", "x日T"},
		{"3", "x日%1(l.T.F)", "x日F"},
		// A character wider than the whole line is not wrapped a second
		// time, which is the only thing that explains a width of one.
		{"1", "日%2(l.T.F)", "日T"},
		{"1", "日%3(l.T.F)", "日F"},
		{"1", "日x%(l.T.F)", "日xT"},
		{"1", "日x%1(l.T.F)", "日xF"},
		// A width of nought is a line that never fits anything, so what is
		// left on it is the last character alone. Measured, and it is what
		// `COLUMNS` unset and `COLUMNS=abc` both come to.
		{"0", `xxxxx%1(l.T.F)`, "xxxxxT"},
		{"0", `xxxxx%2(l.T.F)`, "xxxxxF"},
		{"abc", `xxxxx%1(l.T.F)`, "xxxxxT"},
		{"abc", `xxxxx%2(l.T.F)`, "xxxxxF"},
		// A width below nought is not a line at all: every count at or above
		// nought is false, including nought itself, and every count below it
		// is true.
		{"-1", `xxxxx%(l.T.F)`, "xxxxxF"},
		{"-1", `xxxxx%-1(l.T.F)`, "xxxxxT"},
	} {
		src := "COLUMNS=" + tc.cols + "; v=" + shquote(tc.text) + `; print -r -- "[${(%)v}]"`
		out, st := runZsh(t, dir, src)
		if got := strings.TrimRight(out, "\n"); got != "["+tc.want+"]" || st != 0 {
			t.Errorf("COLUMNS=%s %q = %s (status %d), want [%s]", tc.cols, tc.text, got, st, tc.want)
		}
	}
}

// Nothing inside a prompt-escape expansion is a pattern.
//
// This is the second half of what #1695 set out to remove, and it is why the
// startup carried *two* lines rather than one: the word a `:-` substitutes is
// expanded as a word and globs on its own, so `%$y(l.1.0)` was read as a glob
// qualifier list and named a file attribute before the escape was even
// reached. Measured, `${(%):-ab(N)}` unquoted draws the seven characters,
// where `${(U):-ab(N)}` — which also rewrites its result — is generated away.
func TestNothingInAPromptExpansionIsAPattern(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`print -r -- ${(%):-ab(N)}`, "ab(N)"},
		{`print -r -- ${(%):-ab(#q.)}`, "ab(#q.)"},
		{`COLUMNS=10; y=2; print -r -- ${(%):-ab%$y(l.1.0)}`, "ab1"},
		// The line the issue was filed for, from powerlevel10k's own
		// prompt-length routine: the last character of the expansion is the
		// bit the routine reads.
		{`COLUMNS=10; y=2; print -r -- ${${(%):-ab%$y(l.1.0)}[-1]}`, "1"},
	} {
		out, st := runZsh(t, dir, tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, got, st, tc.want)
		}
	}
}

// The two spellings of the expansion agree about the conditional, the way
// they agree about everything else in the one table.
func TestPrintPAndTheFlagAgreeAboutTheConditional(t *testing.T) {
	dir := t.TempDir()
	for _, text := range []string{`%(?.y.n)`, `%1(l.a.b)`, `a%Gb%3(l.T.F)`, `%(?.%(1?.a.b.c).d)`} {
		src := "v=" + shquote(text) + `; a=$(print -rP -- "$v"); b="${(%)v}"; ` +
			`[[ "$a" == "$b" ]] && print -r -- AGREE || printf 'print=%q flag=%q\n' "$a" "$b"`
		out, st := runZsh(t, dir, src)
		if out != "AGREE\n" || st != 0 {
			t.Errorf("%q: %s (status %d), want the two spellings to agree", text, strings.TrimRight(out, "\n"), st)
		}
	}
}

// promptConditionalClock is the moment the clock's conditionals are asked
// about: 11:09 on Thursday the tenth of September 2026, UTC.
//
// A pinned moment rather than the day the test runs, because every one of
// these letters is an equality and a test that read the real clock would
// assert nothing on any other day. The values are the ones measured against
// zsh 5.9.2 at that moment — in particular the month, which answered 8.
var promptConditionalClock = time.Date(2026, time.September, 10, 11, 9, 30, 0, time.UTC)
