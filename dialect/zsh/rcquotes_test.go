// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `RC_QUOTES` makes a doubled `'` inside a single-quoted string stand for one
// literal quote, with the string carrying on: `””` is one quote character
// and `'a”b'` is `a'b`. It was accepted and then ignored until #4591 — the
// name went into the recorded store, `[[ -o rcquotes ]]` reported it back
// faithfully, and `””` was an empty word in both states.
//
// **It is a parser question**, which is what separates it from the other
// option conversions in this campaign: the option decides how a *later* word
// is **lexed**, so it is syntax.Dialect.DoubledQuoteInSingleQuotesIsALiteral-
// Quote rather than an axis on the semantics vector, and the reach — which
// text is read after it moves — is part of the behavior rather than an
// artifact of a probe.
//
// Every case here runs the snippet in **both** states, because a shell that
// ignores the option passes one half of every pair.
//
// Measured against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — with `-f` and each snippet in a file of its
// own, 2026-09-26. `go version -m` says *not a Go executable* for it.
func TestRcQuotesReadsADoubledQuoteAsOneQuote(t *testing.T) {
	for _, c := range []struct{ name, word, on, off string }{
		// The issue's own reduction, and the shape that makes the rule
		// visible without the word being nothing but quotes.
		{"four quotes", `''''`, "'\n", "\n"},
		{"a quote between two letters", `'a''b'`, "a'b\n", "ab\n"},
		{"two quotes", `''`, "\n", "\n"},
		{"six quotes", `''''''`, "''\n", "\n"},
		{"two pairs inside a run", `'x''''y'`, "x''y\n", "xy\n"},
		{"a run that begins mid-word", `a''''b`, "a'b\n", "ab\n"},
		// The flag reaches the plain run and nothing else. A `''` inside
		// double quotes, inside a here-document body and inside `$'…'` is
		// two ordinary characters in both states.
		{"inside double quotes", `"a''b"`, "a''b\n", "a''b\n"},
		// `$'a'` closes at the first quote and `'b\tc'` is an ordinary run,
		// so the backslash and the `t` survive either way — where a reading
		// that reached inside the dollar-quote would have given a tab.
		{"inside a dollar-single-quote", `$'a''b\tc'`, "ab\\tc\n", "ab\\tc\n"},
		// And the row that says the option really was on in that run: the
		// same text with one more pair, where the pair is a *plain* run
		// after the dollar-quote has closed.
		{"a plain pair after a dollar-quote", `$'p''''q'`, "p'q\n", "pq\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, setopt, want string }{
				{"on", "setopt rcquotes\n", c.on},
				{"off", "unsetopt rcquotes\n", c.off},
			} {
				t.Run(state.name, func(t *testing.T) {
					dir := t.TempDir()
					out, st := runZshSourcing(t, dir, state.setopt+"print -r -- "+c.word+"\n")
					if out != state.want || st != 0 {
						t.Errorf("got %q status %d, want %q at 0", out, st, state.want)
					}
				})
			}
		})
	}
}

// An **odd** number of quotes runs off the end of the input in both states,
// and it is the same refusal. This is the control that separates "a doubled
// quote is one quote" from "a quote inside a run is taken as text": a reading
// of the second kind would swallow the last quote of `”'` and print a word.
func TestRcQuotesLeavesAnOddNumberOfQuotesUnterminated(t *testing.T) {
	for _, word := range []string{`'''`, `'''''`, `x'''y`} {
		t.Run(word, func(t *testing.T) {
			for _, setopt := range []string{"setopt rcquotes\n", "unsetopt rcquotes\n"} {
				dir := t.TempDir()
				out, st := runZshSourcing(t, dir, setopt+"print -r -- "+word+"\n")
				if st == 0 || !strings.Contains(out, "unmatched") {
					t.Errorf("%q under %q: %q at %d, want the run refused", word, setopt, out, st)
				}
			}
		})
	}
}

// The moment the option is read is the one the text is **lexed** at, never
// the one a word is expanded at — and the two are far apart, since a function
// body is read once at its definition and expanded at every call.
//
// Each row holds the state at the *call* fixed at the other's value, so the
// call cannot be what decides. That is the pair #4549 had to draw for the
// option one line along in the table, `rcexpandparam`, and it comes out the
// other way here: that one is read at expansion and this one at the read.
//
// Measured on zsh 5.9.2, `-f`, 2026-09-26.
func TestRcQuotesIsReadWhenTheWordIsLexed(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a function defined on and called off",
			"setopt rcquotes\nf() { print -r -- 'a''b' }\nunsetopt rcquotes\nf\n",
			"a'b\n",
		},
		{
			"a function defined off and called on",
			"unsetopt rcquotes\nf() { print -r -- 'a''b' }\nsetopt rcquotes\nf\n",
			"ab\n",
		},
		{
			// `eval` reads its text when it runs, so the state at the
			// `eval` is what the text is lexed with — the same rule, at a
			// different moment.
			"eval reads its text at the eval",
			"unsetopt rcquotes\ns=\"print -r -- 'a''b'\"\nsetopt rcquotes\neval $s\n",
			"a'b\n",
		},
		{
			"and with the option off at the eval",
			"setopt rcquotes\ns=\"print -r -- 'a''b'\"\nunsetopt rcquotes\neval $s\n",
			"ab\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			out, st := runZshSourcing(t, dir, c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// An alias body is stored as text and read at its **use**, so it answers to
// the state there rather than to the state at the `alias` — the same rule as
// the rows above, reached by the one route that keeps the text as text.
//
// Apart from them because it needs the alias tables in the parser's hands,
// which is a second thing a front end switches on: see
// dialecttest.Preset.CombinedThroughTheAliases for why pasting the snippet in
// front of a plain run would have measured nothing.
//
// Measured on zsh 5.9.2, `-f`, 2026-09-26.
func TestRcQuotesReachesAnAliasBodyAtItsUse(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"defined with the option off and used with it on",
			"unsetopt rcquotes\nalias al=\"print -r -- 'a''b'\"\nsetopt rcquotes\nal\n",
			"a'b\n",
		},
		{
			"defined with it on and used with it off",
			"setopt rcquotes\nalias al=\"print -r -- 'a''b'\"\nunsetopt rcquotes\nal\n",
			"ab\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "inc.zsh")
			if err := os.WriteFile(path, []byte(c.src), 0o600); err != nil {
				t.Fatal(err)
			}
			out, st, err := preset.CombinedThroughTheAliases(t,
				dialecttest.Base{Dir: dir, Vars: map[string]string{"PATH": dir}}, ". "+path+"\n")
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// The signature of an answer read at the *lexing*, and the reason a probe
// written with `-c` reports no difference and is wrong: text read in one go
// is read with the state the option had before any of it ran.
//
// Measured on zsh 5.9.2, `-f`, 2026-09-26: `setopt rcquotes; print -r --
// 'a”b'` on one line of a script file prints `ab`, and so does the same two
// lines handed to `-c` — where `zsh -f -o rcquotes -c` on the word alone
// prints `a'b`, because there the option moved before the read.
func TestRcQuotesDoesNotReachTextAlreadyRead(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the setopt and the word on one line", "setopt rcquotes; print -r -- 'a''b'\n", "ab\n"},
		// The control beside it: the option really did move, so a word read
		// *after* that line is read the new way. Same file, one more line.
		{
			"and the line after it",
			"setopt rcquotes; print -r -- 'a''b'\neval \"print -r -- 'c''d'\"\n",
			"ab\nc'd\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// The reporting half already worked and must not be traded away for the
// behavior, so it is asked here beside it — and so is the state a
// subshell and a `localoptions` function leave behind, which is what the
// option being the *dialect's* rather than a bit in the recorded store buys.
//
// Measured on zsh 5.9.2, `-f`, 2026-09-26.
func TestRcQuotesIsScopedAndReportedLikeAnOption(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the option reports itself",
			"setopt rcquotes\n[[ -o rcquotes ]] && print reported\nprint -r -- \"$options[rcquotes]\"\n",
			"reported\non\n",
		},
		{
			"and reports itself off",
			"unsetopt rcquotes\n[[ -o rcquotes ]] || print not-reported\nprint -r -- \"$options[rcquotes]\"\n",
			"not-reported\noff\n",
		},
		{
			"a subshell keeps its change to itself",
			"unsetopt rcquotes\n( setopt rcquotes; [[ -o rcquotes ]] && print IN-ON )\n" +
				"[[ -o rcquotes ]] && print OUT-ON || print OUT-OFF\n",
			"IN-ON\nOUT-OFF\n",
		},
		{
			// `localoptions` puts it back at the return, so a word lexed
			// after the call is read the old way again.
			"localoptions puts it back at the return",
			"unsetopt rcquotes\nh() { setopt localoptions rcquotes; print in-h }\nh\nprint -r -- 'a''b'\n",
			"in-h\nab\n",
		},
		{
			// The control for the row above: the same body without
			// `localoptions` leaves the option on.
			"and without it the change stands",
			"unsetopt rcquotes\nh() { setopt rcquotes; print in-h }\nh\nprint -r -- 'a''b'\n",
			"in-h\na'b\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			out, st := runZshSourcing(t, dir, c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// No emulation reaches it, which is the row that says this one is opt-in
// where `posixtraps` is not. `rcquotes` is in emulationAlwaysReset and has no
// entry in the per-emulation defaults, so all three modes put it back off.
//
// Both halves are asked — what the emulation does to the option, and what the
// shell then does with a word — because a row that only read the option back
// could pass in a shell that ignores it.
//
// Measured on zsh 5.9.2, `-f`, 2026-09-26.
func TestNoEmulationTurnsRcQuotesOn(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"emulate sh  ; [[ -o rcquotes ]] || print 'sh: off'\n"+
			"emulate ksh ; [[ -o rcquotes ]] || print 'ksh: off'\n"+
			"emulate zsh ; [[ -o rcquotes ]] || print 'zsh: off'\n")
	if want := "sh: off\nksh: off\nzsh: off\n"; out != want || st != 0 {
		t.Errorf("reporting: got %q status %d, want %q at 0", out, st, want)
	}
	for _, head := range []string{"emulate sh\n", "emulate ksh\n", "emulate zsh\n", "emulate -R sh\n"} {
		t.Run(strings.TrimSpace(head), func(t *testing.T) {
			dir := t.TempDir()
			out, st := runZshSourcing(t, dir, "setopt rcquotes\n"+head+"print -r -- 'a''b'\n")
			if want := "ab\n"; out != want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, want)
			}
		})
	}
}

// runZshSourcing runs src as a file a `.` reads, which is the one route in
// this package where a line can decide how the next one is read.
//
// It is needed because the helpers beside it parse the whole snippet before a
// line of it runs, which is the right shape for a question about what a word
// *means* and the wrong one for a question about when the reading is decided.
// Measured on zsh 5.9.2 the two routes disagree, and that disagreement is the
// behavior: a `setopt` and a word read together are read the old way, and a
// sourced file — a real rc file, that is — is read a command at a time.
func runZshSourcing(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	path := filepath.Join(dir, "inc.zsh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return runZsh(t, dir, ". "+path+"\n")
}
