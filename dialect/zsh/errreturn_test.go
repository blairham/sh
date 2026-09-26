// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// `ERR_RETURN` makes a failing command execute an implicit `return`.
//
// It was accepted and then ignored until #4546 — the name went into the
// recorded store, `[[ -o errreturn ]]`, `$options[errreturn]` and the `setopt`
// listing all reported it faithfully, and the shell ran straight past the
// command that failed. That is what these rows are written to catch: every
// case is run in **both** states of the option, so a shell that ignores it
// fails one half of every pair rather than passing a row that only ever asked
// it one question.
//
// Measured against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), which `go version -m` calls *not a Go
// executable* — run `-f` with each snippet in a file of its own, 2026-09-25.
//
// The two states are written as `setopt localoptions errreturn` and
// `localoptions noerrreturn` inside the function rather than as a line at the
// top, because with the option on at the *top level* as well the script ends
// at the call and there is nothing after it to read. That is not a workaround:
// it is the next test's subject.
func TestErrReturnTakesAnImplicitReturn(t *testing.T) {
	// %s is where the one word the two states differ by goes.
	for _, c := range []struct{ name, src, on, off string }{
		{
			// The issue's own reduction.
			name: "a function returns at the command that failed",
			src: "f() { setopt localoptions %s; false; print notreached }\n" +
				"f\n" +
				"print \"after f: status=$?\"\n" +
				"print done\n",
			on:  "after f: status=1\ndone\n",
			off: "notreached\nafter f: status=0\ndone\n",
		},
		{
			// The status is the failing command's own and not a 1 of the
			// mechanism's making.
			name: "with the failing command's status",
			src: "f() { setopt localoptions %s; (exit 42); print notreached }\n" +
				"f\n" +
				"print \"st=$?\"\n",
			on:  "st=42\n",
			off: "notreached\nst=0\n",
		},
		{
			// **The option is read at each level.** With it on in the
			// callee alone, the caller judges a failing statement with the
			// option off and runs on — which is what says this is a
			// judgement per level rather than an unwind that continues
			// until something catches it.
			name: "the caller reads the option for itself",
			src: "g() { setopt localoptions %s; false; print g-after }\n" +
				"f() { g; print f-after }\n" +
				"f\n" +
				"print \"post st=$?\"\n",
			on:  "f-after\npost st=0\n",
			off: "g-after\nf-after\npost st=0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, word, want string }{
				{"on", "errreturn", c.on},
				{"off", "noerrreturn", c.off},
			} {
				t.Run(state.name, func(t *testing.T) {
					out, st := runZsh(t, t.TempDir(), fmt.Sprintf(c.src, state.word))
					if out != state.want || st != 0 {
						t.Errorf("got %q status %d, want %q at 0", out, st, state.want)
					}
				})
			}
		})
	}
}

// And the caller takes one too, all the way out. Written apart from the pairs
// above because with the option on at the top level the *script* is what ends,
// so the two states differ in their status rather than in a line of output.
//
// Measured 2026-09-25: `setopt errreturn` ahead of a two-deep call writes
// nothing at all and leaves 1, where `unsetopt errreturn` ahead of the same
// three lines writes all three and leaves 0.
func TestEveryLevelInTurnTakesTheImplicitReturn(t *testing.T) {
	const body = "g() { false; print g-after }\n" +
		"f() { g; print f-after }\n" +
		"f\n" +
		"print \"post st=$?\"\n"
	for _, c := range []struct {
		name, setopt, want string
		status             int
	}{
		{"on", "setopt errreturn\n", "", 1},
		{"off", "unsetopt errreturn\n", "g-after\nf-after\npost st=0\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.setopt+body)
			if out != c.want || st != c.status {
				t.Errorf("got %q status %d, want %q at %d", out, st, c.want, c.status)
			}
		})
	}
}

// **The noun is `return` and not "the enclosing function".** Where the
// implicit transfer lands is `return`'s own question, and these are the places
// that part the two readings: neither row has a function in it, and a rule
// about functions predicts that both run on.
//
// Each row is the same shape twice — once with the option on and the failure
// implicit, once with a written `return` and no option anywhere — and the two
// have to agree byte for byte. Measured on zsh 5.9.2, 2026-09-25.
func TestTheImplicitReturnLandsWhereAWrittenOneDoes(t *testing.T) {
	for _, c := range []struct {
		name, implicit, written, want string
		status                        int
	}{
		{
			// At the top level of a script it ends the script, which is
			// what `return` does there in this shell.
			name:     "the top level of a script ends the script",
			implicit: "setopt errreturn\nprint start\nfalse\nprint notreached\n",
			written:  "print start\nreturn 1\nprint notreached\n",
			want:     "start\n", status: 1,
		},
		{
			// Inside a subshell it ends the subshell and the script goes
			// on, reading the status the subshell left.
			name:     "a subshell ends the subshell",
			implicit: "( setopt errreturn; print in-sub; false; print notreached )\nprint \"after sub st=$?\"\n",
			written:  "( print in-sub; return 1; print notreached )\nprint \"after sub st=$?\"\n",
			want:     "in-sub\nafter sub st=1\n", status: 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, side := range []struct{ name, src string }{
				{"implicit", c.implicit}, {"written", c.written},
			} {
				t.Run(side.name, func(t *testing.T) {
					out, st := runZsh(t, t.TempDir(), side.src)
					if out != c.want || st != c.status {
						t.Errorf("got %q status %d, want %q at %d", out, st, c.want, c.status)
					}
				})
			}
		})
	}
}

// The third landing place, kept apart because it needs a file on disk: a
// sourced file ends at the failure, the way a written `return` ends one.
//
// The caller then judges a `source` that reported 1 and — with the option
// still on there — takes a return of its own, which is why `f-after` is absent
// as well as `notreached`. Measured on zsh 5.9.2, 2026-09-25.
func TestTheImplicitReturnEndsASourcedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inner.zsh")
	if err := os.WriteFile(path, []byte("print in-src\nfalse\nprint notreached\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, word, want string }{
		{"on", "errreturn", "in-src\nafter st=1\n"},
		{"off", "noerrreturn", "in-src\nnotreached\nf-after\nafter st=0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, dir, "f() { setopt localoptions "+c.word+
				"; source "+path+"; print f-after }\nf\nprint \"after st=$?\"\n")
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
	// And from the top level the same file ends the shell, because the
	// statement that sourced it is judged too.
	out, st := runZsh(t, dir, "setopt errreturn\nsource "+path+"\nprint notreached\n")
	if want := "in-src\n"; out != want || st != 1 {
		t.Errorf("at the top level: got %q status %d, want %q at 1", out, st, want)
	}
}

// **A tested context exempts the implicit return only as far as the call it
// was opened in**, where `set -e` and the ERR trap inherit theirs all the way
// down.
//
// This is the pair that holds the *shape* fixed and moves only which option is
// on: the same `if f; then` over the same body, `errexit` on one side and
// `errreturn` on the other. A rule stated as "a tested context exempts the
// judgement" is right for `set -e` in every column and wrong here, and it is
// wrong in exactly these two cells. Measured on zsh 5.9.2, 2026-09-25.
func TestATestedContextExemptsTheImplicitReturnOnlyToTheCall(t *testing.T) {
	const body = "f() { false; print inner }\n" +
		"if f; then print then; else print else; fi\n" +
		"print after\n"
	for _, c := range []struct{ name, setopt, want string }{
		// `set -e`: the exemption reaches into the body, so `inner` runs
		// and the condition succeeds.
		{"errexit inherits the exemption", "setopt errexit\n", "inner\nthen\nafter\n"},
		// `ERR_RETURN`: the call resets it, so the body returns at the
		// `false` and the condition fails.
		{"errreturn resets it at the call", "setopt errreturn\n", "else\nafter\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.setopt+body)
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
	// And **a call is the only boundary that moves it**: a subshell under
	// the same condition keeps the caller's exemption and runs on, so the
	// reset is not "any new context" — measured the same day.
	out, st := runZsh(t, t.TempDir(),
		"setopt errreturn\nif ( false; print sub ); then print then; else print else; fi\nprint after\n")
	if want := "sub\nthen\nafter\n"; out != want || st != 0 {
		t.Errorf("a subshell: got %q status %d, want %q at 0", out, st, want)
	}
}

// **An ERR trap takes the failure, and a level that did not take it does not
// return.** With a trap set exactly one level takes the implicit return and
// every caller above it runs on; with none, every level in turn takes one.
//
// Measured on zsh 5.9.2, 2026-09-25, over one three-deep snippet run three
// ways. The `[E]` line is what says the trap is the thing that moved and not
// the nesting.
func TestAnErrTrapTakesTheFailureAndStopsThePropagation(t *testing.T) {
	const body = "h() { false; print h-after }\n" +
		"g() { h; print g-after }\n" +
		"f() { g; print f-after }\n" +
		"f\n" +
		"print \"post st=$?\"\n"
	for _, c := range []struct {
		name, trap, want string
		status           int
	}{
		// No trap: `h`, `g`, `f` and the top level each judge a failing
		// statement and each takes a return, so nothing is written at all.
		{"with no trap every level returns", "", "", 1},
		// A trap: `h` returns, and `g` — judging a failure a handler has
		// already taken — runs on, as does everything above it.
		{"with a trap exactly one level returns", "trap 'print \"[E]\"' ERR\n", "[E]\ng-after\nf-after\npost st=0\n", 0},
		// And an **ignored** trap takes it just the same, with nothing to
		// show for it. This is the row Runner.errTrapFired cannot carry,
		// because that flag records a firing and this trap never fires.
		{"and an ignored trap takes it too", "trap \"\" ERR\n", "g-after\nf-after\npost st=0\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), "setopt errreturn\n"+c.trap+body)
			if out != c.want || st != c.status {
				t.Errorf("got %q status %d, want %q at %d", out, st, c.want, c.status)
			}
		})
	}
	// The trap is not a blanket exemption: at the level of the failure
	// itself it fires *and* the return is taken, which is what keeps the
	// rows above from reading as "a trap turns the option off".
	out, st := runZsh(t, t.TempDir(),
		"setopt errreturn\ntrap 'print \"[E]\"' ERR\nf() { false; print notreached }\nf\nprint \"post st=$?\"\n")
	if want := "[E]\npost st=1\n"; out != want || st != 0 {
		t.Errorf("at the failure's own level: got %q status %d, want %q at 0", out, st, want)
	}
}

// `set -e` outranks the implicit return where a shell has both on, whichever
// of the two the script set globally and whichever it set in the function.
//
// Measured on zsh 5.9.2, 2026-09-25: all three end the shell at 1 with nothing
// written, where the implicit return alone would have left the function and
// written `post st=1` at 0.
func TestErrExitOutranksTheImplicitReturn(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"both set in the function", "f() { setopt localoptions errexit errreturn; false; print x }\n"},
		{"errexit global, errreturn local", "setopt errexit\nf() { setopt localoptions errreturn; false; print x }\n"},
		{"errreturn global, errexit local", "setopt errreturn\nf() { setopt localoptions errexit; false; print x }\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src+"f\nprint \"post st=$?\"\n")
			if out != "" || st != 1 {
				t.Errorf("got %q status %d, want %q at 1", out, st, "")
			}
		})
	}
}

// The reporting half was already right and must not move: `setopt` names it,
// `[[ -o ]]` follows it, both spellings are one state, and `set -o` reaches
// it. `emulate sh` and `emulate ksh` leave it **off**, which is what says this
// option's reach does not widen under an emulation the way `POSIX_TRAPS` does
// (#4547).
func TestErrReturnIsStillReportedAsItWas(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the condition follows it", "setopt errreturn\n[[ -o errreturn ]] && print on || print off\n", "on\n"},
		{"and back again", "setopt errreturn\nunsetopt errreturn\n[[ -o errreturn ]] && print on || print off\n", "off\n"},
		{"the listing names it", "setopt errreturn\nsetopt\n", "errreturn\nnohashdirs\n"},
		{"the underscore spelling is the same name", "setopt ERR_RETURN\n[[ -o err_return ]] && print on || print off\n", "on\n"},
		{"`set -o` reaches it", "set -o errreturn\n[[ -o errreturn ]] && print on || print off\n", "on\n"},
		{"emulate sh leaves it off", "emulate sh\n[[ -o errreturn ]] && print on || print off\n", "off\n"},
		{"and so does emulate ksh", "emulate ksh\n[[ -o errreturn ]] && print on || print off\n", "off\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// **At which moment the option is read**, which is a question a conversion of
// this shape can get wrong without any row looking odd: the candidates are
// when the function is defined, when it is entered, and when the failing
// statement is judged.
//
// It is the last of the three, and each row below holds two of the moments
// fixed and moves the option between them. Measured on zsh 5.9.2, 2026-09-25:
//
//   - off when the body was written and on when it runs: it fires, so the
//     definition is not the moment;
//   - on when the call is entered and off by the time the command fails: it
//     does not fire, so entry is not the moment either;
//   - and the complement, off at entry and on at the command, fires.
//
// The last pair is the sharpest, because it is the *caller's* judgement that
// moves: a callee that turns the option on and then fails leaves the caller
// judging its call with the option on, and the caller returns; a callee that
// turns it off leaves the caller running on. Reading the axis when the
// statement *began* rather than when it is judged answers both of those the
// other way round.
func TestTheOptionIsReadWhenTheFailingStatementIsJudged(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			// The definition is not the moment.
			name: "off when the body was written, on when it runs",
			src:  "f() { false; print notreached }\nsetopt errreturn\nf\nprint \"st=$?\"\n",
			want: "", status: 1,
		},
		{
			// Entry is not the moment: the body turns it off on its first
			// line and the `false` after it is judged with it off.
			name: "on at entry, off by the time the command fails",
			src:  "setopt errreturn\nf() { unsetopt errreturn; false; print reached }\nf\nprint \"st=$?\"\n",
			want: "reached\nst=0\n", status: 0,
		},
		{
			// And the complement, which a rule keyed on entry gets wrong in
			// the other direction.
			name: "off at entry, on by the time the command fails",
			src:  "unsetopt errreturn\nf() { setopt localoptions errreturn; false; print notreached }\nf\nprint \"st=$?\"\nprint after\n",
			want: "st=1\nafter\n", status: 0,
		},
		{
			// Two failures in one body with the option turned on between
			// them: the first runs on and the second returns, which is the
			// same rule read twice in one frame.
			name: "turned on between two failures in one body",
			src:  "unsetopt errreturn\nf() { false; print one; setopt localoptions errreturn; false; print notreached }\nf\nprint \"st=$?\"\nprint after\n",
			want: "one\nst=1\nafter\n", status: 0,
		},
		{
			// **The caller reads it at its own judgement**, after whatever
			// the callee left behind. Here the callee turns it on and fails,
			// so the caller — which had it off when the statement began —
			// judges the call with it on and returns.
			name: "the callee turns it on and the caller returns",
			src:  "unsetopt errreturn\ng() { setopt errreturn; return 1 }\ng\nprint \"after st=$?\"\n",
			want: "", status: 1,
		},
		{
			// And the mirror: the callee turns it off, so the caller runs
			// on although it had the option when the statement began.
			name: "the callee turns it off and the caller runs on",
			src:  "setopt errreturn\ng() { unsetopt errreturn; return 1 }\ng\nprint \"after st=$?\"\n",
			want: "after st=1\n", status: 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != c.status {
				t.Errorf("got %q status %d, want %q at %d", out, st, c.want, c.status)
			}
		})
	}
}
