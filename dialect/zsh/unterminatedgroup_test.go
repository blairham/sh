// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A pattern group nothing closes is a **bad pattern**, on the same footing as
// an unterminated bracket and reachable by the same option.
//
// It was a *lexical* refusal until #4645 — `unterminated pattern group`,
// raised while reading the word — which is wrong twice over. The sentence
// does not name the offending word, and, the larger half, the refusal arrives
// from a layer `setopt badpattern` cannot reach: the option is read where
// filenames are generated and a word that never parsed never gets there. So
// the one row that most sharply distinguishes the two readings of #4630's
// rule was answered wrong in *both* states.
//
// **The noun is a pattern that will not compile.** Not "an unterminated
// group" and not "a word carrying a parenthesis" — and the rows that say so
// are here rather than in a comment, because a grid that varies a lot of
// things while holding the deciding one fixed reads exactly like
// confirmation:
//
//   - `a(b|c)` carries two parentheses, compiles, and does not move.
//   - `a[(]b` carries one, compiles — the bracket makes it a character — and
//     does not move.
//   - `[a` carries no parenthesis at all, is not a group by any reading, and
//     moves **identically** to `a(b`.
//
// So the rule is keyed on the compile and not on the construct, and the last
// row is the one that proves it: hold "will not compile" fixed, change
// "is a group", and nothing moves.
//
// Measured against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* —
// run `-f`, 2026-09-26, every row in **both** option states.

// unterminatedGroupDir is the fixture every case here runs in:
//
//	a(b     the name a word spared by the option would match, were it a
//	        pattern — and the file `*(b` must not match, which is what says
//	        the spared word is handed back whole
//	ab, ac  what `a(b|c)` matches, so the control has something to find
func unterminatedGroupDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"a(b", "ab", "ac"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAnUnterminatedPatternGroupIsABadPattern(t *testing.T) {
	for _, c := range []struct {
		name, src        string
		on, off          string
		onStatus, offSt  int
		onErr, offErrStr string
	}{
		{
			// The issue's own reduction. On `main` this was
			// `unterminated pattern group` in both states, at status 1,
			// with nothing after it run.
			name: "a group opened mid-word",
			src:  `print -r -- a(b`,
			on:   "", onStatus: 1, onErr: "bad pattern: a(b",
			off: "a(b\n", offSt: 0,
		},
		{
			// The word-initial spelling, which is the parser's question
			// rather than the lexer's: a leading `(` opens a subshell
			// unless a word may begin with one here.
			name: "a group opening the word",
			src:  `print -r -- (a`,
			on:   "", onStatus: 1, onErr: "bad pattern: (a",
			off: "(a\n", offSt: 0,
		},
		{
			// A bar inside the unclosed group is the group's, so this is
			// one word rather than a pipeline — and it is still a pattern
			// that will not compile.
			name: "an alternation nothing closes",
			src:  `print -r -- (a|b`,
			on:   "", onStatus: 1, onErr: "bad pattern: (a|b",
			off: "(a|b\n", offSt: 0,
		},
		{
			// The `@` is an ordinary character in front of a bare group in
			// this dialect, so this is the same question wearing the
			// spelling the *other* two shells' quantified groups use.
			name: "a quantifier's spelling in front of a bare group",
			src:  `print -r -- a@(b`,
			on:   "", onStatus: 1, onErr: "bad pattern: a@(b",
			off: "a@(b\n", offSt: 0,
		},
		{
			// **Handed back whole, not read with the `(` as an ordinary
			// character.** The file `a(b` is there to be matched and is not
			// matched: a literal reading would answer `*(b` with it.
			name: "a live star in front of the group",
			src:  `print -r -- *(b`,
			on:   "", onStatus: 1, onErr: "bad pattern: *(b",
			off: "*(b\n", offSt: 0,
		},
		{
			// The escape mark a backslash leaves behind is ours and does
			// not belong in the sentence. Measured, the reference writes
			// `bad pattern: a(b)`; this shell wrote `a(b\)` until the same
			// unescaping the *miss* complaint has always done was applied
			// here.
			name: "an escaped parenthesis does not close the group",
			src:  `print -r -- a(b\)`,
			on:   "", onStatus: 1, onErr: "bad pattern: a(b)",
			off: "a(b)\n", offSt: 0,
		},
		{
			// **The control.** A group that closes compiles, so the option
			// has nothing to withhold and the pattern matches in both
			// states. Without this row the change could have been "stop
			// reading groups" and every row above would still pass.
			name: "a group that closes is the control",
			src:  `print -r -- a(b|c)`,
			on:   "ab ac\n", onStatus: 0,
			off: "ab ac\n", offSt: 0,
		},
		{
			// A parenthesis inside a bracket expression is a character of
			// the class and opens nothing, so this compiles and matches the
			// file. It is the row that says the scan steps over brackets:
			// counting the `(` in there would refuse a pattern with nothing
			// wrong with it, in both states.
			name: "a parenthesis inside a bracket is a character",
			src:  `print -r -- a[(]b`,
			on:   "a(b\n", onStatus: 0,
			off: "a(b\n", offSt: 0,
		},
		{
			// Quoting and escaping each take the parenthesis out of the
			// grammar, so neither word is a group at all.
			name: "a quoted parenthesis is not a group",
			src:  `print -r -- "a(b"`,
			on:   "a(b\n", onStatus: 0,
			off: "a(b\n", offSt: 0,
		},
		{
			name: "an escaped parenthesis is not a group",
			src:  `print -r -- a\(b`,
			on:   "a(b\n", onStatus: 0,
			off: "a(b\n", offSt: 0,
		},
		{
			// **A value is not a pattern in this dialect**, so the same
			// three characters arriving from one are ordinary text — and
			// nothing downstream may refuse them. A lone `(` is not a
			// metacharacter, so nothing escapes the value on its way out;
			// without resultReadsAsPattern being told about the group it
			// goes live, meets the new refusal in glob, and a `${v#a(b}`
			// reports the operand and then reports the value it handed
			// back. See #1386, which is the same gap through `[`.
			name: "a value carrying an unclosed group is text",
			src:  `v="a(b"; print -r -- $v`,
			on:   "a(b\n", onStatus: 0,
			off: "a(b\n", offSt: 0,
		},
		{
			// **The pair that pins the noun.** No parenthesis anywhere and
			// the same two answers, because both words fail the same
			// compile. This row is #4630's and is repeated here on purpose:
			// a rule keyed on "an unterminated group" would leave it behind.
			name: "a word with no group in it moves identically",
			src:  `print -r -- [a`,
			on:   "", onStatus: 1, onErr: "bad pattern: [a",
			off: "[a\n", offSt: 0,
		},
		{
			// It is per **word**: the control globs and the bad one stands,
			// in one expansion.
			name: "one good word and one bad one",
			src:  `print -r -- a(b|c) a(b`,
			on:   "", onStatus: 1, onErr: "bad pattern: a(b",
			off: "ab ac a(b\n", offSt: 0,
		},
		{
			// A redirection's target is filename generation too.
			name: "a redirection target",
			src:  `: > a(b && print OK`,
			on:   "", onStatus: 1, onErr: "bad pattern: a(b",
			off: "OK\n", offSt: 0,
		},
		{
			// An expansion result made live by `${~…}` reaches the same
			// gate, so it moves with the option like the written spelling.
			name: "an expansion result read as a pattern",
			src:  `L="a(b"; print -r -- ${~L}`,
			on:   "", onStatus: 1, onErr: "bad pattern: a(b",
			off: "a(b\n", offSt: 0,
		},
		{
			// A prefix trim compiles its operand as a pattern too, and
			// #4646 made that surface refuse one it will not compile — for
			// a bracket. The same scan answers a group, so the two changes
			// compose here rather than growing a second gate. It does not
			// move with the option: `${v#…}` is not filename generation.
			name: "a prefix trim is refused in both states",
			src:  `v="a(bZ"; print -r -- ${v#a(b}`,
			on:   "", onStatus: 1, onErr: "bad pattern: a(b",
			off: "", offSt: 1, offErrStr: "bad pattern: a(b",
		},
		{
			name: "a suffix trim is refused in both states",
			src:  `v="Za(b"; print -r -- ${v%a(b}`,
			on:   "", onStatus: 1, onErr: "bad pattern: a(b",
			off: "", offSt: 1, offErrStr: "bad pattern: a(b",
		},
		{
			// The refusal is eager, for the reason #4646's is: the question
			// is asked of the **pattern**, so a subject the pattern could
			// never have matched is refused just the same. Measured,
			// `v=zzz; ${v#a(b}` is `bad pattern: a(b` at 1.
			name: "a trim is refused on a subject it could not match",
			src:  `v="zzz"; print -r -- ${v#a(b}`,
			on:   "", onStatus: 1, onErr: "bad pattern: a(b",
			off: "", offSt: 1, offErrStr: "bad pattern: a(b",
		},
		{
			// **The control the trim rows need**: a pattern arriving from a
			// value is not a pattern in this dialect, so the same three
			// characters strip nothing and refuse nothing. Without this row
			// the gate could have been reading the subject's own text.
			name: "a trim whose pattern came from a value is text",
			src:  `v="a(bZ"; p="a(b"; print -r -- ${v#$p}`,
			on:   "Z\n", onStatus: 0,
			off: "Z\n", offSt: 0,
		},
		{
			// **And the half that does not move**, which is what says the
			// switch is about filename generation rather than about the
			// matcher. An element filter is not a glob, and measured it is
			// refused with the option off exactly as with it on — the same
			// text as the first row of this table, and only one of them
			// moves.
			name: "an element filter is refused in both states",
			src:  `v="a(b"; print -r -- "X${v:#a(b}Y"`,
			on:   "", onStatus: 1, onErr: "bad pattern: a(b",
			off: "", offSt: 1, offErrStr: "bad pattern: a(b",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct {
				name, setopt, want, wantErr string
				status                      int
			}{
				{"on", "setopt badpattern\n", c.on, c.onErr, c.onStatus},
				{"off", "unsetopt badpattern\n", c.off, c.offErrStr, c.offSt},
			} {
				t.Run(state.name, func(t *testing.T) {
					dir := unterminatedGroupDir(t)
					out, st, errs := runZshSplit(t, dir, state.setopt+c.src)
					if out != state.want {
						t.Errorf("stdout = %q, want %q", out, state.want)
					}
					if st != state.status {
						t.Errorf("status = %d, want %d", st, state.status)
					}
					switch {
					case state.wantErr == "" && errs != "":
						t.Errorf("stderr = %q, want nothing", errs)
					case state.wantErr != "" && !strings.Contains(errs, state.wantErr):
						t.Errorf("stderr = %q, want %q in it", errs, state.wantErr)
					}
				})
			}
		})
	}
}

// The unclosed group does not swallow the rest of the program, and that is
// the half a "stop refusing and take everything" change would get wrong.
//
// The group scan ends the word at an unquoted `;`, `<`, `>` or `&` wherever
// it stands, which it already did — see scanGroupSpans — so the only thing
// #4645 moved was what happens when the *input* runs out. Measured on zsh
// 5.9.2 (`-f`, 2026-09-26) with `unsetopt badpattern`: `print -r -- a(b;
// print AFTER` writes `a(b` and then `AFTER`, and a newline does **not** end
// it, so a group left open on the last line takes the rest of the file.
func TestAnUnterminatedGroupEndsTheWordAtAnOperator(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{
			"a semicolon ends the word and the next command runs",
			"unsetopt badpattern\nprint -r -- a(b; print AFTER\n",
			"a(b\nAFTER\n",
		},
		{
			"a blank does not end it",
			"unsetopt badpattern\nprint -r -- a(b c; print AFTER\n",
			"a(b c\nAFTER\n",
		},
		{
			// The other side of the same rule, and the one that says the
			// word really does run to the end of the input: the second
			// line is text of the first word rather than a command, and the
			// file's **final newline is in the word too**. Measured byte for
			// byte against zsh 5.9.2 from a script file — `a(b\n\n`, the
			// first newline the word's and the second `print`'s own.
			"a newline does not end it either",
			"unsetopt badpattern\nprint -r -- a(b\nprint AFTER\n",
			"a(b\nprint AFTER\n\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := unterminatedGroupDir(t)
			out, st, errs := runZshSplit(t, dir, c.src)
			if out != c.want {
				t.Errorf("stdout = %q, want %q", out, c.want)
			}
			if st != 0 || errs != "" {
				t.Errorf("status = %d stderr = %q, want 0 and nothing", st, errs)
			}
		})
	}
}

// `emulate sh` and `emulate ksh` turn the option off with nobody typing
// `setopt`, and they reach this refusal for the same reason they reach the
// bracket's: there is one switch and one read site.
//
// Measured on zsh 5.9.2 (`-f`, 2026-09-26): both leave `a(b` standing at 0,
// and `emulate zsh` refuses it.
func TestEmulationReachesAnUnterminatedGroup(t *testing.T) {
	for _, c := range []struct {
		emulate, want string
		status        int
		wantErr       bool
	}{
		{"sh", "a(b\n", 0, false},
		{"ksh", "a(b\n", 0, false},
		{"zsh", "", 1, true},
	} {
		t.Run(c.emulate, func(t *testing.T) {
			dir := unterminatedGroupDir(t)
			src := "emulate " + c.emulate + "\nprint -r -- a(b"
			out, st, errs := runZshSplit(t, dir, src)
			if out != c.want {
				t.Errorf("stdout = %q, want %q", out, c.want)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
			if got := strings.Contains(errs, "bad pattern"); got != c.wantErr {
				t.Errorf("stderr = %q, want a complaint: %v", errs, c.wantErr)
			}
		})
	}
}
