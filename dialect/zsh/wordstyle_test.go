// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/repl"
)

// The word styles: select-word-style and the -match widgets it puts in place,
// against rows measured from zsh 5.9.2 with its own copies of them (see the
// header of each testdata file). Nothing here reads zsh's function files; the
// probe called them. The shell style is left out: there zsh moves one
// character after a word into it whenever the cursor is inside the word, and
// that is recorded as a divergence in docs/spec/functions.md rather than
// reproduced.

// wordCharsDefault is `$WORDCHARS` as both shells start with it, which the
// rows were measured under; a bare Runner has none of the driver's defaults.
const wordCharsDefault = "WORDCHARS='*?_-.[]~=/&;!#$%^(){}<>'\n"

// wordStyleRows reads a testdata file of tab-separated rows, skipping the
// comment lines at its head.
func wordStyleRows(t *testing.T, name string) [][]string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var rows [][]string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := sc.Text(); line != "" && !strings.HasPrefix(line, "#") {
			rows = append(rows, strings.Split(line, "\t"))
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatalf("%s holds no rows", name)
	}
	return rows
}

// TestMatchWordsByStyleSplitsTheLineAsZshDoes: the seven parts, for every row
// measured.
func TestMatchWordsByStyleSplitsTheLineAsZshDoes(t *testing.T) {
	r, out := zleRunner(t, "fpath=("+shippedFunctionDir(t)+")\n"+wordCharsDefault+`autoload -Uz select-word-style match-words-by-style
w() {
	zstyle -d ':zle:*'; select-word-style $S
	BUFFER=$B; CURSOR=$C
	local curcontext=:zle:w
	local -a matched_words
	match-words-by-style
	print -r -- ${(j:|:)${(@qqqq)matched_words}}
}
zle -N w
`)
	for _, row := range wordStyleRows(t, "wordstyle-parts.tsv") {
		r.SetVar("B", row[0])
		r.SetVar("C", row[1])
		r.SetVar("S", row[2])
		_, ok, said := runWidget(t, r, out, "w", repl.Line{})
		if !ok {
			t.Fatalf("%q: the widget did not run", row)
		}
		if got := strings.TrimSuffix(said, "\n"); got != row[3] {
			t.Errorf("%q at %s, %s:\n got %s\nwant %s", row[0], row[1], row[2], got, row[3])
		}
	}
}

// TestTheWordWidgetsMoveKillAndChangeAsZshDoes: what each of the eight leaves
// in the line, the cursor and the kill, and the status it answers, for every
// row measured. The function is called directly rather than through `zle`,
// which here answers 0 whatever the function returned (#5939).
func TestTheWordWidgetsMoveKillAndChangeAsZshDoes(t *testing.T) {
	r, out := zleRunner(t, "fpath=("+shippedFunctionDir(t)+")\n"+wordCharsDefault+`autoload -Uz select-word-style
w() {
	zstyle -d ':zle:*'; select-word-style $S
	BUFFER=$B; CURSOR=$C; CUTBUFFER=
	$W-match
	local st=$?
	print -r -- "[$BUFFER|$CURSOR|$CUTBUFFER] st=$st"
}
zle -N w
`)
	rows := wordStyleRows(t, "wordstyle-widgets.tsv")
	for _, row := range rows {
		r.SetVar("B", row[0])
		r.SetVar("C", row[1])
		r.SetVar("S", row[2])
		r.SetVar("W", row[3])
		_, ok, said := runWidget(t, r, out, "w", repl.Line{})
		if !ok {
			t.Fatalf("%q: the widget did not run", row)
		}
		if got := strings.TrimSuffix(said, "\n"); got != row[4] {
			t.Errorf("%s on %q at %s, %s:\n got %s\nwant %s", row[3], row[0], row[1], row[2], got, row[4])
		}
	}
}

// select-word-style sets exactly the styles zsh's sets, per letter, and puts
// the eight widgets in place. Measured 2026-10-04 against zsh 5.9.2, each
// letter after `zstyle -d ':zle:*'`.
func TestSelectWordStyleSetsWhatZshSets(t *testing.T) {
	out, st := runShipped(t, wordCharsDefault+`autoload -Uz select-word-style
for s in bash normal shell whitespace default B N S W specified; do
	zstyle -d ':zle:*'; select-word-style $s; print -r -- "$s $?: ${(j:;:)${(f)"$(zstyle -L ':zle:*')"}};"
done
select-word-style bash; select-word-style shell; zstyle -L ':zle:*'
select-word-style q 2>/dev/null; print -r -- q=$?
zle -lL forward-word kill-word down-case-word`)
	const want = `bash 0: zstyle ':zle:*' skip-whitespace-first true;zstyle ':zle:*' word-chars '';zstyle ':zle:*' word-style standard;
normal 0: zstyle ':zle:*' skip-whitespace-first false;zstyle ':zle:*' word-chars '*?_-.[]~=/&;!#$%^(){}<>';zstyle ':zle:*' word-style standard;
shell 0: zstyle ':zle:*' skip-whitespace-first false;zstyle ':zle:*' word-style shell;
whitespace 0: zstyle ':zle:*' skip-whitespace-first false;zstyle ':zle:*' word-style space;
default 0: ;
B 0: zstyle ':zle:*' skip-whitespace-first true;zstyle ':zle:*' word-chars '';zstyle ':zle:*' word-style standard-subword;
N 0: zstyle ':zle:*' skip-whitespace-first false;zstyle ':zle:*' word-chars '*?_-.[]~=/&;!#$%^(){}<>';zstyle ':zle:*' word-style standard-subword;
S 0: zstyle ':zle:*' skip-whitespace-first false;zstyle ':zle:*' word-style shell-subword;
W 0: zstyle ':zle:*' skip-whitespace-first false;zstyle ':zle:*' word-style space-subword;
specified 0: zstyle ':zle:*' skip-whitespace-first false;zstyle ':zle:*' word-style shell;
zstyle ':zle:*' skip-whitespace-first false
zstyle ':zle:*' word-chars ''
zstyle ':zle:*' word-style shell
q=1
zle -N forward-word forward-word-match
zle -N kill-word kill-word-match
zle -N down-case-word down-case-word-match
`
	if out != want || st != 0 {
		t.Errorf("got (status %d)\n%s\nwant\n%s", st, out, want)
	}
}

// The usage, on standard error, byte for byte as zsh 5.9.2 writes it, for an
// unknown style.
func TestSelectWordStyleUsage(t *testing.T) {
	out, _ := runShipped(t, `autoload -Uz select-word-style
select-word-style foo 2>&1 >/dev/null; print -r -- st=$?`)
	const want = "Usage: select-word-style word-style\n" +
		"where word-style is one of the characters in parentheses:\n" +
		"(b)ash:       Word characters are alphanumerics only\n" +
		"(n)ormal:     Word characters are alphanumerics plus $WORDCHARS\n" +
		"(s)hell:      Words are command arguments using shell syntax\n" +
		"(w)hitespace: Words are whitespace-delimited\n" +
		"(d)efault:    Use default, no special handling (usually same as `n')\n" +
		"(q)uit:       Quit without setting a new style\n" +
		"\nst=1\n"
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

// delete-whole-word-match, and kill-whole-word-match made from it: the word
// the cursor is inside or at the start of goes; on the blanks between two
// words nothing does; and under a name holding `kill` what went is the kill.
// Measured 2026-10-04 against zsh 5.9.2 with select-word-style bash.
func TestDeleteWholeWordMatch(t *testing.T) {
	r, out := zleRunner(t, "fpath=("+shippedFunctionDir(t)+")\n"+`autoload -Uz select-word-style delete-whole-word-match
select-word-style bash
zle -N delete-whole-word-match
zle -N kill-whole-word-match delete-whole-word-match
`)
	for _, tc := range []struct {
		widget, buffer string
		cursor         int
		want           repl.Line
		cut            string
	}{
		{"delete-whole-word-match", "foo bar baz", 0, repl.Line{Buffer: " bar baz", Cursor: 0}, ""},
		{"delete-whole-word-match", "foo bar baz", 2, repl.Line{Buffer: " bar baz", Cursor: 0}, ""},
		{"delete-whole-word-match", "foo bar baz", 3, repl.Line{Buffer: "foo bar baz", Cursor: 3}, ""},
		{"delete-whole-word-match", "foo bar baz", 5, repl.Line{Buffer: "foo  baz", Cursor: 4}, ""},
		{"delete-whole-word-match", "foo bar baz", 11, repl.Line{Buffer: "foo bar ", Cursor: 8}, ""},
		{"delete-whole-word-match", "foo  bar", 4, repl.Line{Buffer: "foo  bar", Cursor: 4}, ""},
		{"delete-whole-word-match", "foo  bar", 5, repl.Line{Buffer: "foo  ", Cursor: 5}, ""},
		{"kill-whole-word-match", "foo bar baz", 5, repl.Line{Buffer: "foo  baz", Cursor: 4}, "bar"},
	} {
		ed := &stubEditor{}
		line, ok, said, _ := runWidgetWatching(t, r, out, tc.widget, repl.Line{Buffer: tc.buffer, Cursor: tc.cursor}, ed)
		if !ok || said != "" {
			t.Fatalf("%s on %q: ran %v, said %q", tc.widget, tc.buffer, ok, said)
		}
		if line.Buffer != tc.want.Buffer || line.Cursor != tc.want.Cursor || ed.cut != tc.cut {
			t.Errorf("%s on %q at %d: got %q @%d cut %q, want %q @%d cut %q", tc.widget, tc.buffer, tc.cursor,
				line.Buffer, line.Cursor, ed.cut, tc.want.Buffer, tc.want.Cursor, tc.cut)
		}
	}
}
